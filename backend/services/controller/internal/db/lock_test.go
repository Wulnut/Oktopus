package db

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

func TestLockCommandResendClaimFilterClaimsStaleRetryOrPendingRows(t *testing.T) {
	id := primitive.NewObjectID()
	observedAt := time.Now().Add(-time.Minute)
	filter := lockCommandResendClaimFilter(id, observedAt)
	if filter["_id"] != id {
		t.Fatalf("claim filter id = %#v, want %s", filter["_id"], id.Hex())
	}
	statuses, ok := filter["status"].(bson.M)
	if !ok {
		t.Fatalf("claim filter status = %#v", filter["status"])
	}
	if got := statuses["$in"]; !reflect.DeepEqual(got, []LockCommandStatus{LockCommandRetry, LockCommandPending}) {
		t.Fatalf("claim statuses = %#v", got)
	}
	updated, ok := filter["updated_at"].(bson.M)
	if !ok || updated["$lte"] != observedAt {
		t.Fatalf("claim updated_at = %#v, want <= %v", filter["updated_at"], observedAt)
	}
}

func TestLockRetryableCommandFilterIncludesStalePendingRows(t *testing.T) {
	retryBefore := time.Now().Add(-time.Minute)
	filter := lockRetryableCommandFilter(retryBefore)
	statuses := filter["status"].(bson.M)
	if got := statuses["$in"]; !reflect.DeepEqual(got, []LockCommandStatus{LockCommandRetry, LockCommandPending}) {
		t.Fatalf("retryable statuses = %#v", got)
	}
}

func TestLockCommandResendUpdateUsesCurrentDecision(t *testing.T) {
	update := lockCommandResendUpdate("203.0.113.8", LockStatusUnlocked, "0")
	set, ok := update["$set"].(bson.M)
	if !ok {
		t.Fatalf("$set = %#v", update["$set"])
	}
	if set["reported_ip"] != "203.0.113.8" || set["target_status"] != LockStatusUnlocked || set["command_value"] != "0" {
		t.Fatalf("retry must use current evaluation, got %#v", set)
	}
	if set["status"] != LockCommandPending {
		t.Fatalf("claimed retry status = %#v, want pending", set["status"])
	}
	inc, ok := update["$inc"].(bson.M)
	if !ok || inc["attempt_count"] != 1 {
		t.Fatalf("attempt increment = %#v", update["$inc"])
	}
}

func TestPrepareLockAuditLogReturnsPersistedMetadata(t *testing.T) {
	entry := prepareLockAuditLog(LockAuditLog{SN: " sn-9 ", Action: "evaluate"})
	if entry.ID.IsZero() {
		t.Fatal("audit ID must be assigned before external publication")
	}
	if entry.CreatedAt.IsZero() {
		t.Fatal("audit timestamp must be assigned before external publication")
	}
	if entry.SN != "SN-9" {
		t.Fatalf("normalized SN = %q", entry.SN)
	}
}

func TestUnsupportedOptOutDeleteFilterIsAtomic(t *testing.T) {
	filter := unsupportedOptOutDeleteFilter([]string{" sn-1 ", "SN-2"})
	if filter["opt_out"] != true {
		t.Fatalf("delete filter must require opt_out=true, got %#v", filter)
	}
	snFilter, ok := filter["sn"].(bson.M)
	if !ok {
		t.Fatalf("sn filter = %#v", filter["sn"])
	}
	got, ok := snFilter["$in"].([]string)
	if !ok || len(got) != 2 || got[0] != "SN-1" || got[1] != "SN-2" {
		t.Fatalf("normalized SNs = %#v", snFilter["$in"])
	}
}
