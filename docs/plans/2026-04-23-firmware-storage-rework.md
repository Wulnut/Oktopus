# Firmware Storage Rework

## Problems

1. **Filename collisions**: Two firmwares with the same original filename overwrite each other on disk, even across tenants
2. **Download URL uses localhost**: `FIRMWARE_BASE_URL` defaults to `http://localhost/firmwares` — devices can't reach it
3. **No duplicate check**: Backend allows uploading firmware with identical vendor+model+hw_version+build_version
4. **Original filename used for storage**: Unpredictable, user-controlled, can cause collisions

## Solution

### 1. Deterministic stored filename

Generate filename from metadata: `<vendor>_<model>_<hw_version>_<build_version>.<ext>`

- Sanitize each field (replace spaces with `_`, remove special characters)
- Extract extension from original upload filename
- Example: `Broadcom_GP601_V3_svn2835.tar`
- Store on disk as `/app/firmwares/<tenant>/<generated_filename>`
- Store `FileName` in DB as the generated name (not original)

### 2. Duplicate check before upload

Before accepting the file, query MongoDB for existing firmware with the same combination:
```
{ vendor, model, hw_version, build_version }
```
If found, return `409 Conflict` with message: `"Firmware with this vendor/model/hw_version/build_version already exists"`

Add a unique compound index on `(vendor, model, hw_version, build_version)` in the firmware collection.

### 3. Dynamic download URL from request Host header

Remove `FIRMWARE_BASE_URL` env var. Construct download URL from the incoming request:
```go
scheme := "http"
if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
    scheme = "https"
}
downloadURL = scheme + "://" + r.Host + "/firmwares/" + tenantSlug + "/" + storedFileName
```

This works because:
- Nginx proxies `/firmwares` to `file-server:8004`
- The request's `Host` header contains the server's actual IP/hostname
- When HTTPS is added to nginx, `X-Forwarded-Proto` carries the scheme

### 4. Frontend: show error on duplicate

The frontend upload dialog should display the 409 error message to the user instead of a generic error.

## Files to Change

### Backend

**`internal/api/firmware.go`**:
- Remove `firmwarePublicBaseURL` env var
- Add `generateStoredFileName(vendor, model, hwVersion, buildVersion, origFilename string) string`
- Add duplicate check: `tdb.GetFirmwareByIdentity(vendor, model, hwVersion, buildVersion)` — return 409 if exists
- Construct download URL from `r.Host` and `X-Forwarded-Proto`
- Pass generated filename to upload service instead of original

**`internal/db/firmware.go`**:
- Add `GetFirmwareByIdentity(ctx, vendor, model, hwVersion, buildVersion)` method
- Add unique compound index on `(vendor, model, hw_version, build_version)` in tenant DB provisioning

**`internal/db/tenant.go`**:
- Add compound index to `provisionTenantDBs`

### Infrastructure

**`deploy/compose/nginx.conf`**:
- Add `proxy_set_header X-Forwarded-Proto $scheme;` to the `/firmwares` location (and ideally to all proxy locations)

**`deploy/compose/.env.controller.example`**:
- Remove `FIRMWARE_BASE_URL`

**`deploy/compose/generate-secrets.sh`**:
- Remove `FIRMWARE_BASE_URL` from controller env generation

### Frontend

**`frontend/src/sections/firmware/firmware-upload-dialog.js`**:
- Handle 409 response: show "Firmware with this vendor/model/hw_version/build_version already exists"

## Execution Order

1. Add `generateStoredFileName` helper
2. Add `GetFirmwareByIdentity` DB method + compound index
3. Update `uploadFirmware` handler: duplicate check, generated filename, dynamic URL
4. Update nginx to forward `X-Forwarded-Proto`
5. Remove `FIRMWARE_BASE_URL` from env
6. Update frontend to handle 409
7. Build and test

## Migration Note

Existing firmware records have original filenames and `localhost` URLs. These will continue to work for listing/display but their download URLs will be broken for devices. A migration script or manual re-upload would be needed for existing firmware.
