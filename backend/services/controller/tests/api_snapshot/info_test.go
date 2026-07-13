package snapshot_test

import (
	"testing"

	"github.com/leandrofars/oktopus/tests/api_snapshot/snapshot"
)

// Stage 4 — Dashboard Info black-box tests.
//
// All four /info/* routes aggregate device data via NATS-mediated calls to
// the adapter service (devices.vendors / devices.class / devices.status +
// RTT pings for general). Without the adapter subscribed, each returns 500
// after the NATS timeout.
//
// These tests assert the routes are wired (not 404) and that they correctly
// reach the broker. Once a real adapter is subscribed in a fuller integration
// environment, the same tests can be upgraded to snapshot real aggregations.

func TestInfo_Vendors_ReachestBroker(t *testing.T) {
	env := snapshot.Setup(t, []string{"devices.json"})
	client := snapshot.NewClient(env.JWT)

	status, _, err := client.Get(env.TenantURL("/info/vendors"))
	if err != nil {
		t.Fatalf("GET /info/vendors: %v", err)
	}
	// Without adapter, NatsReq times out -> 500.
	if status != 500 {
		t.Errorf("want 500 (NATS timeout, no adapter), got %d", status)
	}
}

func TestInfo_Status_ReachestBroker(t *testing.T) {
	env := snapshot.Setup(t, []string{"devices.json"})
	client := snapshot.NewClient(env.JWT)

	status, _, err := client.Get(env.TenantURL("/info/status"))
	if err != nil {
		t.Fatalf("GET /info/status: %v", err)
	}
	if status != 500 {
		t.Errorf("want 500 (NATS timeout, no adapter), got %d", status)
	}
}

func TestInfo_DeviceClass_ReachestBroker(t *testing.T) {
	env := snapshot.Setup(t, []string{"devices.json"})
	client := snapshot.NewClient(env.JWT)

	status, _, err := client.Get(env.TenantURL("/info/device_class"))
	if err != nil {
		t.Fatalf("GET /info/device_class: %v", err)
	}
	if status != 500 {
		t.Errorf("want 500 (NATS timeout, no adapter), got %d", status)
	}
}

func TestInfo_General_ReachestBroker(t *testing.T) {
	env := snapshot.Setup(t, []string{"devices.json"})
	client := snapshot.NewClient(env.JWT)

	// generalInfo fans out to multiple NATS subjects concurrently; any one
	// failure short-circuits the response to 500.
	status, _, err := client.Get(env.TenantURL("/info/general"))
	if err != nil {
		t.Fatalf("GET /info/general: %v", err)
	}
	if status != 500 {
		t.Errorf("want 500 (NATS timeout, no adapter), got %d", status)
	}
}

func TestInfo_Routes_RequireAuth(t *testing.T) {
	env := snapshot.Setup(t, nil)
	unauthed := snapshot.NewClient("")

	for _, p := range []string{"/info/vendors", "/info/status", "/info/device_class", "/info/general"} {
		status, _, err := unauthed.Get(env.TenantURL(p))
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		if status != 401 {
			t.Errorf("%s without token: want 401, got %d", p, status)
		}
	}
}
