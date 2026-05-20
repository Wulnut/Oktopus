package handler

import (
	"encoding/xml"
	"oktopUSP/backend/services/acs/internal/cwmp"
	"strings"
	"testing"
	"time"

	"github.com/oleiade/lane"
)

func TestGenerateConnReqCreds_FormatAndUniqueness(t *testing.T) {
	now := time.Now()
	a, err := generateConnReqCreds(now)
	if err != nil {
		t.Fatalf("generateConnReqCreds: %v", err)
	}

	if !strings.HasPrefix(a.Username, "acs_") {
		t.Errorf("Username = %q, want prefix %q", a.Username, "acs_")
	}
	// "acs_" + 8 hex chars = 12 total. Keeps under the 32-char limit some
	// CPE vendors apply to provisioning-time parameters.
	if got, want := len(a.Username), 12; got != want {
		t.Errorf("Username length = %d, want %d", got, want)
	}
	if got, want := len(a.Password), 32; got != want {
		t.Errorf("Password length = %d, want %d", got, want)
	}
	if a.ProvisionedAt != now.Unix() {
		t.Errorf("ProvisionedAt = %d, want %d", a.ProvisionedAt, now.Unix())
	}

	b, err := generateConnReqCreds(now)
	if err != nil {
		t.Fatalf("second generateConnReqCreds: %v", err)
	}
	if a.Username == b.Username || a.Password == b.Password {
		t.Errorf("expected distinct creds, got %+v vs %+v", a, b)
	}
}

func TestConnReqParamNames(t *testing.T) {
	tests := []struct {
		dataModel string
		wantUser  string
		wantPass  string
	}{
		{"TR181", connReqUsernameTR181, connReqPasswordTR181},
		{"TR098", connReqUsernameTR098, connReqPasswordTR098},
		{"", connReqUsernameTR181, connReqPasswordTR181},
		{"unknown", connReqUsernameTR181, connReqPasswordTR181},
	}
	for _, tt := range tests {
		gotUser, gotPass := connReqParamNames(tt.dataModel)
		if gotUser != tt.wantUser || gotPass != tt.wantPass {
			t.Errorf("connReqParamNames(%q) = (%q, %q), want (%q, %q)",
				tt.dataModel, gotUser, gotPass, tt.wantUser, tt.wantPass)
		}
	}
}

func TestEnqueueProvisionSetParams_TR181(t *testing.T) {
	h := newTestHandler()
	cpe := CPE{
		SerialNumber: "TEST-181",
		DataModel:    "TR181",
		Queue:        lane.NewQueue(),
	}

	creds := connReqCreds{Username: "acs_deadbeef", Password: "01234567890abcdef01234567890abcde", ProvisionedAt: time.Now().Unix()}
	if err := h.enqueueProvisionSetParams("sei", "TEST-181", &cpe, creds); err != nil {
		t.Fatalf("enqueueProvisionSetParams: %v", err)
	}

	if got := cpe.Queue.Size(); got != 1 {
		t.Fatalf("queue size = %d, want 1", got)
	}

	raw := cpe.Queue.Dequeue().(Request)
	body := string(raw.CwmpMsg)
	// Verify the XML enqueued addresses the TR-181 parameter paths and
	// carries the credentials we plan to commit on success.
	if !strings.Contains(body, connReqUsernameTR181) {
		t.Errorf("payload missing %q:\n%s", connReqUsernameTR181, body)
	}
	if !strings.Contains(body, connReqPasswordTR181) {
		t.Errorf("payload missing %q:\n%s", connReqPasswordTR181, body)
	}
	if !strings.Contains(body, creds.Username) {
		t.Errorf("payload missing username %q:\n%s", creds.Username, body)
	}
	if !strings.Contains(body, creds.Password) {
		t.Errorf("payload missing password %q:\n%s", creds.Password, body)
	}
	// Pre-existing string(int) bug used to emit a single non-printable rune
	// in place of the ParameterValueStruct array size — verify the fix.
	if !strings.Contains(body, `ParameterValueStruct[2]`) {
		t.Errorf("expected arrayType length 2 in payload:\n%s", body)
	}
}

func TestEnqueueProvisionSetParams_TR098(t *testing.T) {
	h := newTestHandler()
	cpe := CPE{
		SerialNumber: "TEST-098",
		DataModel:    "TR098",
		Queue:        lane.NewQueue(),
	}

	creds, err := generateConnReqCreds(time.Now())
	if err != nil {
		t.Fatalf("generateConnReqCreds: %v", err)
	}
	if err := h.enqueueProvisionSetParams("sei", "TEST-098", &cpe, creds); err != nil {
		t.Fatalf("enqueueProvisionSetParams: %v", err)
	}
	raw := cpe.Queue.Dequeue().(Request)
	body := string(raw.CwmpMsg)
	if !strings.Contains(body, connReqUsernameTR098) {
		t.Errorf("TR-098 payload missing %q:\n%s", connReqUsernameTR098, body)
	}
}

func TestEnqueueProvisionSetParams_NilQueueRejected(t *testing.T) {
	h := newTestHandler()
	cpe := CPE{SerialNumber: "TEST"}
	err := h.enqueueProvisionSetParams("sei", "TEST", &cpe, connReqCreds{})
	if err == nil {
		t.Fatal("expected error when CPE queue is nil")
	}
}

func TestProvisioningResponseAccepted(t *testing.T) {
	// Synthesize a real SetParameterValuesResponse envelope using the
	// existing XML helper to stay close to what a CPE actually sends.
	okResp := `<?xml version="1.0"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/" xmlns:cwmp="urn:dslforum-org:cwmp-1-0">
  <soap:Header><cwmp:ID>42</cwmp:ID></soap:Header>
  <soap:Body><cwmp:SetParameterValuesResponse><Status>0</Status></cwmp:SetParameterValuesResponse></soap:Body>
</soap:Envelope>`
	accepted, kind, err := provisioningResponseAccepted([]byte(okResp))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if !accepted {
		t.Errorf("expected accepted=true for SetParameterValuesResponse, got false (kind=%q)", kind)
	}

	faultResp := `<?xml version="1.0"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/" xmlns:cwmp="urn:dslforum-org:cwmp-1-0">
  <soap:Header/>
  <soap:Body><soap:Fault><cwmp:Fault><FaultCode>9001</FaultCode><FaultString>nope</FaultString></cwmp:Fault></soap:Fault></soap:Body>
</soap:Envelope>`
	accepted, kind, err = provisioningResponseAccepted([]byte(faultResp))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if accepted {
		t.Errorf("expected accepted=false for Fault, got true (kind=%q)", kind)
	}

	accepted, _, err = provisioningResponseAccepted([]byte("not xml"))
	if accepted {
		t.Errorf("expected accepted=false for garbage, got true")
	}
	if err == nil {
		t.Errorf("expected parse error for garbage input")
	}
}

func TestEnsureConnReqCreds_NoJetStreamIsSafe(t *testing.T) {
	// Without a JetStream client, provisioning must short-circuit so the ACS
	// falls back to env-based credentials and never tries to dereference nil.
	h := newTestHandler()
	cpe := CPE{SerialNumber: "TEST", Queue: lane.NewQueue()}
	u, p, ok := h.ensureConnReqCreds("sei", "TEST", &cpe)
	if ok {
		t.Errorf("ensureConnReqCreds returned ok=true with nil JS (u=%q p=%q)", u, p)
	}
	if got := cpe.Queue.Size(); got != 0 {
		t.Errorf("queue size = %d, want 0 (no provisioning when JS disabled)", got)
	}
}

// Belt-and-suspenders: confirm the XML helper itself emits a well-formed
// ParameterList array size. This guards the pre-existing string(int) bug fix.
func TestSetParameterMultiValuesArraySize(t *testing.T) {
	msg := cwmp.SetParameterMultiValues(map[string]string{
		"Device.A": "1",
		"Device.B": "2",
		"Device.C": "3",
	})
	if !strings.Contains(msg, `ParameterValueStruct[3]`) {
		t.Errorf("expected ParameterValueStruct[3] in XML, got:\n%s", msg)
	}
	// And the XML still parses cleanly.
	var env cwmp.SoapEnvelope
	if err := xml.Unmarshal([]byte(msg), &env); err != nil {
		t.Errorf("generated XML failed to parse: %v", err)
	}
}
