# Scripts Feature — Implementation Plan

## Overview

Scripts are saved sequences of USP/CWMP commands that can be executed against devices. They support variables, conditional branching, error handling, execution history, and mass execution.

## Data Model

### Script (MongoDB collection: `scripts`, database: `general`)

```json
{
  "_id": "ObjectID",
  "name": "Reset WiFi to defaults",
  "description": "Disables WiFi, sets default SSID, re-enables and reboots",
  "tags": ["wifi", "reset"],
  "variables": [
    {
      "name": "SSID_NAME",
      "description": "WiFi network name",
      "default": "DefaultNetwork",
      "required": true
    },
    {
      "name": "WIFI_PASSWORD",
      "description": "WiFi password (min 8 chars)",
      "default": "",
      "required": true
    }
  ],
  "steps": [
    {
      "id": "step_1",
      "name": "Read current WiFi state",
      "type": "GET",
      "param_paths": ["Device.WiFi.Radio.1.", "Device.WiFi.SSID.1."],
      "max_depth": 2,
      "on_error": "abort",
      "save_result_as": "wifi_state"
    },
    {
      "id": "step_2",
      "name": "Check if WiFi is enabled",
      "type": "CONDITION",
      "condition": {
        "source": "wifi_state",
        "path": "Device.WiFi.Radio.1.",
        "param": "Enable",
        "operator": "==",
        "value": "true"
      },
      "on_true": "step_3",
      "on_false": "step_4"
    },
    {
      "id": "step_3",
      "name": "Set SSID name",
      "type": "SET",
      "obj_path": "Device.WiFi.SSID.1.",
      "param_settings": [
        { "param": "SSID", "value": "{{SSID_NAME}}", "required": true }
      ],
      "on_error": "continue"
    },
    {
      "id": "step_4",
      "name": "Enable WiFi radio",
      "type": "SET",
      "obj_path": "Device.WiFi.Radio.1.",
      "param_settings": [
        { "param": "Enable", "value": "true", "required": true }
      ],
      "on_error": "abort"
    },
    {
      "id": "step_5",
      "name": "Reboot device",
      "type": "OPERATE",
      "command": "Device.Reboot()",
      "command_key": "post-wifi-reset",
      "on_error": "abort"
    }
  ],
  "created_at": "2026-03-19T10:00:00Z",
  "updated_at": "2026-03-19T10:00:00Z"
}
```

### Step Types

| Type | Fields | Description |
|------|--------|-------------|
| `GET` | `param_paths[]`, `max_depth`, `save_result_as` | Read parameters, optionally save response for conditions |
| `SET` | `obj_path`, `param_settings[]` | Set parameter values |
| `ADD` | `obj_path`, `param_settings[]` | Create new object instance |
| `DELETE` | `obj_paths[]` | Delete object instances |
| `OPERATE` | `command`, `command_key`, `input_args[]` | Invoke operation (Reboot, FirmwareUpgrade, etc.) |
| `CONDITION` | `condition`, `on_true`, `on_false` | Branch based on a previous GET result |
| `DELAY` | `duration_ms` | Wait between steps (e.g., wait after reboot) |

### Step Error Handling (`on_error`)

- `abort` — stop script execution, mark as failed (default)
- `continue` — log error, proceed to next step
- `skip_to:<step_id>` — jump to a specific step on error

### Variables

Variables use `{{VAR_NAME}}` syntax in any string field (`value`, `obj_path`, `param_paths`, `command`). They are resolved at execution time. Each variable has:
- `name` — identifier used in `{{...}}`
- `description` — shown to user when executing
- `default` — pre-filled value (can be empty)
- `required` — must be provided before execution

### Execution Log (MongoDB collection: `script_executions`, database: `general`)

```json
{
  "_id": "ObjectID",
  "script_id": "ObjectID",
  "script_name": "Reset WiFi to defaults",
  "device_sn": "ABC123",
  "mtp": "mqtt",
  "status": "completed",
  "variables": { "SSID_NAME": "MyNetwork", "WIFI_PASSWORD": "secret123" },
  "step_results": [
    {
      "step_id": "step_1",
      "step_name": "Read current WiFi state",
      "status": "success",
      "response": { ... },
      "started_at": "...",
      "finished_at": "..."
    },
    {
      "step_id": "step_2",
      "step_name": "Check if WiFi is enabled",
      "status": "success",
      "condition_result": true,
      "jumped_to": "step_3"
    }
  ],
  "started_at": "2026-03-19T10:05:00Z",
  "finished_at": "2026-03-19T10:05:12Z",
  "created_at": "2026-03-19T10:05:00Z"
}
```

TTL index on `created_at` to auto-delete old executions (e.g., 30 days).

---

## Implementation Steps

### Step 1: Backend — Database Layer

**File**: `backend/services/controller/internal/db/scripts.go`

Create MongoDB structs and CRUD methods following the `firmware.go` pattern:

- `Script` struct with all fields from data model above
- `ScriptStep` struct for individual steps
- `ScriptVariable` struct for variable definitions
- `ScriptExecution` struct for execution logs
- `StepResult` struct for per-step results
- Methods:
  - `ListScripts(ctx) ([]Script, error)`
  - `CreateScript(ctx, Script) (Script, error)`
  - `GetScript(ctx, id) (Script, error)`
  - `UpdateScript(ctx, id, Script) error`
  - `DeleteScript(ctx, id) error`
  - `CreateExecution(ctx, ScriptExecution) (ScriptExecution, error)`
  - `UpdateExecution(ctx, id, ScriptExecution) error`
  - `ListExecutions(ctx, scriptID) ([]ScriptExecution, error)` — filtered by script
  - `ListExecutionsByDevice(ctx, sn) ([]ScriptExecution, error)` — filtered by device
  - `GetExecution(ctx, id) (ScriptExecution, error)`

**File**: `backend/services/controller/internal/db/db.go`

- Register `scripts` and `script_executions` collections in `NewDatabase()` under `"general"` database
- Create indexes:
  - `scripts`: unique index on `name`
  - `script_executions`: compound index on `{script_id, created_at}`, index on `{device_sn, created_at}`, TTL index on `created_at` (30 days)

### Step 2: Backend — Script Execution Engine

**File**: `backend/services/controller/internal/api/scripts.go`

The execution engine runs steps sequentially, handles variables, conditions, and error policies:

```
function executeScript(script, sn, mtp, variables):
    resolve all {{VAR}} placeholders in steps using provided variables
    create execution log entry (status: "running")

    saved_results = {}     // for CONDITION lookups
    current_step_index = 0

    while current_step_index < len(steps):
        step = steps[current_step_index]
        result = execute_step(step, sn, mtp, saved_results)

        log step result to execution entry

        if result.error:
            switch step.on_error:
                "abort": mark execution "failed", return
                "continue": current_step_index++
                "skip_to:X": current_step_index = indexOf(X)
        else:
            if step.type == "CONDITION":
                target = step.on_true if result.condition_met else step.on_false
                current_step_index = indexOf(target)
            else if step.type == "DELAY":
                sleep(step.duration_ms)
                current_step_index++
            else:
                if step.save_result_as:
                    saved_results[step.save_result_as] = result.response
                current_step_index++

    mark execution "completed"
```

Each step type maps directly to existing USP message construction:
- `GET` → `usp_utils.NewGetMsg()` + `sendUspMsg()`
- `SET` → `usp_utils.NewSetMsg()` + `sendUspMsg()`
- `ADD` → `usp_utils.NewAddMsg()` + `sendUspMsg()`
- `DELETE` → `usp_utils.NewDeleteMsg()` + `sendUspMsg()`
- `OPERATE` → `usp_utils.NewOperateMsg()` + `sendUspMsg()`
- `CONDITION` → evaluate against `saved_results` (no device communication)
- `DELAY` → `time.Sleep()`

**Important**: Execution must NOT use `r.Context()` (it gets cancelled when HTTP handler returns). Use `context.WithTimeout(context.Background(), ...)` for each step's NATS interaction, same pattern as the metrics fix.

**Important**: Steps must execute sequentially (not concurrently) to avoid the NATS subject race condition.

### Security

Scripts are declarative data, not executable code. There is no dynamic code evaluation, no shell execution, no template engine. Each step maps to a fixed protobuf message construction via a Go switch statement. The attack surface is limited to the data fields that get placed into USP messages.

**Validation rules (enforced on create/update AND before execution):**

1. **Path format** — `obj_path`, `param_paths[]`, `obj_paths[]` must match `^Device\.[\w.]+\.?$`. Reject anything that doesn't start with `Device.`.

2. **OPERATE command whitelist** — `command` must match `^Device\.[\w.]+\(\)$`. Optionally maintain a configurable allowlist of permitted commands (e.g., `Device.Reboot()`, `Device.FactoryReset()`, `Device.DeviceInfo.FirmwareImage.*.Download()`). Reject unknown commands by default.

3. **Variable names** — must match `^[A-Za-z0-9_]+$`. No special characters that could interfere with path construction.

4. **Variable values** — max length 1024 characters. After substitution, re-validate that resulting paths still match the path format regex.

5. **Step limits** — max 50 steps per script. Max total execution time 5 minutes (enforced via context timeout wrapping the entire execution loop).

6. **DELAY limits** — `duration_ms` capped at 60000 (1 minute) per step.

7. **Condition operators** — strict enum: `==`, `!=`, `>`, `<`, `contains`, `exists`. Handled by Go `switch` statement, no expression parsing.

8. **No recursive references** — `on_true`/`on_false`/`skip_to` step IDs are validated to exist in the script. Detect loops: track visited steps, abort if a step is visited more than `len(steps)` times.

9. **`save_result_as` names** — must match `^[A-Za-z0-9_]+$`, max 10 saved results per execution to prevent memory abuse.

10. **Input size** — max script JSON body size 256KB (enforce via `http.MaxBytesReader` on the create/update handlers).

### Step 3: Backend — REST API Handlers

**File**: `backend/services/controller/internal/api/scripts.go` (same file as engine)

CRUD handlers following `firmware.go` pattern:

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| `GET` | `/api/scripts` | `listScripts` | List all scripts |
| `POST` | `/api/scripts` | `createScript` | Create new script |
| `GET` | `/api/scripts/{id}` | `getScript` | Get single script |
| `PUT` | `/api/scripts/{id}` | `updateScript` | Update script |
| `DELETE` | `/api/scripts/{id}` | `deleteScript` | Delete script |
| `POST` | `/api/scripts/{id}/execute/{sn}/{mtp}` | `executeScript` | Execute script on device |
| `GET` | `/api/scripts/{id}/executions` | `listExecutions` | Execution history for script |
| `GET` | `/api/scripts/executions/{execId}` | `getExecution` | Single execution detail |

The `executeScript` handler:
1. Loads script from DB
2. Validates provided variables against script's variable definitions
3. Checks device is online via `deviceStateOK()`
4. Runs the execution engine (in the handler goroutine — NOT background, so the response waits for completion)
5. Returns the execution log as JSON response

For long-running scripts, the response may take time. This is acceptable for single-device execution. Mass execution (Step 7) will use background jobs.

### Step 4: Backend — Route Registration

**File**: `backend/services/controller/internal/api/api.go`

Add subrouter:
```go
scripts := r.PathPrefix("/api/scripts").Subrouter()
scripts.HandleFunc("", a.listScripts).Methods("GET")
scripts.HandleFunc("", a.createScript).Methods("POST")
scripts.HandleFunc("/{id}", a.getScript).Methods("GET")
scripts.HandleFunc("/{id}", a.updateScript).Methods("PUT")
scripts.HandleFunc("/{id}", a.deleteScript).Methods("DELETE")
scripts.HandleFunc("/{id}/execute/{sn}/{mtp}", a.executeScript).Methods("POST")
scripts.HandleFunc("/{id}/executions", a.listScriptExecutions).Methods("GET")
scripts.HandleFunc("/executions/{execId}", a.getExecution).Methods("GET")
scripts.Use(middleware)
```

### Step 5: Frontend — Scripts List Page

**File**: `frontend/src/pages/scripts.js`

Main page with:
- Title "Scripts" + "Create Script" button
- Table listing all scripts (name, description, tags, step count, last modified)
- Row actions: Edit, Execute, Delete
- Click row → opens script editor

**File**: `frontend/src/sections/scripts/scripts-table.js`

Table component following `firmware-table.js` pattern:
- Columns: Name, Description, Tags (as Chips), Steps count, Updated, Actions
- Skeleton loading state
- Empty state message
- Actions: Edit (pencil), Execute (play), Duplicate (copy), Delete (trash)

### Step 6: Frontend — Script Editor

**File**: `frontend/src/sections/scripts/script-editor.js`

Full-page or dialog editor with sections:

**Header section:**
- Script name (TextField)
- Description (TextField, multiline)
- Tags (chip input — type and press Enter to add)

**Variables section:**
- List of defined variables with: name, description, default value, required toggle
- Add/remove variable buttons

**Steps section — the core editor:**
- Ordered list of steps, each rendered as a card
- Each step card shows:
  - Step name (editable inline)
  - Step type dropdown (GET/SET/ADD/DELETE/OPERATE/CONDITION/DELAY)
  - Type-specific fields (see below)
  - Error handling dropdown (abort/continue/skip_to)
  - Drag handle for reordering (use `@dnd-kit/core` or simple up/down arrow buttons)
  - Delete button
- "Add Step" button at bottom with type selection

**Type-specific fields per step card:**

| Type | Fields |
|------|--------|
| GET | param_paths (multi-line, one per line), max_depth (number), save_result_as (optional text) |
| SET | obj_path (text), param_settings (table: param, value, required checkbox) |
| ADD | obj_path (text), param_settings (table: param, value, required checkbox) |
| DELETE | obj_paths (multi-line, one per line) |
| OPERATE | command (text, e.g. `Device.Reboot()`), command_key (text), input_args (table: key, value) |
| CONDITION | source (dropdown of save_result_as names), path (text), param (text), operator (dropdown: ==, !=, >, <, contains, exists), value (text), on_true (step dropdown), on_false (step dropdown) |
| DELAY | duration_ms (number input) |

**Variable hints:** In any text field, show autocomplete suggestions for `{{` prefix listing defined variables.

**Step reordering:** Use up/down arrow buttons on each step card. Keep it simple — no drag-and-drop library needed.

### Step 7: Frontend — Execute Dialog

**File**: `frontend/src/sections/scripts/script-execute-dialog.js`

Dialog shown when clicking "Execute" on a script:

1. **Device selection**: dropdown or search field to pick a device (fetch from `/api/devices`)
2. **Variable inputs**: for each script variable, show a TextField pre-filled with default value. Required variables marked with asterisk.
3. **Execute button**: sends POST to `/api/scripts/{id}/execute/{sn}/{mtp}` with `{ variables: { ... } }`
4. **Live progress**: after execution starts, show step-by-step progress:
   - Each step shows: name, status (pending/running/success/failed/skipped), duration
   - Expandable to show response data
5. **Result summary**: overall success/failure with link to full execution log

### Step 8: Frontend — Execution History

**File**: `frontend/src/sections/scripts/script-history.js`

Shown as a tab or expandable section in the script editor:
- Table of past executions: device, status, started_at, duration
- Click to expand and see per-step results
- Color-coded status: green (completed), red (failed), yellow (running)

### Step 9: Frontend — Navigation

**File**: `frontend/src/layouts/dashboard/config.js`

Enable the Scripts menu item:
```javascript
{
  title: 'Scripts',
  path: '/scripts',
  icon: <SvgIcon fontSize="small"><CommandLineIcon /></SvgIcon>,
  // Remove disabled: true and color='gray'
}
```

### Step 10: Mass Execution (extends Step 7)

Add mass execution capability to the execute dialog:

**Device selection mode toggle:**
- Single device (existing)
- Multiple devices (checkbox list or "select all matching filter")

**Backend**: new endpoint `POST /api/scripts/{id}/execute-mass` with body:
```json
{
  "device_sns": ["ABC123", "DEF456", "GHI789"],
  "mtp": "any",
  "variables": { "SSID_NAME": "CompanyWiFi" }
}
```

Execution runs sequentially per device (to avoid NATS congestion). Each device gets its own `ScriptExecution` entry. The endpoint returns immediately with a batch ID, and execution proceeds in background.

**Frontend**: shows batch progress — list of devices with individual status (pending/running/completed/failed).

---

## Step Ordering & Dependencies

```
Step 1 (DB layer) ← no dependencies
Step 2 (Engine)   ← depends on Step 1
Step 3 (API)      ← depends on Steps 1, 2
Step 4 (Routes)   ← depends on Step 3
Step 5 (List page) ← depends on Step 4 (needs API)
Step 6 (Editor)    ← depends on Step 5
Step 7 (Execute)   ← depends on Steps 4, 6
Step 8 (History)   ← depends on Steps 4, 6
Step 9 (Nav)       ← depends on Step 5
Step 10 (Mass)     ← depends on Step 7
```

Parallelizable: Steps 5-9 frontend work can proceed in parallel once Step 4 is done.

---

## Built-in Script Templates

Ship with pre-built scripts users can clone:

1. **Get Device Info** — GET `Device.DeviceInfo.` (simple, good for testing)
2. **Reset WiFi** — SET SSID, password, enable radio
3. **Firmware Upgrade** — OPERATE `Device.DeviceInfo.FirmwareImage.1.Download()` with URL variable
4. **Factory Reset** — OPERATE `Device.FactoryReset()`
5. **Enable/Disable Interface** — SET `Device.IP.Interface.{{IF_INDEX}}.Enable` with variable
6. **Bridge Port Migration** — DELETE port from bridge A, ADD to bridge B with variables

These are stored as regular scripts with a `builtin: true` flag (non-deletable but cloneable).

---

## Files to Create/Modify

### New files:
- `backend/services/controller/internal/db/scripts.go`
- `backend/services/controller/internal/api/scripts.go`
- `frontend/src/pages/scripts.js`
- `frontend/src/sections/scripts/scripts-table.js`
- `frontend/src/sections/scripts/script-editor.js`
- `frontend/src/sections/scripts/script-execute-dialog.js`
- `frontend/src/sections/scripts/script-history.js`

### Modified files:
- `backend/services/controller/internal/db/db.go` — register collections + indexes
- `backend/services/controller/internal/api/api.go` — register routes
- `frontend/src/layouts/dashboard/config.js` — enable Scripts menu item
