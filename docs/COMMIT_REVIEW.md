# Code Review: Commits by Oktopus Dev

25 commits reviewed. Issues fixed in subsequent commits are excluded.

---

## d7625ee -- feat(firmware): add Firmware MongoDB schema and CRUD layer

No concerns.

---

## 655a584 -- feat: add firmware API, device info/control endpoints, metrics schema, topology, and upload service

### HIGH: firmware-upload.js -- auth is presence-only

`firmware-upload.js:21-24` checks `if (!req.headers['authorization'])` but never validates the token. Any non-empty `Authorization` header is accepted.

### HIGH: firmware.go -- unsanitized filename from multipart upload

`firmware.go:91` uses `header.Filename` directly. It flows into:
- `filepath.Join(os.TempDir(), ...)` on line 93 -- path traversal in temp directory
- `downloadURL` on line 104 -- stored in DB and served to devices

The firmware-upload.js service validates filenames on DELETE but not on upload receipt.

### MEDIUM: firmware.go -- MD5 used for firmware fingerprint

`firmware.go:83` uses `md5.New()` for firmware integrity. MD5 is cryptographically broken. For firmware images, SHA-256 should be used to prevent collision attacks.

### MEDIUM: firmware.go -- entire firmware file buffered in memory

`firmware.go:84` calls `io.ReadAll` loading the full file into memory, then writes it to a temp file on line 94. A 500MB firmware consumes 500MB of heap. Should stream to disk directly.

### MEDIUM: metrics.go -- error from cursor.All ignored

`db/metrics.go` `GetDeviceMetricsHistory`: the error returned by `cursor.All(ctx, &results)` is not checked. Partial or corrupted results are returned silently.

### LOW: firmware.go -- no URL-encoding of fileName in delete call

`firmware.go:219` uses `fmt.Sprintf("%s/delete?name=%s", ...)` without URL-encoding. Filenames with `&`, `=`, `#`, or spaces will break the request.

---

## a634920 -- feat(infra): wire firmware-upload service into nginx and docker compose

### MEDIUM: firmware-upload port exposed to host

`docker-compose.yaml:249` maps `8006:8006` to the host. Combined with the presence-only auth in firmware-upload.js, anyone with network access can upload arbitrary files.

### LOW: FIRMWARE_BASE_URL hardcoded to localhost

`.env.controller` sets `FIRMWARE_BASE_URL=http://localhost/firmwares`. Devices will receive download URLs pointing to `localhost`, which breaks any non-local deployment.

### MEDIUM: nginx.conf uses host.docker.internal

All backend proxying (`/api`, `/firmwares`, `/containers`) uses `host.docker.internal`, which is not available on native Linux Docker. Confirmed: project targets native Linux Docker. These should use Docker service names (e.g., `controller:8000`) or `extra_hosts` must be added to the nginx service in docker-compose.yaml.

---

## f290207 -- feat(frontend): add Firmware page, device detail tabs, performance charts, and topology view

No remaining concerns. The infinite re-render loop and hardcoded MTP issues were fixed in commits 2329ae4 and dc1021b.

---

## 5549944 -- fix(infra): resolve port conflicts on shared servers

No concerns.

---

## 2329ae4 -- fix(frontend): stop infinite request loop on device tabs and firmware page

### LOW: stale closure risk

Removing `httpRequest` from `useCallback` deps fixes the loop but introduces a stale closure risk. If `httpRequest` identity changes (e.g., token refresh), callbacks use the old reference. The correct fix is to stabilize `httpRequest` with `useRef` in the context provider.

---

## dc1021b -- fix(frontend): use mtp=any so backend auto-detects transport

No concerns.

---

## 2e176c5 -- Redesign Info/Network/Topology tabs to parse real USP GetResp format

### MEDIUM: err_code 0 treated as error

`devices-network.js:196-199` `isAllErrors` checks `r.err_code != null`. In JavaScript, `0 != null` is `true`, so `err_code: 0` (success) is treated as an error. Same issue in `devices-topology.js` `hasPathError`. Should be `r.err_code != null && r.err_code !== 0`.

### LOW: parseUspFlat silently overwrites duplicate keys

`devices-info.js` `parseUspFlat` uses `Object.assign(flat, rr.result_params)`. If multiple resolved paths return the same parameter name, later values silently overwrite earlier ones.

---

## 03c83e4 -- Add Cause and Reason InputArgs to Device.Reboot USP Operate command

No concerns.

---

## c7a1de5 -- Add CommandKey to Reboot, FactoryReset, RestartAgent OPERATE messages

No concerns.

---

## ee88a9f -- Add Firmware Update button to device Info tab

### MEDIUM: handleFwDeploy bypasses auth context

`devices-info.js` `handleFwDeploy` manually reads `localStorage.getItem('token')` and constructs the Authorization header instead of using `httpRequest` from context. This duplicates auth logic and won't follow token refresh or format changes.

### LOW: no catch for network errors in firmware deploy

`handleFwDeploy` has error feedback for non-200 responses (line 291 shows error alert), but has no `catch` block. A network-level exception (fetch throwing) would leave no toast -- only the spinner stops via `finally`.

---

## 90fd4cd -- Fix firmware upload EXDEV error on cross-device rename

### LOW: orphaned temp files in firmware directory

Setting `uploadDir` to `FIRMWARE_DIR` means failed uploads leave formidable temp files (e.g., `upload_xxxx`) in the production firmware directory. No cleanup mechanism exists. These could accumulate and potentially be served by the file-server.

---

## be4ad29 -- Add id and autoComplete attributes to all input fields

No concerns.

---

## f398f24 -- Remove Restart Agent button from device Info tab

### INFO: dead backend endpoint

The `restart-agent` handler (`deviceinfo.go:305`) and route (`api.go:89`) remain registered but are no longer reachable from the UI.

---

## 1238856 -- Fix device metrics not being stored due to cancelled request context

### LOW: unbounded goroutine spawning

`deviceinfo.go:187-192` spawns a background goroutine per `/performance` request with no concurrency limit. Under load, this could create unbounded goroutines each with a 10-second timeout.

---

## 286e272 -- Fix Network tab not loading by sequencing USP requests

### MEDIUM: symptom treated, root cause unaddressed

Two concurrent requests to the same NATS subject caused response cross-contamination. Sequencing fixes the symptom, but the underlying issue is that the NATS request/response pattern lacks proper per-request correlation (unique reply subjects). This same class of bug can recur anywhere two concurrent USP requests target the same device.

---

## 37696e6 -- Change device access button to navigate to Info tab instead of Parameters

No concerns.

---

## 207f53f -- Add WiFi Radios, SSIDs/AccessPoints, and EndPoints display to Network tab

No concerns.

---

## 1c2eec2 -- Move Ethernet Interfaces to Network tab and rework WiFi display

No concerns.

---

## d554bb4 -- Add Bridging tab with bridge/port management

### MEDIUM: non-atomic port move can lose ports

`devices-bridging.js:297-317` `handleMovePort` performs delete then add as separate operations. If the delete succeeds but the add fails (network error, device rejection), the port is lost from both bridges with no rollback.

### MEDIUM: all USP operations bypass httpRequest

`devices-bridging.js:162-218` `uspGet`, `uspSet`, `uspAdd`, `uspDel` use direct `fetch` with `localStorage.getItem("token")` instead of the `httpRequest` from `useBackendContext()`. This bypasses centralized auth token refresh, error handling, and request interceptors. Inconsistent with the rest of the frontend.

### LOW: no confirmation on destructive operations

`handleDeletePort` and `handleMovePort` execute immediately on click with no confirmation dialog. Deleting or moving a bridge port can disrupt network connectivity.

### LOW: no MAC address validation

`handleSaveMAC` sends the raw user input to the device via USP SET without validating MAC address format.

---

## 4c7cead -- Move device tabs from top bar to sidebar as sub-items under Devices

No concerns.

---

## e552507 -- Add Scripts feature for automating USP command sequences

### MEDIUM: script execution blocks HTTP handler

`scripts.go` `executeScriptHandler` runs the entire script synchronously in the HTTP handler, including `time.Sleep` for DELAY steps. A script with multiple delays or many steps can hold the HTTP connection open for minutes, risking timeout.

### MEDIUM: loop detection allows excessive iterations

`scripts.go:739` allows `visitCounts[stepIndex] > len(script.Steps)` visits per step. With 50 steps and conditional jumps, this permits up to 2500 iterations, each potentially making a network call.

### LOW: UpdateExecution error silently ignored

`scripts.go:850` does not check the error from `a.db.UpdateExecution()`. The execution could be returned as "completed" to the client while the DB still records it as "running".

### LOW: discardResponseWriter.Header() returns new empty map each call

`scripts.go:429` `Header()` returns `http.Header{}` on every call. If bridge code sets and then reads headers, those values are silently lost.

---

## 1cd81d0 -- Add vendor/model fields to firmware and improve Scripts UI

### LOW: updateFirmware does not check matched count

`firmware.go` `updateFirmware`: if the firmware ID does not exist, `UpdateOne` returns `MatchedCount=0` but the handler returns 204 No Content. The user gets no indication that nothing was updated.

### LOW: updateFirmware allows overwriting with empty strings

Unlike `createFirmware`, the update handler does not validate body fields. Empty name or build_version can overwrite valid data.

---

## ef339a9 -- Add Mass Actions for batch firmware updates and script execution

### HIGH: race condition on shared MassAction struct

`mass_actions.go` `runMassFirmwareUpdate` and `runMassScriptExecution`: the `ma` struct is shared across goroutines. Individual field updates are protected by `mu.Lock()`, but `a.db.UpdateMassAction(ctx, ma.ID, ma)` is called after unlock. One goroutine can overwrite another's results because `$set` replaces the entire `device_results` array. Should use atomic MongoDB updates (`$set` on individual array indices) or hold the lock during the DB write.

### HIGH: findAvailablePartition partition extraction is broken

`mass_actions.go:297` does `resolvedPath[len(resolvedPath)-2:]` to extract partition number. For `Device.DeviceInfo.FirmwareImage.1.` this returns `1.`. For `Device.DeviceInfo.FirmwareImage.10.` this returns `0.`. Multi-digit partition numbers are incorrectly extracted.

### MEDIUM: no upper bound on DeviceSNs array size

`massFirmwareUpdate` and `massScriptExecution` accept an unbounded list of device serial numbers. A request with thousands of SNs spawns unbounded goroutines.

### MEDIUM: firmware update does not check USP response for errors

`mass_actions.go:271-282` `performFirmwareUpdate` checks the NATS transport error but does not inspect the USP response body for device-level errors. A failed firmware command is recorded as success.

---

## 393e736 -- Add offline device support: cached info, access button, skip live queries

### LOW: device selector limited to 50 devices

`device-selector.js` fetches `?page_size=50`. For mass actions targeting a large fleet, only the first 50 devices are shown with no pagination to access the rest.

---

## d0f985a -- Add project documentation: CLAUDE.md, roadmap, and implementation plans

No concerns. Documentation only.

---

## Summary by Severity

| Severity | Count | Key Items |
|----------|-------|-----------|
| HIGH | 4 | firmware-upload auth bypass, unsanitized filename, mass action race condition, broken partition extraction |
| MEDIUM | 10 | MD5 fingerprint, memory buffering, err_code 0 handling, non-atomic port move, script sync execution, USP response not checked, host.docker.internal on Linux |
| LOW | 12 | Stale closures, dead code, missing validation, orphaned temp files, unbounded goroutines |
| INFO | 1 | Dead restart-agent endpoint |
