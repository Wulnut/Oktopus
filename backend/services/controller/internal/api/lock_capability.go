package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/api/middleware"
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
		return a.probeOntLockCapabilityCWMP(device.SN, tenantSlug)
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

func (a *Api) probeOntLockCapabilityCWMP(sn, tenantSlug string) (lockProbeResult, string) {
	resp, err := cwmpGetValues(sn, []string{lockParameterPath, lockWanIPPath}, a.nc, tenantSlug)
	if err != nil {
		return classifyLockProbeError(err), err.Error()
	}
	foundLock, foundWan := false, false
	for _, param := range resp.ParameterList {
		switch param.Name {
		case lockParameterPath:
			foundLock = true
		case lockWanIPPath:
			foundWan = true
		}
	}
	if !foundLock {
		detail := fmt.Sprintf("parameter %s not found in CWMP response", lockParameterPath)
		return lockProbeUnsupported, detail
	}
	if !foundWan {
		detail := fmt.Sprintf("parameter %s not found in CWMP response", lockWanIPPath)
		return lockProbeUnsupported, detail
	}
	return lockProbeOK, ""
}

// gateLockCapability runs the OntLock capability gate before evaluate.
// Returns true when evaluate should proceed.
//
// Order: opt_out fast-path (caller may check before TryLock) → probe →
// unsupported upsert+audit / transient skip / OK delete-row.
// Mongo Get/Upsert/Delete failures soft-degrade (log and continue) except when
// the probe itself classified unsupported.
func (a *Api) gateLockCapability(ctx context.Context, tdb *db.TenantDB, device entity.Device, tenantSlug string) bool {
	if a.unsupportedLockOptedOut(ctx, tdb, device.SN) {
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

// unsupportedLockOptedOut returns true when Mongo has opt_out set.
// Get errors soft-degrade to false so evaluate can continue.
func (a *Api) unsupportedLockOptedOut(ctx context.Context, tdb *db.TenantDB, sn string) bool {
	row, err := tdb.GetUnsupportedLockDevice(ctx, sn)
	if err == mongo.ErrNoDocuments {
		return false
	}
	if err != nil {
		log.Printf("lock_capability: get unsupported %s: %v (continuing)", sn, err)
		return false
	}
	return row.OptOut
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
