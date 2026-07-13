#!/usr/bin/env python3
"""sanitize.py — turn raw mongodump/curl captures into commit-safe fixtures.

Rewrites SN / WAN IP / MAC / email / EndpointID values into synthetic but
shape-preserving substitutes. Preserves vendor, model, productClass, version,
status, TR-181 paths, and relative timestamp offsets.

Inputs:  --in  dir with *.archive (mongodump) and http_*.json (curl) files
Outputs: --out dir, written as fixtures/<name>.json (array of docs) and
         fixtures/snapshots/<name>.golden.json (HTTP responses).

Usage:
    ./sanitize.py --in scripts/test-data/raw \
                  --out fixtures \
                  --tenant telkomsel-test
"""
from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any, Dict, List, Tuple

# ---------------------------------------------------------------------------
# Sanitization rules
# ---------------------------------------------------------------------------

SN_RE = re.compile(r"\b\d{10,15}\b")            # serial numbers (10-15 digits)
IPV4_RE = re.compile(r"\b(?:\d{1,3}\.){3}\d{1,3}\b")
CIDR_RE = re.compile(r"\b(?:\d{1,3}\.){3}\d{1,3}/\d{1,2}\b")
MAC_RE = re.compile(r"\b(?:[0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}\b")
EMAIL_RE = re.compile(r"\b[\w.+-]+@[\w-]+\.[\w.-]+\b")
# Endpoint IDs often embed the SN, e.g. "oktopus-081074000888" — handled by
# rewriting any string that contains a sanitized SN.

SANITIZED_SN_RE = re.compile(r"^SN-DEV-\d+$")


def new_sn(seq: int) -> str:
    return f"SN-DEV-{seq:04d}"


def new_ip(seq: int) -> str:
    # Spread across the 10.x range; keep RFC1918 shape.
    return f"10.{(seq // 256) % 256}.{seq % 256}.1"


def new_cidr(seq: int, prefix: int) -> str:
    return f"{new_ip(seq)}/{prefix}"


def new_mac(seq: int) -> str:
    # Locally-administered address range (bit 1 of first octet set).
    return f"02:00:00:00:{(seq >> 8) & 0xFF:02x}:{seq & 0xFF:02x}"


def new_email(seq: int, domain: str = "test.example.com") -> str:
    return f"user{seq}@{domain}"


# ---------------------------------------------------------------------------
# Mongodump archive extraction
# ---------------------------------------------------------------------------

def extract_archive(archive_path: Path) -> List[Dict[str, Any]]:
    """Run `mongorestore --archive --dryRun`-like extraction locally.

    We don't have mongorestore easily; instead we parse the archive's BSON
    via `bsondump` if available, else fall back to a no-op that returns [].
    The capture.sh script will produce JSON instead of BSON when mongorestore
    isn't available — this function tries both.
    """
    # If a sibling .json exists, prefer it.
    sibling = archive_path.with_suffix(".json")
    if sibling.exists():
        return _load_json_array(sibling)
    # Try bsondump if available.
    try:
        result = subprocess.run(
            ["bsondump", "--type=json", str(archive_path)],
            capture_output=True, text=True, check=True, timeout=30,
        )
        docs: List[Dict[str, Any]] = []
        for line in result.stdout.splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                docs.append(json.loads(line))
            except json.JSONDecodeError:
                pass
        return docs
    except (FileNotFoundError, subprocess.CalledProcessError, subprocess.TimeoutExpired):
        print(f"WARN: cannot parse {archive_path.name}; install bsondump or supply .json",
              file=sys.stderr)
        return []


def _load_json_array(p: Path) -> List[Dict[str, Any]]:
    raw = json.loads(p.read_text())
    if isinstance(raw, list):
        return raw
    if isinstance(raw, dict):
        # Some controllers wrap arrays in {"documents": [...]} or {"data": [...]}.
        for k in ("documents", "data", "results", "items"):
            if k in raw and isinstance(raw[k], list):
                return raw[k]
        return [raw]
    return []


# ---------------------------------------------------------------------------
# Field-level rewriting
# ---------------------------------------------------------------------------

class Sanitizer:
    def __init__(self) -> None:
        self.sn_map: Dict[str, str] = {}
        self.ip_map: Dict[str, str] = {}
        self.cidr_map: Dict[str, str] = {}
        self.mac_map: Dict[str, str] = {}
        self.email_map: Dict[str, str] = {}
        self._seq = 0

    def _seq_next(self) -> int:
        self._seq += 1
        return self._seq

    def _map(self, table: Dict[str, str], key: str, factory) -> str:
        if key in table:
            return table[key]
        new = factory(self._seq_next())
        table[key] = new
        return new

    def rewrite_sn(self, sn: str) -> str:
        if not sn or SANITIZED_SN_RE.match(sn):
            return sn
        return self._map(self.sn_map, sn, new_sn)

    def rewrite_string(self, s: str) -> str:
        if not isinstance(s, str) or not s:
            return s

        out = s

        # CIDRs first (before ipv4 strips the prefix off).
        def cidr_sub(m: re.Match) -> str:
            orig = m.group(0)
            ip, _, prefix = orig.partition("/")
            return self._map(self.cidr_map, orig,
                             lambda n: new_cidr(n, int(prefix)))
        out = CIDR_RE.sub(cidr_sub, out)

        # IPs.
        def ip_sub(m: re.Match) -> str:
            return self._map(self.ip_map, m.group(0), new_ip)
        out = IPV4_RE.sub(ip_sub, out)

        # MACs.
        def mac_sub(m: re.Match) -> str:
            return self._map(self.mac_map, m.group(0), new_mac)
        out = MAC_RE.sub(mac_sub, out)

        # SNs (10-15 digit runs, but skip anything that's clearly a timestamp
        # like 1700000000000 — those are 13 digits and start with 1[6-9]).
        def sn_sub(m: re.Match) -> str:
            digits = m.group(0)
            if len(digits) == 13 and digits.startswith("1"):
                return digits  # looks like epoch millis
            return self.rewrite_sn(digits)
        out = SN_RE.sub(sn_sub, out)

        # Emails.
        def email_sub(m: re.Match) -> str:
            return self._map(self.email_map, m.group(0),
                             lambda n: new_email(n))
        out = EMAIL_RE.sub(email_sub, out)

        # Second pass: re-apply SN map to catch endpoint IDs that embed the SN
        # as a substring (e.g. "oktopus-081074000888").
        for orig, sanitized in self.sn_map.items():
            if orig in out and sanitized not in out:
                out = out.replace(orig, sanitized)

        return out

    def rewrite(self, obj: Any) -> Any:
        if isinstance(obj, dict):
            return {self._rewrite_key(k): self.rewrite(v) for k, v in obj.items()}
        if isinstance(obj, list):
            return [self.rewrite(x) for x in obj]
        if isinstance(obj, str):
            return self.rewrite_string(obj)
        return obj

    def _rewrite_key(self, k: str) -> str:
        # Keys are not sanitized — only values. This preserves schema names.
        return k


# ---------------------------------------------------------------------------
# Driver
# ---------------------------------------------------------------------------

# Mapping: archive/capture file name -> output fixture file name.
FIXTURE_FILES = {
    "devices": "devices.json",
    "firmware": "firmware.json",
    "scripts": "scripts.json",
    "campaigns": "campaigns.json",
    "device_lock_policy": "lock_policies.json",
    "device_lock_config": "lock_config.json",
    "device_lock_unauthorized": "lock_unauthorized.json",
    "upgrade_logs": "upgrade_logs.json",
    "device_info": "device_info.json",
    "shared_tenants": "tenants.json",
    "shared_ca_certs": "ca_certs.json",
    "shared_users": "users.json",
    # Lock engine collections (real GCP capture 2026-07-13).
    "fw_policies": "fw_policies.json",
    "lock_audit_logs": "lock_audit_logs.json",
    "lock_command_attempts": "lock_command_attempts.json",
}

# HTTP capture file (http_<route>.json) -> golden snapshot file name.
HTTP_SNAPSHOTS = {
    "device": "devices_list",
    "info_vendors": "info_vendors",
    "info_status": "info_status",
    "info_device_class": "info_device_class",
    "info_general": "info_general",
    "firmware": "firmware_list",
    "scripts": "scripts_list",
    "campaigns": "campaigns_list",
    "lock_policies": "lock_policies",
    "lock_config": "lock_config",
    "lock_unauthorized": "lock_unauthorized",
    "lock_unsupported": "lock_unsupported",
    "lock_commands": "lock_commands",
    "lock_audit": "lock_audit",
    "mass-actions": "mass_actions",
    "ca-certs": "ca_certs",
    "users": "users",
}


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--in", dest="in_dir", required=True, type=Path)
    ap.add_argument("--out", dest="out_dir", required=True, type=Path)
    args = ap.parse_args()

    in_dir: Path = args.in_dir
    out_dir: Path = args.out_dir
    (out_dir / "snapshots").mkdir(parents=True, exist_ok=True)

    sanitizer = Sanitizer()

    # 1. Mongo-collection archives -> fixtures/<name>.json
    for src_name, dst_name in FIXTURE_FILES.items():
        src_archive = in_dir / f"{src_name}.archive"
        if not src_archive.exists():
            # Try .json sibling (capture may have produced that directly).
            src_archive = in_dir / f"{src_name}.json"
            if not src_archive.exists():
                continue
        docs = extract_archive(src_archive)
        if not docs:
            continue
        sanitized = [sanitizer.rewrite(d) for d in docs]
        dst = out_dir / dst_name
        dst.write_text(json.dumps(sanitized, indent=2, ensure_ascii=False) + "\n")
        print(f"  wrote {dst} ({len(sanitized)} docs)")

    # 2. HTTP captures -> fixtures/snapshots/<name>.golden.json
    for src_route, dst_name in HTTP_SNAPSHOTS.items():
        src = in_dir / f"http_{src_route}.json"
        if not src.exists():
            continue
        raw = json.loads(src.read_text())
        sanitized = sanitizer.rewrite(raw)
        dst = out_dir / "snapshots" / f"{dst_name}.golden.json"
        dst.write_text(json.dumps(sanitized, indent=2, ensure_ascii=False) + "\n")
        print(f"  wrote {dst}")

    # 3. Emit the SN/IP/MAC mapping table for auditability (gitignored).
    audit_path = out_dir / "sanitize_map.json"
    audit = {
        "sn": sanitizer.sn_map,
        "ip": sanitizer.ip_map,
        "cidr": sanitizer.cidr_map,
        "mac": sanitizer.mac_map,
        "email": sanitizer.email_map,
    }
    # Write to a gitignored sibling so the mapping never lands in the repo.
    with tempfile.NamedTemporaryFile(mode="w", delete=False, suffix=".json") as tf:
        json.dump(audit, tf, indent=2)
        print(f"\nSanitization map written to: {tf.name}")
        print("(This is a sensitive mapping of real->synthetic; do not commit.)")

    return 0


if __name__ == "__main__":
    sys.exit(main())
