# Per-Tenant Device Credentials for STOMP/MQTT/WebSocket

## Summary

Implement per-tenant device authentication across all three transport protocols. Start with a single shared password per tenant (configurable via Web UI), but the architecture supports multiple credentials per tenant since the KV bucket infrastructure already exists.

## Current State

| Protocol | Auth? | Tenant-aware? | Credential storage |
|----------|-------|---------------|-------------------|
| MQTT | Yes (broker hook) | Yes | Per-tenant KV bucket, key=`<tenant>/<device>` |
| STOMP | Global env var only | No | `STOMP_USERNAME`/`STOMP_PASSWORD` env vars |
| WebSocket | Optional, single global bucket | No | Global `devices-auth` KV bucket |

## Target State

| Protocol | Auth? | Tenant-aware? | Credential storage |
|----------|-------|---------------|-------------------|
| MQTT | Yes | Yes | Per-tenant KV bucket (already done) |
| STOMP | Yes | Yes | Per-tenant KV bucket |
| WebSocket | Yes | Yes | Per-tenant KV bucket |

## Credential Model

### Phase 1: Single password per tenant (this plan)

Each tenant has one shared password for all its devices. Stored as a special key in the tenant's KV bucket:

- Bucket: `devices-auth-<tenant_slug>`
- Key: `__tenant_password__`
- Value: the shared password

Device username format (all protocols): `<tenant_slug>/<endpoint_id>`

The transport service parses the username, extracts the tenant slug, opens the per-tenant KV bucket, and checks:
1. First, look for key = full username (`<tenant>/<device>`) — per-device credential
2. If not found, look for key = `__tenant_password__` — shared tenant credential
3. Compare password

This means per-device credentials (Phase 2) work automatically alongside the shared password.

### Phase 2: Per-device credentials (already supported)

The existing Credentials page (`/credentials`) already creates per-device entries in the tenant KV bucket. Once Phase 1 is done, these override the shared password for specific devices. No additional code needed — the lookup order in Phase 1 handles it.

### How hard is multiple passwords per tenant?

Not hard at all. The KV bucket already supports multiple key-value pairs. The existing Credentials page CRUD is already tenant-scoped. The only change was making the transport services (STOMP/WS) look up from per-tenant buckets — which this plan implements. After Phase 1, the Credentials page automatically works for per-device overrides.

## Implementation

### Task 1: Tenant Password API + UI

**Files:**
- Modify: `backend/services/controller/internal/api/tenant.go`
- Modify: `frontend/src/pages/tenants/index.js` or tenant detail page

Add an endpoint to set/get the tenant shared password:

- `GET /api/tenants/{slug}/device-password` — get current password (TenantAdmin+)
- `PUT /api/tenants/{slug}/device-password` — set password (TenantAdmin+)

Handler reads/writes `__tenant_password__` key in `devices-auth-<tenant_slug>` KV bucket.

UI: Add a "Device Password" field on the Tenant management page (editable by TenantAdmin+). Show the current password with a show/hide toggle and a Save button.

### Task 2: STOMP Server — NATS-Backed Authenticator

**Files:**
- Modify: `backend/services/mtp/stomp/cmd/stomp/main.go`
- Modify: `deploy/compose/docker-compose.yaml` (add NATS env + TLS volume to stomp service)

The STOMP server has an `Authenticator` interface:
```go
type Authenticator interface {
    Authenticate(login, passcode string) bool
}
```

Create a NATS-backed implementation:
```go
type NatsAuthenticator struct {
    js jetstream.JetStream
}

func (a *NatsAuthenticator) Authenticate(login, passcode string) bool {
    tenant, device := parseTenantDevice(login) // split "tenant/device"
    if tenant == "" { return false }
    
    kv, err := a.js.KeyValue(ctx, "devices-auth-"+tenant)
    if err != nil { return false }
    
    // Try per-device credential first
    entry, err := kv.Get(ctx, login)
    if err == nil { return string(entry.Value()) == passcode }
    
    // Fall back to shared tenant password
    entry, err = kv.Get(ctx, "__tenant_password__")
    if err == nil { return string(entry.Value()) == passcode }
    
    return false
}
```

In `main.go`:
- Add NATS config parsing (same pattern as other services: `NATS_URL` env var)
- Connect to NATS, get JetStream
- Replace `Credentials{}` with `NatsAuthenticator{js}`

Docker compose: add `.env.stomp` (or reuse existing) with `NATS_URL` and TLS config. Mount `nats_config` volume.

### Task 3: WebSocket Server — Per-Tenant KV Lookup

**Files:**
- Modify: `backend/services/mtp/ws/internal/ws/handler/client.go`
- Modify: `backend/services/mtp/ws/internal/nats/nats.go`
- Modify: `backend/services/mtp/ws/internal/ws/ws.go`

Currently `ServeAgent` receives a single `jetstream.KeyValue`. Change to receive `jetstream.JetStream` and resolve per-tenant bucket:

```go
func ServeAgent(w, r, cEID string, js jetstream.JetStream, authEnable bool) {
    deviceid := r.URL.Query().Get("eid")
    // Parse tenant from device ID or URL path
    tenant, device := parseTenantDevice(deviceid)
    
    if authEnable {
        kv, err := js.KeyValue(ctx, "devices-auth-"+tenant)
        // Try per-device, then shared tenant password
        // ...
    }
}
```

In `nats.go`: remove global bucket creation, return `(jetstream.JetStream, *nats.Conn)`.
In `ws.go`: pass `js` instead of `kv` to handlers.

### Task 4: MQTT Broker — Add Shared Password Fallback

**Files:**
- Modify: `backend/services/mtp/mqtt/internal/listeners/mqtt/hook.go`

The MQTT broker already does per-tenant KV lookup. Add the fallback to `__tenant_password__`:

```go
func (h *NatsAuthHook) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
    username := string(pk.Connect.Username)
    tenant, _ := parseTenantDevice(username)
    
    kv, _ := h.js.KeyValue(ctx, "devices-auth-"+tenant)
    
    // Try per-device credential
    entry, err := kv.Get(ctx, username)
    if err == nil && bytes.Equal(entry.Value(), pk.Connect.Password) {
        return true
    }
    
    // Fall back to shared tenant password
    entry, err = kv.Get(ctx, "__tenant_password__")
    if err == nil && bytes.Equal(entry.Value(), pk.Connect.Password) {
        return true
    }
    
    return false
}
```

### Task 5: Frontend — CPE Settings Username Update

**Files:**
- Modify: `frontend/src/sections/overview/overview-cpe-settings.js`

Update STOMP and WebSocket username to `<tenant>/<endpoint_id>` format (matching MQTT).

### Task 6: Build, Test, Document

- Build all services: STOMP server, WS server, MQTT broker, controller, frontend
- Test: set a tenant password via UI, connect a device with `<tenant>/<device>` username and tenant password
- Update `docs/SECURITY.md` with credential model documentation

## Device Configuration (All Protocols)

After implementation, all protocols use the same credential pattern:

```
Username: <tenant_slug>/<endpoint_id>
Password: <tenant_shared_password> (or per-device password if provisioned)
```

## Execution Order

1. Task 1 (API + UI) — enables setting passwords
2. Task 4 (MQTT fallback) — quick, MQTT already tenant-aware
3. Task 2 (STOMP auth) — biggest change, needs NATS connection
4. Task 3 (WS auth) — moderate, already has NATS
5. Task 5 (CPE Settings) — frontend only
6. Task 6 (Test + docs)
