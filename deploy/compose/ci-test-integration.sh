#!/bin/sh
# Run integration tests in the CI job container with GitLab service containers.
set -u

ROOT="${CI_PROJECT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"
REPORT_DIR="$ROOT/test-reports"
FAILED=0

mkdir -p "$REPORT_DIR"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export MONGO_TEST_URI="${MONGO_TEST_URI:-mongodb://mongo_test:27017}"
export NATS_TEST_URL="${NATS_TEST_URL:-nats://nats_test:4222}"

echo "Waiting for mongo_test and nats_test..."
sleep 5

run_go_test() {
  name="$1"
  shift

  echo "=== Running ${name} ==="
  if (cd "$ROOT/backend/services/controller" && go test "$@" \
    > "$REPORT_DIR/${name}.log" 2>&1); then
    echo "PASS: ${name}"
  else
    echo "FAIL: ${name}"
    cat "$REPORT_DIR/${name}.log"
    FAILED=1
  fi
}

run_go_test test-db \
  -v -count=1 -tags=integration -timeout=60s ./internal/db/

run_go_test test-bridge \
  -v -count=1 -timeout=120s -run 'ConcurrentSameDevice|DoesNotTimeout|SingleRequest_Success' ./internal/bridge/

run_go_test test-handlers \
  -v -count=1 -tags=integration -timeout=120s ./internal/api/

if [ "$FAILED" -ne 0 ]; then
  exit 1
fi
