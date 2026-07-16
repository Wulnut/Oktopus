#!/usr/bin/env bash
# capture.sh — one-shot raw data capture from a live GCP Oktopus tenant.
#
# Pulls:
#   1. mongodump of selected collections in tenant_<SOURCE_TENANT>_general
#   2. curl of representative GET endpoints on the controller REST API
#
# Output: ./raw/ subdir (mongodump archives + raw HTTP JSON responses).
# This directory is gitignored; sanitize.py turns these into commit-safe fixtures.
#
# See README.md for prerequisites and usage.
set -euo pipefail

: "${GCP_SSH_HOST:?GCP_SSH_HOST is required, e.g. sei@1.2.3.4}"
: "${GCP_OKTOPUS_DIR:=/home/sei/oktopus}"
: "${GCP_SSH_PORT:=22}"
: "${SOURCE_TENANT:=telkomsel}"
: "${MONGO_CONTAINER:=mongo_usp}"
: "${COMPOSE_DIR:=${GCP_OKTOPUS_DIR}/deploy/compose}"

RAW_DIR="$(cd "$(dirname "$0")" && pwd)/raw"
mkdir -p "$RAW_DIR"

echo "==> Capturing from tenant=${SOURCE_TENANT} on ${GCP_SSH_HOST}:${GCP_OKTOPUS_DIR}"

# Collections to dump from tenant_<slug>_general. Names match CLAUDE.md.
COLLECTIONS=(
  devices
  firmware
  scripts
  campaigns
  device_lock_policy
  device_lock_config
  device_lock_unauthorized
  upgrade_logs
  device_info
)

# Also dump the shared account-mngr tenants/users collections (filtered).
SHARED_COLLECTIONS=(
  "account-mngr:tenants:{\"slug\":\"${SOURCE_TENANT}\"}"
  "account-mngr:ca_certs:{}"
)

echo "==> Dumping Mongo collections via mongodump on ${MONGO_CONTAINER}"
for coll in "${COLLECTIONS[@]}"; do
  echo "    - tenant_${SOURCE_TENANT}_general.${coll}"
  ssh -p "$GCP_SSH_PORT" "$GCP_SSH_HOST" \
    "cd ${COMPOSE_DIR} && \
     docker exec ${MONGO_CONTAINER} mongodump \
       --db tenant_${SOURCE_TENANT}_general \
       --collection ${coll} \
       --archive --quiet" \
    > "$RAW_DIR/${coll}.archive" || echo "    (warn: ${coll} dump failed; skipping)"
done

for entry in "${SHARED_COLLECTIONS[@]}"; do
  IFS=':' read -r db coll filter <<<"$entry"
  echo "    - ${db}.${coll} (filter=${filter})"
  ssh -p "$GCP_SSH_PORT" "$GCP_SSH_HOST" \
    "cd ${COMPOSE_DIR} && \
     docker exec ${MONGO_CONTAINER} mongodump \
       --db ${db} \
       --collection ${coll} \
       --query '${filter}' \
       --archive --quiet" \
    > "$RAW_DIR/shared_${coll}.archive" || echo "    (warn: shared ${coll} dump failed; skipping)"
done

echo "==> Capturing HTTP response snapshots"
# Pull representative GET responses. Requires a valid TenantAdmin login on the
# source tenant. We do NOT bake credentials into this script — pass via env.
: "${TENANT_USER:?TENANT_USER (email) is required for HTTP capture}"
: "${TENANT_PASSWORD:?TENANT_PASSWORD is required for HTTP capture}"

TOKEN=$(ssh -p "$GCP_SSH_PORT" "$GCP_SSH_HOST" \
  "curl -fsS -X PUT http://localhost/api/auth/login \
     -H 'Content-Type: application/json' \
     -d '{\"email\":\"${TENANT_USER}\",\"password\":\"${TENANT_PASSWORD}\"}'" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin).get("token",""))')

if [[ -z "$TOKEN" ]]; then
  echo "ERROR: login failed or no token in response" >&2
  exit 1
fi

HTTP_ROUTES=(
  "device"
  "device/filterOptions"
  "info/vendors"
  "info/status"
  "info/device_class"
  "info/general"
  "firmware"
  "scripts"
  "campaigns"
  "lock/policies"
  "lock/config"
  "lock/unauthorized"
  "lock/unsupported"
  "lock/commands"
  "lock/audit"
  "mass-actions"
  "ca-certs"
  "users"
)

for route in "${HTTP_ROUTES[@]}"; do
  fname=$(echo "$route" | tr / _)
  echo "    - GET /api/tenants/${SOURCE_TENANT}/${route}"
  ssh -p "$GCP_SSH_PORT" "$GCP_SSH_HOST" \
    "curl -fsS -H 'Authorization: ${TOKEN}' \
       'http://localhost/api/tenants/${SOURCE_TENANT}/${route}?page_number=0&page_size=20'" \
    > "$RAW_DIR/http_${fname}.json" || echo "    (warn: ${route} fetch failed; skipping)"
done

echo
echo "==> Raw capture complete in: ${RAW_DIR}"
echo "==> Next: ./sanitize.py --in ${RAW_DIR} --out <repo>/fixtures --tenant telkomsel-test"
