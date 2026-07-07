//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/api/auth"
	"github.com/leandrofars/oktopus/internal/api/middleware"
	"github.com/leandrofars/oktopus/internal/bridge"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var testApi Api
var testRouter *mux.Router
var testTenantDB *db.TenantDB

func TestMain(m *testing.M) {
	mongoURI := os.Getenv("MONGO_TEST_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27018"
	}
	natsURL := os.Getenv("NATS_TEST_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4223"
	}

	ctx := context.Background()
	d := db.NewDatabase(ctx, mongoURI)

	if err := d.ProvisionTenantDBs(ctx, "test"); err != nil {
		fmt.Printf("Failed to provision tenant DBs: %v\n", err)
		os.Exit(1)
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		fmt.Printf("Cannot connect to NATS at %s: %v\n", natsURL, err)
		os.Exit(1)
	}
	js, _ := jetstream.New(nc)
	b := bridge.NewBridge(js, nc)

	testApi = Api{
		port:   "0",
		js:     js,
		nc:     nc,
		bridge: b,
		db:     d,
		ctx:    ctx,
	}

	testTenantDB = d.ForTenant("test")

	testRouter = mux.NewRouter()
	setupTestRoutes(testRouter)

	code := m.Run()

	// Cleanup: drop test databases
	cleanupClient, cleanupErr := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if cleanupErr == nil {
		cleanupClient.Database("account-mngr").Drop(ctx)
		cleanupClient.Database("tenant_test_general").Drop(ctx)
		cleanupClient.Database("tenant_test_usp").Drop(ctx)
		cleanupClient.Disconnect(ctx)
	}

	nc.Close()
	os.Exit(code)
}

func setupTestRoutes(r *mux.Router) {
	authRouter := r.PathPrefix("/api/auth").Subrouter()
	authRouter.HandleFunc("/register", testApi.registerUser).Methods("POST")
	authRouter.HandleFunc("/login", testApi.generateToken).Methods("PUT")
	authRouter.HandleFunc("/admin/register", testApi.registerAdminUser).Methods("POST")

	firmware := r.PathPrefix("/api/firmware").Subrouter()
	firmware.HandleFunc("", testApi.listFirmware).Methods("GET")
	firmware.HandleFunc("", testApi.uploadFirmware).Methods("POST")

	scripts := r.PathPrefix("/api/scripts").Subrouter()
	scripts.HandleFunc("", testApi.listScripts).Methods("GET")
	scripts.HandleFunc("", testApi.createScript).Methods("POST")

	campaigns := r.PathPrefix("/api/campaigns").Subrouter()
	campaigns.HandleFunc("", testApi.listCampaigns).Methods("GET")
	campaigns.HandleFunc("", testApi.createCampaign).Methods("POST")

	devices := r.PathPrefix("/api/device").Subrouter()
	devices.HandleFunc("", testApi.retrieveDevices).Methods("GET")

	// Apply middleware to protected routes
	firmware.Use(middleware.AuthMiddleware)
	scripts.Use(middleware.AuthMiddleware)
	campaigns.Use(middleware.AuthMiddleware)
	devices.Use(middleware.AuthMiddleware)
}

func validToken() string {
	token, _ := auth.GenerateJWT("test@test.com", "testuser", "", "test", 0)
	return token
}

func expiredToken() string {
	// GenerateJWT creates 24h tokens -- we can't easily make expired ones
	// without modifying auth package. Use an obviously invalid token instead.
	return "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VybmFtZSI6InRlc3QiLCJlbWFpbCI6InRlc3RAdGVzdC5jb20iLCJleHAiOjEwMDAwMDAwMDB9.invalid"
}

// --- Auth: Protected endpoints reject without token ---

func TestProtectedEndpoints_Reject401_WithoutToken(t *testing.T) {
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/device"},
		{"GET", "/api/firmware"},
		{"GET", "/api/scripts"},
		{"GET", "/api/campaigns"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			testRouter.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s without token: expected 401, got %d", ep.method, ep.path, w.Code)
			}
		})
	}
}

func TestProtectedEndpoints_Reject401_WithInvalidToken(t *testing.T) {
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/device"},
		{"GET", "/api/firmware"},
		{"GET", "/api/scripts"},
		{"GET", "/api/campaigns"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer garbage-token")
			w := httptest.NewRecorder()
			testRouter.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s with invalid token: expected 401, got %d", ep.method, ep.path, w.Code)
			}
		})
	}
}

func TestProtectedEndpoints_Accept_WithValidToken(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/firmware", nil)
	req.Header.Set("Authorization", validToken())
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)
	// Should not be 401 -- may be 200 or other status but not unauthorized
	if w.Code == http.StatusUnauthorized {
		t.Error("Valid token was rejected")
	}
}

// --- Password validation ---

func TestRegisterAdminUser_RejectsEmptyPassword(t *testing.T) {
	body := `{"email":"admin-empty@test.com","password":""}`
	req := httptest.NewRequest("POST", "/api/auth/admin/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)
	if w.Code == http.StatusOK || w.Code == http.StatusCreated || w.Code == http.StatusNoContent {
		t.Errorf("BUG: Empty password accepted on admin register (status %d) -- should be rejected", w.Code)
	}
}

func TestRegisterAdminUser_RejectsShortPassword(t *testing.T) {
	body := `{"email":"admin-short@test.com","password":"abc"}`
	req := httptest.NewRequest("POST", "/api/auth/admin/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)
	// After the first admin is created, subsequent calls return 403.
	// If first admin, it should reject short password with 400.
	if w.Code == http.StatusOK || w.Code == http.StatusCreated || w.Code == http.StatusNoContent {
		t.Errorf("BUG: Short password 'abc' accepted on admin register (status %d) -- no password validation", w.Code)
	}
}

// --- Body size limits ---

func TestCreateScript_RejectsOversizedBody(t *testing.T) {
	// createScript already uses MaxBytesReader -- this should pass
	bigBody := strings.Repeat("x", 2*1024*1024) // 2MB
	req := httptest.NewRequest("POST", "/api/scripts", strings.NewReader(bigBody))
	req.Header.Set("Authorization", validToken())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)
	if w.Code == http.StatusOK || w.Code == http.StatusCreated {
		t.Errorf("2MB body accepted on POST /api/scripts (status %d) -- should be rejected", w.Code)
	}
}

func TestCreateCampaign_RejectsOversizedBody(t *testing.T) {
	bigBody := strings.Repeat("x", 2*1024*1024) // 2MB
	req := httptest.NewRequest("POST", "/api/campaigns", strings.NewReader(bigBody))
	req.Header.Set("Authorization", validToken())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)
	if w.Code == http.StatusOK || w.Code == http.StatusCreated {
		t.Errorf("BUG: 2MB body accepted on POST /api/campaigns (status %d) -- no MaxBytesReader", w.Code)
	}
}

// --- Campaign validation ---

func TestCreateCampaign_ValidInput_Returns201(t *testing.T) {
	// Create firmware first
	fw := db.Firmware{
		Name:         fmt.Sprintf("fw-campaign-test-%d", time.Now().UnixNano()),
		Vendor:       "V",
		Model:        "M",
		BuildVersion: "1.0",
		Phase:        db.PhaseRelease,
	}
	created, err := testTenantDB.CreateFirmware(context.Background(), fw)
	if err != nil {
		t.Fatal(err)
	}

	campaign := map[string]interface{}{
		"vendor":      "V",
		"model":       "M",
		"hw_version":  fmt.Sprintf("hw-%d", time.Now().UnixNano()),
		"firmware_id": created.ID.Hex(),
		"enabled":     false,
		"concurrency": 10,
	}
	body, _ := json.Marshal(campaign)
	req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
	req.Header.Set("Authorization", validToken())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Errorf("Expected 201/200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateCampaign_NonexistentFirmware_Returns400(t *testing.T) {
	campaign := map[string]interface{}{
		"vendor":      "V",
		"model":       "M",
		"hw_version":  "hw-nonexistent",
		"firmware_id": "000000000000000000000099",
		"enabled":     false,
		"concurrency": 10,
	}
	body, _ := json.Marshal(campaign)
	req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
	req.Header.Set("Authorization", validToken())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for nonexistent firmware, got %d", w.Code)
	}
}

// --- Login ---

func TestLogin_ValidCredentials_ReturnsToken(t *testing.T) {
	// Register a user first
	email := fmt.Sprintf("login-test-%d@test.com", time.Now().UnixNano())
	regBody := fmt.Sprintf(`{"email":"%s","password":"testpassword123"}`, email)
	req := httptest.NewRequest("POST", "/api/auth/admin/register", strings.NewReader(regBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)

	// Login
	loginBody := fmt.Sprintf(`{"email":"%s","password":"testpassword123"}`, email)
	req = httptest.NewRequest("PUT", "/api/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Login expected 200, got %d: %s", w.Code, w.Body.String())
		return
	}
	if !strings.Contains(w.Body.String(), "token") {
		t.Error("Login response does not contain token")
	}
}

func TestLogin_WrongPassword_RejectsLogin(t *testing.T) {
	loginBody := `{"email":"nonexistent@test.com","password":"wrongpassword"}`
	req := httptest.NewRequest("PUT", "/api/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		t.Error("Login with wrong password should not return 200")
	}
}
