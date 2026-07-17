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

func TestLockDecisionAlreadyConverged(t *testing.T) {
	cases := []struct {
		name     string
		actual   db.DeviceLockStatus
		decision LockDecision
		want     bool
	}{
		{"locked match", db.LockStatusLocked, LockDecision{Status: db.LockStatusLocked, ShouldCommand: true, CommandValue: "1"}, true},
		{"unlocked match", db.LockStatusUnlocked, LockDecision{Status: db.LockStatusUnlocked, ShouldCommand: true, CommandValue: "0"}, true},
		{"mismatch", db.LockStatusUnlocked, LockDecision{Status: db.LockStatusLocked, ShouldCommand: true, CommandValue: "1"}, false},
		{"unknown actual", "", LockDecision{Status: db.LockStatusLocked, ShouldCommand: true, CommandValue: "1"}, false},
		{"no command", db.LockStatusLocked, LockDecision{Status: db.LockStatusPending}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lockDecisionAlreadyConverged(tc.actual, tc.decision); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHostCIDR_IPv4(t *testing.T) {
	got := hostCIDR("192.168.1.100")
	if got != "192.168.1.100/32" {
		t.Fatalf("expected 192.168.1.100/32, got %q", got)
	}
}

func TestHostCIDR_IPv6(t *testing.T) {
	got := hostCIDR("2001:db8::1")
	if got != "2001:db8::1/128" {
		t.Fatalf("expected 2001:db8::1/128, got %q", got)
	}
}

func TestHostCIDR_Invalid(t *testing.T) {
	got := hostCIDR("garbage")
	if got != "garbage" {
		t.Fatalf("expected garbage unchanged, got %q", got)
	}
}

func TestNormalizeReportedIP_CleanIPv4(t *testing.T) {
	got := normalizeReportedIP("192.168.1.100")
	if got != "192.168.1.100" {
		t.Fatalf("expected 192.168.1.100, got %q", got)
	}
}

func TestNormalizeReportedIP_CleanIPv6(t *testing.T) {
	got := normalizeReportedIP("2001:db8::1")
	if got != "2001:db8::1" {
		t.Fatalf("expected 2001:db8::1, got %q", got)
	}
}

func TestNormalizeReportedIP_DualStack(t *testing.T) {
	got := normalizeReportedIP("192.168.1.100 2001:db8::1")
	if got != "192.168.1.100" {
		t.Fatalf("expected first valid IP 192.168.1.100, got %q", got)
	}
}

func TestNormalizeReportedIP_ZoneID(t *testing.T) {
	got := normalizeReportedIP("fe80::1%eth0")
	if got != "fe80::1" {
		t.Fatalf("expected fe80::1 (zone stripped), got %q", got)
	}
}

func TestNormalizeReportedIP_Empty(t *testing.T) {
	got := normalizeReportedIP("")
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestNormalizeReportedIP_Garbage(t *testing.T) {
	got := normalizeReportedIP("not-an-ip")
	if got != "" {
		t.Fatalf("expected empty for garbage, got %q", got)
	}
}

func TestIpInCIDR_CommaSeparated(t *testing.T) {
	got := ipInCIDR("2001:db8::1", "192.168.1.100/32,2001:db8::1/128")
	if !got {
		t.Fatal("expected IPv6 match in comma-separated dual-stack CIDR")
	}
}

func TestIpInCIDR_IPv6SingleHost(t *testing.T) {
	got := ipInCIDR("2001:db8::1", "2001:db8::1/128")
	if !got {
		t.Fatal("expected IPv6 /128 match")
	}
}

func TestIpInCIDR_IPv6NoMatch(t *testing.T) {
	got := ipInCIDR("2001:db8::2", "2001:db8::1/128")
	if got {
		t.Fatal("expected no match for different IPv6 address")
	}
}

func TestMergeCIDRByFamily_SameFamilyReplace(t *testing.T) {
	got := mergeCIDRByFamily("10.0.0.1/32", "10.0.0.2/32")
	if got != "10.0.0.2/32" {
		t.Fatalf("expected same-family replace to 10.0.0.2/32, got %q", got)
	}
}

func TestMergeCIDRByFamily_DifferentFamilyAppend(t *testing.T) {
	got := mergeCIDRByFamily("10.0.0.1/32", "2001:db8::1/128")
	if got != "10.0.0.1/32,2001:db8::1/128" {
		t.Fatalf("expected dual-stack append, got %q", got)
	}
}

func TestHostCIDR_IPv4MappedIPv6(t *testing.T) {
	got := hostCIDR("::ffff:192.0.2.1")
	if got != "192.0.2.1/32" {
		t.Fatalf("expected mapped IPv4 to normalize to 192.0.2.1/32, got %q", got)
	}
}

func TestMergeCIDRByFamily_DualStackReplaceIPv4(t *testing.T) {
	got := mergeCIDRByFamily("10.0.0.1/32,2001:db8::1/128", "10.0.0.2/32")
	if got != "10.0.0.2/32,2001:db8::1/128" {
		t.Fatalf("expected IPv4 replacement while preserving IPv6, got %q", got)
	}
}

func TestMergeCIDRByFamily_DualStackReplaceIPv6(t *testing.T) {
	got := mergeCIDRByFamily("10.0.0.1/32,2001:db8::1/128", "2001:db8::2/128")
	if got != "10.0.0.1/32,2001:db8::2/128" {
		t.Fatalf("expected IPv6 replacement while preserving IPv4, got %q", got)
	}
}
