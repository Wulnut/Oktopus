# Image and Dependency Version Updates

Current state audit and target versions. All changes should be made in one pass.

---

## EOL and Outdated Items

### Alpine 3.14 (EOL May 2023) -- used in ALL Go service Dockerfiles

Every Go service runtime stage uses the same pinned digest:
```
alpine:3.14@sha256:0f2d5c38dd7a4f4f733e688e3a6733cb5ab1ac6e3cb4603a5dd564e5bfb80eed
```

**Files (12):**
- `backend/services/controller/build/Dockerfile`
- `backend/services/acs/build/Dockerfile`
- `backend/services/bulkdata/http/build/Dockerfile`
- `backend/services/mtp/adapter/build/Dockerfile`
- `backend/services/mtp/mqtt/build/Dockerfile`
- `backend/services/mtp/mqtt-adapter/build/Dockerfile`
- `backend/services/mtp/stomp/build/Dockerfile`
- `backend/services/mtp/stomp-adapter/build/Dockerfile`
- `backend/services/mtp/ws/build/Dockerfile`
- `backend/services/mtp/ws-adapter/build/Dockerfile`
- `backend/services/utils/file-server/build/Dockerfile`

**Update to:** `alpine:3.21` (latest stable, supported until Nov 2026)

### Node 16.20.2 (EOL Sep 2023) -- SocketIO service

**File:** `backend/services/utils/socketio/build/Dockerfile`
**Update to:** `node:22-alpine` (LTS, supported until Apr 2027)

### Node 18.18.0 (EOL Apr 2025) -- frontend, container-upload, registry-certs

**Files (4):**
- `frontend/build/Dockerfile`
- `frontend/build/Dockerfile.dev`
- `deploy/compose/container-upload-service/Dockerfile`
- `deploy/compose/registry-certs-generator/Dockerfile`

**Update to:** `node:22-alpine`

### ubuntu:lunar (EOL Jan 2024) -- agent

**File:** `agent/Dockerfile`
**Update to:** `ubuntu:24.04` (LTS, supported until Apr 2029)

### Go 1.22 and 1.21 builder images -- bulkdata, file-server, stomp

| File | Current | Update to |
|------|---------|-----------|
| `bulkdata/http/build/Dockerfile` | `golang:1.22@sha256:...` | `golang:1.23` |
| `utils/file-server/build/Dockerfile` | `golang:1.22.2@sha256:...` | `golang:1.23` |
| `mtp/stomp/build/Dockerfile` | `golang:1.22@sha256:...` | `golang:1.23` |

Also update `go.mod` to match:
- `bulkdata/http/go.mod`: `1.22.3` -> `1.23.0`
- `utils/file-server/go.mod`: `1.21.3` -> `1.23.0`
- `mtp/stomp/go.mod`: `1.15` -> `1.23.0` (very old, may need code changes)

---

## Unpinned Images in docker-compose.yaml

| Service | Current | Pin to |
|---------|---------|--------|
| msg_broker | `nats:latest` | `nats:2.10` |
| mongo_usp | `mongo` (no tag) | `mongo:7.0` |
| portainer | `portainer/portainer-ce:latest` | `portainer/portainer-ce:2.21` |
| nginx | `nginx:latest` | `nginx:1.27-alpine` |

Also pin the `oktopusp/*` images with a version tag (e.g., `oktopusp/controller:1.0.0`) or at minimum document that they are built locally by `run.sh`.

---

## Summary of Changes

| Category | Count | Files |
|----------|-------|-------|
| Alpine 3.14 -> 3.21 | 11 | All Go Dockerfiles |
| Node 16 -> 22 | 1 | socketio Dockerfile |
| Node 18 -> 22 | 4 | frontend (x2), container-upload, registry-certs |
| Ubuntu lunar -> 24.04 | 1 | agent Dockerfile |
| Go builder 1.22/1.21 -> 1.23 | 3 | bulkdata, file-server, stomp Dockerfiles |
| go.mod version bumps | 3 | bulkdata, file-server, stomp |
| docker-compose pins | 4 | nats, mongo, portainer, nginx |
| **Total** | **27 changes** | **22 files** |

---

## CI Job for Future Checks

Add to `.circleci/config.yml` as a scheduled weekly job (or a standalone script `scripts/check-versions.sh`):

```bash
#!/bin/bash
set -e

FAIL=0

# Check Alpine versions in Dockerfiles
while IFS= read -r f; do
  if grep -q 'alpine:3\.14\|alpine:3\.15\|alpine:3\.16\|alpine:3\.17' "$f"; then
    echo "WARN: $f uses EOL Alpine"
    FAIL=1
  fi
done < <(find . -name 'Dockerfile*' -not -path './.git/*')

# Check Node versions
while IFS= read -r f; do
  if grep -qE 'node:(14|16|18)\.' "$f"; then
    echo "WARN: $f uses EOL Node"
    FAIL=1
  fi
done < <(find . -name 'Dockerfile*' -not -path './.git/*')

# Check unpinned images in docker-compose
if grep -E 'image:.*:latest' deploy/compose/docker-compose.yaml; then
  echo "WARN: docker-compose has :latest images"
  FAIL=1
fi
if grep -P 'image:\s+\w+$' deploy/compose/docker-compose.yaml; then
  echo "WARN: docker-compose has untagged images"
  FAIL=1
fi

# Optional: run trivy on built images
# for img in oktopusp/controller oktopusp/mqtt oktopusp/adapter; do
#   docker run --rm aquasec/trivy image --severity HIGH,CRITICAL "$img"
# done

exit $FAIL
```

This can also be added as an infrastructure test in `deploy/tests/infra_test.go` alongside the existing test plan.
