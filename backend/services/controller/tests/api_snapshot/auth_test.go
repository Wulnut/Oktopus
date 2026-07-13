package snapshot_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/leandrofars/oktopus/tests/api_snapshot/snapshot"
)

// Stage 2 — Auth + Tenant management black-box tests.
//
// These exercise the /api/auth/* and /api/tenants* routes through the real
// router over HTTP, with the test Mongo as the only dependency. No NATS-mediated
// adapter calls are involved, so they run in pure black-box mode.

// --- Auth: admin existence ---

func TestAuth_AdminExists_ReturnsFalseWhenNoUsers(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient("")

	status, body, err := client.Get(env.AdminURL("/api/auth/admin/exists"))
	if err != nil {
		t.Fatalf("GET /api/auth/admin/exists: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	var got bool
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("response is not a JSON bool: %s", body)
	}
	if got {
		t.Errorf("adminUserExists should be false when account-mngr has no users; got true. body=%s", body)
	}
}

// --- Auth: admin register ---

func TestAuth_RegisterAdmin_FirstCallSucceeds(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient("")

	// Use a unique email per test run to avoid collisions with sibling tests
	// sharing the same Mongo container (Setup drops only tenant DBs, not
	// account-mngr.users).
	body := `{"email":"first-admin@test.example.com","password":"Sup3rSecret!","name":"First Admin"}`
	status, respBody, err := client.PostJSON(env.AdminURL("/api/auth/admin/register"), body)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// Handler returns 200 (default) on success — not 201. The handler does not
	// set an explicit success status, so net/http defaults to 200.
	if status != 200 && status != 201 {
		t.Fatalf("first admin register: want 200/201, got %d, body=%s", status, respBody)
	}
}

func TestAuth_RegisterAdmin_SecondCallRejects(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient("")

	// First registration succeeds.
	first := `{"email":"primary-admin@test.example.com","password":"Sup3rSecret!","name":"Primary"}`
	if status, _, err := client.PostJSON(env.AdminURL("/api/auth/admin/register"), first); err != nil || (status != 200 && status != 201) {
		t.Fatalf("first register failed: status=%d err=%v", status, err)
	}

	// Second registration without Authorization header: handler should detect
	// an admin already exists and refuse (403 / 401 — see registerAdminUser).
	second := `{"email":"second-admin@test.example.com","password":"Sup3rSecret!","name":"Second"}`
	status, respBody, err := client.PostJSON(env.AdminURL("/api/auth/admin/register"), second)
	if err != nil {
		t.Fatalf("second register: %v", err)
	}
	if status == 200 || status == 201 {
		t.Errorf("BUG: second admin registration without auth should be rejected; got %d, body=%s", status, respBody)
	}
}

func TestAuth_RegisterAdmin_RejectsShortPassword(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient("")

	// Password "abc" is below minPasswordLength (8).
	body := `{"email":"shortpw@test.example.com","password":"abc","name":"Short"}`
	status, respBody, err := client.PostJSON(env.AdminURL("/api/auth/admin/register"), body)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if status == 200 || status == 201 {
		t.Errorf("BUG: short password 'abc' accepted (status %d) -- minPasswordLength not enforced. body=%s", status, respBody)
	}
}

func TestAuth_RegisterAdmin_RejectsInvalidEmail(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient("")

	body := `{"email":"not-an-email","password":"Sup3rSecret!","name":"Bad Email"}`
	status, respBody, err := client.PostJSON(env.AdminURL("/api/auth/admin/register"), body)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if status == 200 || status == 201 {
		t.Errorf("BUG: invalid email accepted (status %d). body=%s", status, respBody)
	}
}

// --- Auth: login ---

func TestAuth_Login_ValidCredentialsReturnJWT(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient("")

	// Register the first admin so we have someone to log in as.
	reg := `{"email":"login-admin@test.example.com","password":"Sup3rSecret!","name":"Login Admin"}`
	if status, _, err := client.PostJSON(env.AdminURL("/api/auth/admin/register"), reg); err != nil || (status != 200 && status != 201) {
		t.Fatalf("admin register prerequisite failed: status=%d err=%v", status, err)
	}

	// Login with correct password.
	login := `{"email":"login-admin@test.example.com","password":"Sup3rSecret!"}`
	status, body, err := client.Do("PUT", env.AdminURL("/api/auth/login"), login)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if status != 200 {
		t.Fatalf("login with valid creds: want 200, got %d, body=%s", status, body)
	}

	// Handler returns the JWT as a bare JSON string, not a {token: ...} object.
	var tokenStr string
	if err := json.Unmarshal(body, &tokenStr); err != nil {
		t.Fatalf("login response not a JSON string: %s", body)
	}
	if tokenStr == "" {
		t.Errorf("login returned empty token. body=%s", body)
	}
	if !strings.HasPrefix(tokenStr, "eyJ") {
		t.Errorf("token does not look like a JWT: %s", tokenStr)
	}
}

func TestAuth_Login_WrongPasswordReturns401(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient("")

	reg := `{"email":"wrong-pw-admin@test.example.com","password":"Sup3rSecret!","name":"WP"}`
	if status, _, err := client.PostJSON(env.AdminURL("/api/auth/admin/register"), reg); err != nil || (status != 200 && status != 201) {
		t.Fatalf("admin register prerequisite failed: status=%d err=%v", status, err)
	}

	login := `{"email":"wrong-pw-admin@test.example.com","password":"WrongPassword!!!"}`
	status, body, err := client.Do("PUT", env.AdminURL("/api/auth/login"), login)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if status != 401 {
		t.Errorf("wrong password: want 401, got %d, body=%s", status, body)
	}
}

func TestAuth_Login_NonexistentUserReturns401(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient("")

	login := `{"email":"ghost@test.example.com","password":"Sup3rSecret!"}`
	status, body, err := client.Do("PUT", env.AdminURL("/api/auth/login"), login)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if status != 401 {
		t.Errorf("nonexistent user: want 401, got %d, body=%s", status, body)
	}
}

// --- Tenant management (SuperAdmin-only) ---

func TestTenants_List_RejectsNonSuperAdmin(t *testing.T) {
	env := snapshot.Setup(t, nil)
	// TenantAdmin client (level 1) from env.JWT.
	client := snapshot.NewClient(env.JWT)

	status, _, err := client.Get(env.AdminURL("/api/tenants"))
	if err != nil {
		t.Fatalf("GET /api/tenants: %v", err)
	}
	if status != 403 {
		t.Errorf("TenantAdmin should be forbidden from /api/tenants list: want 403, got %d", status)
	}
}

func TestTenants_List_Snapshot(t *testing.T) {
	env := snapshot.Setup(t, nil)
	// SuperAdmin client.
	client := snapshot.NewClient(env.AdminJWT)

	status, body, err := client.Get(env.AdminURL("/api/tenants"))
	if err != nil {
		t.Fatalf("GET /api/tenants: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	snapshot.CompareSnapshot(t, "tenants_list", body)
}

func TestTenants_Get_ReturnsSnapshot(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.AdminJWT)

	status, body, err := client.Get(env.AdminURL("/api/tenants/" + snapshot.SnapshotTenantSlug))
	if err != nil {
		t.Fatalf("GET /api/tenants/%s: %v", snapshot.SnapshotTenantSlug, err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	snapshot.CompareSnapshot(t, "tenant_get", body)
}

func TestTenants_Create_StructMatchesRequest(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.AdminJWT)

	// createTenant derives slug from name. Use a unique name to avoid collisions
	// with other tests in the same Mongo container.
	req := `{"name":"Created Test Tenant","admin_email":"created-admin@test.example.com","admin_name":"Created","admin_password":"Sup3rSecret!"}`
	status, body, err := client.PostJSON(env.AdminURL("/api/tenants"), req)
	if err != nil {
		t.Fatalf("POST /api/tenants: %v", err)
	}
	if status != 201 {
		t.Fatalf("want 201, got %d, body=%s", status, body)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("create response not JSON: %s", body)
	}
	// db.Tenant uses lowercase json tags (id/name/slug/status/...).
	if got["name"] != "Created Test Tenant" {
		t.Errorf("name mismatch: want 'Created Test Tenant', got %v", got["name"])
	}
	if got["slug"] != "created-test-tenant" {
		t.Errorf("slug mismatch: want 'created-test-tenant', got %v", got["slug"])
	}
	if got["status"] == nil {
		t.Errorf("status field missing in response")
	}
}

func TestTenants_Create_RejectsNonSuperAdmin(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT) // TenantAdmin

	req := `{"name":"Forbidden Tenant"}`
	status, _, err := client.PostJSON(env.AdminURL("/api/tenants"), req)
	if err != nil {
		t.Fatalf("POST /api/tenants: %v", err)
	}
	if status != 403 {
		t.Errorf("TenantAdmin should be forbidden from creating tenants: want 403, got %d", status)
	}
}

func TestTenants_Create_RejectsMissingName(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.AdminJWT)

	req := `{}`
	status, body, err := client.PostJSON(env.AdminURL("/api/tenants"), req)
	if err != nil {
		t.Fatalf("POST /api/tenants: %v", err)
	}
	if status != 400 {
		t.Errorf("missing name: want 400, got %d, body=%s", status, body)
	}
}
