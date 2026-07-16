#!/usr/bin/env bash
# seed-test-tenant.sh — create telkomsel-test tenant on GCP and load fixtures.
#
# Used after sanitize.py has produced fixtures/, to populate a real (isolated)
# tenant that POST/PUT/DELETE captures can run against without polluting
# production tenant data.
set -euo pipefail

: "${GCP_SSH_HOST:?GCP_SSH_HOST is required}"
: "${GCP_SSH_PORT:=22}"
: "${GCP_OKTOPUS_DIR:=/home/sei/oktopus}"
: "${COMPOSE_DIR:=${GCP_OKTOPUS_DIR}/deploy/compose}"
: "${MONGO_CONTAINER:=mongo_usp}"
: "${TEST_TENANT:=telkomsel-test}"
: "${SUPERADMIN_EMAIL:?SUPERADMIN_EMAIL is required to create the tenant}"
: "${SUPERADMIN_PASSWORD:?SUPERADMIN_PASSWORD is required}"

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
FIXTURES_DIR="${REPO_ROOT}/fixtures"

if [[ ! -d "$FIXTURES_DIR" ]]; then
  echo "ERROR: $FIXTURES_DIR does not exist; run sanitize.py first." >&2
  exit 1
fi

echo "==> Creating ${TEST_TENANT} on GCP via controller REST API"

TOKEN=$(ssh -p "$GCP_SSH_PORT" "$GCP_SSH_HOST" \
  "curl -fsS -X PUT http://localhost/api/auth/login \
     -H 'Content-Type: application/json' \
     -d '{\"email\":\"${SUPERADMIN_EMAIL}\",\"password\":\"${SUPERADMIN_PASSWORD}\"}'" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin).get("token",""))')

if [[ -z "$TOKEN" ]]; then
  echo "ERROR: superadmin login failed" >&2
  exit 1
fi

# Idempotent: try to create; if it already exists the controller returns an error
# we ignore.
ssh -p "$GCP_SSH_PORT" "$GCP_SSH_HOST" \
  "curl -fsS -X POST http://localhost/api/tenants \
     -H 'Authorization: ${TOKEN}' \
     -H 'Content-Type: application/json' \
     -d '{\"slug\":\"${TEST_TENANT}\",\"name\":\"Telkomsel Test (sanitized fixtures)\"}'" \
  || echo "(tenant may already exist; continuing)"

echo "==> Provisioning tenant_${TEST_TENANT}_general via controller"
# ProvisionTenantDBs runs as a side effect of tenant creation, but if the tenant
# already existed we may need to re-create DBs. Easiest: drop and re-provision
# by hitting a tenant-scoped route (TenantMiddleware tolerates a missing DB
# only for read routes, so we ensure the DB exists by mongorestore'ing fixtures).

echo "==> Copying fixtures to GCP"
REMOTE_TMP="/tmp/fixtures-${TEST_TENANT}"
ssh -p "$GCP_SSH_PORT" "$GCP_SSH_HOST" "rm -rf ${REMOTE_TMP} && mkdir -p ${REMOTE_TMP}"
scp -P "$GCP_SSH_PORT" -r "$FIXTURES_DIR"/*.json \
  "${GCP_SSH_HOST}:${REMOTE_TMP}/" || true

echo "==> Restoring sanitized fixtures into tenant_${TEST_TENANT}_general"
for f in "$FIXTURES_DIR"/*.json; do
  base=$(basename "$f" .json)
  # Map fixture filename to Mongo collection name (mirror sanitize.py FIXTURE_FILES).
  case "$base" in
    devices)           coll=devices ;;
    firmware)          coll=firmware ;;
    scripts)           coll=scripts ;;
    campaigns)         coll=campaigns ;;
    lock_policies)     coll=device_lock_policy ;;
    lock_config)       coll=device_lock_config ;;
    lock_unauthorized) coll=device_lock_unauthorized ;;
    upgrade_logs)      coll=upgrade_logs ;;
    device_info)       coll=device_info ;;
    ca_certs)          coll=ca_certs ;;
    users)             coll=users ;;
    tenants)           echo "  (skipping tenants — created via API)"; continue ;;
    *)                 echo "  (skipping unknown fixture $base)"; continue ;;
  esac
  echo "  -> tenant_${TEST_TENANT}_general.${coll}"
  # mongoimport into the running container, mapping JSON array to docs.
  ssh -p "$GCP_SSH_PORT" "$GCP_SSH_HOST" \
    "docker exec -i ${MONGO_CONTAINER} mongoimport \
       --db tenant_${TEST_TENANT}_general \
       --collection ${coll} \
       --drop --jsonArray --quiet" \
    < "$f" \
    || echo "    (warn: ${coll} import failed)"
done

echo
echo "==> ${TEST_TENANT} seeded."
echo "==> Use this tenant for any further POST/PUT/DELETE HTTP captures;"
echo "    the production ${SOURCE_TENANT:-telkomsel} tenant is untouched."
