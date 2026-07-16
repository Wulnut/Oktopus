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
func (a *Api) probeOntLockCapability(ctx context.Context, device entity.Device, tenantSlug string) (lockProbeResult, string) {
	_ = ctx
	mtp := lockDeviceMTP(device)
	if mtp == "" {
		return lockProbeTransient, "no active MTP"
	}

	if mtp == "cwmp" {
		return a.probeOntLockCapabilityCWMP(device.SN, device.DataModel, tenantSlug)
	}
	return a.probeOntLockCapabilityUSP(device.SN, mtp, tenantSlug)
}

func (a *Api) probeOntLockCapabilityUSP(sn, mtp, tenantSlug string) (lockProbeResult, string) {
	// Lock path is definitive OntLock support; WanIP confirms the object is usable.
	if _, err := uspGetValue(sn, lockParameterPath, mtp, a.nc, tenantSlug); err != nil {
		return classifyLockProbeError(err), err.Error()
	}
	if _, err := uspGetValue(sn, lockWanIPPath, mtp, a.nc, tenantSlug); err != nil {
		return classifyLockProbeError(err), err.Error()
	}
	return lockProbeOK, ""
}

type cwmpLockProbeGetter func(sn string, names []string, tenantSlug string) (cwmp.GetParameterValuesResponse, error)

func (a *Api) probeOntLockCapabilityCWMP(sn, dataModel, tenantSlug string) (lockProbeResult, string) {
	return probeOntLockCapabilityCWMPWithGetter(sn, dataModel, tenantSlug, func(sn string, names []string, tenantSlug string) (cwmp.GetParameterValuesResponse, error) {
		return cwmpGetValues(sn, names, a.nc, tenantSlug)
	})
}

func probeOntLockCapabilityCWMPWithGetter(sn, dataModel, tenantSlug string, get cwmpLockProbeGetter) (lockProbeResult, string) {
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
		foundLock, foundWan := false, false
		for _, param := range resp.ParameterList {
			switch param.Name {
			case lockPath:
				foundLock = true
			case wanPath:
				foundWan = true
			}
		}
		if foundLock && foundWan {
			return lockProbeOK, ""
		}
		details = append(details, fmt.Sprintf("OntLock parameters incomplete under %s", lockCWMPRootPrefix(dm)))
	}
	detail := strings.Join(details, "; ")
	if hasTransientFailure {
		return lockProbeTransient, detail
	}
	if detail == "" {
		detail = "OntLock parameters not found in CWMP response under either root"
	}
	return lockProbeUnsupported, detail
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
func (a *Api) gateLockCapability(ctx context.Context, tdb *db.TenantDB, device entity.Device, tenantSlug, trigger string) bool {
	hasRow, optOut := a.unsupportedLockRowState(ctx, tdb, device.SN)
	if !shouldProbeOntLockCapability(trigger, hasRow, optOut) {
		return false
	}

	result, detail := a.probeOntLockCapability(ctx, device, tenantSlug)
	switch result {
	case lockProbeTransient:
		log.Printf("lock_capability: probe transient %s: %s", device.SN, detail)
		return false
	case lockProbeUnsupported:
		if err := tdb.UpsertUnsupportedLockDevice(ctx, db.UnsupportedLockDevice{
			SN:            device.SN,
			Reason:        db.LockUnsupportedReasonPath,
			Detail:        detail,
			LastCheckedAt: time.Now(),
		}); err != nil {
			log.Printf("lock_capability: upsert unsupported %s: %v", device.SN, err)
		}
		a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
			SN:     device.SN,
			Action: "unsupported",
			Details: bson.M{
				"reason": db.LockUnsupportedReasonPath,
				"detail": detail,
			},
		})
		return false
	default:
		// Probe OK — clear any prior unsupported row (idempotent).
		if delErr := tdb.DeleteUnsupportedLockDevice(ctx, device.SN); delErr != nil {
			log.Printf("lock_capability: delete unsupported %s: %v", device.SN, delErr)
		}
		return true
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
