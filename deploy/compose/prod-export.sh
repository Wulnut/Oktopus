#!/bin/bash
# Export Oktopus images and config for offline/air-gapped production deployment.
#
# Run on a machine with access to the GitLab Container Registry (e.g. CI runner,
# dev server, or local machine after `docker login`).
#
# Produces: oktopus-prod.tar.gz
#   - images.tar   — all custom Oktopus images (docker save)
#   - compose files, nats_config, env templates, helper scripts
#
# Usage:
#   ./prod-export.sh --local                            # export local oktopusp/*:latest
#   ./prod-export.sh <registry_image> [tag]             # pull from registry, then export
#   ./prod-export.sh <registry_image> [tag] --upload-gcs <bucket>
#
# Examples:
#   ./prod-export.sh --local
#   ./prod-export.sh gitlab.rrioo.com:5050/.../oktopus abc1234
#   ./prod-export.sh gitlab.rrioo.com:5050/.../oktopus latest --upload-gcs oktopus-deploy
#   ./prod-build-export.sh                            # build from source + --local export

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

OUTPUT_NAME="${OUTPUT_NAME:-oktopus-prod}"
STAGE_DIR="${SCRIPT_DIR}/${OUTPUT_NAME}"
ARCHIVE="${SCRIPT_DIR}/${OUTPUT_NAME}.tar.gz"

# Custom images built and shipped via registry (base images like mongo/nats/nginx
# are pulled by Docker Hub on the production server).
SERVICES=(
  controller frontend adapter
  mqtt mqtt-adapter ws ws-adapter stomp stomp-adapter
  acs socketio file-server firmware-upload
)

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

REGISTRY_IMAGE=""
TAG="latest"
UPLOAD_GCS=""
LOCAL_ONLY=0
POSITIONAL=()

while [ $# -gt 0 ]; do
    case "$1" in
        --local)
            LOCAL_ONLY=1
            shift
            ;;
        --upload-gcs)
            shift
            [ $# -ge 1 ] || die "--upload-gcs requires bucket name"
            UPLOAD_GCS="$1"
            shift
            ;;
        -*)
            die "Unknown option: $1"
            ;;
        *)
            POSITIONAL+=("$1")
            shift
            ;;
    esac
done

if [ "${#POSITIONAL[@]}" -ge 1 ]; then
    REGISTRY_IMAGE="${POSITIONAL[0]}"
fi
if [ "${#POSITIONAL[@]}" -ge 2 ]; then
    TAG="${POSITIONAL[1]}"
fi
if [ "${#POSITIONAL[@]}" -gt 2 ]; then
    die "Unexpected arguments: ${POSITIONAL[*]}"
fi

if [ "$LOCAL_ONLY" -eq 0 ]; then
    if [ -z "$REGISTRY_IMAGE" ]; then
        REGISTRY_IMAGE="${CI_REGISTRY_IMAGE:-}"
    fi
    [ -z "$REGISTRY_IMAGE" ] && die "Usage: $0 --local
       $0 <registry_image> [tag] [--upload-gcs <bucket>]
Set CI_REGISTRY_IMAGE or pass --local to export oktopusp/*:latest from this machine."

    REGISTRY_HOST="${REGISTRY_IMAGE%%/*}"
    log "Registry: ${REGISTRY_IMAGE}"
    log "Tag:      ${TAG}"
else
    log "Mode:     local (oktopusp/*:latest on this machine)"
fi

# ---------------------------------------------------------------------------
# Registry login (if credentials are available)
# ---------------------------------------------------------------------------
if [ "$LOCAL_ONLY" -eq 0 ] && [ -n "${CI_REGISTRY_PASSWORD:-${CI_JOB_TOKEN:-}}" ]; then
    log "Docker login to ${REGISTRY_HOST}"
    echo "${CI_REGISTRY_PASSWORD:-${CI_JOB_TOKEN}}" | \
      docker login "$REGISTRY_HOST" \
        -u "${CI_REGISTRY_USER:-gitlab-ci-token}" \
        --password-stdin
fi

# ---------------------------------------------------------------------------
# Pull/retag or use local images, then stage
# ---------------------------------------------------------------------------
rm -rf "$STAGE_DIR"
mkdir -p "$STAGE_DIR"

IMAGE_REFS=()
for svc in "${SERVICES[@]}"; do
    dst="oktopusp/${svc}:latest"
    if [ "$LOCAL_ONLY" -eq 1 ]; then
        if ! docker image inspect "$dst" &>/dev/null; then
            die "Missing local image ${dst}. Run ./prod-build-export.sh or ./build.sh first."
        fi
        log "Using local ${svc}"
    else
        src="${REGISTRY_IMAGE}/${svc}:${TAG}"
        log "Pulling ${svc}"
        docker pull "$src"
        docker tag "$src" "$dst"
    fi
    IMAGE_REFS+=("$dst")
done

log "Saving ${#IMAGE_REFS[@]} images to images.tar"
docker save -o "$STAGE_DIR/images.tar" "${IMAGE_REFS[@]}"

# ---------------------------------------------------------------------------
# Copy deployment files
# ---------------------------------------------------------------------------
log "Copying compose and config files"
cp docker-compose.yaml "$STAGE_DIR/"
cp docker-compose.prod.yaml "$STAGE_DIR/"
cp generate-secrets.sh "$STAGE_DIR/"
cp nginx.conf "$STAGE_DIR/"
cp prod-init.sh prod-deploy.sh "$STAGE_DIR/"
cp -r nats_config "$STAGE_DIR/"

# Env templates (reference for generate-secrets.sh, not directly consumed)
cp .env.*.example "$STAGE_DIR/" 2>/dev/null || log "WARN: no .env.*.example files found"

# Services with build: context in compose (built locally on production server)
[ -d container-upload-service ] && cp -r container-upload-service "$STAGE_DIR/"
[ -d registry-certs-generator ] && cp -r registry-certs-generator "$STAGE_DIR/"

# Logo images referenced by nginx
[ -d images ] && cp -r images "$STAGE_DIR/"

# ---------------------------------------------------------------------------
# Create archive
# ---------------------------------------------------------------------------
log "Creating ${OUTPUT_NAME}.tar.gz"
tar czf "$ARCHIVE" -C "$SCRIPT_DIR" "$OUTPUT_NAME"
rm -rf "$STAGE_DIR"

ARCHIVE_SIZE=$(du -h "$ARCHIVE" | cut -f1)
log "Archive: ${ARCHIVE} (${ARCHIVE_SIZE})"

# ---------------------------------------------------------------------------
# Optional: upload to Google Cloud Storage
# ---------------------------------------------------------------------------
if [ -n "$UPLOAD_GCS" ]; then
    if command -v gsutil &>/dev/null; then
        log "Uploading to gs://${UPLOAD_GCS}/"
        gsutil cp "$ARCHIVE" "gs://${UPLOAD_GCS}/"
        log "Upload complete: gs://${UPLOAD_GCS}/$(basename "$ARCHIVE")"
    else
        die "gsutil not found. Install Google Cloud SDK or upload manually."
    fi
fi

log "Done. Deploy with: prod-deploy.sh ${ARCHIVE}"
