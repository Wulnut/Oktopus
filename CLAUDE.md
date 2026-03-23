# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Oktopus

Oktopus is an Open Source USP (User Services Platform) Controller and CWMP (CPE WAN Management Protocol) multi-vendor management platform for CPEs and IoT devices. It manages and controls network devices using multiple transport protocols.

## Running the Project

```bash
# Start all services (production)
cd deploy/compose && ./run.sh

# Start with frontend hot-reload (development)
cd deploy/compose && ./run_debug.sh

# Stop all services
cd deploy/compose && ./stop.sh
```

The compose setup uses Docker profiles: `nats`, `controller`, `cwmp`, `mqtt`, `stomp`, `ws`, `adapter`, `frontend`, `portainer`, `registry`.

## Frontend Development

```bash
cd frontend
npm run dev        # Dev server on localhost:3000
npm run build      # Production build
npm run lint       # ESLint check
npm run lint-fix   # Auto-fix ESLint issues
```

## Building Docker Images

```bash
# Build all images
cd build && make build

# Build only backend or frontend
cd build && make build-backend
cd build && make build-frontend

# Release (build + push to Docker Hub)
cd build && make release
```

Individual services also have their own `Makefile` in their `build/` subdirectory.

## Architecture Overview

### Microservices (Go backend)

All backend services live in `backend/services/` and communicate exclusively through **NATS** message broker. Each service is independently containerized.

**Controller** (`backend/services/controller/`) — the central service:
- REST API on port 8000 (Gorilla Mux), JWT-authenticated
- Connects to MongoDB for persistence and NATS for inter-service messaging
- `internal/api/` — HTTP handlers grouped by domain (device, usp, cwmp, user, wifi, history, info, firmware, scripts, mass-actions)
- `internal/bridge/` — NATS request/response helpers (`NatsReq`, `NatsUspInteraction`, `NatsCwmpInteraction`)
- `internal/usp/` — USP protocol (protobuf) message handling and storage
- `internal/cwmp/` — CWMP protocol handling
- `internal/entity/` — shared data models (`MsgAnswer[T]` generic wrapper for all NATS responses)
- Entry point: `cmd/controller/main.go`

**MTP services** (`backend/services/mtp/`) — transport protocol layer:
- `mqtt/`, `ws/`, `stomp/` — protocol listeners (MQTT:1883, WebSocket:8080, STOMP:61613)
- `mqtt-adapter/`, `ws-adapter/`, `stomp-adapter/` — normalize protocol messages to NATS
- `adapter/` — generic adapter that bridges MTP adapters to the controller via NATS

**Utility services**:
- `utils/socketio/` — Socket.IO bridge (port 5000) for real-time frontend updates via NATS events
- `utils/file-server/` — firmware/image file serving (port 8004)
- `acs/` — CWMP Auto Configuration Server (port 9292)

### NATS Subject Naming Convention

```
device.usp.v1.<sn>      — outbound to USP devices
device.cwmp.v1.<sn>     — outbound to CWMP devices
mqtt.usp.v1.*           — MQTT MTP service
mqtt-adapter.usp.v1.*   — MQTT adapter
ws.usp.v1.*             — WebSocket MTP service
ws-adapter.usp.v1.*     — WebSocket adapter
stomp-adapter.usp.v1.*  — STOMP adapter
adapter.usp.v1.*        — generic adapter
cwmp-adapter.v1.*       — CWMP adapter
account-manager.v1.*    — account management
```

Device auth tokens are stored in a NATS JetStream KeyValue bucket named `devices-auth`.

### Frontend (Next.js)

- **Framework**: Next.js 14 + React 18 + Material UI 5
- **Real-time**: Socket.IO client connected to the `socketio` service
- `src/pages/` — Next.js pages (devices, firmware, mass-actions, scripts, containers-store, credentials, companies, etc.)
- `src/sections/` — heavy page-specific components
- `src/components/` — shared reusable components
- `src/contexts/` — React context providers (auth, settings, etc.)
- `src/guards/` — route protection components

### Infrastructure (deploy/compose/)

- **Nginx** (port 80) — reverse proxy/API gateway; config in `deploy/compose/nginx.conf`
- **MongoDB** (port 27017) — primary database for controller
- **NATS** (ports 4222, 8222) — message broker with JetStream enabled; config in `deploy/compose/nats_config/`
- **Docker Registry** (port 443) — private registry with auto-generated TLS certs via `registry-certs-generator`
- **Portainer** (port 9443) — container management UI
- **container-upload** (port 8005) — custom service for uploading containers to the local registry

Environment variables for each service are in `.env.<service>` files within `deploy/compose/`.

### MongoDB Collections

**Database `account-mngr`:** `users`

**Database `general`:** `templates`, `firmware`, `scripts`, `script_executions`, `mass_actions`

**Database `usp`:** `messages`, `messages_errors`, `device_metrics`, `device_info`

### Key Backend Conventions

- **entity.Device** has NO json tags — all JSON output uses PascalCase field names (`SN`, `Status`, `Vendor`, `Model`, `Alias`)
- **entity.Status** is `uint8` with iota: `Offline=0`, `Associating=1`, `Online=2`
- **Device info caching**: `deviceInfoGet` caches raw JSON in `device_info` collection; `deviceCachedInfoGet` serves it when device is offline. Raw JSON is stored as a string to avoid MongoDB BSON `primitive.D` serialization issues.
- **Offline device access**: The Info tab falls back to cached data when the device is offline, skipping USP queries entirely. Other device tabs show a "Device is Offline" banner.
