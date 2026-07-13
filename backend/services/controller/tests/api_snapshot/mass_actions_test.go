package snapshot_test

import (
	"testing"

	"github.com/leandrofars/oktopus/tests/api_snapshot/snapshot"
)

// Stage 7a — Mass Actions black-box tests.
//
// listMassActions / getMassAction / cancelMassAction are pure-Mongo.
// massScriptExecution (POST /mass-actions/script) needs NATS to fan out to
// devices, so we only test its request validation (4xx) here.

func TestMassActions_List_EmptySnapshot(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/mass-actions"))
	if err != nil {
		t.Fatalf("GET /mass-actions: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	snapshot.CompareSnapshot(t, "mass_actions_empty", body)
}

func TestMassActions_Get_NotFound(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, _, err := client.Get(env.TenantURL("/mass-actions/000000000000000000000000"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if status != 404 {
		t.Errorf("nonexistent mass action: want 404, got %d", status)
	}
}

func TestMassActions_Get_InvalidID(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, _, err := client.Get(env.TenantURL("/mass-actions/not-a-hex-id"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if status != 400 {
		t.Errorf("invalid id: want 400, got %d", status)
	}
}

func TestMassActions_Cancel_NotFound(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	// CancelMassAction uses UpdateOne({_id}, {$set:{status:cancelled}}) which
	// is idempotent and does NOT error when no document matches — it returns
	// 204 No Content either way. This is the production behavior.
	status, _, err := client.PostJSON(env.TenantURL("/mass-actions/000000000000000000000000/cancel"), `{}`)
	if err != nil {
		t.Fatalf("POST cancel: %v", err)
	}
	if status != 204 {
		t.Errorf("cancel nonexistent: want 204 (idempotent), got %d", status)
	}
}

func TestMassActions_Script_Create_RejectsEmptyDevices(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	body := `{"script_id":"000000000000000000000000","device_sns":[]}`
	status, respBody, err := client.PostJSON(env.TenantURL("/mass-actions/script"), body)
	if err != nil {
		t.Fatalf("POST /mass-actions/script: %v", err)
	}
	if status != 400 {
		t.Errorf("empty device_sns: want 400, got %d, body=%s", status, respBody)
	}
}

func TestMassActions_Script_Create_RejectsNoBody(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, _, err := client.PostJSON(env.TenantURL("/mass-actions/script"), `{}`)
	if err != nil {
		t.Fatalf("POST /mass-actions/script: %v", err)
	}
	if status != 400 {
		t.Errorf("empty body: want 400, got %d", status)
	}
}

func TestMassActions_Routes_RequireAuth(t *testing.T) {
	env := snapshot.Setup(t, nil)
	unauthed := snapshot.NewClient("")

	status, _, err := unauthed.Get(env.TenantURL("/mass-actions"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if status != 401 {
		t.Errorf("without token: want 401, got %d", status)
	}
}
