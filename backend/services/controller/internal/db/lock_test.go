package db

import (
	"testing"
	"time"
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
