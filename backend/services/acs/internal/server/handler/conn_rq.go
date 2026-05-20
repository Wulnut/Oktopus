package handler

// CWMP Connection Request credential provisioning.
//
// Per TR-069 (BBF TR-069 Issue 1 Amendment 6, §3.2.2 + TR-181 §A.3.2.6), the
// CPE authenticates the ACS during a Connection Request by verifying HTTP
// Digest credentials against:
//
//	Device.ManagementServer.ConnectionRequestUsername
//	Device.ManagementServer.ConnectionRequestPassword
//
// Best practice (and what GenieACS and other production ACS implementations
// do) is for the ACS to generate per-CPE random credentials, push them to the
// CPE via SetParameterValues during an Inform session, and persist them so
// the ACS can later authenticate as itself. Without this, a single global
// credential pair is shared across every CPE the ACS manages, which is both
// a feature gap (no automatic onboarding) and a security weakness (a single
// leak compromises every device).
//
// This file implements that flow on top of NATS JetStream KV buckets named
// `cwmp-conn-rq-<tenant>` (mirroring the existing `devices-auth-<tenant>`
// bucket lifecycle). Each key is the CPE serial number; the value is JSON
// holding the username/password that the CPE has been told to expect.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"oktopUSP/backend/services/acs/internal/cwmp"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
)

// connReqCreds is the value stored in the cwmp-conn-rq-<tenant> KV bucket,
// one entry per CPE serial number. Confirmed marks whether the CPE has acked
// a SetParameterValues for the same credentials, so a future ACS restart can
// trust that the CPE actually carries them.
type connReqCreds struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	ProvisionedAt int64  `json:"provisioned_at"`
}

// TR-181 / TR-098 parameter names. Picked off cpe.DataModel which the Inform
// handler populates from inform.GetDataModelType().
const (
	connReqUsernameTR181 = "Device.ManagementServer.ConnectionRequestUsername"
	connReqPasswordTR181 = "Device.ManagementServer.ConnectionRequestPassword"
	connReqUsernameTR098 = "InternetGatewayDevice.ManagementServer.ConnectionRequestUsername"
	connReqPasswordTR098 = "InternetGatewayDevice.ManagementServer.ConnectionRequestPassword"

	// cwmpConnReqBucketPrefix mirrors the constant exposed by the nats package
	// (we duplicate it here to avoid an import cycle and to keep this file's
	// dependencies minimal for unit testing).
	cwmpConnReqBucketPrefix = "cwmp-conn-rq-"

	// provisionTimeout caps how long we wait for the CPE to respond to the
	// SetParameterValues we issue for credential provisioning. The CWMP
	// session can drag if the CPE is slow, but we never want the goroutine
	// to leak.
	provisionTimeout = 30 * time.Second
)

// kvForTenant resolves (and caches) the KeyValue handle for a tenant. If the
// bucket is missing we attempt a CreateOrUpdate so that tenants that pre-date
// this feature, or environments where the controller failed to create the
// bucket, still get provisioning working without manual intervention.
func (h *Handler) kvForTenant(tenant string) (jetstream.KeyValue, error) {
	if h.js == nil {
		return nil, errors.New("jetstream client not configured")
	}
	h.kvMu.Lock()
	kv, ok := h.kvCache[tenant]
	h.kvMu.Unlock()
	if ok {
		return kv, nil
	}

	ctx, cancel := context.WithTimeout(h.bgCtx, 5*time.Second)
	defer cancel()

	bucket := cwmpConnReqBucketPrefix + tenant
	kv, err := h.js.KeyValue(ctx, bucket)
	if err != nil {
		// Lazily create the bucket so that ACS keeps working when the
		// controller-side hook was skipped or the tenant pre-dates this code.
		log.Printf("CWMP conn-rq KV bucket %q missing (%v); creating", bucket, err)
		kv, err = h.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:      bucket,
			Description: "CWMP Connection Request credentials for tenant " + tenant,
		})
		if err != nil {
			return nil, fmt.Errorf("cwmp conn-rq KV ensure: %w", err)
		}
	}

	h.kvMu.Lock()
	h.kvCache[tenant] = kv
	h.kvMu.Unlock()
	return kv, nil
}

// loadConnReqCreds fetches stored credentials for (tenant, sn). Returns
// (creds, true, nil) on hit, (zero, false, nil) on a miss, and (zero, false, err)
// only for unexpected NATS errors.
func (h *Handler) loadConnReqCreds(tenant, sn string) (connReqCreds, bool, error) {
	kv, err := h.kvForTenant(tenant)
	if err != nil {
		return connReqCreds{}, false, err
	}
	ctx, cancel := context.WithTimeout(h.bgCtx, 5*time.Second)
	defer cancel()
	entry, err := kv.Get(ctx, sn)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return connReqCreds{}, false, nil
		}
		return connReqCreds{}, false, err
	}
	var creds connReqCreds
	if err := json.Unmarshal(entry.Value(), &creds); err != nil {
		// Corrupt entry — treat as a miss so the next provisioning cycle
		// overwrites it with valid JSON.
		log.Printf("CWMP conn-rq KV: corrupt entry for tenant=%s sn=%s, regenerating: %v", tenant, sn, err)
		return connReqCreds{}, false, nil
	}
	return creds, true, nil
}

func (h *Handler) storeConnReqCreds(tenant, sn string, creds connReqCreds) error {
	kv, err := h.kvForTenant(tenant)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(h.bgCtx, 5*time.Second)
	defer cancel()
	_, err = kv.Put(ctx, sn, raw)
	return err
}

// connReqParamNames returns the TR-181 (default) or TR-098 parameter names
// used for ConnectionRequest credentials on this CPE.
func connReqParamNames(dataModel string) (userParam, passParam string) {
	if dataModel == "TR098" {
		return connReqUsernameTR098, connReqPasswordTR098
	}
	return connReqUsernameTR181, connReqPasswordTR181
}

// generateConnReqCreds produces a fresh credential pair. We deliberately use
// the hex alphabet to dodge encoding quirks across CPE vendors (some choke on
// "/", "+", or "=" in HTTP Digest), and we cap the username length so that
// older devices with restrictive parameter constraints accept the value.
func generateConnReqCreds(now time.Time) (connReqCreds, error) {
	// 4 random bytes -> 8 hex chars -> "acs_xxxxxxxx" (12 chars total).
	userBytes := make([]byte, 4)
	if _, err := rand.Read(userBytes); err != nil {
		return connReqCreds{}, err
	}
	// 16 random bytes -> 32 hex chars. ~128 bits of entropy.
	passBytes := make([]byte, 16)
	if _, err := rand.Read(passBytes); err != nil {
		return connReqCreds{}, err
	}
	return connReqCreds{
		Username:      "acs_" + hex.EncodeToString(userBytes),
		Password:      hex.EncodeToString(passBytes),
		ProvisionedAt: now.Unix(),
	}, nil
}

// ensureConnReqCreds is invoked from the Inform handler under no lock. It
// returns the credentials that the in-memory CPE should advertise to the
// session bridge. On a fresh CPE it also enqueues a SetParameterValues into
// the CPE session queue and asynchronously persists the credentials to KV
// once the CPE acknowledges the write.
//
// Returns (username, password, false) on KV miss when ACS could not even
// start provisioning (in which case the caller should fall back to the global
// env credentials so that admin-driven Connection Requests still have a
// chance of working).
func (h *Handler) ensureConnReqCreds(tenant, sn string, cpe *CPE) (string, string, bool) {
	if h.js == nil {
		// Provisioning disabled (tests). Caller should rely on the env
		// fallback in connectionRequestCreds.
		return "", "", false
	}
	if tenant == "" || sn == "" {
		return "", "", false
	}

	if existing, ok, err := h.loadConnReqCreds(tenant, sn); err != nil {
		log.Printf("CWMP conn-rq: KV read failed for tenant=%s sn=%s, falling back to env: %v", tenant, sn, err)
		return "", "", false
	} else if ok {
		return existing.Username, existing.Password, true
	}

	// KV miss → first time we see this CPE (or its KV entry was wiped).
	// Generate fresh creds and ask the CPE to adopt them via SetParameterValues.
	creds, err := generateConnReqCreds(time.Now())
	if err != nil {
		log.Printf("CWMP conn-rq: failed to generate creds for tenant=%s sn=%s: %v", tenant, sn, err)
		return "", "", false
	}
	if err := h.enqueueProvisionSetParams(tenant, sn, cpe, creds); err != nil {
		log.Printf("CWMP conn-rq: failed to enqueue provisioning for tenant=%s sn=%s: %v", tenant, sn, err)
		return "", "", false
	}
	return creds.Username, creds.Password, true
}

// enqueueProvisionSetParams pushes a SetParameterValues onto the CPE's session
// queue and starts a goroutine that waits for the CPE's response. On a
// successful SetParameterValuesResponse it writes the credentials to KV so
// that future ACS restarts find them. On Fault / timeout it logs and gives
// up — the next Inform will retry from scratch.
func (h *Handler) enqueueProvisionSetParams(tenant, sn string, cpe *CPE, creds connReqCreds) error {
	if cpe == nil || cpe.Queue == nil {
		return errors.New("cpe queue not initialized")
	}

	userParam, passParam := connReqParamNames(cpe.DataModel)
	values := map[string]string{
		userParam: creds.Username,
		passParam: creds.Password,
	}
	payload := []byte(cwmp.SetParameterMultiValues(values))

	// Buffered so the writer (msgAnswer inside the session) never blocks
	// even if our listener already gave up due to provisionTimeout.
	resp := make(chan []byte, 1)
	cpe.Queue.Enqueue(Request{
		Id:       uuid.NewString(),
		CwmpMsg:  payload,
		Callback: resp,
		Time:     time.Now(),
	})

	go h.awaitProvisioningResponse(tenant, sn, creds, resp)
	return nil
}

// provisioningResponseAccepted returns true iff the CPE confirmed the
// SetParameterValues we issued for credential provisioning. Extracted from
// awaitProvisioningResponse so the XML branch logic can be unit-tested
// without standing up NATS or goroutines.
func provisioningResponseAccepted(raw []byte) (accepted bool, kind string, parseErr error) {
	var env cwmp.SoapEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return false, "", err
	}
	kind = env.KindOf()
	return kind == "SetParameterValuesResponse", kind, nil
}

// awaitProvisioningResponse waits for the CPE to respond to the
// SetParameterValues we issued, then (on success) commits the credentials
// to KV. It is run as a goroutine from enqueueProvisionSetParams.
func (h *Handler) awaitProvisioningResponse(tenant, sn string, creds connReqCreds, resp <-chan []byte) {
	select {
	case raw := <-resp:
		accepted, kind, err := provisioningResponseAccepted(raw)
		if err != nil {
			log.Printf("CWMP conn-rq: unparsable response from CPE %s/%s: %v", tenant, sn, err)
			return
		}
		if !accepted {
			log.Printf("CWMP conn-rq: CPE %s/%s rejected provisioning (kind=%q)", tenant, sn, kind)
			return
		}
		if err := h.storeConnReqCreds(tenant, sn, creds); err != nil {
			log.Printf("CWMP conn-rq: KV write failed for tenant=%s sn=%s: %v", tenant, sn, err)
			return
		}
		log.Printf("CWMP conn-rq: provisioned new credentials for tenant=%s sn=%s", tenant, sn)
	case <-time.After(provisionTimeout):
		log.Printf("CWMP conn-rq: provisioning timed out for tenant=%s sn=%s", tenant, sn)
	}
}

