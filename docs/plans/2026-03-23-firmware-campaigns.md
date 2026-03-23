# Firmware Campaigns

> Automatic firmware deployment based on device hardware identity, with per-device override.
> Replaces the current manual mass firmware update flow.

---

## Overview

A **Campaign** is a mapping from device hardware identity (Vendor + Model + HW Version) to a specific firmware. When a device comes online, the system checks if a campaign targets its hardware and — unless the device is excluded — triggers the firmware upgrade automatically.

Each hardware combination can have **at most one active campaign**, eliminating conflicts by design.

The current Mass Actions → Firmware Updates page is replaced with a Campaigns UI.

---

## Data Model

### Campaign

```
Campaign {
  ID              ObjectID
  Vendor          string        // e.g. "Huawei"
  Model           string        // e.g. "HG8245"
  HWVersion       string        // e.g. "v3.0" — from Device.DeviceInfo.HardwareVersion
  FirmwareID      ObjectID      // references Firmware collection
  Concurrency     int           // max simultaneous upgrades on campaign start (default 10)
  TimeWindowStart string        // optional, HH:MM in UTC (e.g. "02:00")
  TimeWindowEnd   string        // optional, HH:MM in UTC (e.g. "05:00")
  Enabled         bool          // pause/resume without deleting
  CreatedAt       time.Time
  UpdatedAt       time.Time
}
```

**Unique constraint**: `(vendor, model, hw_version)` — enforces one campaign per hardware combo.

**Time window**: When set, upgrades only trigger if the current UTC time falls within the window. If unset, upgrades trigger immediately.

**Concurrency**: Limits how many devices are upgraded simultaneously when a campaign is created or enabled (batch processing of already-online devices). Default 10.

### Device Firmware Policy (per-device override)

New field on the Device entity or a separate lightweight collection:

```
DeviceFWPolicy {
  DeviceSN         string        // unique, references device
  Policy           string        // "campaign" | "skip" | "manual"
  ManualFirmwareID ObjectID      // set only when policy == "manual"
  UpdatedAt        time.Time
}
```

**Policy values:**
- `"campaign"` (default for all devices) — follow the active campaign for this device's hardware
- `"skip"` — do not enforce firmware on this device
- `"manual"` — use a specific firmware chosen by admin, regardless of campaign

Devices without an explicit policy record default to `"campaign"`.

### Firmware Upgrade Log

Records every upgrade attempt for audit and de-duplication:

```
FirmwareUpgradeLog {
  ID                ObjectID
  DeviceSN          string
  DeviceAlias       string      // snapshot at trigger time
  CampaignID        ObjectID    // null for manual upgrades
  FirmwareID        ObjectID
  FirmwareName      string      // snapshot: e.g. "HG8245 v2.1.0"
  FirmwareBuildVer  string      // snapshot: e.g. "2.1.0"
  PreviousVersion   string      // device's SoftwareVersion before upgrade
  TriggerType       string      // "on_connect" | "campaign_start" | "manual"
  Status            string      // "pending" | "downloading" | "success" | "failed"
  Error             string      // error message if failed
  RetryCount        int         // number of retry attempts
  TriggeredAt       time.Time
  CompletedAt       time.Time   // null until completed/failed
}
```

**Key fields explained:**
- **PreviousVersion** — the device's `SoftwareVersion` at the moment the upgrade was triggered, for before/after comparison
- **TriggerType** — how the upgrade was initiated:
  - `"on_connect"` — device came online and matched a campaign
  - `"campaign_start"` — campaign was created/enabled and device was already online
  - `"manual"` — admin selected a specific firmware from the dropdown
- **FirmwareName / FirmwareBuildVer** — snapshots so the log remains readable even if the firmware record is later deleted
- **Status flow**: `pending` → `downloading` → `success` or `failed`
- **RetryCount** — incremented on each retry; used to implement max retry policy

---

## Device Entity Changes

### Add HardwareVersion field

```go
type Device struct {
    SN           string  // existing
    Model        string  // existing
    Vendor       string  // existing
    Version      string  // existing (SoftwareVersion)
    ProductClass string  // existing
    HWVersion    string  // NEW — from Device.DeviceInfo.HardwareVersion
    Alias        string  // existing
    Status       Status  // existing
    // ... MTP fields unchanged
}
```

### Cache HardwareVersion on connect

In the adapter's `deviceOnline()` (file: `backend/services/mtp/adapter/internal/events/usp_handler/status.go`), add `Device.DeviceInfo.HardwareVersion` to the USP GET request that already fetches Manufacturer, ModelName, SoftwareVersion, SerialNumber, ProductClass.

In the info handler (`info.go`), parse the HardwareVersion from the response and include it in the Device struct passed to `db.CreateDevice()`.

---

## API Endpoints

### Campaign CRUD

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/campaigns` | List all campaigns (with firmware details populated) |
| POST | `/api/campaigns` | Create campaign (returns 409 if vendor+model+hw_version exists) |
| PUT | `/api/campaigns/{id}` | Update campaign (change firmware, time window, concurrency, enabled) |
| DELETE | `/api/campaigns/{id}` | Delete campaign |

### Device Firmware Policy

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/device/{sn}/fw-policy` | Get device's firmware policy |
| PUT | `/api/device/{sn}/fw-policy` | Set device's firmware policy |

**PUT body:**
```json
{
  "policy": "campaign | skip | manual",
  "firmware_id": "..."
}
```
`firmware_id` required only when `policy == "manual"`.

### Upgrade Log

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/campaigns/{id}/logs` | Upgrade logs for a campaign (paginated) |
| GET | `/api/device/{sn}/upgrade-logs` | Upgrade logs for a device (paginated) |

---

## Frontend Changes

### Mass Actions → Firmware Updates (replaces current page)

The current page (`/mass-actions/firmware`) with its manual "select firmware → select devices → start" flow is replaced entirely with the Campaigns UI.

**Layout: two cards**

#### Card 1 — Campaigns

A table of all campaigns with inline management:

**Table columns:**
- Vendor
- Model
- HW Version
- Firmware (name + build version)
- Time Window (or "Anytime")
- Progress (e.g. "47/120 devices" — computed from upgrade logs vs matching device count)
- Enabled (toggle switch, inline)
- Actions (edit, delete, view logs)

**Create/Edit** (dialog or inline form):
- Vendor (autocomplete from known vendors)
- Model (autocomplete from known models)
- HW Version (text input)
- Firmware (dropdown, filtered by vendor+model when available)
- Concurrency (number input, default 10)
- Time Window (optional toggle, two time pickers: start + end, UTC)

**Validation:** If a campaign already exists for the same vendor+model+hw_version, show error on create. On edit, allow changing firmware/window/concurrency/enabled but not the hardware identity fields.

#### Card 2 — Upgrade Log (for selected campaign)

When a campaign row is expanded or selected, show its upgrade log:

**Table columns:**
- Device SN / Alias
- Previous Version
- Target Version
- Trigger (on_connect / campaign_start / manual)
- Status (pending / downloading / success / failed)
- Error (if failed)
- Triggered At
- Completed At
- Duration (computed)
- Retries

**Summary bar** above the table:
- Total devices targeted
- Success count
- Failed count
- Pending count

Filterable by status.

### Device Info Page — Firmware Dropdown

Replace the current firmware deploy button + modal with a dropdown selector:

```
┌─────────────────────────────────┐
│ ▼ Firmware                      │
├─────────────────────────────────┤
│ ✓ Use campaign firmware         │  ← default, follows active campaign
│   Do not enforce FW             │  ← exclude from automatic upgrades
│ ──────────────────────────────  │
│   FW v2.1.0 (2026-03-20)       │  ← manual selection, filtered by
│   FW v1.9.0 (2026-03-15)       │     device's vendor + model
│   FW v1.8.0 (2026-03-01)       │
└─────────────────────────────────┘
```

- The dropdown shows the current policy as the selected value
- "Use campaign firmware" and "Do not enforce FW" are always the first two options
- Below the separator: all firmwares matching the device's Vendor + Model, sorted by creation date (newest first)
- Selecting a firmware from the list sets policy to `"manual"` with that firmware ID
- Selecting an option immediately saves via `PUT /api/device/{sn}/fw-policy`
- If "Use campaign firmware" is selected but no campaign exists for this hardware, show a subtle info note: "No active campaign for this hardware"

---

## Auto-Apply Logic

### Trigger Path 1: On Device Connect

When a device connects (adapter `deviceOnline` flow), after the device record is created/updated:

1. Load the device's firmware policy (default: `"campaign"`)
2. If policy is `"skip"` → do nothing
3. If policy is `"manual"` → check if `ManualFirmwareID` firmware was already applied → if not, trigger upgrade
4. If policy is `"campaign"`:
   a. Look up campaign by device's `(Vendor, Model, HWVersion)`
   b. If no campaign or campaign is disabled → do nothing
   c. If campaign has a time window and current time is outside it → skip (will retry on next connect)
   d. Check upgrade log: was this firmware already triggered for this device?
   e. If log shows `"success"` or `"pending"` → skip (already done or in progress)
   f. If log shows `"failed"` and retry count < max retries → trigger upgrade, increment retry
   g. If no log entry → trigger upgrade, create log entry with status `"pending"`, trigger type `"on_connect"`

### Trigger Path 2: Campaign Start (batch)

When a campaign is created or enabled, scan all currently online devices matching the campaign's Vendor+Model+HWVersion:

1. Filter out devices with policy `"skip"`
2. Filter out devices that already have a success/pending log entry for this firmware
3. Respect the campaign's concurrency limit (e.g., 10 at a time)
4. For each device: trigger upgrade, create log entry with trigger type `"campaign_start"`
5. Process remaining devices as concurrency slots free up

This runs in a background goroutine, similar to existing mass action pattern.

### Firmware Upgrade Trigger

Reuse the existing `performFirmwareUpdate` logic from mass actions:
1. Find available firmware image slot
2. Set download URL + AutoActivate
3. Send OPERATE Download()
4. Update log entry status: `"pending"` → `"downloading"`

### Upgrade Completion Detection

After the device reboots and reconnects:
1. On-connect flow reads the device's new `SoftwareVersion`
2. Compare with the firmware's `BuildVersion` from the pending/downloading log entry
3. If they match → update log status to `"success"`, set `CompletedAt`
4. If they don't match → update log status to `"failed"`, set error "version mismatch after upgrade"

This means the on-connect flow does double duty: checking for new upgrades AND confirming previous upgrades completed.

---

## What Gets Removed

- **Mass firmware update API** (`POST /api/mass-actions/firmware`) — replaced by campaign auto-apply
- **Mass firmware update page** (`/mass-actions/firmware`) — replaced by campaigns UI
- **Manual device selection for firmware** — no longer needed; campaigns target by hardware identity
- **MassAction records with type "firmware_update"** — replaced by Campaign + FirmwareUpgradeLog

The Mass Actions → Scripts page and its backend remain unchanged.

---

## Implementation Order

1. **Cache HardwareVersion** — add to adapter on-connect fetch, Device entity, DB upsert
2. **Campaign CRUD** — DB model, API endpoints, unique index
3. **Device FW Policy** — DB model, API endpoints
4. **Upgrade Log** — DB model, API endpoints
5. **Auto-apply: on-connect** — check campaign, check log, trigger upgrade
6. **Auto-apply: campaign start batch** — scan online devices, batch upgrade with concurrency
7. **Upgrade completion detection** — on-connect version comparison, log status update
8. **Frontend: campaigns page** — replace `/mass-actions/firmware` with campaigns UI + log view
9. **Frontend: device info dropdown** — replace button with policy dropdown
10. **Cleanup** — remove old mass firmware update API and related code

---

## Open Questions

- **Retry policy**: How many retries for failed upgrades before giving up? Suggest max 3, configurable per campaign.
- **Notification**: Should admins be notified when a campaign completes (all matching devices upgraded) or when failures exceed a threshold? Future addition.
- **CWMP support**: This design is USP-focused. CWMP firmware upgrade uses a different mechanism (Download RPC). Campaign could support both by checking device type, but out of scope for initial implementation.
- **Version comparison**: Currently comparing firmware `BuildVersion` with device `SoftwareVersion` as strings (equality check). This works for "is the device already on this version?" but doesn't support ordering (which version is newer). Acceptable for campaigns since the admin explicitly chooses the target firmware.
