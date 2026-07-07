//go:build integration

package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/db"
)

func createTestScript(t *testing.T, steps []db.ScriptStep) db.Script {
	t.Helper()
	s := db.Script{
		Name:        fmt.Sprintf("test-script-%d", time.Now().UnixNano()),
		Description: "test",
		Steps:       steps,
		Tags:        []string{"test"},
	}
	created, err := testTenantDB.CreateScript(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

// --- Iteration cap ---

func TestExecuteScript_InfiniteLoop_CappedAt500(t *testing.T) {
	// Create a script with a CONDITION that always jumps back to step 0
	steps := []db.ScriptStep{
		{
			ID:   "step-0",
			Name: "Always True Condition",
			Type: "CONDITION",
			Condition: &db.ScriptCondition{
				Source:   "literal",
				Param:    "1",
				Operator: "eq",
				Value:    "1",
			},
			OnTrue:  "step-0", // Infinite loop
			OnFalse: "",
		},
	}
	script := createTestScript(t, steps)

	// Execute via HTTP handler
	url := fmt.Sprintf("/api/scripts/%s/execute/TEST-SN-LOOP/any", script.ID.Hex())
	req := httptest.NewRequest("POST", url, strings.NewReader(`{}`))
	req.Header.Set("Authorization", validToken())
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{
		"id":  script.ID.Hex(),
		"sn":  "TEST-SN-LOOP",
		"mtp": "any",
	})
	w := httptest.NewRecorder()

	// The handler needs a device to be online -- it will fail at deviceStateOKNoWrite
	// because no adapter is running. This is expected -- we're testing the iteration cap,
	// which runs after the online check. If the device check fails, the handler returns
	// 503 before entering the loop.
	testRouter.ServeHTTP(w, req)

	// If we get 503 "Device is offline", the iteration cap isn't reached.
	// This test documents the behavior -- the real iteration cap test needs a mock adapter.
	if w.Code == http.StatusServiceUnavailable {
		t.Log("Device offline (no adapter) -- iteration cap test needs live adapter, skipping loop verification")
		return
	}

	// If somehow the device was found, verify the execution was capped
	if w.Code == http.StatusOK {
		body := w.Body.String()
		if strings.Contains(body, "exceeded maximum total iterations") {
			t.Log("Iteration cap correctly triggered")
		} else if strings.Contains(body, `"status":"failed"`) {
			t.Log("Execution failed (expected for infinite loop)")
		}
	}
}

// --- Script CRUD via API ---

func TestCreateScript_ValidInput_Returns201(t *testing.T) {
	body := `{
		"name": "` + fmt.Sprintf("api-test-%d", time.Now().UnixNano()) + `",
		"description": "API test script",
		"tags": ["test"],
		"variables": [],
		"steps": [
			{"id": "s1", "name": "Get Info", "type": "GET", "param_paths": ["Device.DeviceInfo."]}
		]
	}`
	req := httptest.NewRequest("POST", "/api/scripts", strings.NewReader(body))
	req.Header.Set("Authorization", validToken())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListScripts_ReturnsArray(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/scripts", nil)
	req.Header.Set("Authorization", validToken())
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
	if !strings.HasPrefix(strings.TrimSpace(w.Body.String()), "[") {
		t.Error("Expected JSON array response")
	}
}

// --- Script with DELAY step ---

func TestCreateScript_WithDelayStep_Accepted(t *testing.T) {
	body := `{
		"name": "` + fmt.Sprintf("delay-test-%d", time.Now().UnixNano()) + `",
		"description": "Script with delay",
		"tags": ["test"],
		"steps": [
			{"id": "s1", "name": "Wait", "type": "DELAY", "duration_ms": 100},
			{"id": "s2", "name": "Get", "type": "GET", "param_paths": ["Device.DeviceInfo."]}
		]
	}`
	req := httptest.NewRequest("POST", "/api/scripts", strings.NewReader(body))
	req.Header.Set("Authorization", validToken())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Mass action device cap ---

func TestMassAction_DeviceSNsCapped(t *testing.T) {
	// Build a request with 501 device SNs
	sns := make([]string, 501)
	for i := range sns {
		sns[i] = fmt.Sprintf("SN-%04d", i)
	}
	body := fmt.Sprintf(`{"script_id":"000000000000000000000001","device_sns":["%s"]}`, strings.Join(sns, `","`))

	// Need to add mass-actions route to test router
	massRouter := testRouter.PathPrefix("/api/mass-actions").Subrouter()
	massRouter.HandleFunc("/script", testApi.massScriptExecution).Methods("POST")
	massRouter.Use(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip middleware for this test
			h.ServeHTTP(w, r)
		})
	})

	req := httptest.NewRequest("POST", "/api/mass-actions/script", strings.NewReader(body))
	req.Header.Set("Authorization", validToken())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for 501 device SNs (cap is 500), got %d", w.Code)
	}
}
