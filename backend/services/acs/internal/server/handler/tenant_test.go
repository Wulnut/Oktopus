package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"oktopUSP/backend/services/acs/internal/config"
	"oktopUSP/backend/services/acs/internal/cwmp"

	"github.com/nats-io/nats.go"
)

func TestParseTenantSlug(t *testing.T) {
	tests := []struct {
		path   string
		route  string
		tenant string
	}{
		{"/acs", "/acs", DEFAULT_TENANT},
		{"/acs/", "/acs", DEFAULT_TENANT},
		{"/acs/sei", "/acs", "sei"},
		{"/acs/sei/", "/acs", "sei"},
		{"/acs/sei/extra", "/acs", "sei"},
	}

	for _, tt := range tests {
		got := ParseTenantSlug(tt.path, tt.route)
		if got != tt.tenant {
			t.Errorf("ParseTenantSlug(%q, %q) = %q, want %q", tt.path, tt.route, got, tt.tenant)
		}
	}
}

func Test_CWMPHandler_Inform_TenantSubject(t *testing.T) {
	var publishedSubject string

	pub := func(subject string, data []byte) error {
		publishedSubject = subject
		return nil
	}
	sub := func(subject string, cb func(*nats.Msg)) error { return nil }
	h := NewHandler(pub, sub, config.Acs{
		Route:               "/acs",
		DeviceAnswerTimeout: 10 * time.Second,
	})

	sn := "TEST-TENANT-SN"
	informXML := cwmp.Inform(sn)

	req := httptest.NewRequest(http.MethodPost, "/acs/sei/", strings.NewReader(informXML))
	rec := httptest.NewRecorder()

	h.CwmpHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	expectedSubject := cwmpInfoSubject("sei", sn)
	if publishedSubject != expectedSubject {
		t.Errorf("Published to subject %q, want %q", publishedSubject, expectedSubject)
	}

	cpe, exists := h.Cpes[sn]
	if !exists {
		t.Fatal("CPE was not registered after Inform")
	}
	if cpe.TenantSlug != "sei" {
		t.Errorf("CPE TenantSlug = %q, want %q", cpe.TenantSlug, "sei")
	}
}
