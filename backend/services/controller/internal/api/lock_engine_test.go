package api

import (
	"testing"

	"github.com/leandrofars/oktopus/internal/db"
)

func TestEvaluateLockDecisionBlacklistOverridesWhitelist(t *testing.T) {
	decision := EvaluateLockDecision(LockEvaluationInput{
		SN:         "SN-001",
		ReportedIP: "10.10.1.5",
		Config:     db.DefaultLockConfig(),
		Policy: &db.LockPolicy{
			SN:             "SN-001",
			PolicyType:     db.LockPolicyBlacklist,
			AllowedIPRange: "10.10.0.0/16",
			Status:         true,
		},
	})

	if decision.Status != db.LockStatusLocked {
		t.Fatalf("expected blacklisted device to be locked, got %s", decision.Status)
	}
	if !decision.ShouldCommand || decision.CommandValue != "1" {
		t.Fatalf("expected LOCK command value 1, got command=%v value=%q", decision.ShouldCommand, decision.CommandValue)
	}
}

func TestEvaluateLockDecisionWhitelistAndCIDRUnlocks(t *testing.T) {
	decision := EvaluateLockDecision(LockEvaluationInput{
		SN:         "SN-001",
		ReportedIP: "10.10.1.5",
		Config:     db.DefaultLockConfig(),
		Policy: &db.LockPolicy{
			SN:             "SN-001",
			PolicyType:     db.LockPolicyWhitelist,
			AllowedIPRange: "10.10.0.0/16",
			Status:         true,
		},
	})

	if decision.Status != db.LockStatusUnlocked {
		t.Fatalf("expected whitelisted device in CIDR to be unlocked, got %s", decision.Status)
	}
	if !decision.ShouldCommand || decision.CommandValue != "0" {
		t.Fatalf("expected UNLOCK command value 0, got command=%v value=%q", decision.ShouldCommand, decision.CommandValue)
	}
}

func TestEvaluateLockDecisionAutoLockLocksUnknownDevice(t *testing.T) {
	decision := EvaluateLockDecision(LockEvaluationInput{
		SN:         "SN-002",
		ReportedIP: "10.10.1.5",
		Config:     db.DefaultLockConfig(),
	})

	if decision.Status != db.LockStatusLocked {
		t.Fatalf("expected unknown device to be locked when auto-lock is enabled, got %s", decision.Status)
	}
	if decision.Reason != db.LockReasonUnauthorized {
		t.Fatalf("expected unauthorized reason, got %s", decision.Reason)
	}
}

func TestEvaluateLockDecisionAutoLockDisabledReturnsPending(t *testing.T) {
	cfg := db.DefaultLockConfig()
	cfg.AutoLockEnabled = false

	decision := EvaluateLockDecision(LockEvaluationInput{
		SN:         "SN-002",
		ReportedIP: "10.10.1.5",
		Config:     cfg,
	})

	if decision.Status != db.LockStatusPending {
		t.Fatalf("expected PENDING when auto-lock is disabled, got %s", decision.Status)
	}
	if decision.ShouldCommand {
		t.Fatal("expected no command to be sent for PENDING devices")
	}
}

func TestEvaluateLockDecisionMasterSwitchDisabledUnlocks(t *testing.T) {
	cfg := db.DefaultLockConfig()
	cfg.MasterEnabled = false

	decision := EvaluateLockDecision(LockEvaluationInput{
		SN:         "SN-001",
		ReportedIP: "10.10.1.5",
		Config:     cfg,
		Policy: &db.LockPolicy{
			SN:         "SN-001",
			PolicyType: db.LockPolicyBlacklist,
			Status:     true,
		},
	})

	if decision.Status != db.LockStatusUnlocked {
		t.Fatalf("expected master switch disabled to unlock, got %s", decision.Status)
	}
	if !decision.ShouldCommand || decision.CommandValue != "0" {
		t.Fatalf("expected UNLOCK command value 0, got command=%v value=%q", decision.ShouldCommand, decision.CommandValue)
	}
}
