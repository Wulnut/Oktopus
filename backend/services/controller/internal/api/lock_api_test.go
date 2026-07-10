package api

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leandrofars/oktopus/internal/db"
)

func TestDecodeLockCSV(t *testing.T) {
	csvBody := "sn,allowed_ip_range,reason,description\nSN-001,10.0.0.0/8,lost,test device\n"
	req := httptest.NewRequest("POST", "/lock/whitelist/batch", strings.NewReader(csvBody))
	req.Header.Set("Content-Type", "text/csv")

	policies, err := decodeLockBatch(req)
	if err != nil {
		t.Fatalf("decodeLockBatch: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}
	if policies[0].SN != "SN-001" || policies[0].AllowedIPRange != "10.0.0.0/8" {
		t.Fatalf("unexpected policy %+v", policies[0])
	}
}

func TestDecodeLockBatchJSON(t *testing.T) {
	body := bytes.NewBufferString(`{"items":[{"sn":"SN-002","allowed_ip_range":"192.168.0.0/16"}]}`)
	req := httptest.NewRequest("POST", "/lock/whitelist/batch", body)
	req.Header.Set("Content-Type", "application/json")

	policies, err := decodeLockBatch(req)
	if err != nil {
		t.Fatalf("decodeLockBatch: %v", err)
	}
	if len(policies) != 1 || policies[0].SN != "SN-002" {
		t.Fatalf("unexpected policies %+v", policies)
	}
}

func TestEvaluateLockDecisionWhitelistOutOfCIDRAutoLocks(t *testing.T) {
	decision := EvaluateLockDecision(LockEvaluationInput{
		SN:         "SN-003",
		ReportedIP: "172.16.1.1",
		Config:     db.DefaultLockConfig(),
		Policy: &db.LockPolicy{
			SN:             "SN-003",
			PolicyType:     db.LockPolicyWhitelist,
			AllowedIPRange: "10.0.0.0/8",
			Status:         true,
		},
	})
	if decision.Status != db.LockStatusLocked {
		t.Fatalf("expected out-of-CIDR whitelist device to auto-lock, got %s", decision.Status)
	}
}

func TestParseLockListPaginationDefaults(t *testing.T) {
	req := httptest.NewRequest("GET", "/lock/commands", nil)
	page, size, err := parseLockListPagination(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page != 0 || size != lockListPageSizeDefault {
		t.Fatalf("got page=%d size=%d, want page=0 size=%d", page, size, lockListPageSizeDefault)
	}
}

func TestParseLockListPaginationCustom(t *testing.T) {
	req := httptest.NewRequest("GET", "/lock/commands?page_number=2&page_size=50", nil)
	page, size, err := parseLockListPagination(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page != 2 || size != 50 {
		t.Fatalf("got page=%d size=%d, want 2/50", page, size)
	}
}

func TestParseLockListPaginationRejectsNegativePage(t *testing.T) {
	req := httptest.NewRequest("GET", "/lock/commands?page_number=-1", nil)
	if _, _, err := parseLockListPagination(req); err == nil {
		t.Fatal("expected error for negative page_number")
	}
}

func TestParseLockListPaginationRejectsOversize(t *testing.T) {
	req := httptest.NewRequest("GET", "/lock/commands?page_size=101", nil)
	if _, _, err := parseLockListPagination(req); err == nil {
		t.Fatal("expected error for page_size > max")
	}
}

func TestParseLockListPaginationRejectsZeroSize(t *testing.T) {
	req := httptest.NewRequest("GET", "/lock/commands?page_size=0", nil)
	if _, _, err := parseLockListPagination(req); err == nil {
		t.Fatal("expected error for page_size=0")
	}
}
