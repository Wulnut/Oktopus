# Security Notes

## Authentication and Authorization

### JWT Authentication

All API requests (except `/api/auth/*`) require a JWT token in the `Authorization` header. Tokens are signed with HS256 using the `SECRET_API_KEY` environment variable (auto-generated on first run). Tokens expire after 24 hours.

JWT claims include:
- `email` — user email
- `tenant_id` — tenant ObjectID hex (empty for SuperAdmin)
- `tenant_slug` — tenant slug for DB/route resolution (empty for SuperAdmin)
- `level` — user privilege level (0=SuperAdmin, 1=TenantAdmin, 2=Operator)

### Middleware Chain

All tenant-scoped routes (`/api/tenants/{slug}/*`) pass through two middleware layers:

1. **AuthMiddleware** — validates JWT signature and expiry, extracts claims into request context
2. **TenantMiddleware** — verifies the URL `{slug}` matches the JWT `tenant_slug` for non-SuperAdmin users. SuperAdmin can access any tenant. Suspended/disabled tenants are blocked.

### User Levels and Permissions

| Action | SuperAdmin (0) | TenantAdmin (1) | Operator (2) |
|--------|:-:|:-:|:-:|
| Create/manage tenants | Yes | No | No |
| Access any tenant | Yes | No | No |
| Create users in tenant | Yes | Yes | No |
| Delete users in tenant | Yes | Own tenant only | No |
| Clear message history | Yes | Yes | No |
| Manage devices/firmware/scripts | Yes | Yes | Yes |
| View devices/firmware/scripts | Yes | Yes | Yes |

TenantAdmin can only create Operator-level users. SuperAdmin can create TenantAdmin-level users. No user can escalate their own level.

### Tenant Isolation — API Level

- All data routes are scoped under `/api/tenants/{slug}/`
- TenantMiddleware blocks cross-tenant access for non-SuperAdmin users (returns 403)
- `GET /api/tenants/{slug}` checks that the requesting user's tenant matches the slug
- `GET /api/tenants` (list all) is SuperAdmin-only
- Tenant management (create/update/delete) is SuperAdmin-only

### Tenant Isolation — Database Level

Each tenant has dedicated MongoDB databases:
- `tenant_<slug>_general` — firmware, scripts, campaigns, templates, device_info, etc.
- `tenant_<slug>_usp` — messages, message_errors, device_metrics

Handlers use `a.tenantDB(r)` which resolves the tenant slug from middleware context and returns a `*TenantDB` scoped to that tenant's databases. There is no way for a handler to accidentally query another tenant's data through this pattern.

The shared `account-mngr` database holds users (with `TenantID` field) and tenant records. User queries filter by `TenantID`.

### Tenant Isolation — NATS Level

All NATS subjects include the tenant slug: `adapter.usp.v1.<tenant_slug>.devices.*`. Device credentials are stored in tenant-scoped NATS JetStream KeyValue buckets: `devices-auth-<tenant_slug>`.

The adapter service subscribes to wildcard subjects (`adapter.usp.v1.*.*`) and filters by tenant. This is internal infrastructure — tenant isolation is enforced at the API layer (middleware) and transport layer (device destinations), not at the NATS level.

### Tenant Deletion

Deleting a tenant performs full cleanup:
- Firmware files deleted from file server
- Container images deleted from registry (all `<tenant_slug>/*` repos)
- Devices deleted from adapter DB
- All tenant users deleted
- Tenant MongoDB databases dropped
- NATS KV bucket deleted
- Tenant record removed

Registry cleanup runs in the background (best-effort).

## Container Registry Tenant Isolation

### How it works

Container images are stored in a shared Docker Registry V2 instance. Tenant isolation is enforced at the naming level: all images are prefixed with the tenant slug (e.g., `prpl-test/my-container:v1.0`).

- **Upload**: The container-upload service extracts the tenant slug from the JWT (or `X-Tenant-Slug` header for SuperAdmin) and prefixes the image name automatically. A tenant cannot upload to another tenant's namespace.
- **Catalog browsing**: The `/docker-registry/` nginx proxy requires an `Authorization` header. The frontend filters the catalog to only show images matching the current tenant's prefix.
- **Delete**: The container-upload service extracts the tenant slug and prefixes the image name, so a tenant can only delete their own images.
- **Deployment**: The controller constructs `docker://` URLs with the tenant prefix when deploying containers to devices via USP `InstallDU()`.

### Known limitation: download-level isolation

The Docker Registry itself does NOT enforce authentication for image pulls. Any client that knows an image name (e.g., `docker://host/other-tenant/container:tag`) can pull it.

**Why this is acceptable:**
1. Devices only pull URLs constructed by the controller, not arbitrary URLs.
2. The controller only constructs URLs with the authenticated tenant's prefix.
3. The `/_catalog` endpoint requires authentication, so tenants cannot discover other tenants' image names.
4. A device would need to be deliberately reconfigured to pull from another tenant's namespace — this is outside the platform's threat model.

### Known limitation: registry exposure

The Docker Registry is exposed on the host network (port 443). Any client with network access could potentially push images directly to the registry, bypassing the tenant prefix enforcement. This is mitigated by:
1. The container-upload service being the only intended upload path
2. The frontend only displaying images matching the tenant prefix
3. A warning banner is displayed on the Container Store and LCM pages

**If stronger isolation is needed:**
- Remove the host port mapping for the registry (route all access through nginx)
- Deploy a registry proxy with per-tenant token validation
- Use separate registries per tenant
- Implement Docker Registry token authentication (Bearer token service)

## Secret Management

Secrets are auto-generated on first deployment by `deploy/compose/generate-secrets.sh`:

| Secret | Purpose | Storage |
|--------|---------|---------|
| `NATS_USER` / `NATS_PW` | NATS broker authentication | `.env.nats`, embedded in `NATS_URL` for each service |
| `SECRET_API_KEY` | JWT signing key | `.env.controller` |
| `JWT_SECRET` | Firmware upload auth | `.env.firmware-upload` |
| `CA_PASSPHRASE` | Registry CA certificate passphrase | Docker compose env |

Generated `.env.*` files are gitignored. To rotate secrets, delete `.env.*` files and restart — new secrets will be generated. Note: rotating `SECRET_API_KEY` invalidates all existing JWT tokens (users must re-login).

## Device Authentication

Device credentials (username/password) are stored in NATS JetStream KeyValue buckets, scoped per tenant (`devices-auth-<tenant_slug>`). Credentials are managed via the Credentials page in the frontend.

Currently, device credential validation at the MTP transport layer is not fully implemented — devices connect with tenant-specific destinations but credential verification against the tenant's KV bucket happens at the API level. This means a device with valid credentials for one tenant could theoretically connect to another tenant's transport endpoint if reconfigured. Full transport-layer credential enforcement is planned for a future release.

## TLS

- **Registry**: Auto-generated TLS certificates via `registry-certs-generator`. Certificates include all host IP addresses as SANs and are regenerated if IPs change.
- **NATS**: TLS enabled between services using certificates from `deploy/compose/nats_config/`.
- **Frontend/API**: Currently served over HTTP via nginx. Production deployments should add TLS termination at the nginx layer.
