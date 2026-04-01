# Multi-Tenancy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add organization-level multi-tenancy to Oktopus so each ISP tenant has isolated data, users, and devices, with Provider (SEI) super-admin access across all tenants.

**Architecture:** Hybrid per-tenant DB isolation. Shared `account-mngr` database holds users and tenants collections. Each tenant gets `tenant_<slug>_general` and `tenant_<slug>_usp` databases. JWT carries tenant context. URL path prefix `/api/tenants/:slug/` scopes all data endpoints. Middleware resolves tenant and injects TenantDB into request context.

**Tech Stack:** Go (Gorilla Mux, mongo-driver, golang-jwt, nats.go), MongoDB, NATS JetStream, Next.js 15, React 19, Material UI 6

**Spec:** `docs/superpowers/specs/2026-04-01-multi-tenancy-design.md`

**Scope:** This plan covers backend core (DB layer, entities, auth, middleware, tenant CRUD, route restructuring, tenant-scoped device auth) and frontend tenant integration. MTP transport layer tenant routing and TLS cert-based device auth are deferred to a follow-up plan.

---

## File Structure

### New files

| File | Responsibility |
|------|---------------|
| `backend/services/controller/internal/db/tenant.go` | Tenant CRUD operations, TenantDB struct, per-tenant DB provisioning |
| `backend/services/controller/internal/api/tenant.go` | Tenant management API handlers (SuperAdmin only) |
| `backend/services/controller/internal/api/ca_cert.go` | CA cert management API handlers |
| `frontend/src/pages/tenants/index.js` | Tenant management page (SuperAdmin) |
| `frontend/src/pages/tenants/[slug].js` | Tenant detail/edit page |
| `frontend/src/sections/tenants/tenants-table.js` | Tenant list table component |
| `frontend/src/sections/tenants/tenant-form.js` | Tenant create/edit form |
| `frontend/src/contexts/tenant-context.js` | Tenant context provider (slug, name, API prefix) |

### Modified files

| File | Changes |
|------|---------|
| `backend/services/controller/internal/db/db.go` | Split into shared DB + TenantDB, add ForTenant(), add ProvisionTenantDBs() |
| `backend/services/controller/internal/db/user.go` | Add TenantID field, update UserLevels, tenant-scoped queries |
| `backend/services/controller/internal/entity/device.go` | Replace Customer with TenantID |
| `backend/services/controller/internal/api/auth/auth.go` | Add TenantID/TenantSlug/Level to JWT claims |
| `backend/services/controller/internal/api/middleware/middleware.go` | Extract full JWT claims, add tenant resolution middleware |
| `backend/services/controller/internal/api/api.go` | Restructure routes under /api/tenants/:slug/, pass TenantDB to handlers |
| `backend/services/controller/internal/api/user.go` | Tenant-scoped user CRUD, SuperAdmin/TenantAdmin/Operator roles |
| `backend/services/controller/internal/api/device.go` | Use TenantDB, tenant-scoped device auth KV bucket |
| `backend/services/controller/internal/api/firmware.go` | Use TenantDB from context |
| `backend/services/controller/internal/api/scripts.go` | Use TenantDB from context |
| `backend/services/controller/internal/api/mass_actions.go` | Use TenantDB from context |
| `backend/services/controller/internal/api/deviceinfo.go` | Use TenantDB from context |
| `backend/services/controller/internal/api/info.go` | Use TenantDB from context |
| `backend/services/controller/internal/api/history.go` | Use TenantDB from context |
| `backend/services/controller/internal/nats/nats.go` | Remove global KV bucket, add tenant-scoped KV helpers |
| `backend/services/controller/cmd/controller/main.go` | Pass restructured DB to api |
| `frontend/src/contexts/auth-context.js` | Store tenantSlug/tenantId/level from JWT, expose in user object |
| `frontend/src/layouts/dashboard/config.js` | Conditional nav items by role, add Tenants nav for SuperAdmin |
| `frontend/src/pages/companies.js` | Remove (replaced by tenants page) |

---

## Task 1: Tenant Entity and DB Layer

**Files:**
- Create: `backend/services/controller/internal/db/tenant.go`
- Modify: `backend/services/controller/internal/db/db.go`

- [ ] **Step 1: Create tenant.go with Tenant structs**

Create `backend/services/controller/internal/db/tenant.go`:

```go
package db

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type TenantCACert struct {
	ID       primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Label    string             `bson:"label" json:"label"`
	PEM      string             `bson:"pem" json:"pem"`
	NotAfter time.Time          `bson:"not_after" json:"not_after"`
	AddedAt  time.Time          `bson:"added_at" json:"added_at"`
}

type TenantAuthPolicy struct {
	PasswordRequired bool `bson:"password_required" json:"password_required"`
	CertRequired     bool `bson:"cert_required" json:"cert_required"`
}

type Tenant struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name       string             `bson:"name" json:"name"`
	Slug       string             `bson:"slug" json:"slug"`
	Status     string             `bson:"status" json:"status"`
	AuthPolicy TenantAuthPolicy   `bson:"auth_policy" json:"auth_policy"`
	CACerts    []TenantCACert     `bson:"ca_certs,omitempty" json:"ca_certs,omitempty"`
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time          `bson:"updated_at" json:"updated_at"`
}

// TenantDB holds references to a specific tenant's databases.
// Handlers receive this from request context.
type TenantDB struct {
	General *mongo.Database
	Usp     *mongo.Database
}
```

- [ ] **Step 2: Add tenant CRUD operations to tenant.go**

Append to `backend/services/controller/internal/db/tenant.go`:

```go
func (d *Database) CreateTenant(tenant Tenant) error {
	tenant.CreatedAt = time.Now()
	tenant.UpdatedAt = time.Now()
	if tenant.Status == "" {
		tenant.Status = "active"
	}
	if !tenant.AuthPolicy.PasswordRequired && !tenant.AuthPolicy.CertRequired {
		tenant.AuthPolicy.PasswordRequired = true
	}
	_, err := d.tenants.InsertOne(d.ctx, tenant)
	return err
}

func (d *Database) FindTenant(slug string) (Tenant, error) {
	var tenant Tenant
	err := d.tenants.FindOne(d.ctx, bson.M{"slug": slug}).Decode(&tenant)
	return tenant, err
}

func (d *Database) FindAllTenants() ([]Tenant, error) {
	cursor, err := d.tenants.Find(d.ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var tenants []Tenant
	err = cursor.All(d.ctx, &tenants)
	return tenants, err
}

func (d *Database) UpdateTenant(slug string, update bson.M) error {
	update["updated_at"] = time.Now()
	_, err := d.tenants.UpdateOne(d.ctx, bson.M{"slug": slug}, bson.M{"$set": update})
	return err
}

func (d *Database) DeleteTenant(slug string) error {
	_, err := d.tenants.DeleteOne(d.ctx, bson.M{"slug": slug})
	return err
}

func (d *Database) AddCACert(slug string, cert TenantCACert) error {
	cert.ID = primitive.NewObjectID()
	cert.AddedAt = time.Now()
	_, err := d.tenants.UpdateOne(
		d.ctx,
		bson.M{"slug": slug},
		bson.M{"$push": bson.M{"ca_certs": cert}, "$set": bson.M{"updated_at": time.Now()}},
	)
	return err
}

func (d *Database) RemoveCACert(slug string, certID primitive.ObjectID) error {
	_, err := d.tenants.UpdateOne(
		d.ctx,
		bson.M{"slug": slug},
		bson.M{
			"$pull": bson.M{"ca_certs": bson.M{"_id": certID}},
			"$set":  bson.M{"updated_at": time.Now()},
		},
	)
	return err
}
```

- [ ] **Step 3: Add ForTenant() and ProvisionTenantDBs() to tenant.go**

Append to `backend/services/controller/internal/db/tenant.go`:

```go
// ForTenant returns a TenantDB for the given tenant slug.
func (d *Database) ForTenant(slug string) *TenantDB {
	return &TenantDB{
		General: d.client.Database("tenant_" + slug + "_general"),
		Usp:     d.client.Database("tenant_" + slug + "_usp"),
	}
}

// ProvisionTenantDBs creates collections and indexes for a new tenant.
// Mirrors the index setup in NewDatabase() for general and usp databases.
func (d *Database) ProvisionTenantDBs(slug string) error {
	tdb := d.ForTenant(slug)

	// general collections
	templateCol := tdb.General.Collection("templates")
	templateCol.Indexes().CreateOne(d.ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	firmwareCol := tdb.General.Collection("firmware")
	firmwareCol.Indexes().CreateOne(d.ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	scriptsCol := tdb.General.Collection("scripts")
	scriptsCol.Indexes().CreateOne(d.ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	scriptExecCol := tdb.General.Collection("script_executions")
	scriptExecCol.Indexes().CreateMany(d.ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "created_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(2592000)},
		{Keys: bson.D{{Key: "script_id", Value: 1}}},
		{Keys: bson.D{{Key: "device_sn", Value: 1}}},
	})

	massActionsCol := tdb.General.Collection("mass_actions")
	massActionsCol.Indexes().CreateMany(d.ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "created_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(7776000)},
		{Keys: bson.D{{Key: "status", Value: 1}}},
	})

	deviceInfoCol := tdb.General.Collection("device_info")
	deviceInfoCol.Indexes().CreateOne(d.ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "device_sn", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	campaignsCol := tdb.General.Collection("campaigns")
	campaignsCol.Indexes().CreateOne(d.ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "vendor", Value: 1}, {Key: "model", Value: 1}, {Key: "hw_version", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	fwPoliciesCol := tdb.General.Collection("fw_policies")
	fwPoliciesCol.Indexes().CreateOne(d.ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "device_sn", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	upgradeLogsCol := tdb.General.Collection("upgrade_logs")
	upgradeLogsCol.Indexes().CreateMany(d.ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "device_sn", Value: 1}}},
		{Keys: bson.D{{Key: "campaign_id", Value: 1}}},
		{Keys: bson.D{{Key: "created_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(7776000)},
	})

	// usp collections
	messagesCol := tdb.Usp.Collection("messages")
	messagesCol.Indexes().CreateMany(d.ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "device_serial", Value: 1}}},
		{Keys: bson.D{{Key: "timestamp", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(7776000)},
	})

	tdb.Usp.Collection("messages_errors")

	metricsCol := tdb.Usp.Collection("device_metrics")
	metricsCol.Indexes().CreateMany(d.ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "timestamp", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(604800)},
		{Keys: bson.D{{Key: "device_serial", Value: 1}}},
	})

	return nil
}

// DropTenantDBs removes all databases for a tenant.
func (d *Database) DropTenantDBs(slug string) error {
	err := d.client.Database("tenant_" + slug + "_general").Drop(d.ctx)
	if err != nil {
		return err
	}
	return d.client.Database("tenant_" + slug + "_usp").Drop(d.ctx)
}
```

- [ ] **Step 4: Modify db.go — restructure Database struct**

In `backend/services/controller/internal/db/db.go`, replace the `Database` struct (lines 12-28) with:

```go
type Database struct {
	client  *mongo.Client
	users   *mongo.Collection
	tenants *mongo.Collection
	ctx     context.Context
}
```

Remove all the `general` and `usp` collection fields (template, firmware, messages, messagesErrors, metrics, scripts, scriptExecutions, massActions, deviceInfo, campaigns, fwPolicies, upgradeLogs). These now live in TenantDB, accessed via collection helper methods.

- [ ] **Step 5: Modify NewDatabase() — only init shared collections**

In `backend/services/controller/internal/db/db.go`, rewrite `NewDatabase()` to only initialize the `account-mngr` database with `users` and `tenants` collections:

```go
func NewDatabase(ctx context.Context, mongoUri string) Database {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoUri))
	if err != nil {
		log.Fatal(err)
	}

	accountDb := client.Database("account-mngr")

	usersCol := accountDb.Collection("users")
	usersCol.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	tenantsCol := accountDb.Collection("tenants")
	tenantsCol.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "slug", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	return Database{
		client:  client,
		users:   usersCol,
		tenants: tenantsCol,
		ctx:     ctx,
	}
}
```

- [ ] **Step 6: Add TenantDB collection accessor methods**

Append to `backend/services/controller/internal/db/tenant.go`. These provide typed access to tenant-scoped collections, mirroring what `db.go` used to hold as struct fields:

```go
func (t *TenantDB) Templates() *mongo.Collection    { return t.General.Collection("templates") }
func (t *TenantDB) Firmware() *mongo.Collection      { return t.General.Collection("firmware") }
func (t *TenantDB) Scripts() *mongo.Collection        { return t.General.Collection("scripts") }
func (t *TenantDB) ScriptExecs() *mongo.Collection    { return t.General.Collection("script_executions") }
func (t *TenantDB) MassActions() *mongo.Collection     { return t.General.Collection("mass_actions") }
func (t *TenantDB) DeviceInfo() *mongo.Collection      { return t.General.Collection("device_info") }
func (t *TenantDB) Campaigns() *mongo.Collection       { return t.General.Collection("campaigns") }
func (t *TenantDB) FWPolicies() *mongo.Collection      { return t.General.Collection("fw_policies") }
func (t *TenantDB) UpgradeLogs() *mongo.Collection     { return t.General.Collection("upgrade_logs") }
func (t *TenantDB) Messages() *mongo.Collection        { return t.Usp.Collection("messages") }
func (t *TenantDB) MessagesErrors() *mongo.Collection  { return t.Usp.Collection("messages_errors") }
func (t *TenantDB) Metrics() *mongo.Collection          { return t.Usp.Collection("device_metrics") }
```

- [ ] **Step 7: Update all existing db/*.go files to use TenantDB**

Each existing db file that operates on `general` or `usp` collections needs its methods changed from `(d *Database)` receiver to `(t *TenantDB)` receiver, and collection references updated from `d.firmware` to `t.Firmware()`, etc.

Files to update (receiver change + collection reference):
- `db/firmware.go`: `(d *Database)` -> `(t *TenantDB)`, `d.firmware` -> `t.Firmware()`
- `db/template.go`: `(d *Database)` -> `(t *TenantDB)`, `d.template` -> `t.Templates()`
- `db/message.go`: `(d *Database)` -> `(t *TenantDB)`, `d.messages` -> `t.Messages()`, `d.messagesErrors` -> `t.MessagesErrors()`
- `db/metrics.go`: `(d *Database)` -> `(t *TenantDB)`, `d.metrics` -> `t.Metrics()`
- `db/scripts.go`: `(d *Database)` -> `(t *TenantDB)`, `d.scripts` -> `t.Scripts()`, `d.scriptExecutions` -> `t.ScriptExecs()`
- `db/mass_actions.go`: `(d *Database)` -> `(t *TenantDB)`, `d.massActions` -> `t.MassActions()`
- `db/device_info.go`: `(d *Database)` -> `(t *TenantDB)`, `d.deviceInfo` -> `t.DeviceInfo()`
- `db/campaigns.go`: `(d *Database)` -> `(t *TenantDB)`, `d.campaigns` -> `t.Campaigns()`
- `db/fw_policy.go`: `(d *Database)` -> `(t *TenantDB)`, `d.fwPolicies` -> `t.FWPolicies()`
- `db/upgrade_log.go`: `(d *Database)` -> `(t *TenantDB)`, `d.upgradeLogs` -> `t.UpgradeLogs()`

User operations (`db/user.go`) stay on `(d *Database)` since users are in the shared DB.

- [ ] **Step 8: Verify Go compilation**

Run: `sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build controller 2>&1"`

Expected: Build succeeds (there will be compile errors in api/ files that reference old `d.db.SomeMethod()` — these are fixed in later tasks)

- [ ] **Step 9: Commit**

```bash
git add backend/services/controller/internal/db/
git commit -m "feat: restructure DB layer for multi-tenancy

Split Database into shared (account-mngr) and TenantDB (per-tenant general/usp).
Add Tenant entity, CRUD operations, ForTenant(), ProvisionTenantDBs().
Move collection accessors to TenantDB methods."
```

---

## Task 2: User Model and Auth Changes

**Files:**
- Modify: `backend/services/controller/internal/db/user.go`
- Modify: `backend/services/controller/internal/api/auth/auth.go`
- Modify: `backend/services/controller/internal/entity/device.go`

- [ ] **Step 1: Update UserLevels and User struct**

In `backend/services/controller/internal/db/user.go`, replace the UserLevels and User definitions (lines 12-25):

```go
type UserLevels int32

const (
	SuperAdmin  UserLevels = iota // 0 — Provider (SEI)
	TenantAdmin                   // 1 — ISP admin
	Operator                      // 2 — ISP operator
)

type User struct {
	Email    string             `json:"email"`
	Name     string             `json:"name"`
	Password string             `json:"password,omitempty"`
	Level    UserLevels         `json:"level"`
	Phone    string             `json:"phone"`
	TenantID primitive.ObjectID `json:"tenant_id,omitempty" bson:"tenant_id,omitempty"`
}
```

Add `"go.mongodb.org/mongo-driver/bson/primitive"` to imports.

- [ ] **Step 2: Update user query methods for tenant scoping**

In `backend/services/controller/internal/db/user.go`, update `FindAllUsers()` to accept an optional tenant filter:

```go
func (d *Database) FindAllUsers() ([]User, error) {
	cursor, err := d.users.Find(d.ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var users []User
	err = cursor.All(d.ctx, &users)
	return users, err
}

func (d *Database) FindUsersByTenant(tenantID primitive.ObjectID) ([]User, error) {
	cursor, err := d.users.Find(d.ctx, bson.M{"tenant_id": tenantID})
	if err != nil {
		return nil, err
	}
	var users []User
	err = cursor.All(d.ctx, &users)
	return users, err
}

func (d *Database) DeleteUsersByTenant(tenantID primitive.ObjectID) error {
	_, err := d.users.DeleteMany(d.ctx, bson.M{"tenant_id": tenantID})
	return err
}
```

- [ ] **Step 3: Update JWT claims**

In `backend/services/controller/internal/api/auth/auth.go`, replace the JWTClaim struct (lines 21-25):

```go
type JWTClaim struct {
	Username   string `json:"username"`
	Email      string `json:"email"`
	TenantID   string `json:"tenant_id"`
	TenantSlug string `json:"tenant_slug"`
	Level      int    `json:"level"`
	jwt.RegisteredClaims
}
```

- [ ] **Step 4: Update GenerateJWT to accept tenant context**

In `backend/services/controller/internal/api/auth/auth.go`, replace `GenerateJWT()` (lines 27-40):

```go
func GenerateJWT(email, username, tenantID, tenantSlug string, level int) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &JWTClaim{
		Username:   username,
		Email:      email,
		TenantID:   tenantID,
		TenantSlug: tenantSlug,
		Level:      level,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			Issuer:    "Oktopus",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getJwtKey())
}
```

- [ ] **Step 5: Update ValidateToken to return full claims**

In `backend/services/controller/internal/api/auth/auth.go`, replace `ValidateToken()` (lines 42-70):

```go
func ValidateToken(signedToken string) (*JWTClaim, error) {
	token, err := jwt.ParseWithClaims(
		signedToken,
		&JWTClaim{},
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return getJwtKey(), nil
		},
	)
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*JWTClaim)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}
```

Add `"fmt"` to imports.

- [ ] **Step 6: Update Device entity**

In `backend/services/controller/internal/entity/device.go`, replace `Customer` with `TenantID`:

```go
type Device struct {
	SN           string
	Model        string
	TenantID     string
	Vendor       string
	Version      string
	ProductClass string
	HWVersion    string
	Alias        string
	Status       Status
	Mqtt         Status
	Stomp        Status
	Websockets   Status
	Cwmp         Status
}
```

- [ ] **Step 7: Commit**

```bash
git add backend/services/controller/internal/db/user.go \
        backend/services/controller/internal/api/auth/auth.go \
        backend/services/controller/internal/entity/device.go
git commit -m "feat: add tenant context to User model, JWT claims, and Device entity

UserLevels: SuperAdmin(0), TenantAdmin(1), Operator(2).
User gains TenantID field. JWT carries tenant_id, tenant_slug, level.
Device.Customer replaced with Device.TenantID."
```

---

## Task 3: Middleware — Auth and Tenant Resolution

**Files:**
- Modify: `backend/services/controller/internal/api/middleware/middleware.go`

- [ ] **Step 1: Define context keys**

In `backend/services/controller/internal/api/middleware/middleware.go`, add typed context keys at the top of the file (after imports):

```go
type contextKey string

const (
	CtxEmail      contextKey = "email"
	CtxTenantID   contextKey = "tenant_id"
	CtxTenantSlug contextKey = "tenant_slug"
	CtxLevel      contextKey = "level"
	CtxClaims     contextKey = "claims"
)
```

- [ ] **Step 2: Update auth middleware to extract full claims**

Replace the existing `Middleware` function:

```go
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
```

- [ ] **Step 3: Add tenant resolution middleware**

Append to `backend/services/controller/internal/api/middleware/middleware.go`:

```go
// TenantMiddleware validates that the :slug in the URL path matches the user's
// tenant (for tenant users) or resolves the target tenant (for SuperAdmin).
// It also checks tenant status.
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
```

Add `"github.com/gorilla/mux"` and `"net/http"` to imports.

- [ ] **Step 4: Add helper functions for extracting context values**

Append to `backend/services/controller/internal/api/middleware/middleware.go`:

```go
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
```

- [ ] **Step 5: Update all handler files that read email from context**

Every handler that does `r.Context().Value("email").(string)` must change to `middleware.GetEmail(r)`. Search for this pattern in all api/*.go files and update. Files affected:
- `api/user.go`
- `api/device.go`
- `api/firmware.go` (if present)
- `api/scripts.go` (if present)
- Any other handler that reads email from context

- [ ] **Step 6: Commit**

```bash
git add backend/services/controller/internal/api/middleware/middleware.go \
        backend/services/controller/internal/api/
git commit -m "feat: add tenant-aware auth and resolution middleware

AuthMiddleware extracts full JWT claims (email, tenantID, tenantSlug, level).
TenantMiddleware validates URL slug matches user's tenant.
Add context helper functions for handlers."
```

---

## Task 4: Route Restructuring

**Files:**
- Modify: `backend/services/controller/internal/api/api.go`
- Create: `backend/services/controller/internal/api/tenant.go`

- [ ] **Step 1: Update Api struct**

In `backend/services/controller/internal/api/api.go`, the Api struct needs to hold the restructured DB. Since NATS KV is now tenant-scoped, remove the single `kv` field:

```go
type Api struct {
	port   string
	js     jetstream.JetStream
	nc     *nats.Conn
	bridge bridge.Bridge
	db     db.Database
	ctx    context.Context
}
```

- [ ] **Step 2: Create tenant.go with tenant management handlers**

Create `backend/services/controller/internal/api/tenant.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/api/middleware"
	"github.com/leandrofars/oktopus/internal/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var slugRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func generateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(slug, "")
	slug = regexp.MustCompile(`-+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

func (a *Api) createTenant(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if level != int(db.SuperAdmin) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	var req struct {
		Name  string `json:"name"`
		Email string `json:"admin_email"`
		Pass  string `json:"admin_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Email == "" || req.Pass == "" {
		http.Error(w, `{"error":"name, admin_email, admin_password required"}`, http.StatusBadRequest)
		return
	}

	slug := generateSlug(req.Name)
	if !slugRegex.MatchString(slug) {
		http.Error(w, `{"error":"invalid tenant name"}`, http.StatusBadRequest)
		return
	}

	tenant := db.Tenant{
		Name:   req.Name,
		Slug:   slug,
		Status: "active",
		AuthPolicy: db.TenantAuthPolicy{
			PasswordRequired: true,
			CertRequired:     false,
		},
	}

	if err := a.db.CreateTenant(tenant); err != nil {
		http.Error(w, `{"error":"tenant already exists or DB error"}`, http.StatusConflict)
		return
	}

	if err := a.db.ProvisionTenantDBs(slug); err != nil {
		http.Error(w, `{"error":"failed to provision tenant databases"}`, http.StatusInternalServerError)
		return
	}

	// Look up the created tenant to get its ID
	created, err := a.db.FindTenant(slug)
	if err != nil {
		http.Error(w, `{"error":"tenant created but lookup failed"}`, http.StatusInternalServerError)
		return
	}

	// Create initial TenantAdmin user
	adminUser := db.User{
		Email:    req.Email,
		Name:     req.Email,
		Password: req.Pass,
		Level:    db.TenantAdmin,
		TenantID: created.ID,
	}
	adminUser.HashPassword()
	if err := a.db.RegisterUser(adminUser); err != nil {
		http.Error(w, `{"error":"tenant created but admin user registration failed"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (a *Api) listTenants(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if level != int(db.SuperAdmin) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	tenants, err := a.db.FindAllTenants()
	if err != nil {
		http.Error(w, `{"error":"failed to list tenants"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(tenants)
}

func (a *Api) getTenant(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	tenant, err := a.db.FindTenant(slug)
	if err != nil {
		http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(tenant)
}

func (a *Api) updateTenant(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if level != int(db.SuperAdmin) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	slug := mux.Vars(r)["slug"]
	var update map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	// Only allow updating name, status, auth_policy
	allowed := map[string]bool{"name": true, "status": true, "auth_policy": true}
	for k := range update {
		if !allowed[k] {
			delete(update, k)
		}
	}
	if err := a.db.UpdateTenant(slug, update); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *Api) deleteTenant(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if level != int(db.SuperAdmin) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	slug := mux.Vars(r)["slug"]
	tenant, err := a.db.FindTenant(slug)
	if err != nil {
		http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
		return
	}

	// Delete all tenant users
	a.db.DeleteUsersByTenant(tenant.ID)
	// Drop tenant databases
	a.db.DropTenantDBs(slug)
	// Delete tenant record
	a.db.DeleteTenant(slug)

	w.WriteHeader(http.StatusOK)
}
```

- [ ] **Step 3: Create ca_cert.go with CA cert management handlers**

Create `backend/services/controller/internal/api/ca_cert.go`:

```go
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

func (a *Api) listCACerts(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	tenant, err := a.db.FindTenant(slug)
	if err != nil {
		http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(tenant.CACerts)
}

func (a *Api) addCACert(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if level > int(db.TenantAdmin) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	slug := mux.Vars(r)["slug"]
	var req struct {
		Label string `json:"label"`
		PEM   string `json:"pem"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	// Parse PEM to extract expiry
	block, _ := pem.Decode([]byte(req.PEM))
	if block == nil {
		http.Error(w, `{"error":"invalid PEM data"}`, http.StatusBadRequest)
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		http.Error(w, `{"error":"invalid certificate"}`, http.StatusBadRequest)
		return
	}

	caCert := db.TenantCACert{
		Label:    req.Label,
		PEM:      req.PEM,
		NotAfter: cert.NotAfter,
	}

	if err := a.db.AddCACert(slug, caCert); err != nil {
		http.Error(w, `{"error":"failed to add CA cert"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *Api) removeCACert(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	if level > int(db.TenantAdmin) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	slug := mux.Vars(r)["slug"]
	certIDStr := mux.Vars(r)["certId"]
	certID, err := primitive.ObjectIDFromHex(certIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid cert ID"}`, http.StatusBadRequest)
		return
	}

	if err := a.db.RemoveCACert(slug, certID); err != nil {
		http.Error(w, `{"error":"failed to remove CA cert"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
```

- [ ] **Step 4: Restructure routes in api.go**

Rewrite the route registration in `backend/services/controller/internal/api/api.go`. The key change: all data endpoints move under `/api/tenants/{slug}/`. Auth endpoints stay at `/api/auth/`.

```go
func (a *Api) StartApi() {
	router := mux.NewRouter()
	router.Use(mux.CORSMethodMiddleware(router))

	// --- Auth routes (no middleware) ---
	authRouter := router.PathPrefix("/api/auth").Subrouter()
	authRouter.HandleFunc("/login", a.generateToken).Methods(http.MethodPut, http.MethodOptions)
	authRouter.HandleFunc("/register", a.registerUser).Methods(http.MethodPost, http.MethodOptions)
	authRouter.HandleFunc("/admin/register", a.registerAdminUser).Methods(http.MethodPost, http.MethodOptions)
	authRouter.HandleFunc("/admin/exists", a.adminExists).Methods(http.MethodGet, http.MethodOptions)

	// --- Tenant management routes (auth middleware, SuperAdmin only) ---
	tenantMgmtRouter := router.PathPrefix("/api/tenants").Subrouter()
	tenantMgmtRouter.Use(middleware.AuthMiddleware)
	tenantMgmtRouter.HandleFunc("", a.listTenants).Methods(http.MethodGet, http.MethodOptions)
	tenantMgmtRouter.HandleFunc("", a.createTenant).Methods(http.MethodPost, http.MethodOptions)

	// --- Tenant-scoped routes (auth + tenant resolution middleware) ---
	tenantRouter := router.PathPrefix("/api/tenants/{slug}").Subrouter()
	tenantRouter.Use(middleware.AuthMiddleware)
	tenantRouter.Use(middleware.TenantMiddleware(func(slug string) (interface{}, error) {
		return a.db.FindTenant(slug)
	}))

	// Tenant CRUD (on the tenant itself)
	tenantRouter.HandleFunc("", a.getTenant).Methods(http.MethodGet, http.MethodOptions)
	tenantRouter.HandleFunc("", a.updateTenant).Methods(http.MethodPut, http.MethodOptions)
	tenantRouter.HandleFunc("", a.deleteTenant).Methods(http.MethodDelete, http.MethodOptions)

	// CA certs
	tenantRouter.HandleFunc("/ca-certs", a.listCACerts).Methods(http.MethodGet, http.MethodOptions)
	tenantRouter.HandleFunc("/ca-certs", a.addCACert).Methods(http.MethodPost, http.MethodOptions)
	tenantRouter.HandleFunc("/ca-certs/{certId}", a.removeCACert).Methods(http.MethodDelete, http.MethodOptions)

	// Users (tenant-scoped)
	tenantRouter.HandleFunc("/users", a.retrieveUsers).Methods(http.MethodGet, http.MethodOptions)
	tenantRouter.HandleFunc("/users", a.registerUser).Methods(http.MethodPost, http.MethodOptions)
	tenantRouter.HandleFunc("/users/{email}", a.deleteUser).Methods(http.MethodDelete, http.MethodOptions)

	// Devices
	tenantRouter.HandleFunc("/devices", a.retrieveDevices).Methods(http.MethodGet, http.MethodDelete, http.MethodOptions)
	tenantRouter.HandleFunc("/devices/alias", a.setDeviceAlias).Methods(http.MethodPut, http.MethodOptions)
	// ... register all existing device sub-routes under tenantRouter

	// Device auth (credentials)
	tenantRouter.HandleFunc("/device/auth", a.deviceAuth).Methods(http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions)

	// Firmware
	tenantRouter.HandleFunc("/firmware", a.retrieveFirmwares).Methods(http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions)
	// ... register all firmware sub-routes

	// Scripts
	tenantRouter.HandleFunc("/scripts", a.retrieveScripts).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)
	// ... register all script sub-routes

	// Campaigns
	tenantRouter.HandleFunc("/campaigns", a.retrieveCampaigns).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)
	// ... register all campaign sub-routes

	// Mass actions
	tenantRouter.HandleFunc("/mass-actions", a.retrieveMassActions).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)
	// ... register all mass action sub-routes

	// Info (dashboard)
	tenantRouter.HandleFunc("/info/devices", a.infoDevices).Methods(http.MethodGet, http.MethodOptions)
	// ... register all info sub-routes

	// Device detail routes (USP/CWMP operations)
	// ... register all device detail sub-routes under tenantRouter

	log.Println("Starting API on port", a.port)
	log.Fatal(http.ListenAndServe(":"+a.port, router))
}
```

Note: The exact sub-routes for each handler group should mirror what currently exists in api.go, just moved under the `tenantRouter` prefix. Read the current api.go carefully and register every existing route under the tenant router.

- [ ] **Step 5: Add TenantDB helper for handlers**

Add a helper in `api.go` or a shared utils file that handlers call to get TenantDB from context:

```go
func (a *Api) tenantDB(r *http.Request) *db.TenantDB {
	slug := middleware.GetTenantSlug(r)
	return a.db.ForTenant(slug)
}
```

- [ ] **Step 6: Commit**

```bash
git add backend/services/controller/internal/api/
git commit -m "feat: restructure API routes under /api/tenants/{slug}/

Add tenant management endpoints (CRUD, CA certs).
Move all data endpoints under tenant-scoped URL prefix.
Add tenantDB() helper for handlers."
```

---

## Task 5: Update All Handlers to Use TenantDB

**Files:**
- Modify: `backend/services/controller/internal/api/device.go`
- Modify: `backend/services/controller/internal/api/firmware.go`
- Modify: `backend/services/controller/internal/api/scripts.go`
- Modify: `backend/services/controller/internal/api/mass_actions.go`
- Modify: `backend/services/controller/internal/api/deviceinfo.go`
- Modify: `backend/services/controller/internal/api/info.go`
- Modify: `backend/services/controller/internal/api/history.go`
- Modify: `backend/services/controller/internal/api/user.go`

- [ ] **Step 1: Update device.go — use TenantDB and tenant-scoped KV**

In every handler in `device.go` that accesses data collections, replace `a.db.SomeMethod(...)` with `a.tenantDB(r).SomeMethod(...)`.

For device auth (KV bucket), the handler needs to use a tenant-scoped bucket name. Replace `a.kv` usage with a helper that gets the right bucket:

```go
func (a *Api) tenantKV(r *http.Request) (jetstream.KeyValue, error) {
	slug := middleware.GetTenantSlug(r)
	return a.js.KeyValue(a.ctx, "devices-auth-"+slug)
}
```

In `deviceAuth()`, replace all `a.kv.Get(...)`, `a.kv.PutString(...)`, `a.kv.Purge(...)` calls with:
```go
kv, err := a.tenantKV(r)
if err != nil { ... }
kv.Get(...) // etc.
```

- [ ] **Step 2: Update firmware.go**

Replace all `a.db.SomeFirmwareMethod(...)` calls with `a.tenantDB(r).SomeFirmwareMethod(...)`.

- [ ] **Step 3: Update scripts.go**

Replace all `a.db.SomeScriptMethod(...)` calls with `a.tenantDB(r).SomeScriptMethod(...)`.

- [ ] **Step 4: Update mass_actions.go**

Replace all `a.db.SomeMassActionMethod(...)` calls with `a.tenantDB(r).SomeMassActionMethod(...)`.

- [ ] **Step 5: Update deviceinfo.go**

Replace all `a.db.SomeDeviceInfoMethod(...)` calls with `a.tenantDB(r).SomeDeviceInfoMethod(...)`.

- [ ] **Step 6: Update info.go**

Dashboard info handlers need TenantDB for device/firmware counts. Replace `a.db.` calls with `a.tenantDB(r).` calls.

- [ ] **Step 7: Update history.go**

Replace all `a.db.SomeMessageMethod(...)` calls with `a.tenantDB(r).SomeMessageMethod(...)`.

- [ ] **Step 8: Update user.go — tenant-scoped user management**

Rewrite `retrieveUsers()` to list users for the current tenant (from URL slug):

```go
func (a *Api) retrieveUsers(w http.ResponseWriter, r *http.Request) {
	level := middleware.GetLevel(r)
	slug := middleware.GetTenantSlug(r)

	// SuperAdmin listing all users across system (no slug context)
	// or listing users within a tenant
	tenant, err := a.db.FindTenant(slug)
	if err != nil {
		http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
		return
	}

	if level > int(db.TenantAdmin) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	users, err := a.db.FindUsersByTenant(tenant.ID)
	if err != nil {
		http.Error(w, `{"error":"failed to list users"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(users)
}
```

Update `registerUser()` to assign the new user to the current tenant:

```go
// Inside registerUser(), after validating the request:
slug := middleware.GetTenantSlug(r)
tenant, _ := a.db.FindTenant(slug)
newUser.TenantID = tenant.ID
newUser.Level = db.Operator // default level for new users
```

Update `generateToken()` (login) to include tenant context in JWT:

```go
func (a *Api) generateToken(w http.ResponseWriter, r *http.Request) {
	// ... existing email/password validation ...
	user, err := a.db.FindUser(credentials.Email)
	// ... existing password check ...

	tenantID := ""
	tenantSlug := ""
	if !user.TenantID.IsZero() {
		tenant, err := a.db.FindTenantByID(user.TenantID)
		if err != nil {
			http.Error(w, `{"error":"user tenant not found"}`, http.StatusInternalServerError)
			return
		}
		tenantID = tenant.ID.Hex()
		tenantSlug = tenant.Slug
		if tenant.Status != "active" {
			http.Error(w, `{"error":"tenant suspended"}`, http.StatusForbidden)
			return
		}
	}

	token, err := auth.GenerateJWT(user.Email, user.Name, tenantID, tenantSlug, int(user.Level))
	// ... return token ...
}
```

- [ ] **Step 9: Add FindTenantByID to db/tenant.go**

```go
func (d *Database) FindTenantByID(id primitive.ObjectID) (Tenant, error) {
	var tenant Tenant
	err := d.tenants.FindOne(d.ctx, bson.M{"_id": id}).Decode(&tenant)
	return tenant, err
}
```

- [ ] **Step 10: Verify Go compilation**

Run: `sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build controller 2>&1"`

Expected: Build succeeds

- [ ] **Step 11: Commit**

```bash
git add backend/services/controller/
git commit -m "feat: update all handlers to use tenant-scoped TenantDB

All data handlers use tenantDB(r) for collection access.
Device auth uses tenant-scoped NATS KV bucket.
User management scoped to tenant from URL.
Login returns JWT with tenant context."
```

---

## Task 6: NATS Tenant-Scoped KV Provisioning

**Files:**
- Modify: `backend/services/controller/internal/nats/nats.go`
- Modify: `backend/services/controller/internal/api/tenant.go`

- [ ] **Step 1: Remove global KV bucket creation**

In `backend/services/controller/internal/nats/nats.go`, remove the global `devices-auth` bucket creation from `StartNatsClient()` (lines 54-57). The function should still return `js` and `nc`, but no longer return `kv`.

Update the function signature:

```go
func StartNatsClient(natsConfig config.Nats) (jetstream.JetStream, *nats.Conn) {
	// ... existing connection setup ...
	// Remove: kv, err := js.CreateOrUpdateKeyValue(...)
	return js, nc
}
```

- [ ] **Step 2: Add tenant KV provisioning helper**

In `backend/services/controller/internal/nats/nats.go`:

```go
func CreateTenantKVBucket(js jetstream.JetStream, slug string) (jetstream.KeyValue, error) {
	return js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket:      "devices-auth-" + slug,
		Description: "Device authentication for tenant " + slug,
	})
}

func DeleteTenantKVBucket(js jetstream.JetStream, slug string) error {
	return js.DeleteKeyValue(context.Background(), "devices-auth-"+slug)
}
```

- [ ] **Step 3: Call KV provisioning on tenant creation**

In `backend/services/controller/internal/api/tenant.go`, inside `createTenant()`, after `ProvisionTenantDBs()`:

```go
	// Create tenant-scoped NATS KV bucket for device auth
	if _, err := oktopusNats.CreateTenantKVBucket(a.js, slug); err != nil {
		http.Error(w, `{"error":"failed to create device auth bucket"}`, http.StatusInternalServerError)
		return
	}
```

In `deleteTenant()`, before deleting tenant record:

```go
	// Delete tenant-scoped NATS KV bucket
	oktopusNats.DeleteTenantKVBucket(a.js, slug)
```

- [ ] **Step 4: Update main.go — remove kv from api init**

In `backend/services/controller/cmd/controller/main.go`, update to match new signatures:

```go
js, nc := nats.StartNatsClient(c.Nats)      // was: js, nc, kv := ...
// ...
api := api.NewApi(c, js, nc, bridge, db)     // remove kv parameter
```

Update `NewApi()` in `api.go` accordingly.

- [ ] **Step 5: Commit**

```bash
git add backend/services/controller/internal/nats/ \
        backend/services/controller/internal/api/tenant.go \
        backend/services/controller/cmd/controller/main.go \
        backend/services/controller/internal/api/api.go
git commit -m "feat: tenant-scoped NATS KV buckets for device auth

Remove global devices-auth bucket.
Add CreateTenantKVBucket/DeleteTenantKVBucket helpers.
Provision KV bucket on tenant creation, delete on tenant removal."
```

---

## Task 7: NATS Subject Tenant Prefix

**Files:**
- Modify: `backend/services/controller/internal/nats/nats.go`
- Modify: `backend/services/controller/internal/bridge/bridge.go`

- [ ] **Step 1: Update NATS subject constants to accept tenant slug**

In `backend/services/controller/internal/nats/nats.go`, convert the subject prefix constants into functions:

```go
func DeviceSubjectPrefix(tenantSlug string) string {
	return "device.usp.v1." + tenantSlug + "."
}

func DeviceCwmpSubjectPrefix(tenantSlug string) string {
	return "device.cwmp.v1." + tenantSlug + "."
}

func MqttSubjectPrefix(tenantSlug string) string {
	return "mqtt.usp.v1." + tenantSlug + "."
}

func MqttAdapterSubjectPrefix(tenantSlug string) string {
	return "mqtt-adapter.usp.v1." + tenantSlug + "."
}

func AdapterSubject(tenantSlug string) string {
	return "adapter.usp.v1." + tenantSlug + "."
}

func WsSubjectPrefix(tenantSlug string) string {
	return "ws.usp.v1." + tenantSlug + "."
}

func WsAdapterSubjectPrefix(tenantSlug string) string {
	return "ws-adapter.usp.v1." + tenantSlug + "."
}

func StompAdapterSubjectPrefix(tenantSlug string) string {
	return "stomp-adapter.usp.v1." + tenantSlug + "."
}

func CwmpAdapterSubjectPrefix(tenantSlug string) string {
	return "cwmp-adapter.v1." + tenantSlug + "."
}
```

Keep the old constants temporarily for backward compatibility, but mark them as deprecated.

- [ ] **Step 2: Update bridge.go to accept tenant slug**

In `backend/services/controller/internal/bridge/bridge.go`, update `NatsReq`, `NatsUspInteraction`, `NatsCwmpInteraction` functions to accept a `tenantSlug` parameter and use the new subject prefix functions instead of the old constants.

The exact changes depend on how bridge.go currently constructs NATS subjects. Read the file and update each function that references the old constants.

- [ ] **Step 3: Update all API handlers that call bridge functions**

Pass `middleware.GetTenantSlug(r)` to bridge function calls. This affects handlers in:
- `api/device.go` (USP/CWMP device interactions)
- `api/deviceinfo.go` (device info queries)
- `api/usp.go` (if exists — USP operations)

- [ ] **Step 4: Commit**

```bash
git add backend/services/controller/internal/nats/ \
        backend/services/controller/internal/bridge/ \
        backend/services/controller/internal/api/
git commit -m "feat: add tenant slug to NATS subject prefixes

All device communication subjects now include tenant slug for isolation.
Bridge functions accept tenantSlug parameter.
Handlers pass tenant context from middleware."
```

---

## Task 8: Frontend — Auth Context and Tenant Provider

**Files:**
- Modify: `frontend/src/contexts/auth-context.js`
- Create: `frontend/src/contexts/tenant-context.js`

- [ ] **Step 1: Update auth-context.js to parse tenant info from JWT**

In `frontend/src/contexts/auth-context.js`, add JWT decoding to extract tenant fields. After receiving the token on login:

```javascript
// Add at the top of the file
function parseJwt(token) {
  try {
    const base64Url = token.split('.')[1];
    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
    return JSON.parse(window.atob(base64));
  } catch {
    return null;
  }
}
```

In the `signIn()` function, after receiving the token, parse it:

```javascript
const claims = parseJwt(token);
const user = {
  avatar: '/assets/avatars/default-avatar.png',
  name: claims?.username || email,
  email: email,
  tenantId: claims?.tenant_id || '',
  tenantSlug: claims?.tenant_slug || '',
  level: claims?.level ?? 0,
};
```

Store tenant info in sessionStorage alongside email:

```javascript
window.sessionStorage.setItem('tenantSlug', claims?.tenant_slug || '');
window.sessionStorage.setItem('tenantId', claims?.tenant_id || '');
window.sessionStorage.setItem('level', String(claims?.level ?? 0));
```

In the `initialize()` function, restore tenant info from sessionStorage:

```javascript
const tenantSlug = window.sessionStorage.getItem('tenantSlug') || '';
const tenantId = window.sessionStorage.getItem('tenantId') || '';
const level = parseInt(window.sessionStorage.getItem('level') || '0', 10);
const user = {
  avatar: '/assets/avatars/default-avatar.png',
  name: email,
  email: email,
  tenantSlug,
  tenantId,
  level,
};
```

- [ ] **Step 2: Create tenant-context.js**

Create `frontend/src/contexts/tenant-context.js`:

```javascript
import { createContext, useCallback, useContext, useMemo } from 'react';
import { useAuth } from 'src/hooks/use-auth';

const TenantContext = createContext({
  tenantSlug: '',
  level: 0,
  isSuperAdmin: false,
  apiPrefix: '',
  setActiveTenant: () => {},
});

export function TenantProvider({ children }) {
  const { user } = useAuth();
  const level = user?.level ?? 0;
  const isSuperAdmin = level === 0;

  // SuperAdmin can switch tenants; tenant users use their JWT slug
  const [activeTenantSlug, setActiveTenantSlug] = useState(
    user?.tenantSlug || ''
  );

  const setActiveTenant = useCallback((slug) => {
    if (isSuperAdmin) {
      setActiveTenantSlug(slug);
      window.sessionStorage.setItem('activeTenantSlug', slug);
    }
  }, [isSuperAdmin]);

  // Restore active tenant for SuperAdmin on mount
  useEffect(() => {
    if (isSuperAdmin) {
      const stored = window.sessionStorage.getItem('activeTenantSlug');
      if (stored) setActiveTenantSlug(stored);
    }
  }, [isSuperAdmin]);

  const tenantSlug = isSuperAdmin ? activeTenantSlug : (user?.tenantSlug || '');
  const apiPrefix = tenantSlug ? `/api/tenants/${tenantSlug}` : '/api';

  const value = useMemo(() => ({
    tenantSlug,
    level,
    isSuperAdmin,
    apiPrefix,
    setActiveTenant,
  }), [tenantSlug, level, isSuperAdmin, apiPrefix, setActiveTenant]);

  return (
    <TenantContext.Provider value={value}>
      {children}
    </TenantContext.Provider>
  );
}

export const useTenant = () => useContext(TenantContext);
```

Add missing imports (`useState`, `useEffect`) at the top.

- [ ] **Step 3: Wire TenantProvider into the app**

In `frontend/src/pages/_app.js`, wrap the app with `TenantProvider` inside `AuthProvider`:

```javascript
import { TenantProvider } from 'src/contexts/tenant-context';

// Inside the component tree, after AuthProvider:
<AuthProvider>
  <TenantProvider>
    {/* existing children */}
  </TenantProvider>
</AuthProvider>
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/contexts/ frontend/src/pages/_app.js
git commit -m "feat: add tenant context to frontend

Parse JWT for tenant fields on login.
TenantProvider manages active tenant and API prefix.
SuperAdmin can switch tenants; tenant users locked to their slug."
```

---

## Task 9: Frontend — Update API Calls to Use Tenant Prefix

**Files:**
- Modify: All frontend pages/sections that make API calls

- [ ] **Step 1: Create a tenant-aware fetch helper**

If the frontend uses direct `fetch()` calls, create a helper or update the existing pattern. In the simplest case, each page that calls the API should use `useTenant()` to get `apiPrefix`:

```javascript
import { useTenant } from 'src/contexts/tenant-context';

// Inside component:
const { apiPrefix } = useTenant();

// Replace: fetch('/api/devices/...')
// With:    fetch(`${apiPrefix}/devices/...`)
```

- [ ] **Step 2: Update devices page**

In `frontend/src/pages/devices.js` and `frontend/src/sections/devices/`, replace all `/api/device` fetch URLs with `${apiPrefix}/devices`.

- [ ] **Step 3: Update firmware page**

In `frontend/src/pages/firmware.js` and `frontend/src/sections/firmware/`, replace all `/api/firmware` fetch URLs with `${apiPrefix}/firmware`.

- [ ] **Step 4: Update scripts page**

In `frontend/src/pages/scripts.js` and `frontend/src/sections/scripts/`, replace all `/api/scripts` fetch URLs with `${apiPrefix}/scripts`.

- [ ] **Step 5: Update mass actions page**

In `frontend/src/pages/mass-actions/` and `frontend/src/sections/mass-actions/`, replace all `/api/mass-actions` fetch URLs with `${apiPrefix}/mass-actions`.

- [ ] **Step 6: Update credentials page**

In `frontend/src/pages/credentials.js`, replace all `/api/device/auth` fetch URLs with `${apiPrefix}/device/auth`.

- [ ] **Step 7: Update users page**

In `frontend/src/pages/access-control/users.js`, replace all `/api/users` and `/api/auth/register` fetch URLs. User listing: `${apiPrefix}/users`. User creation: `${apiPrefix}/users`.

- [ ] **Step 8: Update dashboard/overview page**

In `frontend/src/pages/index.js` and info-related sections, replace all `/api/info` fetch URLs with `${apiPrefix}/info`.

- [ ] **Step 9: Update device detail pages**

In `frontend/src/pages/devices/[...id].js` and all device detail sections, replace all `/api/device` fetch URLs with `${apiPrefix}/devices`.

- [ ] **Step 10: Commit**

```bash
git add frontend/src/
git commit -m "feat: update all frontend API calls to use tenant prefix

All fetch() calls now use apiPrefix from TenantContext.
URLs change from /api/resource to /api/tenants/{slug}/resource."
```

---

## Task 10: Frontend — Tenant Management Page and Nav

**Files:**
- Create: `frontend/src/pages/tenants/index.js`
- Create: `frontend/src/sections/tenants/tenants-table.js`
- Create: `frontend/src/sections/tenants/tenant-form.js`
- Modify: `frontend/src/layouts/dashboard/config.js`
- Delete: `frontend/src/pages/companies.js`

- [ ] **Step 1: Create tenants-table.js**

Create `frontend/src/sections/tenants/tenants-table.js` — a Material UI table showing tenant name, slug, status, created date, and action buttons (edit, suspend, delete). Follow the same pattern as `frontend/src/sections/customer/customers-table.js`.

- [ ] **Step 2: Create tenant-form.js**

Create `frontend/src/sections/tenants/tenant-form.js` — a Material UI dialog with fields: Name, Admin Email, Admin Password (for creation). Follow the same dialog pattern as the user creation dialog in `frontend/src/pages/access-control/users.js`.

- [ ] **Step 3: Create tenants page**

Create `frontend/src/pages/tenants/index.js` — the tenant management page (SuperAdmin only). It should:
- Fetch tenants from `GET /api/tenants`
- Display them in `TenantsTable`
- Have a "Create Tenant" button that opens `TenantForm`
- Support delete with confirmation dialog
- Follow the same page structure as `frontend/src/pages/access-control/users.js`

- [ ] **Step 4: Add tenant selector to dashboard layout**

For SuperAdmin users, add a tenant selector dropdown in the top navigation bar. This calls `setActiveTenant(slug)` from `useTenant()`. When no tenant is selected, show the tenants management page. When a tenant is selected, show the normal dashboard scoped to that tenant.

Location: `frontend/src/layouts/dashboard/top-nav.js` or equivalent layout component.

- [ ] **Step 5: Update nav config for role-based visibility**

In `frontend/src/layouts/dashboard/config.js`, add the Tenants nav item (SuperAdmin only) and add a `level` or `role` field to items that should be conditionally shown:

```javascript
{
  title: 'Tenants',
  path: '/tenants',
  icon: <BusinessIcon />,
  superAdminOnly: true,
},
```

In the nav rendering component, filter items based on user level. Remove the Companies nav item.

- [ ] **Step 6: Remove companies page**

Delete `frontend/src/pages/companies.js` and `frontend/src/sections/companies/` directory.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/pages/tenants/ \
        frontend/src/sections/tenants/ \
        frontend/src/layouts/dashboard/config.js \
        frontend/src/layouts/dashboard/
git rm frontend/src/pages/companies.js \
       frontend/src/sections/companies/company-card.js \
       frontend/src/sections/companies/companies-search.js
git commit -m "feat: add tenant management page and role-based navigation

Tenants page for SuperAdmin with CRUD operations.
Tenant selector dropdown in top nav for SuperAdmin.
Role-based nav item visibility.
Remove placeholder companies page."
```

---

## Task 11: Nginx Config Update

**Files:**
- Modify: `deploy/compose/nginx.conf`

- [ ] **Step 1: Update nginx routes**

The nginx reverse proxy needs to route `/api/tenants/` to the controller. Since all data endpoints now live under `/api/tenants/{slug}/...`, the existing `/api/` proxy rule should already handle this. Verify and update if needed.

If the existing rule is:
```nginx
location /api/ {
    proxy_pass http://controller:8000;
}
```

This already covers `/api/tenants/...`. No change needed unless there are specific location blocks for `/api/device/`, `/api/firmware/`, etc. that need updating.

- [ ] **Step 2: Commit (if changes made)**

```bash
git add deploy/compose/nginx.conf
git commit -m "feat: update nginx config for tenant-scoped API routes"
```

---

## Task 12: Integration Verification

- [ ] **Step 1: Build controller**

Run: `sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build controller 2>&1"`

Expected: Build succeeds with no errors.

- [ ] **Step 2: Build frontend**

Run: `sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build frontend 2>&1"`

Expected: Build succeeds with no errors.

- [ ] **Step 3: Start services and test basic flow**

Run: `sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && ./run_debug.sh"`

Test manually:
1. Register SuperAdmin via `POST /api/auth/admin/register`
2. Login as SuperAdmin, verify JWT contains `level: 0`, empty `tenant_id`
3. Create a tenant via `POST /api/tenants` with admin email/password
4. Login as TenantAdmin, verify JWT contains `level: 1`, correct `tenant_id` and `tenant_slug`
5. Access `GET /api/tenants/{slug}/devices` as TenantAdmin — should return empty list
6. Try accessing a different tenant's slug as TenantAdmin — should get 403
7. Access `GET /api/tenants/{slug}/devices` as SuperAdmin — should work

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix: integration test fixes for multi-tenancy"
```
