# Mass Actions Implementation Plan

## Overview

Mass Actions lets operators run firmware upgrades or scripts across multiple devices at once. It reuses the existing firmware and scripts infrastructure, adding device selection/filtering, batch execution with concurrency control, and a job tracking UI.

The sidebar "Mass Actions" item gets two children: **Firmware Update** and **Scripts** (renamed from "Message").

### Key Design Decisions

- **Firmware vendor/model matching**: Firmware records already have `Vendor` and `Model` fields. When creating a mass firmware update, the system auto-filters the device list to show only devices whose `Manufacturer` and `ModelName` match the selected firmware. This reuses the same case-insensitive matching logic already implemented in the single-device firmware update dialog.
- **No separate "filter-based" device selection**: Keep it simple — operator selects firmware, system shows compatible online devices, operator picks which ones. Filter criteria are implicit from firmware vendor/model.
- **Reuse existing execution engines**: FW update reuses `performFirmwareUpdate`, script execution reuses `executeScriptForDevice` — no new protocol logic.

---

## Data Model

### MassAction (new MongoDB collection: `general.mass_actions`)

```
MassAction {
  ID            ObjectID
  Type          string           // "firmware_update" | "script"
  Name          string           // User-given name or auto-generated
  Status        string           // "pending" | "running" | "completed" | "failed" | "cancelled"

  // Target devices — explicit list of selected serial numbers
  DeviceSNs     []string
  TotalDevices  int              // Count at execution start

  // Firmware-specific
  FirmwareID    ObjectID         // Reference to firmware document
  FirmwareName  string           // Snapshot: name + version
  FirmwareURL   string           // Download URL

  // Script-specific
  ScriptID      ObjectID         // Reference to script document
  ScriptName    string           // Snapshot of script name
  Variables     map[string]string// Variable values for script execution

  // Execution tracking
  DeviceResults []DeviceResult   // Per-device outcome
  Progress      int              // Count of completed devices (success + failed)
  SuccessCount  int
  FailureCount  int
  Concurrency   int              // Max parallel executions (default 5)

  CreatedAt     time.Time
  StartedAt     time.Time
  FinishedAt    time.Time
}

DeviceResult {
  DeviceSN    string
  Status      string            // "pending" | "running" | "success" | "failed" | "skipped"
  MTP         string            // Protocol used
  Error       string
  ExecutionID ObjectID          // For scripts: reference to ScriptExecution record
  StartedAt   time.Time
  FinishedAt  time.Time
}
```

**Indexes:**
- `created_at` with TTL = 90 days
- `status + created_at` (for listing active/recent jobs)

---

## Backend Implementation

### Step 1: DB Layer

**File:** `backend/services/controller/internal/db/mass_actions.go`

- `MassAction`, `DeviceResult` structs
- `CreateMassAction(ctx, action) (MassAction, error)`
- `GetMassAction(ctx, id) (MassAction, error)`
- `UpdateMassAction(ctx, id, action) error` — updates status, progress, device_results
- `ListMassActions(ctx) ([]MassAction, error)` — sorted by created_at desc, limit 100
- `CancelMassAction(ctx, id) error` — sets status to "cancelled"

**File:** `backend/services/controller/internal/db/db.go`

- Add `massActions *mongo.Collection` field to Database struct
- Register `general.mass_actions` collection with indexes

### Step 2: Mass Firmware Update Handler

**File:** `backend/services/controller/internal/api/mass_actions.go`

**Endpoint:** `POST /api/mass-actions/firmware`

**Request body:**
```json
{
  "firmware_id": "ObjectID",
  "device_sns": ["SN1", "SN2"],
  "concurrency": 5
}
```

**Logic:**
1. Validate firmware exists, get download URL
2. Use `device_sns` as the explicit device list (frontend already filtered by vendor/model)
3. Create MassAction record (status = "running"), snapshot firmware name/version/URL
4. Return MassAction ID immediately (HTTP 202 Accepted)
5. Launch goroutine with semaphore (concurrency limiter):
   - For each device (in parallel, capped by concurrency):
     - Check device online status via `deviceStateOKNoWrite()`
     - If offline → mark "skipped"
     - Call `performFirmwareUpdate(sn, mtp, firmware)` (extracted from existing fw_update handler)
     - Update DeviceResult in MassAction record
   - When all done → set MassAction status to "completed" or "failed"

**Reuse:** Extract core FW update logic from the existing single-device handler into `performFirmwareUpdate(sn, mtp string, fw db.Firmware, nc *nats.Conn) error`.

### Step 3: Mass Script Execution Handler

**File:** same `mass_actions.go`

**Endpoint:** `POST /api/mass-actions/script`

**Request body:**
```json
{
  "script_id": "ObjectID",
  "variables": { "key": "value" },
  "device_sns": ["SN1", "SN2"],
  "concurrency": 5
}
```

**Logic:**
1. Validate script exists, validate required variables
2. Create MassAction record (status = "running")
3. Return MassAction ID immediately (HTTP 202 Accepted)
4. Launch goroutine with semaphore:
   - For each device (in parallel, capped by concurrency):
     - Check device online, resolve MTP
     - Call `executeScriptForDevice(script, sn, mtp, variables)` (extracted from `executeScriptHandler`)
     - Each device execution creates its own ScriptExecution record (existing model)
     - Store ScriptExecution ID in DeviceResult.ExecutionID
     - Update DeviceResult in MassAction record
   - When all done → finalize MassAction status

**Reuse:** Extract the core execution loop from `executeScriptHandler` in `scripts.go` into `executeScriptForDevice(ctx, script, sn, mtp, variables) (ScriptExecution, error)`.

### Step 4: Status & Listing Endpoints

**File:** same `mass_actions.go`

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/mass-actions` | List all mass actions (last 100) |
| `GET` | `/api/mass-actions/{id}` | Get mass action with device results |
| `POST` | `/api/mass-actions/{id}/cancel` | Cancel a running mass action |

### Step 5: Route Registration

**File:** `backend/services/controller/internal/api/api.go`

```go
mass := r.PathPrefix("/api/mass-actions").Subrouter()
mass.HandleFunc("", a.listMassActions).Methods("GET")
mass.HandleFunc("/firmware", a.massFirmwareUpdate).Methods("POST")
mass.HandleFunc("/script", a.massScriptExecution).Methods("POST")
mass.HandleFunc("/{id}", a.getMassAction).Methods("GET")
mass.HandleFunc("/{id}/cancel", a.cancelMassAction).Methods("POST")

mass.Use(func(handler http.Handler) http.Handler {
    return middleware.Middleware(handler)
})
```

---

## Frontend Implementation

### Step 6: Navigation Update

**File:** `frontend/src/layouts/dashboard/config.js`

- Enable "Mass Actions" parent item (remove `disabled: true`, add `path: '/mass-actions'`)
- Rename "Message" child to "Scripts"
- Add paths: `/mass-actions/firmware` and `/mass-actions/scripts`
- Remove `disabled: true` and gray colors from children

### Step 7: Compatible Device Selector Component

**File:** `frontend/src/sections/mass-actions/device-selector.js`

A component for selecting target devices, with vendor/model awareness for firmware updates.

**Props:**
- `vendor` (string, optional) — firmware vendor to match against device Manufacturer
- `model` (string, optional) — firmware model to match against device ModelName
- `selectedSNs` (array) — currently selected serial numbers
- `onChange(sns)` — callback when selection changes

**Behavior:**
1. Fetch devices via `GET /api/device`
2. When `vendor`/`model` props are set (firmware update mode):
   - Auto-filter to show only devices matching vendor/model (case-insensitive)
   - Show info banner: "Showing X devices matching Vendor / Model"
   - Still allow manual filter override to see all devices
3. When no vendor/model (script mode): show all devices
4. Checkbox column for multi-select with "Select All" option
5. Show columns: Serial Number, Manufacturer, Model, Status (online/offline chip)
6. Online devices selectable, offline devices shown but grayed out with tooltip "Device offline"
7. Selected count displayed prominently

### Step 8: Mass Firmware Update Page

**File:** `frontend/src/pages/mass-actions/firmware.js`

**Layout — two cards:**

**Card 1 — New Firmware Update:**
1. **Firmware selector**: Radio list from `GET /api/firmware` showing name, version, vendor, model, phase chip — same table layout as the single-device FirmwareUpdateDialog
2. When firmware is selected → Device Selector (Step 7) auto-populates with `vendor={selectedFw.vendor}` and `model={selectedFw.model}`, showing only compatible devices
3. Concurrency slider (1–20, default 5)
4. "Start Update" button → `POST /api/mass-actions/firmware`
5. Shows selected firmware summary + device count before confirming

**Card 2 — Recent Jobs:**
- Table listing recent mass firmware jobs from `GET /api/mass-actions` (filtered by type=firmware_update)
- Columns: Name, Firmware, Progress (bar or X/Y), Status (chip), Created
- Click row → expand to show per-device results (Step 10)
- Cancel button for running jobs

### Step 9: Mass Script Execution Page

**File:** `frontend/src/pages/mass-actions/scripts.js`

**Layout — two cards:**

**Card 1 — Run Script:**
1. Script selector: dropdown from `GET /api/scripts` (show name, description, step count)
2. Variable inputs: dynamic form based on selected script's variables (with defaults pre-filled)
3. Device Selector (Step 7) — no vendor/model filtering, shows all devices
4. Concurrency slider (1–20, default 5)
5. "Execute" button → `POST /api/mass-actions/script`

**Card 2 — Recent Jobs:**
- Same pattern as firmware page but for script executions
- Per-device rows link to individual ScriptExecution records for detailed step results

### Step 10: Job Detail View

**File:** `frontend/src/sections/mass-actions/mass-action-detail.js`

Expandable component shown when clicking a job row:

- Progress bar with color segments (green=success, red=failed, gray=pending)
- Device results table:
  - Columns: Device SN, Status (chip), MTP, Error, Duration
  - For script jobs: "View Details" link opening existing ScriptHistoryDialog per execution
- Auto-refresh while status is "running" (poll every 5 seconds)
- Cancel button for running jobs

---

## Execution Flow

### Firmware Update Flow
```
User selects firmware → system shows compatible devices (matching vendor/model)
  → User selects devices → POST /api/mass-actions/firmware
  → Backend creates MassAction (status=running), returns ID (202)
  → Goroutine spawns workers (capped by semaphore):
       For each device:
         1. Check online → deviceStateOKNoWrite()
         2. If offline → mark "skipped"
         3. Send OPERATE Download() with firmware URL
         4. Update DeviceResult in MassAction
  → Frontend polls GET /api/mass-actions/{id} every 5s
  → Progress bar updates in real-time
```

### Script Execution Flow
```
User selects script + variables + devices → POST /api/mass-actions/script
  → Backend creates MassAction (status=running), returns ID (202)
  → Goroutine spawns workers:
       For each device:
         1. Check online → deviceStateOKNoWrite()
         2. Create ScriptExecution record
         3. Run steps sequentially (existing engine)
         4. Update DeviceResult + ScriptExecution
  → Frontend polls GET /api/mass-actions/{id} every 5s
  → Each device row links to its ScriptExecution for step details
```

---

## Cancellation

- `POST /api/mass-actions/{id}/cancel` sets status to "cancelled"
- Goroutine checks `massAction.Status` before starting each new device
- Already-running device executions complete normally (no mid-step abort)
- Remaining devices get status "skipped"

---

## File Summary

| Step | File | Action |
|------|------|--------|
| 1 | `db/mass_actions.go` | New: structs + CRUD |
| 1 | `db/db.go` | Modify: add collection + indexes |
| 2 | `api/mass_actions.go` | New: firmware handler + `performFirmwareUpdate` extraction |
| 3 | `api/mass_actions.go` | Add: script handler |
| 3 | `api/scripts.go` | Modify: extract `executeScriptForDevice` |
| 4 | `api/mass_actions.go` | Add: list, get, cancel handlers |
| 5 | `api/api.go` | Modify: register `/api/mass-actions` routes |
| 6 | `config.js` | Modify: enable Mass Actions nav, rename Message→Scripts |
| 7 | `sections/mass-actions/device-selector.js` | New: vendor/model-aware device selection |
| 8 | `pages/mass-actions/firmware.js` | New: mass FW update page |
| 9 | `pages/mass-actions/scripts.js` | New: mass script execution page |
| 10 | `sections/mass-actions/mass-action-detail.js` | New: job detail/progress component |

---

## Implementation Order

Steps 1–5 (backend) first, then 6–10 (frontend). Steps 2 and 3 can be done in parallel since they share the same file but different handlers. Steps 8 and 9 can also be parallelized since they share the device-selector from Step 7.

---

## Security Considerations

- Mass actions use same JWT auth middleware as all other endpoints
- Concurrency cap prevents overloading NATS/devices (max 20 parallel)
- Cancelled jobs don't interrupt in-progress device operations (graceful)
- MassAction records auto-delete after 90 days via TTL index
