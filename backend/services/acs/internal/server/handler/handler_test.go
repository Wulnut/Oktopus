package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"oktopUSP/backend/services/acs/internal/config"
	"oktopUSP/backend/services/acs/internal/cwmp"

	"github.com/nats-io/nats.go"
)

// newTestHandler creates a Handler with no-op pub/sub and zero-value config.
func newTestHandler() *Handler {
	pub := func(subject string, data []byte) error { return nil }
	sub := func(subject string, cb func(*nats.Msg)) error { return nil }
	return NewHandler(pub, sub, config.Acs{})
}

// Test_CWMPHandler_MalformedXML_ReturnsError sends invalid XML and verifies the
// handler does not silently succeed with a 200 containing meaningful data.
// EXPECTED TO FAIL: current code ignores xml.Unmarshal errors.
func Test_CWMPHandler_MalformedXML_ReturnsError(t *testing.T) {
	h := newTestHandler()

	body := `<this is not valid xml!!! <<>>`
	req := httptest.NewRequest(http.MethodPost, "/acs", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.CwmpHandler(rec, req)

	// With malformed XML, the handler should not produce a 200 with an InformResponse.
	// The current code silently ignores the unmarshal error and falls through,
	// eventually writing to h.Cpes with an empty serial number key.
	if rec.Code == http.StatusOK && rec.Body.Len() > 0 {
		t.Errorf("Expected non-200 or empty body for malformed XML, got status %d with body length %d",
			rec.Code, rec.Body.Len())
	}

	// Additionally, the handler should not create a CPE entry for an empty serial number.
	if _, exists := h.Cpes[""]; exists {
		t.Error("Handler created a CPE entry with empty serial number from malformed XML")
	}
}

// Test_CWMPHandler_ConcurrentSessions_RaceDetected exercises the handler from
// multiple goroutines with different device serial numbers. Run with -race.
// EXPECTED TO FAIL: h.Cpes map is accessed without synchronization.
func Test_CWMPHandler_ConcurrentSessions_RaceDetected(t *testing.T) {
	h := newTestHandler()

	const numGoroutines = 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			sn := fmt.Sprintf("DEVICE%04d", idx)
			informXML := cwmp.Inform(sn)

			req := httptest.NewRequest(http.MethodPost, "/acs", strings.NewReader(informXML))
			rec := httptest.NewRecorder()

			h.CwmpHandler(rec, req)
		}(i)
	}

	wg.Wait()

	// If we get here without the race detector complaining, that's surprising.
	// Under -race, this test should report data races on h.Cpes.
	if len(h.Cpes) != numGoroutines {
		t.Logf("Expected %d CPEs registered, got %d (possible race condition lost writes)", numGoroutines, len(h.Cpes))
	}
}

// Test_CWMPHandler_OversizedBody_Rejected sends a 10MB body and checks that the
// handler doesn't blindly consume it all via ioutil.ReadAll.
// EXPECTED TO FAIL: current code uses ioutil.ReadAll with no size limit.
func Test_CWMPHandler_OversizedBody_Rejected(t *testing.T) {
	h := newTestHandler()

	// Generate a ~10MB XML-like body.
	const size = 10 * 1024 * 1024 // 10 MB
	bigBody := "<data>" + strings.Repeat("A", size) + "</data>"

	req := httptest.NewRequest(http.MethodPost, "/acs", strings.NewReader(bigBody))
	rec := httptest.NewRecorder()

	h.CwmpHandler(rec, req)

	// The handler should reject oversized bodies with an appropriate error status
	// (e.g., 413 Request Entity Too Large) rather than processing them.
	// Current code reads the entire body into memory without limit.
	if rec.Code == http.StatusOK || rec.Code == http.StatusNoContent {
		t.Errorf("Expected rejection of oversized body (10MB), but handler returned status %d; "+
			"ioutil.ReadAll has no size limit, risking memory exhaustion", rec.Code)
	}
}

// Test_CWMPHandler_ValidInform_ProcessesSuccessfully sends a valid CWMP Inform
// XML and verifies the response contains an InformResponse.
func Test_CWMPHandler_ValidInform_ProcessesSuccessfully(t *testing.T) {
	var publishedSubject string
	var publishedData []byte

	pub := func(subject string, data []byte) error {
		publishedSubject = subject
		publishedData = data
		return nil
	}
	sub := func(subject string, cb func(*nats.Msg)) error { return nil }
	h := NewHandler(pub, sub, config.Acs{
		DeviceAnswerTimeout: 10 * time.Second,
	})

	sn := "TEST123456"
	informXML := cwmp.Inform(sn)

	req := httptest.NewRequest(http.MethodPost, "/acs", strings.NewReader(informXML))
	rec := httptest.NewRecorder()

	h.CwmpHandler(rec, req)

	// Verify HTTP status is 200.
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	// Verify response body contains InformResponse.
	respBody := rec.Body.String()
	if !strings.Contains(respBody, "InformResponse") {
		t.Errorf("Response body does not contain InformResponse:\n%s", respBody)
	}

	// Verify a cookie was set with the device serial number.
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "oktopus" && c.Value == sn {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'oktopus' cookie with device serial number, but not found")
	}

	// Verify the CPE was registered in the handler's map.
	cpe, exists := h.Cpes[sn]
	if !exists {
		t.Fatal("CPE was not registered in h.Cpes after Inform")
	}
	if cpe.SerialNumber != sn {
		t.Errorf("CPE serial number = %q, want %q", cpe.SerialNumber, sn)
	}
	if cpe.OUI != "0013C8" {
		t.Errorf("CPE OUI = %q, want %q", cpe.OUI, "0013C8")
	}
	if cpe.DataModel != "TR098" {
		t.Errorf("CPE DataModel = %q, want %q", cpe.DataModel, "TR098")
	}

	// Verify NATS publish was called with the correct subject.
	expectedSubject := NATS_CWMP_SUBJECT_PREFIX + DEFAULT_TENANT + "." + sn + ".info"
	if publishedSubject != expectedSubject {
		t.Errorf("Published to subject %q, want %q", publishedSubject, expectedSubject)
	}
	if len(publishedData) == 0 {
		t.Error("Published data was empty")
	}
}
