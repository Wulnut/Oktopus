# Oktopus Project Summary

## What It Is

Oktopus is an open-source, multi-tenant USP (TR-369) and CWMP (TR-069) controller platform for managing CPE and IoT devices across multiple transport protocols (MQTT, WebSocket, STOMP). Licensed under Apache 2.0.

## Architecture

Microservices backend in Go, Next.js frontend, connected via NATS message broker, persisted in MongoDB. All data is tenant-scoped.

### Multi-Tenancy Model

Oktopus uses a three-tier role model:

| Level | Role | Scope |
|---|---|---|
| 0 | SuperAdmin | Global -- manages tenants, can switch between them |
| 1 | TenantAdmin | Single tenant -- manages users and resources within their tenant |
| 2 | Operator | Single tenant -- read/write access to tenant resources |

**Database isolation:** Each tenant gets dedicated databases (`tenant_<slug>_general`, `tenant_<slug>_usp`). The shared `account-mngr` database holds `users` and `tenants` collections.

**API routing:** All tenant-scoped data routes use `/api/tenants/{slug}/`. Tenant management routes use `/api/tenants`. Auth routes (`/api/auth/`) are unauthenticated.

**JWT claims:** Tokens include `email`, `tenant_id`, `tenant_slug`, and `level`.

**Middleware chain:** `AuthMiddleware` validates JWT and injects claims into context. `TenantMiddleware` verifies the tenant exists and enforces tenant-scoping (non-SuperAdmin users can only access their own tenant).

**TenantDB pattern:** API handlers call `a.tenantDB(r)` to get a `TenantDB` scoped to the request's tenant, providing access to `General` and `Usp` databases. The shared `a.db` is used only for cross-tenant operations (user management, tenant CRUD).

### Services (12 total)

| Service | Port | Language | Role |
|---|---|---|---|
| controller | 8000 | Go | REST API, business logic, MongoDB/NATS integration |
| mqtt | 1883 | Go | MQTT broker (mochi-mqtt) |
| mqtt-adapter | - | Go | MQTT to NATS bridge |
| ws | 8080 | Go | WebSocket server |
| ws-adapter | - | Go | WebSocket to NATS bridge |
| stomp | 61613 | Go | STOMP server |
| stomp-adapter | - | Go | STOMP to NATS bridge |
| adapter | - | Go | Generic MTP-to-controller bridge (tenant-aware) |
| socketio | 5000 | Go | Real-time frontend events |
| file-server | 8004 | Go | Firmware file serving |
| acs | 9292 | Go | CWMP Auto Configuration Server |
| frontend | 3000 | Node.js | Next.js 15 UI |

### Infrastructure

| Component | Port | Purpose |
|---|---|---|
| Nginx | 80 | Reverse proxy, rate limiting |
| MongoDB | 27017 | Primary database |
| NATS | 4222/8222 | Message broker (JetStream) |
| Docker Registry | 443 | Private image registry (tenant-scoped) |
| Portainer | 9443 | Container management UI |
| container-upload | 8005 | Container image upload (prefixes with tenant slug) |

### Databases (MongoDB)

| Database | Collections |
|---|---|
| account-mngr | users, tenants |
| tenant_\<slug\>_general | templates, firmware, scripts, script_executions, mass_actions, device_info, campaigns, fw_policies |
| tenant_\<slug\>_usp | messages, messages_errors, device_metrics |

### NATS Subject Naming

NATS subjects include the tenant slug for routing isolation:

```
adapter.usp.v1.<tenant_slug>.devices.<action>
mqtt-adapter.usp.v1.<tenant_slug>.<sn>.<event>
ws-adapter.usp.v1.<tenant_slug>.<sn>.<event>
stomp-adapter.usp.v1.<tenant_slug>.<sn>.<event>
cwmp-adapter.v1.<tenant_slug>.*
```

Device auth tokens are stored in per-tenant NATS JetStream KeyValue buckets named `devices-auth-<tenant_slug>`.

### Device-to-Tenant Routing

Devices are assigned to tenants via transport protocol destinations (MQTT topics, WebSocket paths, STOMP destinations). The adapter service extracts the tenant slug from the NATS subject and filters all device operations by `tenantid`. Device records include a `tenantid` field that gets updated on each connection.

### Container Registry

The container-upload service (port 8005) prefixes all uploaded images with the tenant slug, ensuring namespace isolation in the shared Docker registry. The tenant slug is extracted from the JWT for tenant users or from the `X-Tenant-Slug` header for SuperAdmin.

### Message Flow

```
HTTP Request -> Controller API -> NATS Bridge -> MTP Adapter -> MTP Service -> Device
Device -> MTP Service -> MTP Adapter -> NATS -> Controller -> MongoDB/Socket.IO -> Frontend
```

## Backend (Go)

**Module:** `github.com/leandrofars/oktopus`
**Go version:** 1.23.0
**Key deps:** Gorilla Mux, NATS v1.33, MongoDB driver, JWT v5, protobuf

### Controller Internal Structure

```
internal/
  api/         HTTP handlers, route registration, tenant-scoped data access
  api/auth/    JWT generation and validation (includes tenant_id, tenant_slug, level)
  api/cors/    CORS configuration
  api/middleware/  AuthMiddleware, TenantMiddleware, context helpers
  db/          MongoDB operations, TenantDB type, tenant CRUD
  entity/      Device, Status, MsgAnswer[T], MTP constants
  bridge/      NATS request helpers (NatsReq, NatsUspInteraction, NatsCwmpInteraction)
  usp/         Protobuf USP message handling, message interceptor
  cwmp/        XML CWMP message construction
  config/      Configuration (CLI flags, env vars, .env file)
  nats/        NATS client setup
```

### API Endpoints (grouped)

- **Auth:** login, register admin, check admin exists
- **Tenants:** create, list, get, update, delete (SuperAdmin only)
- **CA Certs:** list, add, remove (per tenant)
- **Users:** list, create, delete, change password (per tenant)
- **Devices:** list, auth, alias, filter options, USP/CWMP message passing
- **Device Info:** info, WiFi, interfaces, performance, metrics history, reboot, factory-reset, restart-agent, cached info, firmware policy, upgrade logs
- **Topology:** WiFi clients, hosts, Ethernet
- **Dashboard:** vendors, status, product class, general info
- **Firmware:** CRUD, phase management
- **Scripts:** CRUD, execute, execution history
- **Campaigns:** CRUD, upgrade logs
- **Mass Actions:** list, script execution, get, cancel
- **Templates:** CRUD

All protected routes use `AuthMiddleware`. Tenant-scoped routes additionally use `TenantMiddleware`.

### Conventions

- `entity.Device` has no JSON tags; output uses PascalCase (`SN`, `Status`, `Vendor`, `Model`)
- `entity.Status` is `uint8` iota: Offline=0, Associating=1, Online=2
- Generic NATS response wrapper: `MsgAnswer[T]` with `Code` and `Msg` fields
- Device info cached as raw JSON string in MongoDB to avoid BSON serialization issues
- NATS request timeout: 30 seconds
- All tenant data access uses `a.tenantDB(r)`, never `a.db` directly

## Frontend (Next.js)

**Stack:** Next.js 15, React 19, Material UI 6, Socket.IO client, ApexCharts, Formik/Yup

### Pages

- Overview (dashboard)
- Devices list + detail pages (USP and CWMP, dynamic routes)
- Firmware management
- Scripts management
- Mass Actions (script execution)
- Campaigns (firmware update campaigns)
- Credentials, Containers Store, Settings, Account
- Access Control (users)
- Tenants management (SuperAdmin)
- Chat
- Auth (login, register)

### Sections (heavy components)

- **devices/usp/** — info, network, bridging, performance, topology, discovery, LCM, history, RPC
- **devices/cwmp/** — WiFi, RPC
- **firmware/** — table, upload dialog, edit dialog
- **scripts/** — table, editor, execute dialog, history
- **mass-actions/** — device selector, detail view
- **overview/** — dashboard widgets (with CPE Settings for TR-181 parameters per protocol)

### State Management

6 React Context providers: auth, backend API, error alerts, Socket.IO, settings (theme), tenant

### Theme

Dark/light theme toggle with `SettingsContext`. Theme files in `src/theme/`:
- `create-palette.js` — light mode colors
- `create-palette-dark.js` — dark mode colors
- `index.js` — theme creation with mode selection
- Preference persisted to localStorage

### Patterns

- File-based routing with `Component.getLayout()` for layout composition
- Auth guard on protected routes
- Real-time updates via Socket.IO
- Offline device support with cached data fallback
- `TenantContext` provides `apiPrefix` (`/api/tenants/{slug}`) for all API calls
- SuperAdmin can switch active tenant via session storage

## Build & Deploy

```bash
# Development
cd deploy/compose && ./run_debug.sh

# Production
cd deploy/compose && ./run.sh

# Build Docker images
cd build && make build

# Frontend dev server (via Docker)
sg docker -c "cd deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build frontend"
```

Docker Compose uses profiles: nats, controller, cwmp, mqtt, stomp, ws, adapter, frontend, portainer, registry.

All Go services use multi-stage Docker builds (golang:1.23 -> alpine:3.14), CGO disabled.

Kubernetes manifests available in `deploy/kubernetes/`.

## Documentation

| File | Content |
|---|---|
| `CLAUDE.md` | AI assistant guidance, architecture reference |
| `README.md` | Project overview, quickstart, environment configuration |
| `CONTRIBUTING.md` | Contribution guidelines, development setup |
| `docs/start.md` | Current state, features, development goals |
| `docs/PROJECT_SUMMARY.md` | This file -- comprehensive architecture document |

## Key Features

1. **Multi-Tenancy** -- tenant creation, user roles (SuperAdmin/TenantAdmin/Operator), database isolation, tenant-scoped NATS subjects, tenant deletion with full cleanup
2. **Dark/Light Theme** -- toggle with localStorage persistence
3. **Tenant-Scoped Container Registry** -- images prefixed with tenant slug
4. **Firmware Management** -- upload, versioning, vendor/model matching, phase management, campaigns
5. **Per-Device Dashboard** -- Info, Network, Performance, Bridging tabs with live USP queries
6. **Network Topology** -- WiFi clients, hosts, Ethernet visualization
7. **Scripts** -- saved USP command sequences with step types (GET, SET, ADD, DELETE, OPERATE, CONDITION, DELAY)
8. **Mass Actions** -- batch script execution across device groups
9. **Offline Device Access** -- cached device info served when device is offline
10. **CPE Settings** -- TR-181 parameter display on Overview page per protocol
