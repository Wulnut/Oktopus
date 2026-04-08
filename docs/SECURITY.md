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
   namespace -- this is outside the platform's threat model.

### Known limitation: registry exposure

The Docker Registry is exposed on the host network. Any tenant with network access
could potentially push images directly to the registry, bypassing the tenant prefix
enforcement in the container-upload service. This is mitigated by:
1. The upload service being the only intended upload path
2. The frontend only displaying images matching the tenant prefix
3. Future work will add registry-level authentication

**If stronger isolation is needed:**
- Deploy a registry proxy with per-tenant token validation
- Use separate registries per tenant
- Implement Docker Registry token authentication (Bearer token service)
