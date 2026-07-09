# ONT Lock: Online IP Change Re-evaluation Design

**Date:** 2026-07-09  
**Status:** Approved 2026-07-09  
**Goal:** While a device stays online, if `InternetWanIP` changes, re-run `EvaluateLockDecision` promptly and command `Lock` only when the **decision status** changes (with online/chase still force-converging).

## Problem

Today lock evaluation runs only on:

- NATS `device.v1.<tenant>.online`
- Policy/config chase (`chaseLockForSN` / `chaseLockAfterConfigUpdate`)

There is **no** in-session IP-change path. Same SN with a new WAN IP is ignored until the next online/chase.

## Goals

1. Detect WAN IP change while the device remains online and re-evaluate immediately when possible.
2. Avoid command storms when IP changes but authorization outcome is unchanged.
3. Make Redis + Kafka first-class compose defaults for lock scale adapters.
4. Skip devices that do not expose OntLock parameters; list them in UI with opt-out.

## Non-goals (phase 1)

- CWMP `SetParameterAttributes` / Inform-driven ValueChange (CWMP uses online/chase + poll only).
- Greenplum audit sink default-on (remains optional).
- Changing the core decision matrix in `EvaluateLockDecision` (Master / whitelist CIDR / AutoLock).

## Architecture overview

```
                    ┌─────────────────────────────┐
  device online ───►│ capability probe (OntLock)  │──unsupported──► Mongo lock_unsupported_devices
                    │ (skip if opt_out)           │                  (no evaluate / no Set)
                    └─────────────┬───────────────┘
                                  │ supported
                    ┌─────────────▼───────────────┐
                    │ USP Add Subscription        │
                    │ ValueChange(InternetWanIP)  │
                    └─────────────┬───────────────┘
                                  │
         Notify ──────────────────┤
         Poll (60s, excl. healthy │
              notify SNs) ────────┤
         online / chase ──────────┤
                                  ▼
                    ┌─────────────────────────────┐
                    │ per-SN Redis eval lock      │
                    │ Get IP → EvaluateLockDecision│
                    │ compare Redis last_status   │
                    └─────────────┬───────────────┘
                                  │
              status changed ─────┼──── status same ──► update last_ip + audit
              or online/chase     │                      (command_skipped=true)
              force converge      ▼
                           sendLockCommand
                           update Redis state
                           Mongo audit (+ Kafka soft-fail)
```

## Trigger model

### 1. USP ValueChange Notify (primary, USP MTP only)

- On successful capability probe for MQTT/WS/STOMP devices, create a USP subscription for ValueChange on `Device.X_TELKOMSEL_OntLock.InternetWanIP`.
- Async Notify path must parse ValueChange, extract SN/tenant/MTP, then enter the shared evaluate pipeline with `trigger=ip_change_notify`.
- If subscription fails with **schema / path-not-in-schema (e.g. USP 7026)** → treat as unsupported (see Capability).
- If subscription fails for transient reasons → do not mark unsupported; device remains on poll until retry on next online.

### 2. Poller (fallback)

- Interval: `LOCK_IP_POLL_INTERVAL_SEC` default **60**, configurable.
- Scope: tenants with **MasterEnabled=true**; all **Online** devices in that tenant.
- **Exclude** SNs with healthy Notify (`notify_ok_at` recent, e.g. within `2 * poll_interval` or last successful subscribe + no failure).
- If Notify has been silent beyond the health window → re-include in poll and optionally re-subscribe on next online.
- Per device:
  1. Skip if unsupported / opt_out.
  2. Get `InternetWanIP`.
  3. On Get failure or empty IP → **skip this round** (no unsupported mark, no command).
  4. If Redis `last_ip` equals current IP → **early exit** (no evaluate, no audit).
  5. Else full evaluate with `trigger=ip_change_poll`.

### 3. Online / chase (force converge)

- Still run full evaluate after capability check.
- **Always allow Set when `ShouldCommand`** even if Redis `last_status` matches (device-side Lock may have drifted).
- Still update Redis state and audit.
- `trigger=online` or `trigger=chase`.

## Command / audit policy

| Trigger | IP unchanged | Status unchanged (IP may change) | Status changed |
|---------|--------------|----------------------------------|----------------|
| Poll | Early exit | Audit + update `last_ip`; **no Set** (`command_skipped`) | Set + update state |
| Notify | N/A (event implies change) or treat as change | Same as poll | Set |
| Online / chase | Evaluate anyway | **Set if ShouldCommand** (force converge) | Set |

“Status” means `LockDecision.Status` (`UNLOCKED` / `LOCKED` / `PENDING`), not the raw IP string.

Audit `details` should include at least:

- `reported_ip`
- `reason`
- `trigger` (`online` | `chase` | `ip_change_poll` | `ip_change_notify`)
- `command_skipped` (bool, when applicable)
- `previous_ip` / `previous_status` when available

## Redis (default on)

Compose: add `redis` service on `oktopus_usp_network`, volume + AOF.

Controller defaults (phase 1):

- `LOCK_REDIS_ENABLED=true`
- `LOCK_REDIS_URL=redis://redis:6379/0`

Uses:

1. Existing policy cache (`oktopus:lock:policy:{tenant}:{sn}`)
2. Device lock state: `oktopus:lock:state:{tenant}:{sn}` → JSON `{last_ip, last_status, last_command, notify_ok_at, updated_at}`
3. Per-SN eval lock: `oktopus:lock:eval:{tenant}:{sn}` (`SET NX` + short TTL, e.g. 5–15s)

### Redis failure (soft degrade)

- Policy cache miss → Mongo (existing).
- Missing/unreadable state → treat as **no previous state** → allow command when `ShouldCommand` (same as first evaluate).
- Eval lock unavailable → log and proceed without lock (best-effort), or skip only the poll path; online must not be blocked forever.

## Kafka (default on)

Compose: add Kafka (single-node / KRaft or zookeeper-based stack suitable for single VM).

Controller defaults:

- `LOCK_KAFKA_ENABLED=true`
- `LOCK_KAFKA_BROKERS=kafka:9092` (exact service name TBD in compose)
- Topic: existing `LOCK_KAFKA_TOPIC` default

### Kafka failure (soft degrade)

- Init/write failure → log only; Mongo `lock_audit_logs` remains source of truth; evaluate/Set continue.

## Capability / unsupported devices

### Probe

On online (and when re-including a device):

- Attempt Get (or GetSupportedDM) for:
  - `Device.X_TELKOMSEL_OntLock.InternetWanIP`
  - `Device.X_TELKOMSEL_OntLock.Lock`

### Classification

| Outcome | Action |
|---------|--------|
| Schema / path-not-in-schema (e.g. 7026) | Mark **unsupported**; **no** evaluate / Set / poll / notify for this SN |
| Timeout / transport error | **Do not** mark unsupported; skip this attempt |
| Success | Clear unsupported (if any); proceed; try Notify subscribe on USP |

### Mongo collection `lock_unsupported_devices` (per-tenant general DB)

Suggested fields:

- `sn`
- `reason` (`unsupported_path` | …)
- `detail` (error text / err_code)
- `opt_out` (bool) — operator “stop detecting”
- `last_checked_at`
- `created_at` / `updated_at`
- `operator_id` (when opt_out toggled)

### Behavior with `opt_out`

- `opt_out=true` → never probe/evaluate until operator clears opt-out (“resume detection”).
- `opt_out=false` + unsupported → skip lock pipeline; **re-probe on each online** so firmware upgrades auto-enroll.
- When probe succeeds → delete unsupported row (or mark resolved) and enter normal pipeline.

### API / UI

- REST: list / set opt-out / clear opt-out (TenantAdmin+).
- Frontend: ONT Lock page new **Unsupported** tab (alongside whitelist / unauthorized).

## Concurrency

All evaluate entry points acquire per-SN Redis lock before read-state → decide → write-state → command, to prevent double Set when online and poll/notify race.

Keep existing global `lockEngineSem` for pool protection.

## Config / env (phase 1 additions)

| Variable | Default | Meaning |
|----------|---------|---------|
| `LOCK_REDIS_ENABLED` | `true` | Enable Redis adapters |
| `LOCK_REDIS_URL` | `redis://redis:6379/0` | |
| `LOCK_KAFKA_ENABLED` | `true` | Enable Kafka audit sink |
| `LOCK_KAFKA_BROKERS` | `kafka:9092` | |
| `LOCK_IP_POLL_ENABLED` | `true` | IP poller |
| `LOCK_IP_POLL_INTERVAL_SEC` | `60` | |
| `LOCK_NOTIFY_ENABLED` | `true` | Attempt USP ValueChange subscribe |
| `LOCK_NOTIFY_HEALTH_SEC` | `120` | Exclude from poll while notify healthy |

Exact broker hostnames must match compose service names.

## Implementation sketch (files)

Backend (controller):

- Extend `lock_adapters.go` / new `lock_state_redis.go` for device state + eval lock
- New `lock_ip_poller.go` scheduler
- New Notify subscribe + async Notify handler wiring (MTP async subjects)
- Capability probe + `lock_unsupported_devices` DB/API
- Refactor `handleLockDeviceOnline` into shared `evaluateAndMaybeCommand(trigger)`

Compose:

- `redis` + volume/AOF
- `kafka` (+ deps if needed)
- Wire controller env defaults in `.env.controller.example` / `generate-secrets.sh`

Frontend:

- ONT Lock **Unsupported** tab + opt-out actions

Tests:

- Unit: status-diff skip vs online force; empty IP poll skip; unsupported vs timeout
- Adapter tests with miniredis for state/lock keys

## Risks

- CPE may not support ValueChange on vendor OntLock params → poll remains the real safety net.
- Default Redis+Kafka increases compose footprint on the single GCP VM (memory); size images conservatively.
- Force-converge on every online reintroduces some Set traffic; acceptable for correctness.
- Async Notify plumbing across mqtt/ws/stomp adapters is the largest engineering risk in phase 1.

## Rollout

1. Land compose Redis/Kafka + enable flags on staging.
2. Ship poller + Redis state/diff + unsupported list/UI (works even before Notify).
3. Enable USP Notify subscribe/handler; verify on test SN (`081074000888`).
4. Tune poll interval / notify health window from production metrics.

## Open items for implementation plan (not unresolved product questions)

- Exact Kafka compose topology (KRaft single-node vs bitnami/zookeeper).
- Precise USP Add/Subscription object paths and async subject routing per MTP.
- Whether unsupported probe uses Get vs GetSupportedDM first.
