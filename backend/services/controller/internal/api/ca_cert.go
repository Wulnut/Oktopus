package api

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/api/middleware"
	"github.com/leandrofars/oktopus/internal/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GET /api/tenants/{slug}/ca-certs
func (a *Api) listCACerts(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]

	tenant, err := a.db.FindTenant(r.Context(), slug)
	if err != nil {
		http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
		return
	}

	certs := tenant.CACerts
	if certs == nil {
		certs = []db.TenantCACert{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(certs)
}

// POST /api/tenants/{slug}/ca-certs (TenantAdmin+)
func (a *Api) addCACert(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if db.UserLevels(level) > db.TenantAdmin {
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
		Label string `json:"label"`
		PEM   string `json:"pem"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if body.PEM == "" {
		http.Error(w, `{"error":"pem is required"}`, http.StatusBadRequest)
		return
	}

	// Parse PEM to extract expiry
	block, _ := pem.Decode([]byte(body.PEM))
	if block == nil {
		http.Error(w, `{"error":"invalid PEM data"}`, http.StatusBadRequest)
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		http.Error(w, `{"error":"failed to parse certificate: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	caCert := db.TenantCACert{
		Label:    body.Label,
		PEM:      body.PEM,
		NotAfter: cert.NotAfter,
	}

	if err := a.db.AddCACert(r.Context(), tenant.ID, caCert); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// DELETE /api/tenants/{slug}/ca-certs/{certId} (TenantAdmin+)
func (a *Api) removeCACert(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if db.UserLevels(level) > db.TenantAdmin {
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

	certID, err := primitive.ObjectIDFromHex(vars["certId"])
	if err != nil {
		http.Error(w, `{"error":"invalid cert ID"}`, http.StatusBadRequest)
		return
	}

	if err := a.db.RemoveCACert(r.Context(), tenant.ID, certID); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
