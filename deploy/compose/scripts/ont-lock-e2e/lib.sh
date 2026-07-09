#!/usr/bin/env bash
# Shared helpers for ONT Lock E2E. Sourced by run.sh — do not execute directly.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_DIR="${COMPOSE_DIR:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
MONGO_CONTAINER="${MONGO_CONTAINER:-mongo_usp}"
CONTROLLER_CONTAINER="${CONTROLLER_CONTAINER:-controller}"
NATS_NETWORK="${NATS_NETWORK:-oktopus_usp_network}"
NATS_CERTS_DIR="${NATS_CERTS_DIR:-$COMPOSE_DIR/nats_config}"
NATS_BOX_IMAGE="${NATS_BOX_IMAGE:-natsio/nats-box:0.14.3}"
API_BASE="${API_BASE:-http://127.0.0.1}"
MTP="${MTP:-mqtt}"
TENANT="${TENANT:-telkomsel}"
GOOD_CIDR="${GOOD_CIDR:-10.172.0.0/16}"
BAD_CIDR="${BAD_CIDR:-10.0.0.0/16}"
EVAL_TIMEOUT_SEC="${EVAL_TIMEOUT_SEC:-30}"
CMD_TIMEOUT_SEC="${CMD_TIMEOUT_SEC:-30}"
POLL_INTERVAL_SEC="${POLL_INTERVAL_SEC:-1}"

GENERAL_DB="tenant_${TENANT}_general"

die() {
  echo "ERROR: $*" >&2
  return 1
}

log() {
  echo "[$(date -u +%H:%M:%S)] $*"
}

mongo_eval() {
  local js="$1"
  docker exec "$MONGO_CONTAINER" mongosh --quiet --eval "$js"
}

utc_now() {
  date -u +%Y-%m-%dT%H:%M:%SZ
}

# Sleep 1s so subsequent created_at is strictly after marker.
mark_time() {
  sleep 1
  utc_now
}

set_config() {
  local master="$1" auto="$2"
  mongo_eval "
db=db.getSiblingDB('${GENERAL_DB}');
db.device_lock_config.updateOne(
  {_id:'global'},
  {\$set:{
    master_enabled:${master},
    auto_lock_enabled:${auto},
    updated_by:'ont-lock-e2e',
    updated_at:new Date()
  }},
  {upsert:true}
);
"
}

upsert_whitelist() {
  local cidr="$1"
  local desc="${2:-ont-lock-e2e}"
  mongo_eval "
db=db.getSiblingDB('${GENERAL_DB}');
db.device_lock_policy.updateOne(
  {sn:'${SN}'},
  {\$set:{
    sn:'${SN}',
    policy_type:'WHITELIST',
    allowed_ip_range:'${cidr}',
    description:'${desc}',
    operator_id:'ont-lock-e2e',
    status:true,
    reason_code:'',
    updated_at:new Date()
  }, \$setOnInsert:{created_at:new Date()}},
  {upsert:true}
);
"
}

delete_policy() {
  mongo_eval "
db=db.getSiblingDB('${GENERAL_DB}');
db.device_lock_policy.deleteOne({sn:'${SN}'});
"
}

clear_unauthorized() {
  mongo_eval "
db=db.getSiblingDB('${GENERAL_DB}');
db.lock_unauthorized_devices.deleteOne({sn:'${SN}'});
"
}

device_snapshot() {
  mongo_eval "
printjson(db.getSiblingDB('adapter').devices.findOne(
  {sn:'${SN}'},
  {sn:1,status:1,mqtt:1,stomp:1,websockets:1,cwmp:1,tenantid:1,vendor:1,model:1}
));
"
}

device_online_json() {
  # Prefer live adapter fields when present; fall back to MQTT online defaults.
  local vendor model mqtt stomp ws cwmp status
  vendor="$(mongo_eval "var d=db.getSiblingDB('adapter').devices.findOne({sn:'${SN}'},{vendor:1}); print(d&&d.vendor?d.vendor:'ATEL');" | tr -d '\r')"
  model="$(mongo_eval "var d=db.getSiblingDB('adapter').devices.findOne({sn:'${SN}'},{model:1}); print(d&&d.model?d.model:'FH220');" | tr -d '\r')"
  status="$(mongo_eval "var d=db.getSiblingDB('adapter').devices.findOne({sn:'${SN}'},{status:1}); print(d&&d.status!=null?d.status:2);" | tr -d '\r')"
  mqtt="$(mongo_eval "var d=db.getSiblingDB('adapter').devices.findOne({sn:'${SN}'},{mqtt:1}); print(d&&d.mqtt!=null?d.mqtt:2);" | tr -d '\r')"
  stomp="$(mongo_eval "var d=db.getSiblingDB('adapter').devices.findOne({sn:'${SN}'},{stomp:1}); print(d&&d.stomp!=null?d.stomp:0);" | tr -d '\r')"
  ws="$(mongo_eval "var d=db.getSiblingDB('adapter').devices.findOne({sn:'${SN}'},{websockets:1}); print(d&&d.websockets!=null?d.websockets:0);" | tr -d '\r')"
  cwmp="$(mongo_eval "var d=db.getSiblingDB('adapter').devices.findOne({sn:'${SN}'},{cwmp:1}); print(d&&d.cwmp!=null?d.cwmp:0);" | tr -d '\r')"
  printf '{"SN":"%s","Status":%s,"Mqtt":%s,"Stomp":%s,"Websockets":%s,"Cwmp":%s,"TenantID":"%s","Vendor":"%s","Model":"%s"}' \
    "$SN" "$status" "$mqtt" "$stomp" "$ws" "$cwmp" "$TENANT" "$vendor" "$model"
}

publish_online() {
  local payload
  payload="$(device_online_json)"
  local nats_url
  nats_url="$(docker exec "$CONTROLLER_CONTAINER" printenv NATS_URL | sed 's#nats://#tls://#')"
  docker run --rm --network "$NATS_NETWORK" \
    -v "${NATS_CERTS_DIR}:/certs:ro" \
    "$NATS_BOX_IMAGE" \
    nats --tlsca=/certs/rootCA.pem --tlscert=/certs/cert.pem --tlskey=/certs/key.pem \
    -s "$nats_url" \
    pub "device.v1.${TENANT}.online" "$payload" >/dev/null
  log "published device.v1.${TENANT}.online for ${SN}"
}

wait_evaluate_after() {
  local marker="$1"
  local deadline=$((SECONDS + EVAL_TIMEOUT_SEC))
  local out=""
  while (( SECONDS < deadline )); do
    out="$(mongo_eval "
db=db.getSiblingDB('${GENERAL_DB}');
var m=ISODate('${marker}');
var a=db.lock_audit_logs.find({sn:'${SN}', action:'evaluate', created_at:{\$gte:m}}).sort({created_at:-1}).limit(1).toArray();
if(a.length){ print(a[0].status+'|'+((a[0].details&&a[0].details.reason)||'')+'|'+(a[0].created_at.toISOString())); }
")"
    if [[ -n "${out// }" ]]; then
      echo "$out"
      return 0
    fi
    sleep "$POLL_INTERVAL_SEC"
  done
  die "timeout waiting for evaluate after ${marker}"
}

wait_command_after() {
  local marker="$1"
  local deadline=$((SECONDS + CMD_TIMEOUT_SEC))
  local out=""
  while (( SECONDS < deadline )); do
    out="$(mongo_eval "
db=db.getSiblingDB('${GENERAL_DB}');
var m=ISODate('${marker}');
var a=db.lock_command_attempts.find({device_sn:'${SN}', created_at:{\$gte:m}}).sort({created_at:-1}).limit(1).toArray();
if(a.length){ print(a[0].command_value+'|'+a[0].status+'|'+(a[0].created_at.toISOString())); }
")"
    if [[ -n "${out// }" ]]; then
      echo "$out"
      return 0
    fi
    sleep "$POLL_INTERVAL_SEC"
  done
  die "timeout waiting for command after ${marker}"
}

count_commands_after() {
  local marker="$1"
  mongo_eval "
db=db.getSiblingDB('${GENERAL_DB}');
print(db.lock_command_attempts.countDocuments({device_sn:'${SN}', created_at:{\$gte:ISODate('${marker}')}}));
" | tr -d '\r'
}

assert_evaluate() {
  local got_line="$1" expect_status="$2" expect_reason="$3"
  local got_status got_reason
  IFS='|' read -r got_status got_reason _ <<<"$got_line"
  [[ "$got_status" == "$expect_status" ]] || die "evaluate status want=${expect_status} got=${got_status} (${got_line})"
  [[ "$got_reason" == "$expect_reason" ]] || die "evaluate reason want=${expect_reason} got=${got_reason} (${got_line})"
}

assert_command() {
  local got_line="$1" expect_value="$2" expect_status="${3:-success}"
  local got_value got_status
  IFS='|' read -r got_value got_status _ <<<"$got_line"
  [[ "$got_value" == "$expect_value" ]] || die "command value want=${expect_value} got=${got_value} (${got_line})"
  [[ "$got_status" == "$expect_status" ]] || die "command status want=${expect_status} got=${got_status} (${got_line})"
}

assert_no_new_command() {
  local marker="$1"
  # Give the engine a short window to (incorrectly) emit a Set.
  sleep 5
  local n
  n="$(count_commands_after "$marker")"
  [[ "$n" == "0" ]] || die "expected no new command after ${marker}, got count=${n}"
}

get_lock() {
  python3 "$SCRIPT_DIR/usp_get.py" \
    --sn "$SN" \
    --tenant "$TENANT" \
    --mtp "$MTP" \
    --api-base "$API_BASE" \
    --controller "$CONTROLLER_CONTAINER" \
    --params "Device.X_TELKOMSEL_OntLock.Lock" \
    | awk -F= '/^Lock=/{print $2; exit}'
}

assert_lock() {
  local expect="$1"
  local got
  got="$(get_lock)"
  [[ "$got" == "$expect" ]] || die "Lock node want=${expect} got=${got}"
  log "Lock=${got} OK"
}

# Bring device to unlocked state via whitelist hit (best-effort).
unlock_helper() {
  log "unlock_helper: whitelist ${GOOD_CIDR}, AutoLock OFF"
  set_config true false
  upsert_whitelist "$GOOD_CIDR" "inter-case unlock"
  clear_unauthorized
  local marker
  marker="$(mark_time)"
  publish_online
  local eval_line cmd_line
  eval_line="$(wait_evaluate_after "$marker")"
  assert_evaluate "$eval_line" "UNLOCKED" "AUTHORIZED"
  cmd_line="$(wait_command_after "$marker")"
  assert_command "$cmd_line" "0" "success"
  assert_lock "0" || log "WARN: unlock_helper Lock Get failed (device may be offline)"
}

restore_safe() {
  log "restore_safe: Master ON, AutoLock OFF, whitelist ${GOOD_CIDR}"
  set_config true false || true
  upsert_whitelist "$GOOD_CIDR" "restored by ont-lock-e2e" || true
  clear_unauthorized || true
  local marker
  marker="$(mark_time)"
  if publish_online; then
    wait_evaluate_after "$marker" >/dev/null || true
    wait_command_after "$marker" >/dev/null || true
    get_lock >/dev/null 2>&1 || true
  fi
  log "restore_safe done"
}
