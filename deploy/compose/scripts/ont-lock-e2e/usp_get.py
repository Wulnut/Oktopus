#!/usr/bin/env python3
"""USP Get helper for ONT Lock E2E. Prints KEY=value lines only (no secrets)."""

from __future__ import annotations

import argparse
import base64
import hashlib
import hmac
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request


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


def extract_params(obj, wanted: set[str]) -> dict[str, str]:
    out: dict[str, str] = {}

    def walk(o) -> None:
        if isinstance(o, dict):
            rp = o.get("result_params") or o.get("ResultParams")
            if isinstance(rp, dict):
                for k, v in rp.items():
                    short = k.rsplit(".", 1)[-1]
                    if short in wanted or k in wanted:
                        out[short] = str(v)
            for v in o.values():
                walk(v)
        elif isinstance(o, list):
            for x in o:
                walk(x)

    walk(obj)
    return out


def main() -> int:
    p = argparse.ArgumentParser(description="USP Get Lock/IP for ONT Lock E2E")
    p.add_argument("--sn", default=os.environ.get("SN", ""))
    p.add_argument("--tenant", default=os.environ.get("TENANT", "telkomsel"))
    p.add_argument("--mtp", default=os.environ.get("MTP", "mqtt"))
    p.add_argument("--api-base", default=os.environ.get("API_BASE", "http://127.0.0.1"))
    p.add_argument("--controller", default=os.environ.get("CONTROLLER_CONTAINER", "controller"))
    p.add_argument(
        "--params",
        default="Device.X_TELKOMSEL_OntLock.Lock",
        help="Comma-separated USP param paths",
    )
    args = p.parse_args()
    if not args.sn:
        print("SN required", file=sys.stderr)
        return 2

    secret = (
        subprocess.check_output(
            ["docker", "exec", args.controller, "printenv", "SECRET_API_KEY"],
            text=True,
        )
        .strip()
        .encode()
    )
    if not secret:
        print("empty SECRET_API_KEY", file=sys.stderr)
        return 2

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
        return 1
    except Exception as e:
        print(f"GET_FAILED {type(e).__name__}", file=sys.stderr)
        return 1

    data = json.loads(body)
    found = extract_params(data, wanted)
    for name in sorted(wanted):
        if name not in found:
            print(f"MISSING {name}", file=sys.stderr)
            return 1
        print(f"{name}={found[name]}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
