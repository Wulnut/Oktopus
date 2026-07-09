package api

import (
	"testing"

	"github.com/leandrofars/oktopus/internal/db"
)

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
			PolicyType: db.LockPolicyWhitelist,
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

func TestShouldSkipLockCommandSameStatusPollSkips(t *testing.T) {
	skip := shouldSkipLockCommand(
		lockTriggerIPChangePoll,
		true,
		db.LockStatusLocked,
		db.LockStatusLocked,
		true,
	)
	if !skip {
		t.Fatal("same status + poll should skip command")
	}
}

func TestShouldSkipLockCommandSameStatusNotifySkips(t *testing.T) {
	skip := shouldSkipLockCommand(
		lockTriggerIPChangeNotify,
		true,
		db.LockStatusUnlocked,
		db.LockStatusUnlocked,
		true,
	)
	if !skip {
		t.Fatal("same status + notify should skip command")
	}
}

func TestShouldSkipLockCommandSameStatusOnlineSends(t *testing.T) {
	skip := shouldSkipLockCommand(
		lockTriggerOnline,
		true,
		db.LockStatusLocked,
		db.LockStatusLocked,
		true,
	)
	if skip {
		t.Fatal("same status + online must force-converge (send)")
	}
}

func TestShouldSkipLockCommandSameStatusChaseSends(t *testing.T) {
	skip := shouldSkipLockCommand(
		lockTriggerChase,
		true,
		db.LockStatusLocked,
		db.LockStatusLocked,
		true,
	)
	if skip {
		t.Fatal("same status + chase must force-converge (send)")
	}
}

func TestShouldSkipLockCommandStatusChangePollSends(t *testing.T) {
	skip := shouldSkipLockCommand(
		lockTriggerIPChangePoll,
		true,
		db.LockStatusLocked,
		db.LockStatusUnlocked,
		true,
	)
	if skip {
		t.Fatal("status change + poll should send")
	}
}

func TestShouldSkipLockCommandMissingRedisStateSends(t *testing.T) {
	skip := shouldSkipLockCommand(
		lockTriggerIPChangePoll,
		false,
		"",
		db.LockStatusLocked,
		true,
	)
	if skip {
		t.Fatal("missing Redis state should send when ShouldCommand")
	}
}

func TestShouldSkipLockCommandNoShouldCommandNeverSkips(t *testing.T) {
	// Skip helper only applies when a command would otherwise be sent.
	skip := shouldSkipLockCommand(
		lockTriggerIPChangePoll,
		true,
		db.LockStatusPending,
		db.LockStatusPending,
		false,
	)
	if skip {
		t.Fatal("!ShouldCommand path is handled separately; helper must return false")
	}
}
