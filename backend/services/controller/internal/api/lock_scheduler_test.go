package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/leandrofars/oktopus/internal/db"
)

func TestShouldMarkLockCommandForRetry_Boundary(t *testing.T) {
	// At maxAttempts-1, should still allow retry
	if !shouldMarkLockCommandForRetry(9, 10) {
		t.Fatal("attempt 9/10 should be retryable")
	}

	// At exactly maxAttempts, should stop retrying
	if shouldMarkLockCommandForRetry(10, 10) {
		t.Fatal("attempt 10/10 should not be retryable")
	}

	// Beyond maxAttempts
	if shouldMarkLockCommandForRetry(15, 10) {
		t.Fatal("attempt 15/10 should not be retryable")
	}

	// Zero maxAttempts should use default
	if !shouldMarkLockCommandForRetry(5, 0) {
		t.Fatal("attempt 5/default should be retryable")
	}
	if shouldMarkLockCommandForRetry(db.DefaultLockCommandMaxAttempts, 0) {
		t.Fatal("attempt at default max should not be retryable")
	}

	// Negative maxAttempts should use default
	if !shouldMarkLockCommandForRetry(1, -1) {
		t.Fatal("attempt 1 with negative max should be retryable")
	}
}

func TestShouldMarkLockCommandForRetry_NeverRetriesZeroAttempts(t *testing.T) {
	// Even with zero attempts, should be retryable (first attempt hasn't happened yet)
	if !shouldMarkLockCommandForRetry(0, 10) {
		t.Fatal("attempt 0/10 should be retryable")
	}
}

func TestIsPermanentLockCommandError_SchemaMissing(t *testing.T) {
	err := fmt.Errorf("usp error %d: CheckPathProperties: Path (Device.X_TELKOMSEL_OntLock) does not exist in the schema", uspErrCodePathNotInSchema)
	if !isPermanentLockCommandError(err) {
		t.Fatal("schema-missing USP error should be permanent")
	}
	if isPermanentLockCommandError(fmt.Errorf("usp request timeout")) {
		t.Fatal("timeout should remain retryable")
	}
}

func TestParseUSPErrorBody(t *testing.T) {
	body := []byte(`{"err_code":7026,"err_msg":"path missing"}`)
	err := parseUSPErrorBody(body)
	if err == nil || !strings.Contains(err.Error(), "7026") {
		t.Fatalf("expected parsed USP error, got %v", err)
	}
	if parseUSPErrorBody([]byte(`{"req_path_results":[]}`)) != nil {
		t.Fatal("GetResp body should not be treated as USP error")
	}
}
