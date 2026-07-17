# ONT Lock Per-Device Command Failure Backoff Design

Date: 2026-07-16
Status: Approved design
Target branch: `telkomsel/ont-lock-dev`

## Summary

Add a Redis-backed, per-device and per-target-state failure backoff for ONT Lock command delivery. The backoff prevents a persistently failing device from generating an unbounded stream of new commands and retries while preserving immediate recovery when the required command direction changes.

The initial production case is device `081074000666`, which repeatedly returns USP error 7003 with `DATABASE_CommitTransaction ... sqlite3_exec failed: database is locked`. The design applies to every command delivery error already classified as retryable, not only error 7003.

## Root Cause

The existing single-command retry limit is not the source of the sustained command storm:

1. A lock command attempt is limited to 10 deliveries by `LOCK_COMMAND_MAX_ATTEMPTS`.
2. On a retryable delivery failure, `deliverLockCommand` marks the Mongo command row for retry.
3. A failed fresh command causes `evaluateAndMaybeCommand` to return before writing `LastIP` and `LastStatus` into `lockDeviceState`.
4. The IP poller, chase, notify, or online path evaluates the device again and creates a new Mongo command because the device still appears unconverged.
5. Each new command receives its own retry budget.

The result is amplification across command records: repeated fresh command creation multiplied by retries per command. The existing circuit breaker cannot stop this because it tracks successful tenant-wide LOCK delivery rate, not failures for an individual device.

## Goals

- Stop fresh ONT Lock command creation for a device and target state after repeated retryable delivery failures.
- Stop existing retry rows from consuming attempts or occupying the retry queue while a device is cooling down.
- Keep LOCK and UNLOCK backoff independent so a failed LOCK never delays a required UNLOCK, and vice versa.
- Probe automatically after the cooldown and recover without operator intervention.
- Clear all backoff state when any command delivery succeeds, because success proves the device write path has recovered.
- Preserve the current Redis-optional, soft-degrade behavior.
- Provide transition-level logs and audit records without producing a new skip/audit storm.

## Non-Goals

- Changing the existing per-command maximum attempt count.
- Adding tenant UI controls or tenant-level Mongo configuration.
- Adding a Mongo collection or index for backoff state.
- Special-casing only USP error 7003.
- Changing unsupported schema/path handling.
- Solving the device's internal SQLite lock itself.

## Chosen Approach

Extend the existing Redis `lockDeviceState` JSON. This keeps the state alongside the device's last IP/status, reuses the existing per-device `TryLock` serialization, remains backward compatible with existing Redis JSON, and follows the established Redis soft-degrade architecture.

Alternatives rejected:

- A separate Redis key and Lua/INCR state machine would provide stronger standalone atomicity but duplicate the existing per-device lock and add key lifecycle complexity.
- Mongo persistence would improve queryability but add synchronous database writes, schema/index/cleanup work, and a new dependency in a feature that currently treats Redis-backed scale controls as optional.

## Configuration

Add a `LockDeviceFailureBackoff` configuration section and wire it into `Config` and `Api`.

| Environment variable | Compose example default | Code default | Validation |
| --- | ---: | ---: | --- |
| `LOCK_DEVICE_FAILURE_BACKOFF_ENABLED` | `true` | `false` | Boolean |
| `LOCK_DEVICE_FAILURE_THRESHOLD` | `3` | `3` | Values below 1 fall back to 3 |
| `LOCK_DEVICE_FAILURE_COOLDOWN_SEC` | `600` | `600` | Minimum 60 seconds |

The code default remains disabled to preserve behavior when operators run the binary without compose-provided environment variables. Existing deployments receive the new `LOCK_*` defaults through `merge_missing_env_defaults` in `ci-source-deploy.sh`.

If backoff is enabled but Redis is disabled or initialization fails, startup logs must state that backoff is soft-degrading because no persistent state store is available.

## State Model

Extend `lockDeviceState` with an optional map keyed by target `DeviceLockStatus`:

```text
CommandBackoffs:
  LOCKED:
    ConsecutiveFailures
    CooldownUntil
    LastFailureAt
    LastError
  UNLOCKED:
    ConsecutiveFailures
    CooldownUntil
    LastFailureAt
    LastError
```

The concrete Go representation may use a map or an equivalent small structure, but it must preserve these semantics:

- LOCKED and UNLOCKED have independent counters and cooldowns.
- Existing Redis JSON without the field decodes successfully.
- Empty backoff state is omitted from JSON.
- Failure errors stored in Redis and audit details are truncated to 512 characters.
- A successful LOCK or UNLOCK clears the complete device backoff map.
- Existing `LastIP`, `LastStatus`, `LastCommand`, `NotifyOKAt`, and `UpdatedAt` values are preserved by read-modify-write operations.

### Failure transition

For a retryable failure against target state `T`:

1. Increment `T.ConsecutiveFailures`.
2. Set `LastFailureAt` and `LastError`.
3. If the count is below 3, do not set a cooldown.
4. If the count is 3 or greater, set `CooldownUntil = now + 10 minutes`.

The failure count is not reset when a cooldown expires. The first post-cooldown failure therefore immediately starts a new cooldown rather than requiring three more failures.

### Success transition

Any successful command delivery clears all target-state backoff entries for the device. This prevents a stale LOCK cooldown from delaying a future LOCK after a successful UNLOCK demonstrated that the device write path recovered.

## Command Delivery Data Flow

### Result recording boundary

`deliverLockCommand` is the single command outcome boundary used by both fresh commands and retry commands.

- Retryable failure: record the failure transition in Redis, then choose the Mongo command outcome according to the resulting backoff state.
- Permanent schema/path failure: retain existing permanent-failure behavior and do not increment device backoff.
- Success: clear all device backoff state, mark the command successful, then retain existing circuit-breaker success accounting.

Redis Get/Put failures are logged but never convert a successful device delivery into a failure and never block the original command flow.

### Before threshold

For failure counts 1 and 2:

- Keep the existing Mongo behavior: mark the command `retry` if its per-command attempt budget remains.
- Do not emit backoff transition audit records.

### Threshold reached

On failure count 3 or a failed post-cooldown probe:

- Set or extend the target cooldown.
- Mark the current Mongo command `failed`, including the cooldown deadline in its error text.
- Emit one `device_command_backoff_started` transition log and audit record.
- Do not leave the current row eligible for the retry scheduler.

## Cooldown Enforcement

### Fresh evaluation

`evaluateAndMaybeCommand` checks the current decision's target-state cooldown after resolving policy and before creating a Mongo command record.

While a cooldown is active:

- Do not create a command row.
- Do not send USP or CWMP.
- Do not advance `LastIP`, `LastStatus`, or `LastCommand` as though the device converged.
- Do not emit an audit record for every skip.

The opposite target state is allowed immediately. Device online events do not bypass an active same-target cooldown, preventing a flapping device from defeating the backoff.

### Existing retry rows

`retryLockCommand` checks the current target-state cooldown before `PrepareLockCommandResend`, so a suppressed row does not increment `attempt_count`.

If a same-target cooldown is active:

- Mark the old retry/pending row `failed` with `suppressed by device cooldown until <deadline>`.
- Do not send a command.
- Do not emit an additional audit record.

This drains legacy retry rows rather than allowing them to occupy the first 50 scheduler results indefinitely.

If policy re-evaluation changes the target state, check the newly resolved target. An old LOCK row may therefore be superseded by an allowed UNLOCK according to the existing retry re-evaluation behavior.

### Post-cooldown probe

After `CooldownUntil`:

- A fresh evaluation is allowed to create and deliver one command.
- Existing `TryLock(tenant,SN)` serialization prevents concurrent paths from releasing multiple probes for the same device.
- Success clears all backoff state and advances normal device convergence state.
- A retryable failure immediately starts another 10-minute cooldown.

## State Preservation

Every existing `lockDeviceState` write in evaluate, notify, poll, chase, retry, and subscription code must preserve the optional backoff map. Struct literals that construct a new state must explicitly carry forward backoff state from the previous value when appropriate.

A command delivery failure must not advance `LastIP` or `LastStatus`. Advancing them would make same-IP or same-status guards skip the post-cooldown recovery probe.

## Error Classification

Backoff counts every command delivery error for which `isPermanentLockCommandError` returns false. This includes USP error 7003, command response timeouts, and temporary transport errors.

Permanent schema/path errors continue through the existing failed/unsupported capability behavior and do not count toward backoff.

## Logging and Audit

Only state transitions are logged and audited.

### Enter or extend cooldown

Log:

```text
lock_command_backoff: tenant=<tenant> sn=<sn> target=<status> failures=<n> until=<time> error=<truncated-error>
```

Audit action: `device_command_backoff_started`

Details:

- `target_status`
- `consecutive_failures`
- `cooldown_until`
- `last_error`

### Recovery

Log when a successful command clears non-empty backoff state:

```text
lock_command_backoff: recovered tenant=<tenant> sn=<sn> successful_target=<status> previous_failures=<max>
```

Audit action: `device_command_backoff_recovered`

Details:

- `successful_target_status`
- `cleared_targets`
- `previous_max_failures`

Cooldown skips do not produce per-cycle logs or audit entries. Old Mongo retry rows receive explanatory error text but no separate audit entry.

## Soft-Degrade Behavior

- Redis disabled or unavailable: commands and retries retain current behavior.
- Redis Get failure during a cooldown check: log and allow the command path.
- Redis Put failure while recording a result: log; preserve the device command's actual success/failure outcome.
- Noop state store: behaves as no backoff state and never blocks a command.

Backoff infrastructure must never become a reason to fail an otherwise successful LOCK or UNLOCK.

## Test Strategy

Implementation follows TDD: add failing tests before production code.

### Pure state-machine tests

- First and second retryable failures increment without cooldown.
- Third retryable failure starts a 10-minute cooldown.
- Same target is blocked during cooldown.
- Opposite target remains allowed.
- Cooldown expiry allows one probe.
- Failed post-cooldown probe immediately extends cooldown.
- Any successful target clears all device backoff entries.
- Permanent errors do not increment backoff.
- Stored/audited errors are truncated to 512 characters.

### JSON compatibility tests

- Old JSON without backoff fields decodes successfully.
- New state round-trips without losing last IP/status/command/notify values.
- Empty backoff state is omitted.

### Command-path tests

- Fresh evaluation during same-target cooldown creates no command and makes no delivery.
- Opposite-target fresh evaluation remains deliverable.
- Retry during same-target cooldown marks the old row failed before claim and does not increment `attempt_count`.
- Redis Get/Put errors allow original behavior.
- Fresh and retry success clear backoff.
- Retryable failures record backoff; permanent failures do not.
- Existing state writes preserve backoff fields.

### Configuration tests

- Environment variables parse correctly.
- Threshold below 1 falls back to 3.
- Cooldown below 60 seconds clamps to 60 seconds.
- Unset variables use disabled, 3, and 600 seconds.

### Docker validation

Use the project `run-tests` skill for exact commands:

- Controller unit tests.
- Controller DB/integration tests for Mongo status and attempt-count behavior.
- Existing ONT Lock tests covering poll, notify, chase, retry, and circuit breaker.
- Compose config rendering with the new example environment variables.

## Deployment and Production Verification

1. Implement only on `telkomsel/ont-lock-dev`.
2. Require the full dev pipeline and `deploy:staging` to succeed.
3. On staging, verify controller startup and Redis soft-degrade behavior without new panic/error patterns.
4. Merge through a separate MR into `telkomsel/ont-lock`.
5. Require `deploy:production` to succeed.
6. Through Jumpserver, verify:
   - Controller is healthy and `/readyz` returns 200.
   - Device `081074000666` produces one cooldown transition after three retryable failures.
   - No further USP 7003 retry logs for that device during the next 10 minutes.
   - Existing retry rows become failed without increasing attempt counts during cooldown.
   - Other devices continue receiving commands.
   - The adapter `$sort` error count remains zero.

## Rollback

- Operational rollback: set `LOCK_DEVICE_FAILURE_BACKOFF_ENABLED=false` and restart controller.
- Code rollback requires no Redis or Mongo migration; old code ignores the new JSON field.
- Disabling the feature does not delete stored backoff state. Re-enabling before a stored deadline resumes protection; an expired deadline permits a probe.
- No Mongo schema or index changes are introduced.

## Success Criteria

- A persistently failing device generates at most two normal retryable failures followed by one threshold failure, then no same-target delivery until the 10-minute probe.
- A policy change requiring the opposite target state is never blocked by the old target's cooldown.
- Any successful command clears all device backoff state.
- Redis failure never blocks core command delivery.
- Existing ONT Lock tests and deployment pipelines remain green.
