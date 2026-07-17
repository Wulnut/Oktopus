# ONT Lock Actual-State Reconciliation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop redundant ONT Lock Set commands by comparing policy targets with the device's actual Lock value, and migrate the legacy production Redis default to enabled exactly once.

**Architecture:** Capability probing becomes a data-producing device snapshot that retains actual Lock and WAN IP values. Fresh evaluation and retry paths use one convergence predicate before command creation or resend. Source deployment sources a small environment-migration helper whose marker makes the legacy false-to-true Redis migration one-time and operator-safe.

**Tech Stack:** Go 1.24 controller, MongoDB, Redis, NATS/USP, CWMP, Bash deployment scripts, Docker Compose test services.

## Global Constraints

- Build and test only through Docker Compose; do not run host Go or Node tools.
- Preserve tenant-scoped subjects, databases, Redis keys, and audit records.
- Unknown or malformed actual Lock values are transient and must never cause Set.
- Accept `0`/`false`/`unlocked` and `1`/`true`/`locked`, case-insensitively.
- All triggers use actual-state comparison; online and chase force a fresh read, not a Set.
- Redis failures remain soft-degraded, but stable devices must not receive repeated Set.
- The Redis default migration runs once and must not overwrite a later operator change.

---

### Task 1: Retain The Device Lock Snapshot

**Files:**
- Modify: `backend/services/controller/internal/api/lock_capability.go`
- Modify: `backend/services/controller/internal/api/lock_capability_test.go`

**Interfaces:**
- Produces: `lockCapabilitySnapshot { Result lockProbeResult; Detail string; LockStatus db.DeviceLockStatus; ReportedIP string; CWMPRoot string }`
- Produces: `normalizeActualLockValue(string) (db.DeviceLockStatus, bool)`
- Produces: `probeOntLockCapabilityUSPWithGetter(...) lockCapabilitySnapshot`
- Produces: `probeOntLockCapabilityCWMPWithGetter(...) lockCapabilitySnapshot`
- Produces: `gateLockCapability(...) (lockCapabilitySnapshot, bool)`

- [ ] **Step 1: Write normalization and snapshot extraction tests**

Add table tests that require all supported boolean representations to map to
`db.LockStatusLocked` or `db.LockStatusUnlocked`, while empty/unknown values
return `ok=false`. Change the CWMP alternate-root test to assert that the second
response returns Lock `UNLOCKED`, WAN IP `203.0.113.10`, and the resolved root.
Add a USP getter test using this test seam:

```go
type uspLockProbeGetter func(sn, path, mtp, tenantSlug string) (string, error)

func TestProbeOntLockCapabilityUSPRetainsValues(t *testing.T) {
    getter := func(_ string, path, _, _ string) (string, error) {
        switch path {
        case lockParameterPath:
            return "true", nil
        case lockWanIPPath:
            return "203.0.113.20", nil
        default:
            return "", fmt.Errorf("unexpected path %s", path)
        }
    }
    got := probeOntLockCapabilityUSPWithGetter("SN-1", "mqtt", "tenant-a", getter)
    if got.Result != lockProbeOK || got.LockStatus != db.LockStatusLocked || got.ReportedIP != "203.0.113.20" {
        t.Fatalf("unexpected snapshot: %+v", got)
    }
}
```

- [ ] **Step 2: Run the focused controller test and verify RED**

Run:

```bash
cd deploy/compose && docker compose -f docker-compose.test.yaml --profile unit run --rm test-controller-unit sh -c 'go test -count=1 ./internal/api -run "TestNormalizeActualLockValue|TestProbeOntLockCapability(USP|CWMP)"'
```

Expected: FAIL because the snapshot type, normalization function, and USP getter seam do not exist, and the CWMP helper still returns `(result, detail)`.

- [ ] **Step 3: Implement snapshot-producing probes**

Add:

```go
type lockCapabilitySnapshot struct {
    Result     lockProbeResult
    Detail     string
    LockStatus db.DeviceLockStatus
    ReportedIP string
    CWMPRoot   string
}

func normalizeActualLockValue(raw string) (db.DeviceLockStatus, bool) {
    switch strings.ToLower(strings.TrimSpace(raw)) {
    case "0", "false", "unlocked":
        return db.LockStatusUnlocked, true
    case "1", "true", "locked":
        return db.LockStatusLocked, true
    default:
        return "", false
    }
}
```

Make USP and CWMP helpers retain values. If a Lock value cannot be normalized,
return `lockProbeTransient` with a bounded detail. Change
`probeOntLockCapability`, its transport helpers, and `gateLockCapability` to
return the snapshot. Preserve existing unsupported-row and opt-out behavior.

- [ ] **Step 4: Run focused and complete API tests and verify GREEN**

Run the focused command from Step 2, then:

```bash
cd deploy/compose && docker compose -f docker-compose.test.yaml --profile unit run --rm test-controller-unit
```

Expected: PASS with no controller unit failures.

- [ ] **Step 5: Commit the snapshot change**

```bash
git add backend/services/controller/internal/api/lock_capability.go backend/services/controller/internal/api/lock_capability_test.go
git commit -m "fix(lock): retain actual device state during probing"
```

---

### Task 2: Skip Commands For Already-Converged Devices

**Files:**
- Modify: `backend/services/controller/internal/api/lock_engine.go`
- Modify: `backend/services/controller/internal/api/lock_engine_test.go`
- Modify: `backend/services/controller/internal/api/lock_scheduler.go`
- Modify: `backend/services/controller/internal/api/lock_scheduler_test.go`
- Verify: `backend/services/controller/internal/api/lock_engine_backoff_test.go`
- Verify: `backend/services/controller/internal/api/lock_ip_poller_test.go`

**Interfaces:**
- Consumes: `lockCapabilitySnapshot` from Task 1.
- Produces: `lockDecisionAlreadyConverged(actual db.DeviceLockStatus, decision LockDecision) bool`.
- Produces audit detail `command_skipped=true`, `skip_reason=actual_state_match`, and `actual_status`.

- [ ] **Step 1: Replace force-converge expectations with actual-state tests**

Remove tests tied to `shouldSkipLockCommand(trigger, RedisStatus, ...)` and add:

```go
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
```

Add scheduler tests for a pure retry convergence predicate using the same
helper, covering matching and mismatching refreshed targets.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
cd deploy/compose && docker compose -f docker-compose.test.yaml --profile unit run --rm test-controller-unit sh -c 'go test -count=1 ./internal/api -run "TestLockDecisionAlreadyConverged|TestRetry.*Converged"'
```

Expected: FAIL because the convergence helper is absent and the old trigger
force-send implementation remains.

- [ ] **Step 3: Apply actual-state reconciliation to fresh evaluation**

In `evaluateAndMaybeCommand`:

```go
snapshot, proceed := a.gateLockCapability(ctx, tdb, device, tenantSlug, trigger)
if !proceed {
    return
}
if reportedIP == "" {
    reportedIP = snapshot.ReportedIP
}
decision, reportedIP, err := a.resolveCurrentLockDecision(ctx, tdb, device, tenantSlug, reportedIP)
```

After `ShouldCommand` and before cooldown/circuit-breaker gates, skip when:

```go
func lockDecisionAlreadyConverged(actual db.DeviceLockStatus, decision LockDecision) bool {
    return decision.ShouldCommand && actual != "" && actual == decision.Status
}
```

Record the skip details, clear stale command backoffs in the next persisted
state, and do not call `sendLockCommand`. Remove trigger-based force Set and the
Redis desired-status helper. Preserve the poller's earlier LastIP optimization.

- [ ] **Step 4: Apply convergence to retries**

Capture the capability snapshot in `retryLockCommand`. Resolve policy using the
snapshot WAN IP. If the refreshed decision is already converged:

```go
_ = tdb.UpdateLockCommandStatus(ctx, command.ID, db.LockCommandSuccess, "")
a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
    SN: command.DeviceSN,
    Action: "retry_already_converged",
    Status: decision.Status,
    Details: bson.M{"reported_ip": reportedIP, "actual_status": snapshot.LockStatus},
})
```

Clear stale device backoff state using the existing successful-state helper and
return before `PrepareLockCommandResend`.

- [ ] **Step 5: Run controller tests and verify GREEN**

Run the focused command from Step 2, then:

```bash
cd deploy/compose && docker compose -f docker-compose.test.yaml --profile unit run --rm test-controller-unit
```

Expected: PASS, including failure-backoff and IP-poller regression tests.

- [ ] **Step 6: Commit reconciliation behavior**

```bash
git add backend/services/controller/internal/api/lock_engine.go backend/services/controller/internal/api/lock_engine_test.go backend/services/controller/internal/api/lock_scheduler.go backend/services/controller/internal/api/lock_scheduler_test.go
git commit -m "fix(lock): skip commands for converged devices"
```

---

### Task 3: Migrate The Legacy Redis Default Once

**Files:**
- Create: `deploy/compose/env-migrations.sh`
- Modify: `deploy/compose/ci-source-deploy.sh`
- Modify: `deploy/tests/infra_test.go`
- Modify: `AGENTS.md`

**Interfaces:**
- Produces: `migrate_env_default_once <target> <marker> <migration-id> <key> <old> <new>`.
- Consumes: `.env.controller` after `merge_missing_env_defaults`.
- Produces retained marker: `deploy/compose/.env.migrations` (already ignored by `deploy/compose/.env.*`).

- [ ] **Step 1: Add failing executable migration tests**

Import `os/exec` in `deploy/tests/infra_test.go`. Add a helper that invokes Bash,
sources `env-migrations.sh`, and calls `migrate_env_default_once` against
`t.TempDir()` files. Add tests for:

```go
func TestEnvMigration_LegacyRedisDefaultEnabledOnce(t *testing.T)
func TestEnvMigration_AlreadyEnabledRemainsEnabled(t *testing.T)
func TestEnvMigration_OperatorChangeAfterMarkerIsPreserved(t *testing.T)
```

The first expects exact `false` to become `true` and the marker id to appear.
The third runs migration once, rewrites the value to `false`, runs it again, and
expects `false` to remain.

- [ ] **Step 2: Run infra tests and verify RED**

```bash
cd deploy/compose && docker compose -f docker-compose.test.yaml --profile unit run --rm test-infra
```

Expected: FAIL because `env-migrations.sh` and its function do not exist.

- [ ] **Step 3: Implement the one-time migration helper**

Create an ASCII Bash library guarded against direct side effects. The function
must validate non-empty arguments, return immediately when the exact migration
id is already in the marker, replace only a line exactly equal to
`${key}=${old}`, and append the migration id only after a successful rewrite or
no-op. Use a temporary file plus `mv` instead of platform-specific `sed -i`.

Source it near the top of `ci-source-deploy.sh`, then after
`merge_missing_env_defaults` call:

```bash
migrate_env_default_once \
  "$SCRIPT_DIR/.env.controller" \
  "$SCRIPT_DIR/.env.migrations" \
  "20260717-lock-redis-default-true" \
  "LOCK_REDIS_ENABLED" \
  "false" \
  "true"
```

- [ ] **Step 4: Update project infrastructure documentation**

Update `AGENTS.md` to state that source deployment performs a one-time migration
of the original ONT Lock Redis default from false to true, records the migration
locally, and preserves later operator overrides.

- [ ] **Step 5: Run infra tests and verify GREEN**

```bash
cd deploy/compose && docker compose -f docker-compose.test.yaml --profile unit run --rm test-infra
```

Expected: PASS with all migration and existing infrastructure conventions.

- [ ] **Step 6: Commit deployment migration**

```bash
git add deploy/compose/env-migrations.sh deploy/compose/ci-source-deploy.sh deploy/tests/infra_test.go AGENTS.md
git commit -m "fix(deploy): migrate legacy ONT Lock Redis default"
```

---

### Task 4: Full Verification And Branch Delivery

**Files:**
- Verify all files modified in Tasks 1-3.
- Update the plan checkboxes as tasks finish.

**Interfaces:**
- Consumes the complete implementation.
- Produces a verified `telkomsel/ont-lock-dev` commit set ready for staging CI.

- [ ] **Step 1: Run complete controller and infra tests**

```bash
cd deploy/compose && docker compose -f docker-compose.test.yaml --profile unit run --rm test-controller-unit
cd deploy/compose && docker compose -f docker-compose.test.yaml --profile unit run --rm test-infra
```

Expected: both services exit 0 with no test failures.

- [ ] **Step 2: Run race verification**

```bash
cd deploy/compose && docker compose -f docker-compose.test.yaml --profile unit run --rm test-controller-race
```

Expected: exit 0 with no race reports.

- [ ] **Step 3: Build the controller image**

```bash
cd deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build controller
```

Expected: controller image builds successfully.

- [ ] **Step 4: Review the complete diff and repository state**

```bash
git diff origin/telkomsel/ont-lock-dev...HEAD --check
git status --short --branch
git log --oneline origin/telkomsel/ont-lock-dev..HEAD
```

Expected: no whitespace errors, clean worktree, and only the fast-forwarded
production commits plus design/implementation commits.

- [ ] **Step 5: Push dev and monitor staging CI**

```bash
git push origin telkomsel/ont-lock-dev
glab ci list --branch telkomsel/ont-lock-dev
```

Expected: push succeeds and the new staging pipeline reaches success before any
production merge.

- [ ] **Step 6: Merge to production only after staging success**

Compare branches, merge `telkomsel/ont-lock-dev` into `telkomsel/ont-lock`
without force-pushing, push, and wait for the production pipeline to succeed.
Then use Electerm MCP to verify Redis enabled/state keys and two stable poll
intervals without new redundant command attempts.

