package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireMaxLevel(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	tests := []struct {
		name       string
		level      interface{}
		wantStatus int
	}{
		{name: "super admin", level: 0, wantStatus: http.StatusNoContent},
		{name: "tenant admin", level: 1, wantStatus: http.StatusNoContent},
		{name: "operator", level: 2, wantStatus: http.StatusForbidden},
		{name: "missing claims", level: nil, wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			if tt.level != nil {
				req = req.WithContext(context.WithValue(req.Context(), CtxLevel, tt.level))
			}
			res := httptest.NewRecorder()

			RequireMaxLevel(1)(next).ServeHTTP(res, req)

			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusForbidden {
				var body map[string]string
				if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
					t.Fatalf("forbidden response is not valid JSON: %q (%v)", res.Body.String(), err)
				}
				if body["error"] != "forbidden" {
					t.Fatalf("error = %q, want forbidden", body["error"])
				}
			}
		})
	}
}
