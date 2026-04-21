# STOMP Service Account Authentication

## Problem

The STOMP broker requires authentication via per-tenant NATS KV buckets. The stomp-adapter is an internal service that needs to connect but has no device credentials. On a fresh VM after tenant creation, the stomp-adapter fails with `authentication failed`.

## Solution

Use a shared secret generated per-deployment. `generate-secrets.sh` creates a random `STOMP_SERVICE_KEY` and writes it to both the STOMP broker's and stomp-adapter's `.env` files. The broker checks this env var for service account logins before falling through to device KV auth.

## Files to Change

### 1. `deploy/compose/generate-secrets.sh`

- Generate `STOMP_SERVICE_KEY` (random 24-char alphanumeric, same as `NATS_PW`)
- Write `STOMP_SERVICE_KEY=<value>` to `.env.stomp-adapter`
- Write `STOMP_SERVICE_KEY=<value>` to `.env.stomp` (the broker's env, create if needed)

### 2. `deploy/compose/.env.stomp-adapter.example`

Add:
```
STOMP_USER=oktopusAdapter
STOMP_PASSWD=__STOMP_SERVICE_KEY__
```

### 3. `deploy/compose/.env.stomp.example` (new file if not exists)

Add:
```
STOMP_SERVICE_USER=oktopusAdapter
STOMP_SERVICE_KEY=__STOMP_SERVICE_KEY__
```

Plus existing NATS config (broker needs NATS for device auth).

### 4. `backend/services/mtp/stomp/cmd/stomp/main.go`

In `NatsAuthenticator.Authenticate`:
- Read `STOMP_SERVICE_USER` and `STOMP_SERVICE_KEY` from env (once at init, store on struct)
- Before parsing tenant/device format, check if `login == serviceUser && passcode == serviceKey`
- If match, return true (service account authenticated)

```go
type NatsAuthenticator struct {
    js          jetstream.JetStream
    serviceUser string
    serviceKey  string
}
```

Init:
```go
auth := &NatsAuthenticator{
    js:          js,
    serviceUser: os.Getenv("STOMP_SERVICE_USER"),
    serviceKey:  os.Getenv("STOMP_SERVICE_KEY"),
}
```

Auth check (add before tenant/device parsing):
```go
if a.serviceKey != "" && login == a.serviceUser && passcode == a.serviceKey {
    log.Printf("auth: service account %q authenticated", login)
    return true
}
```

### 5. `backend/services/mtp/stomp-adapter/internal/config/config.go`

Already reads `STOMP_USER` and `STOMP_PASSWD` from env. No changes needed.

### 6. `deploy/compose/docker-compose.yaml`

Ensure the STOMP broker service has `env_file: .env.stomp` (add if missing).

## Execution Order

1. Create `.env.stomp.example` with NATS + service key placeholders
2. Update `.env.stomp-adapter.example` with `STOMP_USER` and `STOMP_PASSWD`
3. Update `generate-secrets.sh` to generate `STOMP_SERVICE_KEY` and write to both env files
4. Update `NatsAuthenticator` in `stomp/cmd/stomp/main.go` to check service account
5. Update `docker-compose.yaml` to add `env_file` for STOMP broker if needed
6. Test on fresh deployment

## Security Properties

- Secret is random per-deployment (24 chars alphanumeric)
- Never committed to git (`.env.*` files gitignored)
- Only shared between two internal containers on the same Docker network
- No hardcoded values in source code
