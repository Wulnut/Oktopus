package snapshot_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/leandrofars/oktopus/tests/api_snapshot/snapshot"
)

// Stage 5a — Firmware CRUD black-box tests.
//
// All firmware routes are pure-Mongo (list, create via download_url, update,
// delete, set phase). The file-upload path needs an external file-server and
// is not exercised here; we test the download_url path instead.

func TestFirmware_List_Snapshot(t *testing.T) {
	env := snapshot.Setup(t, []string{"firmware.json"})
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/firmware"))
	if err != nil {
		t.Fatalf("GET /firmware: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	snapshot.CompareSnapshot(t, "firmware_list", body)
}

func TestFirmware_List_Empty(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/firmware"))
	if err != nil {
		t.Fatalf("GET /firmware: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d", status)
	}
	got := strings.TrimSpace(string(body))
	if got != "[]" && got != "null" {
		t.Errorf("expected empty array, got %s", body)
	}
}

func TestFirmware_Create_ViaDownloadURL_Struct(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	// Create via download_url (no file upload). Use multipart/form-data since
	// the handler reads r.FormValue.
	form := "--bnd\r\n" +
		`Content-Disposition: form-data; name="name"` + "\r\n\r\n" +
		"New Test Firmware\r\n" +
		"--bnd\r\n" +
		`Content-Disposition: form-data; name="build_version"` + "\r\n\r\n" +
		"V1.0.0-TEST\r\n" +
		"--bnd\r\n" +
		`Content-Disposition: form-data; name="vendor"` + "\r\n\r\n" +
		"TestVendor\r\n" +
		"--bnd\r\n" +
		`Content-Disposition: form-data; name="model"` + "\r\n\r\n" +
		"TestModel\r\n" +
		"--bnd\r\n" +
		`Content-Disposition: form-data; name="phase"` + "\r\n\r\n" +
		"release\r\n" +
		"--bnd\r\n" +
		`Content-Disposition: form-data; name="download_url"` + "\r\n\r\n" +
		"http://example.com/test.bin\r\n" +
		"--bnd--\r\n"

	status, body, err := client.DoWithContentType("POST", env.TenantURL("/firmware"), form, "multipart/form-data; boundary=bnd")
	if err != nil {
		t.Fatalf("POST /firmware: %v", err)
	}
	if status != 201 {
		t.Fatalf("want 201, got %d, body=%s", status, body)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("create response not JSON: %s", body)
	}
	if got["name"] != "New Test Firmware" {
		t.Errorf("name mismatch: got %v", got["name"])
	}
	if got["build_version"] != "V1.0.0-TEST" {
		t.Errorf("build_version mismatch: got %v", got["build_version"])
	}
}

func TestFirmware_Create_RejectsMissingName(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	form := "--bnd\r\n" +
		`Content-Disposition: form-data; name="build_version"` + "\r\n\r\n" +
		"V1.0.0\r\n" +
		"--bnd--\r\n"

	status, _, err := client.DoWithContentType("POST", env.TenantURL("/firmware"), form, "multipart/form-data; boundary=bnd")
	if err != nil {
		t.Fatalf("POST /firmware: %v", err)
	}
	if status != 400 {
		t.Errorf("missing name: want 400, got %d", status)
	}
}

func TestFirmware_Update_PhaseChanged(t *testing.T) {
	env := snapshot.Setup(t, []string{"firmware.json"})
	client := snapshot.NewClient(env.JWT)

	// Find one firmware ID via the list endpoint.
	status, body, err := client.Get(env.TenantURL("/firmware"))
	if err != nil {
		t.Fatalf("GET /firmware: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d", status)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("list not JSON array: %s", body)
	}
	if len(list) == 0 {
		t.Skip("no firmware in fixture")
	}

	// Pick the one that's currently "internal_testing" so we can flip to release.
	var id string
	for _, fw := range list {
		if fw["phase"] == "internal_testing" {
			idT, _ := fw["id"].(string)
			id = idT
			break
		}
	}
	if id == "" {
		id, _ = list[0]["id"].(string)
	}

	// Update the phase to release.
	phaseBody := `{"phase":"release"}`
	status, _, err = client.PutJSON(env.TenantURL("/firmware/"+id+"/phase"), phaseBody)
	if err != nil {
		t.Fatalf("PUT /firmware/%s/phase: %v", id, err)
	}
	if status != 204 {
		t.Errorf("set phase: want 204, got %d", status)
	}

	// Verify via re-list.
	status, body, err = client.Get(env.TenantURL("/firmware"))
	if err != nil {
		t.Fatalf("re-list: %v", err)
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("re-list not JSON: %s", body)
	}
	for _, fw := range list {
		if fw["id"] == id && fw["phase"] != "release" {
			t.Errorf("phase not updated in DB: got %v", fw["phase"])
		}
	}
}

func TestFirmware_Update_RejectsInvalidPhase(t *testing.T) {
	env := snapshot.Setup(t, []string{"firmware.json"})
	client := snapshot.NewClient(env.JWT)

	// Use a valid hex ObjectID (any will do; we expect 400 before DB lookup
	// if the phase is invalid, but the handler validates phase after ID parse).
	body := `{"phase":"bogus"}`
	status, _, err := client.PutJSON(env.TenantURL("/firmware/000000000000000000000000/phase"), body)
	if err != nil {
		t.Fatalf("PUT phase: %v", err)
	}
	if status != 400 {
		t.Errorf("invalid phase: want 400, got %d", status)
	}
}

func TestFirmware_Delete_RemovesRecord(t *testing.T) {
	env := snapshot.Setup(t, []string{"firmware.json"})
	client := snapshot.NewClient(env.JWT)

	// Get an ID.
	status, body, err := client.Get(env.TenantURL("/firmware"))
	if err != nil {
		t.Fatalf("GET /firmware: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(body, &list); err != nil || len(list) == 0 {
		t.Skip("no firmware to delete")
	}
	id, _ := list[0]["id"].(string)

	// Delete it.
	status, _, err = client.Delete(env.TenantURL("/firmware/" + id))
	if err != nil {
		t.Fatalf("DELETE /firmware/%s: %v", id, err)
	}
	if status != 204 {
		t.Errorf("delete: want 204, got %d", status)
	}

	// Verify via re-list.
	status, body, err = client.Get(env.TenantURL("/firmware"))
	if err != nil {
		t.Fatalf("re-list: %v", err)
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("re-list not JSON: %s", body)
	}
	for _, fw := range list {
		if fw["id"] == id {
			t.Errorf("deleted firmware still present in list")
		}
	}
}

func TestFirmware_Delete_NotFound(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, _, err := client.Delete(env.TenantURL("/firmware/000000000000000000000000"))
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	if status != 404 {
		t.Errorf("delete nonexistent: want 404, got %d", status)
	}
}

func TestFirmware_Routes_RequireAuth(t *testing.T) {
	env := snapshot.Setup(t, nil)
	unauthed := snapshot.NewClient("")

	for _, p := range []string{"/firmware"} {
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
