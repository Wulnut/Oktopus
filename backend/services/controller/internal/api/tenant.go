package api

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/api/middleware"
	"github.com/leandrofars/oktopus/internal/db"
	local "github.com/leandrofars/oktopus/internal/nats"
)

var slugRegex = regexp.MustCompile(`[^a-z0-9-]+`)

// generateSlug converts a name to a URL-friendly slug.
func generateSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	// Collapse multiple hyphens
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return s
}

// POST /api/tenants (SuperAdmin only)
func (a *Api) createTenant(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if db.UserLevels(level) != db.SuperAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	var body struct {
		Name       string              `json:"name"`
		AuthPolicy db.TenantAuthPolicy `json:"auth_policy"`
		AdminEmail string              `json:"admin_email"`
		AdminName  string              `json:"admin_name"`
		AdminPass  string              `json:"admin_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	slug := generateSlug(body.Name)
	if slug == "" {
		http.Error(w, `{"error":"name produces empty slug"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	tenant := db.Tenant{
		Name:       body.Name,
		Slug:       slug,
		Status:     db.TenantStatusActive,
		AuthPolicy: body.AuthPolicy,
	}
	created, err := a.db.CreateTenant(ctx, tenant)
	if err != nil {
		http.Error(w, `{"error":"failed to create tenant: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// Provision tenant databases
	if err := a.db.ProvisionTenantDBs(ctx, slug); err != nil {
		http.Error(w, `{"error":"failed to provision databases: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// Create tenant KV bucket for device auth
	if _, err := local.CreateTenantKVBucket(a.js, slug); err != nil {
		http.Error(w, `{"error":"failed to create KV bucket: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// Create initial TenantAdmin user if credentials provided
	if body.AdminEmail != "" && body.AdminPass != "" {
		adminUser := db.User{
			Email:    body.AdminEmail,
			Name:     body.AdminName,
			Level:    db.TenantAdmin,
			TenantID: created.ID,
		}
		if err := adminUser.HashPassword(body.AdminPass); err != nil {
			http.Error(w, `{"error":"failed to hash password"}`, http.StatusInternalServerError)
			return
		}
		if err := a.db.RegisterUser(adminUser); err != nil {
			http.Error(w, `{"error":"failed to create admin user: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// GET /api/tenants (SuperAdmin only)
func (a *Api) listTenants(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if db.UserLevels(level) != db.SuperAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	tenantsList, err := a.db.FindAllTenants(r.Context())
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	if tenantsList == nil {
		tenantsList = []db.Tenant{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenantsList)
}

// GET /api/tenants/{slug}
func (a *Api) getTenant(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	tenantSlug := middleware.GetTenantSlug(r)
	vars := mux.Vars(r)
	slug := vars["slug"]

	// Tenant users can only view their own tenant
	if db.UserLevels(level) != db.SuperAdmin && slug != tenantSlug {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	tenant, err := a.db.FindTenant(r.Context(), slug)
	if err != nil {
		http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenant)
}

// PUT /api/tenants/{slug} (SuperAdmin only)
func (a *Api) updateTenant(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if db.UserLevels(level) != db.SuperAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	slug := vars["slug"]

	tenant, err := a.db.FindTenant(r.Context(), slug)
	if err != nil {
		http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
		return
	}

	var body struct {
		Name       string              `json:"name"`
		Status     db.TenantStatus     `json:"status"`
		AuthPolicy db.TenantAuthPolicy `json:"auth_policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	if body.Name != "" {
		tenant.Name = body.Name
	}
	if body.Status != "" {
		tenant.Status = body.Status
	}
	tenant.AuthPolicy = body.AuthPolicy
	tenant.UpdatedAt = time.Now()

	if err := a.db.UpdateTenant(r.Context(), tenant.ID, tenant); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/tenants/{slug} (SuperAdmin only)
func (a *Api) deleteTenant(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if db.UserLevels(level) != db.SuperAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	slug := vars["slug"]

	tenant, err := a.db.FindTenant(r.Context(), slug)
	if err != nil {
		http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
		return
	}

	ctx := r.Context()

	// Delete all users belonging to this tenant
	if err := a.db.DeleteUsersByTenant(tenant.ID); err != nil {
		http.Error(w, `{"error":"failed to delete tenant users: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// Drop tenant databases
	if err := a.db.DropTenantDBs(ctx, slug); err != nil {
		http.Error(w, `{"error":"failed to drop databases: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// Delete tenant KV bucket
	if err := local.DeleteTenantKVBucket(a.js, slug); err != nil {
		// Non-fatal: bucket may not exist
	}

	// Delete tenant record
	if err := a.db.DeleteTenant(context.Background(), tenant.ID); err != nil {
		http.Error(w, `{"error":"failed to delete tenant: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
