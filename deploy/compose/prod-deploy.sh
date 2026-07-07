#!/bin/bash
# Deploy Oktopus on a production server from an exported tarball.
#
# Run on the production server (e.g. GCP VM). Loads pre-built images and
# starts the full compose stack. Does NOT compile anything.
#
# Usage:
#   ./prod-deploy.sh oktopus-prod.tar.gz                    # from local file
#   ./prod-deploy.sh --from-gcs gs://bucket/file.tar.gz     # download from GCS
#
# Environment:
#   DEPLOY_DIR      deployment directory (default: ~/oktopus)
#   PROD_PROFILES   compose profiles (default: all except portainer)

set -euo pipefail

DEPLOY_DIR="${DEPLOY_DIR:-$HOME/oktopus}"
DEFAULT_PROFILES="nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,registry"
PROD_PROFILES="${PROD_PROFILES:-$DEFAULT_PROFILES}"

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

ARCHIVE=""
GCS_URI=""

# Parse arguments
if [ $# -eq 0 ]; then
    die "Usage: $0 <tarball> | --from-gcs gs://bucket/file.tar.gz"
fi

if [ "$1" = "--from-gcs" ]; then
    [ $# -lt 2 ] && die "Missing GCS URI after --from-gcs"
    GCS_URI="$2"
else
    ARCHIVE="$1"
fi

# ---------------------------------------------------------------------------
# Obtain the tarball
# ---------------------------------------------------------------------------
if [ -n "$GCS_URI" ]; then
    ARCHIVE="/tmp/$(basename "$GCS_URI")"
    log "Downloading from ${GCS_URI}"
    if command -v gsutil &>/dev/null; then
        gsutil cp "$GCS_URI" "$ARCHIVE"
    else
        die "gsutil not found. Install Google Cloud SDK or download manually."
    fi
fi

[ -f "$ARCHIVE" ] || die "File not found: ${ARCHIVE}"

# ---------------------------------------------------------------------------
# Extract to deployment directory
# ---------------------------------------------------------------------------
log "Extracting to ${DEPLOY_DIR}"
mkdir -p "$DEPLOY_DIR"

# Tarball contains a top-level directory (oktopus-prod/); extract and flatten
TMP_EXTRACT="$(mktemp -d)"
tar xzf "$ARCHIVE" -C "$TMP_EXTRACT"

# Find the top-level directory and move contents
TOP_DIR=$(find "$TMP_EXTRACT" -mindepth 1 -maxdepth 1 -type d | head -1)
[ -z "$TOP_DIR" ] && die "Tarball structure unexpected — no top-level directory found"

# Sync all files including dotfiles (overwrite existing, keep data dirs)
cp -rf "$TOP_DIR"/. "$DEPLOY_DIR/"
rm -rf "$TMP_EXTRACT"

# Create data directories if they don't exist
mkdir -p "$DEPLOY_DIR"/{mongo_data,nats_data,portainer_data,firmwares}

# ---------------------------------------------------------------------------
# Load Docker images
# ---------------------------------------------------------------------------
log "Loading images from images.tar"
docker load -i "$DEPLOY_DIR/images.tar"

# ---------------------------------------------------------------------------
# Generate secrets on first deploy
# ---------------------------------------------------------------------------
cd "$DEPLOY_DIR"
if [ ! -f .env.nats ] || grep -q '__PLACEHOLDER__' .env.nats 2>/dev/null; then
    log "Generating secrets (first-time setup)"
    chmod +x generate-secrets.sh

    # Ensure .env.*.example templates exist for generate-secrets.sh
    for tmpl in .env.*.example; do
        [ -f "$tmpl" ] || continue
        target="${tmpl%.example}"
        [ -f "$target" ] || cp "$tmpl" "$target"
    done

    ./generate-secrets.sh
else
    log "Secrets already generated, skipping"
fi

# ---------------------------------------------------------------------------
# Build local infrastructure images (have build: context in compose)
# ---------------------------------------------------------------------------
log "Building local infrastructure images"
COMPOSE_PROFILES="$PROD_PROFILES" \
    docker compose -f docker-compose.yaml build container-upload registry-certs-generator \
    || log "WARN: local infra build skipped (may be cached)"

# ---------------------------------------------------------------------------
# Start the stack
# ---------------------------------------------------------------------------
log "Starting services (profiles: ${PROD_PROFILES})"
COMPOSE_PROFILES="$PROD_PROFILES" \
    docker compose -f docker-compose.yaml -f docker-compose.prod.yaml \
    up -d --no-build --force-recreate

echo ""
log "Service status:"
COMPOSE_PROFILES="$PROD_PROFILES" \
    docker compose -f docker-compose.yaml -f docker-compose.prod.yaml \
    ps --format 'table {{.Name}}\t{{.Status}}' 2>/dev/null || true

echo ""
log "Memory usage:"
free -h

echo ""
log "Deploy complete."
echo "Web UI: http://$(curl -s --connect-timeout 3 ifconfig.me 2>/dev/null || echo 'localhost')"
