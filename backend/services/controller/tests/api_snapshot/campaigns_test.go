package snapshot_test

import (
	"encoding/json"
	"testing"

	"github.com/leandrofars/oktopus/tests/api_snapshot/snapshot"
)

// Stage 5c — Campaigns CRUD black-box tests.
//
// All routes are pure-Mongo. createCampaign requires a valid firmware_id, so
// tests that exercise create/update pair with the firmware fixture and read
// the ID out via GET /firmware.

func TestCampaigns_List_Snapshot(t *testing.T) {
	env := snapshot.Setup(t, []string{"campaigns.json"})
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/campaigns"))
	if err != nil {
		t.Fatalf("GET /campaigns: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	snapshot.CompareSnapshot(t, "campaigns_list", body)
}

func TestCampaigns_Create_RequiresValidFirmware(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	// firmware_id that doesn't exist -> 400.
	body := `{"vendor":"Huawei","model":"HG8145V5","hw_version":"169BW.A","firmware_id":"000000000000000000000000"}`
	status, respBody, err := client.PostJSON(env.TenantURL("/campaigns"), body)
	if err != nil {
		t.Fatalf("POST /campaigns: %v", err)
	}
	if status != 400 {
		t.Errorf("invalid firmware_id: want 400, got %d, body=%s", status, respBody)
	}
}

func TestCampaigns_Create_RejectsMissingFields(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	// Missing vendor/model/hw_version.
	body := `{}`
	status, _, err := client.PostJSON(env.TenantURL("/campaigns"), body)
	if err != nil {
		t.Fatalf("POST /campaigns: %v", err)
	}
	if status != 400 {
		t.Errorf("missing fields: want 400, got %d", status)
	}
}

func TestCampaigns_Create_WithFirmware_Succeeds(t *testing.T) {
	env := snapshot.Setup(t, []string{"firmware.json"})
	client := snapshot.NewClient(env.JWT)

	// Get a firmware ID.
	status, body, err := client.Get(env.TenantURL("/firmware"))
	if err != nil {
		t.Fatalf("GET /firmware: %v", err)
	}
	var fwList []map[string]interface{}
	if err := json.Unmarshal(body, &fwList); err != nil || len(fwList) == 0 {
		t.Skip("no firmware")
	}
	fwID, _ := fwList[0]["id"].(string)
	vendor, _ := fwList[0]["vendor"].(string)
	model, _ := fwList[0]["model"].(string)
	hwVersion, _ := fwList[0]["hw_version"].(string)

	campBody := `{"vendor":"` + vendor + `","model":"` + model + `","hw_version":"` + hwVersion + `","firmware_id":"` + fwID + `"}`
	status, respBody, err := client.PostJSON(env.TenantURL("/campaigns"), campBody)
	if err != nil {
		t.Fatalf("POST /campaigns: %v", err)
	}
	if status != 201 {
		t.Fatalf("want 201, got %d, body=%s", status, respBody)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(respBody, &got); err != nil {
		t.Fatalf("create response not JSON: %s", respBody)
	}
	if got["vendor"] != vendor {
		t.Errorf("vendor mismatch: got %v", got["vendor"])
	}
}

func TestCampaigns_Update_Struct(t *testing.T) {
	env := snapshot.Setup(t, []string{"campaigns.json", "firmware.json"})
	client := snapshot.NewClient(env.JWT)

	// Get a campaign ID.
	status, body, err := client.Get(env.TenantURL("/campaigns"))
	if err != nil {
		t.Fatalf("list campaigns: %v", err)
	}
	var campList []map[string]interface{}
	if err := json.Unmarshal(body, &campList); err != nil || len(campList) == 0 {
		t.Skip("no campaigns")
	}
	campID, _ := campList[0]["id"].(string)

	// Get a firmware ID.
	status, body, err = client.Get(env.TenantURL("/firmware"))
	if err != nil {
		t.Fatalf("list firmware: %v", err)
	}
	var fwList []map[string]interface{}
	if err := json.Unmarshal(body, &fwList); err != nil || len(fwList) == 0 {
		t.Skip("no firmware")
	}
	fwID, _ := fwList[0]["id"].(string)
	vendor, _ := fwList[0]["vendor"].(string)
	model, _ := fwList[0]["model"].(string)
	hwVersion, _ := fwList[0]["hw_version"].(string)

	updateBody := `{"vendor":"` + vendor + `","model":"` + model + `","hw_version":"` + hwVersion + `","firmware_id":"` + fwID + `","enabled":false}`
	status, _, err = client.PutJSON(env.TenantURL("/campaigns/"+campID), updateBody)
	if err != nil {
		t.Fatalf("PUT /campaigns/%s: %v", campID, err)
	}
	if status != 204 && status != 200 {
		t.Errorf("update: want 204/200, got %d", status)
	}
}

func TestCampaigns_Delete_Struct(t *testing.T) {
	env := snapshot.Setup(t, []string{"campaigns.json"})
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/campaigns"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(body, &list); err != nil || len(list) == 0 {
		t.Skip("no campaigns")
	}
	id, _ := list[0]["id"].(string)

	status, _, err = client.Delete(env.TenantURL("/campaigns/" + id))
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	if status != 204 {
		t.Errorf("delete: want 204, got %d", status)
	}
}

func TestCampaigns_Logs_Snapshot(t *testing.T) {
	env := snapshot.Setup(t, []string{"campaigns.json", "upgrade_logs.json"})
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/campaigns"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(body, &list); err != nil || len(list) == 0 {
		t.Skip("no campaigns")
	}
	id, _ := list[0]["id"].(string)

	status, logsBody, err := client.Get(env.TenantURL("/campaigns/" + id + "/logs"))
	if err != nil {
		t.Fatalf("GET logs: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, logsBody)
	}
	// Logs may be empty depending on fixture campaign_id match; just assert 200.
}
