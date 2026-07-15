package db

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestLockPolicyNormalizeTrimsAndUppercases(t *testing.T) {
	policy := LockPolicy{
		SN:             "  sn-001  ",
		PolicyType:     LockPolicyWhitelist,
		AllowedIPRange: " 10.10.0.0/16 ",
		Status:         true,
	}

	policy.Normalize()

	if policy.SN != "SN-001" {
		t.Fatalf("expected normalized SN SN-001, got %q", policy.SN)
	}
	if policy.AllowedIPRange != "10.10.0.0/16" {
		t.Fatalf("expected CIDR to be trimmed, got %q", policy.AllowedIPRange)
	}
}

func TestLockPolicyValidateRequiresWhitelistCIDR(t *testing.T) {
	policy := LockPolicy{
		SN:         "SN-001",
		PolicyType: LockPolicyWhitelist,
		Status:     true,
	}

	if err := policy.Validate(); err == nil {
		t.Fatal("expected whitelist without CIDR to fail validation")
	}
}

func TestLockConfigDefaultsEnableMasterAndAutoLock(t *testing.T) {
	cfg := DefaultLockConfig()

	if !cfg.MasterEnabled {
		t.Fatal("expected master lock switch to default enabled")
	}
	if !cfg.AutoLockEnabled {
		t.Fatal("expected auto-lock switch to default enabled")
	}
	if cfg.UpdatedAt.IsZero() || time.Since(cfg.UpdatedAt) > time.Second {
		t.Fatalf("expected UpdatedAt to be initialized, got %v", cfg.UpdatedAt)
	}
}

func TestLockCommandHistoryClearFilterOnlyCompleted(t *testing.T) {
	filter := lockCommandHistoryClearFilter()
	statusFilter, ok := filter["status"].(bson.M)
	if !ok {
		t.Fatalf("expected status filter map, got %#v", filter["status"])
	}
	in, ok := statusFilter["$in"].([]LockCommandStatus)
	if !ok {
		t.Fatalf("expected $in []LockCommandStatus, got %#v", statusFilter["$in"])
	}
	want := map[LockCommandStatus]bool{
		LockCommandSuccess: true,
		LockCommandFailed:  true,
	}
	if len(in) != len(want) {
		t.Fatalf("expected %d statuses, got %v", len(want), in)
	}
	for _, st := range in {
		if !want[st] {
			t.Fatalf("unexpected clear status %q (must not delete pending/retry)", st)
		}
	}
	for _, preserve := range []LockCommandStatus{LockCommandPending, LockCommandRetry} {
		if want[preserve] {
			t.Fatalf("%q must not be cleared", preserve)
		}
	}
}

func TestLockPolicyValidateCommaSeparatedCIDRs(t *testing.T) {
	policy := LockPolicy{
		SN:             "SN-001",
		PolicyType:     LockPolicyWhitelist,
		AllowedIPRange: "10.0.0.0/8,2001:db8::/32",
		Status:         true,
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("expected comma-separated v4+v6 CIDRs to validate, got %v", err)
	}
}

func TestLockPolicyValidateRejectsBadCIDRInList(t *testing.T) {
	policy := LockPolicy{
		SN:             "SN-001",
		PolicyType:     LockPolicyWhitelist,
		AllowedIPRange: "10.0.0.0/8,not-a-cidr",
		Status:         true,
	}
	if err := policy.Validate(); err == nil {
		t.Fatal("expected validation error for bad CIDR in comma-separated list")
	}
}

func TestLockPolicyValidateRejectsEmptyCIDRList(t *testing.T) {
	policy := LockPolicy{
		SN:             "SN-001",
		PolicyType:     LockPolicyWhitelist,
		AllowedIPRange: " , , ",
		Status:         true,
	}
	if err := policy.Validate(); err == nil {
		t.Fatal("expected validation error when CIDR list has no values")
	}
}
