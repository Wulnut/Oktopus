package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/leandrofars/oktopus/internal/api/middleware"
	"github.com/leandrofars/oktopus/internal/db"
)

// GET /api/campaigns
func (a *Api) listCampaigns(w http.ResponseWriter, r *http.Request) {
	list, err := a.tenantDB(r).ListCampaigns(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []db.Campaign{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// POST /api/campaigns
func (a *Api) createCampaign(w http.ResponseWriter, r *http.Request) {
	var c db.Campaign
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	c.Vendor = strings.TrimSpace(c.Vendor)
	c.Model = strings.TrimSpace(c.Model)
	c.HWVersion = strings.TrimSpace(c.HWVersion)

	if c.Vendor == "" || c.Model == "" || c.HWVersion == "" {
		http.Error(w, "vendor, model, and hw_version are required", http.StatusBadRequest)
		return
	}
	if c.FirmwareID.IsZero() {
		http.Error(w, "firmware_id is required", http.StatusBadRequest)
		return
	}
	if _, err := a.tenantDB(r).GetFirmware(r.Context(), c.FirmwareID); err != nil {
		http.Error(w, "firmware not found", http.StatusBadRequest)
		return
	}
	if c.Concurrency < 1 {
		c.Concurrency = 10
	} else if c.Concurrency > 50 {
		c.Concurrency = 50
	}

	created, err := a.tenantDB(r).CreateCampaign(r.Context(), c)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			http.Error(w, "campaign already exists for this vendor+model+hw_version", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if created.Enabled {
		tdb := a.tenantDB(r)
		go a.RunCampaignBatch(tdb, created, middleware.GetTenantSlug(r))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// PUT /api/campaigns/{id}
func (a *Api) updateCampaign(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body db.Campaign
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if !body.FirmwareID.IsZero() {
		if _, err := a.tenantDB(r).GetFirmware(r.Context(), body.FirmwareID); err != nil {
			http.Error(w, "firmware not found", http.StatusBadRequest)
			return
		}
	}
	if body.Concurrency < 1 {
		body.Concurrency = 10
	} else if body.Concurrency > 50 {
		body.Concurrency = 50
	}
	matched, err := a.tenantDB(r).UpdateCampaign(r.Context(), id, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if matched == 0 {
		http.Error(w, "campaign not found", http.StatusNotFound)
		return
	}

	if body.Enabled {
		tdb := a.tenantDB(r)
		updated, fetchErr := tdb.GetCampaign(r.Context(), id)
		if fetchErr == nil {
			go a.RunCampaignBatch(tdb, updated, middleware.GetTenantSlug(r))
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/campaigns/{id}
func (a *Api) deleteCampaign(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := a.tenantDB(r).DeleteCampaign(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/campaigns/{id}/logs?page=0&page_size=20
func (a *Api) campaignUpgradeLogs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	page, _ := strconv.ParseInt(r.URL.Query().Get("page"), 10, 64)
	pageSize, _ := strconv.ParseInt(r.URL.Query().Get("page_size"), 10, 64)
	if pageSize <= 0 {
		pageSize = 20
	}

	logs, total, err := a.tenantDB(r).ListUpgradeLogsByCampaign(r.Context(), id, page, pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if logs == nil {
		logs = []db.FirmwareUpgradeLog{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"logs":  logs,
		"total": total,
	})
}

// GET /api/device/{sn}/fw-policy
func (a *Api) getDeviceFWPolicy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sn := vars["sn"]
	if sn == "" {
		http.Error(w, "device serial number is required", http.StatusBadRequest)
		return
	}

	policy, err := a.tenantDB(r).GetDeviceFWPolicy(r.Context(), sn)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(policy)
}

// PUT /api/device/{sn}/fw-policy
func (a *Api) setDeviceFWPolicy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sn := vars["sn"]
	if sn == "" {
		http.Error(w, "device serial number is required", http.StatusBadRequest)
		return
	}

	var body db.DeviceFWPolicy
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Policy != "campaign" && body.Policy != "skip" && body.Policy != "manual" {
		http.Error(w, "policy must be 'campaign', 'skip', or 'manual'", http.StatusBadRequest)
		return
	}
	body.DeviceSN = sn

	if err := a.tenantDB(r).SetDeviceFWPolicy(r.Context(), body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/device/{sn}/upgrade-logs
func (a *Api) deviceUpgradeLogs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sn := vars["sn"]
	if sn == "" {
		http.Error(w, "device serial number is required", http.StatusBadRequest)
		return
	}

	logs, err := a.tenantDB(r).ListUpgradeLogsByDevice(r.Context(), sn)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if logs == nil {
		logs = []db.FirmwareUpgradeLog{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}
