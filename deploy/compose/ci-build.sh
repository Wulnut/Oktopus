#!/bin/bash
# CI build: build all Oktopus images, tag for GitLab Container Registry, push.
# Run on the CI runner (192.168.0.83). Uses the host Docker daemon via socket bind.
#
# Required CI variables (predefined by GitLab):
#   CI_REGISTRY          — registry host, e.g. gitlab.rrioo.com:5050
#   CI_REGISTRY_IMAGE    — full project path, e.g. gitlab.rrioo.com:5050/.../oktopus
#   CI_REGISTRY_USER     — registry username
#   CI_REGISTRY_PASSWORD — registry password / job token
#   CI_COMMIT_SHORT_SHA  — git short hash

set -euo pipefail

REGISTRY="${CI_REGISTRY_IMAGE:?CI_REGISTRY_IMAGE not set}"
TAG="${CI_COMMIT_SHORT_SHA:?CI_COMMIT_SHORT_SHA not set}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Services built via docker compose (build: defined in compose files)
COMPOSE_SERVICES=(controller frontend adapter mqtt mqtt-adapter ws ws-adapter stomp stomp-adapter)

# Services built via individual Makefiles: name:build-dir
MAKEFILE_SERVICES=(
  "acs:backend/services/acs/build"
  "socketio:backend/services/utils/socketio/build"
  "file-server:backend/services/utils/file-server/build"
  "firmware-upload:backend/services/utils/firmware-upload/build"
)

ALL_SERVICES=("${COMPOSE_SERVICES[@]}" acs socketio file-server firmware-upload)
COMPOSE_PROFILES_VAL="nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,portainer,registry"

log() { printf '==> %s\n' "$*"; }

# ---------------------------------------------------------------------------
# Registry login
# ---------------------------------------------------------------------------
log "Docker login to ${CI_REGISTRY}"
echo "${CI_REGISTRY_PASSWORD}" | docker login "${CI_REGISTRY}" \
  -u "${CI_REGISTRY_USER}" --password-stdin

# ---------------------------------------------------------------------------
# Build compose services (uses docker-compose.dev.yaml build contexts)
# ---------------------------------------------------------------------------
log "Building compose services: ${COMPOSE_SERVICES[*]}"
cd "$SCRIPT_DIR"
COMPOSE_PROFILES="$COMPOSE_PROFILES_VAL" \
  docker compose -f docker-compose.yaml -f docker-compose.dev.yaml \
  build "${COMPOSE_SERVICES[@]}"

# ---------------------------------------------------------------------------
# Build Makefile services
# ---------------------------------------------------------------------------
for entry in "${MAKEFILE_SERVICES[@]}"; do
  svc="${entry%%:*}"
  dir="${entry#*:}"
  log "Building $svc via Makefile"
  make build -C "$REPO_ROOT/$dir" DOCKER_TAG=latest
done

# ---------------------------------------------------------------------------
# Tag and push every image to registry (both :<sha> and :latest)
# ---------------------------------------------------------------------------
for svc in "${ALL_SERVICES[@]}"; do
  local_ref="oktopusp/${svc}:latest"
  sha_ref="${REGISTRY}/${svc}:${TAG}"
  latest_ref="${REGISTRY}/${svc}:latest"

  log "Pushing $svc ($TAG + latest)"
  docker tag "$local_ref" "$sha_ref"
  docker tag "$local_ref" "$latest_ref"
  docker push "$sha_ref"
  docker push "$latest_ref"
done

log "Done: built and pushed ${#ALL_SERVICES[@]} images"
