# ONT Lock E2E Automation Script Design

**Date:** 2026-07-09  
**Status:** Approved for implementation (pending user review of this written spec)  
**Approach:** Bash entry + shared shell helpers + small Python helper for JWT/USP Get (Approach B)

## Goal

Provide a copy-pasteable end-to-end script that operators can run on **local Docker compose** or **GCP compose hosts** by setting `SN` (and optional env vars). It exercises the ONT Lock decision matrix, asserts platform audit/command records, and verifies the device DataModel node `Device.X_TELKOMSEL_OntLock.Lock` via USP Get.

## Non-goals

- Not wired into GitLab CI in this iteration
- Does not enumerate users or print secrets / JWT tokens
- Does not treat “device stays online after Lock=1” as a PASS criterion (only Lock node value; if Get fails because the device dropped, that step FAILs)
- Does not replace Go unit tests for `EvaluateLockDecision` / `parseUspGetParamValue`

## Location

```
deploy/compose/scripts/ont-lock-e2e/
  run.sh          # entrypoint
  lib.sh          # mongo / nats / wait / assert / restore
  usp_get.py      # mint JWT from SECRET_API_KEY, USP Get, print Lock/IP only
  README.md       # usage for local + GCP
```

Working directory expectation: `deploy/compose` (or set `COMPOSE_DIR`).

## Runtime dependencies

- `docker`, `mongosh` (via `mongo_usp` container), `python3` (host), `curl` optional
- Running stack: `controller`, `mongo_usp`, NATS (TLS certs under `nats_config/`), nginx or direct controller API
- `natsio/nats-box` image pullable for publishing online events
- Target device online on the configured MTP (default MQTT) for cases that Set/Get Lock

## Configuration

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SN` | — | **yes** | Device serial number |
| `TENANT` | `telkomsel` | no | Tenant slug |
| `COMPOSE_DIR` | script-relative `../..` | no | Path to compose root |
| `MONGO_CONTAINER` | `mongo_usp` | no | Mongo container name |
| `CONTROLLER_CONTAINER` | `controller` | no | Controller container name |
| `NATS_NETWORK` | `oktopus_usp_network` | no | Docker network for nats-box |
| `NATS_CERTS_DIR` | `$COMPOSE_DIR/nats_config` | no | TLS certs for NATS client |
| `API_BASE` | `http://127.0.0.1` | no | Base URL for USP Get (nginx) |
| `MTP` | `mqtt` | no | Device MTP path segment |
| `GOOD_CIDR` | `10.172.0.0/16` | no | Whitelist CIDR covering real WAN |
| `BAD_CIDR` | `10.0.0.0/16` | no | Wrong CIDR for TC-2 |
| `CASES` | `tc1,tc2,tc4,tc5,tc3` | no | Case list / order |
| `RESTORE` | `1` | no | Restore safe config + unlock at end |
| `EVAL_TIMEOUT_SEC` | `30` | no | Wait for evaluate audit |
| `CMD_TIMEOUT_SEC` | `30` | no | Wait for command attempt |

Secrets: `usp_get.py` reads `SECRET_API_KEY` from the controller container via `docker exec … printenv` into process memory only; never echo the key or token.

## Trigger path under test

1. Publish NATS subject `device.v1.<tenant>.online` with device JSON (`SN`, `Status`, `Mqtt`/`…`, `TenantID`, …)
2. Controller `handleLockDeviceOnline` → USP Get WAN IP → `EvaluateLockDecision` → optional USP Set `Lock`
3. Persist `lock_audit_logs` / `lock_command_attempts` / maybe `lock_unauthorized_devices`
4. Script USP Get `Device.X_TELKOMSEL_OntLock.Lock` for readback

## Case matrix

| Case | Setup | Expect audit | Expect command | Expect Lock node |
|------|--------|--------------|----------------|------------------|
| TC-1 | Master ON, whitelist `GOOD_CIDR`, AutoLock irrelevant | `UNLOCKED` / `AUTHORIZED` | `0` success | `0` |
| TC-2 | Master ON, whitelist `BAD_CIDR`, AutoLock ON | `LOCKED` / `UNAUTHORIZED` | `1` success | `1` |
| TC-4 | Master ON, **no** whitelist, AutoLock OFF | `PENDING` / `UNAUTHORIZED` | **no new Set** | unchanged (not asserted to a new value) |
| TC-5 | Master OFF | `UNLOCKED` / `MASTER_DISABLED` | `0` success | `0` |
| TC-3 | Master ON, no whitelist, AutoLock ON | `LOCKED` / `UNAUTHORIZED` | `1` success | `1` |

Default `CASES` order: `tc1,tc2,tc4,tc5,tc3`.

### Inter-case unlock

If the previous case left `Lock=1` and the next case expects unlock or a clean PENDING baseline, run an internal **unlock helper**: set whitelist `GOOD_CIDR`, AutoLock OFF, Master ON, publish online, wait for `command=0` success and optionally confirm `Lock=0` before continuing.

### Restore (`RESTORE=1`)

On exit (success or failure, via `trap` when practical):

- Master ON, AutoLock OFF
- Upsert whitelist `GOOD_CIDR` for `SN`
- Clear unauthorized row for `SN`
- Publish online; if device still online, wait for unlock command and Get `Lock=0`

## Helpers

### `lib.sh`

- `mongo_eval '<js>'` — `docker exec $MONGO_CONTAINER mongosh --quiet --eval …` against `tenant_<slug>_general` / `adapter`
- `set_config master auto_lock`
- `upsert_whitelist cidr` / `delete_policy`
- `publish_online` — nats-box TLS pub to `device.v1.$TENANT.online`
- `wait_latest_evaluate` / `wait_latest_command` — poll by `created_at` after a marker timestamp
- `assert_evaluate status reason` / `assert_command value status` / `assert_no_new_command`
- `restore_safe`
- Device snapshot: adapter `status`/`mqtt` for diagnostics (not PASS gate except when Get needs online)

### `usp_get.py`

- Args or env: `SN`, `TENANT`, `MTP`, `API_BASE`, param paths
- Mint HS256 JWT: `level=0`, empty tenant fields, `iss=Oktopus`, key from `SECRET_API_KEY`
- `PUT {API_BASE}/api/tenants/{tenant}/device/{sn}/{mtp}/get` with body `{"param_paths":[...]}`
- Authorization header = raw JWT string (no `Bearer` prefix; matches controller middleware)
- Print only extracted params, e.g. `Lock=1` and `InternetWanIP=10.172.16.165`
- Non-zero exit if HTTP/parse/param missing

### `run.sh`

- Require `SN`
- Source `lib.sh`
- Record baseline; run selected cases; print PASS/FAIL table
- Exit 0 iff all selected cases passed

## PASS / FAIL rules

A case **PASS**es only if:

1. Platform audit (and command / no-command) assertions match the matrix, **and**
2. For cases that expect a Lock value (`tc1`,`tc2`,`tc5`,`tc3`): USP Get returns that exact `Lock` string (`0` or `1`)

TC-4 PASS: PENDING + UNAUTHORIZED + no new command after trigger; Lock Get is optional diagnostic only.

## Usage examples

```bash
cd deploy/compose   # local or ~/oktopus/deploy/compose on GCP

SN=081074000888 ./scripts/ont-lock-e2e/run.sh

SN=081074000888 CASES=tc1,tc4,tc5 ./scripts/ont-lock-e2e/run.sh

SN=081074000888 GOOD_CIDR=10.172.0.0/16 BAD_CIDR=10.0.0.0/16 ./scripts/ont-lock-e2e/run.sh
```

## Risks and operator notes

- Script **mutates** live `device_lock_config` / `device_lock_policy` for the tenant
- `Lock=1` may drop MQTT on some devices; subsequent Get/Set may fail until reconnect
- Prefer a dedicated test SN; do not run against production customer CPEs without approval
- Ensure `GOOD_CIDR` matches the device’s real `InternetWanIP` before relying on restore unlock

## Implementation follow-up

After this spec is reviewed, create an implementation plan under `docs/superpowers/plans/` and implement the four files above without changing controller lock engine behavior.
