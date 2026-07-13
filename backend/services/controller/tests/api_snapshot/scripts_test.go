package snapshot_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/leandrofars/oktopus/tests/api_snapshot/snapshot"
)

// Stage 5b — Scripts CRUD black-box tests. All routes are pure-Mongo.

func TestScripts_List_Snapshot(t *testing.T) {
	env := snapshot.Setup(t, []string{"scripts.json"})
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/scripts"))
	if err != nil {
		t.Fatalf("GET /scripts: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	snapshot.CompareSnapshot(t, "scripts_list", body)
}

func TestScripts_List_Empty(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/scripts"))
	if err != nil {
		t.Fatalf("GET /scripts: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d", status)
	}
	got := strings.TrimSpace(string(body))
	if got != "[]" && got != "null" {
		t.Errorf("expected empty array, got %s", body)
	}
}

func TestScripts_Create_Struct(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	body := `{
		"name": "new-test-script",
		"description": "Created by black-box test",
		"steps": [
			{"id":"s1","name":"Get","type":"GET","param_paths":["Device.DeviceInfo.ModelName"],"max_depth":1}
		]
	}`
	status, respBody, err := client.PostJSON(env.TenantURL("/scripts"), body)
	if err != nil {
		t.Fatalf("POST /scripts: %v", err)
	}
	if status != 200 && status != 201 {
		t.Fatalf("want 200/201, got %d, body=%s", status, respBody)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(respBody, &got); err != nil {
		t.Fatalf("create response not JSON: %s", respBody)
	}
	if got["name"] != "new-test-script" {
		t.Errorf("name mismatch: got %v", got["name"])
	}
}

func TestScripts_Create_RejectsMissingName(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	body := `{"description":"no name"}`
	status, _, err := client.PostJSON(env.TenantURL("/scripts"), body)
	if err != nil {
		t.Fatalf("POST /scripts: %v", err)
	}
	if status != 400 {
		t.Errorf("missing name: want 400, got %d", status)
	}
}

func TestScripts_Create_RejectsNoSteps(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	body := `{"name":"empty-script"}`
	status, _, err := client.PostJSON(env.TenantURL("/scripts"), body)
	if err != nil {
		t.Fatalf("POST /scripts: %v", err)
	}
	if status != 400 {
		t.Errorf("no steps: want 400, got %d", status)
	}
}

func TestScripts_Get_Snapshot(t *testing.T) {
	env := snapshot.Setup(t, []string{"scripts.json"})
	client := snapshot.NewClient(env.JWT)

	// Look up an ID first.
	status, body, err := client.Get(env.TenantURL("/scripts"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(body, &list); err != nil || len(list) == 0 {
		t.Skip("no scripts")
	}
	id, _ := list[0]["id"].(string)

	status, body, err = client.Get(env.TenantURL("/scripts/" + id))
	if err != nil {
		t.Fatalf("GET /scripts/%s: %v", id, err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	snapshot.CompareSnapshot(t, "script_get", body)
}

func TestScripts_Get_NotFound(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, _, err := client.Get(env.TenantURL("/scripts/000000000000000000000000"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if status != 404 {
		t.Errorf("nonexistent script: want 404, got %d", status)
	}
}

func TestScripts_Update_Struct(t *testing.T) {
	env := snapshot.Setup(t, []string{"scripts.json"})
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/scripts"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(body, &list); err != nil || len(list) == 0 {
		t.Skip("no scripts")
	}
	id, _ := list[0]["id"].(string)

	update := `{
		"name": "updated-name",
		"description": "Updated by test",
		"steps": [
			{"id":"s1","name":"Get","type":"GET","param_paths":["Device.DeviceInfo.SerialNumber"]}
		]
	}`
	status, _, err = client.PutJSON(env.TenantURL("/scripts/"+id), update)
	if err != nil {
		t.Fatalf("PUT /scripts/%s: %v", id, err)
	}
	if status != 204 && status != 200 {
		t.Errorf("update: want 204/200, got %d", status)
	}

	// Verify via GET.
	status, body, err = client.Get(env.TenantURL("/scripts/" + id))
	if err != nil {
		t.Fatalf("re-get: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("re-get not JSON: %s", body)
	}
	if got["name"] != "updated-name" {
		t.Errorf("name not updated: got %v", got["name"])
	}
}

func TestScripts_Delete_RemovesRecord(t *testing.T) {
	env := snapshot.Setup(t, []string{"scripts.json"})
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/scripts"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(body, &list); err != nil || len(list) == 0 {
		t.Skip("no scripts")
	}
	id, _ := list[0]["id"].(string)

	status, _, err = client.Delete(env.TenantURL("/scripts/" + id))
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	if status != 204 && status != 200 {
		t.Errorf("delete: want 204/200, got %d", status)
	}

	// Verify gone.
	status, _, err = client.Get(env.TenantURL("/scripts/" + id))
	if err != nil {
		t.Fatalf("re-get: %v", err)
	}
	if status != 404 {
		t.Errorf("deleted script should 404: got %d", status)
	}
}

func TestScripts_Executions_EmptyList(t *testing.T) {
	env := snapshot.Setup(t, []string{"scripts.json"})
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/scripts"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(body, &list); err != nil || len(list) == 0 {
		t.Skip("no scripts")
	}
	id, _ := list[0]["id"].(string)

	status, execBody, err := client.Get(env.TenantURL("/scripts/" + id + "/executions"))
	if err != nil {
		t.Fatalf("GET executions: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d", status)
	}
	got := strings.TrimSpace(string(execBody))
	if got != "[]" && got != "null" {
		t.Errorf("expected empty array, got %s", execBody)
	}
}
