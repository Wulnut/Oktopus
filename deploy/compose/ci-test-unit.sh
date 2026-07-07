#!/bin/sh
# Run Oktopus unit tests serially in CI (one compose stack at a time).
set -u

COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.test.yaml}"
REPORT_DIR="${CI_PROJECT_DIR:-../..}/test-reports"
FAILED=0

mkdir -p "$REPORT_DIR"
cd "$(dirname "$0")"

run_service() {
  name="$1"
  profile="$2"
  echo "=== Running ${name} ==="
  if docker compose -f "$COMPOSE_FILE" --profile "$profile" run --rm \
    -v "$REPORT_DIR:/test-reports" \
    "$name" > "$REPORT_DIR/${name}.log" 2>&1; then
    echo "PASS: ${name}"
  else
    echo "FAIL: ${name}"
    cat "$REPORT_DIR/${name}.log"
    FAILED=1
  fi
}

run_service test-controller-unit unit
run_service test-acs unit
run_service test-adapter unit
run_service test-infra unit
run_service test-frontend unit

if [ "$FAILED" -ne 0 ]; then
  exit 1
fi
