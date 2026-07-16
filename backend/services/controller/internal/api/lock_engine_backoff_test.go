package api

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/leandrofars/oktopus/internal/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// fakeLockStateStore is an in-memory lockDeviceStateStore for command-path tests.
type fakeLockStateStore struct {
	mu     sync.Mutex
	states map[string]lockDeviceState
}

func newFakeLockStateStore() *fakeLockStateStore {
	return &fakeLockStateStore{states: map[string]lockDeviceState{}}
}

func keyOf(tenant, sn string) string { return tenant + "|" + db.NormalizeSN(sn) }

func (f *fakeLockStateStore) Get(_ context.Context, tenant, sn string) (lockDeviceState, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.states[keyOf(tenant, sn)]
	return st, ok, nil
}

func (f *fakeLockStateStore) Put(_ context.Context, tenant, sn string, st lockDeviceState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[keyOf(tenant, sn)] = st
	return nil
}

func (f *fakeLockStateStore) TryLock(_ context.Context, _, _ string, _ time.Duration) (func(), bool, error) {
	return func() {}, true, nil
}

func (f *fakeLockStateStore) snapshot(tenant, sn string) lockDeviceState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.states[keyOf(tenant, sn)]
}

func attemptFor(sn string, target db.DeviceLockStatus, n int) db.LockCommandAttempt {
	return db.LockCommandAttempt{
		ID:           primitive.NewObjectID(),
		DeviceSN:     db.NormalizeSN(sn),
		TargetStatus: target,
		Status:       db.LockCommandRetry,
		AttemptCount: n,
	}
}

func TestRecordDeviceBackoffFailure_ReachesCooldownAndStoresDeadline(t *testing.T) {
	resetLockAdaptersForTest()
	store := newFakeLockStateStore()
	setLockStateStoreForTest(store)
	defer setLockStateStoreForTest(noopLockDeviceStateStore{})

	a := &Api{lockBackoff: normalizeLockBackoffConfig(lockBackoffConfig{Enabled: true, Threshold: 3, Cooldown: 10 * time.Minute})}
	// Seed two prior failures so the third trips the threshold.
	prev := store.states[keyOf("t", "081074000666")]
	prev.CommandBackoffs = map[db.DeviceLockStatus]*lockCommandBackoff{
		db.LockStatusLocked: {ConsecutiveFailures: 2},
	}
	store.states[keyOf("t", "081074000666")] = prev

	cooled, deadline := a.recordDeviceBackoffFailure(context.Background(), nil, "t", "081074000666", db.LockStatusLocked, fmt.Errorf("usp error 7003: db locked"))
	if !cooled {
		t.Fatal("third failure must cool down")
	}
	got := store.snapshot("t", "081074000666")
	b := got.CommandBackoffs[db.LockStatusLocked]
	if b == nil || b.ConsecutiveFailures != 3 {
		t.Fatalf("expected 3 stored failures, got %+v", b)
	}
	if !b.CooldownUntil.Equal(deadline) {
		t.Fatalf("stored deadline must match returned deadline")
	}
}

func TestHandleLockCommandFailure_PermanentErrorDoesNotBackoff(t *testing.T) {
	resetLockAdaptersForTest()
	store := newFakeLockStateStore()
	setLockStateStoreForTest(store)
	defer setLockStateStoreForTest(noopLockDeviceStateStore{})

	a := &Api{lockBackoff: normalizeLockBackoffConfig(lockBackoffConfig{Enabled: true, Threshold: 3, Cooldown: 10 * time.Minute})}
	a.handleLockCommandFailure(context.Background(), nil,
		attemptFor("081074000666", db.LockStatusLocked, 1), db.LockStatusLocked,
		fmt.Errorf("usp error 7026: path not in schema"), "t")
	got := store.snapshot("t", "081074000666")
	if len(got.CommandBackoffs) != 0 {
		t.Fatal("permanent error must not increment backoff")
	}
}

func TestHandleLockCommandSuccess_ClearsAllBackoff(t *testing.T) {
	resetLockAdaptersForTest()
	store := newFakeLockStateStore()
	setLockStateStoreForTest(store)
	defer setLockStateStoreForTest(noopLockDeviceStateStore{})

	store.states[keyOf("t", "081074000666")] = lockDeviceState{
		LastIP:     "10.0.0.1",
		LastStatus: db.LockStatusLocked,
		CommandBackoffs: map[db.DeviceLockStatus]*lockCommandBackoff{
			db.LockStatusLocked:   {ConsecutiveFailures: 3},
			db.LockStatusUnlocked: {ConsecutiveFailures: 2},
		},
	}
	a := &Api{lockBackoff: normalizeLockBackoffConfig(lockBackoffConfig{Enabled: true, Threshold: 3, Cooldown: 10 * time.Minute})}
	a.handleLockCommandSuccess(context.Background(), nil, "081074000666", db.LockStatusLocked, "t")
	got := store.snapshot("t", "081074000666")
	if len(got.CommandBackoffs) != 0 {
		t.Fatalf("success must clear all backoff, got %+v", got)
	}
	if got.LastIP != "10.0.0.1" || got.LastStatus != db.LockStatusLocked {
		t.Fatal("success clear must preserve LastIP/LastStatus")
	}
}

func TestHandleLockCommandFailure_DisabledKeepsLegacyRetry(t *testing.T) {
	resetLockAdaptersForTest()
	store := newFakeLockStateStore()
	setLockStateStoreForTest(store)
	defer setLockStateStoreForTest(noopLockDeviceStateStore{})

	// Backoff disabled: behavior must match the old markLockCommandOutcome.
	a := &Api{lockBackoff: defaultLockBackoffConfig()}
	a.handleLockCommandFailure(context.Background(), nil,
		attemptFor("081074000666", db.LockStatusLocked, 1), db.LockStatusLocked,
		fmt.Errorf("usp error 7003: db locked"), "t")
	got := store.snapshot("t", "081074000666")
	if len(got.CommandBackoffs) != 0 {
		t.Fatal("disabled backoff must not write backoff state")
	}
}
