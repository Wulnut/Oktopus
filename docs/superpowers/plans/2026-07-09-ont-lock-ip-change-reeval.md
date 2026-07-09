# ONT Lock IP-Change Re-evaluation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect WAN IP changes while devices stay online (USP ValueChange Notify + poll fallback), re-evaluate lock decisions with Redis-backed status diffing, default-on Redis/Kafka in compose, and skip/list OntLock-unsupported devices in the UI.

**Architecture:** Refactor `handleLockDeviceOnline` into a shared `evaluateAndMaybeCommand(trigger)` pipeline gated by capability probe + per-SN Redis lock. Poller and Notify feed the same pipeline with skip-on-same-status; online/chase force-converge. Unsupported devices live in Mongo; Redis holds `last_ip/last_status/notify_ok_at`.

**Tech Stack:** Go controller, Redis (go-redis), Kafka (segmentio/kafka-go), MongoDB, Docker Compose, Next.js ONT Lock page

**Spec:** `docs/superpowers/specs/2026-07-09-ont-lock-ip-change-reeval-design.md`

---

## File map

| File | Responsibility |
|------|----------------|
| `deploy/compose/docker-compose.yaml` (or overlay) | Add `redis`, `kafka` (+ controller depends_on / network) |
| `deploy/compose/.env.controller.example` | Default `LOCK_REDIS_ENABLED=true`, `LOCK_KAFKA_ENABLED=true`, poll/notify envs |
| `deploy/compose/generate-secrets.sh` | Same defaults when generating `.env.controller` |
| `backend/.../config/config.go` | Extend `LockScale` with poll/notify settings; flip Redis/Kafka defaults to true when env unset carefully |
| `backend/.../api/lock_state.go` | Redis device state + eval lock interfaces + noop |
| `backend/.../api/lock_adapters.go` | Wire state store from same Redis client; keep soft-fail |
| `backend/.../api/lock_engine.go` | Shared evaluate pipeline, triggers, force vs diff |
| `backend/.../api/lock_capability.go` | OntLock probe; unsupported classification (7026 vs timeout) |
| `backend/.../api/lock_ip_poller.go` | Master=ON online poller |
| `backend/.../api/lock_notify.go` | USP subscribe ValueChange + handle Notify |
| `backend/.../db/lock.go` | `lock_unsupported_devices` CRUD |
| `backend/.../api/lock.go` + `api.go` routes | Unsupported list / opt-out APIs |
| `frontend/src/pages/ont-lock.js` | Unsupported tab |
| Unit tests | `lock_engine` diff/force; capability; state redis (miniredis); poller helpers |

---

### Task 1: Compose Redis + Kafka defaults

**Files:**
- Modify: `deploy/compose/docker-compose.yaml`
- Modify: `deploy/compose/.env.controller.example`
- Modify: `deploy/compose/generate-secrets.sh`
- Modify: `backend/services/controller/internal/config/config.go`
- Modify: `AGENTS.md` / `CLAUDE.md` (infra note)

- [ ] **Step 1: Add redis service** (AOF + volume, `oktopus_usp_network`, no host port required unless debugging)

```yaml
  redis:
    image: redis:7-alpine
    container_name: redis
    command: ["redis-server", "--appendonly", "yes"]
    volumes:
      - redis_data:/data
    networks:
      - usp_network
    profiles: ["controller"]  # or always-on with controller stack — match existing profile style
```

- [ ] **Step 2: Add single-node Kafka** suitable for one VM (prefer Bitnami KRaft or `apache/kafka` native mode; keep memory modest). Expose only on compose network as `kafka:9092`.

- [ ] **Step 3: Wire controller env**

```env
LOCK_REDIS_ENABLED=true
LOCK_REDIS_URL=redis://redis:6379/0
LOCK_KAFKA_ENABLED=true
LOCK_KAFKA_BROKERS=kafka:9092
LOCK_KAFKA_AUDIT_TOPIC=ont-lock-audit
LOCK_IP_POLL_ENABLED=true
LOCK_IP_POLL_INTERVAL_SEC=60
LOCK_NOTIFY_ENABLED=true
LOCK_NOTIFY_HEALTH_SEC=120
```

- [ ] **Step 4: Extend `LockScale` / flags** for poll/notify; keep Greenplum default false. Redis/Kafka **enabled default true** in env examples; code `lookupEnvOrBool(..., true)` only if product agrees — prefer explicit env in compose so local unit tests without Redis still work via false in test compose.

- [ ] **Step 5: Commit** `infra: add redis/kafka for ONT Lock scale adapters`

---

### Task 2: Redis device state + per-SN eval lock

**Files:**
- Create: `backend/services/controller/internal/api/lock_state.go`
- Create: `backend/services/controller/internal/api/lock_state_test.go`
- Modify: `backend/services/controller/internal/api/lock_adapters.go`

- [ ] **Step 1: Define types and interface**

```go
type lockDeviceState struct {
	LastIP       string             `json:"last_ip"`
	LastStatus   db.DeviceLockStatus `json:"last_status"`
	LastCommand  string             `json:"last_command,omitempty"`
	NotifyOKAt   time.Time          `json:"notify_ok_at,omitempty"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

type lockDeviceStateStore interface {
	Get(ctx context.Context, tenant, sn string) (lockDeviceState, bool, error)
	Put(ctx context.Context, tenant, sn string, st lockDeviceState) error
	TryLock(ctx context.Context, tenant, sn string, ttl time.Duration) (unlock func(), ok bool, err error)
}
```

Keys: `oktopus:lock:state:{tenant}:{sn}`, `oktopus:lock:eval:{tenant}:{sn}`.

- [ ] **Step 2: Implement redis + noop**; share Redis client with `redisLockPolicyCache` (refactor `newRedisLockPolicyCache` to return client or a bundle).

- [ ] **Step 3: Unit tests with miniredis** — Get/Put, TryLock contention, soft miss.

- [ ] **Step 4: Commit** `feat(lock): redis device state and per-SN eval lock`

---

### Task 3: Shared evaluate pipeline (diff vs force)

**Files:**
- Modify: `backend/services/controller/internal/api/lock_engine.go`
- Modify: `backend/services/controller/internal/api/lock_chase.go`
- Create/Modify: `backend/services/controller/internal/api/lock_engine_test.go`

- [ ] **Step 1: Introduce trigger constants**

```go
const (
	lockTriggerOnline       = "online"
	lockTriggerChase        = "chase"
	lockTriggerIPChangePoll = "ip_change_poll"
	lockTriggerIPChangeNotify = "ip_change_notify"
)
```

- [ ] **Step 2: Refactor `handleLockDeviceOnline` → `evaluateAndMaybeCommand(ctx, tdb, device, tenant, trigger string, reportedIP string)`**

Logic (after capability gate from Task 4):

1. `TryLock` SN (if Redis up); defer unlock.
2. Load policy/config; `EvaluateLockDecision`.
3. Load previous state from Redis.
4. Build audit details with `trigger`, `reported_ip`, `previous_*`.
5. Command decision:
   - If `!ShouldCommand` → audit only; update state IP/status; return.
   - If trigger is `online` or `chase` → force send when ShouldCommand.
   - Else if previous exists && `previous.LastStatus == decision.Status` → audit with `command_skipped=true`; `Put` state with new IP; return.
   - Else send command; on success `Put` state.
6. PENDING → `RecordUnauthorizedDevice` as today.

- [ ] **Step 3: Tests**

- Same status + poll → no send (mock deliver).
- Same status + online → send.
- Status change + poll → send.
- Missing Redis state → send when ShouldCommand.

- [ ] **Step 4: Commit** `feat(lock): shared evaluate pipeline with status diffing`

---

### Task 4: Capability probe + unsupported Mongo/API

**Files:**
- Create: `backend/services/controller/internal/api/lock_capability.go`
- Modify: `backend/services/controller/internal/db/lock.go`
- Modify: `backend/services/controller/internal/api/lock.go`
- Modify: route registration (search existing `/lock/` handlers in `api.go` / mux setup)

- [ ] **Step 1: DB model**

```go
type UnsupportedLockDevice struct {
	SN            string    `bson:"sn" json:"sn"`
	Reason        string    `bson:"reason" json:"reason"` // unsupported_path
	Detail        string    `bson:"detail,omitempty" json:"detail,omitempty"`
	OptOut        bool      `bson:"opt_out" json:"opt_out"`
	LastCheckedAt time.Time `bson:"last_checked_at" json:"last_checked_at"`
	CreatedAt     time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at" json:"updated_at"`
	OperatorID    string    `bson:"operator_id,omitempty" json:"operator_id,omitempty"`
}
```

Collection: `lock_unsupported_devices`. CRUD: upsert unsupported, list, set opt-out, delete on success probe.

- [ ] **Step 2: Probe**

- Get `InternetWanIP` and `Lock` (or single multi-path Get).
- If `isPermanentLockCommandError` / USP 7026 / path not in schema → unsupported.
- Timeout/transport → return `probeTransient` (caller skips without writing unsupported).

- [ ] **Step 3: Gate evaluate**

Before evaluate:

1. If Mongo opt_out → return.
2. If online: always re-probe (unless opt_out).
3. If probe unsupported → upsert unsupported; audit `action=unsupported`; return.
4. If probe OK → delete unsupported row if present; continue (Notify subscribe in Task 6).

- [ ] **Step 4: HTTP API** (mirror whitelist auth levels)

- `GET /api/tenants/{slug}/lock/unsupported`
- `POST /api/tenants/{slug}/lock/unsupported/{sn}/opt-out`
- `DELETE /api/tenants/{slug}/lock/unsupported/{sn}/opt-out` (resume)

- [ ] **Step 5: Unit tests** for 7026 vs timeout classification.

- [ ] **Step 6: Commit** `feat(lock): skip and list OntLock-unsupported devices`

---

### Task 5: IP poller

**Files:**
- Create: `backend/services/controller/internal/api/lock_ip_poller.go`
- Modify: `cmd/controller/main.go` (start with lock engine)
- Modify: config

- [ ] **Step 1: Scheduler** tick every `LOCK_IP_POLL_INTERVAL_SEC` (min 30).

- [ ] **Step 2: Each tick**

1. List tenants (or iterate known tenants from `account-mngr`).
2. Skip if `GetLockConfig` MasterEnabled=false.
3. List online devices (`getDevicesNoHTTP` status Online).
4. Skip unsupported/opt_out; skip if `NotifyOKAt` within `LOCK_NOTIFY_HEALTH_SEC`.
5. Get IP; empty/error → continue.
6. If state.LastIP == ip → continue.
7. `evaluateAndMaybeCommand(..., lockTriggerIPChangePoll, ip)`.

- [ ] **Step 3: Respect `lockEngineSem`** / per-SN lock.

- [ ] **Step 4: Commit** `feat(lock): IP change poller for online devices`

---

### Task 6: USP ValueChange Notify subscribe + handle

**Files:**
- Create: `backend/services/controller/internal/api/lock_notify.go`
- Modify: USP utils / send path for Add Subscription
- Modify: message interceptor or NATS subscriber for inbound Notify on OntLock IP

**Context:** Device async already forwards to `{mtp}.usp.v1.{tenant}.{sn}.api` (`adapter/.../async.go`). Interceptor stores messages; extend to detect Notify ValueChange for `InternetWanIP` and call lock evaluate.

- [ ] **Step 1: After successful probe on USP MTP**, send USP Add for Subscription:

- `NotifType` = ValueChange
- `ReferenceList` / path = `Device.X_TELKOMSEL_OntLock.InternetWanIP`
- Enable + persistent as supported by agent
- On success: set Redis `NotifyOKAt=now`
- On 7026: mark unsupported
- On transient fail: leave on poll

- [ ] **Step 2: Inbound handler**

When Notify ValueChange for that path arrives:

1. Parse new value (param value in Notify).
2. Touch `NotifyOKAt`.
3. `evaluateAndMaybeCommand(..., lockTriggerIPChangeNotify, newIP)` (if value empty, Get IP once).

- [ ] **Step 3: Manual verification plan** on `081074000888` after deploy (document in PR): change WAN or simulate Notify; confirm audit `trigger=ip_change_notify`.

- [ ] **Step 4: Commit** `feat(lock): USP ValueChange notify for InternetWanIP`

---

### Task 7: Frontend Unsupported tab

**Files:**
- Modify: `frontend/src/pages/ont-lock.js` (and any section components if split)

- [ ] **Step 1: Add tab** next to whitelist / unauthorized.

- [ ] **Step 2: Table columns:** SN, reason, detail, last_checked, opt_out, actions (Stop detecting / Resume).

- [ ] **Step 3: Wire** to new APIs with `apiPrefix`.

- [ ] **Step 4: Commit** `feat(frontend): ONT Lock unsupported devices tab`

---

### Task 8: Docs + E2E script note

**Files:**
- Modify: `AGENTS.md`, `CLAUDE.md`
- Modify: `deploy/compose/scripts/ont-lock-e2e/README.md` (optional: note poller may fire during tests; set longer interval in test env)

- [ ] **Step 1: Document** Redis/Kafka defaults, poll/notify envs, unsupported behavior.

- [ ] **Step 2: Commit** `docs: ONT Lock IP re-eval ops notes`

---

## Suggested implementation order

1 → 2 → 3 → 4 → 5 → 7 can ship **without** Notify and already satisfy “IP change while online” via poll.  
6 (Notify) last among backend features; highest MTP risk.  
8 continuous.

## Spec coverage checklist

| Spec item | Task |
|-----------|------|
| Redis default + AOF volume | 1, 2 |
| Kafka default soft-fail | 1, existing adapters |
| Status diff / force online | 3 |
| Poll Master=ON, early exit, empty IP skip | 5 |
| Notify USP + health exclude from poll | 5, 6 |
| Unsupported + opt-out + UI tab | 4, 7 |
| Per-SN Redis lock | 2, 3 |
| CWMP no Attribute Notify | 6 scoped to USP only |

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-09-ont-lock-ip-change-reeval.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks  
2. **Inline Execution** — execute tasks in this session with checkpoints  

Which approach?
