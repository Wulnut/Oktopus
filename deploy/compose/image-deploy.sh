#!/bin/bash
# Build, verify, export, and deploy individual Oktopus Docker images.
#
# Build machine examples:
#   ./image-deploy.sh pipeline controller
#   ./image-deploy.sh pipeline acs adapter frontend
#   ./image-deploy.sh scp controller acs          # PROD_HOST=prod PROD_DIR=~/compose
#
# Production machine examples:
#   ./image-deploy.sh load controller
#   ./image-deploy.sh restart controller
#   ./image-deploy.sh deploy controller             # load + restart
#
# Environment variables:
#   PROD_HOST       scp destination host       (default: prod)
#   PROD_DIR        remote/local compose dir   (default: ~/compose)
#   DOCKER_USER     image namespace            (default: oktopusp)
#   OUTPUT_DIR      where *.tar files are written (default: script directory)
#   COMPOSE_DIR     docker compose directory on prod (default: script directory)
#   GIT_PULL=0      skip git pull during build
#   SKIP_VERIFY=1   skip post-build grep checks

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPOSE_DIR="${COMPOSE_DIR:-$SCRIPT_DIR}"

DOCKER_USER="${DOCKER_USER:-oktopusp}"
PROD_HOST="${PROD_HOST:-prod}"
PROD_DIR="${PROD_DIR:-~/compose}"
OUTPUT_DIR="${OUTPUT_DIR:-$SCRIPT_DIR}"
GIT_PULL="${GIT_PULL:-1}"
SKIP_VERIFY="${SKIP_VERIFY:-0}"

# Services built via deploy/compose/build.sh (docker-compose.dev.yaml)
COMPOSE_SERVICES=(
  controller frontend adapter
  mqtt mqtt-adapter ws ws-adapter stomp stomp-adapter
)

# Services built via backend Makefile (not in compose dev overlay)
declare -A MAKEFILE_BUILD_DIRS=(
  [acs]="backend/services/acs/build"
  [socketio]="backend/services/utils/socketio/build"
  [file-server]="backend/services/utils/file-server/build"
  [firmware-upload]="backend/services/utils/firmware-upload/build"
)

declare -A IMAGE_NAMES=(
  [controller]="controller"
  [frontend]="frontend"
  [adapter]="adapter"
  [mqtt]="mqtt"
  [mqtt-adapter]="mqtt-adapter"
  [ws]="ws"
  [ws-adapter]="ws-adapter"
  [stomp]="stomp"
  [stomp-adapter]="stomp-adapter"
  [acs]="acs"
  [socketio]="socketio"
  [file-server]="file-server"
  [firmware-upload]="firmware-upload"
)

declare -A COMPOSE_SERVICE_NAMES=(
  [controller]="controller"
  [frontend]="frontend"
  [adapter]="adapter"
  [mqtt]="mqtt"
  [mqtt-adapter]="mqtt-adapter"
  [ws]="ws"
  [ws-adapter]="ws-adapter"
  [stomp]="stomp"
  [stomp-adapter]="stomp-adapter"
  [acs]="acs"
  [socketio]="socketio"
  [file-server]="file-server"
  [firmware-upload]="firmware-upload"
)

ALL_SERVICES=(
  controller acs adapter frontend
  mqtt mqtt-adapter ws ws-adapter stomp stomp-adapter
  socketio file-server firmware-upload
)

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

image_ref() {
  local svc="$1"
  local name="${IMAGE_NAMES[$svc]:-$svc}"
  printf '%s/%s:latest' "$DOCKER_USER" "$name"
}

tar_path() {
  local svc="$1"
  printf '%s/%s.tar' "$OUTPUT_DIR" "$svc"
}

is_known_service() {
  local svc="$1"
  local known
  for known in "${ALL_SERVICES[@]}"; do
    [[ "$known" == "$svc" ]] && return 0
  done
  return 1
}

uses_compose_build() {
  local svc="$1"
  local item
  for item in "${COMPOSE_SERVICES[@]}"; do
    [[ "$item" == "$svc" ]] && return 0
  done
  return 1
}

git_short_hash() {
  git -C "$REPO_ROOT" log --format='%h' -n 1
}

compose_profiles() {
  printf '%s' 'nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,portainer,registry'
}

compose_built_ref() {
  local svc="$1"
  local built
  built=$(
    cd "$COMPOSE_DIR"
    COMPOSE_PROFILES="$(compose_profiles)" \
      docker compose -f docker-compose.yaml -f docker-compose.dev.yaml \
      images --format '{{.Repository}}:{{.Tag}}' "$svc" 2>/dev/null | head -1
  )
  [[ -n "$built" && "$built" != "<none>:<none>" ]] && printf '%s' "$built"
}

compose_prod() {
  (
    cd "$COMPOSE_DIR"
    COMPOSE_PROFILES="$(compose_profiles)" \
      docker compose -f docker-compose.yaml "$@"
  )
}

image_base_name() {
  local ref="$1"
  ref="${ref%%:*}"
  printf '%s' "$ref"
}

compose_service_image_name() {
  local svc="$1"
  compose_prod config --format json 2>/dev/null \
    | python3 -c "import json,sys; data=json.load(sys.stdin); print(data.get('services', {}).get('${svc}', {}).get('image', ''))" 2>/dev/null
}

assert_compose_image_name() {
  local svc="$1"
  local expected actual
  expected="$(image_base_name "$(image_ref "$svc")")"
  actual="$(compose_service_image_name "$svc")"
  if [[ -z "$actual" ]]; then
    die "${svc}: docker-compose.yaml has no image: field. Sync compose config before deploy."
  fi
  if [[ "$(image_base_name "$actual")" != "$expected" ]]; then
    die "${svc}: compose image is '${actual}', expected '${expected}'"
  fi
}

running_container_image_id() {
  local container="$1"
  docker inspect "$container" --format '{{.Image}}' 2>/dev/null
}

show_service_status() {
  local svc="$1"
  local container="${COMPOSE_SERVICE_NAMES[$svc]:-$svc}"
  local expected_ref expected_id running_id compose_image

  expected_ref="$(image_ref "$svc")"
  compose_image="$(compose_service_image_name "$svc")"
  expected_id="$(docker image inspect "$expected_ref" --format '{{.Id}}' 2>/dev/null || true)"
  running_id="$(running_container_image_id "$container")"

  printf 'service:     %s\n' "$svc"
  printf 'container:   %s\n' "$container"
  printf 'compose:     %s\n' "${compose_image:-<missing image:>}"
  printf 'loaded:      %s\n' "${expected_ref}${expected_id:+ }${expected_id}"
  if [[ -n "$running_id" ]]; then
    printf 'running:     %s\n' "$running_id"
    if [[ -n "$expected_id" && "$running_id" == "$expected_id" ]]; then
      printf 'status:      OK (container matches loaded image)\n'
    else
      printf 'status:      MISMATCH (container is not using loaded image)\n'
    fi
  else
    printf 'running:     <container not found>\n'
    printf 'status:      NOT RUNNING\n'
  fi
  printf '\n'
}

ensure_latest_tag() {
  local svc="$1"
  local ref hash_ref name built_ref legacy_ref
  ref="$(image_ref "$svc")"
  name="${IMAGE_NAMES[$svc]:-$svc}"

  if docker image inspect "$ref" >/dev/null 2>&1; then
    return 0
  fi

  hash_ref="${DOCKER_USER}/${name}:$(git_short_hash)"
  if docker image inspect "$hash_ref" >/dev/null 2>&1; then
    log "Tagging ${hash_ref} -> ${ref}"
    docker tag "$hash_ref" "$ref"
    return 0
  fi

  if uses_compose_build "$svc"; then
    built_ref="$(compose_built_ref "$svc")"
    if [[ -n "$built_ref" ]] && docker image inspect "$built_ref" >/dev/null 2>&1; then
      log "Tagging ${built_ref} -> ${ref}"
      docker tag "$built_ref" "$ref"
      return 0
    fi

    # Older compose files without image: produced compose-<service>
    legacy_ref="compose-${svc}:latest"
    if docker image inspect "$legacy_ref" >/dev/null 2>&1; then
      log "Tagging ${legacy_ref} -> ${ref}"
      docker tag "$legacy_ref" "$ref"
      return 0
    fi
  fi

  die "Image not found: ${ref} (also tried ${hash_ref}, compose-${svc}:latest)"
}

service_build() {
  local svc="$1"

  if uses_compose_build "$svc"; then
    log "Building ${svc} via compose"
    (
      cd "$COMPOSE_DIR"
      COMPOSE_PROFILES="$(compose_profiles)" \
        docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build "$svc"
    )
  elif [[ -n "${MAKEFILE_BUILD_DIRS[$svc]+x}" ]]; then
    log "Building ${svc} via Makefile"
    make build -C "${REPO_ROOT}/${MAKEFILE_BUILD_DIRS[$svc]}"
    ensure_latest_tag "$svc"
  else
    die "Unknown service: ${svc}"
  fi

  ensure_latest_tag "$svc"
  log "Built $(image_ref "$svc")"
}

verify_service() {
  local svc="$1"
  local ref cmd_ok=0

  ensure_latest_tag "$svc"
  ref="$(image_ref "$svc")"

  case "$svc" in
    controller)
      docker run --rm --entrypoint sh "$ref" -c \
        'grep -aq "devices.retrieve" /controller && grep -aq "campaign_scheduled" /controller && echo OK || echo FAIL'
      ;;
    acs)
      docker run --rm --entrypoint sh "$ref" -c \
        'grep -aq "cwmp-conn-rq-" /acs && echo OK || echo FAIL'
      ;;
    adapter)
      docker run --rm --entrypoint sh "$ref" -c \
        'grep -aq "Malformed CWMP event subject received" /adapter && echo OK || echo FAIL'
      ;;
    frontend)
      docker run --rm --entrypoint sh "$ref" -c \
        'grep -rq "Read-Only" /app/.next && echo OK || echo FAIL'
      ;;
    mqtt|mqtt-adapter|ws|ws-adapter|stomp|stomp-adapter|socketio|file-server|firmware-upload|acs)
      case "$svc" in
        mqtt) docker run --rm --entrypoint sh "$ref" -c 'test -x /mqtt && echo OK || echo FAIL' ;;
        mqtt-adapter) docker run --rm --entrypoint sh "$ref" -c 'test -x /mqtt-adapter && echo OK || echo FAIL' ;;
        ws) docker run --rm --entrypoint sh "$ref" -c 'test -x /ws && echo OK || echo FAIL' ;;
        ws-adapter) docker run --rm --entrypoint sh "$ref" -c 'test -x /ws-adapter && echo OK || echo FAIL' ;;
        stomp) docker run --rm --entrypoint sh "$ref" -c 'test -x /stomp && echo OK || echo FAIL' ;;
        stomp-adapter) docker run --rm --entrypoint sh "$ref" -c 'test -x /stomp-adapter && echo OK || echo FAIL' ;;
        socketio) docker run --rm --entrypoint sh "$ref" -c 'test -f /app/server.js && echo OK || echo FAIL' ;;
        file-server) docker run --rm --entrypoint sh "$ref" -c 'test -x /file-server && echo OK || echo FAIL' ;;
        firmware-upload) docker run --rm --entrypoint sh "$ref" -c 'test -f /app/entrypoint.sh && echo OK || echo FAIL' ;;
      esac
      ;;
    *)
      die "No verify rule for service: ${svc}"
      ;;
  esac

  cmd_ok=$?
  if [[ $cmd_ok -ne 0 ]]; then
    die "Verify command failed for ${svc}"
  fi

  log "Verify OK: ${svc}"
}

save_service() {
  local svc="$1"
  local ref out
  ref="$(image_ref "$svc")"
  out="$(tar_path "$svc")"

  ensure_latest_tag "$svc"
  mkdir -p "$OUTPUT_DIR"
  log "Saving ${ref} -> ${out}"
  docker save "$ref" -o "$out"
}

scp_service() {
  local svc="$1"
  local out
  out="$(tar_path "$svc")"
  [[ -f "$out" ]] || die "Missing tar file: ${out} (run save first)"

  log "Copying ${out} -> ${PROD_HOST}:${PROD_DIR}/"
  scp "$out" "${PROD_HOST}:${PROD_DIR}/"
}

load_service() {
  local svc="$1"
  local out
  out="$(tar_path "$svc")"
  [[ -f "$out" ]] || die "Missing tar file: ${out}"

  log "Loading ${out}"
  docker load -i "$out"
}

restart_service() {
  local svc="$1"
  local compose_name="${COMPOSE_SERVICE_NAMES[$svc]:-$svc}"
  local expected_ref expected_id running_id

  expected_ref="$(image_ref "$svc")"
  assert_compose_image_name "$svc"

  log "Restarting compose service: ${compose_name} (image: ${expected_ref})"
  compose_prod stop "$compose_name" 2>/dev/null || true
  compose_prod rm -f "$compose_name" 2>/dev/null || true
  compose_prod up -d --no-build --force-recreate --pull never "$compose_name"

  expected_id="$(docker image inspect "$expected_ref" --format '{{.Id}}')"
  running_id="$(running_container_image_id "$compose_name")"
  if [[ -z "$running_id" ]]; then
    die "Container ${compose_name} failed to start"
  fi
  if [[ "$running_id" != "$expected_id" ]]; then
    die "Container ${compose_name} uses ${running_id}, expected ${expected_id}. Check docker-compose.yaml image: and loaded tar."
  fi

  log "Recent logs for ${compose_name}:"
  docker logs "$compose_name" 2>&1 | head -25 || true
  log "Deploy OK: ${compose_name} is running ${expected_ref}"
}

maybe_git_pull() {
  if [[ "$GIT_PULL" == "1" ]]; then
    log "git pull in ${REPO_ROOT}"
    git -C "$REPO_ROOT" pull
  else
    log "Skipping git pull (GIT_PULL=0)"
  fi
}

usage() {
  cat <<EOF
Usage: $(basename "$0") <command> [service ...]

Commands:
  list                 List supported services
  build   <svc...>     Build one or more images (tags :latest)
  verify  <svc...>     Grep/smoke-check built images
  save    <svc...>     docker save to OUTPUT_DIR/<svc>.tar
  scp     <svc...>     scp tar files to PROD_HOST:PROD_DIR
  load    <svc...>     docker load from tar (run on production)
  restart <svc...>     docker compose stop/rm/up -d (run on production)
  deploy  <svc...>     load + restart (run on production)
  status  <svc...>     show loaded vs running image IDs
  pipeline <svc...>    git pull + build + verify + save

Services:
  $(printf '%s\n' "${ALL_SERVICES[@]}" | paste -sd' ' -)

Examples:
  ./image-deploy.sh pipeline controller
  ./image-deploy.sh pipeline acs adapter frontend
  PROD_HOST=prod PROD_DIR=~/compose ./image-deploy.sh scp controller acs
  COMPOSE_DIR=~/compose OUTPUT_DIR=~/compose ./image-deploy.sh deploy controller

Notes:
  - acs, socketio, file-server, firmware-upload use Makefile build (not build.sh)
  - Set SKIP_VERIFY=1 to skip grep checks
  - Set GIT_PULL=0 to skip git pull in pipeline/build
EOF
}

resolve_services() {
  if [[ $# -eq 0 ]]; then
    die "No services specified. Run '$(basename "$0") list' for options."
  fi

  local svc
  for svc in "$@"; do
    is_known_service "$svc" || die "Unknown service: ${svc}"
  done
}

cmd="${1:-}"
shift || true

case "$cmd" in
  list)
    printf '%s\n' "${ALL_SERVICES[@]}"
    ;;
  build)
    resolve_services "$@"
    maybe_git_pull
    for svc in "$@"; do
      service_build "$svc"
      if [[ "$SKIP_VERIFY" != "1" ]]; then
        verify_service "$svc"
      fi
    done
    ;;
  verify)
    resolve_services "$@"
    for svc in "$@"; do
      verify_service "$svc"
    done
    ;;
  save)
    resolve_services "$@"
    for svc in "$@"; do
      save_service "$svc"
    done
    ;;
  scp)
    resolve_services "$@"
    for svc in "$@"; do
      scp_service "$svc"
    done
    ;;
  load)
    resolve_services "$@"
    for svc in "$@"; do
      load_service "$svc"
    done
    ;;
  restart)
    resolve_services "$@"
    for svc in "$@"; do
      restart_service "$svc"
    done
    ;;
  deploy)
    resolve_services "$@"
    for svc in "$@"; do
      load_service "$svc"
      restart_service "$svc"
    done
    ;;
  status)
    resolve_services "$@"
    for svc in "$@"; do
      show_service_status "$svc"
    done
    ;;
  pipeline)
    resolve_services "$@"
    maybe_git_pull
    for svc in "$@"; do
      service_build "$svc"
      if [[ "$SKIP_VERIFY" != "1" ]]; then
        verify_service "$svc"
      fi
      save_service "$svc"
    done
    log "Done. Next: PROD_HOST=${PROD_HOST} PROD_DIR=${PROD_DIR} $0 scp $*"
    ;;
  help|-h|--help|"")
    usage
    ;;
  *)
    die "Unknown command: ${cmd}. Run '$(basename "$0") help'."
    ;;
esac
