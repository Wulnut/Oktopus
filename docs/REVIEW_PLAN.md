# Full Project Review Plan

Structured review plan for the Oktopus codebase. Each section lists what to check, where to look, and what known issues exist from prior reviews.

---

## 1. Security

### 1.1 Authentication and Authorization
- [ ] JWT validation in controller middleware (`api/middleware/middleware.go`, `api/auth/auth.go`)
- [ ] JWT secret management across services (controller `.env.controller`, firmware-upload `.env.firmware-upload`) -- are secrets consistent and rotatable?
- [ ] firmware-upload.js JWT verification (`firmware-upload.js:27-32`) -- verify it rejects expired/malformed tokens
- [ ] ACS device auth (`acs/internal/auth/auth.go`) -- how are CWMP devices authenticated?
- [ ] NATS JetStream KeyValue bucket `devices-auth` -- how are device tokens issued and rotated?
- [ ] Socket.IO service (`utils/socketio/server.js`) -- is the connection authenticated or open?
- [ ] CORS configuration (`api/cors/cors.go`) -- what origins are allowed?

### 1.2 Input Validation
- [ ] All `json.NewDecoder(r.Body).Decode` calls in controller API -- check for `http.MaxBytesReader` limits
- [ ] All `r.FormValue` calls -- check for sanitization before DB queries or file operations
- [ ] Filename handling in firmware upload pipeline (`firmware.go:85`, `firmware-upload.js:46`)
- [ ] Campaign CRUD inputs (`campaigns.go`) -- vendor/model/hw_version sanitization
- [ ] CWMP XML parsing (`cwmp/cwmp.go`, `acs/internal/cwmp/cwmp.go`) -- XML injection, entity expansion
- [ ] USP protobuf deserialization -- malformed message handling

### 1.3 Secrets and Credentials
- [ ] All `.env.*` files (13 files in `deploy/compose/`) -- hardcoded secrets, default passwords
- [ ] NATS TLS certificates (`deploy/compose/nats_config/`) -- are they self-signed? Expiration?
- [ ] Docker Registry TLS cert generation (`registry-certs-generator/`) -- is it production-safe?
- [ ] MongoDB connection string -- authentication enabled?

### 1.4 Network Exposure
- [ ] `docker-compose.yaml` -- which ports are mapped to host vs internal-only?
- [ ] Nginx rate limiting (`nginx.conf`) -- are all endpoints covered?
- [ ] container-upload service -- port 8005 still exposed to host (known issue)

---

## 2. Concurrency and Race Conditions

### 2.1 NATS Message Correlation
- [ ] `bridge/bridge.go` -- how are NATS request/reply subjects generated? Are they unique per request?
- [ ] Known issue: concurrent USP requests to the same device can cross-contaminate responses (workaround: sequential requests in Network tab)
- [ ] Check all callers of `NatsUspInteraction` and `NatsReq` -- any concurrent calls to the same device?

### 2.2 Goroutine Management
- [ ] Grep for `go func` and `go a.` across the controller -- verify each has: timeout, error handling, concurrency limits
- [ ] Campaign engine `onConnectSem` (cap 50) -- adequate for production?
- [ ] `RunCampaignBatch` per-device context (60s) -- adequate for slow devices?
- [ ] Performance metrics `perfStoreSem` (cap 10) -- verify skip-on-full behavior is acceptable

### 2.3 Shared State
- [ ] Campaign engine: verify unique index on `(device_sn, firmware_id)` prevents duplicate upgrades
- [ ] Mass action script execution: `variables` map shared across goroutines (read-only, but fragile)
- [ ] Device state checks: race between `device.v1.online` event and `deviceStateOKNoWrite` call

---

## 3. Error Handling

### 3.1 Backend
- [ ] All `cursor.All` calls in `db/` -- verify errors are checked (known fixed in `metrics.go`, check others)
- [ ] All `UpdateOne`, `InsertOne`, `DeleteOne` results -- verify `MatchedCount`/`DeletedCount` checked where relevant
- [ ] `mongo.ErrNoDocuments` vs generic error -- verify distinguished in all `FindOne` callers
- [ ] Background goroutine errors -- verify logged, not swallowed
- [ ] NATS publish/request errors -- verify handled in bridge layer

### 3.2 Frontend
- [ ] **devices.js** -- all API errors go to `console.error()` only, no user feedback (known issue, ~8 locations)
- [ ] mass-actions/firmware.js -- several handlers lack catch blocks (known issue)
- [ ] Check all `httpRequest` call sites across sections/ -- verify catch blocks or error alerts exist
- [ ] Socket.IO disconnection handling (`socketio-context.js`) -- what happens when real-time connection drops?

---

## 4. Data Integrity

### 4.1 MongoDB
- [ ] Index definitions in `db/db.go` -- verify all queries have supporting indexes
- [ ] TTL indexes: messages (90 days), device_metrics (7 days), script_executions (30 days), mass_actions (90 days) -- are these appropriate?
- [ ] Unique constraints: firmware name, campaign (vendor+model+hw_version), upgrade_log (device_sn+firmware_id), user email
- [ ] Cascading deletes: when firmware is deleted, are campaigns referencing it handled? When campaign is deleted, are upgrade_logs orphaned?
- [ ] Device info cache (`device_info` collection) -- staleness detection, eviction policy

### 4.2 NATS JetStream
- [ ] `devices-auth` KeyValue bucket -- persistence guarantees, backup, recovery
- [ ] Message delivery guarantees -- at-least-once vs at-most-once for device commands

---

## 5. Performance and Scalability

### 5.1 Memory
- [ ] Firmware upload pipeline -- verify streaming end-to-end (known: `forwardFileToUploadService` now uses `io.Pipe`)
- [ ] USP message interceptor (`usp/message_interceptor.go`, 376 lines) -- does it buffer messages in memory?
- [ ] Device list API -- does it load all devices into memory or paginate at DB level?
- [ ] Frontend: large device tables (1000+ devices) -- rendering performance, virtualization?

### 5.2 Database Queries
- [ ] `getMatchingOnlineDevices` in campaign engine -- how does it query? Full collection scan?
- [ ] `message.go` (340 lines) -- message storage queries, are they indexed?
- [ ] Device metrics history query -- time-range filtering, index usage
- [ ] Script execution logs -- query patterns, index coverage

### 5.3 Network
- [ ] NATS request timeout (30s in `api.go`) -- appropriate for all operations?
- [ ] Campaign batch: 500 devices * per-device timeout -- total wall time?
- [ ] Script execution: synchronous HTTP handler with `time.Sleep` for DELAY steps (known issue) -- timeout risk

---

## 6. Frontend UX/UI

### 6.1 Error Feedback
- [ ] devices.js -- replace `console.error` with user-facing alerts (known, ~8 locations)
- [ ] mass-actions/firmware.js -- add catch blocks and error alerts
- [ ] All dialog forms -- verify error states are shown to user, not just logged

### 6.2 Accessibility
- [ ] `severity-pill.js` -- add aria labels, do not rely on color alone (known issue)
- [ ] Table headers -- add `scope="col"`, `aria-sort` where sortable
- [ ] Skip-to-content link in layout
- [ ] Keyboard navigation through tables and dialogs

### 6.3 Responsive Design
- [ ] `firmware-table.js` `minWidth: 1000` -- breaks mobile (known)
- [ ] devices.js table with 10+ columns -- needs responsive strategy
- [ ] Dialog forms on small screens -- verify MUI handles overflow

### 6.4 Loading and Empty States
- [ ] Standardize loading indicator pattern (`loading` vs `Loading`, skeleton vs spinner)
- [ ] Verify all pages show meaningful empty states
- [ ] Verify initial data fetch has loading indicator (mass-actions firmware list)

### 6.5 Navigation
- [ ] No breadcrumb trail on device detail pages (known)
- [ ] Sidebar active item highlighting (working)
- [ ] Back navigation from device tabs to device list

### 6.6 Consistency
- [ ] Error handling pattern: Alert component vs console.error -- standardize
- [ ] Button placement and naming across pages
- [ ] Confirmation dialogs on all destructive actions (working for delete, check edit/overwrite)

### 6.7 React Patterns
- [ ] Grep for `// eslint-disable` in frontend -- each is a potential stale closure or infinite loop
- [ ] `useEffect` dependency arrays -- verify completeness or intentional omissions
- [ ] `useCallback` stability -- verify `httpRequest` from context is stable

---

## 7. Infrastructure

### 7.1 Docker Compose
- [ ] All services use correct Docker DNS names (not `host.docker.internal`) -- known: controller and file-server fixed, container-upload still uses fallback
- [ ] Port exposure: only nginx (80), NATS (4222/8222), MongoDB (27017) should be on host
- [ ] Volume mounts: `mongo_data`, `nats_data`, `firmwares`, `images` -- backup strategy?
- [ ] Health checks on services -- are any defined?
- [ ] Resource limits (memory, CPU) -- any set?
- [ ] Restart policies -- what happens when a service crashes?

### 7.2 Nginx
- [ ] Rate limiting coverage -- all API endpoints covered?
- [ ] Request body size limits for firmware upload
- [ ] WebSocket upgrade handling for Socket.IO and frontend hot-reload
- [ ] TLS termination -- not configured, assumed behind external LB?

### 7.3 CI/CD
- [ ] `.circleci/config.yml` -- what does it build/test? Does it run tests?
- [ ] No automated tests in controller or frontend (only STOMP service has tests)

### 7.4 Dockerfiles
- [ ] All 17 Dockerfiles -- verify multi-stage builds, no secrets baked in, minimal base images
- [ ] Alpine versions -- are they pinned or floating?

---

## 8. Protocol Compliance

### 8.1 USP (TR-369)
- [ ] Protobuf schema versions: controller uses v1.3, adapter uses v1.2 -- is this intentional? Compatibility?
- [ ] `usp_utils/utils.go` -- USP message construction correctness
- [ ] Device.Reboot, FactoryReset, RestartAgent OPERATE messages -- correct InputArgs per TR-181?
- [ ] FirmwareImage.Download() invocation in campaign engine -- correct USP command?

### 8.2 CWMP (TR-069)
- [ ] `cwmp/cwmp.go` (590 lines) -- XML message construction, SOAP envelope correctness
- [ ] ACS handler (`acs/internal/server/handler/cwmp.go`) -- session management, inform handling
- [ ] CWMP device authentication flow

---

## 9. Code Quality

### 9.1 Dead Code
- [ ] `restart-agent` endpoint in `api.go:89` and `deviceinfo.go:305` -- no frontend caller (known)
- [ ] Unused imports, unreachable functions
- [ ] `bulkdata` service -- is it used? Wired into compose?

### 9.2 Duplicated Logic
- [ ] Script execution in `scripts.go` vs `mass_actions.go` -- should be a shared function
- [ ] USP message construction in `deviceinfo.go`, `scripts.go`, `mass_actions.go`, `campaign_engine.go` -- overlapping patterns
- [ ] CWMP handling in controller (`internal/cwmp/`) vs ACS service (`acs/internal/cwmp/`)

### 9.3 Test Coverage
- [ ] Only `backend/services/mtp/stomp/` has tests (26 test files)
- [ ] No tests for controller API, DB layer, campaign engine, bridge
- [ ] No frontend tests
- [ ] No integration tests

---

## 10. MTP Services (transport layer)

### 10.1 MQTT Service
- [ ] Device authentication and topic isolation (`mqtt/internal/listeners/mqtt/hook.go`)
- [ ] QoS handling -- what QoS level for firmware commands?
- [ ] Connection limits and backpressure

### 10.2 WebSocket Service
- [ ] Connection lifecycle, reconnection handling
- [ ] Message framing and size limits

### 10.3 STOMP Service
- [ ] Subscription management, ack handling
- [ ] The only service with tests -- verify tests pass

### 10.4 Adapter Service
- [ ] Device state tracking (`adapter/internal/db/`) -- consistency with controller
- [ ] `info.go:128` index-based parsing of USP response (`ReqPathResults[5]`) -- fragile, no bounds check (known)
- [ ] Online/offline event publishing to NATS

---

## Review Priority

| Priority | Area | Reason |
|----------|------|--------|
| 1 | Security (Section 1) | External attack surface, credential management |
| 2 | Concurrency (Section 2) | Data corruption, race conditions in production |
| 3 | Error Handling (Section 3) | Silent failures mask bugs |
| 4 | Data Integrity (Section 4) | Cascading deletes, orphaned records |
| 5 | Frontend UX (Section 6) | User-facing quality |
| 6 | Performance (Section 5) | Scalability under load |
| 7 | Infrastructure (Section 7) | Deployment reliability |
| 8 | Protocol Compliance (Section 8) | Interoperability |
| 9 | Code Quality (Section 9) | Maintainability |
| 10 | MTP Services (Section 10) | Transport reliability |
