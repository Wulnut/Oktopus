# Oktopus API Reference

Base URL: `http://localhost/api`

## Authentication

All endpoints require JWT in the `Authorization` header, except `/api/auth/*`.

```
Authorization: Bearer <token>
```

### User Levels

| Level | Role | Description |
|-------|------|-------------|
| 0 | SuperAdmin | Platform owner, full access |
| 1 | TenantAdmin | ISP admin, manages resources in their tenant |
| 2 | Operator | ISP user, day-to-day device management |

### Tenant Prefix

Tenant-scoped endpoints use `/api/tenants/{slug}/...`. SuperAdmin without active tenant calls `/api` directly.

---

## 1. Auth

No authentication required for these endpoints.

### `PUT /api/auth/login`

Login to get a JWT token.

```bash
curl -X PUT http://localhost/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"secret"}'
```

| Request Field | Type | Required | |
|---------------|------|----------|--|
| email | string | yes | |
| password | string | yes | |

Response: JWT token string.

### `POST /api/auth/admin/register`

Register the first SuperAdmin. Once an admin exists, this requires a SuperAdmin JWT.

| Request Field | Type | Required | |
|---------------|------|----------|--|
| email | string | yes | |
| name | string | no | |
| password | string | yes | |
| phone | string | no | |

### `GET /api/auth/admin/exists`

Check if a SuperAdmin exists. Returns `true` / `false`.

---

## 2. Tenants

SuperAdmin only.

### `GET /api/tenants`

List all tenants, sorted by name.

Response: `Tenant[]`

### `POST /api/tenants`

Create a tenant. Provisions databases, NATS KV bucket, and optionally a TenantAdmin user.

| Request Field | Type | Required | Notes |
|---------------|------|----------|-------|
| name | string | yes | Slug auto-generated from name |
| auth_policy | object | no | `{password_required, cert_required}` |
| admin_email | string | no | Initial TenantAdmin email |
| admin_name | string | no | Initial TenantAdmin name |
| admin_password | string | no | Initial TenantAdmin password |

Response: `Tenant`

### `GET /api/tenants/{slug}`

Get tenant by URL slug. Tenant users can only view their own.

### `PUT /api/tenants/{slug}`

Update tenant name, status, or auth policy.

| Request Field | Type | Notes |
|---------------|------|-------|
| name | string | |
| status | string | `active` or `disabled` |
| auth_policy | object | |

### `DELETE /api/tenants/{slug}`

Full cleanup: deletes firmware files, registry containers, adapter devices, users, databases, KV buckets.

---

## 3. Devices

### List & Search

#### `GET /api/tenants/{slug}/device`

| Query Param | Type | Description |
|-------------|------|-------------|
| page | int | 0-based page number |
| page_size | int | Items per page |
| status | string | `offline`, `associating`, `online` |
| vendor | string | Filter by vendor |
| model | string | Filter by model |
| product_class | string | Filter by product class |
| search | string | Search SN or alias |

Response: `{ devices: Device[], total: int }`

#### `GET /api/tenants/{slug}/device/filterOptions`

Get available filter values. Response: `{ models, productClasses, vendors, versions }`

#### `GET /api/tenants/{slug}/device/{sn}/{mtp}/info`

Get live device info via USP. `mtp` is the Message Transfer Protocol: `mqtt`, `ws` (WebSocket), `stomp`, or `cwmp`.

#### `GET /api/tenants/{slug}/device/{sn}/cached-info`

Get cached device info when the device is offline.

### Device Actions

All take path params `{sn}` (serial number) and `{mtp}` (protocol).

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/device/alias` | PUT | Set device alias `{sn, alias}` |
| `/device/auth` | GET | List device credentials |
| `/device/auth` | POST | Add device credential `{key, value}` |
| `/device/auth` | DELETE | Delete device credential |
| `/device/{sn}/{mtp}/generic` | PUT | Send generic USP message |
| `/device/{sn}/{mtp}/get` | PUT | USP Get `{param_paths, max_depth}` |
| `/device/{sn}/{mtp}/set` | PUT | USP Set `{obj_path, param_settings}` |
| `/device/{sn}/{mtp}/add` | PUT | USP Add (create object) |
| `/device/{sn}/{mtp}/del` | PUT | USP Delete |
| `/device/{sn}/{mtp}/notify` | PUT | USP Notify |
| `/device/{sn}/{mtp}/parameters` | PUT | Get supported parameters |
| `/device/{sn}/{mtp}/instances` | PUT | Get parameter instances `{obj_path}` |
| `/device/{sn}/{mtp}/operate` | PUT | USP Operate `{command, command_key, input_args}` |
| `/device/{sn}/{mtp}/fw_update` | PUT | Trigger firmware upgrade `{firmware_id, download_url}` |
| `/device/{sn}/{mtp}/reboot` | PUT | Reboot device |
| `/device/{sn}/{mtp}/factory-reset` | PUT | Factory reset |
| `/device/{sn}/{mtp}/restart-agent` | PUT | Restart USP agent |
| `/device/{sn}/{mtp}/topology` | GET | Network topology |
| `/device/{sn}/{mtp}/interfaces` | GET | Network interfaces |
| `/device/{sn}/{mtp}/performance` | GET | Performance metrics |
| `/device/{sn}/{mtp}/wifi-usp` | GET | WiFi config via USP |
| `/device/{sn}/wifi` | GET | Cached WiFi config |
| `/device/{sn}/wifi` | PUT | Set WiFi config |

### CWMP Operations

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/device/cwmp/{sn}/generic` | PUT | Generic CWMP message |
| `/device/cwmp/{sn}/getParameterNames` | PUT | CWMP GetParameterNames |
| `/device/cwmp/{sn}/getParameterValues` | PUT | CWMP GetParameterValues `{parameter_names}` |
| `/device/cwmp/{sn}/getParameterAttributes` | PUT | CWMP GetParameterAttributes |
| `/device/cwmp/{sn}/setParameterValues` | PUT | CWMP SetParameterValues `{parameter_list}` |
| `/device/cwmp/{sn}/addObject` | PUT | CWMP AddObject |
| `/device/cwmp/{sn}/deleteObject` | PUT | CWMP DeleteObject |

### History & Metrics

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/device/{sn}/history` | GET | Message history `?page&page_size` |
| `/device/{sn}/history` | DELETE | Clear message history |
| `/device/{sn}/metrics` | GET | Device metrics history |

### Message Templates

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/device/message/{type}` | POST | Add template `{name, message}` |
| `/device/message` | GET | Get template |
| `/device/message` | PUT | Update template |
| `/device/message` | DELETE | Delete template |

### FW Policy & Upgrade Logs

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/device/{sn}/fw-policy` | GET | Get FW upgrade policy |
| `/device/{sn}/fw-policy` | PUT | Set policy `{policy, manual_firmware_id}` |
| `/device/{sn}/upgrade-logs` | GET | Device upgrade history |

FW Policy values: `campaign`, `skip`, `manual`.

### Device Password

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/device-password` | GET | Get shared device password |
| `/device-password` | PUT | Set shared device password `{password}` |

---

## 4. Firmware

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/firmware` | GET | List firmware |
| `/firmware` | POST | Upload (multipart/form-data) |
| `/firmware/{id}` | PUT | Update metadata |
| `/firmware/{id}` | DELETE | Delete (file + DB + disable campaigns) |
| `/firmware/{id}/phase` | PUT | Update phase |

### POST `/firmware` (multipart/form-data)

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| name | string | yes | Display name |
| build_version | string | yes | Build version string |
| vendor | string | no | |
| model | string | no | |
| hw_version | string | no | |
| phase | string | no | `internal_testing` (default) or `release` |
| file | binary | no | Firmware file (mutually exclusive with download_url) |
| download_url | string | no | External download URL (alternative to file) |

Firmware identity uniqueness: `vendor + model + hw_version + build_version`.

Response: `Firmware` (201 Created), or `409` if identity already exists.

### PUT `/firmware/{id}`

| Request Field | Type | Required | |
|---------------|------|----------|--|
| name | string | yes | |
| vendor | string | no | |
| model | string | no | |
| hw_version | string | no | |
| build_version | string | yes | |

### PUT `/firmware/{id}/phase`

| Request Field | Type | Required | |
|---------------|------|----------|--|
| phase | string | yes | `internal_testing` or `release` |

---

## 5. Scripts

### CRUD

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/scripts` | GET | List scripts |
| `/scripts` | POST | Create script |
| `/scripts/{id}` | GET | Get script |
| `/scripts/{id}` | PUT | Update script |
| `/scripts/{id}` | DELETE | Delete script (built-in scripts are protected) |

### Script Structure

| Field | Type | Notes |
|-------|------|-------|
| name | string | Required. Unique within tenant. |
| description | string | |
| tags | string[] | |
| variables | object[] | `{name, description, default, required}` |
| steps | object[] | Ordered step list (see below) |
| builtin | bool | Read-only. Built-in scripts cannot be deleted. |

### Step Types

#### GET — Read parameters

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| id | string | yes | Unique step ID |
| type | string | yes | `"GET"` |
| param_paths | string[] | yes | TR-181 paths, e.g. `["Device.DeviceInfo."]` |
| max_depth | int | no | Response depth |
| save_result_as | string | no | Store result for later `CONDITION` steps |
| on_error | string | no | `"abort"`, `"continue"`, or `"skip_to:<step_id>"` |

#### SET / ADD — Write parameters

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| id | string | yes | |
| type | string | yes | `"SET"` or `"ADD"` |
| obj_path | string | yes | Target object path |
| param_settings | object[] | yes | `[{param, value, required}]` |
| on_error | string | no | |

#### DELETE — Remove objects

| Field | Type | Required | |
|-------|------|----------|--|
| id | string | yes | |
| type | string | yes | `"DELETE"` |
| obj_paths | string[] | yes | Object paths to delete |
| on_error | string | no | |

#### OPERATE — Trigger commands

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| id | string | yes | |
| type | string | yes | `"OPERATE"` |
| command | string | yes | e.g. `"Device.Reboot()"` |
| command_key | string | no | |
| input_args | object[] | no | `[{key, value}]` |
| on_error | string | no | |

#### CONDITION — Branching

| Field | Type | Notes |
|-------|------|-------|
| id | string | yes | |
| type | string | yes | `"CONDITION"` |
| condition | object | `{source, path, param, operator, value}` |
| on_true | string | Step ID to jump to if true |
| on_false | string | Step ID to jump to if false |

Operators: `==`, `!=`, `>`, `<`, `contains`, `exists`.

#### DELAY — Pause

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| id | string | yes | |
| type | string | yes | `"DELAY"` |
| duration_ms | int | yes | 1–60000 ms |

### Execution

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/scripts/{id}/execute/{sn}/{mtp}` | POST | Execute on a device `{variables}` |
| `/scripts/{id}/executions` | GET | List execution history (last 50) |
| `/scripts/{id}/executions/{execId}` | GET | Get single execution details |

Response: `ScriptExecution` with status, step_results, and timing.

### Validation Rules

- 1–50 steps per script
- Max 500 total iterations (loop detection)
- Max 10 `save_result_as` per script
- Max variable value 1024 chars
- Variable names: `[A-Za-z0-9_]+`

---

## 6. Campaigns

Firmware upgrade campaigns targeting devices by vendor + model + hardware version.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/campaigns` | GET | List campaigns |
| `/campaigns` | POST | Create campaign |
| `/campaigns/{id}` | PUT | Update campaign |
| `/campaigns/{id}` | DELETE | Delete campaign |
| `/campaigns/{id}/logs` | GET | Upgrade logs `?page&page_size` |

### POST `/campaigns`

| Request Field | Type | Required | Notes |
|---------------|------|----------|-------|
| vendor | string | yes | |
| model | string | yes | |
| hw_version | string | yes | |
| firmware_id | string | yes | Target firmware ObjectID |
| concurrency | int | no | 1–50 (default 10) |
| time_window_start | string | no | |
| time_window_end | string | no | |
| enabled | bool | no | If true, starts immediately |

Uniqueness: `vendor + model + hw_version`. Response: `Campaign` (201) or 409 Conflict.

### Campaign Logs Response

```json
{
  "logs": [FirmwareUpgradeLog ...],
  "total": 123
}
```

Statuses: `pending` → `downloading` → `success` | `failed`.

---

## 7. Mass Actions

Bulk script execution across multiple devices.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/mass-actions` | GET | List (last 100) |
| `/mass-actions/script` | POST | Execute script on multiple devices |
| `/mass-actions/{id}` | GET | Get details with per-device results |
| `/mass-actions/{id}/cancel` | POST | Cancel |

### POST `/mass-actions/script`

| Request Field | Type | Required | |
|---------------|------|----------|--|
| script_id | string | yes | |
| device_sns | string[] | yes | Target device serial numbers |
| variables | object | no | Script variables |
| concurrency | int | no | |

Response: `MassAction` (201).

### MassAction Status

`pending` → `running` → `completed` / `failed` / `cancelled`.

Per-device status in `device_results[]`: `pending` → `running` → `success` / `failed` / `skipped`.

---

## 8. Users

Tenant-scoped user management.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/users` | GET | List users in tenant |
| `/users` | POST | Register user |
| `/users/{user}` | DELETE | Delete user by email |
| `/users/password` | PUT | Change own password |
| `/users/password/{user}` | PUT | Admin changes another user's password |

### POST `/users`

| Request Field | Type | Required | |
|---------------|------|----------|--|
| email | string | yes | Must be valid email format |
| password | string | yes | |
| name | string | no | |
| phone | string | no | |

- TenantAdmin can only create Operators.
- SuperAdmin can create TenantAdmins.
- Users cannot delete themselves.

### PUT `/users/password`

| Request Field | Type | Required | |
|---------------|------|----------|--|
| password | string | yes | Min 8 characters |

---

## 9. CA Certs

Tenant CA certificate management.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/ca-certs` | GET | List CA certs |
| `/ca-certs` | POST | Add cert `{label, pem}` |
| `/ca-certs/{certId}` | DELETE | Remove cert |

---

## 10. Dashboard Info

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/info/vendors` | GET | Vendor distribution `[{vendor, count}]` |
| `/info/status` | GET | Device status distribution `[{status, count}]` |
| `/info/device_class` | GET | Product class distribution `[{productClass, count}]` |
| `/info/general` | GET | `{device_count, online_count, offline_count}` |

---

## Data Models

### Device

| Field | Type | Description |
|-------|------|-------------|
| SN | string | Serial number |
| Vendor | string | |
| Model | string | |
| Version | string | |
| ProductClass | string | |
| HWVersion | string | Hardware version |
| Alias | string | User-assigned alias |
| Status | int | 0=Offline, 1=Associating, 2=Online |
| Mqtt | int | MQTT connection status |
| Stomp | int | STOMP connection status |
| Websockets | int | WebSocket connection status |
| Cwmp | int | CWMP connection status |
| TenantID | string | Tenant ObjectID |

### Firmware

| Field | Type | Description |
|-------|------|-------------|
| id | string | ObjectID |
| name | string | Display name |
| vendor | string | |
| model | string | |
| hw_version | string | Hardware version |
| build_version | string | Build version |
| file_size | int | Bytes |
| fingerprint | string | SHA-256 hash |
| phase | string | `internal_testing` or `release` |
| download_url | string | |
| file_name | string | Stored filename |
| created_at | datetime | |
| updated_at | datetime | |

### Campaign

| Field | Type | |
|-------|------|--|
| id | string | |
| vendor | string | |
| model | string | |
| hw_version | string | |
| firmware_id | string | Target firmware |
| concurrency | int | 1–50 |
| time_window_start | string | |
| time_window_end | string | |
| enabled | bool | |
| created_at | datetime | |
| updated_at | datetime | |

### Script

| Field | Type | |
|-------|------|--|
| id | string | |
| name | string | Unique in tenant |
| description | string | |
| tags | string[] | |
| variables | ScriptVariable[] | |
| steps | ScriptStep[] | Max 50 |
| builtin | bool | Read-only if true |
| created_at | datetime | |
| updated_at | datetime | |

### Tenant

| Field | Type | |
|-------|------|--|
| id | string | |
| name | string | |
| slug | string | URL-safe identifier |
| status | string | `active` or `disabled` |
| auth_policy | object | `{password_required, cert_required}` |
| ca_certs | TenantCACert[] | |
| created_at | datetime | |
| updated_at | datetime | |

### FirmwareUpgradeLog

| Field | Type | |
|-------|------|--|
| id | string | |
| device_sn | string | |
| device_alias | string | |
| campaign_id | string | |
| firmware_id | string | |
| firmware_name | string | |
| firmware_build_ver | string | |
| previous_version | string | |
| trigger_type | string | `campaign`, `manual`, `retry` |
| status | string | `pending`, `downloading`, `success`, `failed` |
| error | string | |
| retry_count | int | |
| triggered_at | datetime | |
| completed_at | datetime | |

### MassAction

| Field | Type | |
|-------|------|--|
| id | string | |
| type | string | `firmware_update` or `script` |
| name | string | |
| status | string | `pending`, `running`, `completed`, `failed`, `cancelled` |
| device_sns | string[] | |
| total_devices | int | |
| device_results | DeviceResult[] | Per-device status |
| progress | int | |
| success_count | int | |
| failure_count | int | |
| concurrency | int | |
| created_at | datetime | |

### User

| Field | Type | |
|-------|------|--|
| _id | string | |
| email | string | |
| name | string | |
| phone | string | |
| level | int | 0=SuperAdmin, 1=TenantAdmin, 2=Operator |
| createdAt | datetime | |

---

## HTTP Status Codes

| Code | Usage |
|------|-------|
| 200 | Success with response body |
| 201 | Resource created |
| 204 | Success with no body (update/delete) |
| 400 | Bad request — missing/invalid fields |
| 401 | Unauthorized — missing/invalid JWT |
| 403 | Forbidden — insufficient level |
| 404 | Resource not found |
| 409 | Conflict — duplicate resource |
| 429 | Rate limit exceeded |
| 500 | Internal server error |
| 503 | Service unavailable (e.g. device offline) |

## Error Response Format

```json
{"error": "description of the error"}
```

## Rate Limiting

The Nginx reverse proxy enforces rate limits:

| Zone | Limit | Scope |
|------|-------|-------|
| `api_general` | 30 req/min | Most endpoints |
| `api_history_get` | 30 req/min | Device history with auto-refresh |
| `api_login` | 10 req/min | Login endpoint |

File uploads (firmware): `client_max_body_size 500M`.

## Multi-Tenancy Notes

- Each tenant has dedicated MongoDB databases: `tenant_<slug>_general`, `tenant_<slug>_usp`.
- Device credentials stored in tenant-scoped NATS KV bucket: `devices-auth-<slug>`.
- All NATS subjects include tenant slug for message isolation.
- Container images prefixed with tenant slug: `<tenant>/<name>:<tag>`.
- SuperAdmin can switch tenants via the `setActiveTenant(slug)` frontend method.
