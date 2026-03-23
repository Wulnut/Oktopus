# Fix Plan: Code Review Issues

> Based on `docs/COMMIT_REVIEW.md` and `docs/COMMIT_REVIEW_RESPONSE.md`

27 issues total: 4 HIGH, 10 MEDIUM, 12 LOW, 1 INFO.
Grouped into 8 tasks by file/domain affinity. Tasks are ordered by priority.

---

## Task 1: Mass Actions — race condition, partition bug, input validation, USP error checking

**Priority:** HIGH
**Files:**
- `backend/services/controller/internal/api/mass_actions.go`

### Step 1: Fix race condition on shared MassAction struct

The `ma` struct is shared across goroutines. `mu.Unlock()` is called before `UpdateMassAction()`, so concurrent goroutines can overwrite each other's `DeviceResults`.

**Fix:** Use atomic MongoDB array updates instead of replacing the entire document.

In `runMassFirmwareUpdate` and `runMassScriptExecution`, replace the pattern:
```go
mu.Lock()
ma.DeviceResults[idx] = result
mu.Unlock()
// ... modify ma.Progress, ma.SuccessCount, etc.
a.db.UpdateMassAction(ctx, ma.ID, ma)  // RACE: full doc replace
```

With atomic per-device updates:
```go
a.db.UpdateMassActionDevice(ctx, ma.ID, idx, result)
a.db.IncrementMassActionProgress(ctx, ma.ID, success)
```

Add two new DB methods in `db/mass_actions.go`:
```go
// UpdateMassActionDevice atomically updates a single device result by index
func (d *Database) UpdateMassActionDevice(ctx context.Context, id primitive.ObjectID, idx int, result DeviceResult) error {
    field := fmt.Sprintf("device_results.%d", idx)
    _, err := d.massActions.UpdateByID(ctx, id, bson.M{
        "$set": bson.M{field: result},
    })
    return err
}

// IncrementMassActionProgress atomically increments progress counters
func (d *Database) IncrementMassActionProgress(ctx context.Context, id primitive.ObjectID, success bool) error {
    inc := bson.M{"progress": 1}
    if success {
        inc["success_count"] = 1
    } else {
        inc["failure_count"] = 1
    }
    _, err := d.massActions.UpdateByID(ctx, id, bson.M{"$inc": inc})
    return err
}
```

Remove the `mu` mutex from both run functions — it's no longer needed.

Keep the final status update (`completed`/`failed`) as a single `UpdateMassAction` call after all goroutines finish (no concurrency at that point).

### Step 2: Fix partition extraction for multi-digit numbers

In `findAvailablePartition`, replace:
```go
if len(resolvedPath) >= 2 {
    return resolvedPath[len(resolvedPath)-2:]
}
```

With:
```go
// resolvedPath is like "Device.DeviceInfo.FirmwareImage.1." — split by "." and take last non-empty segment
parts := strings.Split(strings.TrimSuffix(resolvedPath, "."), ".")
if len(parts) > 0 {
    return parts[len(parts)-1]
}
```

Add `"strings"` to imports if not present.

### Step 3: Cap DeviceSNs array size

At the top of `massFirmwareUpdate` and `massScriptExecution`, after parsing the request body, add:
```go
const maxDevices = 500
if len(req.DeviceSNs) > maxDevices {
    http.Error(w, fmt.Sprintf("Too many devices: %d (max %d)", len(req.DeviceSNs), maxDevices), http.StatusBadRequest)
    return
}
```

### Step 4: Check USP response for errors in performFirmwareUpdate

After the `bridge.NatsUspInteraction` call for the Download() operate, inspect the response:
```go
resp, err := bridge.NatsUspInteraction(...)
if err != nil {
    return fmt.Errorf("firmware download command failed: %w", err)
}
// Check USP-level error in response
if resp.StatusCode >= 400 {
    return fmt.Errorf("device rejected firmware command: status %d", resp.StatusCode)
}
```

Adapt to the actual `NatsUspInteraction` return type — check the response body for USP error codes if applicable.

### Step 5: Verify

```bash
sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build controller 2>&1"
```

---

## Task 2: Firmware upload — auth, filename sanitization, streaming, hash

**Priority:** HIGH (auth, filename) + MEDIUM (streaming, hash)
**Files:**
- `backend/services/controller/internal/api/firmware.go`
- `backend/services/utils/firmware-upload/firmware-upload.js`

### Step 1: Validate JWT in firmware-upload.js

Replace the presence-only check with actual JWT verification. Install `jsonwebtoken`:
```bash
cd backend/services/utils/firmware-upload && npm install jsonwebtoken
```

Replace auth check:
```javascript
const jwt = require('jsonwebtoken');
// ...
const token = req.headers['authorization'];
if (!token) {
    res.writeHead(401, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: 'Unauthorized' })); return;
}
try {
    jwt.verify(token.replace('Bearer ', ''), process.env.JWT_SECRET || 'default_secret');
} catch (e) {
    res.writeHead(401, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: 'Invalid token' })); return;
}
```

Note: The JWT secret must match the controller's secret. Pass it via environment variable in docker-compose.

### Step 2: Sanitize filename in firmware.go

After `header.Filename` is read (line ~91):
```go
fileName = filepath.Base(header.Filename)
// Reject filenames with path separators or suspicious patterns
if fileName == "." || fileName == ".." || strings.ContainsAny(fileName, `/\`) {
    http.Error(w, "Invalid filename", http.StatusBadRequest)
    return
}
```

### Step 3: Stream firmware to disk instead of buffering

Replace `io.ReadAll` with streaming to temp file:
```go
h := sha256.New()  // also fixes MD5 issue
tmpFile, err := os.CreateTemp(os.TempDir(), "firmware-*")
if err != nil {
    http.Error(w, "Failed to create temp file", http.StatusInternalServerError)
    return
}
defer os.Remove(tmpFile.Name())
defer tmpFile.Close()

written, err := io.Copy(tmpFile, io.TeeReader(file, h))
if err != nil {
    http.Error(w, "Failed to write firmware", http.StatusInternalServerError)
    return
}
fingerprint = hex.EncodeToString(h.Sum(nil))
```

This also replaces MD5 with SHA-256. Update the import from `"crypto/md5"` to `"crypto/sha256"`.

### Step 4: URL-encode filename in delete call

In `deleteFileFromUploadService` (line ~258):
```go
import "net/url"
// ...
fmt.Sprintf("%s/delete?name=%s", firmwareUploadServiceURL, url.QueryEscape(fileName))
```

### Step 5: Verify

Build controller and firmware-upload service.

---

## Task 3: Infrastructure — port exposure, nginx proxying, firmware URL

**Priority:** MEDIUM
**Files:**
- `deploy/compose/docker-compose.yaml`
- `deploy/compose/nginx.conf`
- `deploy/compose/.env.controller`

### Step 1: Remove firmware-upload host port mapping

In `docker-compose.yaml`, change the firmware-upload service ports from `"8006:8006"` to only expose internally (remove the `ports` directive or use `expose`):
```yaml
firmware-upload:
  # Remove: ports: ["8006:8006"]
  expose:
    - "8006"
```

### Step 2: Replace host.docker.internal with Docker service names

In `nginx.conf`, replace `host.docker.internal` references with Docker service names:
- `host.docker.internal:8000` → `controller:8000`
- `host.docker.internal:8004` → `file-server:8004`
- `host.docker.internal:8005` → `container-upload:8005`
- `host.docker.internal:8006` → `firmware-upload:8006`

This requires nginx to be on the same Docker network as the backend services. Verify in docker-compose.yaml.

### Step 3: Make FIRMWARE_BASE_URL configurable

In `.env.controller`, add a comment explaining the variable must be set to the public-facing URL:
```env
# Set to the public URL where devices can download firmware
# For local development: http://localhost/firmwares
# For production: https://your-domain.com/firmwares
FIRMWARE_BASE_URL=http://localhost/firmwares
```

No code change needed — the variable is already read from env.

### Step 4: Verify

```bash
sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && ./run_debug.sh"
```
Verify nginx can reach backend services by name.

---

## Task 4: Frontend — err_code handling, bridging tab fixes, auth consistency

**Priority:** MEDIUM
**Files:**
- `frontend/src/sections/devices/usp/devices-network.js`
- `frontend/src/sections/devices/usp/devices-topology.js`
- `frontend/src/sections/devices/usp/devices-bridging.js`
- `frontend/src/sections/devices/usp/devices-info.js`

### Step 1: Fix err_code 0 treated as error

In `devices-network.js`, change `isAllErrors`:
```javascript
const isAllErrors = (data) => {
  if (!data?.req_path_results?.length) return false;
  return data.req_path_results.every(r => r.err_code != null && r.err_code !== 0);
};
```

In `devices-topology.js`, apply the same fix to `hasPathError` (same pattern).

### Step 2: Bridging tab — replace direct fetch with httpRequest

In `devices-bridging.js`, replace `uspGet`, `uspSet`, `uspAdd`, `uspDel` to use `httpRequest` from `useBackendContext()`:
```javascript
const { httpRequest } = useBackendContext();

const uspGet = useCallback(async (paramPaths, maxDepth = 3) => {
  const body = JSON.stringify({ param_paths: paramPaths, max_depth: maxDepth });
  const { status, result } = await httpRequest(`/api/device/${sn}/${mtp}/get`, 'PUT', body);
  return status === 200 ? result : null;
}, [sn, mtp, httpRequest]);
```

Apply same pattern to `uspSet`, `uspAdd`, `uspDel`. Remove manual `Headers` construction and `localStorage.getItem("token")` calls.

### Step 3: Bridging tab — reverse port move order (add-then-delete)

In `handleMovePort`, add the port to the target bridge first, then delete from source:
```javascript
const handleMovePort = async (fromBridgeIdx, port, toBridgeIdx) => {
  // ...
  // Add to target bridge first
  const addResult = await uspAdd(`Device.Bridging.Bridge.${toBridgeIdx}.Port.`, params);
  if (!addResult) {
    setAlert({ severity: 'error', message: 'Failed to add port to target bridge. No changes made.' });
    return;
  }
  // Only delete from source if add succeeded
  await uspDel([`Device.Bridging.Bridge.${fromBridgeIdx}.Port.${port._idx}.`]);
  await fetchAll();
};
```

### Step 4: Bridging tab — add confirmation dialogs

Add a confirmation dialog (reuse the `ConfirmDialog` pattern from `devices-info.js`) before `handleDeletePort` and `handleMovePort`.

### Step 5: Bridging tab — MAC address validation

In `handleSaveMAC`, validate before sending:
```javascript
const macRegex = /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$/;
if (!macRegex.test(newMAC)) {
  setAlert({ severity: 'error', message: 'Invalid MAC address format. Use XX:XX:XX:XX:XX:XX.' });
  return;
}
```

### Step 6: Remove manual auth header from handleFwDeploy

In `devices-info.js`, `handleFwDeploy` — remove the manual `headers` construction:
```javascript
const handleFwDeploy = async (downloadUrl) => {
  if (!downloadUrl) return;
  setFwLoading(true);
  try {
    const body = JSON.stringify({ Url: downloadUrl });
    const { status } = await httpRequest(`/api/device/${sn}/${mtp}/fw_update`, 'PUT', body);
    // ...
  } catch {
    setAlert({ severity: 'error', message: 'Firmware update failed. Network error.' });
  } finally {
    setFwLoading(false);
  }
};
```

This also adds the missing `catch` block for network errors.

### Step 7: Verify

```bash
sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build frontend 2>&1"
```

---

## Task 5: Scripts — async execution, loop cap, error handling

**Priority:** MEDIUM
**Files:**
- `backend/services/controller/internal/api/scripts.go`

### Step 1: Add hard cap on total iterations

Replace the loop detection check:
```go
// Before:
if visitCounts[stepIndex] > len(script.Steps) {

// After:
const maxTotalIterations = 500
totalIterations++
if totalIterations > maxTotalIterations {
    execution.Status = "failed"
    execution.Error = fmt.Sprintf("Exceeded maximum iterations (%d)", maxTotalIterations)
    break
}
if visitCounts[stepIndex] > len(script.Steps) {
```

Add `totalIterations := 0` before the loop.

### Step 2: Check UpdateExecution error

At line ~799:
```go
if err := a.db.UpdateExecution(r.Context(), execution.ID, execution); err != nil {
    log.Printf("executeScriptHandler: failed to update execution %s: %v", execution.ID.Hex(), err)
}
```

### Step 3: Make script execution async (optional — larger refactor)

This is a bigger change. The pattern would match mass actions:
1. Create execution record with status "running"
2. Return execution ID immediately (HTTP 202)
3. Run steps in a goroutine
4. Frontend polls `GET /api/scripts/{id}/executions/{execId}` for status

This is already how mass-action script execution works via `executeScriptForDevice`. The single-device `executeScriptHandler` should be refactored to use the same pattern.

**Note:** This step is optional for this plan. The synchronous approach works for short scripts and the mass-actions path already handles async. Consider implementing if script execution times become problematic.

### Step 4: Verify

Build controller.

---

## Task 6: Metrics and DB error handling

**Priority:** LOW
**Files:**
- `backend/services/controller/internal/db/metrics.go`
- `backend/services/controller/internal/api/firmware.go`

### Step 1: Check cursor.All error in GetDeviceMetricsHistory

In `db/metrics.go`, change:
```go
cursor.All(ctx, &results)
return results, nil
```
To:
```go
if err := cursor.All(ctx, &results); err != nil {
    return nil, err
}
return results, nil
```

### Step 2: Check MatchedCount in updateFirmware

In `firmware.go` `updateFirmware`, after `UpdateFirmware` call:
```go
matched, err := a.db.UpdateFirmware(r.Context(), id, fw)
if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
}
if matched == 0 {
    http.Error(w, "Firmware not found", http.StatusNotFound)
    return
}
```

Update `db.UpdateFirmware` to return `(int64, error)` with `result.MatchedCount`.

### Step 3: Validate updateFirmware body fields

Add validation before the DB update:
```go
if fw.Name == "" {
    http.Error(w, "Name is required", http.StatusBadRequest)
    return
}
```

### Step 4: Verify

Build controller.

---

## Task 7: Cleanup and minor fixes

**Priority:** LOW
**Files:**
- `backend/services/controller/internal/api/deviceinfo.go`
- `backend/services/controller/internal/api/api.go`
- `backend/services/utils/firmware-upload/firmware-upload.js`
- `frontend/src/sections/mass-actions/device-selector.js`
- `frontend/src/contexts/backend-context.js`

### Step 1: Remove dead restart-agent endpoint (or document it)

Option A (remove): Delete `deviceRestartAgent` handler from `deviceinfo.go` and route from `api.go`.
Option B (keep): Add a comment in `api.go` explaining it's available via API but not exposed in UI.

### Step 2: Add temp file cleanup for firmware uploads

In `firmware-upload.js`, configure formidable to use a separate temp directory and add a periodic cleanup:
```javascript
const TEMP_DIR = path.join(FIRMWARE_DIR, '.tmp');
fs.mkdirSync(TEMP_DIR, { recursive: true });
// In form options:
form.uploadDir = TEMP_DIR;
```

### Step 3: Increase device selector page size

In `device-selector.js`, increase from 50 to a larger limit or add pagination:
```javascript
// Quick fix: increase limit
const { status, result } = await httpRequest('/api/device?page_size=500', 'GET');
```

For large fleets, a proper solution would be server-side filtering with `vendor` and `model` query parameters and scroll-to-load pagination.

### Step 4: Stabilize httpRequest with useRef (optional)

In `backend-context.js`, wrap `httpRequest` in a ref to prevent identity changes:
```javascript
const httpRequestRef = useRef(httpRequest);
httpRequestRef.current = httpRequest;
const stableHttpRequest = useCallback((...args) => httpRequestRef.current(...args), []);
```

This eliminates the stale closure risk when `httpRequest` is used in `useCallback` deps.

### Step 5: Limit goroutine spawning in devicePerformanceGet

Add a simple semaphore or check:
```go
// At package level:
var perfStoreSem = make(chan struct{}, 10)

// In devicePerformanceGet:
select {
case perfStoreSem <- struct{}{}:
    go func() {
        defer func() { <-perfStoreSem }()
        // ... store metrics
    }()
default:
    log.Printf("devicePerformanceGet: metrics store queue full, skipping for %s", sn)
}
```

### Step 6: Verify

Build controller and frontend.

---

## Task 8: NATS request correlation (architectural)

**Priority:** MEDIUM (but large scope — separate initiative)
**Files:**
- `backend/services/controller/internal/bridge/`

### Description

The NATS request/response pattern currently lacks per-request correlation. Two concurrent USP requests to the same device can receive each other's responses. The current workaround (sequencing requests in the frontend) is fragile.

### Fix approach

Use unique NATS reply subjects per request:
```go
// Instead of:
nc.Request(subject, data, timeout)

// Use:
inbox := nats.NewInbox()
sub, _ := nc.SubscribeSync(inbox)
nc.PublishRequest(subject, inbox, data)
msg, _ := sub.NextMsg(timeout)
sub.Unsubscribe()
```

This is the standard NATS pattern for request/response correlation. Each request gets its own unique inbox, so responses cannot cross-contaminate.

### Scope

This affects `bridge.NatsReq`, `bridge.NatsUspInteraction`, and `bridge.NatsCwmpInteraction`. All callers benefit automatically. Requires careful testing with concurrent requests.

**Recommendation:** Implement as a separate focused effort after the other tasks are complete.

---

## Implementation Order

```
Task 1 (mass actions)      ████████  HIGH — data corruption, broken functionality
Task 2 (firmware upload)   ████████  HIGH — security vulnerabilities
Task 3 (infrastructure)    ██████    MEDIUM — deployment issues
Task 4 (frontend fixes)    ██████    MEDIUM — UI bugs and auth consistency
Task 5 (scripts)           █████     MEDIUM — execution safety
Task 6 (DB error handling) ███       LOW — silent errors
Task 7 (cleanup)           ███       LOW — minor improvements
Task 8 (NATS correlation)  ██████    MEDIUM — architectural, separate effort
```

Tasks 1-2 should be done first (security + data integrity). Tasks 3-5 can be parallelized. Tasks 6-7 are quick wins. Task 8 is a separate initiative.

---

## Verification

After all tasks, run full stack:
```bash
sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build controller frontend 2>&1"
sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && ./run_debug.sh"
```

Test:
- Firmware upload with special characters in filename
- Mass firmware update with 2+ concurrent devices
- Bridging port move (verify add-then-delete order)
- Network tab with devices that return `err_code: 0`
- Script with conditional loops hitting the iteration cap
