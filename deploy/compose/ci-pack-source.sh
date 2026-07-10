#!/bin/bash
# CI pack: create a source tarball for remote build-and-deploy.
# Run on the CI runner (or locally). Preserves repo layout required by
# docker-compose.dev.yaml build contexts (backend/, frontend/, deploy/compose/).
#
# Output: /tmp/oktopus-src.tgz (override with OUTPUT=...)
#
# Excludes server-side data, secrets, and build artifacts so extraction on the
# deploy host does not overwrite mongo_data, .env.*, firmwares, etc.
#
# Also stamps frontend/public/version.json before packing (pure shell — the
# GitLab deploy job image is debian without node). Remote builds have no .git.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUTPUT="${OUTPUT:-/tmp/oktopus-src.tgz}"

log() { printf '==> %s\n' "$*"; }

cd "$REPO_ROOT"

# Stamp frontend/public/version.json before packing so Docker builds without .git
# still get the correct commit (CI_COMMIT_SHORT_SHA or local git).
stamp_version() {
  local commit="${OKTOPUS_GIT_COMMIT:-${CI_COMMIT_SHORT_SHA:-}}"
  if [ -z "$commit" ] && command -v git >/dev/null 2>&1 && [ -d "$REPO_ROOT/.git" ]; then
    commit="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || true)"
  fi
  if [ -z "$commit" ]; then
    log "WARN: no git commit available for version.json stamp"
    return 0
  fi
  export OKTOPUS_GIT_COMMIT="$commit"

  local version="${OKTOPUS_VERSION:-}"
  if [ -z "$version" ] && [ -n "${CI_COMMIT_TAG:-}" ]; then
    version="${CI_COMMIT_TAG#v}"
  fi
  if [ -z "$version" ] && [ -f "$REPO_ROOT/VERSION" ]; then
    version="$(tr -d '[:space:]' < "$REPO_ROOT/VERSION")"
  fi
  if [ -z "$version" ] && [ -f "$REPO_ROOT/frontend/VERSION" ]; then
    version="$(tr -d '[:space:]' < "$REPO_ROOT/frontend/VERSION")"
  fi
  if [ -z "$version" ]; then
    version="0.0.0-dev"
  fi
  export OKTOPUS_VERSION="$version"

  local built_at="${OKTOPUS_BUILT_AT:-}"
  if [ -z "$built_at" ]; then
    built_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  fi

  mkdir -p "$REPO_ROOT/frontend/public"
  # Pure shell write — deploy job image (debian:bookworm-slim) has no node.
  cat > "$REPO_ROOT/frontend/public/version.json" <<EOF
{
  "version": "${version}",
  "commit": "${commit}",
  "built_at": "${built_at}",
  "label": "v${version} (${commit})"
}
EOF
  log "Stamped version.json: v${version} (${commit})"
}
stamp_version

log "Packing source from ${REPO_ROOT} -> ${OUTPUT}"

tar czf "$OUTPUT" \
  --exclude='.git' \
  --exclude='.codegraph' \
  --exclude='node_modules' \
  --exclude='.next' \
  --exclude='test-reports' \
  --exclude='*.tar.gz' \
  --exclude='images.tar' \
  --exclude='deploy/compose/mongo_data' \
  --exclude='deploy/compose/nats_data' \
  --exclude='deploy/compose/portainer_data' \
  --exclude='deploy/compose/firmwares' \
  --exclude='deploy/compose/images' \
  --exclude='deploy/compose/.env.controller' \
  --exclude='deploy/compose/.env.adapter' \
  --exclude='deploy/compose/.env.nats' \
  --exclude='deploy/compose/.env.mqtt' \
  --exclude='deploy/compose/.env.mqtt-adapter' \
  --exclude='deploy/compose/.env.ws' \
  --exclude='deploy/compose/.env.ws-adapter' \
  --exclude='deploy/compose/.env.stomp' \
  --exclude='deploy/compose/.env.stomp-adapter' \
  --exclude='deploy/compose/.env.socketio' \
  --exclude='deploy/compose/.env.acs' \
  --exclude='deploy/compose/.env.file-server' \
  --exclude='deploy/compose/.env.firmware-upload' \
  .

SIZE="$(du -h "$OUTPUT" | cut -f1)"
log "Done: ${OUTPUT} (${SIZE})"
