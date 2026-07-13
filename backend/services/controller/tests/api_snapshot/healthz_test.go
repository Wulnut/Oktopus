package snapshot_test

import (
	"testing"

	"github.com/leandrofars/oktopus/tests/api_snapshot/snapshot"
)

// TestHealthz is the Stage 1 acceptance test: it proves the snapshot module
// boots, reaches the test Mongo, mounts the real router via BuildRouter, and
// can serve a public GET request through httptest.Server. No fixture required.
func TestHealthz(t *testing.T) {
	if testing.Short() {
		t.Skip("snapshot tests require Mongo + NATS")
	}
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient("")

	status, body, err := client.Get(env.AdminURL("/healthz"))
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	if string(body) != `{"status":"ok"}` {
		t.Fatalf("unexpected body: %s", body)
	}
}

// TestReadyz proves the readiness probe wires through to the test Mongo via Ping.
func TestReadyz(t *testing.T) {
	if testing.Short() {
		t.Skip("snapshot tests require Mongo + NATS")
	}
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient("")

	status, body, err := client.Get(env.AdminURL("/readyz"))
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
}
