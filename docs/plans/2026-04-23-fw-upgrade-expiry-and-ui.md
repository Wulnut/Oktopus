# Firmware Upgrade: Expiry, Error Display, and Version Matching

## Problem 1: "downloading" status never expires

When the controller sends a Download() operate to the device, the upgrade log is set to "downloading". If the download silently fails (wrong URL, network issue, device rejects), the status stays "downloading" forever, blocking all future upgrade attempts for that device+firmware pair.

## Problem 2: No upgrade status visible in Web-UI

Users have no visibility into upgrade history, success/failure, or why an upgrade was blocked.

## Problem 3: Version mismatch causes infinite re-upgrade loop

`checkUpgradeCompletion` compares `device.Version` with `fw.BuildVersion` using exact string match. If the device reports a different format (e.g., `V4.0.0-260422` vs `V4.0.0_20260420`), the upgrade is marked as "failed" and re-triggered on next connect, creating an infinite loop.

---

## Fix 1: Downloading expiry

### Backend (`campaign_engine.go`)

Add a timeout check in `handleManualPolicy` and `handleCampaignPolicy`: if an existing log has status "downloading" and `triggered_at` is older than 15 minutes, treat it as failed.

```go
if existingLog.Status == "downloading" {
    if time.Since(existingLog.TriggeredAt) > 15*time.Minute {
        tdb.UpdateUpgradeLogStatus(ctx, existingLog.ID, "failed", "download timed out")
        // Fall through to retry
    } else {
        return // Still within timeout window
    }
}
```

Apply same logic in `handleCampaignPolicy` where it checks existing logs.

### Files to change
- `backend/services/controller/internal/api/campaign_engine.go` — `handleManualPolicy`, `handleCampaignPolicy`, and `RunCampaignBatch`

## Fix 2: Upgrade status in Web-UI

### Backend — new API endpoint

Add `GET /api/tenants/{slug}/device/{sn}/upgrade-logs` (already exists at `api.go:134`).

Verify it returns the full log list for the device.

### Frontend — Info tab upgrade history

Add an "Upgrade History" section to the device Info tab (`devices-info.js`):
- Fetch `GET /api/tenants/{slug}/device/{sn}/upgrade-logs` on mount
- Display as a small table or list:
  - Firmware name + version
  - Status (color-coded: green=success, yellow=downloading/pending, red=failed)
  - Trigger type (manual/campaign/on_connect)
  - Error message (if failed)
  - Triggered at / Completed at
- Auto-refresh every 30 seconds while status is "pending" or "downloading"

### Files to change
- `frontend/src/sections/devices/usp/devices-info.js` — add upgrade history section

## Fix 3: Version mismatch handling

### Option A: Normalize version strings before comparison

Strip common separators and compare normalized forms:
```go
normalize := func(v string) string {
    v = strings.ReplaceAll(v, "_", "")
    v = strings.ReplaceAll(v, "-", "")
    v = strings.ToLower(v)
    return v
}
if normalize(device.Version) == normalize(pendingLog.FirmwareBuildVer) {
    // success
}
```

### Option B: Skip version check, mark as "needs_verification"

After reboot, instead of auto-checking, mark the log as "needs_verification" and let the user confirm in the UI. Too manual.

### Option C: Store device-reported version alongside build_version

After upload, the user enters `build_version` (human label). Add a separate field `expected_device_version` that the user can optionally set to match what the device will report. If not set, skip the auto-check.

### Recommended: Option A

Simple, handles common format differences. If the normalized versions don't match, it's a real mismatch (wrong firmware was flashed).

### Files to change
- `backend/services/controller/internal/api/campaign_engine.go` — `checkUpgradeCompletion`

---

## Execution Order

1. Fix downloading expiry (campaign_engine.go)
2. Fix version normalization (campaign_engine.go)
3. Verify upgrade-logs endpoint works
4. Add upgrade history to frontend Info tab
