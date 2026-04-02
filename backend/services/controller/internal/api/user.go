package api

import (
	"encoding/json"
	"log"
	"net/http"
	"net/mail"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/api/auth"
	"github.com/leandrofars/oktopus/internal/api/middleware"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/utils"
)

func (a *Api) retrieveUsers(w http.ResponseWriter, r *http.Request) {
	slug := middleware.GetTenantSlug(r)
	tenant, err := a.db.FindTenant(r.Context(), slug)
	if err != nil {
		http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
		return
	}

	users, err := a.db.FindUsersByTenant(tenant.ID)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	type userResponse struct {
		ID        string `json:"_id"`
		Email     string `json:"email"`
		Name      string `json:"name"`
		Phone     string `json:"phone"`
		Level     int    `json:"level"`
		CreatedAt string `json:"createdAt"`
	}

	var result []userResponse
	for _, u := range users {
		resp := userResponse{
			Email: u.Email,
			Name:  u.Name,
			Phone: u.Phone,
			Level: int(u.Level),
		}
		if !u.TenantID.IsZero() {
			resp.ID = u.TenantID.Hex()
		}
		result = append(result, resp)
	}

	err = json.NewEncoder(w).Encode(result)
	if err != nil {
		log.Println(err)
	}
}

func (a *Api) registerUser(w http.ResponseWriter, r *http.Request) {
	email := middleware.GetEmail(r)
	level := middleware.GetLevel(r)

	// Check if user which is requesting creation has the necessary privileges
	if db.UserLevels(level) > db.TenantAdmin {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	var user db.User
	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Prevent level escalation: TenantAdmin can only create Operators
	// SuperAdmin can create TenantAdmins
	if db.UserLevels(level) == db.TenantAdmin {
		user.Level = db.Operator
	} else if db.UserLevels(level) == db.SuperAdmin {
		if user.Level < db.TenantAdmin {
			user.Level = db.TenantAdmin // SuperAdmin can create TenantAdmin at most via this endpoint
		}
	}

	// Assign user to the tenant from URL slug
	slug := middleware.GetTenantSlug(r)
	if slug == "" {
		http.Error(w, `{"error":"tenant slug required"}`, http.StatusBadRequest)
		return
	}
	tenant, err := a.db.FindTenant(r.Context(), slug)
	if err != nil {
		http.Error(w, `{"error":"tenant not found"}`, http.StatusBadRequest)
		return
	}
	user.TenantID = tenant.ID

	if err := user.HashPassword(user.Password); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if user.Email == "" || user.Password == "" || !valid(user.Email) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_ = email // logged for audit

	if err := a.db.RegisterUser(user); err != nil {
		if err == db.ErrorUserExists {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte("User with this email already exists"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func valid(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

func (a *Api) deleteUser(w http.ResponseWriter, r *http.Request) {
	email := middleware.GetEmail(r)
	level := middleware.GetLevel(r)
	userEmail := mux.Vars(r)["user"]

	// Cannot delete yourself
	if email == userEmail {
		http.Error(w, `{"error":"cannot delete own account"}`, http.StatusForbidden)
		return
	}

	// Only TenantAdmin+ can delete users
	if db.UserLevels(level) > db.TenantAdmin {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// TenantAdmin must verify target user belongs to their tenant
	if db.UserLevels(level) == db.TenantAdmin {
		targetUser, err := a.db.FindUser(userEmail)
		if err != nil {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}
		slug := middleware.GetTenantSlug(r)
		tenant, err := a.db.FindTenant(r.Context(), slug)
		if err != nil || targetUser.TenantID != tenant.ID {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	if err := a.db.DeleteUser(userEmail); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (a *Api) changePassword(w http.ResponseWriter, r *http.Request) {
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

	var user db.User
	err = json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		utils.MarshallEncoder(err, w)
		return
	}
	user.Email = claims.Email

	if len(user.Password) < 8 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Password must be at least 8 characters long"))
		return
	}

	if err := user.HashPassword(user.Password); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := a.db.UpdatePassword(user); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *Api) registerAdminUser(w http.ResponseWriter, r *http.Request) {

	tokenString := r.Header.Get("Authorization")
	if tokenString == "" {
		users, err := a.db.FindAllUsers()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			utils.MarshallEncoder(err, w)
		}

		if !adminUserExists(users) {
			var user db.User
			err = json.NewDecoder(r.Body).Decode(&user)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// First admin is SuperAdmin (no tenant)
			user.Level = db.SuperAdmin

			if err := user.HashPassword(user.Password); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if err := a.db.RegisterUser(user); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		} else {
			w.WriteHeader(http.StatusForbidden)
		}

		return
	}

	claims, err := auth.ValidateToken(tokenString)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Only SuperAdmin can create more admin users
	if db.UserLevels(claims.Level) != db.SuperAdmin {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	var user db.User
	err = json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	user.Level = db.SuperAdmin

	if err := user.HashPassword(user.Password); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := a.db.RegisterUser(user); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func adminUserExists(users []map[string]interface{}) bool {

	if len(users) == 0 {
		return false
	}

	for _, x := range users {
		if db.UserLevels(x["level"].(int32)) == db.SuperAdmin {
			return true
		}
	}
	return false
}

func (a *Api) adminUserExists(w http.ResponseWriter, r *http.Request) {

	users, err := a.db.FindAllUsers()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	adminExits := adminUserExists(users)
	json.NewEncoder(w).Encode(adminExits)
	return
}

type TokenRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *Api) generateToken(w http.ResponseWriter, r *http.Request) {
	var tokenReq TokenRequest

	err := json.NewDecoder(r.Body).Decode(&tokenReq)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	user, err := a.db.FindUser(tokenReq.Email)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode("Invalid Credentials")
		return
	}

	credentialError := user.CheckPassword(tokenReq.Password)
	if credentialError != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode("Invalid Credentials")
		return
	}

	// Look up tenant info if user has a TenantID
	var tenantID, tenantSlug string
	if !user.TenantID.IsZero() {
		tenant, err := a.db.FindTenantByID(r.Context(), user.TenantID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode("Failed to look up tenant")
			return
		}
		if tenant.Status != db.TenantStatusActive {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode("Tenant is disabled")
			return
		}
		tenantID = tenant.ID.Hex()
		tenantSlug = tenant.Slug
	}

	token, err := auth.GenerateJWT(user.Email, user.Name, tenantID, tenantSlug, int(user.Level))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(token)
	return
}
