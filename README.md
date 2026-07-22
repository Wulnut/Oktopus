# Oktopus

Oktopus is an Open Source USP Controller and CWMP compatible multi-vendor management platform for CPEs and IoTs. It is a multi-tenant SaaS platform where each ISP (tenant) manages their own devices, firmware, scripts, and campaigns in isolation.

**Original Project:** [https://github.com/OktopUSP/oktopus](https://github.com/OktopUSP/oktopus)

## Quickstart

```bash
cd deploy/compose
./build.sh        # Build all images from source
./run.sh          # Deploy all services
```

On first run, `generate-secrets.sh` automatically creates `.env.*` files with random credentials from the `.env.*.example` templates. The UI is available at `http://localhost`.

### Scripts

| Script | Purpose |
|--------|---------|
| `build.sh` | Build all Docker images from source. Pass service names to build specific ones: `./build.sh controller` |
| `run.sh` | Deploy all services using locally built images |
| `run_debug.sh` | Deploy with frontend hot-reload (source mounted, no rebuild needed for frontend changes) |
| `docker compose ... up -d frontend` | Run only the frontend (hot-reload) in Docker, without other services |
| `stop.sh` | Stop all services |
| `package.sh` | Create offline deployment archive (see [Offline Deployment](#offline-deployment)) |

Stop all services:

```bash
cd deploy/compose
./stop.sh
```

### First-Time Setup

After starting services, open the web UI:

1. **Create SuperAdmin** — the login page shows a registration form when no admin exists. This is the platform administrator (Provider) account.
2. **Create a Tenant** — login as SuperAdmin, go to Tenants page, click "Create Tenant". Provide a name and initial TenantAdmin credentials. This provisions:
   - Tenant-scoped MongoDB databases (`tenant_<slug>_general`, `tenant_<slug>_usp`)
   - NATS KeyValue bucket for device credentials (`devices-auth-<slug>`)
   - Initial TenantAdmin user
3. **Use the platform** — login as TenantAdmin to manage devices, firmware, scripts, campaigns, and containers for that tenant.

SuperAdmin can create multiple tenants and switch between them using the dropdown in the top navigation bar.

## Multi-Tenancy

Oktopus supports full organization-level multi-tenancy:

| Role | Level | Description |
|------|-------|-------------|
| **SuperAdmin** | 0 | Platform owner (SEI). Full access to all tenants. |
| **TenantAdmin** | 1 | ISP administrator. Manages users and resources within their tenant. |
| **Operator** | 2 | ISP user. Day-to-day device management within their tenant. |

**Isolation model:**
- Each tenant has dedicated MongoDB databases for all data (firmware, scripts, messages, device info, etc.)
- Device credentials stored in tenant-scoped NATS KV buckets
- All API routes scoped under `/api/tenants/{slug}/`
- Container registry images prefixed with tenant slug (`<tenant>/<name>:<tag>`)
- NATS subjects include tenant slug for message isolation

## Device Connection

Devices connect via STOMP, MQTT, or WebSocket. Each device must be configured with tenant-specific destinations. The **CPE Settings** section on the Overview page shows the exact TR-181 parameters for each protocol.

Example for STOMP:
```
Device.LocalAgent.MTP.1.Protocol                          STOMP
Device.LocalAgent.MTP.1.STOMP.Destination                 oktopus/usp/v1/<tenant_slug>/agent/<endpoint_id>
Device.STOMP.Connection.1.Host                            <server_host>
Device.STOMP.Connection.1.Port                            61613
Device.LocalAgent.Controller.1.MTP.1.STOMP.Destination    oktopus/usp/v1/<tenant_slug>/controller/<endpoint_id>
```

## Advanced Configuration

### Environment Files

Each service has a `.env.<service>.example` template in `deploy/compose/`. On first run, `generate-secrets.sh` copies these to `.env.<service>` with generated secrets. The `.env.*` files are gitignored; only `.example` templates are tracked.

**Auto-generated secrets** (shared across services):

| Secret | Used by |
|--------|---------|
| `NATS_USER` / `NATS_PW` | All NATS-connected services (embedded in `NATS_URL`) |
| `JWT_SECRET` | `controller` (`SECRET_API_KEY`) and `firmware-upload` |

**Service environment files:**

| File | Key variables |
|------|--------------|
| `.env.nats` | `NATS_NAME`, `NATS_USER`, `NATS_PW` |
| `.env.controller` | `MONGO_URI`, `NATS_URL`, `FIRMWARE_UPLOAD_URL`, `SECRET_API_KEY` |
| `.env.adapter` | `MONGO_URI`, `NATS_URL` |
| `.env.mqtt` | `NATS_URL` |
| `.env.mqtt-adapter` | `MQTT_URL`, `NATS_URL` |
| `.env.ws` | `NATS_URL` |
| `.env.ws-adapter` | `WS_ADDR`, `NATS_URL` |
| `.env.stomp` | `NATS_URL`, `STOMP_SERVICE_USER`, `STOMP_SERVICE_KEY` |
| `.env.stomp-adapter` | `STOMP_SERVER`, `STOMP_USER`, `STOMP_PASSWD`, `NATS_URL` |
| `.env.acs` | `NATS_URL` |
| `.env.socketio` | `NATS_URL` |
| `.env.firmware-upload` | `SERVER_PORT`, `FIRMWARE_DIR`, `JWT_SECRET` |
| `.env.file-server` | `DIRECTORY_PATH`, `SERVER_PORT` |

To regenerate secrets, delete the `.env.*` files and re-run `./run.sh`.

### Running Tests

All tests use `deploy/compose/docker-compose.test.yaml` with Docker profiles.

```bash
cd deploy/compose

# Unit tests
docker compose -f docker-compose.test.yaml --profile unit run --rm test-controller-unit
docker compose -f docker-compose.test.yaml --profile unit run --rm test-acs
docker compose -f docker-compose.test.yaml --profile unit run --rm test-adapter
docker compose -f docker-compose.test.yaml --profile unit run --rm test-mqtt
docker compose -f docker-compose.test.yaml --profile unit run --rm test-infra
docker compose -f docker-compose.test.yaml --profile unit run --rm test-frontend

# Integration tests (spins up MongoDB + NATS automatically)
docker compose -f docker-compose.test.yaml --profile integration run --rm test-db
docker compose -f docker-compose.test.yaml --profile integration run --rm test-bridge
docker compose -f docker-compose.test.yaml --profile integration run --rm test-handlers

# Black-box HTTP snapshot suite (see docs/plans/2026-07-11-test-plan-v3.md)
# Runs the real controller router over httptest.Server with sanitized GCP-derived fixtures.
docker compose -f docker-compose.test.yaml --profile snapshot --profile integration run --rm test-controller-snapshot

# Same suite with load tests enabled (RUN_LOAD=1 gates TestLoad_* in the load/ subpackage)
RUN_LOAD=1 docker compose -f docker-compose.test.yaml --profile snapshot --profile integration run --rm test-controller-snapshot

# Clean up
docker compose -f docker-compose.test.yaml --profile unit --profile integration --profile snapshot down
```

### Certificates

Registry certificates are automatically generated by the `registry-certs-generator` service. Certificates are stored in the `registry-certs` Docker volume and include all host IP addresses. Certificates are regenerated automatically if host IPs change.

### Containers & Registry

- **Docker Registry**: Integrated into compose with automated TLS certificate generation (port 443)
- **Tenant Isolation**: Container images are prefixed with tenant slug (`<tenant>/<name>:<tag>`)
- **Container Management**: UI for uploading, managing, and deploying containers to devices via USP LCM
- **Container Store page**: Lists containers for the current tenant, supports upload and delete
- **LCM page**: Deploy containers to devices using `Device.SoftwareModules.InstallDU()` USP command

See `docs/SECURITY.md` for known limitations on registry isolation.

## Frontend

- **Dark/Light Theme**: Toggle via the gear icon in the top navigation. Default: dark mode. Persisted to localStorage.
- **Tenant Selector**: SuperAdmin sees a dropdown to switch tenants. Tenant users see their organization name.
- **Settings Drawer**: Right-side drawer with theme toggle and future settings.

## Offline Deployment

To deploy on a VM without git or build tools:

```bash
# On the build machine
cd deploy/compose
./package.sh                    # Creates oktopus-deploy.tar.gz

# Copy to target VM, then:
tar xzf oktopus-deploy.tar.gz
cd oktopus-deploy
docker load -i images.tar       # Load all Docker images
./run.sh                        # Deploy
```

The archive contains all Docker images, compose config, nginx config, env templates, and scripts. Only Docker and docker compose are required on the target VM.

## Documentation

- [Official Website](https://oktopus.app.br/controller)
- [Official Documentation](https://docs.oktopus.app.br)
- [Security Notes](docs/SECURITY.md)
- [Architecture Details](CLAUDE.md)
