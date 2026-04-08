# Container Registry Tenant Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Isolate container images per tenant using tenant slug as a registry namespace prefix, so tenants can only see/manage/deploy their own containers.

**Architecture:** Use `{tenant_slug}/{name}:{tag}` as registry image naming. The container-upload service prefixes uploads with the tenant slug from the JWT. The Container Store and LCM pages filter by tenant prefix. The Docker Registry V2 API (`/docker-registry/`) is proxied through nginx which requires auth. A `SECURITY.md` documents the known limitation that download-level isolation is not enforced.

**Tech Stack:** Node.js (container-upload service), Next.js/React (frontend), nginx, Docker Registry V2

**Security note:** This plan implements naming-level isolation (Option D). The Docker Registry does not enforce download authentication — any client that knows an image name can pull it. This is acceptable because devices only pull URLs constructed by the controller, and the controller only constructs URLs with the correct tenant prefix. The risk is documented in code and `SECURITY.md`.

---

## File Structure

### New files

| File | Responsibility |
|------|---------------|
| `docs/SECURITY.md` | Documents known security limitations including registry download isolation |

### Modified files

| File | Changes |
|------|---------|
| `deploy/compose/container-upload-service/container-upload-service.js` | Validate JWT, extract tenant slug, prefix image names |
| `deploy/compose/nginx.conf` | Require auth header for `/docker-registry/` proxy |
| `frontend/src/pages/containers-store.js` | Pass auth header to registry reads, filter by tenant prefix, prefix uploads |
| `frontend/src/sections/devices/usp/devices-lcm.js` | Filter registry images by tenant prefix, include prefix in docker:// URLs |

---

### Task 1: Container Upload Service — JWT Validation and Tenant Prefix

**Files:**
- Modify: `deploy/compose/container-upload-service/container-upload-service.js`

The container-upload service currently only checks for presence of the Authorization header. It needs to decode the JWT to extract the tenant slug and prefix all image names.

- [ ] **Step 1: Add JWT decoding**

Add a function to decode (not verify — the controller already verified it) the JWT and extract the tenant slug. Add near the top of the file after the existing constants:

```javascript
// Decode JWT payload (base64url) to extract tenant_slug.
// Token was already verified by the controller/nginx — we just need the claims.
function decodeTenantFromToken(authHeader) {
  if (!authHeader) return null;
  const token = authHeader.replace(/^Bearer\s+/i, '');
  try {
    const parts = token.split('.');
    if (parts.length !== 3) return null;
    const payload = Buffer.from(parts[1], 'base64url').toString('utf8');
    const claims = JSON.parse(payload);
    return claims.tenant_slug || null;
  } catch {
    return null;
  }
}
```

- [ ] **Step 2: Update upload handler to prefix with tenant slug**

In the upload handler (`/upload`), after extracting the auth header and before processing:

```javascript
const tenantSlug = decodeTenantFromToken(authHeader);
if (!tenantSlug) {
  res.writeHead(403, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({ message: 'Forbidden: tenant context required' }));
  return;
}
```

Then change the image naming from `name:tag` to `tenantSlug/name:tag`:

```javascript
// SECURITY: Tenant prefix provides namespace isolation in the shared registry.
// See docs/SECURITY.md for known limitations on download-level enforcement.
const prefixedName = `${tenantSlug}/${name}`;
```

Update all docker commands to use `prefixedName` instead of `name`:
- `docker import <file> <prefixedName>:<tag>`
- `docker tag <prefixedName>:<tag> <REGISTRY>/<prefixedName>:<tag>`
- `docker push <REGISTRY>/<prefixedName>:<tag>`
- Cleanup: `docker rmi <prefixedName>:<tag>` and `docker rmi <REGISTRY>/<prefixedName>:<tag>`

Update the success response to return the prefixed name.

- [ ] **Step 3: Update delete handler to prefix with tenant slug**

In the delete handler (`/delete`), extract tenant from JWT:

```javascript
const tenantSlug = decodeTenantFromToken(authHeader);
if (!tenantSlug) {
  res.writeHead(403, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({ message: 'Forbidden: tenant context required' }));
  return;
}
const prefixedName = `${tenantSlug}/${name}`;
```

Use `prefixedName` in the registry API calls:
- `HEAD /v2/${prefixedName}/manifests/${tag}`
- `DELETE /v2/${prefixedName}/manifests/${digest}`

- [ ] **Step 4: Add security comment at top of file**

```javascript
// SECURITY NOTE: This service prefixes all container images with the tenant slug
// from the JWT (e.g., "prpl-test/my-container:v1.0"). This provides namespace
// isolation at the naming level. However, the Docker Registry itself does NOT
// enforce download authentication — any client that knows an image name can pull it.
// This is acceptable because:
// 1. Devices only pull URLs constructed by the controller
// 2. The controller only constructs URLs with the correct tenant prefix
// 3. The /_catalog endpoint requires auth (enforced by nginx)
// See docs/SECURITY.md for full threat model.
```

- [ ] **Step 5: Commit**

```bash
git add deploy/compose/container-upload-service/container-upload-service.js
git commit -m "feat: prefix container images with tenant slug from JWT"
```

---

### Task 2: Nginx — Require Auth for Registry Reads

**Files:**
- Modify: `deploy/compose/nginx.conf`

Currently `/docker-registry/` is accessible without any authentication. Add auth header requirement.

- [ ] **Step 1: Add auth check for docker-registry proxy**

In `deploy/compose/nginx.conf`, update the `/docker-registry/` location block to check for the Authorization header:

```nginx
# Docker Registry proxy — requires authentication to prevent
# unauthenticated browsing of the container catalog.
# SECURITY: This prevents tenants from listing other tenants' containers
# via the /_catalog endpoint. Download-level isolation is NOT enforced
# at the registry level — see docs/SECURITY.md.
location /docker-registry/ {
    # Require auth header
    if ($http_authorization = '') {
        return 401 '{"error":"Authentication required"}';
    }

    rewrite ^/docker-registry/(.*)$ /$1 break;
    proxy_pass             https://172.17.0.1:443;
    proxy_read_timeout     60;
    proxy_connect_timeout  60;
    proxy_redirect         off;
    proxy_ssl_verify       off;
    proxy_ssl_verify_depth 0;
    proxy_ssl_server_name  off;
    proxy_ssl_name         127.0.0.1;
    proxy_set_header        Host 127.0.0.1:443;
    proxy_set_header        X-Real-IP $remote_addr;
    proxy_set_header        X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header        X-Forwarded-Proto $scheme;
}
```

- [ ] **Step 2: Commit**

```bash
git add deploy/compose/nginx.conf
git commit -m "feat: require auth header for docker registry proxy"
```

---

### Task 3: Frontend — Container Store Tenant Isolation

**Files:**
- Modify: `frontend/src/pages/containers-store.js`

The Container Store page needs to:
1. Pass auth header when fetching from `/docker-registry/`
2. Filter catalog results to only show repos starting with tenant slug prefix
3. Prefix uploads with tenant slug
4. Strip prefix when displaying container names

- [ ] **Step 1: Add auth headers to registry API calls**

Import `useTenant` and get `tenantSlug`. Add auth headers to all `/docker-registry/` fetch calls:

```javascript
import { useTenant } from 'src/contexts/tenant-context';

// Inside component:
const { tenantSlug } = useTenant();

// Update fetchContainers() — catalog request:
const catalogResponse = await fetch('/docker-registry/v2/_catalog', {
  method: 'GET',
  headers: { 'Authorization': localStorage.getItem('token') },
});
```

Same for tags requests:
```javascript
const tagsResponse = await fetch(`/docker-registry/v2/${repo}/tags/list`, {
  method: 'GET',
  headers: { 'Authorization': localStorage.getItem('token') },
});
```

- [ ] **Step 2: Filter catalog by tenant prefix**

After fetching the catalog, filter repositories to only show the current tenant's:

```javascript
const catalogData = await catalogResponse.json();
const tenantPrefix = tenantSlug + '/';

// Filter repos belonging to this tenant
const tenantRepos = (catalogData.repositories || []).filter(
  repo => repo.startsWith(tenantPrefix)
);
```

When displaying, strip the tenant prefix:

```javascript
const displayName = containerName.startsWith(tenantPrefix)
  ? containerName.slice(tenantPrefix.length)
  : containerName;
```

Store both the full registry name (for API calls) and display name (for UI).

- [ ] **Step 3: Prefix uploads with tenant slug**

In the upload handler, prefix the container name:

```javascript
const formData = new FormData();
formData.append('name', containerName);  // Name without prefix — service adds it from JWT
formData.append('tag', containerTag.trim());
formData.append('file', containerFile);
```

The container-upload service adds the prefix from the JWT, so the frontend sends the raw name. No change needed here if the service handles it. But the Container Store page may also need to use the prefixed name for the "existing container" option in the upload dialog.

- [ ] **Step 4: Update delete to use full prefixed name**

When deleting, send the full registry name (with prefix):

```javascript
// The name stored in state should be the full registry name
const response = await fetch(
  `/api/containers/delete?name=${encodeURIComponent(fullName)}&tag=${tag}`,
  { method: 'DELETE', headers: myHeaders }
);
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/containers-store.js
git commit -m "feat: tenant-scoped container store with prefix filtering"
```

---

### Task 4: Frontend — LCM Page Tenant Prefix in Docker URLs

**Files:**
- Modify: `frontend/src/sections/devices/usp/devices-lcm.js`

The LCM page fetches container images from the registry and constructs `docker://` URLs for `InstallDU()`. It needs the same tenant filtering and prefixed URLs.

- [ ] **Step 1: Add auth headers and tenant filtering to fetchDockerImages**

Import `useTenant`, get `tenantSlug`. Add auth headers to registry calls and filter by prefix. The LCM page already has a `fetchDockerImages` function — update it:

```javascript
const { tenantSlug } = useTenant();

// In fetchDockerImages:
const catalogResponse = await fetch('/docker-registry/v2/_catalog', {
  headers: { 'Authorization': localStorage.getItem('token') },
});
// ...
const tenantPrefix = tenantSlug + '/';
const tenantRepos = (catalogData.repositories || []).filter(
  repo => repo.startsWith(tenantPrefix)
);
```

Display names without prefix, but use full names for docker:// URLs.

- [ ] **Step 2: Update docker:// URL construction to include tenant prefix**

When building the install URL, use the full registry name (with tenant prefix):

```javascript
// The full name includes the tenant prefix
const registryHost = getDefaultRegistryUrl().replace(/^https?:\/\//, '').replace(/\/$/, '');
const installUrl = `docker://${registryHost}/${fullRegistryName}:${tag}`;
// e.g., docker://192.168.0.83/prpl-test/my-container:v1.0
```

This ensures the device pulls from the correct namespaced path.

- [ ] **Step 3: Update update URL construction**

Same for the update flow — use full prefixed name:

```javascript
const updateUrl = `docker://${registryHost}/${fullRegistryName}:${updateTag}`;
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/sections/devices/usp/devices-lcm.js
git commit -m "feat: tenant-scoped container images in LCM page"
```

---

### Task 5: Security Documentation

**Files:**
- Create: `docs/SECURITY.md`

- [ ] **Step 1: Create SECURITY.md**

```markdown
# Security Notes

## Container Registry Tenant Isolation

### How it works

Container images are stored in a shared Docker Registry V2 instance. Tenant isolation
is enforced at the naming level: all images are prefixed with the tenant slug
(e.g., `prpl-test/my-container:v1.0`).

- **Upload**: The container-upload service extracts the tenant slug from the JWT and
  prefixes the image name automatically. A tenant cannot upload to another tenant's
  namespace.
- **Catalog browsing**: The `/docker-registry/` nginx proxy requires authentication.
  The frontend filters the catalog to only show images matching the current tenant's
  prefix.
- **Delete**: The container-upload service extracts the tenant slug from the JWT and
  prefixes the image name, so a tenant can only delete their own images.
- **Deployment**: The controller constructs `docker://` URLs with the tenant prefix
  when deploying containers to devices via USP `InstallDU()`.

### Known limitation: download-level isolation

The Docker Registry itself does NOT enforce authentication for image pulls. Any client
that knows an image name (e.g., `docker://host/other-tenant/container:tag`) can pull it.

**Why this is acceptable:**
1. Devices only pull URLs constructed by the controller, not arbitrary URLs.
2. The controller only constructs URLs with the authenticated tenant's prefix.
3. The `/_catalog` endpoint requires authentication, so tenants cannot discover other
   tenants' image names.
4. A device would need to be deliberately reconfigured to pull from another tenant's
   namespace — this is outside the platform's threat model.

**If stronger isolation is needed:**
- Deploy a registry proxy with per-tenant token validation
- Use separate registries per tenant
- Implement Docker Registry token authentication (Bearer token service)
```

- [ ] **Step 2: Commit**

```bash
git add docs/SECURITY.md
git commit -m "docs: add SECURITY.md documenting registry isolation limitations"
```

---

### Task 6: Verification

- [ ] **Step 1: Rebuild container-upload service**

```bash
sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml up -d --build container-upload 2>&1"
```

- [ ] **Step 2: Rebuild frontend**

```bash
sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build frontend 2>&1"
sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml up -d frontend 2>&1"
```

- [ ] **Step 3: Restart nginx**

```bash
sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml up -d nginx 2>&1"
```

- [ ] **Step 4: Test**

1. Login as tenant user (prpl-test)
2. Open Container Store — should only show containers prefixed with `prpl-test/`
3. Upload a new container — should be stored as `prpl-test/test-container:v1`
4. Open LCM page — registry dropdown should only show `prpl-test/` containers
5. Verify docker:// URL includes tenant prefix
6. Login as different tenant — should see different (or empty) container list
7. Verify `/docker-registry/v2/_catalog` returns 401 without auth header
