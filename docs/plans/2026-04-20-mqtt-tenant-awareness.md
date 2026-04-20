# MQTT Tenant Awareness

## Problem

The MQTT MTP service (broker) uses hardcoded topic paths without tenant slug. Devices configured with tenant-prefixed topics (`oktopus/usp/v1/<tenant>/agent/<device>`) are rejected by ACL, status messages lack tenant context, and auth uses a single shared KV bucket instead of per-tenant buckets.

## Files to Change

### 1. `backend/services/mtp/mqtt/internal/listeners/mqtt/hook.go`

**OnACLCheck** (line 141): Allow tenant-prefixed topics.
- Read: `oktopus/usp/v1/+/agent/<device>`
- Write: `oktopus/usp/v1/+/controller/<device>`, `oktopus/usp/v1/+/api/<device>`, `oktopus/usp/v1/+/async/<device>`

**OnConnectAuthenticate** (line 120): Parse tenant from MQTT username format `<tenant>/<device>`. Look up credentials in `devices-auth-<tenant>` KV bucket.

**OnSubscribed** (line 62): Extract tenant from subscribe filter `oktopus/usp/v1/<tenant>/agent/<device>`. Store tenant in client user properties. Use tenant-scoped status/will topics: `oktopus/usp/v1/<tenant>/status/<device>`.

**OnDisconnect** (line 48): Read tenant from client properties. Publish disconnect status to `oktopus/usp/v1/<tenant>/status/<device>`.

**OnPacketEncode** (line 88): Remove the hardcoded subscribe-topic injection. The device already knows its subscribe topic from `ResponseTopicConfigured`.

### 2. `backend/services/mtp/mqtt/internal/nats/nats.go`

- Remove hardcoded `BUCKET_NAME = "devices-auth"` single bucket creation
- Auth hook creates/opens per-tenant KV buckets on demand: `devices-auth-<tenant>`
- Pass JetStream handle to auth hook instead of a single KV

### 3. `backend/services/mtp/mqtt-adapter/internal/bridge/bridge.go`

Already partially fixed. Verify:
- Subscriptions use `oktopus/usp/v1/+/<type>/+` (done)
- Outbound publishes include tenant from NATS subject (done)
- `mqttMessageHandler` extracts tenant from MQTT topic (done)
- `buildClientConfig` message router correctly routes tenant-prefixed topics

### 4. Device MQTT Username Convention

MQTT username format: `<tenant_slug>/<endpoint_id>` (e.g., `prpl-test/proto::GFBD53400004`).

The device credential must be provisioned in the tenant-scoped KV bucket `devices-auth-<tenant_slug>` with key = `<tenant_slug>/<endpoint_id>`.

Update tenant creation and device credential provisioning in the controller to use this format.

## Execution Order

1. **OnACLCheck** — allow tenant-prefixed topics (unblocks device connections)
2. **OnConnectAuthenticate** — parse `<tenant>/<device>` username, auth against `devices-auth-<tenant>` KV
3. **OnSubscribed** — extract tenant from subscribe topic, tenant-scoped status/will
4. **OnDisconnect** — tenant-scoped disconnect status
5. **OnPacketEncode** — remove subscribe-topic injection
6. **nats.go** — pass JetStream instead of single KV to auth hook
7. **mqtt-adapter** — verify end-to-end (already mostly done)
8. **Controller** — update device credential provisioning to use `<tenant>/<device>` username format

## Device Configuration

```
Device.LocalAgent.MTP.1.Protocol                          MQTT
Device.LocalAgent.MTP.1.MQTT.ResponseTopicConfigured      oktopus/usp/v1/<tenant>/agent/<endpoint_id>
Device.LocalAgent.Controller.1.MTP.1.MQTT.Topic           oktopus/usp/v1/<tenant>/controller/<endpoint_id>
Device.MQTT.Client.1.BrokerAddress                        <server_host>
Device.MQTT.Client.1.BrokerPort                           1883
Device.MQTT.Client.1.Username                             <tenant>/<endpoint_id>
Device.MQTT.Client.1.Password                             <credential>
```
