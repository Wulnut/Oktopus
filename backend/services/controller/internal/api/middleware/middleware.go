package middleware

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/api/auth"
	"golang.org/x/net/context"
)

type contextKey string

const (
	CtxEmail      contextKey = "email"
	CtxTenantID   contextKey = "tenant_id"
	CtxTenantSlug contextKey = "tenant_slug"
	CtxLevel      contextKey = "level"
	CtxClaims     contextKey = "claims"
)

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		tokenString := r.Header.Get("Authorization")
		if tokenString == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		ctx := r.Context()
		ctx = context.WithValue(ctx, CtxEmail, claims.Email)
		ctx = context.WithValue(ctx, CtxTenantID, claims.TenantID)
		ctx = context.WithValue(ctx, CtxTenantSlug, claims.TenantSlug)
		ctx = context.WithValue(ctx, CtxLevel, claims.Level)
		ctx = context.WithValue(ctx, CtxClaims, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireMaxLevel permits only authenticated users whose numeric privilege
// level is at or above the requested role (lower values are more privileged).
// Missing or malformed level claims are denied rather than treated as level 0.
func RequireMaxLevel(maxLevel int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			level, ok := r.Context().Value(CtxLevel).(int)
			if !ok || level > maxLevel {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func TenantMiddleware(findTenant func(slug string) (interface{}, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			slug := mux.Vars(r)["slug"]
			if slug == "" {
				http.Error(w, `{"error":"tenant slug required"}`, http.StatusBadRequest)
				return
			}

			level, _ := r.Context().Value(CtxLevel).(int)
			tenantSlug, _ := r.Context().Value(CtxTenantSlug).(string)

			// Tenant users can only access their own tenant
			if level > 0 && slug != tenantSlug {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}

			// Verify tenant exists and is active
			tenant, err := findTenant(slug)
			if err != nil {
				http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
				return
			}

			ctx := context.WithValue(r.Context(), CtxTenantSlug, slug)
			ctx = context.WithValue(ctx, "tenant", tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetEmail(r *http.Request) string {
	email, _ := r.Context().Value(CtxEmail).(string)
	return email
}

func GetTenantSlug(r *http.Request) string {
	slug, _ := r.Context().Value(CtxTenantSlug).(string)
	return slug
}

func GetLevel(r *http.Request) int {
	level, _ := r.Context().Value(CtxLevel).(int)
	return level
}

func GetTenantID(r *http.Request) string {
	id, _ := r.Context().Value(CtxTenantID).(string)
	return id
}
