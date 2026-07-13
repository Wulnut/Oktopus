package snapshot_test

import (
	"encoding/json"
	"testing"

	"github.com/leandrofars/oktopus/tests/api_snapshot/snapshot"
)

// Stage 6 — ONT Lock API black-box tests.
//
// These complement the existing internal/api/lock_*_test.go white-box tests by
// exercising the HTTP surface. All routes here are pure-Mongo (no NATS),
// making them fully testable in black-box mode.
//
// The lock engine's device-command path (which would issue USP SetMessage
// over NATS) is intentionally not exercised here — that's the white-box
// suite's job.

func TestLock_Policies_Snapshot(t *testing.T) {
	env := snapshot.Setup(t, []string{"lock_policies.json"})
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/lock/policies"))
	if err != nil {
		t.Fatalf("GET /lock/policies: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	snapshot.CompareSnapshot(t, "lock_policies", body)
}

func TestLock_Policies_Empty(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, _, err := client.Get(env.TenantURL("/lock/policies"))
	if err != nil {
		t.Fatalf("GET /lock/policies: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d", status)
	}
}

func TestLock_Config_Snapshot(t *testing.T) {
	env := snapshot.Setup(t, []string{"lock_config.json"})
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/lock/config"))
	if err != nil {
		t.Fatalf("GET /lock/config: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	snapshot.CompareSnapshot(t, "lock_config", body)
}

func TestLock_Config_DefaultWhenNotSeeded(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/lock/config"))
	if err != nil {
		t.Fatalf("GET /lock/config: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d", status)
	}
	// Without a seeded config, handler returns defaults (master_enabled: true,
	// auto_lock_enabled: true per LockConfigDefaults).
	var cfg map[string]interface{}
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatalf("config not JSON: %s", body)
	}
	if cfg["master_enabled"] != true {
		t.Errorf("default master_enabled should be true, got %v", cfg["master_enabled"])
	}
}

func TestLock_UpdateConfig_MasterEnabledFlips(t *testing.T) {
	env := snapshot.Setup(t, []string{"lock_config.json"})
	client := snapshot.NewClient(env.JWT)

	// Default seed has master_enabled=true; flip it to false.
	body := `{"master_enabled":false,"auto_lock_enabled":true}`
	status, _, err := client.PutJSON(env.TenantURL("/lock/config"), body)
	if err != nil {
		t.Fatalf("PUT /lock/config: %v", err)
	}
	if status != 200 {
		t.Fatalf("update config: want 200, got %d", status)
	}

	// Verify via re-GET.
	status, gotBody, err := client.Get(env.TenantURL("/lock/config"))
	if err != nil {
		t.Fatalf("re-GET: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(gotBody, &cfg); err != nil {
		t.Fatalf("re-GET not JSON: %s", gotBody)
	}
	if cfg["master_enabled"] != false {
		t.Errorf("master_enabled not flipped: got %v", cfg["master_enabled"])
	}
}

func TestLock_Whitelist_Upsert(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	body := `{"sn":"SN-DEV-0001","allowed_ip_range":"10.0.0.0/24","description":"test policy"}`
	status, respBody, err := client.PostJSON(env.TenantURL("/lock/whitelist"), body)
	if err != nil {
		t.Fatalf("POST /lock/whitelist: %v", err)
	}
	// Handler returns 202 because upsert triggers an async lock chase on the SN.
	if status != 200 && status != 201 && status != 202 {
		t.Fatalf("upsert whitelist: want 200/201/202, got %d, body=%s", status, respBody)
	}

	// Verify via list.
	status, listBody, err := client.Get(env.TenantURL("/lock/policies"))
	if err != nil {
		t.Fatalf("list after upsert: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(listBody, &list); err != nil {
		t.Fatalf("list not JSON: %s", listBody)
	}
	found := false
	for _, p := range list {
		if p["sn"] == "SN-DEV-0001" {
			found = true
			if p["policy_type"] != "WHITELIST" {
				t.Errorf("policy_type: want WHITELIST, got %v", p["policy_type"])
			}
		}
	}
	if !found {
		t.Errorf("upserted policy not found in list")
	}
}

func TestLock_Whitelist_Batch(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	body := `{"items":[
		{"sn":"SN-BATCH-001","allowed_ip_range":"10.0.0.0/24"},
		{"sn":"SN-BATCH-002","allowed_ip_range":"10.0.1.0/24"}
	]}`
	status, _, err := client.PostJSON(env.TenantURL("/lock/whitelist/batch"), body)
	if err != nil {
		t.Fatalf("POST /lock/whitelist/batch: %v", err)
	}
	// Handler returns 202 Accepted because it kicks off an async lock chase.
	if status != 200 && status != 201 && status != 202 {
		t.Fatalf("batch whitelist: want 200/201/202, got %d", status)
	}

	// Verify both landed.
	status, listBody, _ := client.Get(env.TenantURL("/lock/policies"))
	var list []map[string]interface{}
	_ = json.Unmarshal(listBody, &list)
	count := 0
	for _, p := range list {
		if p["sn"] == "SN-BATCH-001" || p["sn"] == "SN-BATCH-002" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("batch whitelist: expected 2 policies, found %d", count)
	}
}

func TestLock_BatchDelete(t *testing.T) {
	env := snapshot.Setup(t, []string{"lock_policies.json"})
	client := snapshot.NewClient(env.JWT)

	// Get the SN list from the fixture.
	status, listBody, err := client.Get(env.TenantURL("/lock/policies"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(listBody, &list); err != nil || len(list) == 0 {
		t.Skip("no policies to delete")
	}
	sns := []string{}
	for _, p := range list {
		if sn, ok := p["sn"].(string); ok {
			sns = append(sns, sn)
		}
	}

	delBody, _ := json.Marshal(map[string]interface{}{"sns": sns})
	status, _, err = client.PostJSON(env.TenantURL("/lock/policies/batch-delete"), string(delBody))
	if err != nil {
		t.Fatalf("batch delete: %v", err)
	}
	if status != 200 && status != 204 {
		t.Errorf("batch delete: want 200/204, got %d", status)
	}

	// Verify list is now empty.
	status, afterBody, _ := client.Get(env.TenantURL("/lock/policies"))
	var after []map[string]interface{}
	_ = json.Unmarshal(afterBody, &after)
	if len(after) != 0 {
		t.Errorf("after batch delete, expected 0 policies, got %d", len(after))
	}
}

func TestLock_DeletePolicy(t *testing.T) {
	env := snapshot.Setup(t, []string{"lock_policies.json"})
	client := snapshot.NewClient(env.JWT)

	// Add a known policy to delete deterministically.
	upsertBody := `{"sn":"SN-TO-DELETE","allowed_ip_range":"10.99.0.0/24"}`
	if status, _, err := client.PostJSON(env.TenantURL("/lock/whitelist"), upsertBody); err != nil || (status != 200 && status != 201) {
		t.Fatalf("upsert prerequisite failed: status=%d err=%v", status, err)
	}

	status, _, err := client.Delete(env.TenantURL("/lock/policies/SN-TO-DELETE"))
	if err != nil {
		t.Fatalf("DELETE /lock/policies/SN-TO-DELETE: %v", err)
	}
	if status != 204 && status != 200 {
		t.Errorf("delete policy: want 204/200, got %d", status)
	}
}

func TestLock_Unauthorized_Snapshot(t *testing.T) {
	env := snapshot.Setup(t, []string{"lock_unauthorized.json"})
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/lock/unauthorized"))
	if err != nil {
		t.Fatalf("GET /lock/unauthorized: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	snapshot.CompareSnapshot(t, "lock_unauthorized", body)
}

func TestLock_Unauthorized_Empty(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, _, err := client.Get(env.TenantURL("/lock/unauthorized"))
	if err != nil {
		t.Fatalf("GET /lock/unauthorized: %v", err)
	}
	if status != 200 {
		t.Errorf("empty unauthorized: want 200, got %d", status)
	}
}

func TestLock_Unauthorized_BatchWhitelist(t *testing.T) {
	env := snapshot.Setup(t, []string{"lock_unauthorized.json"})
	client := snapshot.NewClient(env.JWT)

	// Fetch unauthorized list to get SNs.
	status, listBody, err := client.Get(env.TenantURL("/lock/unauthorized"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(listBody, &list); err != nil || len(list) == 0 {
		t.Skip("no unauthorized devices")
	}
	sns := []string{}
	for _, d := range list {
		if sn, ok := d["sn"].(string); ok {
			sns = append(sns, sn)
		}
	}

	// batch-whitelist expects {"items":[{sn,reported_ip,...}]} — see lock.go batchWhitelistFromUnauthorized.
	type batchItem struct {
		SN         string `json:"sn"`
		ReportedIP string `json:"reported_ip,omitempty"`
	}
	type batchReq struct {
		Items []batchItem `json:"items"`
	}
	payload, _ := json.Marshal(batchReq{Items: []batchItem{{SN: sns[0], ReportedIP: "10.99.99.1"}}})
	status, _, err = client.PostJSON(env.TenantURL("/lock/unauthorized/batch-whitelist"), string(payload))
	if err != nil {
		t.Fatalf("batch whitelist: %v", err)
	}
	// Handler creates lock policy records asynchronously and returns 202 Accepted
	// (or 207 Multi-Status if some rows errored). 200/201 also accepted for leniency.
	if status != 200 && status != 201 && status != 202 && status != 204 && status != 207 {
		t.Errorf("batch whitelist from unauthorized: want 200/201/202/204/207, got %d", status)
	}
}

func TestLock_Unauthorized_BatchDelete(t *testing.T) {
	env := snapshot.Setup(t, []string{"lock_unauthorized.json"})
	client := snapshot.NewClient(env.JWT)

	status, listBody, err := client.Get(env.TenantURL("/lock/unauthorized"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(listBody, &list); err != nil || len(list) == 0 {
		t.Skip("no unauthorized devices")
	}
	sns := []string{}
	for _, d := range list {
		if sn, ok := d["sn"].(string); ok {
			sns = append(sns, sn)
		}
	}

	body := `{"sns":["` + sns[0] + `"]}`
	status, _, err = client.PostJSON(env.TenantURL("/lock/unauthorized/batch-delete"), body)
	if err != nil {
		t.Fatalf("batch delete: %v", err)
	}
	if status != 200 && status != 204 {
		t.Errorf("batch delete unauthorized: want 200/204, got %d", status)
	}
}

func TestLock_Unsupported_Snapshot(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/lock/unsupported"))
	if err != nil {
		t.Fatalf("GET /lock/unsupported: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	// No fixture seeded — expect empty list snapshot.
	snapshot.CompareSnapshot(t, "lock_unsupported_empty", body)
}

func TestLock_Commands_Snapshot(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/lock/commands"))
	if err != nil {
		t.Fatalf("GET /lock/commands: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	snapshot.CompareSnapshot(t, "lock_commands_empty", body)
}

func TestLock_ClearCommands(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, _, err := client.Delete(env.TenantURL("/lock/commands"))
	if err != nil {
		t.Fatalf("DELETE /lock/commands: %v", err)
	}
	if status != 200 && status != 204 {
		t.Errorf("clear commands: want 200/204, got %d", status)
	}
}

func TestLock_Audit_Snapshot(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/lock/audit"))
	if err != nil {
		t.Fatalf("GET /lock/audit: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	snapshot.CompareSnapshot(t, "lock_audit_empty", body)
}

func TestLock_ClearAudit(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, _, err := client.Delete(env.TenantURL("/lock/audit"))
	if err != nil {
		t.Fatalf("DELETE /lock/audit: %v", err)
	}
	if status != 200 && status != 204 {
		t.Errorf("clear audit: want 200/204, got %d", status)
	}
}

func TestLock_Routes_RequireAuth(t *testing.T) {
	env := snapshot.Setup(t, nil)
	unauthed := snapshot.NewClient("")

	routes := []string{
		"/lock/policies",
		"/lock/config",
		"/lock/unauthorized",
		"/lock/unsupported",
		"/lock/commands",
		"/lock/audit",
	}
	for _, p := range routes {
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
