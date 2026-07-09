#!/bin/bash
# One-time migration: flatten ~/oktopus (prod-deploy layout) -> standard repo layout.
#
# Run on the GCP production server BEFORE the first ci-source-deploy.
# Moves compose config, data dirs, and secrets into deploy/compose/ while
# keeping the repo root ready for CI source sync (backend/, frontend/, etc.).
#
# Usage:
#   cd ~/oktopus && ./prod-migrate-layout.sh
#   cd ~/oktopus && ./prod-migrate-layout.sh /path/to/oktopus
#
# Safe to re-run: skips items already under deploy/compose/.

set -euo pipefail

OKTOPUS_ROOT="${1:-$HOME/oktopus}"
COMPOSE_DIR="${OKTOPUS_ROOT}/deploy/compose"

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

[ -d "$OKTOPUS_ROOT" ] || die "Directory not found: ${OKTOPUS_ROOT}"

if [ -f "${COMPOSE_DIR}/docker-compose.yaml" ] && [ -d "${COMPOSE_DIR}/mongo_data" ]; then
  log "Already migrated (${COMPOSE_DIR} has docker-compose.yaml and mongo_data)"
  exit 0
fi

log "Migrating ${OKTOPUS_ROOT} to standard repo layout"
mkdir -p "$COMPOSE_DIR"

move_if_exists() {
  local name="$1"
  local src="${OKTOPUS_ROOT}/${name}"
  local dst="${COMPOSE_DIR}/${name}"
  if [ -e "$src" ] && [ ! -e "$dst" ]; then
    log "Moving ${name} -> deploy/compose/"
    mv "$src" "$dst"
  fi
}

# Compose and config
for item in \
  docker-compose.yaml \
  docker-compose.prod.yaml \
  nginx.conf \
  generate-secrets.sh \
  nats_config \
  registry-certs-generator \
  container-upload-service \
  service.sh \
  prod-deploy.sh \
  prod-init.sh \
  prod-build-export.sh \
  prod-export.sh \
  prod-migrate-layout.sh \
  ci-deploy.sh \
  ci-build.sh \
  ci-source-deploy.sh \
  ci-pack-source.sh \
  build.sh \
  run.sh \
  stop.sh \
  package.sh \
  image-deploy.sh
do
  src="${OKTOPUS_ROOT}/${item}"
  [ -e "$src" ] || continue
  dst="${COMPOSE_DIR}/${item}"
  if [ ! -e "$dst" ]; then
    log "Moving ${item} -> deploy/compose/"
    mv "$src" "$dst"
  fi
done

for src in "${OKTOPUS_ROOT}"/docker-compose.yaml.bak.*; do
  [ -e "$src" ] || continue
  base="$(basename "$src")"
  dst="${COMPOSE_DIR}/${base}"
  if [ ! -e "$dst" ]; then
    log "Moving ${base} -> deploy/compose/"
    mv "$src" "$dst"
  fi
done

# Data directories and secrets
for item in mongo_data nats_data portainer_data firmwares images; do
  move_if_exists "$item"
done

for envfile in "${OKTOPUS_ROOT}"/.env.*; do
  [ -f "$envfile" ] || continue
  base="$(basename "$envfile")"
  if [[ "$base" == *.example ]]; then
    continue
  fi
  dst="${COMPOSE_DIR}/${base}"
  if [ ! -f "$dst" ]; then
    log "Moving ${base} -> deploy/compose/"
    mv "$envfile" "$dst"
  fi
done

# Copy env templates if missing
for tmpl in "${COMPOSE_DIR}"/.env.*.example; do
  [ -f "$tmpl" ] || continue
  target="${tmpl%.example}"
  [ -f "$target" ] || cp "$tmpl" "$target"
done

log "Migration complete."
echo ""
echo "Next steps:"
echo "  1. CI will sync full source to ${OKTOPUS_ROOT} (backend/, frontend/, deploy/compose/)"
echo "  2. Run deploy from: cd ${COMPOSE_DIR} && ./ci-source-deploy.sh prod"
echo "  3. Verify: cd ${COMPOSE_DIR} && docker compose ps"
