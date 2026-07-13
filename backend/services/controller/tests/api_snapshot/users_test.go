package snapshot_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/leandrofars/oktopus/tests/api_snapshot/snapshot"
)

// Stage 7b — Users black-box tests.
//
// retrieveUsers is pure-Mongo. registerUser hashes password (slow like admin
// register). deleteUser/changePassword are pure-Mongo.
//
// Note: these tests target the snapshot-test tenant, and the TenantAdmin JWT
// in env.JWT has tenant_slug="snapshot-test" (level 1), so it can manage
// users within its own tenant.

func TestUsers_List_Snapshot(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/users"))
	if err != nil {
		t.Fatalf("GET /users: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	// No users seeded in the snapshot tenant, expect empty list.
	snapshot.CompareSnapshot(t, "users_list_empty", body)
}

func TestUsers_Create_Struct(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	body := `{"email":"new-operator@test.example.com","name":"New Operator","password":"Sup3rSecret!","level":2}`
	status, respBody, err := client.PostJSON(env.TenantURL("/users"), body)
	if err != nil {
		t.Fatalf("POST /users: %v", err)
	}
	if status != 200 && status != 201 {
		t.Fatalf("create user: want 200/201, got %d, body=%s", status, respBody)
	}

	// Verify via list.
	status, listBody, err := client.Get(env.TenantURL("/users"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(listBody, &list); err != nil {
		t.Fatalf("list not JSON: %s", listBody)
	}
	found := false
	for _, u := range list {
		if u["email"] == "new-operator@test.example.com" {
			found = true
		}
	}
	if !found {
		t.Errorf("created user not in list. body=%s", listBody)
	}
}

func TestUsers_Create_RejectsOperatorLevel(t *testing.T) {
	// Operator (level 2) must NOT be allowed to create users — handler returns
	// 403 in the very first privilege check before touching the DB.
	env := snapshot.Setup(t, nil)
	op := snapshot.NewClient(env.OperatorJWT)

	body := `{"email":"should-not-exist@test.example.com","name":"X","password":"Sup3rSecret!","level":2}`
	status, respBody, err := op.PostJSON(env.TenantURL("/users"), body)
	if err != nil {
		t.Fatalf("POST /users as operator: %v", err)
	}
	if status != 403 {
		t.Fatalf("operator creating user: want 403, got %d, body=%s", status, respBody)
	}

	// Belt-and-suspenders: verify nothing was actually written.
	admin := snapshot.NewClient(env.JWT)
	_, listBody, _ := admin.Get(env.TenantURL("/users"))
	if strings.Contains(string(listBody), "should-not-exist@test.example.com") {
		t.Errorf("forbidden create leaked a row into users")
	}
}

func TestUsers_Delete(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	// Create a user to delete.
	create := `{"email":"to-delete@test.example.com","name":"ToDelete","password":"Sup3rSecret!","level":2}`
	if status, _, err := client.PostJSON(env.TenantURL("/users"), create); err != nil || (status != 200 && status != 201) {
		t.Fatalf("create prerequisite failed: status=%d err=%v", status, err)
	}

	status, _, err := client.Delete(env.TenantURL("/users/to-delete@test.example.com"))
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	if status != 204 && status != 200 {
		t.Errorf("delete: want 204/200, got %d", status)
	}

	// Verify gone.
	status, listBody, _ := client.Get(env.TenantURL("/users"))
	var list []map[string]interface{}
	_ = json.Unmarshal(listBody, &list)
	for _, u := range list {
		if u["email"] == "to-delete@test.example.com" {
			t.Errorf("deleted user still present")
		}
	}
}

func TestUsers_Delete_CannotDeleteSelf(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	// env.JWT is for tenant@test.example.com — try deleting self.
	status, _, err := client.Delete(env.TenantURL("/users/tenant@test.example.com"))
	if err != nil {
		t.Fatalf("DELETE self: %v", err)
	}
	if status != 403 {
		t.Errorf("delete self: want 403, got %d", status)
	}
}

func TestUsers_ChangePassword(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	// Create a user first.
	create := `{"email":"pw-change@test.example.com","name":"PW","password":"OldPassw0rd!","level":2}`
	if status, _, err := client.PostJSON(env.TenantURL("/users"), create); err != nil || (status != 200 && status != 201) {
		t.Fatalf("create prerequisite failed: status=%d err=%v", status, err)
	}

	// Change password as that user (needs their own JWT, but the handler accepts
	// any authenticated user changing their own password). Use the admin JWT.
	body := `{"email":"pw-change@test.example.com","password":"NewPassw0rd!!!"}`
	status, respBody, err := client.PutJSON(env.TenantURL("/users/password/pw-change@test.example.com"), body)
	if err != nil {
		t.Fatalf("PUT password: %v", err)
	}
	if status != 200 && status != 204 {
		t.Errorf("change password: want 200/204, got %d, body=%s", status, respBody)
	}
}

func TestUsers_Routes_RequireAuth(t *testing.T) {
	env := snapshot.Setup(t, nil)
	unauthed := snapshot.NewClient("")

	status, body, err := unauthed.Get(env.TenantURL("/users"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if status != 401 {
		t.Errorf("without token: want 401, got %d, body=%s", status, body)
	}
}

func TestUsers_List_ResponseShape(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	// Seed one user, then verify the list has expected fields.
	create := `{"email":"shape-check@test.example.com","name":"Shape","password":"Sup3rSecret!","level":2}`
	if status, _, err := client.PostJSON(env.TenantURL("/users"), create); err != nil || (status != 200 && status != 201) {
		t.Fatalf("create prerequisite failed: status=%d err=%v", status, err)
	}

	status, body, err := client.Get(env.TenantURL("/users"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("list not JSON: %s", body)
	}
	if len(list) == 0 {
		t.Fatalf("expected at least one user")
	}
	// Check that the password field is not leaked.
	for _, u := range list {
		if pwd, exists := u["password"]; exists && pwd != "" {
			t.Errorf("password field leaked in user list: %v", pwd)
		}
		if _, exists := u["email"]; !exists {
			t.Errorf("user object missing email field")
		}
	}
	_ = status

	// Confirm the response does not contain raw password strings.
	if strings.Contains(string(body), "Sup3rSecret!") {
		t.Errorf("plaintext password found in response body")
	}
}
