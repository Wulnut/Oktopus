#!/bin/sh
# Run frontend unit tests directly in the CI job container.
set -u

ROOT="${CI_PROJECT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"
REPORT_DIR="$ROOT/test-reports"

mkdir -p "$REPORT_DIR"
cd "$ROOT/frontend"

echo "=== Running test-frontend ==="
export JEST_JUNIT_OUTPUT_FILE="$REPORT_DIR/frontend-junit.xml"

if npm ci && npm test -- --verbose --ci --reporters=default --reporters=jest-junit \
  > "$REPORT_DIR/test-frontend.log" 2>&1; then
  echo "PASS: test-frontend"
else
  echo "FAIL: test-frontend"
  cat "$REPORT_DIR/test-frontend.log"
  exit 1
fi
