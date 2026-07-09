# ONT Lock E2E Script Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a copy-pasteable `deploy/compose/scripts/ont-lock-e2e/` package that runs TC-1/2/4/5/3 against a configurable `SN`, asserts Mongo audit/commands, and USP-Gets `Lock`.

**Architecture:** Bash `run.sh` orchestrates cases; `lib.sh` mutates Mongo, publishes NATS online events, waits/asserts; `usp_get.py` mints a SuperAdmin JWT from `SECRET_API_KEY` and prints only Lock/IP. No controller code changes.

**Tech Stack:** bash, python3 (stdlib only), docker, mongosh, nats-box, curl/urllib

**Spec:** `docs/superpowers/specs/2026-07-09-ont-lock-e2e-script-design.md`

---

### Task 1: `usp_get.py`

**Files:**
- Create: `deploy/compose/scripts/ont-lock-e2e/usp_get.py`

- [ ] **Step 1: Create helper that mints JWT and GETs params**

```python
#!/usr/bin/env python3
"""USP Get helper for ONT Lock E2E. Prints KEY=value lines only (no secrets)."""
import argparse, base64, hashlib, hmac, json, os, subprocess, sys, time, urllib.error, urllib.request

def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()

def mint_jwt(secret: bytes) -> str:
    header = b64url(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
    now = int(time.time())
    payload = {
        "username": "ont-lock-e2e",
        "email": "ont-lock-e2e@local",
        "tenant_id": "",
        "tenant_slug": "",
        "level": 0,
        "exp": now + 600,
        "iat": now,
        "iss": "Oktopus",
    }
    body = b64url(json.dumps(payload, separators=(",", ":")).encode())
    sig = b64url(hmac.new(secret, f"{header}.{body}".encode(), hashlib.sha256).digest())
    return f"{header}.{body}.{sig}"

def extract_params(obj, wanted):
    out = {}
    def walk(o):
        if isinstance(o, dict):
            rp = o.get("result_params") or o.get("ResultParams")
            if isinstance(rp, dict):
                for k, v in rp.items():
                    short = k.rsplit(".", 1)[-1]
                    if short in wanted or k in wanted:
                        out[short] = v
            for v in o.values():
                walk(v)
        elif isinstance(o, list):
            for x in o:
                walk(x)
    walk(obj)
    return out

def main():
    p = argparse.ArgumentParser()
    p.add_argument("--sn", default=os.environ.get("SN", ""))
    p.add_argument("--tenant", default=os.environ.get("TENANT", "telkomsel"))
    p.add_argument("--mtp", default=os.environ.get("MTP", "mqtt"))
    p.add_argument("--api-base", default=os.environ.get("API_BASE", "http://127.0.0.1"))
    p.add_argument("--controller", default=os.environ.get("CONTROLLER_CONTAINER", "controller"))
    p.add_argument("--params", default="Device.X_TELKOMSEL_OntLock.Lock")
    args = p.parse_args()
    if not args.sn:
        print("SN required", file=sys.stderr)
        sys.exit(2)
    secret = subprocess.check_output(
        ["docker", "exec", args.controller, "printenv", "SECRET_API_KEY"], text=True
    ).strip().encode()
    if not secret:
        print("empty SECRET_API_KEY", file=sys.stderr)
        sys.exit(2)
    token = mint_jwt(secret)
    paths = [x.strip() for x in args.params.split(",") if x.strip()]
    wanted = {x.rsplit(".", 1)[-1] for x in paths}
    url = f"{args.api_base.rstrip('/')}/api/tenants/{args.tenant}/device/{args.sn}/{args.mtp}/get"
    req = urllib.request.Request(
        url,
        data=json.dumps({"param_paths": paths}).encode(),
        headers={"Authorization": token, "Content-Type": "application/json"},
        method="PUT",
    )
    try:
        with urllib.request.urlopen(req, timeout=25) as resp:
            body = resp.read().decode()
    except urllib.error.HTTPError as e:
        print(f"HTTP_{e.code}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"GET_FAILED {type(e).__name__}", file=sys.stderr)
        sys.exit(1)
    data = json.loads(body)
    found = extract_params(data, wanted)
    for name in sorted(wanted):
        if name not in found:
            print(f"MISSING {name}", file=sys.stderr)
            sys.exit(1)
        print(f"{name}={found[name]}")

if __name__ == "__main__":
    main()
```

- [ ] **Step 2: chmod +x**

```bash
chmod +x deploy/compose/scripts/ont-lock-e2e/usp_get.py
```

---

### Task 2: `lib.sh`

**Files:**
- Create: `deploy/compose/scripts/ont-lock-e2e/lib.sh`

Implement functions from the spec: `mongo_eval`, `set_config`, `upsert_whitelist`, `delete_policy`, `clear_unauthorized`, `utc_now`, `publish_online`, `wait_evaluate_after`, `wait_command_after`, `assert_evaluate`, `assert_command`, `assert_no_new_command`, `get_lock`, `unlock_helper`, `restore_safe`, `device_snapshot`.

Key behaviors:
- General DB: `tenant_${TENANT}_general`
- Adapter DB: `adapter.devices` field `sn` lowercase
- NATS URL: `docker exec $CONTROLLER_CONTAINER printenv NATS_URL | sed 's#nats://#tls://#'`
- Online payload: `{"SN":"$SN","Status":2,"Mqtt":2,"TenantID":"$TENANT","Vendor":"ATEL","Model":"FH220"}` (adjust Mqtt/Ws/Stomp from device snapshot when possible)
- Marker waits: poll `created_at > marker` ISODate
- Audit reason lives in `details.reason`

---

### Task 3: `run.sh` + `README.md`

**Files:**
- Create: `deploy/compose/scripts/ont-lock-e2e/run.sh`
- Create: `deploy/compose/scripts/ont-lock-e2e/README.md`

- [ ] **Step 1: `run.sh` cases**

Implement `run_tc1` … `run_tc5` / `run_tc3` matching the spec matrix; inter-case `unlock_helper` when previous left Lock=1; `trap restore_safe` when `RESTORE=1`; print PASS/FAIL summary; exit non-zero on any fail.

- [ ] **Step 2: README** with local/GCP usage, env table, risks.

---

### Task 4: Smoke syntax check

- [ ] `bash -n run.sh lib.sh`
- [ ] `python3 -m py_compile usp_get.py`

Do **not** run against live production SN unless the user asks.

---

### Spec coverage

| Spec item | Task |
|-----------|------|
| Directory layout | 1–3 |
| Env vars / SN required | 3 |
| TC-1/2/4/5/3 + Lock Get | 2–3 |
| Inter-case unlock + restore | 2–3 |
| No secret printing | 1 |
| README | 3 |
