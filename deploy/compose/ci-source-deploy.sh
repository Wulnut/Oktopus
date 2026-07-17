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
. "$SCRIPT_DIR/env-migrations.sh"

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
# That creates compose_usp_network on 172.16.235.0/24, which collides with
# oktopus_usp_network (same subnet). Always pin the project name.
setup_compose_project() {
  export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-oktopus}"
  log "Docker Compose project: ${COMPOSE_PROJECT_NAME}"
}

# Tear down a prior stack that used the default project name "compose".
# An empty-network prune is not enough: staging often still has a full
# compose_* stack running, which blocks creating oktopus_usp_network.
migrate_legacy_compose_project() {
  local legacy="compose"
  local legacy_net="${legacy}_usp_network"
  local profiles="$1"

  if [ "${COMPOSE_PROJECT_NAME}" = "$legacy" ]; then
    return 0
  fi
  if ! docker network inspect "$legacy_net" &>/dev/null \
    && ! docker ps -aq --filter "label=com.docker.compose.project=${legacy}" | grep -q .; then
    return 0
  fi

  log "Migrating legacy Compose project '${legacy}' -> '${COMPOSE_PROJECT_NAME}' (frees ${legacy_net})"
  COMPOSE_PROJECT_NAME="$legacy" COMPOSE_PROFILES="$profiles" \
    docker compose -f docker-compose.yaml -f docker-compose.dev.yaml \
    down --remove-orphans || true

  if docker network inspect "$legacy_net" &>/dev/null; then
    local count
    count="$(docker network inspect "$legacy_net" --format '{{len .Containers}}' 2>/dev/null || echo 0)"
    if [ "$count" = "0" ]; then
      log "Removing leftover ${legacy_net}"
      docker network rm "$legacy_net" || true
    else
      die "${legacy_net} still has ${count} container(s); cannot create ${COMPOSE_PROJECT_NAME}_usp_network"
    fi
  fi
}

merge_missing_env_defaults() {
  local example_file="$1"
  local target_file="$2"
  local key_prefix="${3:-}"
  local line key appended=0

  [ -f "$example_file" ] || die "Missing environment template: ${example_file}"
  [ -f "$target_file" ] || die "Missing environment file: ${target_file}"

  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      ''|'#'*) continue ;;
    esac
    case "$line" in
      *=*) ;;
      *) continue ;;
    esac

    key="${line%%=*}"
    case "$key" in
      "${key_prefix}"*) ;;
      *) continue ;;
    esac

    if ! grep -q "^${key}=" "$target_file"; then
      printf '\n%s\n' "$line" >> "$target_file"
      log "Added missing ${key} default to $(basename "$target_file")"
      appended=$((appended + 1))
    fi
  done < "$example_file"

  if [ "$appended" -eq 0 ]; then
    log "Environment defaults already current: $(basename "$target_file") (${key_prefix}*)"
  fi
}

remove_legacy_path() {
  local path="$1"
  [ -e "$path" ] || return 0
  if find "$path" -user root 2>/dev/null | head -1 | grep -q .; then
    docker run --rm -v "$path:/data" alpine sh -c 'find /data -depth -delete 2>/dev/null || true'
  else
    find "$path" -depth -delete 2>/dev/null || true
  fi
}

# Flat prod layout kept data at ~/oktopus/{mongo_data,...}; compose expects ./ under deploy/compose/.
migrate_legacy_flat_layout() {
  local root="$REPO_ROOT"
  local compose="$SCRIPT_DIR"

  if [ -d "$root/mongo_data" ] && [ ! -d "$compose/mongo_data" ]; then
    log "Migrating mongo_data from repo root"
    mv "$root/mongo_data" "$compose/mongo_data"
  elif [ -d "$root/mongo_data" ] && [ -d "$compose/mongo_data" ]; then
    local root_kb compose_kb
    root_kb="$(du -sk "$root/mongo_data" | cut -f1)"
    compose_kb="$(du -sk "$compose/mongo_data" | cut -f1)"
    if [ "$root_kb" -gt "$compose_kb" ] && [ "$compose_kb" -lt 1024 ]; then
      log "Replacing compose mongo_data (${compose_kb}K) with legacy root data (${root_kb}K)"
      mv "$compose/mongo_data" "$compose/mongo_data.ci-replaced-$(date +%Y%m%d)"
      mv "$root/mongo_data" "$compose/mongo_data"
    fi
  fi

  for item in portainer_data nats_data nats_config firmwares images; do
    if [ -e "$root/$item" ] && [ ! -e "$compose/$item" ]; then
      log "Migrating ${item} from repo root"
      mv "$root/$item" "$compose/$item"
    fi
  done

  if [ -f "$root/images/logo.png" ] && [ ! -f "$compose/images/logo.png" ]; then
    log "Copying images/logo.png to deploy/compose/images/"
    mkdir -p "$compose/images"
    cp "$root/images/logo.png" "$compose/images/" 2>/dev/null || \
      docker run --rm -v "$root/images:/src" -v "$compose/images:/dst" alpine cp /src/logo.png /dst/
  fi

  for item in container-upload-service firmwares images nats_config nats_data portainer_data registry-certs-generator mongo_data; do
    if [ -e "$root/$item" ]; then
      log "Removing legacy ${item}/ from repo root"
      remove_legacy_path "$root/$item"
    fi
  done
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
migrate_legacy_flat_layout

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

# Existing deployments keep their secrets, but receive newly introduced LOCK_*
# defaults without overwriting operator-provided values.
merge_missing_env_defaults "$SCRIPT_DIR/.env.controller.example" "$SCRIPT_DIR/.env.controller" "LOCK_"
migrate_env_default_once \
  "$SCRIPT_DIR/.env.controller" \
  "$SCRIPT_DIR/.env.migrations" \
  "20260717-lock-redis-default-true" \
  "LOCK_REDIS_ENABLED" \
  "false" \
  "true"

mkdir -p firmwares
chown 1000:1000 firmwares 2>/dev/null || chmod 1777 firmwares

# ---------------------------------------------------------------------------
# Build compose services (docker-compose.dev.yaml provides build contexts)
# ---------------------------------------------------------------------------
# Prefer commit from CI / env; else read pre-stamped version.json from the pack.
if [ -z "${OKTOPUS_GIT_COMMIT:-}" ] || [ "${OKTOPUS_GIT_COMMIT}" = "unknown" ]; then
  if [ -n "${CI_COMMIT_SHORT_SHA:-}" ]; then
    export OKTOPUS_GIT_COMMIT="$CI_COMMIT_SHORT_SHA"
  elif [ -f "$REPO_ROOT/frontend/public/version.json" ]; then
    OKTOPUS_GIT_COMMIT="$(sed -n 's/.*"commit"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
      "$REPO_ROOT/frontend/public/version.json" | head -1)"
    export OKTOPUS_GIT_COMMIT
  fi
fi
# Ensure version.json exists for Docker COPY even if pack stamp was skipped.
if [ -n "${OKTOPUS_GIT_COMMIT:-}" ] && [ "${OKTOPUS_GIT_COMMIT}" != "unknown" ]; then
  ver="${OKTOPUS_VERSION:-}"
  if [ -z "$ver" ] && [ -f "$REPO_ROOT/VERSION" ]; then
    ver="$(tr -d '[:space:]' < "$REPO_ROOT/VERSION")"
  fi
  ver="${ver:-0.0.0-dev}"
  built_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  mkdir -p "$REPO_ROOT/frontend/public"
  cat > "$REPO_ROOT/frontend/public/version.json" <<EOF
{
  "version": "${ver}",
  "commit": "${OKTOPUS_GIT_COMMIT}",
  "built_at": "${built_at}",
  "label": "v${ver} (${OKTOPUS_GIT_COMMIT})"
}
EOF
  export OKTOPUS_VERSION="$ver"
  log "Frontend version stamp: OKTOPUS_GIT_COMMIT=${OKTOPUS_GIT_COMMIT} label=v${ver} (${OKTOPUS_GIT_COMMIT})"
else
  log "WARN: OKTOPUS_GIT_COMMIT unset; frontend version.json may show unknown"
fi

log "Building compose services: ${COMPOSE_SERVICES[*]}"
COMPOSE_PROFILES="$STAGING_PROFILES" \
  OKTOPUS_GIT_COMMIT="${OKTOPUS_GIT_COMMIT:-}" \
  OKTOPUS_VERSION="${OKTOPUS_VERSION:-}" \
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
  docker compose -f docker-compose.yaml build container-upload registry-certs-generator

# ---------------------------------------------------------------------------
# Restart stack (--no-build: use images built above)
# ---------------------------------------------------------------------------
if [ "$ENV" = "prod" ]; then
  migrate_legacy_compose_project "$PROD_PROFILES"
  log "Starting production stack (profiles: ${PROD_PROFILES})"
  COMPOSE_PROFILES="$PROD_PROFILES" \
    docker compose -f docker-compose.yaml -f docker-compose.prod.yaml \
    up -d --no-build --force-recreate --remove-orphans \
    --wait --wait-timeout "${DEPLOY_HEALTH_TIMEOUT_SEC:-300}"
  PS_FILES="-f docker-compose.yaml -f docker-compose.prod.yaml"
  PS_PROFILES="$PROD_PROFILES"
else
  migrate_legacy_compose_project "$STAGING_PROFILES"
  log "Starting staging stack (profiles: ${STAGING_PROFILES})"
  COMPOSE_PROFILES="$STAGING_PROFILES" \
    docker compose -f docker-compose.yaml -f docker-compose.dev.yaml \
    up -d --no-build --force-recreate --remove-orphans \
    --wait --wait-timeout "${DEPLOY_HEALTH_TIMEOUT_SEC:-300}"
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
