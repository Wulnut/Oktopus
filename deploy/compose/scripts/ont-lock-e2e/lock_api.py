#!/usr/bin/env python3
"""Lock REST API helper for ONT Lock E2E (JWT auth, no secret output)."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import urllib.error
import urllib.request

# Reuse JWT + mint from usp_get.py (same directory).
from usp_get import mint_jwt  # noqa: E402


def controller_secret(controller: str) -> bytes:
    secret = (
        subprocess.check_output(
            ["docker", "exec", controller, "printenv", "SECRET_API_KEY"],
            text=True,
        )
        .strip()
        .encode()
    )
    if not secret:
        print("empty SECRET_API_KEY", file=sys.stderr)
        sys.exit(2)
    return secret


def api_request(
    method: str,
    url: str,
    token: str,
    payload: dict | None = None,
) -> tuple[int, object]:
    body = None if payload is None else json.dumps(payload).encode()
    req = urllib.request.Request(
        url,
        data=body,
        headers={"Authorization": token, "Content-Type": "application/json"},
        method=method,
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw = resp.read().decode()
            return resp.status, json.loads(raw) if raw else None
    except urllib.error.HTTPError as e:
        err_body = e.read().decode()
        print(f"HTTP_{e.code} {err_body}", file=sys.stderr)
        sys.exit(1)


def cmd_batch_whitelist(args: argparse.Namespace) -> int:
    if not args.reported_ip:
        print("reported_ip required", file=sys.stderr)
        return 2
    cidr = args.allowed_ip_range or f"{args.reported_ip}/32"
    token = mint_jwt(controller_secret(args.controller))
    url = (
        f"{args.api_base.rstrip('/')}/api/tenants/{args.tenant}/"
        "lock/unauthorized/batch-whitelist"
    )
    payload = {
        "remove_from_unauthorized": True,
        "items": [
            {
                "sn": args.sn,
                "reported_ip": args.reported_ip,
                "allowed_ip_range": cidr,
                "description": args.description,
            }
        ],
    }
    status, result = api_request("POST", url, token, payload)
    print(f"HTTP_STATUS={status}")
    print(json.dumps(result))
    return 0 if status in (202, 207) else 1


def main() -> int:
    p = argparse.ArgumentParser(description="ONT Lock REST API helper")
    sub = p.add_subparsers(dest="cmd", required=True)

    bw = sub.add_parser("batch-whitelist", help="POST unauthorized/batch-whitelist")
    bw.add_argument("--sn", default=os.environ.get("SN", ""))
    bw.add_argument("--tenant", default=os.environ.get("TENANT", "telkomsel"))
    bw.add_argument("--api-base", default=os.environ.get("API_BASE", "http://127.0.0.1"))
    bw.add_argument("--controller", default=os.environ.get("CONTROLLER_CONTAINER", "controller"))
    bw.add_argument("--reported-ip", default=os.environ.get("REPORTED_IP", ""))
    bw.add_argument("--allowed-ip-range", default="")
    bw.add_argument("--description", default="ont-lock-e2e TC-6")

    args = p.parse_args()
    if not args.sn:
        print("SN required", file=sys.stderr)
        return 2

    if args.cmd == "batch-whitelist":
        return cmd_batch_whitelist(args)
    return 2


if __name__ == "__main__":
    sys.exit(main())
