# Full Project Review

Comprehensive review of the Oktopus codebase covering all 10 sections from `REVIEW_PLAN.md`.

---

## 1. Security

### 1.1 Authentication and Authorization

**[HIGH] Hardcoded JWT secrets with weak defaults**
- `api/auth/auth.go:15-16` -- falls back to `"supersecretkey"` if `SECRET_API_KEY` is unset.
- `firmware-upload.js:28` -- falls back to `'default_secret'`.
- `deploy/compose/.env.firmware-upload:4` -- `JWT_SECRET=supersecretkey` committed to repo.
- Controller `.env:19` -- `SECRET_API_KEY="secretkey"` committed to repo.

**[HIGH] Socket.IO has no authentication**
- `utils/socketio/server.js:29` -- `io.on('connection')` performs no token/session validation. Any client receives all real-time events.

**[HIGH] Container-upload service auth is a no-op**
- `container-upload-service.js:82-88` -- checks only that `Authorization` header exists, never validates contents. Combined with Docker socket access, this allows arbitrary Docker commands.

**[MEDIUM] Auth subrouter has no middleware**
- `api/api.go:45-52` -- `/api/auth/*` routes do not use `middleware.Middleware`. Each handler does manual token checks. Any new route added to this subrouter is unprotected by default.

**[MEDIUM] Admin registration race condition**
- `api/user.go:166-199` -- checks if admin exists by querying all users, then creates one. No lock or unique constraint, so concurrent requests can create multiple admins.

**[MEDIUM] No password validation on registration**
- `api/user.go:178-194` -- no minimum password length on `registerAdminUser` or `registerUser`. Only `changePassword` enforces 8 characters.

**[LOW] ACS session cookie lacks Secure/HttpOnly/SameSite flags**
- `acs/handler/cwmp.go:89`

### 1.2 Input Validation

**[HIGH] No request body size limit on most API handlers**
- Only `scripts.go:181,224` use `http.MaxBytesReader`. All other handlers (user.go, firmware.go, mass_actions.go, campaigns.go) accept unbounded request bodies.

**[HIGH] ACS reads entire request body without size limit**
- `acs/handler/cwmp.go:23` -- `ioutil.ReadAll(r.Body)` with no limit. A malicious device can exhaust server memory.

**[MEDIUM] CORS set to wildcard everywhere**
- `api/cors/cors.go:37` -- defaults to `["*"]`.
- `firmware-upload.js:14` -- hardcoded `*`.
- `container-upload-service.js:70` -- hardcoded `*`.

**[LOW] FormValue inputs not sanitized beyond presence checks**
- `api/firmware.go:53-63` -- no length limits or character validation on name, vendor, model.

### 1.3 Secrets and Credentials

**[HIGH] NATS credentials hardcoded in committed .env files**
- All `.env.*` files contain `NATS_URL=nats://oktopususer:oktopuspw@msg_broker:4222`.
- `.env.nats:2-3` -- `NATS_USER=oktopususer`, `NATS_PW=oktopuspw`.

**[HIGH] MongoDB has no authentication**
- `.env.controller:1` -- `mongodb://mongo_usp:27017` with no username/password. No `MONGO_INITDB_ROOT_*` env vars in docker-compose.

### 1.4 Network Exposure

**[HIGH] Multiple internal services exposed on host ports**
- Host-mapped ports: 4222/8222 (NATS), 8000 (controller API bypassing nginx), 5000 (unauthenticated Socket.IO), 8004 (file server, no auth), 8005 (container-upload, fake auth + Docker socket), 9292 (ACS), 9443 (Portainer), 1883/8883 (MQTT), 8080 (WebSocket MTP).

**[HIGH] Controller port 8000 exposed to host -- bypasses nginx rate limiting entirely**

**[HIGH] Docker socket mounted in container-upload service with no real auth**
- `docker-compose.yaml:264` -- combined with presence-only auth, allows arbitrary Docker commands from the network.

**[HIGH] 10GB global client_max_body_size in nginx**
- `nginx.conf:73` -- applies to all endpoints including `/api`.

**[MEDIUM] File server has no authentication**
- `file-server/main.go:39-42` -- `http.FileServer` with no auth.

**[MEDIUM] No rate limiting on /firmwares, /images, /api/containers/**
- `nginx.conf:142-172`

**[MEDIUM] No TLS configured on nginx**
- `nginx.conf:65` -- HTTP only. All traffic including JWTs and passwords is plaintext.

**[MEDIUM] Docker registry proxy disables SSL verification**
- `nginx.conf:181` -- `proxy_ssl_verify off`.

---

## 2. Concurrency

### 2.1 NATS Message Correlation

**[HIGH] NATS subscribe subject not unique per request**
- `bridge/bridge.go:49` -- `NatsUspInteraction` subscribes to `device.usp.v1.<sn>.api`, which is deterministic per device. Two concurrent requests to the same device both subscribe to the same subject. NATS delivers to both; the first `select` to fire wins, the second gets the wrong response or times out.
- This is the root cause of the Network tab bug (fixed by sequencing) and will affect any concurrent device operations (campaign engine + user browser, mass actions, etc.).

**[HIGH] `NatsCustomReq` is fundamentally broken**
- `bridge/bridge.go:91-146` -- subscribes and waits for response BEFORE publishing the request (line 119 `select` happens before line 135 `Publish`). Will always time out. Also returns `nil, nil` on line 145, discarding the result. Likely dead code but dangerous if called.

### 2.2 Goroutine Management

**[MEDIUM] HTTP server goroutine has no graceful shutdown**
- `api/api.go:173` -- `srv` is local, never stored. `srv.Shutdown()` cannot be called. On process exit, in-flight requests are dropped.

**[MEDIUM] Campaign engine NATS subscription not drainable**
- `campaign_engine.go:27` -- `sub` is captured but never stored on the struct. No shutdown hook calls `sub.Drain()`.

**[LOW] Unbounded goroutines for device info caching**
- `deviceinfo.go:59` -- goroutine spawned per request with no semaphore. Has 10s timeout but no concurrency cap.

**[LOW] `onConnectSem` drops events silently with no retry**
- `campaign_engine.go:39` -- device events dropped when semaphore full. No queue or retry.

**[LOW] Firmware upload pipe goroutine has no timeout**
- `firmware.go:243` -- if upload service hangs, goroutine blocks indefinitely.

### 2.3 Shared State

**[MEDIUM] `handleDeviceOnline` + `RunCampaignBatch` can race on same device**
- Both call `triggerUpgrade` concurrently. Unique index on `(device_sn, firmware_id)` prevents duplicate logs, but the duplicate key error from `CreateUpgradeLog` is not handled -- will log as an error and potentially send duplicate firmware commands before the insert fails.

**[MEDIUM] Multiple `RunCampaignBatch` for same campaign**
- `campaigns.go:69,114` -- create and update both fire `go a.RunCampaignBatch()` with no deduplication. Quick create+update spawns two concurrent batches.

**[MEDIUM] `NatsUspInteraction` timeout writes to ResponseWriter from goroutine**
- `bridge.go:72-74` -- safe in current callers but fragile pattern. Any future caller that writes to `w` after timeout will race.

---

## 3. Error Handling

### 3.1 Backend

**[HIGH] `log.Fatal` on cursor decode kills the server**
- `db/user.go:55` -- `cursor.All` error calls `log.Fatal(err)`. Should return the error.

**[HIGH] `log.Fatal` in adapter message parsing**
- `adapter/events/usp_handler/info.go:51,82` -- `log.Fatal(err)` on unmarshal failure kills the adapter.
- `adapter/events/usp_handler/status.go:66` -- `log.Fatal(err)` on DB update failure kills the adapter.

**[HIGH] `NatsCustomReq` broken logic**
- `bridge/bridge.go:91-146` -- (same as Section 2). Subscribes before publishing, always times out, discards result.

**[MEDIUM] HTTP server failure only logged, not fatal**
- `api/api.go:174` -- if `ListenAndServe` fails (port conflict), it is logged with `log.Println` and the goroutine exits. Service appears running but serves no HTTP.

**[MEDIUM] Mass action DB update errors silently discarded**
- `mass_actions.go:290,297,298,319,320,335,345` -- `UpdateMassActionDevice`, `IncrementMassActionProgress`, and `UpdateMassAction` return values ignored. Mass actions can get stuck in "running" forever.

**[MEDIUM] Campaign engine DB update errors silently discarded**
- `campaign_engine.go:73,77,152,191,202,208,231-233` -- multiple `UpdateUpgradeLogStatus` and `IncrementRetryAndResetStatus` errors ignored.

**[MEDIUM] ACS `xml.Unmarshal` errors silently ignored**
- `acs/handler/cwmp.go:31,56,109` -- malformed XML produces empty structs with no error reporting.

**[MEDIUM] ACS `h.Cpes` map not concurrent-safe**
- `acs/handler/cwmp.go:43,69,149` -- read/write from HTTP handler goroutines without mutex.

### 3.2 Frontend

**[HIGH] devices.js uses raw fetch(), bypasses centralized error handling entirely**
- 6 API calls (lines 223, 251, 277, 310, 372, 424) -- all errors go to `console.error` only. No user-visible feedback. This is the main page of the app.

**[HIGH] BackendContext caches token at init, never refreshes**
- `backend-context.js:14-16` -- `myHeaders` set once with `localStorage.getItem("token")`. If token changes (re-login), stale token is used until page refresh.

**[HIGH] No token refresh mechanism**
- `auth-context.js` -- JWT stored once at login, never refreshed. Expiry causes abrupt redirect to login with no user feedback.

**[HIGH] Token logged to browser console**
- `auth-context.js:100` -- `console.log("AUTH CONTEXT --> auth.user.token:", ...)` exposes token in DevTools.

**[MEDIUM] ~20 direct localStorage token accesses across frontend**
- `devices-discovery.js` (7 occurrences), `devices.js` (3), `containers-store.js` (3), `devices-wifi.js` (2), `firmware-upload-dialog.js` (1), `overview-latest-orders.js` (1), `firmware.js` (1), `index.js` (1), `chat.js` (1). All bypass `httpRequest` error handling and 401 redirect logic.

**[MEDIUM] socketio-context.js creates socket on every render**
- `socketio-context.js:24` -- socket not memoized. Disconnect handler (line 47-49) is empty. No error/reconnect handling.

**[MEDIUM] firmware.js, scripts.js, mass-actions/scripts.js -- no catch blocks on httpRequest calls**
- Errors silently swallowed; non-200 statuses produce no user feedback beyond context alert.

**[MEDIUM] Hardcoded dev user data in auth-context.js**
- `auth-context.js:87-89` -- ID `'5e86809283e28b96d2d38537'` and email `'anika.visser@devias.io'` from Devias template.

---

## 4. Data Integrity

### 4.1 MongoDB

**[HIGH] Firmware deletion does not clean up campaigns**
- `api/firmware.go:148-169` -- deleting firmware does not update or delete campaigns referencing it. Campaign engine silently fails at `GetFirmware` (returns without upgrading). Campaign remains enabled but broken.

**[MEDIUM] Campaign deletion orphans upgrade_logs**
- `db/campaigns.go:79-82` -- logs remain until 90-day TTL expires.

**[MEDIUM] device_info has no TTL and grows unboundedly**
- `db/device_info.go` -- cached info for removed devices stays forever.

**[MEDIUM] Messages cursor pagination uses _id sort but indexes use timestamp**
- `db/message.go:200` -- `_id: -1` sort does not match compound indexes on `timestamp: -1`. Requires in-memory sort.

**[LOW] firmware listing sorts by created_at with no index**
- `db/firmware.go:36`

**[LOW] messages_errors has no device_serial index for delete**
- `db/message.go:285`

**[LOW] Off-by-one in device pagination skip calculation**
- `device.go:130` -- `skip := page_number * (page_size - 1)`. Page 1 with page_size=20 skips 19 instead of 20. Pages overlap by one device.

### 4.2 NATS

**[LOW] Message interceptor uses `nc.Subscribe` not `QueueSubscribe`**
- `usp/message_interceptor.go:124,135,154` -- horizontal scaling would duplicate all stored messages.

---

## 5. Performance

### 5.1 Memory

**[HIGH] `getMatchingOnlineDevices` fetches ALL devices into memory**
- `campaign_engine.go:284-304` -- requests entire device list from adapter via NATS, then filters in-memory. Does not scale for large fleets.

**[MEDIUM] Message interceptor spawns unbounded goroutines**
- `usp/message_interceptor.go:126,136,155` -- each NATS message spawns a goroutine with no concurrency limit.

**[LOW] `mtpCache` never prunes expired entries**
- `usp/message_interceptor.go:22-34` -- expired entries only removed on access. Cache grows monotonically.

**[LOW] `forwardFileToUploadService` uses `http.DefaultClient` with no timeout**
- `firmware.go:270` -- upload service hang blocks goroutine indefinitely.

### 5.2 Database Queries

**[MEDIUM] N+1 queries in RunCampaignBatch**
- `campaign_engine.go:229-246` -- for each matched device, calls `GetDeviceFWPolicy` and `GetUpgradeLogByDeviceAndFirmware` individually. 1000 devices = ~2000 queries.

**[LOW] Regex substring search on msg_id**
- `db/message.go:185` -- `$regex` with `$options: "i"` cannot use index.

### 5.3 Timeouts

**[MEDIUM] `REQUEST_TIMEOUT` (30s) in api.go:29 appears unused**
- Actual NATS timeout is `NATS_REQUEST_TIMEOUT` (10s) from `nats/nats.go:14`. Confusing constant.

**[LOW] `handleDeviceOnline` parent context (30s) not passed to child operations**
- `campaign_engine.go:52` -- child calls create independent 5s contexts from `context.Background()`.

---

## 6. Frontend UX/UI

### 6.1 Error Feedback

**[HIGH] devices.js -- zero user-visible error feedback**
- 6 API calls, all use raw `fetch()` with `console.error` only. Device load failure, delete failure, rename failure -- user sees nothing.

**[MEDIUM] firmware.js, scripts.js, mass-actions/scripts.js -- non-200 statuses silently ignored**
- Dialog closes on failure, no error shown.

**[MEDIUM] mass-actions/firmware.js -- firmware fetch error swallowed**
- `firmware.js:529` -- `catch { // ignore }`. Campaign dialog shows "No firmware available" with no explanation.

### 6.2 Accessibility

**[HIGH] No skip-to-content link**
- `layouts/dashboard/layout.js` -- keyboard users must tab through entire sidebar on every page.

**[MEDIUM] severity-pill.js -- no aria labels, color-only status indicator**
- Uses styled `<span>` with no `role`, no `aria-label`. Violates WCAG 1.4.1. Mitigated by text children.

**[MEDIUM] No aria-label on nav element or logo image**
- `side-nav.js:93` -- `<Box component="nav">` lacks `aria-label`.
- `side-nav.js:82` -- logo image has no alt text.

### 6.3 Responsive Design

**[HIGH] All tables use hardcoded minWidth 800-1000px**
- `devices.js:553` (800), `firmware-table.js:56` (1000), `credentials-table.js:72` (800), `customers-table.js:55` (800), `overview-latest-orders.js:120` (800). No mobile-responsive alternative.

### 6.4 Loading and Empty States

**[MEDIUM] Inconsistent loading state naming**
- `devices.js:75` uses `Loading` (capitalized, violates React conventions). All other pages use `loading`.

**[LOW] Some pages delegate loading to child components without fallback indicator**

### 6.5 React Patterns

**[HIGH] ~20 direct localStorage.getItem('token') calls bypass httpRequest**
- Listed in Section 3.2 above.

**[MEDIUM] 6 eslint-disable-next-line suppressions for react-hooks/exhaustive-deps**
- `layout.js:48`, `socketio-context.js:131`, `auth-context.js:112`, `devices-lcm.js:1450`, `devices-history.js:562,729`. Each is a potential stale closure.

### 6.6 Navigation

**[LOW] No breadcrumb trail on device detail pages**
- User navigates Devices > SN123 > Info but only sidebar shows location.

---

## 7. Infrastructure

### 7.1 Docker Compose

**[HIGH] No restart policies on 18 of 20 services**
- Only `registry` has `restart: always`. If controller, NATS, MongoDB, or nginx crash, they stay down.

**[HIGH] No health checks on critical services**
- MongoDB, controller, NATS, nginx lack health checks. Controller may start before MongoDB is ready.

**[HIGH] No resource limits on any service**
- No `mem_limit`, `cpus`, or `deploy.resources`. A runaway process can consume all host resources.

**[MEDIUM] Unpinned images**
- `nats:latest`, `mongo` (no tag), `portainer/portainer-ce:latest`, `nginx:latest`.

**[LOW] Static IP addressing on all containers**
- Hardcoded /24 IPs. Fragile for adding services.

### 7.2 Nginx

- 10GB body limit, missing rate limits, no TLS -- covered in Section 1.4.

**[MEDIUM] Unused rate limit zone `api_history_delete`**
- `nginx.conf:56` -- defined but never applied.

### 7.3 CI/CD

**[HIGH] No tests in CI pipeline**
- `.circleci/config.yml` -- only builds and pushes Docker images. No Go tests, no ESLint, no security scanning.

**[MEDIUM] Outdated CI base image**
- `cimg/base:2022.09` (from September 2022).

### 7.4 Dockerfiles

**[HIGH] Controller uses EOL Alpine 3.14**
- `controller/build/Dockerfile:6`

**[MEDIUM] All 4 Dockerfiles run as root**
- No `USER` directive in controller, frontend, firmware-upload, or container-upload Dockerfiles.

**[MEDIUM] Frontend Node 18 is EOL**
- `frontend/build/Dockerfile:1` -- Node 18 LTS ended April 2025.

---

## 8. Protocol Compliance

### 8.1 USP (TR-369)

**[HIGH] Missing dot separator in firmware Download() command path**
- `mass_actions.go:50` -- `"Device.DeviceInfo.FirmwareImage." + partition + "Download()"` produces `...Image.1Download()` instead of `...Image.1.Download()`. Firmware upgrade commands will fail on compliant USP agents.

**[MEDIUM] Proto version skew: controller v1.3, adapter v1.2**
- Wire-compatible for existing fields, but Register/Deregister messages (v1.3 only) will be silently dropped by the adapter.

**[MEDIUM] `NewNotifyMsg` creates agent-side Notify from controller code**
- `usp_utils/utils.go:151` -- controllers should send NotifyResp, not Notify. May be unused.

### 8.2 CWMP (TR-069)

**[HIGH] XML injection in all CWMP message constructors**
- `cwmp/cwmp.go` -- `GetParameterValues` (line 201), `SetParameterValues` (lines 239-240), `Download` (lines 312-315), and all other constructors concatenate raw parameter values into XML strings without escaping. Special characters (`<`, `>`, `&`, `"`) will produce malformed SOAP or enable injection.

**[MEDIUM] Namespace typo in ChangeDuState**
- `cwmp/cwmp.go:438` -- `<cmwp:ChangeDUState>` should be `<cwmp:ChangeDUState>`. Compliant CPEs will reject this.

**[MEDIUM] Malformed XML in CancelTransfer**
- `cwmp/cwmp.go:333` -- self-closing `<cwmp:CancelTransfer/>` used as closing tag instead of `</cwmp:CancelTransfer>`.

**[MEDIUM] ACS `h.Cpes` map not concurrent-safe**
- `acs/handler/cwmp.go:43,69,149` -- shared map accessed from HTTP handler goroutines without mutex.

**[LOW] `GetDataModelType` panics on empty ParameterList**
- `cwmp/cwmp.go:161` -- accesses `i.ParameterList[0].Name` without bounds check.

---

## 9. Code Quality

### 9.1 Dead Code

**[MEDIUM] `bulkdata` service not wired into docker-compose**
- Entire service directory exists but is never deployed.

**[LOW] `/restart-agent` endpoint registered but no UI caller**
- `api/api.go:89`, `deviceinfo.go:315-334`

**[LOW] `DeleteDevice()` empty method stub**
- `adapter/internal/db/device.go:220-222`

### 9.2 Duplicated Logic

**[HIGH] Script execution engine duplicated ~130 lines**
- `scripts.go:688-807` and `mass_actions.go:371-487` -- near-identical step execution loops. Fixes applied to one are missed in the other (already happened twice).

**[HIGH] CWMP code near-100% duplicated between controller and ACS**
- `controller/internal/cwmp/cwmp.go` (591 lines) and `acs/internal/cwmp/cwmp.go` (548 lines) -- mostly byte-for-byte identical. ACS version also has a bug at line 218: `string(len(data))` should be `fmt.Sprint(len(data))`.

**[MEDIUM] USP marshal/send/unmarshal pattern duplicated in 3 places**
- `sendUspMsg`, `sendUspMsgDirect`, and inline in `performFirmwareUpdate`.

### 9.3 Test Coverage

**[CRITICAL] Zero test coverage for all project-authored code**
- 25 test files exist but all belong to the vendored STOMP library.
- No Go tests for controller, adapter, ACS, or any other service.
- No frontend tests (no `.test.js` or `.spec.js` files).

---

## 10. MTP Services

### 10.1 MQTT

**[MEDIUM] No connection limit on MQTT broker**
- mochi-co server started without `MaximumClients`. Can exhaust memory.

**[OK] Device authentication and topic isolation correctly implemented**
- `hook.go:120-186` -- validates credentials from NATS KV, restricts topics per device.

### 10.2 Adapter

**[CRITICAL] Index-based USP response parsing without bounds checking**
- `adapter/events/usp_handler/info.go:124-128` -- hardcoded indices `[0]` through `[5]` with no bounds checking. A device returning fewer than 6 results causes `index out of range` panic, crashing the adapter.

**[HIGH] `log.Fatal` in message parsing and status handling**
- `info.go:51,82` and `status.go:66` -- unmarshal or DB errors kill the entire adapter process. Should log and return.

### 10.3 WebSocket

**[MEDIUM] No message size limit**
- `ws/handler/client.go:26-27` -- `maxMessageSize` and `SetReadLimit` are commented out. Malicious client can exhaust memory.

**[MEDIUM] No connection limit**
- `ws/handler/hub.go:60-66` -- unbounded client map.

**[LOW] `CheckOrigin` allows all origins**
- `ws/handler/client.go:41` -- `return true`

### 10.4 STOMP

**[OK] Vendored library has 25 test files**
- No project-specific integration tests.

---

## Summary

| Severity | Section 1 | Section 2 | Section 3 | Section 4 | Section 5 | Section 6 | Section 7 | Section 8 | Section 9 | Section 10 | Total |
|----------|-----------|-----------|-----------|-----------|-----------|-----------|-----------|-----------|-----------|------------|-------|
| CRITICAL | - | - | - | - | - | - | - | - | 1 | 1 | **2** |
| HIGH | 9 | 2 | 5 | 1 | 1 | 3 | 4 | 2 | 2 | 1 | **30** |
| MEDIUM | 6 | 5 | 6 | 3 | 2 | 5 | 5 | 4 | 1 | 2 | **39** |
| LOW | 2 | 4 | - | 4 | 3 | 2 | 1 | 1 | 2 | 1 | **20** |

### Top 10 Items by Impact

1. **NATS subject not unique per request** -- concurrent device requests cross-contaminate (Section 2)
2. **Zero test coverage** for project code (Section 9)
3. **Hardcoded secrets in committed env files** -- JWT, NATS, MongoDB (Section 1)
4. **All internal service ports exposed to host** -- bypasses nginx (Section 1)
5. **Missing dot in firmware Download() USP command** -- firmware upgrades fail (Section 8)
6. **XML injection in all CWMP constructors** -- malformed SOAP (Section 8)
7. **Adapter crashes on partial USP response** -- index out of range panic (Section 10)
8. **`log.Fatal` in adapter and DB layer** -- single bad message kills the service (Sections 3, 10)
9. **Script execution duplicated 130+ lines** -- fixes diverge (Section 9)
10. **devices.js has zero error feedback** -- main page of the app (Section 6)
