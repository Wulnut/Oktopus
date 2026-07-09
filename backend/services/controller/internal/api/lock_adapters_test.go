package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/leandrofars/oktopus/internal/db"
)

func TestShouldMarkLockCommandForRetry(t *testing.T) {
	if !shouldMarkLockCommandForRetry(1, 10) {
		t.Fatal("expected first failure to be retryable")
	}
	if shouldMarkLockCommandForRetry(10, 10) {
		t.Fatal("expected max attempts to stop retrying")
	}
	if !shouldMarkLockCommandForRetry(3, 0) {
		t.Fatal("expected default max attempts when unset")
	}
}

func TestRedisLockPolicyCacheRoundTrip(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	cache, err := newRedisLockPolicyCache("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("newRedisLockPolicyCache: %v", err)
	}

	policy := db.LockPolicy{
		SN:             "SN-001",
		PolicyType:     db.LockPolicyWhitelist,
		AllowedIPRange: "10.0.0.0/8",
		Status:         true,
	}
	ctx := context.Background()
	if err := cache.PutLockPolicy(ctx, "tenant-a", policy); err != nil {
		t.Fatalf("PutLockPolicy: %v", err)
	}
	got, found, err := cache.GetLockPolicy(ctx, "tenant-a", "SN-001")
	if err != nil {
		t.Fatalf("GetLockPolicy: %v", err)
	}
	if !found {
		t.Fatal("expected cached policy")
	}
	if got.SN != policy.SN || got.AllowedIPRange != policy.AllowedIPRange {
		t.Fatalf("unexpected policy %+v", got)
	}
	if err := cache.DeleteLockPolicy(ctx, "tenant-a", "SN-001"); err != nil {
		t.Fatalf("DeleteLockPolicy: %v", err)
	}
	_, found, err = cache.GetLockPolicy(ctx, "tenant-a", "SN-001")
	if err != nil {
		t.Fatalf("GetLockPolicy after delete: %v", err)
	}
	if found {
		t.Fatal("expected cache miss after delete")
	}
}

func TestLockAuditKafkaMessageSerialization(t *testing.T) {
	entry := db.LockAuditLog{
		SN:     "SN-002",
		Action: "evaluate",
		Status: db.LockStatusLocked,
		Details: map[string]interface{}{
			"reason": string(db.LockReasonUnauthorized),
		},
		CreatedAt: time.Now().UTC(),
	}
	msg := lockAuditToKafkaMessage("tenant-b", entry)
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("invalid json: %s", string(raw))
	}
	if msg.TenantSlug != "tenant-b" || msg.SN != "SN-002" {
		t.Fatalf("unexpected message %+v", msg)
	}
}

func TestNoopLockAdaptersDoNotPanic(t *testing.T) {
	resetLockAdaptersForTest()
	defer resetLockAdaptersForTest()

	ctx := context.Background()
	if _, found, err := lockCache.GetLockPolicy(ctx, "tenant", "SN"); err != nil || found {
		t.Fatalf("noop cache should miss without error, found=%v err=%v", found, err)
	}
	if _, found, err := lockStateStore.Get(ctx, "tenant", "SN"); err != nil || found {
		t.Fatalf("noop state store should miss without error, found=%v err=%v", found, err)
	}
	if err := lockAuditSink.PublishLockAudit(ctx, "tenant", db.LockAuditLog{Action: "noop"}); err != nil {
		t.Fatalf("noop sink: %v", err)
	}
}
