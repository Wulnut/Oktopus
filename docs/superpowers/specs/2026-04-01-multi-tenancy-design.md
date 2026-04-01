# Multi-Tenancy Design Spec

## Overview

Add organization-level multi-tenancy to Oktopus. SEI (the Provider) runs the platform and serves multiple ISPs (Tenants). Each tenant manages their own devices, firmware, scripts, campaigns, and users in isolation. Provider users (SuperAdmin) can access any tenant for support purposes.

## Terminology

| Concept | Term | Description |
|---------|------|-------------|
| SEI | **Provider** | Platform owner, operates Oktopus |
| ISP / client org | **Tenant** | Manages their own CPEs, users, and resources |
| Provider admin | **SuperAdmin** (level 0) | Full platform access, can enter any tenant |
| Tenant admin | **TenantAdmin** (level 1) | Manages users and resources within their tenant |
| Tenant user | **Operator** (level 2) | Day-to-day device management within tenant |

Higher level number = lower privilege. Future roles (e.g., end-user) can be added as level 3+ without migration.

## Data Model

### `tenants` collection (in `account-mngr` DB)

```go
type TenantCACert struct {
    ID       primitive.ObjectID `bson:"_id,omitempty" json:"id"`
    Label    string             `bson:"label" json:"label"`           // e.g. "Primary CA 2026"
    PEM      string             `bson:"pem" json:"pem"`               // PEM-encoded CA certificate
    NotAfter time.Time          `bson:"not_after" json:"not_after"`   // cert expiry (extracted from PEM)
    AddedAt  time.Time          `bson:"added_at" json:"added_at"`
}

type TenantAuthPolicy struct {
    PasswordRequired bool `bson:"password_required" json:"password_required"` // default: true
    CertRequired     bool `bson:"cert_required" json:"cert_required"`         // default: false
}

type Tenant struct {
    ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
    Name       string             `bson:"name" json:"name"`           // Display name, e.g. "Acme ISP"
    Slug       string             `bson:"slug" json:"slug"`           // URL-safe unique identifier
    Status     string             `bson:"status" json:"status"`       // "active", "suspended", "disabled"
    AuthPolicy TenantAuthPolicy   `bson:"auth_policy" json:"auth_policy"`
    CACerts    []TenantCACert     `bson:"ca_certs,omitempty" json:"ca_certs,omitempty"`
    CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
    UpdatedAt  time.Time          `bson:"updated_at" json:"updated_at"`
}
```

Indexes:
- `slug`: unique

### `users` collection changes (in `account-mngr` DB)

```go
type UserLevels int32

const (
    SuperAdmin  UserLevels = iota // 0 — Provider (SEI)
    TenantAdmin                   // 1 — ISP admin
    Operator                      // 2 — ISP operator
)

type User struct {
    Email    string             `json:"email"`
    Name     string             `json:"name"`
    Password string             `json:"password,omitempty"`
    Level    UserLevels         `json:"level"`
    Phone    string             `json:"phone"`
    TenantID primitive.ObjectID `json:"tenant_id,omitempty" bson:"tenant_id,omitempty"`
}
```

- SuperAdmin users: `TenantID` is zero-value (omitted)
- TenantAdmin and Operator: `TenantID` is required, references `tenants._id`

### Per-tenant databases

On tenant creation, two databases are provisioned with the same collection structure and indexes as the current `general` and `usp` databases:

- `tenant_<slug>_general` — collections: `firmware`, `scripts`, `script_executions`, `mass_actions`, `device_info`, `campaigns`, `fw_policies`, `upgrade_logs`, `templates`
- `tenant_<slug>_usp` — collections: `messages`, `messages_errors`, `device_metrics`

### Device entity change

```go
type Device struct {
    SN           string
    Model        string
    TenantID     string    // replaces unused Customer field
    Vendor       string
    Version      string
    ProductClass string
    HWVersion    string
    Alias        string
    Status       Status
    Mqtt         Status
    Stomp        Status
    Websockets   Status
    Cwmp         Status
}
```

`TenantID` is set at the transport layer when a device first connects via a tenant-specific endpoint.

## JWT Claims

```go
type JWTClaim struct {
    Username   string `json:"username"`
    Email      string `json:"email"`
    TenantID   string `json:"tenant_id"`   // ObjectID hex, empty for SuperAdmin
    TenantSlug string `json:"tenant_slug"` // for DB resolution without lookup
    Level      int    `json:"level"`       // 0=SuperAdmin, 1=TenantAdmin, 2=Operator
    jwt.RegisteredClaims
}
```

## API Route Structure

### Tenant management (SuperAdmin only)

```
POST   /api/tenants                          # create tenant
GET    /api/tenants                          # list all tenants
GET    /api/tenants/:slug                    # get tenant details
PUT    /api/tenants/:slug                    # update tenant
DELETE /api/tenants/:slug                    # delete tenant
```

### Tenant-scoped resources

All data endpoints move under `/api/tenants/:slug/`:

```
GET|POST|PUT|DELETE  /api/tenants/:slug/devices/...
GET|POST|PUT|DELETE  /api/tenants/:slug/firmware/...
GET|POST|PUT|DELETE  /api/tenants/:slug/scripts/...
GET|POST|PUT|DELETE  /api/tenants/:slug/campaigns/...
GET|POST|PUT|DELETE  /api/tenants/:slug/mass-actions/...
GET                  /api/tenants/:slug/info/...
GET                  /api/tenants/:slug/messages/...
```

### User management (tenant-scoped)

```
GET    /api/tenants/:slug/users              # list tenant users
POST   /api/tenants/:slug/users              # create user in tenant
DELETE /api/tenants/:slug/users/:email        # delete user
```

### Device credentials (tenant-scoped)

```
GET    /api/tenants/:slug/device/auth
POST   /api/tenants/:slug/device/auth
DELETE /api/tenants/:slug/device/auth
```

### Auth (no tenant prefix)

```
PUT    /api/auth/login
POST   /api/auth/register
POST   /api/auth/admin/register
GET    /api/auth/admin/exists
```

Login returns JWT with tenant context. The `admin/register` and `admin/exists` endpoints handle first-time SuperAdmin setup.

## Middleware Architecture

Request flow through middleware chain:

```
Request
  -> Auth middleware (validate JWT, extract email/level/tenantID/tenantSlug)
  -> Tenant resolution middleware (on /api/tenants/:slug/* routes):
       - SuperAdmin: uses :slug from URL path
       - TenantAdmin/Operator: verifies :slug matches JWT tenantSlug, 403 if mismatch
       - Checks tenant status, 403 if suspended/disabled
  -> Tenant DB injection (resolves TenantDB from slug, injects into request context)
  -> Handler (uses TenantDB from context, no tenant logic in handlers)
```

## DB Layer Changes

```go
// TenantDB holds references to a specific tenant's databases
type TenantDB struct {
    General *mongo.Database  // tenant_<slug>_general
    Usp     *mongo.Database  // tenant_<slug>_usp
    // Collection accessors for each collection
}

// Database keeps the shared account-mngr DB and provides tenant DB resolution
type Database struct {
    AccountMgr *mongo.Database  // shared: users, tenants collections
    client     *mongo.Client
}

// ForTenant returns a TenantDB for the given tenant slug
func (db *Database) ForTenant(slug string) *TenantDB {
    return &TenantDB{
        General: db.client.Database("tenant_" + slug + "_general"),
        Usp:     db.client.Database("tenant_" + slug + "_usp"),
    }
}
```

Handlers receive `TenantDB` from request context. Provider-level operations (tenant CRUD, user listing across tenants) use `Database.AccountMgr` directly.

## Tenant Lifecycle

### Creation (SuperAdmin only)

`POST /api/tenants` triggers:

1. Validate tenant name, generate slug (lowercase, hyphenated)
2. Insert `Tenant` record into `tenants` collection (status: "active")
3. Create `tenant_<slug>_general` database with all collection indexes
4. Create `tenant_<slug>_usp` database with all collection indexes
5. Create initial TenantAdmin user for the tenant

### Suspension

Set `Tenant.Status = "suspended"`. Middleware rejects all requests from that tenant's users with 403. Devices remain connected but commands are blocked at the API layer.

### Deletion

1. Drop `tenant_<slug>_general` and `tenant_<slug>_usp` databases
2. Remove all users with matching `TenantID`
3. Remove tenant record from `tenants` collection
4. Device cleanup at transport layer (separate concern)

## Device-to-Tenant Assignment

Two-layer approach: transport routing determines the tenant, credentials/certificates prove the device is authorized.

### Layer 1: Tenant-specific transport endpoints

Each tenant gets dedicated endpoint paths. CPEs are provisioned with their tenant's endpoint:

| Protocol | Endpoint pattern | CPE data model |
|----------|-----------------|----------------|
| MQTT | Topic prefix `<tenant_slug>/usp/v1/<endpoint_id>` | `Device.MQTT.Client.{i}.Topic` |
| WebSocket | `ws://host/<tenant_slug>/` | `Device.WebSocket.Client.{i}.URL` (proposed) |
| STOMP | Destination prefix `/<tenant_slug>/` | `Device.STOMP.Connection.{i}.Destination` (proposed) |
| CWMP | ACS URL `http://host/acs/<tenant_slug>/` | `Device.ManagementServer.URL` |

The MTP service extracts the tenant slug from the path/topic and tags the connection. If the slug doesn't match a valid active tenant, the connection is rejected.

### Layer 2: Device authentication

Two mechanisms, usable independently or together:

#### Password-based authentication

Uses TR-181 data model fields already present on CPEs:

| Protocol | Username field | Password field |
|----------|---------------|----------------|
| MQTT | `Device.MQTT.Client.{i}.Username` | `Device.MQTT.Client.{i}.Password` |
| STOMP | `Device.STOMP.Connection.{i}.Username` | `Device.STOMP.Connection.{i}.Password` |
| WebSocket | Provided via HTTP Basic Auth or protocol-specific mechanism | |
| CWMP | `Device.ManagementServer.Username` | `Device.ManagementServer.Password` |

Platform side:
- Device credentials stored in tenant-scoped NATS KV bucket: `devices-auth-<tenant_slug>`
- MTP service validates: credential exists in the bucket for the tenant resolved from Layer 1
- A valid credential on the wrong tenant's endpoint is rejected — both layers must agree

#### Certificate-based authentication (TLS client certificates)

Uses USP trust model (TR-369 Section "Trusted Certificate Authorities"):

- Each tenant is assigned a CA certificate (or uses their own CA)
- CPE devices are provisioned with client certificates signed by their tenant's CA
- Relevant CPE data model: `Device.LocalAgent.Certificate`, `Device.Security.Certificate`
- Platform side:
  - Tenant record stores the tenant's trusted CA certificate(s)
  - MTP service terminates TLS, validates client cert against the tenant's CA (resolved from Layer 1)
  - Cert tenant identity must match the transport endpoint tenant

#### Enforcement rule

The tenant slug in the transport path MUST match the tenant that owns the credential or issued the certificate. This prevents a device from connecting to the wrong tenant even with otherwise valid credentials.

#### Device lifecycle and bootstrap

New-out-of-box devices have no tenant-specific certificate. Bootstrap flow:

1. Device ships with factory credentials (username/password from manufacturer or pre-provisioned)
2. First connect uses password auth only (no cert required)
3. Platform authenticates device, registers it to the tenant
4. Platform pushes tenant CA trust anchor to device via USP Set on `Device.Security.Certificate.{i}.`
5. Platform pushes device-specific cert via USP Set on `Device.LocalAgent.Certificate.{i}.`
6. Device reconnects with TLS client cert on subsequent connections

#### Per-tenant auth policy

Tenants configure their auth requirements:

```go
type TenantAuthPolicy struct {
    PasswordRequired bool // require device credentials (default: true)
    CertRequired     bool // require TLS client cert (default: false)
}
```

- During onboarding: `PasswordRequired: true, CertRequired: false`
- After cert provisioning: tenant can enable `CertRequired: true`
- Per-tenant choice — some tenants may never use certs

#### CA rotation

When a tenant needs to rotate their CA:

1. Tenant uploads new CA cert to platform (`POST /api/tenants/:slug/ca-certs`) — platform trusts both old and new
2. Tenant triggers mass action "Push CA Trust Anchor" — pushes new CA to all devices via USP
3. Tenant re-provisions device certs signed by new CA (via USP Set or EST)
4. Tenant removes old CA once all devices have migrated (`DELETE /api/tenants/:slug/ca-certs/:id`)

Platform tracks which CA signed each device's cert, enabling a rotation dashboard showing migration progress.

#### CA cert management API

```
GET    /api/tenants/:slug/ca-certs          # list tenant's trusted CAs
POST   /api/tenants/:slug/ca-certs          # upload new CA (PEM)
DELETE /api/tenants/:slug/ca-certs/:id       # remove old CA
```

#### Recommended deployment

- **Minimum**: Password-based auth (extends existing `devices-auth` mechanism, no PKI needed)
- **Enhanced**: Certificate-based auth (stronger identity, requires CA management per tenant)
- **Maximum**: Both (defense in depth — cert validates tenant identity, password validates device identity)

## Frontend Changes

### SuperAdmin experience

- Tenant selector dropdown in the top navigation
- Tenant management page (CRUD)
- Navigating into a tenant prefixes all API calls with `/api/tenants/<slug>/`
- All existing pages (devices, firmware, scripts, etc.) work within tenant context

### Tenant user experience

- No tenant selector — slug read from JWT on login
- Frontend auto-prefixes all API calls with `/api/tenants/<slug>/`
- Existing pages work unchanged, scoped to their tenant
- TenantAdmin sees user management for their tenant
- Operator has restricted access (exact permissions deferred)

### Companies page

The existing placeholder companies page is replaced by the real tenant management page for SuperAdmin users.

## NATS Subject Changes

Tenant context must be included in NATS subjects to ensure message isolation:

```
device.usp.v1.<tenant_slug>.<sn>       # outbound to USP devices
device.cwmp.v1.<tenant_slug>.<sn>      # outbound to CWMP devices
mqtt.usp.v1.<tenant_slug>.*            # MQTT MTP service
mqtt-adapter.usp.v1.<tenant_slug>.*    # MQTT adapter
ws.usp.v1.<tenant_slug>.*             # WebSocket MTP service
ws-adapter.usp.v1.<tenant_slug>.*     # WebSocket adapter
stomp-adapter.usp.v1.<tenant_slug>.*  # STOMP adapter
adapter.usp.v1.<tenant_slug>.*        # generic adapter
cwmp-adapter.v1.<tenant_slug>.*       # CWMP adapter
```

NATS JetStream KeyValue bucket for device auth becomes tenant-scoped: `devices-auth-<tenant_slug>`.

## Out of Scope

- Operator permission granularity (read-only vs read-write per resource) — deferred
- Billing / usage metering per tenant
- Tenant-specific branding or theming
- Rate limiting per tenant
- Automatic CPE provisioning (zero-touch) via TR-369 OnBoardRequest
- EST (Enrollment over Secure Transport) integration for automated cert issuance
- E2E session context (USP sec:e2e-message-exchange) — not needed for transport-level tenant isolation; can be added later for defense-in-depth if required
