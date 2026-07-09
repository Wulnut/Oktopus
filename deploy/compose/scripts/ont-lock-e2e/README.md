# ONT Lock E2E scripts

Copy-pasteable end-to-end checks for the ONT Lock decision engine on **local** or **GCP** compose hosts. Sets Mongo config/policy, publishes a NATS online event, asserts audit/command rows, and USP-Gets `Device.X_TELKOMSEL_OntLock.Lock`.

## Requirements

- Running compose stack: `controller`, `mongo_usp`, NATS, nginx (or reachable `API_BASE`)
- Host tools: `docker`, `python3`, `bash`
- Image: `natsio/nats-box:0.14.3` (pulled on first run)
- TLS certs: `$COMPOSE_DIR/nats_config/{rootCA.pem,cert.pem,key.pem}`
- Target device **online** on the chosen MTP (default MQTT)

## Quick start

```bash
cd deploy/compose          # local
# or: cd ~/oktopus/deploy/compose   # GCP

SN=081074000888 ./scripts/ont-lock-e2e/run.sh
```

Subset:

```bash
SN=081074000888 CASES=tc1,tc4,tc5 ./scripts/ont-lock-e2e/run.sh
```

Custom CIDRs (must match real WAN for unlock/restore):

```bash
SN=081074000888 GOOD_CIDR=10.172.0.0/16 BAD_CIDR=10.0.0.0/16 ./scripts/ont-lock-e2e/run.sh
```

## Cases (default order)

| Case | Setup | Expect |
|------|--------|--------|
| TC-1 | Master ON, whitelist `GOOD_CIDR` | `AUTHORIZED` / Set `0` / **Lock=0** |
| TC-2 | Master ON, `BAD_CIDR`, AutoLock ON | `UNAUTHORIZED` / Set `1` / **Lock=1** |
| TC-4 | Master ON, no whitelist, AutoLock OFF | `PENDING` / **no Set** |
| TC-5 | Master OFF | `MASTER_DISABLED` / Set `0` / **Lock=0** |
| TC-3 | Master ON, no whitelist, AutoLock ON | `UNAUTHORIZED` / Set `1` / **Lock=1** |

After lock cases, the runner unlocks (whitelist hit) before TC-1/4/5. On exit (`RESTORE=1`), restores Master ON, AutoLock OFF, whitelist `GOOD_CIDR`, and attempts unlock.

## Environment

| Variable | Default | Notes |
|----------|---------|--------|
| `SN` | — | **Required** |
| `TENANT` | `telkomsel` | |
| `CASES` | `tc1,tc2,tc4,tc5,tc3` | Comma-separated |
| `GOOD_CIDR` | `10.172.0.0/16` | Must cover device WAN IP |
| `BAD_CIDR` | `10.0.0.0/16` | TC-2 only |
| `RESTORE` | `1` | Restore on exit |
| `API_BASE` | `http://127.0.0.1` | nginx or controller URL |
| `MTP` | `mqtt` | Path segment for USP Get |
| `COMPOSE_DIR` | script `../..` | |
| `MONGO_CONTAINER` | `mongo_usp` | |
| `CONTROLLER_CONTAINER` | `controller` | |
| `NATS_NETWORK` | `oktopus_usp_network` | |
| `NATS_CERTS_DIR` | `$COMPOSE_DIR/nats_config` | |

`usp_get.py` reads `SECRET_API_KEY` from the controller container in-process and never prints it.

## Files

- `run.sh` — entrypoint
- `lib.sh` — Mongo / NATS / wait / assert / restore
- `usp_get.py` — JWT + USP Get (`Lock=…` lines only)

## Risks

- Mutates live `device_lock_config` / `device_lock_policy` for the tenant
- `Lock=1` may drop MQTT on some CPEs; later Get/Set then fail until reconnect
- Use a dedicated test SN; do not run on customer devices without approval
