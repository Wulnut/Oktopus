# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

**IMPORTANT:** When you make changes that affect project architecture, conventions, dependencies, build/test commands, or infrastructure, update the relevant sections of this file to keep it accurate. Examples: upgrading frameworks, adding/removing services, changing database schemas, modifying build pipelines, or introducing new patterns.

## Working Style

- Do not use emojis in code, commits, or communication.
- Be precise and concise.
- When in doubt, ask the user — do not make assumptions unless explicitly asked to.
- Verify before acting: read the code, check the current state, confirm understanding.

## Recommended Plugins

The following Claude Code plugins are used in this project:

- **Feature Dev** — guided feature development with codebase understanding
- **Frontend Design** — production-grade frontend interface generation
- **Code Review** — code review for pull requests
- **Superpowers** — planning, execution, debugging, and verification workflows
- **Playwright** — browser automation for testing and verification
- **Context7** — up-to-date documentation lookup for libraries and frameworks
- **Firecrawl** — web scraping and crawling
- **PR Review Toolkit** — pull request review utilities
- **Code Simplifier** — review changed code for reuse, quality, and efficiency
- **CLAUDE.md Management** — manage and update CLAUDE.md files
- **Commit Commands** — commit workflow helpers
- **Playground** — experimentation and prototyping

## What is Oktopus

Oktopus is an Open Source USP (User Services Platform) Controller and CWMP (CPE WAN Management Protocol) multi-vendor management platform for CPEs and IoT devices. It is a multi-tenant SaaS platform where SEI (the Provider) operates the platform and serves multiple ISPs (Tenants). Each tenant manages their own devices, firmware, scripts, campaigns, and users in isolation.

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

### First-Time Setup

After starting the services:

1. Open the web UI (default: `http://localhost`)
2. Create the initial SuperAdmin account (the login page shows a registration form when no admin exists)
3. Login as SuperAdmin, go to Tenants page, create the first tenant (this provisions databases, KV bucket, and initial TenantAdmin user)
4. Login as the TenantAdmin to manage devices, firmware, scripts within that tenant

## Frontend Development

**推荐：通过 Docker 运行（避免污染宿主机）**

```bash
# 完整开发栈（所有后端服务 + 前端热重载）
cd deploy/compose && ./run_debug.sh

# 仅前端热重载（需要后端已运行）
cd deploy/compose
docker compose -f docker-compose.yaml -f docker-compose.dev.yaml -f docker-compose.debug.yaml up -d frontend
```

**本地直接运行（需要先 npm install）**

```bash
cd frontend
npm install         # 安装依赖（node_modules 在本地）
npm run dev        # Dev server on localhost:3000
npm run build      # Production build
npm run lint       # ESLint check
npm run lint-fix   # Auto-fix ESLint issues
```

- Docker 模式下 `node_modules` 和 `.next` 缓存位于容器内，不在本地目录
- 修改 `frontend/src/` 文件自动热重载，无需重建镜像
- 需要后端 API 时，至少启动 nginx + controller

### Branding & version

- Favicon / apple-touch icons use the SEI mark under `frontend/public/` (`favicon*.png`, `favicon.ico`, `apple-touch-icon.png`, `assets/sei-mark.png`)
- App version is sourced from repo-root `VERSION` (mirrored to `frontend/VERSION` for Docker builds). `frontend/scripts/write-version.js` writes `frontend/public/version.json` on `npm run predev` / `prebuild` and `run_debug.sh`. CI pack (`ci-pack-source.sh`) stamps `version.json` with pure shell using `CI_COMMIT_SHORT_SHA` (deploy job image has no node). `ci-source-deploy.sh` re-stamps and passes `OKTOPUS_GIT_COMMIT` / `OKTOPUS_VERSION` as compose build-args. `version.json` is gitignored
- Side-nav and auth footers show `version.json` label (e.g. `v3.0.0 (b61206e)`) instead of "Powered by Oktopus"
- Controller exposes `/healthz` (liveness) and `/readyz` (Mongo ping readiness). Compose controller/nginx healthchecks use `/readyz`; nginx proxies both without API rate limits
- Overview CPE Settings CWMP status shows `offline` / RTT only (no `ACS` prefix)

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

### Multi-Tenancy Model

| Term | Role | Level | Description |
|------|------|-------|-------------|
| **Provider** | SEI | — | Platform owner, operates Oktopus |
| **SuperAdmin** | Provider user | 0 | Full platform access, can enter any tenant |
| **TenantAdmin** | ISP admin | 1 | Manages users and resources within their tenant |
| **Operator** | ISP user | 2 | Day-to-day device management within tenant |

Higher level number = lower privilege. Levels are stored in `User.Level` and carried in JWT claims.

**Database isolation:** Each tenant gets dedicated MongoDB databases:
- `tenant_<slug>_general` — firmware, scripts, campaigns, mass_actions, device_info, templates, etc.
- `tenant_<slug>_usp` — messages, messages_errors, device_metrics

The shared `account-mngr` database holds `users` (with `TenantID` field) and `tenants` collections.

**API route structure:**
- `/api/auth/*` — authentication (no tenant prefix)
- `/api/tenants` — tenant management (SuperAdmin only)
- `/api/tenants/{slug}/*` — all tenant-scoped data routes (devices, firmware, scripts, etc.)

**Middleware chain:**
```
Request -> AuthMiddleware (JWT validation, extract email/tenantID/tenantSlug/level)
        -> TenantMiddleware (verify URL slug matches JWT tenant, check tenant status)
        -> Handler (uses a.tenantDB(r) for scoped DB access)
```

**JWT claims:**
```go
type JWTClaim struct {
    Username   string `json:"username"`
    Email      string `json:"email"`
    TenantID   string `json:"tenant_id"`   // ObjectID hex, empty for SuperAdmin
    TenantSlug string `json:"tenant_slug"` // for DB resolution, empty for SuperAdmin
    Level      int    `json:"level"`       // 0=SuperAdmin, 1=TenantAdmin, 2=Operator
    jwt.RegisteredClaims
}
```

### Microservices (Go backend)

All backend services live in `backend/services/` and communicate exclusively through **NATS** message broker. Each service is independently containerized.

**Controller** (`backend/services/controller/`) — the central service:
- REST API on port 8000 (Gorilla Mux), JWT-authenticated
- Connects to MongoDB for persistence and NATS for inter-service messaging
- `internal/api/` — HTTP handlers: tenant, user, device, usp, cwmp, wifi, history, info, firmware, scripts, mass-actions, campaigns, ca_cert, topology
- `internal/api/middleware/` — AuthMiddleware, TenantMiddleware, context helpers (GetEmail, GetTenantSlug, GetLevel)
- `internal/bridge/` — NATS request/response helpers (`NatsReq`, `NatsUspInteraction`, `NatsCwmpInteraction`)
- `internal/db/` — Database layer: `Database` (shared) and `TenantDB` (per-tenant). `Database.ForTenant(slug)` returns a `*TenantDB`.
- `internal/usp/` — USP protocol (protobuf) message handling, interception, and storage
- `internal/entity/` — shared data models (`MsgAnswer[T]` generic wrapper for all NATS responses)
- Entry point: `cmd/controller/main.go`
- **Campaign scheduler** (`StartCampaignScheduler`): ticks every `CAMPAIGN_SCHEDULER_INTERVAL_SEC` (default 60), runs `RunCampaignBatch` once per UTC time-window occurrence for enabled campaigns with `time_window_start/end` set. Env: `CAMPAIGN_SCHEDULER_ENABLED` (default true). Upgrade logs use `trigger_type=campaign_scheduled`.

**TenantDB pattern** — all handlers that access tenant data use:
```go
func (a *Api) someHandler(w http.ResponseWriter, r *http.Request) {
    tdb := a.tenantDB(r) // resolves tenant slug from middleware context
    tdb.SomeMethod(r.Context(), ...)
}
```

**MTP services** (`backend/services/mtp/`) — transport protocol layer:
- `mqtt/`, `ws/`, `stomp/` — protocol listeners (MQTT:1883, WebSocket:8080, STOMP:61613)
- `mqtt-adapter/`, `ws-adapter/`, `stomp-adapter/` — normalize protocol messages to NATS, extract tenant slug from transport destination
- `adapter/` — generic adapter that bridges MTP adapters to the controller via NATS, tenant-aware device storage

**Utility services**:
- `utils/socketio/` — Socket.IO bridge (port 5000) for real-time frontend updates via NATS events
- `utils/file-server/` — firmware/image file serving (port 8004)
- `acs/` — CWMP Auto Configuration Server (port 9292)

### NATS Subject Naming Convention

All subjects include the tenant slug for isolation:

```
device.usp.v1.<tenant_slug>.<sn>           — outbound to USP devices
device.cwmp.v1.<tenant_slug>.<sn>          — outbound to CWMP devices
mqtt.usp.v1.<tenant_slug>.*               — MQTT MTP service
mqtt-adapter.usp.v1.<tenant_slug>.*       — MQTT adapter
ws.usp.v1.<tenant_slug>.*                 — WebSocket MTP service
ws-adapter.usp.v1.<tenant_slug>.*         — WebSocket adapter
stomp.usp.v1.<tenant_slug>.*              — STOMP MTP service
stomp-adapter.usp.v1.<tenant_slug>.*      — STOMP adapter
adapter.usp.v1.<tenant_slug>.*            — generic adapter
cwmp-adapter.v1.<tenant_slug>.*           — CWMP adapter
device.v1.<tenant_slug>.online            — device online events (for campaigns)
device.v1.<tenant_slug>.new               — new device discovery
```

Device auth credentials are stored in tenant-scoped NATS JetStream KeyValue buckets: `devices-auth-<tenant_slug>`.

### Device-to-Tenant Assignment

Devices connect to tenant-specific transport destinations. The MTP adapter extracts the tenant slug from the destination path:
- STOMP: `Device.LocalAgent.MTP.{i}.STOMP.Destination` = `oktopus/usp/v1/<tenant_slug>/agent/<endpoint_id>`
- MQTT: `Device.LocalAgent.MTP.{i}.MQTT.ResponseTopicConfigured` = `oktopus/usp/v1/<tenant_slug>/agent/<endpoint_id>`
- WebSocket: `Device.LocalAgent.Controller.{i}.MTP.{i}.WebSocket.Path` = `/<tenant_slug>/`

The CPE Settings section on the Overview page shows the exact TR-181 parameters to configure per protocol. Reference: https://usp-data-models.broadband-forum.org/tr-181-2-19-1-usp.html

### Frontend (Next.js)

- **Framework**: Next.js 15 + React 19 + Material UI 6
- **Theme**: Dark/light mode toggle via `SettingsContext`, persisted to localStorage
- **Real-time**: Socket.IO client connected to the `socketio` service
- `src/pages/` — Next.js pages (devices, firmware, mass-actions, scripts, containers-store, credentials, tenants, settings, etc.)
- `src/sections/` — heavy page-specific components
- `src/components/` — shared reusable components
- `src/contexts/` — React context providers:
  - `auth-context.js` — authentication state, JWT token, login/logout
  - `tenant-context.js` — active tenant slug, API prefix, SuperAdmin tenant switching
  - `settings-context.js` — theme mode (dark/light), persisted to localStorage
- `src/guards/` — route protection components
- `src/theme/` — MUI theme with light (`create-palette.js`) and dark (`create-palette-dark.js`) palettes
- `src/layouts/dashboard/` — side nav, top nav (tenant selector for SuperAdmin, org name for tenant users), settings drawer

**Frontend tenant context:** All API calls use `apiPrefix` from `useTenant()`:
```javascript
const { apiPrefix, tenantSlug, isSuperAdmin } = useTenant();
// apiPrefix = '/api/tenants/<slug>' when a tenant is active
// apiPrefix = '/api' when no tenant selected (SuperAdmin without selection)
fetch(`${apiPrefix}/devices`, { headers: { Authorization: token } });
```

### Infrastructure (deploy/compose/)

- **Nginx** (port 80) — reverse proxy/API gateway; config in `deploy/compose/nginx.conf`
- **MongoDB** (port 27017) — primary database for controller and adapter
- **NATS** (ports 4222, 8222) — message broker with JetStream enabled; config in `deploy/compose/nats_config/`
- **Redis** (compose service `redis`, profile `controller`, static IP `172.16.235.22`) — ONT Lock policy cache and device state; AOF + `redis_data` volume; default-on via `LOCK_REDIS_ENABLED` in `.env.controller`. Soft-degrades if Redis is down.
- **Kafka** (compose service `kafka:9092`, profile `controller`, static IP `172.16.235.23`, apache/kafka 3.9.0 KRaft single-node) — ONT Lock audit stream on the compose network only (not published to host); default-on via `LOCK_KAFKA_ENABLED`. Soft-degrades if Kafka is down. Greenplum audit sink remains optional/off. Two compose-level workarounds are required: (1) `kafka-volume-init` one-shot busybox container `chown`s `kafka_data` to `1000:1000` before Kafka starts (apache/kafka runs as appuser; a fresh named volume is root-owned and appuser cannot write); (2) `KAFKA_LISTENERS` uses implicit bind (`://:9092`) instead of explicit `0.0.0.0` to work around [KAFKA-18281](https://issues.apache.org/jira/browse/KAFKA-18281) (3.9.0 wrongly validates the non-advertised CONTROLLER listener as routable; fixed in 3.9.1+/4.0.0).
- **Production overlay** (`docker-compose.prod.yaml`) — GCP prod VM (8GB RAM): Mongo cache 1GB, frontend heap 512MB, JetStream memory 256MB, Redis 128MB / Kafka 512MB mem limits
- **Docker Registry** (port 443) — private registry with auto-generated TLS certs via `registry-certs-generator`
- **Portainer** (port 9443) — container management UI
- **container-upload** (port 8005) — custom Node.js service for uploading containers to the local registry; prefixes images with tenant slug from JWT

Environment variables: `.env.<service>.example` templates are tracked in git; `generate-secrets.sh` creates actual `.env.<service>` files with generated secrets on first run. See README for details. Existing `.env.controller` is **not** auto-migrated — operators must add new `LOCK_*` keys (or regenerate from the example) when new vars appear.

#### ONT Lock controller env

Compose `.env.controller.example` defaults these to `true`; code defaults to `false` when the env is unset:

| Env | Notes |
| --- | --- |
| `LOCK_REDIS_ENABLED` / `LOCK_REDIS_URL` | Policy cache + device state (`last_status`, `last_ip`, notify health) |
| `LOCK_KAFKA_ENABLED` / `LOCK_KAFKA_BROKERS` / `LOCK_KAFKA_AUDIT_TOPIC` | Audit stream (default topic `ont-lock-audit`) |
| `LOCK_IP_POLL_ENABLED` / `LOCK_IP_POLL_INTERVAL_SEC` | WAN IP re-eval poller; interval min 30s, default 60s |
| `LOCK_NOTIFY_ENABLED` / `LOCK_NOTIFY_HEALTH_SEC` | USP ValueChange notify path; health window default 120s |
| `LOCK_GREENPLUM_*` | Optional audit sink; remains off by default |
| `LOCK_DEVICE_FAILURE_BACKOFF_ENABLED` / `LOCK_DEVICE_FAILURE_THRESHOLD` / `LOCK_DEVICE_FAILURE_COOLDOWN_SEC` | Per-device + per-target retryable-failure backoff (threshold 3, 10-min cooldown, post-cooldown probe, clears on any success). Code default disabled; compose true |

**Behavior:** Online/chase force-converge Set when `ShouldCommand`. Poll/notify skip Set when Redis `last_status` is unchanged; poll early-exits on same `last_ip`. Unsupported OntLock devices are listed in the UI; opt-out stops probing; re-probe on device online. Per-device failure backoff stops the same target after 3 retryable delivery failures for 10 minutes; the opposite target and a post-cooldown probe remain allowed, and any successful command clears all device backoff. State lives in the existing `lockDeviceState` Redis JSON and soft-degrades when Redis is unavailable.

### MongoDB Databases

**Shared database `account-mngr`:** `users`, `tenants`

**Per-tenant database `tenant_<slug>_general`:** `templates`, `firmware`, `scripts`, `script_executions`, `mass_actions`, `device_info`, `campaigns`, `fw_policies`, `upgrade_logs`

**Per-tenant database `tenant_<slug>_usp`:** `messages`, `messages_errors`, `device_metrics`

**Adapter database `adapter`:** `devices` (shared, filtered by `tenantid` field)

### Container Registry Tenant Isolation

Container images are prefixed with the tenant slug: `<tenant_slug>/<name>:<tag>`. The container-upload service extracts the tenant from the JWT (or `X-Tenant-Slug` header for SuperAdmin). The Container Store and LCM pages filter by tenant prefix. See `docs/SECURITY.md` for known limitations on download-level isolation.

### Key Backend Conventions

- **entity.Device** has NO json tags — all JSON output uses PascalCase field names (`SN`, `Status`, `Vendor`, `Model`, `Alias`, `TenantID`)
- **entity.Status** is `uint8` with iota: `Offline=0`, `Associating=1`, `Online=2`
- **User levels**: `SuperAdmin=0`, `TenantAdmin=1`, `Operator=2` (in `db.UserLevels`)
- **Device info caching**: `deviceInfoGet` caches raw JSON in `device_info` collection; `deviceCachedInfoGet` serves it when device is offline. Raw JSON is stored as a string to avoid MongoDB BSON `primitive.D` serialization issues.
- **Offline device access**: The Info tab falls back to cached data when the device is offline, skipping USP queries entirely. Other device tabs show a "Device is Offline" banner.
- **Tenant deletion** performs full cleanup: firmware files, registry containers, adapter devices, users, databases, KV buckets.
- **Black-box HTTP snapshot suite**: `backend/services/controller/tests/api_snapshot/` is an independent Go module that drives the real controller router over `httptest.Server` with sanitized GCP-derived fixtures. Zero production-code surface change beyond the extracted `Api.BuildRouter()` method. Coexists with the 54 white-box tests; never gates deploys (CI `test-snapshot` stage is manual/`allow_failure`). See `docs/plans/2026-07-11-test-plan-v3.md`. NATS-mediated routes (GET `/device`, `/info/*`, USP get/operate) assert the expected 500/504 timeout shape in this env since no adapter is subscribed; load tests target pure-Mongo routes only.

### Build & Deploy Scripts

All scripts are in `deploy/compose/`:

| Script | Purpose |
|--------|---------|
| `build.sh` | Build all images from source. Accepts service names: `./build.sh controller` |
| `run.sh` | Deploy using locally built images |
| `run_debug.sh` | Deploy with frontend hot-reload |
| `stop.sh` | Stop all services |
| `package.sh` | Create offline deployment archive (`oktopus-deploy.tar.gz`) |
| `image-deploy.sh` | Build, verify, save, scp, and deploy individual service images |
| `ci-pack-source.sh` | Pack repo source for CI deploy (excludes data dirs, secrets, macOS `._*` / `.DS_Store`) |
| `ci-source-deploy.sh` | Remote build from source + restart stack (`staging` or `prod`). Pins `COMPOSE_PROJECT_NAME=oktopus` and tears down a legacy `compose` project if it still owns `172.16.235.0/24`. |
| `prod-migrate-layout.sh` | One-time GCP migration from flat prod-deploy layout to repo layout |
| `ci-build.sh` / `ci-deploy.sh` | Legacy registry build/pull (not used by CI; kept for manual use) |
| `prod-build-export.sh` / `prod-deploy.sh` | Offline tarball export/load for air-gapped production |
| `scripts/ont-lock-e2e/run.sh` | ONT Lock E2E: set `SN=…` then run decision-matrix cases + USP Lock readback (local/GCP). See `scripts/ont-lock-e2e/README.md`. |

### GitLab CI/CD (`.gitlab-ci.yml`)

Runner tag: `oktopus-docker`. Stages: `test-unit` → `test-integration` → `deploy`.

**Branches:**

| Branch | Deploy job | Target |
|--------|------------|--------|
| `telkomsel/ont-lock-dev` | `deploy:staging` (auto) | Test server `/root/oktopus` |
| `telkomsel/ont-lock` | `deploy:production` (auto) | GCP `/home/sei/oktopus` |

**Flow:** integration tests pass → `ci-pack-source.sh` creates tarball → SCP to server → extract → `ci-source-deploy.sh staging|prod` builds images on the server, restarts compose, then deletes `backend/`, `frontend/`, and other source (keeps only `deploy/compose/`). Set `SKIP_SOURCE_CLEANUP=1` on the server to skip cleanup for debugging.

**CI/CD variables:**

- Staging: `DEPLOY_DEV_SSH_KEY` (base64), `DEPLOY_DEV_HOST`, `DEPLOY_DEV_USER`, optional `DEPLOY_DEV_SSH_PORT`
- Production: `SSH_PRIVATE_KEY` (raw PEM or base64), `GCP_HOST`, `GCP_USER`, `SSH_PORT` (default 22)

**GCP first-time setup:** run `prod-migrate-layout.sh` on the VM if the directory is still flat from `prod-deploy.sh` (compose files in `~/oktopus` root instead of `~/oktopus/deploy/compose/`).

Deploy jobs use `timeout: 2h` and `resource_group` to prevent concurrent deploys.

- **Always use Docker** for building and testing — never use host tools (`npx`, `npm`, `node`, `go`) directly. Use `sg docker -c "..."` if the docker group requires it.
- **Build all**: `sg docker -c "cd deploy/compose && ./build.sh"`
- **Build specific service**: `sg docker -c "cd deploy/compose && ./build.sh controller"`
- **Run tests**: `cd deploy/compose && docker compose -f docker-compose.test.yaml --profile unit run --rm <service>` (see README for full list)
- **Run white-box unit/integration tests** (existing): `cd deploy/compose && docker compose -f docker-compose.test.yaml --profile {unit|integration} run --rm test-{controller-unit|bridge|db|handlers|...}`
- **Run black-box API snapshot suite** (new, see `docs/plans/2026-07-11-test-plan-v3.md`): bring up `mongo_test`/`nats_test` once, then run the independent Go module under `backend/services/controller/tests/api_snapshot/`:
  ```bash
  cd deploy/compose
  docker compose -f docker-compose.test.yaml --profile integration up -d mongo_test nats_test
  docker run --rm --network oktopus-test_test_network \
    -v "$PWD/../..":/workspace \
    -w /workspace/backend/services/controller/tests/api_snapshot \
    -e GOPROXY=https://goproxy.cn,direct -e GOFLAGS=-mod=mod \
    -e MONGO_TEST_URI=mongodb://mongo_test:27017 \
    -e NATS_TEST_URL=nats://nats_test:4222 \
    -e SECRET_API_KEY=test-secret-key-for-snapshot \
    golang:1.23 go test -v -count=1 -timeout=300s ./...
  ```
  Or via the compose service: `docker compose -f docker-compose.test.yaml --profile snapshot --profile integration run --rm test-controller-snapshot`.
- **Run load tests** (gated, off by default): add `-e RUN_LOAD=1` and target `./load/...` with `-run TestLoad`. The default `go test ./...` skips them.

