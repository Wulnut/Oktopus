#!/bin/bash
# CI source deploy: build all Oktopus images from source on the target server, then restart.
# Run on the deploy server after ci-pack-source.sh tarball is extracted.
# After a successful build and restart, removes backend/, frontend/, etc.
# (keeps deploy/compose/ only). Set SKIP_SOURCE_CLEANUP=1 to retain sources.
#
# Usage:  ci-source-deploy.sh [staging|prod]
#   staging — full profiles including portainer (test server)
#   prod      — production overlay, no portainer (GCP VM)

set -euo pipefail

ENV="${1:-staging}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

COMPOSE_SERVICES=(controller frontend adapter mqtt mqtt-adapter ws ws-adapter stomp stomp-adapter)
MAKEFILE_SERVICES=(
  "acs:backend/services/acs/build"
  "socketio:backend/services/utils/socketio/build"
  "file-server:backend/services/utils/file-server/build"
  "firmware-upload:backend/services/utils/firmware-upload/build"
)

STAGING_PROFILES="nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,portainer,registry"
PROD_PROFILES="nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,registry"

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

# Non-root deploy users (GCP sei) often have root-owned ~/.docker/buildx after sudo docker.
# Use an isolated config under /tmp to avoid "buildx/.lock: permission denied".
setup_docker_cli() {
  if [ "$(id -u)" -eq 0 ]; then
    return 0
  fi
  export DOCKER_CONFIG="/tmp/oktopus-docker-$(id -un)-$(id -u)"
  mkdir -p "$DOCKER_CONFIG/buildx"
  log "Non-root deploy: DOCKER_CONFIG=${DOCKER_CONFIG}"
}

# Compose defaults to the working directory name ("compose" under deploy/compose/).
# That creates compose_usp_network on 172.16.235.0/24 and fails if oktopus_usp_network
# already exists from the flat ~/oktopus prod layout.
setup_compose_project() {
  export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-oktopus}"
  log "Docker Compose project: ${COMPOSE_PROJECT_NAME}"
}

prune_orphan_compose_network() {
  local orphan="compose_usp_network"
  if ! docker network inspect "$orphan" &>/dev/null; then
    return 0
  fi
  local count
  count="$(docker network inspect "$orphan" --format '{{len .Containers}}' 2>/dev/null || echo 0)"
  if [ "$count" = "0" ]; then
    log "Removing unused ${orphan} (subnet overlaps ${COMPOSE_PROJECT_NAME}_usp_network)"
    docker network rm "$orphan" || true
  fi
}

build_makefile_service() {
  local svc="$1"
  local rel_build_dir="$2"
  local build_dir="$REPO_ROOT/$rel_build_dir"
  local context_dir
  context_dir="$(dirname "$build_dir")"
  [ -f "${build_dir}/Dockerfile" ] || die "Missing Dockerfile: ${build_dir}/Dockerfile"
  log "Building ${svc} via docker (${context_dir})"
  docker build -t "oktopusp/${svc}:latest" -f "${build_dir}/Dockerfile" "${context_dir}"
}

# Accept common aliases (GitLab environment name is "production", not "prod").
case "$ENV" in
  production) ENV=prod ;;
esac

case "$ENV" in
  staging|prod) ;;
  *) die "Usage: ci-source-deploy.sh [staging|prod] (received: '${1:-<empty>}')" ;;
esac

cd "$SCRIPT_DIR"
setup_docker_cli
setup_compose_project

# ---------------------------------------------------------------------------
# Bootstrap secrets and data dirs (idempotent; does not overwrite real secrets)
# ---------------------------------------------------------------------------
if [ ! -f "$SCRIPT_DIR/.env.nats" ]; then
  log "Generating secrets (first-time setup)"
  chmod +x "$SCRIPT_DIR/generate-secrets.sh" 2>/dev/null || true
  "$SCRIPT_DIR/generate-secrets.sh"
else
  log "Secrets already present, skipping generate-secrets.sh"
fi

mkdir -p firmwares
chown 1000:1000 firmwares 2>/dev/null || chmod 1777 firmwares

# ---------------------------------------------------------------------------
# Build compose services (docker-compose.dev.yaml provides build contexts)
# ---------------------------------------------------------------------------
log "Building compose services: ${COMPOSE_SERVICES[*]}"
COMPOSE_PROFILES="$STAGING_PROFILES" \
  docker compose -f docker-compose.yaml -f docker-compose.dev.yaml \
  build "${COMPOSE_SERVICES[@]}"

# ---------------------------------------------------------------------------
# Build Makefile-backed services (acs, socketio, file-server, firmware-upload)
# ---------------------------------------------------------------------------
for entry in "${MAKEFILE_SERVICES[@]}"; do
  svc="${entry%%:*}"
  dir="${entry#*:}"
  build_makefile_service "$svc" "$dir"
done

# ---------------------------------------------------------------------------
# Build local-only infrastructure images
# ---------------------------------------------------------------------------
log "Building local infrastructure services"
COMPOSE_PROFILES="$STAGING_PROFILES" \
  docker compose -f docker-compose.yaml build container-upload registry-certs-generator \
  || log "WARN: local infra build skipped (may be cached)"

# ---------------------------------------------------------------------------
# Restart stack (--no-build: use images built above)
# ---------------------------------------------------------------------------
prune_orphan_compose_network

if [ "$ENV" = "prod" ]; then
  log "Starting production stack (profiles: ${PROD_PROFILES})"
  COMPOSE_PROFILES="$PROD_PROFILES" \
    docker compose -f docker-compose.yaml -f docker-compose.prod.yaml \
    up -d --no-build --force-recreate --remove-orphans
  PS_FILES="-f docker-compose.yaml -f docker-compose.prod.yaml"
  PS_PROFILES="$PROD_PROFILES"
else
  log "Starting staging stack (profiles: ${STAGING_PROFILES})"
  COMPOSE_PROFILES="$STAGING_PROFILES" \
    docker compose -f docker-compose.yaml -f docker-compose.dev.yaml \
    up -d --no-build --force-recreate --remove-orphans
  PS_FILES="-f docker-compose.yaml -f docker-compose.dev.yaml"
  PS_PROFILES="$STAGING_PROFILES"
fi

log "Deploy complete (${ENV}). Service status:"
COMPOSE_PROFILES="$PS_PROFILES" \
  docker compose $PS_FILES ps --format 'table {{.Name}}\t{{.Status}}' 2>/dev/null || true

# ---------------------------------------------------------------------------
# Remove source tree after successful build+deploy (runtime uses images only).
# Next CI run re-uploads a fresh tarball. Set SKIP_SOURCE_CLEANUP=1 to keep sources.
# ---------------------------------------------------------------------------
cleanup_deploy_source() {
  if [ "${SKIP_SOURCE_CLEANUP:-0}" = "1" ]; then
    log "SKIP_SOURCE_CLEANUP=1 — keeping source tree"
    return 0
  fi

  log "Removing source tree under ${REPO_ROOT} (keeping deploy/compose only)"

  for name in backend frontend build agent docs; do
    if [ -d "$REPO_ROOT/$name" ]; then
      rm -rf "$REPO_ROOT/$name"
      log "Removed ${name}/"
    fi
  done

  if [ -d "$REPO_ROOT/deploy" ]; then
    for sub in "$REPO_ROOT/deploy"/*; do
      [ -e "$sub" ] || continue
      if [ "$(basename "$sub")" = "compose" ]; then
        continue
      fi
      rm -rf "$sub"
      log "Removed deploy/$(basename "$sub")/"
    done
  fi

  while IFS= read -r -d '' f; do
    rm -f "$f"
    log "Removed $(basename "$f")"
  done < <(find "$REPO_ROOT" -maxdepth 1 -type f -print0)

  for hidden in .gitlab-ci.yml .gitignore .cursor .husky .DS_Store CLAUDE.md AGENTS.md; do
    if [ -e "$REPO_ROOT/$hidden" ]; then
      rm -rf "$REPO_ROOT/$hidden"
      log "Removed ${hidden}"
    fi
  done

  log "Source cleanup done; retained ${SCRIPT_DIR}"
}

cleanup_deploy_source
