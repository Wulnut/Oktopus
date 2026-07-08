package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/api/middleware"
	"github.com/leandrofars/oktopus/internal/db"
	"go.mongodb.org/mongo-driver/bson"
)

type lockBatchRequest struct {
	Items []db.LockPolicy `json:"items"`
}

type lockBatchResult struct {
	Created int      `json:"created"`
	Errors  []string `json:"errors"`
}


type batchUnauthorizedWhitelistRequest struct {
	Items                  []unauthorizedWhitelistItem `json:"items"`
	RemoveFromUnauthorized bool                        `json:"remove_from_unauthorized"`
}

type unauthorizedWhitelistItem struct {
	SN             string `json:"sn"`
	AllowedIPRange string `json:"allowed_ip_range"`
	ReportedIP       string `json:"reported_ip"`
	Description    string `json:"description"`
}

func (a *Api) listLockPolicies(w http.ResponseWriter, r *http.Request) {
	policyType := db.LockPolicyType(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("type"))))
	policies, err := a.tenantDB(r).ListLockPolicies(r.Context(), policyType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, policies)
}

func (a *Api) upsertWhitelistPolicy(w http.ResponseWriter, r *http.Request) {
	a.upsertLockPolicy(w, r)
}

func (a *Api) upsertLockPolicy(w http.ResponseWriter, r *http.Request) {
	var policy db.LockPolicy
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	policy.PolicyType = db.LockPolicyWhitelist
	policy.Status = true
	policy.OperatorID = middleware.GetEmail(r)
	tenantSlug := middleware.GetTenantSlug(r)

	created, err := a.tenantDB(r).UpsertLockPolicy(r.Context(), policy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = lockCache.PutLockPolicy(r.Context(), tenantSlug, created)
	a.recordLockAudit(r.Context(), a.tenantDB(r), tenantSlug, db.LockAuditLog{
		SN:          created.SN,
		Action:      "policy_upsert",
		PolicyType:  created.PolicyType,
		OperatorID:  created.OperatorID,
		Description: created.Description,
		Details: bson.M{
			"allowed_ip_range": created.AllowedIPRange,
			"reason_code":      created.ReasonCode,
		},
	})
	_ = a.tenantDB(r).DeleteUnauthorizedDevice(r.Context(), created.SN)
	a.chaseLockForSN(tenantSlug, created.SN)
	writeJSON(w, http.StatusOK, created)
}

func (a *Api) deleteLockPolicy(w http.ResponseWriter, r *http.Request) {
	sn := mux.Vars(r)["sn"]
	if sn == "" {
		http.Error(w, "sn is required", http.StatusBadRequest)
		return
	}
	tenantSlug := middleware.GetTenantSlug(r)
	if err := a.tenantDB(r).DeleteLockPolicy(r.Context(), sn); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = lockCache.DeleteLockPolicy(r.Context(), tenantSlug, sn)
	a.recordLockAudit(r.Context(), a.tenantDB(r), tenantSlug, db.LockAuditLog{
		SN:         sn,
		Action:     "policy_delete",
		OperatorID: middleware.GetEmail(r),
	})
	a.chaseLockAfterPolicyDelete(tenantSlug, sn)
	w.WriteHeader(http.StatusNoContent)
}

type lockBatchDeleteRequest struct {
	SNs []string `json:"sns"`
}

type lockBatchDeleteResult struct {
	Deleted int64    `json:"deleted"`
	Errors  []string `json:"errors"`
}

func (a *Api) batchDeleteLockPolicies(w http.ResponseWriter, r *http.Request) {
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
	deleted, err := a.tenantDB(r).DeleteLockPolicies(r.Context(), req.SNs)
	result := lockBatchDeleteResult{Deleted: deleted}
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
	for _, sn := range req.SNs {
		_ = lockCache.DeleteLockPolicy(r.Context(), tenantSlug, sn)
		a.recordLockAudit(r.Context(), a.tenantDB(r), tenantSlug, db.LockAuditLog{
			SN:         sn,
			Action:     "policy_batch_delete",
			OperatorID: middleware.GetEmail(r),
		})
	}
	a.chaseLockForSNs(tenantSlug, req.SNs)
	status := http.StatusOK
	if len(result.Errors) > 0 {
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, result)
}

func (a *Api) batchWhitelistPolicies(w http.ResponseWriter, r *http.Request) {
	a.batchLockPolicies(w, r)
}

func (a *Api) batchLockPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := decodeLockBatch(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tenantSlug := middleware.GetTenantSlug(r)
	result := lockBatchResult{}
	for idx, policy := range policies {
		policy.PolicyType = db.LockPolicyWhitelist
		policy.Status = true
		policy.OperatorID = middleware.GetEmail(r)
		created, err := a.tenantDB(r).UpsertLockPolicy(r.Context(), policy)
		if err != nil {
			result.Errors = append(result.Errors, "row "+strconv.Itoa(idx+1)+": "+err.Error())
			continue
		}
		_ = lockCache.PutLockPolicy(r.Context(), tenantSlug, created)
		result.Created++
		if created.PolicyType == db.LockPolicyWhitelist {
			_ = a.tenantDB(r).DeleteUnauthorizedDevice(r.Context(), created.SN)
		}
		a.recordLockAudit(r.Context(), a.tenantDB(r), tenantSlug, db.LockAuditLog{
			SN:          created.SN,
			Action:      "policy_batch_upsert",
			PolicyType:  created.PolicyType,
			OperatorID:  created.OperatorID,
			Description: created.Description,
		})
	}
	if a.lockCircuitBreaker != nil {
		a.lockCircuitBreaker.reset(tenantSlug)
	}
	go a.chaseLockAfterConfigUpdate(tenantSlug)
	status := http.StatusAccepted
	if len(result.Errors) > 0 {
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, result)
}

func decodeLockBatch(r *http.Request) ([]db.LockPolicy, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/csv") {
		return decodeLockCSV(r)
	}
	var req lockBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	if len(req.Items) == 0 {
		return nil, errors.New("items is required")
	}
	return req.Items, nil
}

func decodeLockCSV(r *http.Request) ([]db.LockPolicy, error) {
	reader := csv.NewReader(r.Body)
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, errors.New("csv requires a header and at least one row")
	}
	header := map[string]int{}
	for idx, col := range records[0] {
		header[strings.ToLower(strings.TrimSpace(col))] = idx
	}
	var policies []db.LockPolicy
	for _, row := range records[1:] {
		policies = append(policies, db.LockPolicy{
			SN:             csvValue(row, header, "sn"),
			AllowedIPRange: csvValue(row, header, "allowed_ip_range"),
			ReasonCode:     csvValue(row, header, "reason"),
			Description:    csvValue(row, header, "description"),
		})
	}
	return policies, nil
}

func csvValue(row []string, header map[string]int, name string) string {
	idx, ok := header[name]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func (a *Api) getLockConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.tenantDB(r).GetLockConfig(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (a *Api) updateLockConfig(w http.ResponseWriter, r *http.Request) {
	var cfg db.LockConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	cfg.UpdatedBy = middleware.GetEmail(r)
	tenantSlug := middleware.GetTenantSlug(r)
	saved, err := a.tenantDB(r).SaveLockConfig(r.Context(), cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.recordLockAudit(r.Context(), a.tenantDB(r), tenantSlug, db.LockAuditLog{
		Action:     "config_update",
		OperatorID: cfg.UpdatedBy,
		Details: bson.M{
			"master_enabled":    saved.MasterEnabled,
			"auto_lock_enabled": saved.AutoLockEnabled,
		},
	})
	if a.lockCircuitBreaker != nil {
		a.lockCircuitBreaker.reset(tenantSlug)
	}
	a.chaseLockAfterConfigUpdate(tenantSlug)
	writeJSON(w, http.StatusOK, saved)
}

func (a *Api) batchWhitelistFromUnauthorized(w http.ResponseWriter, r *http.Request) {
	var req batchUnauthorizedWhitelistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if len(req.Items) == 0 {
		http.Error(w, "items is required", http.StatusBadRequest)
		return
	}

	tenantSlug := middleware.GetTenantSlug(r)
	result := lockBatchResult{}
	removedSNs := make([]string, 0, len(req.Items))
	for idx, item := range req.Items {
		policy := db.LockPolicy{
			SN:             item.SN,
			AllowedIPRange: item.AllowedIPRange,
			Description:    item.Description,
			PolicyType:     db.LockPolicyWhitelist,
			Status:         true,
			OperatorID:     middleware.GetEmail(r),
		}
		if policy.AllowedIPRange == "" && item.ReportedIP != "" {
			policy.AllowedIPRange = item.ReportedIP + "/32"
		}
		created, err := a.tenantDB(r).UpsertLockPolicy(r.Context(), policy)
		if err != nil {
			result.Errors = append(result.Errors, "row "+strconv.Itoa(idx+1)+": "+err.Error())
			continue
		}
		_ = lockCache.PutLockPolicy(r.Context(), tenantSlug, created)
		result.Created++
		removedSNs = append(removedSNs, created.SN)
		a.recordLockAudit(r.Context(), a.tenantDB(r), tenantSlug, db.LockAuditLog{
			SN:          created.SN,
			Action:      "unauthorized_batch_whitelist",
			PolicyType:  created.PolicyType,
			OperatorID:  created.OperatorID,
			Description: created.Description,
		})
	}
	if req.RemoveFromUnauthorized {
		_ = a.tenantDB(r).DeleteUnauthorizedDevices(r.Context(), removedSNs)
	}
	if a.lockCircuitBreaker != nil {
		a.lockCircuitBreaker.reset(tenantSlug)
	}
	go a.chaseLockAfterConfigUpdate(tenantSlug)
	status := http.StatusAccepted
	if len(result.Errors) > 0 {
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, result)
}

func (a *Api) listUnauthorizedDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := a.tenantDB(r).ListUnauthorizedDevices(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

func (a *Api) listLockCommands(w http.ResponseWriter, r *http.Request) {
	commands, err := a.tenantDB(r).ListLockCommands(r.Context(), r.URL.Query().Get("sn"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, commands)
}

func (a *Api) listLockAuditLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := a.tenantDB(r).ListLockAuditLogs(r.Context(), r.URL.Query().Get("sn"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

func (a *Api) clearLockAuditLogs(w http.ResponseWriter, r *http.Request) {
	tenantSlug := middleware.GetTenantSlug(r)
	deleted, err := a.tenantDB(r).ClearLockAuditLogs(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// The purge itself is an auditable, privileged action; keep a trace.
	a.recordLockAudit(r.Context(), a.tenantDB(r), tenantSlug, db.LockAuditLog{
		Action:      "audit_clear",
		OperatorID:  middleware.GetEmail(r),
		Description: "Audit history cleared",
		Details:     bson.M{"deleted_count": deleted},
	})
	writeJSON(w, http.StatusOK, map[string]int64{"deleted": deleted})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
