package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
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
	getErr error
	putErr error
}

func newFakeLockStateStore() *fakeLockStateStore {
	return &fakeLockStateStore{states: map[string]lockDeviceState{}}
}

func keyOf(tenant, sn string) string { return tenant + "|" + db.NormalizeSN(sn) }

func (f *fakeLockStateStore) Get(_ context.Context, tenant, sn string) (lockDeviceState, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return lockDeviceState{}, false, f.getErr
	}
	st, ok := f.states[keyOf(tenant, sn)]
	return st, ok, nil
}

func (f *fakeLockStateStore) Put(_ context.Context, tenant, sn string, st lockDeviceState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.putErr != nil {
		return f.putErr
	}
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

func TestRecordDeviceBackoffFailure_PutErrorSoftDegrades(t *testing.T) {
	resetLockAdaptersForTest()
	store := newFakeLockStateStore()
	store.putErr = errors.New("redis unavailable")
	store.states[keyOf("t", "081074000666")] = lockDeviceState{
		CommandBackoffs: map[db.DeviceLockStatus]*lockCommandBackoff{
			db.LockStatusLocked: {ConsecutiveFailures: 2},
		},
	}
	setLockStateStoreForTest(store)
	defer setLockStateStoreForTest(noopLockDeviceStateStore{})

	a := &Api{lockBackoff: normalizeLockBackoffConfig(lockBackoffConfig{Enabled: true, Threshold: 3, Cooldown: 10 * time.Minute})}
	cooled, deadline := a.recordDeviceBackoffFailure(context.Background(), nil, "t", "081074000666", db.LockStatusLocked, errors.New("delivery failed"))
	if cooled || !deadline.IsZero() {
		t.Fatalf("failed persistence must soft-degrade without cooldown, got cooled=%v deadline=%s", cooled, deadline)
	}
	got := store.snapshot("t", "081074000666")
	if got.CommandBackoffs[db.LockStatusLocked].ConsecutiveFailures != 2 {
		t.Fatal("failed persistence must leave stored backoff unchanged")
	}
}

func TestRecordDeviceBackoffFailure_PreservesDeviceState(t *testing.T) {
	resetLockAdaptersForTest()
	store := newFakeLockStateStore()
	setLockStateStoreForTest(store)
	defer setLockStateStoreForTest(noopLockDeviceStateStore{})

	notifyAt := time.Date(2026, time.July, 16, 10, 0, 0, 0, time.UTC)
	store.states[keyOf("t", "081074000666")] = lockDeviceState{
		LastIP:      "10.0.0.1",
		LastStatus:  db.LockStatusUnlocked,
		LastCommand: "0",
		NotifyOKAt:  notifyAt,
	}
	a := &Api{lockBackoff: normalizeLockBackoffConfig(lockBackoffConfig{Enabled: true, Threshold: 3, Cooldown: 10 * time.Minute})}
	a.recordDeviceBackoffFailure(context.Background(), nil, "t", "081074000666", db.LockStatusLocked, errors.New("delivery failed"))

	got := store.snapshot("t", "081074000666")
	if got.LastIP != "10.0.0.1" || got.LastStatus != db.LockStatusUnlocked || got.LastCommand != "0" || !got.NotifyOKAt.Equal(notifyAt) {
		t.Fatalf("failure RMW must preserve device state, got %+v", got)
	}
}

func TestHandleLockCommandSuccess_DisabledStillClearsBackoff(t *testing.T) {
	resetLockAdaptersForTest()
	store := newFakeLockStateStore()
	setLockStateStoreForTest(store)
	defer setLockStateStoreForTest(noopLockDeviceStateStore{})

	store.states[keyOf("t", "081074000666")] = lockDeviceState{
		CommandBackoffs: map[db.DeviceLockStatus]*lockCommandBackoff{
			db.LockStatusLocked: {ConsecutiveFailures: 3},
		},
	}
	a := &Api{lockBackoff: defaultLockBackoffConfig()}
	a.handleLockCommandSuccess(context.Background(), nil, "081074000666", db.LockStatusUnlocked, "t")
	if got := store.snapshot("t", "081074000666"); len(got.CommandBackoffs) != 0 {
		t.Fatalf("successful delivery must clear stale backoff while disabled, got %+v", got.CommandBackoffs)
	}
}

func TestHandleLockCommandSuccess_PutErrorKeepsStateAndDoesNotLogRecovery(t *testing.T) {
	resetLockAdaptersForTest()
	store := newFakeLockStateStore()
	store.states[keyOf("t", "081074000666")] = lockDeviceState{
		CommandBackoffs: map[db.DeviceLockStatus]*lockCommandBackoff{
			db.LockStatusLocked: {ConsecutiveFailures: 3},
		},
	}
	store.putErr = errors.New("redis unavailable")
	setLockStateStoreForTest(store)
	defer setLockStateStoreForTest(noopLockDeviceStateStore{})

	var logs bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousWriter)

	a := &Api{lockBackoff: normalizeLockBackoffConfig(lockBackoffConfig{Enabled: true, Threshold: 3, Cooldown: 10 * time.Minute})}
	a.handleLockCommandSuccess(context.Background(), nil, "081074000666", db.LockStatusLocked, "t")

	if got := store.snapshot("t", "081074000666"); len(got.CommandBackoffs) != 1 {
		t.Fatalf("failed cleanup must preserve stored backoff, got %+v", got.CommandBackoffs)
	}
	if strings.Contains(logs.String(), "recovered tenant=") {
		t.Fatalf("failed cleanup must not emit recovery log: %s", logs.String())
	}
}

func TestApplyBackoffFailure_ActiveCooldownDoesNotExtendOrTransition(t *testing.T) {
	now := time.Date(2026, time.July, 16, 15, 0, 0, 0, time.UTC)
	deadline := now.Add(5 * time.Minute)
	prev := map[db.DeviceLockStatus]*lockCommandBackoff{
		db.LockStatusLocked: {
			ConsecutiveFailures: 3,
			CooldownUntil:       deadline,
		},
	}

	next, transitioned := applyBackoffFailure(prev, db.LockStatusLocked, now, 3, 10*time.Minute, "fourth failure")
	if transitioned {
		t.Fatal("failure during active cooldown must not report a new transition")
	}
	got := next[db.LockStatusLocked]
	if !got.CooldownUntil.Equal(deadline) {
		t.Fatalf("active cooldown deadline must remain %s, got %s", deadline, got.CooldownUntil)
	}
	if got.ConsecutiveFailures != 4 {
		t.Fatalf("failure metadata should still increment, got %d", got.ConsecutiveFailures)
	}
}

func TestFormatBackoffCommandError_PreservesSuffixWithinLimit(t *testing.T) {
	deadline := time.Date(2026, time.July, 16, 15, 15, 45, 0, time.UTC)
	got := formatBackoffCommandError(strings.Repeat("界", 600), deadline)
	suffix := " (device backoff until " + deadline.Format(time.RFC3339) + ")"
	if !strings.HasSuffix(got, suffix) {
		t.Fatalf("formatted error must preserve deadline suffix, got %q", got)
	}
	if len([]rune(got)) > maxLockBackoffErrorLen {
		t.Fatalf("formatted error exceeds %d runes: %d", maxLockBackoffErrorLen, len([]rune(got)))
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
