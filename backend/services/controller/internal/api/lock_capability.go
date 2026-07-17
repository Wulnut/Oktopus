package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/api/middleware"
	"github.com/leandrofars/oktopus/internal/cwmp"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/entity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type lockProbeResult int

const (
	lockProbeOK lockProbeResult = iota
	lockProbeUnsupported
	lockProbeTransient
)

type lockCapabilitySnapshot struct {
	Result     lockProbeResult
	Detail     string
	LockStatus db.DeviceLockStatus
	ReportedIP string
	CWMPRoot   string
}

func normalizeActualLockValue(raw string) (db.DeviceLockStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "0", "false", "unlocked":
		return db.LockStatusUnlocked, true
	case "1", "true", "locked":
		return db.LockStatusLocked, true
	default:
		return "", false
	}
}

func malformedActualLockSnapshot(raw string) lockCapabilitySnapshot {
	return lockCapabilitySnapshot{
		Result: lockProbeTransient,
		Detail: fmt.Sprintf("unrecognized OntLock Lock value %q", truncateBackoffError(raw)),
	}
}

func classifyLockProbeError(err error) lockProbeResult {
	if err == nil {
		return lockProbeOK
	}
	if isPermanentLockCommandError(err) {
		return lockProbeUnsupported
	}
	// Schema miss surfaced as empty GetResp (parameter absent) rather than USP 7026.
	if strings.Contains(err.Error(), "not found in USP response") {
		return lockProbeUnsupported
	}
	return lockProbeTransient
}

// probeOntLockCapability verifies the device exposes OntLock Lock (+ WanIP) paths.
// Permanent schema errors → unsupported; timeout/transport → transient.
func (a *Api) probeOntLockCapability(ctx context.Context, device entity.Device, tenantSlug string) lockCapabilitySnapshot {
	_ = ctx
	mtp := lockDeviceMTP(device)
	if mtp == "" {
		return lockCapabilitySnapshot{Result: lockProbeTransient, Detail: "no active MTP"}
	}

	if mtp == "cwmp" {
		return a.probeOntLockCapabilityCWMP(device.SN, device.DataModel, tenantSlug)
	}
	return a.probeOntLockCapabilityUSP(device.SN, mtp, tenantSlug)
}

type uspLockProbeGetter func(sn, path, mtp, tenantSlug string) (string, error)

func (a *Api) probeOntLockCapabilityUSP(sn, mtp, tenantSlug string) lockCapabilitySnapshot {
	return probeOntLockCapabilityUSPWithGetter(sn, mtp, tenantSlug, func(sn, path, mtp, tenantSlug string) (string, error) {
		return uspGetValue(sn, path, mtp, a.nc, tenantSlug)
	})
}

func probeOntLockCapabilityUSPWithGetter(sn, mtp, tenantSlug string, get uspLockProbeGetter) lockCapabilitySnapshot {
	// Lock path is definitive OntLock support; WanIP confirms the object is usable.
	lockValue, err := get(sn, lockParameterPath, mtp, tenantSlug)
	if err != nil {
		return lockCapabilitySnapshot{Result: classifyLockProbeError(err), Detail: err.Error()}
	}
	lockStatus, ok := normalizeActualLockValue(lockValue)
	if !ok {
		return malformedActualLockSnapshot(lockValue)
	}
	reportedIP, err := get(sn, lockWanIPPath, mtp, tenantSlug)
	if err != nil {
		return lockCapabilitySnapshot{Result: classifyLockProbeError(err), Detail: err.Error()}
	}
	return lockCapabilitySnapshot{
		Result:     lockProbeOK,
		LockStatus: lockStatus,
		ReportedIP: normalizeReportedIP(reportedIP),
	}
}

type cwmpLockProbeGetter func(sn string, names []string, tenantSlug string) (cwmp.GetParameterValuesResponse, error)

func (a *Api) probeOntLockCapabilityCWMP(sn, dataModel, tenantSlug string) lockCapabilitySnapshot {
	return probeOntLockCapabilityCWMPWithGetter(sn, dataModel, tenantSlug, func(sn string, names []string, tenantSlug string) (cwmp.GetParameterValuesResponse, error) {
		return cwmpGetValues(sn, names, a.nc, tenantSlug)
	})
}

func probeOntLockCapabilityCWMPWithGetter(sn, dataModel, tenantSlug string, get cwmpLockProbeGetter) lockCapabilitySnapshot {
	// Try the datamodel-native root first, then the alternate root. A partial
	// response under one root must not prevent a complete alternate-root match.
	roots := []string{dataModel, lockOppositeDataModel(dataModel)}
	var details []string
	hasTransientFailure := false
	for _, dm := range roots {
		lockPath := lockParamPathCWMP(dm)
		wanPath := lockWanIPPathCWMP(dm)
		resp, err := get(sn, []string{lockPath, wanPath}, tenantSlug)
		if err != nil {
			if classifyLockProbeError(err) == lockProbeTransient {
				hasTransientFailure = true
			}
			details = append(details, err.Error())
			continue
		}
		var lockValue, reportedIP string
		foundLock, foundWan := false, false
		for _, param := range resp.ParameterList {
			switch param.Name {
			case lockPath:
				foundLock = true
				lockValue = param.Value
			case wanPath:
				foundWan = true
				reportedIP = param.Value
			}
		}
		if foundLock && foundWan {
			lockStatus, ok := normalizeActualLockValue(lockValue)
			if !ok {
				hasTransientFailure = true
				details = append(details, malformedActualLockSnapshot(lockValue).Detail)
				continue
			}
			return lockCapabilitySnapshot{
				Result:     lockProbeOK,
				LockStatus: lockStatus,
				ReportedIP: normalizeReportedIP(reportedIP),
				CWMPRoot:   lockCWMPRootPrefix(dm),
			}
		}
		details = append(details, fmt.Sprintf("OntLock parameters incomplete under %s", lockCWMPRootPrefix(dm)))
	}
	detail := strings.Join(details, "; ")
	if hasTransientFailure {
		return lockCapabilitySnapshot{Result: lockProbeTransient, Detail: detail}
	}
	if detail == "" {
		detail = "OntLock parameters not found in CWMP response under either root"
	}
	return lockCapabilitySnapshot{Result: lockProbeUnsupported, Detail: detail}
}

// shouldProbeOntLockCapability decides whether to run an OntLock capability probe.
//
//	opt_out → never probe
//	existing unsupported row && trigger != online → skip (no poll/chase/notify hammer)
//	online → always re-probe (firmware upgrade auto-enroll)
//	no row yet → probe on any trigger that reaches the gate
func shouldProbeOntLockCapability(trigger string, hasUnsupportedRow bool, optOut bool) bool {
	if optOut {
		return false
	}
	if hasUnsupportedRow && trigger != lockTriggerOnline {
		return false
	}
	return true
}

// gateLockCapability runs the OntLock capability gate before evaluate.
// Returns true when evaluate should proceed.
//
// Order: load unsupported row → shouldProbe (opt_out / skip-without-probe) →
// probe → unsupported upsert+audit / transient skip / OK delete-row.
// Mongo Get/Upsert/Delete failures soft-degrade (log and continue) except when
// the probe itself classified unsupported.
func (a *Api) gateLockCapability(ctx context.Context, tdb *db.TenantDB, device entity.Device, tenantSlug, trigger string) (lockCapabilitySnapshot, bool) {
	hasRow, optOut := a.unsupportedLockRowState(ctx, tdb, device.SN)
	if !shouldProbeOntLockCapability(trigger, hasRow, optOut) {
		return lockCapabilitySnapshot{}, false
	}

	snapshot := a.probeOntLockCapability(ctx, device, tenantSlug)
	switch snapshot.Result {
	case lockProbeTransient:
		log.Printf("lock_capability: probe transient %s: %s", device.SN, snapshot.Detail)
		return snapshot, false
	case lockProbeUnsupported:
		if err := tdb.UpsertUnsupportedLockDevice(ctx, db.UnsupportedLockDevice{
			SN:            device.SN,
			Reason:        db.LockUnsupportedReasonPath,
			Detail:        snapshot.Detail,
			LastCheckedAt: time.Now(),
		}); err != nil {
			log.Printf("lock_capability: upsert unsupported %s: %v", device.SN, err)
		}
		a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
			SN:     device.SN,
			Action: "unsupported",
			Details: bson.M{
				"reason": db.LockUnsupportedReasonPath,
				"detail": snapshot.Detail,
			},
		})
		return snapshot, false
	default:
		// Probe OK — clear any prior unsupported row (idempotent).
		if delErr := tdb.DeleteUnsupportedLockDevice(ctx, device.SN); delErr != nil {
			log.Printf("lock_capability: delete unsupported %s: %v", device.SN, delErr)
		}
		return snapshot, true
	}
}

// unsupportedLockRowState returns whether an unsupported row exists and its opt_out.
// Get errors soft-degrade to (false, false) so evaluate can continue to probe.
func (a *Api) unsupportedLockRowState(ctx context.Context, tdb *db.TenantDB, sn string) (hasRow bool, optOut bool) {
	row, err := tdb.GetUnsupportedLockDevice(ctx, sn)
	if err == mongo.ErrNoDocuments {
		return false, false
	}
	if err != nil {
		log.Printf("lock_capability: get unsupported %s: %v (continuing)", sn, err)
		return false, false
	}
	return true, row.OptOut
}

// unsupportedLockOptedOut returns true when Mongo has opt_out set.
// Get errors soft-degrade to false so evaluate can continue.
func (a *Api) unsupportedLockOptedOut(ctx context.Context, tdb *db.TenantDB, sn string) bool {
	_, optOut := a.unsupportedLockRowState(ctx, tdb, sn)
	return optOut
}

func (a *Api) listUnsupportedLockDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := a.tenantDB(r).ListUnsupportedLockDevices(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if devices == nil {
		devices = []db.UnsupportedLockDevice{}
	}
	writeJSON(w, http.StatusOK, devices)
}

func (a *Api) optOutUnsupportedLockDevice(w http.ResponseWriter, r *http.Request) {
	sn := mux.Vars(r)["sn"]
	if sn == "" {
		http.Error(w, "sn is required", http.StatusBadRequest)
		return
	}
	tenantSlug := middleware.GetTenantSlug(r)
	operatorID := middleware.GetEmail(r)
	out, err := a.tenantDB(r).SetUnsupportedLockOptOut(r.Context(), sn, true, operatorID)
	if err == mongo.ErrNoDocuments {
		http.Error(w, "unsupported device not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.recordLockAudit(r.Context(), a.tenantDB(r), tenantSlug, db.LockAuditLog{
		SN:         out.SN,
		Action:     "unsupported_opt_out",
		OperatorID: operatorID,
	})
	writeJSON(w, http.StatusOK, out)
}

// batchDeleteUnsupportedLockDevices removes entries from the unsupported list
// only while the database row is still opt_out=true. The condition is part of
// the DeleteMany filter, eliminating the check/delete race with opt-out changes.
func (a *Api) batchDeleteUnsupportedLockDevices(w http.ResponseWriter, r *http.Request) {
	var req lockBatchDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if len(req.SNs) == 0 {
		http.Error(w, "sns is required", http.StatusBadRequest)
		return
	}
	tenantSlug := middleware.GetTenantSlug(r)
	operator := middleware.GetEmail(r)
	tdb := a.tenantDB(r)
	deleted, err := tdb.DeleteUnsupportedLockDevices(r.Context(), req.SNs)
	result := lockBatchDeleteResult{Deleted: deleted}
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		writeJSON(w, http.StatusMultiStatus, result)
		return
	}
	a.recordLockAudit(r.Context(), tdb, tenantSlug, db.LockAuditLog{
		Action:     "unsupported_batch_delete",
		OperatorID: operator,
		Details: bson.M{
			"requested_sns": req.SNs,
			"deleted":       deleted,
		},
	})
	writeJSON(w, http.StatusOK, result)
}

func (a *Api) clearOptOutUnsupportedLockDevice(w http.ResponseWriter, r *http.Request) {
	sn := mux.Vars(r)["sn"]
	if sn == "" {
		http.Error(w, "sn is required", http.StatusBadRequest)
		return
	}
	tenantSlug := middleware.GetTenantSlug(r)
	operatorID := middleware.GetEmail(r)
	out, err := a.tenantDB(r).SetUnsupportedLockOptOut(r.Context(), sn, false, operatorID)
	if err == mongo.ErrNoDocuments {
		http.Error(w, "unsupported device not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.recordLockAudit(r.Context(), a.tenantDB(r), tenantSlug, db.LockAuditLog{
		SN:         out.SN,
		Action:     "unsupported_opt_out_clear",
		OperatorID: operatorID,
	})
	writeJSON(w, http.StatusOK, out)
}
