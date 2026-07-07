#!/bin/sh
# Run backend unit tests directly in the CI job container (no nested docker compose).
set -u

ROOT="${CI_PROJECT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"
REPORT_DIR="$ROOT/test-reports"
FAILED=0

mkdir -p "$REPORT_DIR"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

run_go_test() {
  name="$1"
  dir="$2"
  shift 2

  echo "=== Running ${name} ==="
  if (cd "$ROOT/$dir" && go test "$@" > "$REPORT_DIR/${name}.log" 2>&1); then
    echo "PASS: ${name}"
  else
    echo "FAIL: ${name}"
    cat "$REPORT_DIR/${name}.log"
    FAILED=1
  fi
}

run_go_test test-controller-unit backend/services/controller \
  -v -count=1 ./internal/cwmp/ ./internal/usp/usp_utils/ ./internal/api/

run_go_test test-acs backend/services/acs \
  -race -v -count=1 ./internal/server/handler/

run_go_test test-adapter backend/services/mtp/adapter \
  -v -count=1 ./internal/events/usp_handler/

echo "=== Running test-infra ==="
if (cd "$ROOT/deploy/tests" && PROJECT_ROOT="$ROOT" go test -v -count=1 ./... \
  > "$REPORT_DIR/test-infra.log" 2>&1); then
  echo "PASS: test-infra"
else
  echo "FAIL: test-infra"
  cat "$REPORT_DIR/test-infra.log"
  FAILED=1
fi

if [ "$FAILED" -ne 0 ]; then
  exit 1
fi
