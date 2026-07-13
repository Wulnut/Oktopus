# Test Data Capture

One-time (or as-needed) pipeline that pulls realistic data from the GCP
production tenant, sanitizes PII, and commits the resulting fixtures into the
repo so the `tests/api_snapshot` suite runs fully offline thereafter.

## Layout

```
scripts/test-data/
├── capture.sh             # SSH to GCP, mongodump telkomsel collections + curl REST
├── sanitize.py            # rewrite SN/IP/MAC/email into synthetic values
├── seed-test-tenant.sh    # create telkomsel-test tenant + mongorestore sanitized fixtures
└── README.md              # this file
```

## Prerequisites

- A Jumpserver SSH session already established (e.g. the `Jumpserver` electerm
  bookmark at 192.168.180.204). The capture script SSH-hops from there to the
  GCP VM where `~/oktopus/deploy/compose` is the live compose project.
- Local `python3` (>=3.8) with no extra packages.
- `ssh`, `scp`, `docker` reachable via the Jumpserver session.

## Usage

```bash
# 1. Capture raw dumps from GCP (writes to ./raw/*.bson and ./raw/*.json).
#    GCP_SSH_HOST is the production VM host (per CLAUDE.md: GCP_USER@GCP_HOST).
GCP_SSH_HOST=sei@<gcp-host> \
GCP_SSH_PORT=22 \
GCP_OKTOPUS_DIR=/home/sei/oktopus \
SOURCE_TENANT=telkomsel \
./scripts/test-data/capture.sh

# 2. Sanitize into repo-root fixtures/ (rewrites SN/IP/MAC/email).
./scripts/test-data/sanitize.py \
  --in scripts/test-data/raw \
  --out fixtures \
  --tenant telkomsel-test

# 3. (Optional) Seed a real telkomsel-test tenant on GCP for re-capturing
#    POST/PUT/DELETE responses without disturbing the live tenant.
GCP_SSH_HOST=sei@<gcp-host> \
./scripts/test-data/seed-test-tenant.sh
```

## Sanitization rules

| Field shape              | Source example          | Sanitized value           |
|--------------------------|-------------------------|---------------------------|
| Serial number (SN)       | `081074000888`          | `SN-DEV-{seq:04d}`        |
| WAN IP / CIDR            | `10.172.5.20`           | `10.{seq}.{x}.{y}`        |
| MAC address              | `AA:BB:CC:DD:EE:FF`     | `02:00:00:00:{seq:02x}:01`|
| Email                    | `ops@telkomsel.example` | `user{seq}@test.example.com` |
| EndpointID (contains SN) | `oktopus-081074000888`  | synced SN replacement     |

Vendor, model, productClass, version, status, TR-181 path structure, and
relative timestamp offsets are all preserved — the data shape stays realistic
so fixtures are drop-in for the test Mongo.

## Safety

- `raw/` is gitignored. Only `fixtures/` (sanitized) is committed.
- The `sanitize.py` script refuses to emit a fixture containing a value that
  matches the original-source regexes (e.g. an unmasked SN passes through
  unchanged only if it already matches the synthetic `SN-DEV-\d+` form).
- Never run `capture.sh` against a tenant without ops approval. Telkomsel is a
  live customer tenant.
