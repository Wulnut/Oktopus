#!/bin/bash
# Build Oktopus images locally and export oktopus-prod.tar.gz for offline GCP deploy.
#
# Run on a build machine with Docker (e.g. CI runner 192.168.0.83).
# Produces the same tarball as prod-export.sh, without requiring a registry pull.
#
# Usage:
#   ./prod-build-export.sh
#       Build all images from source, then docker save + tar (recommended on .83).
#
#   ./prod-build-export.sh --export-only
#       Skip build; export existing oktopusp/*:latest images only.
#
#   ./prod-build-export.sh --from-registry <registry_image> [tag]
#       Pull from GitLab Registry and export (delegates to prod-export.sh).
#
# Examples:
#   cd ~/oktopus && git pull origin telkomsel/ont-lock-dev
#   cd deploy/compose && ./prod-build-export.sh
#
#   # After CI has pushed images:
#   ./prod-build-export.sh --from-registry gitlab.rrioo.com:5050/sei-robotics/router/oktopus latest
#
# Output: deploy/compose/oktopus-prod.tar.gz
# Deploy on GCP: prod-deploy.sh oktopus-prod.tar.gz

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

EXPORT_ONLY=0
FROM_REGISTRY=0
REGISTRY_IMAGE=""
TAG="latest"

while [ $# -gt 0 ]; do
    case "$1" in
        --export-only)
            EXPORT_ONLY=1
            shift
            ;;
        --from-registry)
            FROM_REGISTRY=1
            shift
            [ $# -ge 1 ] || die "Usage: $0 --from-registry <registry_image> [tag]"
            REGISTRY_IMAGE="$1"
            shift
            if [ $# -ge 1 ] && [[ "$1" != --* ]]; then
                TAG="$1"
                shift
            fi
            ;;
        -h|--help)
            sed -n '2,26p' "$0" | sed 's/^# \?//'
            exit 0
            ;;
        *)
            die "Unknown option: $1 (try --help)"
            ;;
    esac
done

if [ "$FROM_REGISTRY" -eq 1 ]; then
    exec "$SCRIPT_DIR/prod-export.sh" "$REGISTRY_IMAGE" "$TAG" "$@"
fi

# ---------------------------------------------------------------------------
# Local build (same services as ci-build.sh, without registry push)
# ---------------------------------------------------------------------------
prod_build_local() {
    COMPOSE_SERVICES=(controller frontend adapter mqtt mqtt-adapter ws ws-adapter stomp stomp-adapter)
    MAKEFILE_SERVICES=(
        "acs:backend/services/acs/build"
        "socketio:backend/services/utils/socketio/build"
        "file-server:backend/services/utils/file-server/build"
        "firmware-upload:backend/services/utils/firmware-upload/build"
    )
    COMPOSE_PROFILES_VAL="nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,portainer,registry"

    cd "$SCRIPT_DIR"
    if [ ! -f "$SCRIPT_DIR/.env.nats" ]; then
        log "Generating placeholder .env files for compose config parsing"
        "$SCRIPT_DIR/generate-secrets.sh"
    fi

    log "Building compose services: ${COMPOSE_SERVICES[*]}"
    COMPOSE_PROFILES="$COMPOSE_PROFILES_VAL" \
        docker compose -f docker-compose.yaml -f docker-compose.dev.yaml \
        build "${COMPOSE_SERVICES[@]}"

    for entry in "${MAKEFILE_SERVICES[@]}"; do
        svc="${entry%%:*}"
        dir="${entry#*:}"
        log "Building ${svc} via Makefile"
        make build -C "$REPO_ROOT/$dir" DOCKER_TAG=latest
    done

    log "Local build complete (oktopusp/*:latest)"
}

if [ "$EXPORT_ONLY" -eq 0 ]; then
    prod_build_local
else
    log "Skipping build (--export-only)"
fi

exec "$SCRIPT_DIR/prod-export.sh" --local
