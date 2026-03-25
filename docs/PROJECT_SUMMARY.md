# Oktopus Project Summary

## What It Is

Oktopus is an open-source USP (TR-369) and CWMP (TR-069) controller platform for managing CPE and IoT devices across multiple transport protocols (MQTT, WebSocket, STOMP). Licensed under Apache 2.0.

## Architecture

Microservices backend in Go, Next.js frontend, connected via NATS message broker, persisted in MongoDB.

### Services (11 total)

| Service | Port | Language | Role |
|---|---|---|---|
| controller | 8000 | Go | REST API, business logic, MongoDB/NATS integration |
| mqtt | 1883 | Go | MQTT broker (mochi-mqtt) |
| mqtt-adapter | - | Go | MQTT to NATS bridge |
| ws | 8080 | Go | WebSocket server |
| ws-adapter | - | Go | WebSocket to NATS bridge |
| stomp | 61613 | Go | STOMP server |
| stomp-adapter | - | Go | STOMP to NATS bridge |
| adapter | - | Go | Generic MTP-to-controller bridge |
| socketio | 5000 | Go | Real-time frontend events |
| file-server | 8004 | Go | Firmware file serving |
| firmware-upload | 8006 | Node.js | Multipart firmware upload |
| acs | 9292 | Go | CWMP Auto Configuration Server |
| frontend | 3000 | Node.js | Next.js 14 UI |

### Infrastructure

| Component | Port | Purpose |
|---|---|---|
| Nginx | 80 | Reverse proxy, rate limiting |
| MongoDB | 27017 | Primary database |
| NATS | 4222/8222 | Message broker (JetStream) |
| Docker Registry | 443 | Private image registry |
| Portainer | 9443 | Container management UI |

### Databases (MongoDB)

| Database | Collections |
|---|---|
| account-mngr | users |
| general | templates, firmware, scripts, script_executions, mass_actions, device_info |
| usp | messages, messages_errors, device_metrics |

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
  api/         15 files, ~4700 lines, 30+ endpoints
  db/           9 files, ~1200 lines, 10 collections
  entity/       4 files (Device, Status, MsgAnswer[T], MTP constants)
  bridge/       1 file, NATS request helpers (NatsReq, NatsUspInteraction, NatsCwmpInteraction)
  usp/          Protobuf USP message handling, message interceptor
  cwmp/         XML CWMP message construction
  config/       Configuration (CLI flags, env vars, .env file)
  nats/         NATS client setup
```

### API Endpoints (grouped)

- **Auth:** login, register, password management
- **Devices:** list, auth, alias, USP/CWMP message passing
- **Device Info:** info, WiFi, interfaces, performance, metrics history, reboot, factory-reset
- **Topology:** WiFi clients, hosts, Ethernet
- **Firmware:** CRUD, upload, phase management
- **Scripts:** CRUD, execute, execution history
- **Mass Actions:** batch firmware updates, batch script execution, cancel
- **Templates:** CRUD
- **Users:** list

All protected routes use JWT middleware.

### Conventions

- `entity.Device` has no JSON tags; output uses PascalCase (`SN`, `Status`, `Vendor`, `Model`)
- `entity.Status` is `uint8` iota: Offline=0, Associating=1, Online=2
- Generic NATS response wrapper: `MsgAnswer[T]` with `Code` and `Msg` fields
- Device info cached as raw JSON string in MongoDB to avoid BSON serialization issues
- NATS request timeout: 30 seconds

## Frontend (Next.js)

**Stack:** Next.js 14, React 18, Material UI 5, Socket.IO client, ApexCharts, Formik/Yup

### Pages (22)

- Overview (dashboard)
- Devices list + detail pages (USP and CWMP, dynamic routes)
- Firmware management
- Scripts management
- Mass Actions (firmware update, script execution)
- Credentials, Containers Store, Settings, Account
- Access Control (users)
- Auth (login, register)

### Sections (heavy components)

- **devices/usp/** (9 files): info, network, bridging, performance, topology, discovery, LCM, history, RPC
- **devices/cwmp/** (2 files): WiFi, RPC
- **firmware/** (3 files): table, upload dialog, edit dialog
- **scripts/** (4 files): table, editor, execute dialog, history
- **mass-actions/** (2 files): device selector, detail view
- **overview/** (8 files): dashboard widgets

### State Management

4 React Context providers: auth, backend API, alerts, Socket.IO

### Patterns

- File-based routing with `Component.getLayout()` for layout composition
- Auth guard on protected routes
- Real-time updates via Socket.IO
- Offline device support with cached data fallback

## Build & Deploy

```bash
# Development
cd deploy/compose && ./run_debug.sh

# Production
cd deploy/compose && ./run.sh

# Build Docker images
cd build && make build

# Frontend dev server
cd frontend && npm run dev
```

Docker Compose uses profiles: nats, controller, cwmp, mqtt, stomp, ws, adapter, frontend, portainer, registry.

All Go services use multi-stage Docker builds (golang:1.23 -> alpine:3.14), CGO disabled.

Kubernetes manifests available in `deploy/kubernetes/`.

CI/CD via CircleCI (`.circleci/config.yml`).

## Documentation

| File | Content |
|---|---|
| `CLAUDE.md` | AI assistant guidance, architecture reference |
| `README.md` | Project overview, usage |
| `CONTRIBUTING.md` | Contribution guidelines |
| `docs/start.md` | Current state, features, development goals |
| `docs/plans/2026-03-04-roadmap-features.md` | Detailed roadmap (firmware, dashboard, topology) |
| `docs/plans/2026-03-19-scripts-feature.md` | Scripts feature implementation plan |
| `docs/plans/2026-03-19-mass-actions.md` | Mass actions implementation plan |

## Recently Completed Features

1. Firmware Management -- upload, versioning, vendor/model matching, phase management
2. Per-Device Dashboard -- Info, Network, Performance, Bridging tabs with live USP queries
3. Network Topology -- WiFi clients, hosts, Ethernet visualization
4. Scripts -- saved USP command sequences with step types (GET, SET, ADD, DELETE, OPERATE, CONDITION, DELAY)
5. Mass Actions -- batch firmware updates and script execution across device groups
6. Offline Device Access -- cached device info served when device is offline
