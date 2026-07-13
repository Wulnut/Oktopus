package snapshot_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/leandrofars/oktopus/tests/api_snapshot/snapshot"
)

// Stage 7c — CA Certs black-box tests.
//
// CA certs are stored as an array on the Tenant document in account-mngr
// (db.Tenant.CACerts). All routes are pure-Mongo (after the PEM parse in
// addCACert). We generate a fresh self-signed cert per test to avoid
// committing a long-lived test key.

// generateTestPEM returns a freshly-minted self-signed cert in PEM form.
func generateTestPEM(t *testing.T) string {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "snapshot-test-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return string(pemBytes)
}

func TestCACerts_List_EmptySnapshot(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	status, body, err := client.Get(env.TenantURL("/ca-certs"))
	if err != nil {
		t.Fatalf("GET /ca-certs: %v", err)
	}
	if status != 200 {
		t.Fatalf("want 200, got %d, body=%s", status, body)
	}
	snapshot.CompareSnapshot(t, "ca_certs_empty", body)
}

func TestCACerts_Add_Struct(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	pemStr := generateTestPEM(t)
	// Strip trailing newline for JSON embedding.
	pemClean := strings.TrimRight(pemStr, "\n")
	body := `{"label":"test-ca","pem":` + jsonString(pemClean) + `}`

	status, respBody, err := client.PostJSON(env.TenantURL("/ca-certs"), body)
	if err != nil {
		t.Fatalf("POST /ca-certs: %v", err)
	}
	if status != 201 {
		t.Fatalf("add CA cert: want 201, got %d, body=%s", status, respBody)
	}

	// Verify via list.
	status, listBody, err := client.Get(env.TenantURL("/ca-certs"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(string(listBody), "test-ca") {
		t.Errorf("added CA cert not in list. body=%s", listBody)
	}
}

func TestCACerts_Add_RejectsInvalidPEM(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	body := `{"label":"bad","pem":"not a pem"}`
	status, _, err := client.PostJSON(env.TenantURL("/ca-certs"), body)
	if err != nil {
		t.Fatalf("POST /ca-certs: %v", err)
	}
	if status != 400 {
		t.Errorf("invalid PEM: want 400, got %d", status)
	}
}

func TestCACerts_Add_RejectsMissingPEM(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	body := `{"label":"no-pem"}`
	status, _, err := client.PostJSON(env.TenantURL("/ca-certs"), body)
	if err != nil {
		t.Fatalf("POST /ca-certs: %v", err)
	}
	if status != 400 {
		t.Errorf("missing PEM: want 400, got %d", status)
	}
}

func TestCACerts_Delete(t *testing.T) {
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	// Add a cert.
	pemStr := generateTestPEM(t)
	pemClean := strings.TrimRight(pemStr, "\n")
	addBody := `{"label":"to-delete-ca","pem":` + jsonString(pemClean) + `}`
	if status, _, err := client.PostJSON(env.TenantURL("/ca-certs"), addBody); err != nil || status != 201 {
		t.Fatalf("add prerequisite failed: status=%d err=%v", status, err)
	}

	// Get the cert ID from list.
	status, listBody, err := client.Get(env.TenantURL("/ca-certs"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var certs []map[string]interface{}
	if err := json.Unmarshal(listBody, &certs); err != nil || len(certs) == 0 {
		t.Fatalf("no certs in list: %s", listBody)
	}
	id, _ := certs[0]["id"].(string)

	// Delete it.
	status, _, err = client.Delete(env.TenantURL("/ca-certs/" + id))
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	if status != 204 && status != 200 {
		t.Errorf("delete: want 204/200, got %d", status)
	}
}

func TestCACerts_Routes_RequireAuth(t *testing.T) {
	env := snapshot.Setup(t, nil)
	unauthed := snapshot.NewClient("")

	status, _, err := unauthed.Get(env.TenantURL("/ca-certs"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if status != 401 {
		t.Errorf("without token: want 401, got %d", status)
	}
}

// jsonString encodes s as a JSON string literal (with surrounding quotes).
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
