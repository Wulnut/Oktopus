# ONT Lock Per-Device Command Failure Backoff — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop a persistently/repeat-failing ONT Lock device from generating an unbounded stream of fresh lock commands and retries, by adding a Redis-backed per-device + per-target-state failure backoff with a post-cooldown recovery probe.

**Architecture:** Extend the existing Redis `lockDeviceState` JSON with a `CommandBackoffs` map keyed by `LOCKED`/`UNLOCKED`. `deliverLockCommand` is the single outcome boundary: on a retryable failure it records the transition in Redis and fails the Mongo row once the per-target threshold is reached; on any success it clears all device backoff. Fresh evaluation (`evaluateAndMaybeCommand`) and the retry scheduler (`retryLockCommand`) both check the active target cooldown before creating/claiming a command. Redis is optional (soft-degrade); when disabled, behavior is identical to today.

**Tech Stack:** Go 1.x (controller service), MongoDB (per-tenant `lock_command_attempts`), Redis (per-device state via `lockStateStore`), Docker Compose, `testing`.

## Live evidence this is grounded in (Phase 1, 2026-07-16)

Production `tenant_telkomsel_general.lock_command_attempts` for `081074000666`: 347 attempts, 22 `failed/LOCKED` at `attempt_count=10`, error `usp error 7003: DATABASE_CommitTransaction(712): sqlite3_exec failed: (err=5) database is locked`. The storm is intermittent (last fired 10:07Z, device recovered by 13:51Z), so the post-cooldown probe is a load-bearing path. See `docs/superpowers/specs/2026-07-16-ont-lock-device-failure-backoff-design.md` for the full approved design.

## Global Constraints

- Code default for `LOCK_DEVICE_FAILURE_BACKOFF_ENABLED` is **false** (binary runs without compose env); compose `.env.controller.example` sets it **true**. Existing deployments receive new `LOCK_*` keys via `merge_missing_env_defaults` in `deploy/compose/ci-source-deploy.sh` — do not auto-migrate `.env.controller`.
- Threshold default **3**; values `< 1` fall back to **3**. Cooldown default **600s** (10 min); minimum **60s** (clamp).
- LOCKED and UNLOCKED backoffs are independent. Any successful command clears **all** device backoff. A failure must NOT advance `LastIP`/`LastStatus`.
- Redis Get/Put failures are logged and never block command delivery, never convert success into failure, and never convert failure into success.
- All code reading is via the **codegraph MCP** (`codegraph_explore`); the repo has a `.codegraph/` index. Do not use the Read tool / grep for source.
- All `go test` commands run **inside the controller container** via the project `run-tests` skill (repo rule: never use host `go`). Invoke `run-tests` for the exact `docker compose` invocation; the test selectors below are the `go test` args to pass.
- Target branch: `telkomsel/ont-lock-dev`. Do NOT bundle with the already-shipped `start_period` + adapter sort fix (on prod via MR !11). Ship as a separate MR to `telkomsel/ont-lock`.
- No emojis in code, commits, or communication. Conventional Commit messages.

---

## File Structure

- **Create** `backend/services/controller/internal/api/lock_backoff.go` — pure backoff state machine: `lockCommandBackoff` struct, `lockBackoffConfig`, constants, and pure helpers (`normalizeLockBackoffConfig`, `truncateBackoffError`, `backoffFor`, `isBackoffCoolingDown`, `applyBackoffFailure`, `maxBackoffFailures`). No I/O. Fully unit-testable.
- **Create** `backend/services/controller/internal/api/lock_backoff_test.go` — pure state-machine + JSON-compat tests.
- **Modify** `backend/services/controller/internal/api/lock_state.go` — add `CommandBackoffs` field to `lockDeviceState` (same package, so it can use `lockCommandBackoff`).
- **Modify** `backend/services/controller/internal/api/lock_engine.go` — `deliverLockCommand` records outcome via new `handleLockCommandFailure`/`handleLockCommandSuccess` (replacing `markLockCommandOutcome`); add the fresh-evaluation cooldown gate and state preservation in `evaluateAndMaybeCommand`.
- **Modify** `backend/services/controller/internal/api/lock_scheduler.go` — retry-path cooldown gate in `retryLockCommand`.
- **Modify** `backend/services/controller/internal/api/api.go` — add `lockBackoff lockBackoffConfig` field; wire in `NewApi`.
- **Modify** `backend/services/controller/internal/config/config.go` — add `LockDeviceFailureBackoff` struct + `Config` field + `LoadConfig` parsing.
- **Modify** `backend/services/controller/internal/api/lock_adapters.go` — startup soft-degrade log when backoff enabled but Redis disabled.
- **Create** `backend/services/controller/internal/config/config_test.go` — config parsing/validation tests.
- **Create** `backend/services/controller/internal/api/lock_engine_backoff_test.go` — command-path tests (fresh-eval cooldown, retry cooldown, success clears, failure records) using a fake store.
- **Modify** `deploy/compose/.env.controller.example` — add the three env vars.

---

## Task 1: Pure backoff state machine

**Files:**
- Create: `backend/services/controller/internal/api/lock_backoff.go`
- Test: `backend/services/controller/internal/api/lock_backoff_test.go`

**Interfaces:**
- Produces: `lockCommandBackoff` struct (fields `ConsecutiveFailures int`, `CooldownUntil time.Time`, `LastFailureAt time.Time`, `LastError string`); `lockBackoffConfig` struct (fields `Enabled bool`, `Threshold int`, `Cooldown time.Duration`); `normalizeLockBackoffConfig(lockBackoffConfig) lockBackoffConfig`; `truncateBackoffError(string) string`; `backoffFor(map, DeviceLockStatus) *lockCommandBackoff`; `isBackoffCoolingDown(map, DeviceLockStatus, time.Time) bool`; `applyBackoffFailure(prev map, status, now, threshold int, cooldown time.Duration, lastErr string) (map, bool)`; `maxBackoffFailures(map) int`.

- [ ] **Step 1: Write the failing tests**

Create `backend/services/controller/internal/api/lock_backoff_test.go`:

```go
package api

import (
	"testing"
	"time"

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
```

- [ ] **Step 2: Run tests to verify they fail**

Run (controller unit tests via `run-tests`): `go test ./internal/api/ -run 'TestNormalize|TestTruncate|TestApplyBackoffFailure|TestIsBackoffCoolingDown|TestMaxBackoffFailures' -v`
Expected: FAIL / build error — `lockCommandBackoff`, `lockBackoffConfig`, and the helpers are undefined.

- [ ] **Step 3: Write the implementation**

Create `backend/services/controller/internal/api/lock_backoff.go`:

```go
package api

import (
	"time"

	"github.com/leandrofars/oktopus/internal/db"
)

const (
	defaultLockBackoffThreshold = 3
	defaultLockBackoffCooldown  = 10 * time.Minute
	minLockBackoffCooldown      = 60 * time.Second
	maxLockBackoffErrorLen      = 512
)

// lockCommandBackoff tracks consecutive retryable delivery failures for one
// target state (LOCKED or UNLOCKED) of a single device.
type lockCommandBackoff struct {
	ConsecutiveFailures int       `json:"consecutive_failures"`
	CooldownUntil       time.Time `json:"cooldown_until,omitempty"`
	LastFailureAt       time.Time `json:"last_failure_at,omitempty"`
	LastError           string    `json:"last_error,omitempty"`
}

// lockBackoffConfig is the resolved per-device failure-backoff policy.
type lockBackoffConfig struct {
	Enabled   bool
	Threshold int
	Cooldown  time.Duration
}

func defaultLockBackoffConfig() lockBackoffConfig {
	return lockBackoffConfig{
		Enabled:   false,
		Threshold: defaultLockBackoffThreshold,
		Cooldown:  defaultLockBackoffCooldown,
	}
}

// normalizeLockBackoffConfig applies the spec validation: threshold < 1 falls
// back to 3 and cooldown < 60s is clamped to 60s.
func normalizeLockBackoffConfig(cfg lockBackoffConfig) lockBackoffConfig {
	if cfg.Threshold < 1 {
		cfg.Threshold = defaultLockBackoffThreshold
	}
	if cfg.Cooldown < minLockBackoffCooldown {
		cfg.Cooldown = minLockBackoffCooldown
	}
	return cfg
}

// truncateBackoffError caps an error string to maxLockBackoffErrorLen runes for
// Redis/audit storage.
func truncateBackoffError(s string) string {
	r := []rune(s)
	if len(r) <= maxLockBackoffErrorLen {
		return s
	}
	return string(r[:maxLockBackoffErrorLen])
}

func backoffFor(m map[db.DeviceLockStatus]*lockCommandBackoff, status db.DeviceLockStatus) *lockCommandBackoff {
	if m == nil {
		return nil
	}
	return m[status]
}

// isBackoffCoolingDown reports whether status is within an active cooldown at now.
func isBackoffCoolingDown(m map[db.DeviceLockStatus]*lockCommandBackoff, status db.DeviceLockStatus, now time.Time) bool {
	b := backoffFor(m, status)
	return b != nil && now.Before(b.CooldownUntil)
}

// applyBackoffFailure returns a NEW map with one retryable failure recorded for
// status. It does not mutate prev. The failure count is never reset on cooldown
// expiry, so the first post-cooldown failure (count already >= threshold)
// immediately starts a new cooldown. Returns the new map and cooled=true when the
// threshold was reached (cooldown now active for status).
func applyBackoffFailure(
	prev map[db.DeviceLockStatus]*lockCommandBackoff,
	status db.DeviceLockStatus,
	now time.Time,
	threshold int,
	cooldown time.Duration,
	lastErr string,
) (map[db.DeviceLockStatus]*lockCommandBackoff, bool) {
	next := make(map[db.DeviceLockStatus]*lockCommandBackoff, len(prev))
	for k, v := range prev {
		next[k] = &lockCommandBackoff{
			ConsecutiveFailures: v.ConsecutiveFailures,
			CooldownUntil:       v.CooldownUntil,
			LastFailureAt:       v.LastFailureAt,
			LastError:           v.LastError,
		}
	}
	b := next[status]
	if b == nil {
		b = &lockCommandBackoff{}
		next[status] = b
	}
	b.ConsecutiveFailures++
	b.LastFailureAt = now
	b.LastError = truncateBackoffError(lastErr)
	cooled := false
	if threshold > 0 && b.ConsecutiveFailures >= threshold {
		b.CooldownUntil = now.Add(cooldown)
		cooled = true
	}
	return next, cooled
}

// maxBackoffFailures returns the highest ConsecutiveFailures across all targets.
// Used for the recovery audit's previous_max_failures field.
func maxBackoffFailures(m map[db.DeviceLockStatus]*lockCommandBackoff) int {
	max := 0
	for _, b := range m {
		if b.ConsecutiveFailures > max {
			max = b.ConsecutiveFailures
		}
	}
	return max
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/ -run 'TestNormalize|TestTruncate|TestApplyBackoffFailure|TestIsBackoffCoolingDown|TestMaxBackoffFailures' -v`
Expected: PASS — all listed tests pass.

- [ ] **Step 5: Commit**

```bash
git add backend/services/controller/internal/api/lock_backoff.go backend/services/controller/internal/api/lock_backoff_test.go
git commit -m "feat(lock): add pure per-device failure-backoff state machine"
```

---

## Task 2: State model + JSON compatibility

**Files:**
- Modify: `backend/services/controller/internal/api/lock_state.go` (add field to `lockDeviceState`)
- Test: `backend/services/controller/internal/api/lock_backoff_test.go` (append JSON tests)

**Interfaces:**
- Consumes: `lockCommandBackoff` (Task 1).
- Produces: `lockDeviceState.CommandBackoffs map[db.DeviceLockStatus]*lockCommandBackoff` with json tag `command_backoffs,omitempty`.

- [ ] **Step 1: Write the failing tests**

Append to `backend/services/controller/internal/api/lock_backoff_test.go` (add `"encoding/json"` and `"strings"` to the import block):

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run 'TestLockDeviceState_' -v`
Expected: FAIL — `st.CommandBackoffs undefined` (compile error).

- [ ] **Step 3: Write the implementation**

In `backend/services/controller/internal/api/lock_state.go`, add the field to `lockDeviceState` (currently lines 26–32). Replace the struct with:

```go
type lockDeviceState struct {
	LastIP          string                                `json:"last_ip"`
	LastStatus      db.DeviceLockStatus                   `json:"last_status"`
	LastCommand     string                                `json:"last_command,omitempty"`
	NotifyOKAt      time.Time                             `json:"notify_ok_at,omitempty"`
	UpdatedAt       time.Time                             `json:"updated_at"`
	CommandBackoffs map[db.DeviceLockStatus]*lockCommandBackoff `json:"command_backoffs,omitempty"`
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/ -run 'TestLockDeviceState_' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/services/controller/internal/api/lock_state.go backend/services/controller/internal/api/lock_backoff_test.go
git commit -m "feat(lock): add CommandBackoffs to lockDeviceState (backward-compatible JSON)"
```

---

## Task 3: Configuration parsing

**Files:**
- Modify: `backend/services/controller/internal/config/config.go` (struct + Config field + LoadConfig parsing)
- Create: `backend/services/controller/internal/config/config_test.go`

**Interfaces:**
- Consumes: existing `lookupEnvOrBool`, `lookupEnvOrInt` helpers in `config.go`.
- Produces: `config.LockDeviceFailureBackoff{Enabled bool; Threshold int; Cooldown time.Duration}` and `Config.LockDeviceFailureBackoff`.

- [ ] **Step 1: Write the failing tests**

Create `backend/services/controller/internal/config/config_test.go`:

```go
package config

import (
	"os"
	"testing"
	"time"
)

func clearBackoffEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"LOCK_DEVICE_FAILURE_BACKOFF_ENABLED",
		"LOCK_DEVICE_FAILURE_THRESHOLD",
		"LOCK_DEVICE_FAILURE_COOLDOWN_SEC",
	} {
		os.Unsetenv(k)
	}
}

func TestParseLockDeviceFailureBackoff_DefaultsDisabled(t *testing.T) {
	clearBackoffEnv(t)
	cfg := parseLockDeviceFailureBackoff()
	if cfg.Enabled {
		t.Fatal("default must be disabled")
	}
	if cfg.Threshold != 3 {
		t.Fatalf("default threshold = %d, want 3", cfg.Threshold)
	}
	if cfg.Cooldown != 600*time.Second {
		t.Fatalf("default cooldown = %v, want 600s", cfg.Cooldown)
	}
}

func TestParseLockDeviceFailureBackoff_Enabled(t *testing.T) {
	clearBackoffEnv(t)
	t.Setenv("LOCK_DEVICE_FAILURE_BACKOFF_ENABLED", "true")
	t.Setenv("LOCK_DEVICE_FAILURE_THRESHOLD", "5")
	t.Setenv("LOCK_DEVICE_FAILURE_COOLDOWN_SEC", "120")
	cfg := parseLockDeviceFailureBackoff()
	if !cfg.Enabled || cfg.Threshold != 5 || cfg.Cooldown != 120*time.Second {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}
```

Note: `t.Setenv` automatically restores the env on test end. The helper `parseLockDeviceFailureBackoff()` is added in Step 3.

- [ ] **Step 2: Run tests to verify they fail**

Run (controller unit tests via `run-tests`): `go test ./internal/config/ -run 'TestParseLockDeviceFailureBackoff' -v`
Expected: FAIL — `parseLockDeviceFailureBackoff` undefined.

- [ ] **Step 3: Write the implementation**

In `backend/services/controller/internal/config/config.go`:

3a. Add the struct next to `LockCircuitBreaker` (after line 69):

```go
type LockDeviceFailureBackoff struct {
	Enabled   bool
	Threshold int
	Cooldown  time.Duration
}
```

3b. Add the field to the `Config` struct (find the existing `LockRetryScheduler`/`LockCircuitBreaker` fields in `Config` and add alongside):

```go
	LockDeviceFailureBackoff LockDeviceFailureBackoff
```

3c. Add the parsing helper (near the other `lookupEnvOr*` helpers, e.g. after `lookupEnvOrInt`):

```go
// parseLockDeviceFailureBackoff reads the per-device failure-backoff env vars.
// Code defaults keep the feature OFF and use threshold=3 / cooldown=600s; compose
// sets LOCK_DEVICE_FAILURE_BACKOFF_ENABLED=true. Validation (threshold<1->3,
// cooldown<60s->60s) is applied in the api package when building the runtime policy.
func parseLockDeviceFailureBackoff() LockDeviceFailureBackoff {
	return LockDeviceFailureBackoff{
		Enabled:   lookupEnvOrBool("LOCK_DEVICE_FAILURE_BACKOFF_ENABLED", false),
		Threshold: lookupEnvOrInt("LOCK_DEVICE_FAILURE_THRESHOLD", 3),
		Cooldown:  time.Duration(lookupEnvOrInt("LOCK_DEVICE_FAILURE_COOLDOWN_SEC", 600)) * time.Second,
	}
}
```

3d. Wire it into `LoadConfig`: in the `Config{ ... }` literal that `LoadConfig` returns, add the field assignment alongside the existing `LockRetryScheduler: ...` and `LockCircuitBreaker: ...` entries:

```go
		LockDeviceFailureBackoff: parseLockDeviceFailureBackoff(),
```

(If `LoadConfig` builds these inline rather than via helpers, mirror the exact `lookupEnvOrBool`/`lookupEnvOrInt` reads shown in `parseLockDeviceFailureBackoff` directly in the struct literal — the helper exists primarily for testability.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -run 'TestParseLockDeviceFailureBackoff' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/services/controller/internal/config/config.go backend/services/controller/internal/config/config_test.go
git commit -m "feat(lock): parse LOCK_DEVICE_FAILURE_BACKOFF_* env config"
```

---

## Task 4: Wire config into Api

**Files:**
- Modify: `backend/services/controller/internal/api/api.go` (struct field + `NewApi`)

**Interfaces:**
- Consumes: `config.LockDeviceFailureBackoff` (Task 3), `normalizeLockBackoffConfig` (Task 1).
- Produces: `Api.lockBackoff lockBackoffConfig`.

- [ ] **Step 1: Write the failing test**

Append to `backend/services/controller/internal/api/lock_backoff_test.go`:

```go
func TestNewApi_NormalizesBackoffConfig(t *testing.T) {
	// NewApi must normalize threshold/cooldown via normalizeLockBackoffConfig.
	api := NewApi(&config.Config{
		LockDeviceFailureBackoff: config.LockDeviceFailureBackoff{
			Enabled: true, Threshold: 0, Cooldown: 5 * time.Second,
		},
	}, nil, nil, nil, nil)
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
```

Add `"github.com/leandrofars/oktopus/internal/config"` to the test import block.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run 'TestNewApi_NormalizesBackoffConfig' -v`
Expected: FAIL — `api.lockBackoff undefined`.

- [ ] **Step 3: Write the implementation**

In `backend/services/controller/internal/api/api.go`:

3a. Add the field to the `Api` struct (next to `lockNotifyEnabled bool`, the last lock-related field):

```go
	lockBackoff                 lockBackoffConfig
```

3b. In `NewApi`, add to the returned `Api{ ... }` literal (alongside the existing `lockMaxAttempts: c.LockRetryScheduler.MaxAttempts,` line):

```go
		lockBackoff: normalizeLockBackoffConfig(lockBackoffConfig{
			Enabled:   c.LockDeviceFailureBackoff.Enabled,
			Threshold: c.LockDeviceFailureBackoff.Threshold,
			Cooldown:  c.LockDeviceFailureBackoff.Cooldown,
		}),
```

Add the import `"github.com/leandrofars/oktopus/internal/config"` to `api.go` if not already present (it is — `NewApi` takes `*config.Config`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/ -run 'TestNewApi_NormalizesBackoffConfig' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/services/controller/internal/api/api.go backend/services/controller/internal/api/lock_backoff_test.go
git commit -m "feat(lock): wire normalized backoff config into Api"
```

---

## Task 5: Outcome boundary — record failure / clear on success

**Files:**
- Modify: `backend/services/controller/internal/api/lock_engine.go`
- Test: `backend/services/controller/internal/api/lock_engine_backoff_test.go` (create)

**Interfaces:**
- Consumes: `lockBackoff` (Task 4), `applyBackoffFailure`/`maxBackoffFailures`/`truncateBackoffError` (Task 1), `lockStateStore` (`Get`/`Put`), `recordLockAudit`, `shouldMarkLockCommandForRetry`, `isPermanentLockCommandError`, db methods `UpdateLockCommandStatus`/`MarkLockCommandForRetry`.
- Produces: `Api.handleLockCommandFailure`, `Api.handleLockCommandSuccess`, `Api.recordDeviceBackoffFailure`; modified `deliverLockCommand` (replaces the `markLockCommandOutcome` call site).

- [ ] **Step 1: Write the failing tests**

Create `backend/services/controller/internal/api/lock_engine_backoff_test.go`:

```go
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
	_ = a.handleLockCommandFailure(context.Background(), nil,
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
		t.Fatalf("success must clear all backoff, got %+v", got.CommandBackoffs)
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
	_ = a.handleLockCommandFailure(context.Background(), nil,
		attemptFor("081074000666", db.LockStatusLocked, 1), db.LockStatusLocked,
		fmt.Errorf("usp error 7003: db locked"), "t")
	got := store.snapshot("t", "081074000666")
	if len(got.CommandBackoffs) != 0 {
		t.Fatal("disabled backoff must not write backoff state")
	}
}
```

Note: these tests pass `nil` for `*db.TenantDB`; the handler methods must therefore guard against a nil `tdb` when the backoff is recorded purely in Redis and no Mongo write is required for the assertion. (See Step 3 — `handleLockCommandFailure` only touches Mongo via `tdb`, and the unit tests for the Redis transition call `recordDeviceBackoffFailure` / `handleLockCommandSuccess` directly with `nil tdb` because those paths only do Redis + audit; `recordLockAudit` already tolerates a nil/real `tdb` only when `noopLockEventSink` is installed — `resetLockAdaptersForTest()` sets `lockAuditSink = noopLockEventSink{}`, but `recordLockAudit` still calls `tdb.CreateLockAuditLog`. Therefore the success/failure handlers must skip the audit when `tdb == nil`. See Step 3 implementation note.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run 'TestRecordDeviceBackoffFailure|TestHandleLockCommand' -v`
Expected: FAIL — `recordDeviceBackoffFailure`, `handleLockCommandFailure`, `handleLockCommandSuccess` undefined.

- [ ] **Step 3: Write the implementation**

In `backend/services/controller/internal/api/lock_engine.go`:

3a. Replace the body of `deliverLockCommand` (currently lines 596–607) so it records the outcome through the new handlers. Replace the whole function with:

```go
// deliverLockCommand dispatches the lock/unlock command via the device's active
// MTP protocol, then records the per-device failure backoff (on retryable
// failure) or clears all device backoff (on success). It is the single command
// outcome boundary used by both fresh commands and retries.
func (a *Api) deliverLockCommand(ctx context.Context, tdb *db.TenantDB, attempt db.LockCommandAttempt, decision LockDecision, tenantSlug string) error {
	if err := a.deliverLockCommandByMTP(attempt, decision, tenantSlug); err != nil {
		a.handleLockCommandFailure(ctx, tdb, attempt, decision.Status, err, tenantSlug)
		return err
	}

	_ = tdb.UpdateLockCommandStatus(ctx, attempt.ID, db.LockCommandSuccess, "")
	a.handleLockCommandSuccess(ctx, tdb, attempt.DeviceSN, decision.Status, tenantSlug)
	if decision.Status == db.LockStatusLocked && a.lockCircuitBreaker != nil {
		a.lockCircuitBreaker.recordLock(tenantSlug)
	}
	return nil
}
```

3b. Replace `markLockCommandOutcome` (currently lines 628–642) with `handleLockCommandFailure`. Delete the old `markLockCommandOutcome` and add:

```go
// handleLockCommandFailure records a delivery failure. Permanent schema/path
// errors keep the existing failed-status behavior and do NOT count toward
// backoff. Retryable errors record the per-device backoff transition in Redis;
// when the target threshold is reached the Mongo row is marked failed (leaving
// the retry queue) with the cooldown deadline in its error text. Below the
// threshold, the legacy retry/fail behavior is preserved. When backoff is
// disabled, this is equivalent to the former markLockCommandOutcome.
func (a *Api) handleLockCommandFailure(ctx context.Context, tdb *db.TenantDB, attempt db.LockCommandAttempt, target db.DeviceLockStatus, err error, tenantSlug string) {
	if isPermanentLockCommandError(err) {
		if tdb != nil {
			_ = tdb.UpdateLockCommandStatus(ctx, attempt.ID, db.LockCommandFailed, err.Error())
		}
		return
	}
	cooledDown := false
	deadline := time.Time{}
	if a.lockBackoff.Enabled {
		cooledDown, deadline = a.recordDeviceBackoffFailure(ctx, tdb, tenantSlug, attempt.DeviceSN, target, err)
	}
	if cooledDown {
		if tdb != nil {
			_ = tdb.UpdateLockCommandStatus(ctx, attempt.ID, db.LockCommandFailed,
				fmt.Sprintf("%s (device backoff until %s)", truncateBackoffError(err.Error()), deadline.Format(time.RFC3339)))
		}
		return
	}
	maxAttempts := a.lockMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = db.DefaultLockCommandMaxAttempts
	}
	if shouldMarkLockCommandForRetry(attempt.AttemptCount, maxAttempts) {
		if tdb != nil {
			_ = tdb.MarkLockCommandForRetry(ctx, attempt.ID, err.Error())
		}
		return
	}
	if tdb != nil {
		_ = tdb.UpdateLockCommandStatus(ctx, attempt.ID, db.LockCommandFailed, err.Error())
	}
}

// recordDeviceBackoffFailure records one retryable failure in the device's Redis
// state (read-modify-write preserving LastIP/LastStatus). Returns cooled=true and
// the deadline when this failure reached/re-extended the cooldown threshold, and
// emits the device_command_backoff_started transition log + audit. Redis errors
// are logged and never block.
func (a *Api) recordDeviceBackoffFailure(ctx context.Context, tdb *db.TenantDB, tenantSlug, sn string, target db.DeviceLockStatus, err error) (bool, time.Time) {
	state, found, errGet := lockStateStore.Get(ctx, tenantSlug, sn)
	if errGet != nil {
		log.Printf("lock_backoff: get state %s: %v", sn, errGet)
		return false, time.Time{}
	}
	var prev map[db.DeviceLockStatus]*lockCommandBackoff
	if found {
		prev = state.CommandBackoffs
	}
	now := time.Now()
	next, cooled := applyBackoffFailure(prev, target, now, a.lockBackoff.Threshold, a.lockBackoff.Cooldown, err.Error())
	state.CommandBackoffs = next
	state.UpdatedAt = now
	if errPut := lockStateStore.Put(ctx, tenantSlug, sn, state); errPut != nil {
		log.Printf("lock_backoff: put state %s: %v", sn, errPut)
	}
	if !cooled {
		return false, time.Time{}
	}
	b := next[target]
	log.Printf("lock_command_backoff: tenant=%s sn=%s target=%s failures=%d until=%s error=%s",
		tenantSlug, sn, target, b.ConsecutiveFailures, b.CooldownUntil.Format(time.RFC3339), truncateBackoffError(err.Error()))
	if tdb != nil {
		a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
			SN:     sn,
			Action: "device_command_backoff_started",
			Status: target,
			Details: bson.M{
				"target_status":        target,
				"consecutive_failures": b.ConsecutiveFailures,
				"cooldown_until":       b.CooldownUntil,
				"last_error":           truncateBackoffError(err.Error()),
			},
		})
	}
	return true, b.CooldownUntil
}

// handleLockCommandSuccess clears all per-device backoff after a successful
// delivery and emits device_command_backoff_recovered when backoff was active.
// Redis errors are logged and never block; success is never converted to failure.
func (a *Api) handleLockCommandSuccess(ctx context.Context, tdb *db.TenantDB, sn string, target db.DeviceLockStatus, tenantSlug string) {
	if !a.lockBackoff.Enabled {
		return
	}
	state, found, err := lockStateStore.Get(ctx, tenantSlug, sn)
	if err != nil {
		log.Printf("lock_backoff: get state on success %s: %v", sn, err)
		return
	}
	if !found || len(state.CommandBackoffs) == 0 {
		return
	}
	prevMax := maxBackoffFailures(state.CommandBackoffs)
	cleared := make([]string, 0, len(state.CommandBackoffs))
	for k := range state.CommandBackoffs {
		cleared = append(cleared, string(k))
	}
	state.CommandBackoffs = nil
	state.UpdatedAt = time.Now()
	if errPut := lockStateStore.Put(ctx, tenantSlug, sn, state); errPut != nil {
		log.Printf("lock_backoff: put state on success %s: %v", sn, errPut)
	}
	log.Printf("lock_command_backoff: recovered tenant=%s sn=%s successful_target=%s previous_failures=%d",
		tenantSlug, sn, target, prevMax)
	if tdb != nil {
		a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
			SN:     sn,
			Action: "device_command_backoff_recovered",
			Status: target,
			Details: bson.M{
				"successful_target_status": target,
				"cleared_targets":          cleared,
				"previous_max_failures":    prevMax,
			},
		})
	}
}
```

Ensure the `fmt` import is present in `lock_engine.go` (it is — `isPermanentLockCommandError` uses `fmt.Sprintf`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/ -run 'TestRecordDeviceBackoffFailure|TestHandleLockCommand' -v`
Expected: PASS. Then run the full api package to confirm no regression: `go test ./internal/api/ -v`.
Expected: PASS (all existing + new tests green).

- [ ] **Step 5: Commit**

```bash
git add backend/services/controller/internal/api/lock_engine.go backend/services/controller/internal/api/lock_engine_backoff_test.go
git commit -m "feat(lock): record per-device backoff at the deliverLockCommand boundary"
```

---

## Task 6: Cooldown enforcement — fresh evaluation + state preservation

**Files:**
- Modify: `backend/services/controller/internal/api/lock_engine.go` (`evaluateAndMaybeCommand`)

**Interfaces:**
- Consumes: `a.lockBackoff.Enabled`, `isBackoffCoolingDown` (Task 1), `prev.CommandBackoffs` (Task 2).

- [ ] **Step 1: Write the failing test**

Append to `backend/services/controller/internal/api/lock_engine_backoff_test.go`:

```go
func TestEvaluateCooldownSkipReturnsPriorStateUnchanged(t *testing.T) {
	resetLockAdaptersForTest()
	store := newFakeLockStateStore()
	setLockStateStoreForTest(store)
	defer setLockStateStoreForTest(noopLockDeviceStateStore{})

	// Device has an active LOCKED cooldown and a prior IP/status that must NOT be
	// advanced (else the post-cooldown probe would be masked).
	store.states[keyOf("t", "081074000666")] = lockDeviceState{
		LastIP:     "10.0.0.1",
		LastStatus: db.LockStatusLocked,
		CommandBackoffs: map[db.DeviceLockStatus]*lockCommandBackoff{
			db.LockStatusLocked: {ConsecutiveFailures: 3, CooldownUntil: time.Now().Add(5 * time.Minute)},
		},
	}

	a := &Api{lockBackoff: normalizeLockBackoffConfig(lockBackoffConfig{Enabled: true, Threshold: 3, Cooldown: 10 * time.Minute})}
	// evaluateFreshCooldown returns true when the decision target is cooling down.
	if !a.evaluateFreshCooldown(context.Background(), "t", "081074000666", db.LockStatusLocked) {
		t.Fatal("LOCKED must be suppressed during active cooldown")
	}
	// Opposite target is allowed.
	if a.evaluateFreshCooldown(context.Background(), "t", "081074000666", db.LockStatusUnlocked) {
		t.Fatal("UNLOCK must remain allowed during a LOCKED cooldown")
	}
	// State untouched.
	got := store.snapshot("t", "081074000666")
	if got.LastIP != "10.0.0.1" || got.LastStatus != db.LockStatusLocked {
		t.Fatal("cooldown skip must not advance LastIP/LastStatus")
	}
	if got.CommandBackoffs[db.LockStatusLocked].ConsecutiveFailures != 3 {
		t.Fatal("cooldown skip must not mutate backoff")
	}
}
```

This test drives a thin helper `evaluateFreshCooldown(sn, target) bool` (added in Step 3) that encapsulates the exact cooldown predicate used inline in `evaluateAndMaybeCommand`. Keeping the predicate in a helper makes the gate unit-testable without standing up the whole evaluate pipeline (which needs NATS/Mongo).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run 'TestEvaluateCooldownSkip' -v`
Expected: FAIL — `evaluateFreshCooldown` undefined.

- [ ] **Step 3: Write the implementation**

In `backend/services/controller/internal/api/lock_engine.go`:

3a. Add the helper near `shouldSkipLockCommand`:

```go
// evaluateFreshCooldown reports whether a fresh command for target should be
// suppressed because the device is in an active per-target cooldown. Reads the
// stored device state; on Redis error or missing state it allows the command
// (soft-degrade). The caller must NOT advance LastIP/LastStatus when this
// returns true, so the post-cooldown recovery probe is not masked.
func (a *Api) evaluateFreshCooldown(ctx context.Context, tenantSlug, sn string, target db.DeviceLockStatus) bool {
	if !a.lockBackoff.Enabled {
		return false
	}
	state, found, err := lockStateStore.Get(ctx, tenantSlug, sn)
	if err != nil || !found {
		return false
	}
	return isBackoffCoolingDown(state.CommandBackoffs, target, time.Now())
}
```

3b. Add the cooldown gate inside `evaluateAndMaybeCommand`, immediately after the `shouldSkipLockCommand` block (after current line 473, before the `recordLockAudit(... "evaluate" ...)` at line 475). Insert:

```go
	if a.evaluateFreshCooldown(ctx, tenantSlug, device.SN, decision.Status) {
		// Active per-target cooldown: do not create a command, do not send, and do
		// NOT advance LastIP/LastStatus (that would mask the post-cooldown probe).
		// Leave prior Redis state intact by returning without Putting nextState.
		return
	}
```

3c. Preserve backoff in the `nextState` construction (currently lines 434–446) for the non-command paths that Put `nextState`. In the `if found { ... }` block (currently lines 440–443), add one line so it reads:

```go
	if found {
		nextState.LastCommand = prev.LastCommand
		nextState.NotifyOKAt = prev.NotifyOKAt
		nextState.CommandBackoffs = prev.CommandBackoffs
	}
```

3d. On the success path (currently lines 499–502), clear backoff because success clears all device backoff (the Redis clear is also performed by `handleLockCommandSuccess` inside `deliverLockCommand`; setting it nil here keeps this final Put consistent). Replace the block with:

```go
	nextState.LastCommand = decision.CommandValue
	nextState.CommandBackoffs = nil // success clears all device backoff
	if err := lockStateStore.Put(ctx, tenantSlug, device.SN, nextState); err != nil {
		log.Printf("lock_engine: put device state %s: %v", device.SN, err)
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/ -run 'TestEvaluateCooldownSkip' -v` then `go test ./internal/api/ -v`
Expected: PASS (new + existing).

- [ ] **Step 5: Commit**

```bash
git add backend/services/controller/internal/api/lock_engine.go backend/services/controller/internal/api/lock_engine_backoff_test.go
git commit -m "feat(lock): suppress fresh commands during per-target cooldown"
```

---

## Task 7: Cooldown enforcement — retry scheduler

**Files:**
- Modify: `backend/services/controller/internal/api/lock_scheduler.go` (`retryLockCommand`)

**Interfaces:**
- Consumes: `a.lockBackoff.Enabled`, `isBackoffCoolingDown` (Task 1).

- [ ] **Step 1: Write the failing test**

Append to `backend/services/controller/internal/api/lock_engine_backoff_test.go`:

```go
func TestRetryCooldownSuppressedDecision(t *testing.T) {
	resetLockAdaptersForTest()
	store := newFakeLockStateStore()
	setLockStateStoreForTest(store)
	defer setLockStateStoreForTest(noopLockDeviceStateStore{})

	deadline := time.Now().Add(5 * time.Minute)
	store.states[keyOf("t", "081074000666")] = lockDeviceState{
		CommandBackoffs: map[db.DeviceLockStatus]*lockCommandBackoff{
			db.LockStatusLocked: {ConsecutiveFailures: 3, CooldownUntil: deadline},
		},
	}

	a := &Api{lockBackoff: normalizeLockBackoffConfig(lockBackoffConfig{Enabled: true, Threshold: 3, Cooldown: 10 * time.Minute})}
	msg, suppress := a.retryCooldownMessage(context.Background(), "t", "081074000666", db.LockStatusLocked)
	if !suppress {
		t.Fatal("retry during active cooldown must be suppressed")
	}
	if !strings.Contains(msg, "suppressed by device cooldown") || !strings.Contains(msg, deadline.Format(time.RFC3339)) {
		t.Fatalf("suppression message must carry the deadline, got %q", msg)
	}

	// Opposite target / expired cooldown -> not suppressed.
	if _, suppress := a.retryCooldownMessage(context.Background(), "t", "081074000666", db.LockStatusUnlocked); suppress {
		t.Fatal("UNLOCK retry must not be suppressed by a LOCKED cooldown")
	}
	store.states[keyOf("t", "081074000666")] = lockDeviceState{
		CommandBackoffs: map[db.DeviceLockStatus]*lockCommandBackoff{
			db.LockStatusLocked: {ConsecutiveFailures: 3, CooldownUntil: time.Now().Add(-1 * time.Minute)},
		},
	}
	if _, suppress := a.retryCooldownMessage(context.Background(), "t", "081074000666", db.LockStatusLocked); suppress {
		t.Fatal("expired cooldown must not suppress")
	}
}
```

Add `"strings"` to the test import block. The helper `retryCooldownMessage(sn, target) (string, bool)` is added in Step 3.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run 'TestRetryCooldownSuppressedDecision' -v`
Expected: FAIL — `retryCooldownMessage` undefined.

- [ ] **Step 3: Write the implementation**

In `backend/services/controller/internal/api/lock_scheduler.go`:

3a. Add the helper (near `shouldMarkLockCommandForRetry`):

```go
// retryCooldownMessage returns the suppression message and true when a retry for
// target must be skipped because the device is in an active per-target cooldown.
// Returns ("", false) when backoff is disabled, Redis is unavailable, no state
// exists, or the cooldown has expired.
func (a *Api) retryCooldownMessage(ctx context.Context, tenantSlug, sn string, target db.DeviceLockStatus) (string, bool) {
	if !a.lockBackoff.Enabled {
		return "", false
	}
	state, found, err := lockStateStore.Get(ctx, tenantSlug, sn)
	if err != nil || !found {
		return "", false
	}
	b := backoffFor(state.CommandBackoffs, target)
	if b == nil || !time.Now().Before(b.CooldownUntil) {
		return "", false
	}
	return fmt.Sprintf("suppressed by device cooldown until %s", b.CooldownUntil.Format(time.RFC3339)), true
}
```

3b. Add the gate inside `retryLockCommand`, immediately after the breaker check (after current line 120) and before `PrepareLockCommandResend` (current line 122). Insert:

```go
	if msg, suppress := a.retryCooldownMessage(ctx, tenantSlug, command.DeviceSN, decision.Status); suppress {
		_ = tdb.UpdateLockCommandStatus(ctx, command.ID, db.LockCommandFailed, msg)
		return
	}
```

This marks the old retry row failed (so it leaves the retry queue) WITHOUT calling `PrepareLockCommandResend`, so `attempt_count` is not incremented — matching the spec's "drain legacy retry rows" requirement.

Add `"fmt"` to the import block of `lock_scheduler.go` (currently imports `context`, `log`, `time`, `db`, `bson`, `mongo`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/ -run 'TestRetryCooldownSuppressedDecision' -v` then `go test ./internal/api/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/services/controller/internal/api/lock_scheduler.go backend/services/controller/internal/api/lock_engine_backoff_test.go
git commit -m "feat(lock): suppress retry claims during per-target cooldown"
```

---

## Task 8: Startup soft-degrade log + compose env defaults

**Files:**
- Modify: `backend/services/controller/internal/api/lock_adapters.go`
- Modify: `deploy/compose/.env.controller.example`

**Interfaces:**
- Consumes: `config.LockDeviceFailureBackoff.Enabled`, `config.LockScale.RedisEnabled`.

- [ ] **Step 1: Locate the Redis-init site (read-only, via codegraph)**

Run: `codegraph_explore` query `"initLockStateStore OR setLockStateStore OR redisLockBackend assignment in lock_adapters.go; the function that assigns the package-level lockStateStore to a redisLockBackend gated on LockScale.RedisEnabled"`.
Identify the exact function and line where `lockStateStore = &redisLockBackend{...}` (or equivalent) is conditionally assigned.

- [ ] **Step 2: Add the soft-degrade log**

In that Redis-init function in `backend/services/controller/internal/api/lock_adapters.go`, immediately after the branch that decides whether Redis is enabled, add (adjusting the struct field names to the actual config arg names used by that function):

```go
	if cfg.LockDeviceFailureBackoff.Enabled && !cfg.LockScale.RedisEnabled {
		log.Printf("lock_backoff: enabled but Redis is disabled; soft-degrading (no persistent backoff state)")
	}
```

If the init function receives the config fields by value rather than the whole `config.Config`, reference the same two booleans it already uses (the Redis-enabled gate and the backoff-enabled flag) — the intent is: log once at startup when backoff is on but Redis is off.

- [ ] **Step 3: Add the compose env defaults**

In `deploy/compose/.env.controller.example`, append after the `LOCK_CIRCUIT_BREAKER_WINDOW_SEC=60` line (current line 30):

```
# Per-device command failure backoff: after N retryable failures against the same
# target state (LOCKED/UNLOCKED), stop commanding that target for the cooldown,
# then probe once. Any successful command clears all device backoff.
LOCK_DEVICE_FAILURE_BACKOFF_ENABLED=true
LOCK_DEVICE_FAILURE_THRESHOLD=3
LOCK_DEVICE_FAILURE_COOLDOWN_SEC=600
```

- [ ] **Step 4: Verify build + config renders**

Run (via `run-tests`): `go build ./...` inside the controller container.
Expected: builds cleanly.
Then validate the compose env file is still well-formed:
```bash
sg docker -c "cd deploy/compose && docker compose --env-file .env.controller.example config >/dev/null && echo OK"
```
Expected: `OK` (or, if `.env.controller` is required to exist, create it from the example first via the project's `generate-secrets.sh` flow — do not commit a real `.env.controller`).

- [ ] **Step 5: Commit**

```bash
git add backend/services/controller/internal/api/lock_adapters.go deploy/compose/.env.controller.example
git commit -m "feat(lock): soft-degrade log + compose defaults for device failure backoff"
```

---

## Task 9: Full suite, CLAUDE.md note, branch verification

**Files:**
- Modify: `CLAUDE.md` (add the three env vars to the ONT Lock controller env table)

- [ ] **Step 1: Update CLAUDE.md**

In `CLAUDE.md`, in the "ONT Lock controller env" table (under the `LOCK_GREENPLUM_*` row), add:

```markdown
| `LOCK_DEVICE_FAILURE_BACKOFF_ENABLED` / `LOCK_DEVICE_FAILURE_THRESHOLD` / `LOCK_DEVICE_FAILURE_COOLDOWN_SEC` | Per-device + per-target retryable-failure backoff (threshold 3, 10-min cooldown, post-cooldown probe, clears on any success). Code default disabled; compose true |
```

Also append one line to the **Behavior:** paragraph below that table:

```markdown
Per-device failure backoff: after 3 retryable delivery failures against the same target state, that target is cooled down for 10 minutes; the opposite target and a post-cooldown probe remain allowed; any successful command clears all device backoff. State lives in the existing `lockDeviceState` Redis JSON. Soft-degrades when Redis is down; disabled by default in code.
```

- [ ] **Step 2: Run the full controller test suite**

Run (via `run-tests`): controller unit tests `go test ./internal/... -v` plus the existing ONT Lock DB/integration tests (poll, notify, chase, retry, circuit breaker).
Expected: all PASS.

- [ ] **Step 3: Build all images**

```bash
sg docker -c "cd deploy/compose && ./build.sh controller"
```
Expected: image builds.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(lock): document per-device failure backoff env + behavior"
```

- [ ] **Step 5: Push and verify staging deploy**

```bash
git push origin telkomsel/ont-lock-dev
```
Watch the GitLab pipeline (via electerm Jumpserver session or `glab ci view`): `test-unit` → `test-integration` → `deploy:staging` must all pass.
Expected: staging pipeline green; on staging verify (via Jumpserver) that the controller starts cleanly, `/readyz` returns 200, and there are no new panic/error patterns in controller logs.

- [ ] **Step 6: Separate MR to `telkomsel/ont-lock`**

Open a MR `telkomsel/ont-lock-dev` → `telkomsel/ont-lock` (do NOT bundle with MR !11 which is already on prod). After merge, `deploy:production` must succeed. Through the Jumpserver session, verify on prod:
- Controller healthy, `/readyz` 200.
- For a repeat-failing device, `lock_command_attempts` shows ~2–3 failures then no same-target delivery until the 10-minute probe (check `device_command_backoff_started` / `_recovered` audit rows in `lock_audit_logs`).
- Existing retry rows for cooled devices become `failed` without increasing `attempt_count`.
- The adapter `$sort` error count remains zero.
- Other devices continue receiving commands normally.

---

## Self-Review (completed during authoring)

- **Spec coverage:** State model (Task 1–2), Configuration (Task 3–4, 8), Result recording boundary (Task 5), Cooldown enforcement fresh (Task 6) + retry (Task 7), State preservation (Task 6 3c/3d + retry path naturally preserves via read-modify-write), Error classification (Task 5 permanent vs retryable), Logging & audit transitions (Task 5), Soft-degrade (Task 5 nil-guards + Task 8 startup log + disabled-config equivalence), Tests (Tasks 1, 2, 3, 4, 5, 6, 7), Deployment & verification (Task 9), Rollback (disable via env, no schema change — covered by disabled-path equivalence in Task 5). All spec sections mapped.
- **Placeholder scan:** none — every code step shows the full code; Task 8 Step 1 is a codegraph lookup of an exact line, with the concrete code to add in Step 2.
- **Type consistency:** `lockCommandBackoff`, `lockBackoffConfig`, `handleLockCommandFailure`, `handleLockCommandSuccess`, `recordDeviceBackoffFailure`, `evaluateFreshCooldown`, `retryCooldownMessage`, `applyBackoffFailure`, `isBackoffCoolingDown`, `backoffFor`, `maxBackoffFailures`, `normalizeLockBackoffConfig`, `truncateBackoffError` — names match across all tasks. `lockDeviceState.CommandBackoffs` field name consistent everywhere.
