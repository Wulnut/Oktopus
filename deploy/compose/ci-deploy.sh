#!/bin/bash
# CI deploy: pull images from GitLab Container Registry, retag to oktopusp/*,
# and restart the full compose stack.
# Run on the deploy server (192.168.0.106). Does NOT compile anything.
#
# Usage:  ci-deploy.sh <registry_image> <tag>
#   registry_image — e.g. gitlab.rrioo.com:5050/sei-robotics/router/oktopus
#   tag            — e.g. CI_COMMIT_SHORT_SHA
#
# Optional environment (passed from CI via SSH):
#   CI_REGISTRY          — registry host
#   CI_REGISTRY_USER     — registry username
#   CI_REGISTRY_PASSWORD — registry password / job token

set -euo pipefail

REGISTRY_IMAGE="${1:?Usage: ci-deploy.sh <registry_image> <tag>}"
TAG="${2:?Usage: ci-deploy.sh <registry_image> <tag>}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REGISTRY_HOST="${REGISTRY_IMAGE%%/*}"

SERVICES=(controller frontend adapter mqtt mqtt-adapter ws ws-adapter stomp stomp-adapter acs socketio file-server firmware-upload)
COMPOSE_PROFILES_VAL="nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,portainer,registry"

log() { printf '==> %s\n' "$*"; }

# ---------------------------------------------------------------------------
# Bootstrap: generate .env secrets on first deploy (idempotent)
# ---------------------------------------------------------------------------
if [ ! -f "$SCRIPT_DIR/.env.nats" ]; then
  log "First-time setup: generating secrets"
  chmod +x "$SCRIPT_DIR/generate-secrets.sh" 2>/dev/null || true
  "$SCRIPT_DIR/generate-secrets.sh"
fi

# ---------------------------------------------------------------------------
# Registry login (skip if no credentials provided — assume pre-authenticated)
# ---------------------------------------------------------------------------
if [ -n "${CI_REGISTRY_PASSWORD:-}" ]; then
  log "Docker login to ${REGISTRY_HOST}"
  echo "${CI_REGISTRY_PASSWORD}" | docker login "${REGISTRY_HOST}" \
    -u "${CI_REGISTRY_USER:-gitlab-ci-token}" --password-stdin
else
  log "No CI_REGISTRY_PASSWORD — assuming pre-authenticated"
fi

# ---------------------------------------------------------------------------
# Pull each image and retag to the local name docker-compose.yaml expects
# ---------------------------------------------------------------------------
for svc in "${SERVICES[@]}"; do
  src="${REGISTRY_IMAGE}/${svc}:${TAG}"
  dst="oktopusp/${svc}:latest"
  log "Pulling $svc"
  docker pull "$src"
  docker tag "$src" "$dst"
done

# ---------------------------------------------------------------------------
# Build small local-only infrastructure services (not shipped via registry)
# ---------------------------------------------------------------------------
log "Building local infrastructure services"
cd "$SCRIPT_DIR"
COMPOSE_PROFILES="$COMPOSE_PROFILES_VAL" \
  docker compose -f docker-compose.yaml build container-upload registry-certs-generator \
  || log "WARN: local infra build skipped (may be cached)"

# ---------------------------------------------------------------------------
# Restart everything
# ---------------------------------------------------------------------------
log "Restarting all services"
COMPOSE_PROFILES="$COMPOSE_PROFILES_VAL" \
  docker compose -f docker-compose.yaml up -d --force-recreate --no-build

log "Deploy complete. Service status:"
COMPOSE_PROFILES="$COMPOSE_PROFILES_VAL" \
  docker compose -f docker-compose.yaml ps --format 'table {{.Name}}\t{{.Status}}' 2>/dev/null || true
