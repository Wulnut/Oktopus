#!/usr/bin/env bash
# ONT Lock E2E: decision matrix + USP Lock readback.
# Usage: SN=081074000888 ./scripts/ont-lock-e2e/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

CASES="${CASES:-tc1,tc2,tc4,tc5,tc3,tc6}"
RESTORE="${RESTORE:-1}"

PASS_COUNT=0
FAIL_COUNT=0
RESULTS=()

usage() {
  cat <<EOF
Usage: SN=<serial> [TENANT=telkomsel] ./run.sh

Env:
  SN            required device serial
  TENANT        default telkomsel
  CASES         default tc1,tc2,tc4,tc5,tc3,tc6
  GOOD_CIDR     optional; auto <WAN>/32 when unset (AUTO_GOOD_CIDR=1)
  AUTO_GOOD_CIDR default 1
  BAD_CIDR      default 10.0.0.0/16
  RESTORE       default 1 (restore whitelist + unlock on exit)
  API_BASE      default http://127.0.0.1
  MTP           default mqtt
  COMPOSE_DIR   default <script>/../..
EOF
}

record_result() {
  local name="$1" ok="$2" detail="${3:-}"
  if [[ "$ok" == "PASS" ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    RESULTS+=("PASS  $name  $detail")
    log "PASS $name $detail"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    RESULTS+=("FAIL  $name  $detail")
    log "FAIL $name $detail"
  fi
}

run_case() {
  local name="$1"
  shift
  log "======== ${name} ========"
  if "$@"; then
    record_result "$name" PASS
    return 0
  else
    local ec=$?
    record_result "$name" FAIL "exit=${ec}"
    return 0  # continue remaining cases
  fi
}

# --- cases ---

case_tc1() {
  set_config true false
  upsert_whitelist "$GOOD_CIDR" "TC-1 whitelist hit"
  clear_unauthorized
  local marker eval_line cmd_line
  marker="$(mark_time)"
  publish_online
  eval_line="$(wait_evaluate_after "$marker")"
  assert_evaluate "$eval_line" "UNLOCKED" "AUTHORIZED"
  cmd_line="$(wait_command_after "$marker")"
  assert_command "$cmd_line" "0" "success"
  assert_lock "0"
}

case_tc2() {
  set_config true true
  upsert_whitelist "$BAD_CIDR" "TC-2 wrong CIDR"
  clear_unauthorized
  local marker eval_line cmd_line
  marker="$(mark_time)"
  publish_online
  eval_line="$(wait_evaluate_after "$marker")"
  assert_evaluate "$eval_line" "LOCKED" "UNAUTHORIZED"
  cmd_line="$(wait_command_after "$marker")"
  assert_command "$cmd_line" "1" "success"
  assert_lock "1"
}

case_tc4() {
  # PENDING path: ensure unlocked baseline first so Lock is stable.
  unlock_helper
  set_config true false
  delete_policy
  clear_unauthorized
  local marker eval_line
  marker="$(mark_time)"
  publish_online
  eval_line="$(wait_evaluate_after "$marker")"
  assert_evaluate "$eval_line" "PENDING" "UNAUTHORIZED"
  assert_no_new_command "$marker"
}

case_tc5() {
  # Master OFF forces unlock; ensure we can talk to device.
  unlock_helper || true
  set_config false false
  # keep or clear policy — Master OFF ignores whitelist
  clear_unauthorized
  local marker eval_line cmd_line
  marker="$(mark_time)"
  publish_online
  eval_line="$(wait_evaluate_after "$marker")"
  assert_evaluate "$eval_line" "UNLOCKED" "MASTER_DISABLED"
  cmd_line="$(wait_command_after "$marker")"
  assert_command "$cmd_line" "0" "success"
  assert_lock "0"
}

case_tc3() {
  set_config true true
  delete_policy
  clear_unauthorized
  local marker eval_line cmd_line
  marker="$(mark_time)"
  publish_online
  eval_line="$(wait_evaluate_after "$marker")"
  assert_evaluate "$eval_line" "LOCKED" "UNAUTHORIZED"
  cmd_line="$(wait_command_after "$marker")"
  assert_command "$cmd_line" "1" "success"
  assert_lock "1"
}

# Exceptions batch-whitelist API: reported_ip/32 must chase-evaluate to UNLOCKED.
case_tc6() {
  local reported_ip wan_cidr marker audit_details eval_line cmd_line

  reported_ip="$(get_wan_ip)"
  if [[ -z "$reported_ip" ]]; then
    die "TC-6: could not read WAN IP for ${SN}"
    return 1
  fi
  wan_cidr="${reported_ip}/32"
  log "TC-6 WAN IP=${reported_ip} whitelist CIDR=${wan_cidr}"

  set_config true true
  delete_policy
  clear_unauthorized
  marker="$(mark_time)"
  publish_online
  eval_line="$(wait_evaluate_after "$marker")"
  assert_evaluate "$eval_line" "LOCKED" "UNAUTHORIZED"
  cmd_line="$(wait_command_after "$marker")"
  assert_command "$cmd_line" "1" "success"
  assert_lock "1"

  seed_unauthorized "$reported_ip"
  marker="$(mark_time)"
  api_batch_whitelist "$reported_ip" "$wan_cidr"

  audit_details="$(wait_audit_action_after "$marker" "unauthorized_batch_whitelist")"
  assert_audit_cidr "$audit_details" "$wan_cidr"

  eval_line="$(wait_evaluate_after "$marker")"
  assert_evaluate "$eval_line" "UNLOCKED" "AUTHORIZED"
  cmd_line="$(wait_command_after "$marker")"
  assert_command "$cmd_line" "0" "success"
  assert_lock "0"
}

EXIT_CODE=0
on_exit() {
  if [[ "$RESTORE" == "1" ]]; then
    restore_safe || true
  fi
  exit "$EXIT_CODE"
}

main() {
  if [[ -z "${SN:-}" ]]; then
    usage
    fail "SN is required"
  fi

  resolve_good_cidr

  log "ONT Lock E2E  SN=${SN} TENANT=${TENANT} CASES=${CASES}"
  log "COMPOSE_DIR=${COMPOSE_DIR} API_BASE=${API_BASE} MTP=${MTP}"
  log "device snapshot:"
  device_snapshot || true

  if [[ "$RESTORE" == "1" ]]; then
    trap on_exit EXIT
  fi

  local prev_locked=0
  IFS=',' read -ra CASE_LIST <<<"$CASES"
  local c
  for c in "${CASE_LIST[@]}"; do
    c="$(echo "$c" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')"
    [[ -n "$c" ]] || continue

    # After a lock case, unlock before cases that need Lock=0 or clean PENDING.
    if [[ "$prev_locked" == "1" ]]; then
      case "$c" in
        tc1|tc4|tc5)
          log "previous case locked device; running unlock_helper before ${c}"
          unlock_helper || log "WARN: unlock_helper failed before ${c}"
          prev_locked=0
          ;;
      esac
    fi

    case "$c" in
      tc1) run_case TC-1 case_tc1; prev_locked=0 ;;
      tc2) run_case TC-2 case_tc2; prev_locked=1 ;;
      tc3) run_case TC-3 case_tc3; prev_locked=1 ;;
      tc4) run_case TC-4 case_tc4; prev_locked=0 ;;
      tc5) run_case TC-5 case_tc5; prev_locked=0 ;;
      tc6) run_case TC-6 case_tc6; prev_locked=0 ;;
      *)
        record_result "$c" FAIL "unknown case"
        ;;
    esac
  done

  echo
  echo "======== SUMMARY ========"
  local line
  for line in "${RESULTS[@]}"; do
    echo "$line"
  done
  echo "PASS=${PASS_COUNT} FAIL=${FAIL_COUNT}"

  if (( FAIL_COUNT > 0 )); then
    EXIT_CODE=1
  else
    EXIT_CODE=0
  fi
  return "$EXIT_CODE"
}

main "$@"
EXIT_CODE=$?
# Explicit exit so trap sees EXIT_CODE (main already set it).
exit "$EXIT_CODE"
