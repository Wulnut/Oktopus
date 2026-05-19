package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

func TestJSONNotFoundHandler_ReturnsStructuredResponse(t *testing.T) {
	router := mux.NewRouter()
	setJSONNotFoundHandler(router)

	req := httptest.NewRequest(http.MethodPost, "/api/missing-route", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json content type, got %q", got)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}

	if body["error"] != "route not registered" {
		t.Fatalf("expected route-not-registered error, got %q", body["error"])
	}
	if body["path"] != "/api/missing-route" {
		t.Fatalf("expected request path in body, got %q", body["path"])
	}
	if body["method"] != http.MethodPost {
		t.Fatalf("expected request method in body, got %q", body["method"])
	}
}
