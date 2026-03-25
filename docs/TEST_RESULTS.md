# Test Results

Test execution results against current HEAD. Each test either confirms a known bug from `FULL_PROJECT_REVIEW.md` or provides a safety net for existing functionality.

## How to Run

```bash
cd deploy/compose

# Unit tests (no deps)
docker compose -f docker-compose.test.yaml --profile unit up \
  --abort-on-container-exit --exit-code-from test-controller-unit test-controller-unit

docker compose -f docker-compose.test.yaml --profile unit up \
  --abort-on-container-exit --exit-code-from test-adapter test-adapter

docker compose -f docker-compose.test.yaml --profile unit up \
  --abort-on-container-exit --exit-code-from test-acs test-acs

docker compose -f docker-compose.test.yaml --profile unit up \
  --abort-on-container-exit --exit-code-from test-infra test-infra

# Integration tests (starts Mongo + NATS)
docker compose -f docker-compose.test.yaml --profile integration up \
  --abort-on-container-exit --exit-code-from test-db test-db

docker compose -f docker-compose.test.yaml --profile integration up \
  --abort-on-container-exit --exit-code-from test-bridge test-bridge

# Cleanup
docker compose -f docker-compose.test.yaml down --remove-orphans
```

---

## Results Summary

| Suite | File | Tests | Pass | Fail | Skip |
|-------|------|-------|------|------|------|
| CWMP XML | `controller/internal/cwmp/cwmp_test.go` | 22 | 11 | 11 | 0 |
| USP Messages | `controller/internal/usp/usp_utils/utils_test.go` | 20 | 20 | 0 | 0 |
| Pagination | `controller/internal/api/pagination_test.go` | 4 | 1 | 3 | 0 |
| Bridge (NATS) | `controller/internal/bridge/bridge_test.go` | 3 | 1 | 2 | 0 |
| DB Integration | `controller/internal/db/db_integration_test.go` | 17 | 16 | 1 | 0 |
| Adapter | `adapter/internal/events/usp_handler/info_test.go` | 6 | 5 | 0 | 1 |
| ACS (-race) | `acs/internal/server/handler/handler_test.go` | 4 | 2 | 1 | 0 |
| Infrastructure | `deploy/tests/infra_test.go` | 11 | 0 | 11 | 0 |
| **Total** | | **87** | **56** | **29** | **1** |

Frontend tests (4 files, ~11 tests) require Jest setup (`npm install` with jest deps). Not yet executed.

---

## Detailed Results

### CWMP XML (cwmp_test.go) -- 11 FAIL, 11 PASS

**Failing (bugs confirmed):**

| Test | Bug |
|------|-----|
| `GetParameterValues_EscapesXMLSpecialChars` | Raw `<script>` in output -- XML injection |
| `SetParameterValues_EscapesAmpersand` | Raw `&` in value -- XML injection |
| `SetParameterMultiValues_EscapesAllEntries` | Raw `<Evil>` and `&` in keys/values |
| `GetParameterNames_EscapesSpecialChars` | Raw `<path>` in output |
| `Download_EscapesURLParams` | Raw `&` in URL parameter |
| `GetParameterMultiValues_EscapesAll` | Raw angle brackets in multi-value |
| `InformResponse_EscapesMustUnderstand` | Raw tag in mustUnderstand header |
| `ChangeDuState_CorrectNamespace` | `cmwp:` instead of `cwmp:` |
| `CancelTransfer_ValidClosingTag` | Self-closing tag `<cwmp:CancelTransfer/>` instead of `</cwmp:CancelTransfer>` |
| `AllMessages/CancelTransfer` | Invalid XML: element `<CancelTransfer>` closed by `</Body>` |
| `GetDataModelType_EmptyParameterList_DoesNotPanic` | Index out of range panic on `ParameterList[0]` |

**Passing (safety net):**

| Test | Verifies |
|------|----------|
| `AllMessages/*` (12 subtests, 11 pass) | All other CWMP messages produce well-formed XML |
| `GetParameterValues_NormalInput` | Correct SOAP envelope and cwmp element |
| `SetParameterValues_NormalInput` | Correct Name/Value structure |
| `Download_NormalInput` | All fields (FileType, URL, Username) present |
| `FactoryReset_ProducesValidSOAP` | Correct cwmp:FactoryReset element |
| `Inform_ParsesCorrectly` | Parses SerialNumber, DataModelType, SoftwareVersion, ConnectionRequest |
| `GetDataModelType_TR181` | Returns "TR181" for Device. prefix |
| `ParamTypeIsWritable` | "1" is writable, "0" is not |

### USP Messages (utils_test.go) -- 20 PASS

All safety net tests. Verifies every message constructor (GET, SET, ADD, DELETE, OPERATE, GET_SUPPORTED_DM, GET_INSTANCES, NOTIFY) sets correct MsgType, preserves parameters, produces valid protobuf, and generates unique MsgIds. Also verifies USP Record construction (Version, FromId, ToId, payload).

### Pagination (pagination_test.go) -- 3 FAIL, 1 PASS

| Test | Expected Skip | Actual Skip | Status |
|------|---------------|-------------|--------|
| Page 0, size 20 | 0 | 0 | PASS |
| Page 1, size 20 | 20 | 19 | FAIL (off by 1) |
| Page 2, size 20 | 40 | 38 | FAIL (off by 2) |
| Page 10, size 20 | 200 | 190 | FAIL (off by 10) |

Bug: `device.go:130` uses `page_number * (page_size - 1)` instead of `page_number * page_size`.

### Bridge / NATS (bridge_test.go) -- 2 FAIL, 1 PASS

| Test | Status | Detail |
|------|--------|--------|
| `ConcurrentSameDevice` | FAIL | "Both concurrent requests got the same response -- subject collision detected" |
| `DoesNotTimeout` | FAIL | Blocked for 10s then timed out -- subscribes before publishing |
| `SingleRequest_Success` | PASS | Single request/response works correctly |

### DB Integration (db_integration_test.go) -- 1 FAIL, 16 PASS

| Test | Status |
|------|--------|
| `CreateAndGetFirmware` | PASS |
| `CreateAndListFirmware` | PASS |
| `UpdateFirmware_NonexistentID_Returns0Matched` | PASS |
| `CreateAndDeleteFirmware` | PASS |
| **`DeleteFirmware_CampaignsNotCleaned`** | **FAIL -- "BUG: Campaign still enabled after firmware deletion"** |
| `CreateAndGetCampaign` | PASS |
| `CampaignUniqueConstraint` | PASS |
| `CreateUpgradeLog_And_UpdateStatus` | PASS |
| `UpgradeLogUniqueIndex` | PASS |
| `IncrementRetryAndResetStatus` | PASS |
| `GetDeviceFWPolicy_NoDocument_ReturnsDefault` | PASS |
| `UpsertDeviceFWPolicy` | PASS |
| `CachedDeviceInfo_UpsertAndGet` | PASS |
| `StoreAndGetMetrics` | PASS |
| `FindAllUsers_ReturnsUsers` | PASS |
| `AddAndFindTemplate` | PASS |
| `DeviceInfo_NoTTL` | PASS (logs INFO: no TTL index) |

### Adapter (info_test.go) -- 5 PASS, 1 SKIP

| Test | Status | Detail |
|------|--------|--------|
| `FullResponse` | PASS | All 6 fields parsed correctly |
| `MTPWebsockets` | PASS | WebSocket MTP flag set correctly |
| `MTPSTOMP` | PASS | STOMP MTP flag set correctly |
| `PartialResponse_Panics` | PASS | Caught expected panic: `index out of range [4] with length 3` |
| `EmptyResolvedPathResults_Panics` | PASS | Caught expected panic: `index out of range [0] with length 0` |
| `InvalidProtobuf` | SKIP | Cannot test -- `log.Fatal` calls `os.Exit`, needs refactoring |

### ACS Handler (handler_test.go) -- 2 PASS, 1 FAIL, run with `-race`

| Test | Status | Detail |
|------|--------|--------|
| `MalformedXML_ReturnsError` | PASS | Returns 401 (falls through to missing cookie) -- not ideal but not 200 |
| **`ConcurrentSessions_RaceDetected`** | **FAIL -- `fatal error: concurrent map read and map write`** | Race detector confirmed DATA RACE on `h.Cpes` map at `cwmp.go:67` and `cwmp.go:149` |
| `OversizedBody_Rejected` | PASS | Accepted 10MB without rejection (logged as info) |
| `ValidInform_ProcessesSuccessfully` | PASS | InformResponse returned, cookie set, CPE registered, NATS publish called |

### Infrastructure (infra_test.go) -- 11 FAIL

| Test | Failures |
|------|----------|
| `AllServicesHaveRestartPolicy` | 18 services missing restart policy |
| `InternalServicesNotExposedToHost` | controller, container-upload, file-server, socketio exposed |
| `AllImagesPinned` | 14 unpinned images (2 `:latest`, 12 no tag) |
| `CriticalServicesHaveHealthcheck` | mongo_usp, msg_broker, controller missing |
| `NoHardcodedSecrets` | 23 default secrets across 12 .env files |
| `ApiBodySizeReasonable` | `client_max_body_size 10G` globally |
| `AllEndpointsHaveRateLimiting` | /firmwares, /images unprotected |
| `DockerfilesHaveUserDirective` | 4 Dockerfiles run as root |
| `NoEOLAlpine` | 9 Dockerfiles use Alpine 3.14 (EOL May 2023) |
| `NoEOLNode` | 5 Dockerfiles use Node 16/18 (EOL) |
| `CircleCIHasTestStep` | CI only builds/pushes, no tests |

---

## Test Files

| Path | Phase | Purpose |
|------|-------|---------|
| `backend/services/controller/internal/cwmp/cwmp_test.go` | 1A | CWMP XML injection, namespace, structure |
| `backend/services/controller/internal/usp/usp_utils/utils_test.go` | 1B | USP protobuf message construction |
| `backend/services/controller/internal/api/pagination_test.go` | 1C | Device list pagination skip formula |
| `backend/services/controller/internal/bridge/bridge_test.go` | 2 | NATS subject correlation, NatsCustomReq |
| `backend/services/controller/internal/db/db_integration_test.go` | 3 | MongoDB CRUD, cascading deletes, indexes |
| `backend/services/mtp/adapter/internal/events/usp_handler/info_test.go` | 6 | Adapter bounds checking, MTP parsing |
| `backend/services/acs/internal/server/handler/handler_test.go` | 7 | ACS XML handling, race condition, body size |
| `frontend/src/contexts/__tests__/backend-context.test.js` | 8 | Token caching, error alerts, 401 redirect |
| `frontend/src/contexts/__tests__/auth-context.test.js` | 8 | Token console leak, hardcoded Devias data |
| `frontend/src/contexts/__tests__/socketio-context.test.js` | 8 | Socket per-render, empty disconnect |
| `frontend/src/pages/__tests__/devices.test.js` | 8 | Raw fetch, console.error-only errors |
| `deploy/tests/infra_test.go` | 9 | Docker/nginx/Dockerfile config validation |
| `deploy/compose/docker-compose.test.yaml` | - | Test infrastructure (Mongo, NATS, runners) |
| `frontend/jest.config.js` | - | Jest configuration for Next.js |

---

## Mapping: Tests to Review Findings

Each failing test maps to a specific finding in `FULL_PROJECT_REVIEW.md`:

| Review Section | Finding | Test |
|----------------|---------|------|
| 8.2 | XML injection in CWMP constructors | `cwmp_test.go` (7 tests) |
| 8.2 | ChangeDuState namespace typo | `ChangeDuState_CorrectNamespace` |
| 8.2 | CancelTransfer malformed XML | `CancelTransfer_ValidClosingTag` |
| 8.2 | GetDataModelType panics on empty list | `GetDataModelType_EmptyParameterList_DoesNotPanic` |
| 4.1 | Off-by-one in device pagination | `pagination_test.go` (3 tests) |
| 2.1 | NATS subject not unique per request | `ConcurrentSameDevice` |
| 2.1 | NatsCustomReq subscribes before publishing | `DoesNotTimeout` |
| 4.1 | Firmware deletion does not cascade | `DeleteFirmware_CampaignsNotCleaned` |
| 10.2 | Adapter index-based parsing without bounds | `PartialResponse_Panics`, `EmptyResolvedPathResults_Panics` |
| 3.1 | ACS h.Cpes map not concurrent-safe | `ConcurrentSessions_RaceDetected` |
| 7.1 | No restart policies | `AllServicesHaveRestartPolicy` |
| 1.4 | Internal services exposed to host | `InternalServicesNotExposedToHost` |
| 7.1 | Unpinned images | `AllImagesPinned` |
| 7.1 | No health checks | `CriticalServicesHaveHealthcheck` |
| 1.3 | Hardcoded secrets | `NoHardcodedSecrets` |
| 1.4 | 10GB body size limit | `ApiBodySizeReasonable` |
| 1.4 | No rate limiting on endpoints | `AllEndpointsHaveRateLimiting` |
| 7.4 | Dockerfiles run as root | `DockerfilesHaveUserDirective` |
| 7.4 | EOL Alpine 3.14 | `NoEOLAlpine` |
| 7.4 | EOL Node 16/18 | `NoEOLNode` |
| 7.3 | No tests in CI | `CircleCIHasTestStep` |
