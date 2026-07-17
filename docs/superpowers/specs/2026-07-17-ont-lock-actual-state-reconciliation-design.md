# ONT Lock Actual-State Reconciliation Design

## Problem

ONT Lock currently decides the desired device state from policy and WAN IP, but
does not compare that target with the value already stored in
`X_TELKOMSEL_OntLock.Lock`. The capability probe reads the Lock parameter only
to prove that the path exists and discards its value. Online and chase triggers
then intentionally force a Set, even when the device is already converged.

Production also retains the legacy `LOCK_REDIS_ENABLED=false` value generated
before Redis became default-on. The source deployment script only adds missing
environment keys and preserves existing values. With the no-op state store,
the one-minute IP poll cannot perform LastIP or LastStatus deduplication, so it
re-evaluates and writes every online supported device on every round.

## Goals

- Send Lock Set commands only when the device's actual Lock value differs from
  the current policy target.
- Apply the same comparison to online, startup/config chase, IP poll, notify,
  and retry paths.
- Remain safe when Redis is disabled or unavailable: device reads may continue,
  but stable devices must not receive repeated Set commands.
- Reuse capability reads so reconciliation does not add another device request.
- Migrate the known legacy Redis default from false to true exactly once during
  source deployment without permanently overriding later operator choices.
- Preserve unsupported-device detection, circuit breaking, failure backoff,
  audit history, tenant isolation, and soft-degrade behavior.

## Non-Goals

- Changing whitelist, CIDR, unauthorized-device, or circuit-breaker policy.
- Removing Redis-backed caching or device failure backoff.
- Changing the Recent Commands API or adding a new command status.
- Automatically mutating production outside the existing CI/CD deployment.

## Device Snapshot

Capability probing will return a structured snapshot rather than only a probe
result and detail string. A successful snapshot contains:

- the actual Lock parameter value;
- the reported InternetWanIP value;
- the resolved CWMP root when the device is CWMP;
- the existing supported/transient/unsupported classification and detail.

USP continues to read Lock and InternetWanIP through the active MTP. CWMP
continues to request both parameters together and tries the stored data-model
root before the opposite root. The successful response values are retained and
passed into evaluation. When poll or notify already supplies a WAN IP, that
reported value remains authoritative for the decision, while the snapshot Lock
value is still used for convergence.

## Lock Value Normalization

The device value is normalized case-insensitively after trimming whitespace:

- `0`, `false`, and `unlocked` mean UNLOCKED;
- `1`, `true`, and `locked` mean LOCKED.

An empty or unrecognized value is a transient probe failure, not an unsupported
schema result. Evaluation stops without issuing Set. This is fail-safe: the
controller never guesses a device's current lock state.

## Reconciliation Flow

For every evaluation trigger:

1. Acquire the existing per-device evaluation lock.
2. Run the capability probe and capture the actual Lock/WAN snapshot.
3. Resolve the current desired decision from configuration, policy, and WAN IP.
4. Persist unauthorized-device and audit state using the existing rules.
5. If `ShouldCommand` is false, persist evaluated state and return.
6. If actual Lock equals the desired target, record an `evaluate` audit entry
   with `command_skipped=true` and `skip_reason=actual_state_match`, persist the
   evaluated state when a state store is available, and return.
7. Otherwise apply failure cooldown and circuit-breaker gates, then create and
   deliver the command through the existing transport boundary.

Online and chase no longer force Set solely because of trigger type. They still
force a fresh device read, which is the reliable post-restart/reconnect source
of truth and does not rely on possibly stale Redis state.

## Retry Behavior

Retry re-evaluation uses the same capability snapshot and current policy. If
the actual device Lock value already equals the refreshed target, the existing
attempt is marked successful without another Set and an audit entry records
`retry_already_converged`. This covers lost responses where the original Set
reached the device even though delivery was recorded as retryable. A mismatch
continues through the existing resend, cooldown, and circuit-breaker logic.

## Redis Legacy Migration

The deployment flow will run a versioned, one-time environment migration after
merging missing defaults:

- migration id: `20260717-lock-redis-default-true`;
- target: `deploy/compose/.env.controller`;
- only the exact legacy value `LOCK_REDIS_ENABLED=false` is changed to `true`;
- the migration id is recorded in a retained local marker file under
  `deploy/compose/` whether the value needed changing or was already current;
- future deployments skip a recorded migration, so an operator can explicitly
  set the value back to false without CI changing it again.

The Redis URL, credentials, and all unrelated environment values are preserved.
Fresh installations continue to receive `true` from the existing templates.

## Error Handling

- Transport timeout or malformed Lock value: log as transient and do not Set.
- Unsupported path/schema: preserve the existing unsupported-device workflow.
- Redis read/write failure: continue using the actual device snapshot; skip a
  command when actual state matches, and soft-degrade cache/backoff persistence.
- Mongo audit/state failure: preserve current logging and command semantics.
- Migration marker/write failure: fail deployment before restarting services so
  a partially applied environment migration cannot be hidden.

## Test Strategy

Controller tests will cover:

- normalization of all accepted Lock representations and rejection of unknowns;
- USP and CWMP snapshot extraction, including opposite-root CWMP fallback;
- all triggers skipping Set when actual state matches the target;
- mismatch still reaching the command path;
- Redis/no-op state store not causing stable-device commands;
- retry completion without resend when the device is already converged;
- transient/unknown Lock values never issuing Set;
- existing failure backoff and post-cooldown probe behavior remaining intact.

Deployment tests will run the one-time migration against temporary environment
files and verify legacy upgrade, already-current behavior, marker idempotency,
and preservation of an operator change after the marker exists.

All controller and deployment tests run in Docker according to repository
rules. The controller race test and controller Docker build are included in
final verification.

## Delivery And Production Verification

1. Fast-forward `telkomsel/ont-lock-dev` to the current production merge point.
2. Implement and verify on `telkomsel/ont-lock-dev`.
3. Push dev and wait for the staging pipeline and deployment to succeed.
4. Review the final dev-versus-production commit set.
5. Merge dev into `telkomsel/ont-lock`, push, and wait for production CI/CD.
6. Through Electerm, verify the controller reports Redis state enabled, Redis
   contains ONT Lock device-state keys, and stable devices produce no new Recent
   Commands over at least two IP-poll intervals.
