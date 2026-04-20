package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/api/middleware"
	"github.com/leandrofars/oktopus/internal/db"
	local "github.com/leandrofars/oktopus/internal/nats"
	"github.com/nats-io/nats.go"
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
	tdb := a.db.ForTenant(slug)

	// 1. Delete firmware files from file server (best-effort, before dropping DB)
	firmwares, err := tdb.ListFirmware(ctx)
	if err == nil {
		authHeader := r.Header.Get("Authorization")
		for _, fw := range firmwares {
			if fw.FileName != "" {
				deleteFileFromUploadService(fw.FileName, authHeader)
			}
		}
	}

	// 2. Delete container images from registry (best-effort)
	// Uses the container-upload service delete endpoint for each image
	go cleanupTenantRegistryImages(slug, r.Header.Get("Authorization"))

	// 3. Delete devices from adapter DB via NATS
	deleteTenantDevices(a.nc, slug)

	// 4. Delete all users belonging to this tenant
	if err := a.db.DeleteUsersByTenant(tenant.ID); err != nil {
		http.Error(w, `{"error":"failed to delete tenant users: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// 5. Drop tenant databases (firmware, scripts, campaigns, messages, metrics, etc.)
	if err := a.db.DropTenantDBs(ctx, slug); err != nil {
		http.Error(w, `{"error":"failed to drop databases: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// 6. Delete tenant KV bucket (device credentials)
	if err := local.DeleteTenantKVBucket(a.js, slug); err != nil {
		// Non-fatal: bucket may not exist
	}

	// 7. Delete tenant record
	if err := a.db.DeleteTenant(context.Background(), tenant.ID); err != nil {
		http.Error(w, `{"error":"failed to delete tenant: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// deleteTenantDevices removes all devices belonging to a tenant from the adapter DB via NATS
func deleteTenantDevices(nc *nats.Conn, tenantSlug string) {
	// The adapter listens on adapter.usp.v1.*.devices.delete
	// Send a request with empty filter to delete all devices for this tenant
	subject := local.NatsAdapterSubject(tenantSlug) + "devices.delete"
	// Encode all device SNs as empty — adapter should delete by tenant filter
	data, _ := json.Marshal(map[string]interface{}{"all": true})
	msg, err := nc.Request(subject, data, 10*time.Second)
	if err != nil {
		log.Printf("Warning: failed to delete adapter devices for tenant %s: %v", tenantSlug, err)
		return
	}
	log.Printf("Deleted adapter devices for tenant %s: %s", tenantSlug, string(msg.Data))
}

// GET /api/tenants/{slug}/device-password
func (a *Api) getDevicePassword(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if db.UserLevels(level) > db.TenantAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	slug := middleware.GetTenantSlug(r)
	kv, err := a.js.KeyValue(a.ctx, "devices-auth-"+slug)
	if err != nil {
		http.Error(w, `{"error":"device auth store not found"}`, http.StatusNotFound)
		return
	}
	entry, err := kv.Get(r.Context(), "__tenant_password__")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"password": ""})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"password": string(entry.Value())})
}

// PUT /api/tenants/{slug}/device-password
func (a *Api) setDevicePassword(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if db.UserLevels(level) > db.TenantAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	slug := middleware.GetTenantSlug(r)
	var body struct {
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Password == "" {
		http.Error(w, `{"error":"password required"}`, http.StatusBadRequest)
		return
	}
	kv, err := a.js.KeyValue(a.ctx, "devices-auth-"+slug)
	if err != nil {
		http.Error(w, `{"error":"device auth store not found"}`, http.StatusNotFound)
		return
	}
	kv.PutString(r.Context(), "__tenant_password__", body.Password)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// cleanupTenantRegistryImages deletes all container images prefixed with the tenant slug
// from the Docker registry. Runs in background (best-effort).
func cleanupTenantRegistryImages(tenantSlug, authHeader string) {
	// Fetch catalog from registry via nginx
	client := &http.Client{Timeout: 30 * time.Second}

	// List all repos from registry
	req, err := http.NewRequest("GET", "http://nginx/docker-registry/v2/_catalog", nil)
	if err != nil {
		log.Printf("Warning: failed to list registry catalog for tenant cleanup: %v", err)
		return
	}
	req.Header.Set("Authorization", authHeader)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Warning: failed to fetch registry catalog: %v", err)
		return
	}
	defer resp.Body.Close()

	var catalog struct {
		Repositories []string `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		log.Printf("Warning: failed to parse registry catalog: %v", err)
		return
	}

	prefix := tenantSlug + "/"
	for _, repo := range catalog.Repositories {
		if !strings.HasPrefix(repo, prefix) {
			continue
		}
		// Get tags for this repo
		tagsReq, _ := http.NewRequest("GET", fmt.Sprintf("http://nginx/docker-registry/v2/%s/tags/list", repo), nil)
		tagsReq.Header.Set("Authorization", authHeader)
		tagsResp, err := client.Do(tagsReq)
		if err != nil {
			continue
		}
		var tagsData struct {
			Tags []string `json:"tags"`
		}
		json.NewDecoder(tagsResp.Body).Decode(&tagsData)
		tagsResp.Body.Close()

		// Delete each tag via container-upload service
		for _, tag := range tagsData.Tags {
			name := strings.TrimPrefix(repo, prefix)
			delReq, _ := http.NewRequest("DELETE",
				fmt.Sprintf("http://container-upload:8005/delete?name=%s&tag=%s",
					url.QueryEscape(name), url.QueryEscape(tag)), nil)
			delReq.Header.Set("Authorization", authHeader)
			delReq.Header.Set("X-Tenant-Slug", tenantSlug)
			delResp, err := client.Do(delReq)
			if err != nil {
				log.Printf("Warning: failed to delete registry image %s:%s: %v", repo, tag, err)
				continue
			}
			delResp.Body.Close()
			log.Printf("Deleted registry image %s:%s for tenant %s", repo, tag, tenantSlug)
		}
	}
}
