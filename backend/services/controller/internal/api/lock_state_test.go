package api

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/redis/go-redis/v9"
)

func newTestRedisStateStore(t *testing.T) (*redisLockBackend, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return &redisLockBackend{client: client}, mr
}

func TestRedisLockDeviceStateGetMiss(t *testing.T) {
	store, mr := newTestRedisStateStore(t)
	defer mr.Close()
	defer store.client.Close()

	st, found, err := store.Get(context.Background(), "tenant-a", "SN-001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatalf("expected miss, got %+v", st)
	}
}

func TestRedisLockDeviceStatePutGetRoundTrip(t *testing.T) {
	store, mr := newTestRedisStateStore(t)
	defer mr.Close()
	defer store.client.Close()

	want := lockDeviceState{
		LastIP:      "10.1.2.3",
		LastStatus:  db.LockStatusLocked,
		LastCommand: "1",
		NotifyOKAt:  time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 7, 9, 12, 1, 0, 0, time.UTC),
	}
	ctx := context.Background()
	if err := store.Put(ctx, "tenant-a", "sn-001", want); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// SN normalization: lookup with different case should hit same key.
	got, found, err := store.Get(ctx, "tenant-a", "SN-001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected found")
	}
	if got.LastIP != want.LastIP || got.LastStatus != want.LastStatus || got.LastCommand != want.LastCommand {
		t.Fatalf("unexpected state %+v", got)
	}
	if !got.NotifyOKAt.Equal(want.NotifyOKAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("unexpected timestamps %+v", got)
	}
}

func TestRedisLockDeviceStateTryLockContention(t *testing.T) {
	store, mr := newTestRedisStateStore(t)
	defer mr.Close()
	defer store.client.Close()

	ctx := context.Background()
	unlock1, ok, err := store.TryLock(ctx, "tenant-a", "SN-001", 10*time.Second)
	if err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	if !ok {
		t.Fatal("expected first TryLock to succeed")
	}
	if unlock1 == nil {
		t.Fatal("expected unlock func")
	}

	unlock2, ok, err := store.TryLock(ctx, "tenant-a", "SN-001", 10*time.Second)
	if err != nil {
		t.Fatalf("second TryLock: %v", err)
	}
	if ok {
		t.Fatal("expected contention (ok=false)")
	}
	if unlock2 != nil {
		t.Fatal("expected nil unlock on contention")
	}

	unlock1()

	unlock3, ok, err := store.TryLock(ctx, "tenant-a", "SN-001", 10*time.Second)
	if err != nil {
		t.Fatalf("TryLock after unlock: %v", err)
	}
	if !ok {
		t.Fatal("expected TryLock to succeed after unlock")
	}
	unlock3()
}

func TestRedisLockDeviceStateTryLockSoftDegradeOnRedisError(t *testing.T) {
	store, mr := newTestRedisStateStore(t)
	store.client.Close()
	mr.Close()

	unlock, ok, err := store.TryLock(context.Background(), "tenant-a", "SN-001", time.Second)
	if err != nil {
		t.Fatalf("expected soft degrade without error, got %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true when Redis unavailable")
	}
	if unlock == nil {
		t.Fatal("expected noop unlock")
	}
	unlock() // must not panic
}

func TestRedisLockDeviceStateTryLockTokenUnlockSafeAfterExpiry(t *testing.T) {
	store, mr := newTestRedisStateStore(t)
	defer mr.Close()
	defer store.client.Close()

	ctx := context.Background()
	unlock1, ok, err := store.TryLock(ctx, "tenant-a", "SN-001", 50*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("first TryLock: ok=%v err=%v", ok, err)
	}

	// Expire the first holder's key, then let a second holder acquire it.
	mr.FastForward(100 * time.Millisecond)

	unlock2, ok, err := store.TryLock(ctx, "tenant-a", "SN-001", 10*time.Second)
	if err != nil || !ok {
		t.Fatalf("second TryLock after expiry: ok=%v err=%v", ok, err)
	}

	// Stale unlock from the first holder must not delete the second holder's lock.
	unlock1()

	unlock3, ok, err := store.TryLock(ctx, "tenant-a", "SN-001", 10*time.Second)
	if err != nil {
		t.Fatalf("third TryLock: %v", err)
	}
	if ok {
		t.Fatal("expected contention: second holder's lock should still be present")
	}
	if unlock3 != nil {
		t.Fatal("expected nil unlock on contention")
	}

	unlock2()

	unlock4, ok, err := store.TryLock(ctx, "tenant-a", "SN-001", 10*time.Second)
	if err != nil || !ok {
		t.Fatalf("TryLock after rightful unlock: ok=%v err=%v", ok, err)
	}
	unlock4()
}

func TestNoopLockDeviceStateStoreSoftMiss(t *testing.T) {
	var store noopLockDeviceStateStore
	ctx := context.Background()

	st, found, err := store.Get(ctx, "tenant", "SN")
	if err != nil || found {
		t.Fatalf("noop Get should miss without error, found=%v err=%v st=%+v", found, err, st)
	}
	if err := store.Put(ctx, "tenant", "SN", lockDeviceState{LastIP: "1.2.3.4"}); err != nil {
		t.Fatalf("noop Put: %v", err)
	}
	unlock, ok, err := store.TryLock(ctx, "tenant", "SN", time.Second)
	if err != nil || !ok {
		t.Fatalf("noop TryLock should succeed, ok=%v err=%v", ok, err)
	}
	unlock()
}
