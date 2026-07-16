package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leandrofars/oktopus/internal/bridge"
	"github.com/leandrofars/oktopus/internal/config"
	"github.com/leandrofars/oktopus/internal/db"
)

func fixedNow() time.Time { return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC) }

func TestNormalizeLockBackoffConfig_DefaultsAndClamps(t *testing.T) {
	got := normalizeLockBackoffConfig(lockBackoffConfig{Enabled: true, Threshold: 0, Cooldown: 10 * time.Second})
	if got.Threshold != 3 {
		t.Fatalf("threshold<1 should fall back to 3, got %d", got.Threshold)
	}
	if got.Cooldown != 60*time.Second {
		t.Fatalf("cooldown<60s should clamp to 60s, got %v", got.Cooldown)
	}
	if !got.Enabled {
		t.Fatal("enabled should be preserved")
	}
}

func TestNormalizeLockBackoffConfig_KeepsValid(t *testing.T) {
	got := normalizeLockBackoffConfig(lockBackoffConfig{Enabled: false, Threshold: 5, Cooldown: 2 * time.Minute})
	if got.Threshold != 5 || got.Cooldown != 2*time.Minute {
		t.Fatalf("valid values should be kept, got %+v", got)
	}
}

func TestTruncateBackoffError(t *testing.T) {
	long := make([]rune, 600)
	for i := range long {
		long[i] = 'x'
	}
	got := truncateBackoffError(string(long))
	if len([]rune(got)) != 512 {
		t.Fatalf("expected 512 runes, got %d", len([]rune(got)))
	}
	if truncateBackoffError("short") != "short" {
		t.Fatal("short string should be unchanged")
	}
}

func TestApplyBackoffFailure_BelowThresholdNoCooldown(t *testing.T) {
	now := fixedNow()
	m, cooled := applyBackoffFailure(nil, db.LockStatusLocked, now, 3, 10*time.Minute, "usp error 7003: db locked")
	if cooled {
		t.Fatal("first failure must not cool down")
	}
	b := m[db.LockStatusLocked]
	if b == nil || b.ConsecutiveFailures != 1 {
		t.Fatalf("expected 1 failure, got %+v", b)
	}
	if !b.CooldownUntil.IsZero() {
		t.Fatalf("expected zero cooldown before threshold, got %v", b.CooldownUntil)
	}
	if b.LastError != "usp error 7003: db locked" {
		t.Fatalf("last error not stored: %q", b.LastError)
	}
}

func TestApplyBackoffFailure_ThirdFailureStartsCooldown(t *testing.T) {
	now := fixedNow()
	prev := map[db.DeviceLockStatus]*lockCommandBackoff{
		db.LockStatusLocked: {ConsecutiveFailures: 2},
	}
	m, cooled := applyBackoffFailure(prev, db.LockStatusLocked, now, 3, 10*time.Minute, "err")
	if !cooled {
		t.Fatal("third failure must start cooldown")
	}
	b := m[db.LockStatusLocked]
	if b.ConsecutiveFailures != 3 {
		t.Fatalf("expected 3 failures, got %d", b.ConsecutiveFailures)
	}
	want := now.Add(10 * time.Minute)
	if !b.CooldownUntil.Equal(want) {
		t.Fatalf("cooldown deadline = %v, want %v", b.CooldownUntil, want)
	}
}

func TestApplyBackoffFailure_DoesNotMutatePrev(t *testing.T) {
	now := fixedNow()
	prev := map[db.DeviceLockStatus]*lockCommandBackoff{
		db.LockStatusLocked: {ConsecutiveFailures: 2},
	}
	_, _ = applyBackoffFailure(prev, db.LockStatusLocked, now, 3, 10*time.Minute, "err")
	if prev[db.LockStatusLocked].ConsecutiveFailures != 2 {
		t.Fatal("applyBackoffFailure must not mutate the input map")
	}
}

func TestApplyBackoffFailure_OppositeTargetUnaffectedAndIndependent(t *testing.T) {
	now := fixedNow()
	prev := map[db.DeviceLockStatus]*lockCommandBackoff{
		db.LockStatusLocked: {ConsecutiveFailures: 3, CooldownUntil: now.Add(5 * time.Minute)},
	}
	// A UNLOCK failure must not touch the LOCKED cooldown.
	m, cooled := applyBackoffFailure(prev, db.LockStatusUnlocked, now, 3, 10*time.Minute, "unlock err")
	if cooled {
		t.Fatal("first UNLOCK failure must not cool down")
	}
	if m[db.LockStatusLocked].ConsecutiveFailures != 3 {
		t.Fatal("LOCKED counter must be preserved on UNLOCK failure")
	}
	if !m[db.LockStatusLocked].CooldownUntil.Equal(now.Add(5 * time.Minute)) {
		t.Fatal("LOCKED cooldown deadline must be preserved on UNLOCK failure")
	}
	if m[db.LockStatusUnlocked].ConsecutiveFailures != 1 {
		t.Fatal("UNLOCK failure must increment UNLOCK counter")
	}
}

func TestApplyBackoffFailure_PostCooldownFailureRecoolsImmediately(t *testing.T) {
	// Count is NOT reset when cooldown expires: the first post-cooldown failure
	// (count already >= threshold) immediately starts a new cooldown.
	now := fixedNow()
	prev := map[db.DeviceLockStatus]*lockCommandBackoff{
		db.LockStatusLocked: {ConsecutiveFailures: 3, CooldownUntil: now.Add(-1 * time.Minute)}, // expired
	}
	m, cooled := applyBackoffFailure(prev, db.LockStatusLocked, now, 3, 10*time.Minute, "err")
	if !cooled {
		t.Fatal("post-cooldown failure must immediately re-cool")
	}
	if m[db.LockStatusLocked].ConsecutiveFailures != 4 {
		t.Fatalf("expected count incremented to 4, got %d", m[db.LockStatusLocked].ConsecutiveFailures)
	}
}

func TestIsBackoffCoolingDown(t *testing.T) {
	now := fixedNow()
	m := map[db.DeviceLockStatus]*lockCommandBackoff{
		db.LockStatusLocked: {ConsecutiveFailures: 3, CooldownUntil: now.Add(5 * time.Minute)},
	}
	if !isBackoffCoolingDown(m, db.LockStatusLocked, now) {
		t.Fatal("LOCKED should be cooling down")
	}
	if isBackoffCoolingDown(m, db.LockStatusUnlocked, now) {
		t.Fatal("UNLOCK should not be cooling down")
	}
	if isBackoffCoolingDown(m, db.LockStatusLocked, now.Add(6*time.Minute)) {
		t.Fatal("after deadline, should not be cooling down")
	}
	if isBackoffCoolingDown(nil, db.LockStatusLocked, now) {
		t.Fatal("nil map should not be cooling down")
	}
}

func TestMaxBackoffFailures(t *testing.T) {
	m := map[db.DeviceLockStatus]*lockCommandBackoff{
		db.LockStatusLocked:   {ConsecutiveFailures: 3},
		db.LockStatusUnlocked: {ConsecutiveFailures: 5},
	}
	if maxBackoffFailures(m) != 5 {
		t.Fatal("expected max 5")
	}
	if maxBackoffFailures(nil) != 0 {
		t.Fatal("nil map max should be 0")
	}
}

func TestLockDeviceState_DecodesOldJSONWithoutBackoff(t *testing.T) {
	old := `{"last_ip":"10.0.0.1","last_status":"LOCKED","last_command":"1","updated_at":"2026-07-16T12:00:00Z"}`
	var st lockDeviceState
	if err := json.Unmarshal([]byte(old), &st); err != nil {
		t.Fatalf("old json must decode: %v", err)
	}
	if st.LastIP != "10.0.0.1" || st.LastStatus != db.LockStatusLocked || st.LastCommand != "1" {
		t.Fatalf("fields mismatch: %+v", st)
	}
	if st.CommandBackoffs != nil {
		t.Fatal("old json must leave backoff nil")
	}
}

func TestLockDeviceState_OmitsEmptyBackoff(t *testing.T) {
	st := lockDeviceState{LastIP: "10.0.0.1", LastStatus: db.LockStatusLocked, UpdatedAt: fixedNow()}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "command_backoffs") {
		t.Fatalf("empty backoff must be omitted: %s", b)
	}
}

func TestLockDeviceState_RoundTripsBackoff(t *testing.T) {
	st := lockDeviceState{
		LastIP:      "10.0.0.1",
		LastStatus:  db.LockStatusLocked,
		LastCommand: "1",
		UpdatedAt:   fixedNow(),
		CommandBackoffs: map[db.DeviceLockStatus]*lockCommandBackoff{
			db.LockStatusLocked: {ConsecutiveFailures: 3, CooldownUntil: fixedNow().Add(10 * time.Minute), LastError: "x"},
		},
	}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var out lockDeviceState
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.LastIP != st.LastIP || out.LastCommand != st.LastCommand || out.LastStatus != st.LastStatus {
		t.Fatalf("roundtrip lost core fields: %+v", out)
	}
	if len(out.CommandBackoffs) != 1 || out.CommandBackoffs[db.LockStatusLocked].ConsecutiveFailures != 3 {
		t.Fatalf("roundtrip lost backoff: %+v", out.CommandBackoffs)
	}
}

func TestNewApi_NormalizesBackoffConfig(t *testing.T) {
	// NewApi must normalize threshold/cooldown via normalizeLockBackoffConfig.
	api := NewApi(&config.Config{
		LockDeviceFailureBackoff: config.LockDeviceFailureBackoff{
			Enabled: true, Threshold: 0, Cooldown: 5 * time.Second,
		},
	}, nil, nil, bridge.Bridge{}, db.Database{})
	if !api.lockBackoff.Enabled {
		t.Fatal("enabled must be preserved")
	}
	if api.lockBackoff.Threshold != 3 {
		t.Fatalf("threshold must normalize to 3, got %d", api.lockBackoff.Threshold)
	}
	if api.lockBackoff.Cooldown != 60*time.Second {
		t.Fatalf("cooldown must clamp to 60s, got %v", api.lockBackoff.Cooldown)
	}
}
