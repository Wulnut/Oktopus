// Package snapshot boots the full Oktopus controller behind an httptest.Server
// so external black-box tests can drive the real router over HTTP with sanitized
// fixtures loaded from the test Mongo.
//
// No production code is reflected upon. The test module owns its own *mongo.Client
// (separate from the controller's) so the db package surface stays untouched.
// The only production-code change required by the whole suite is the exported
// Api.BuildRouter() method that lets httptest reuse the real route table.
package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/leandrofars/oktopus/internal/api"
	"github.com/leandrofars/oktopus/internal/api/auth"
	"github.com/leandrofars/oktopus/internal/bridge"
	"github.com/leandrofars/oktopus/internal/config"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SnapshotTenantSlug is the tenant slug all snapshot tests target. Tests load
// fixtures into tenant_<slug>_general and assert against routes scoped to it.
const SnapshotTenantSlug = "snapshot-test"

// FixtureMap maps a fixture file name to the Mongo collection (under
// tenant_<slug>_general) it should be seeded into.
var FixtureMap = map[string]string{
	"devices.json":               "devices",
	"firmware.json":              "firmware",
	"scripts.json":               "scripts",
	"campaigns.json":             "campaigns",
	"lock_policies.json":         "device_lock_policy",
	"lock_config.json":           "device_lock_config",
	"users.json":                 "users",
	"ca_certs.json":              "ca_certs",
	"upgrade_logs.json":          "upgrade_logs",
	"device_info.json":           "device_info",
	"lock_unauthorized.json":     "lock_unauthorized_devices",
	"fw_policies.json":           "fw_policies",
	"lock_audit_logs.json":       "lock_audit_logs",
	"lock_command_attempts.json": "lock_command_attempts",
}

// Env holds everything a snapshot test needs. Setup returns a populated Env
// plus a cleanup registered via t.Cleanup that drops the tenant DBs and tears
// the server down.
type Env struct {
	Server      *httptest.Server
	JWT         string // TenantAdmin token scoped to SnapshotTenantSlug (level 1)
	AdminJWT    string // SuperAdmin token (level 0)
	OperatorJWT string // Operator token scoped to SnapshotTenantSlug (level 2)
	Mongo       *mongo.Client
}

// Setup boots test Mongo + test NATS (provided by docker-compose.test.yaml),
// provisions the snapshot tenant, loads requested fixtures, mounts the real
// controller router on httptest.Server, and returns a ready Env.
//
// fixtures is a subset of FixtureMap keys (e.g. []string{"devices.json"}).
// Pass nil to boot the server with an empty tenant DB.
func Setup(t *testing.T, fixtures []string) *Env {
	t.Helper()

	mongoURI := getenv("MONGO_TEST_URI", "mongodb://localhost:27017")
	natsURL := getenv("NATS_TEST_URL", "nats://localhost:4222")

	// Short-lived setup ctx for the bootstrap phase.
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer setupCancel()

	// Our own Mongo client — independent from the controller's, so we can seed
	// and tear down without needing new exported methods on db.Database.
	client, err := mongo.Connect(setupCtx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("snapshot: mongo.Connect failed: %v", err)
	}
	if err := client.Ping(setupCtx, nil); err != nil {
		t.Fatalf("snapshot: cannot reach Mongo at %s: %v\n"+
			"hint: start deps with `docker compose -f docker-compose.test.yaml --profile integration up -d mongo_test nats_test`",
			mongoURI, err)
	}

	// Drop any prior snapshot tenant DBs so each test starts clean.
	dropTenantDBs(setupCtx, t, client, SnapshotTenantSlug)

	// Also wipe account-mngr.users and account-mngr.tenants so each test starts
	// from a known empty state (some auth tests assert "no admin exists").
	// The snapshot tenant row is re-inserted below.
	_, _ = client.Database("account-mngr").Collection("users").DeleteMany(setupCtx, bson.M{})
	_, _ = client.Database("account-mngr").Collection("tenants").DeleteMany(setupCtx, bson.M{})

	// Long-lived context for the controller's Database — it stores this ctx
	// internally and uses it for all subsequent operations, so it must outlive
	// Setup(). context.Background() is correct here; cleanup disconnects the
	// client at the end of the test.
	controllerCtx := context.Background()
	database := db.NewDatabase(controllerCtx, mongoURI)
	if err := database.Ping(controllerCtx); err != nil {
		t.Fatalf("snapshot: controller db ping failed: %v", err)
	}
	if err := database.ProvisionTenantDBs(controllerCtx, SnapshotTenantSlug); err != nil {
		t.Fatalf("snapshot: ProvisionTenantDBs failed: %v", err)
	}
	seedTenantRow(setupCtx, t, client)

	fixtureDir := resolveFixtureDir(t)
	for _, f := range fixtures {
		coll, ok := FixtureMap[f]
		if !ok {
			t.Fatalf("snapshot: unknown fixture %q — not in FixtureMap", f)
		}
		loadFixture(setupCtx, t, client, filepath.Join(fixtureDir, f), coll)
	}

	nc, err := nats.Connect(natsURL, nats.Name("snapshot-test"))
	if err != nil {
		t.Fatalf("snapshot: cannot reach NATS at %s: %v", natsURL, err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("snapshot: jetstream.New failed: %v", err)
	}

	br := bridge.NewBridge(js, nc)

	cfg := &config.Config{
		RestApi:            config.RestApi{Port: "0", Ctx: context.Background()},
		CampaignScheduler:  config.CampaignScheduler{Enabled: false, Interval: 60 * time.Second},
		LockRetryScheduler: config.LockRetryScheduler{Enabled: false, Interval: 30 * time.Second, CommandTimeout: 30 * time.Second, MaxAttempts: 3},
		LockCircuitBreaker: config.LockCircuitBreaker{Enabled: false},
	}

	a := api.NewApi(cfg, js, nc, br, database)
	srv := httptest.NewServer(a.BuildRouter())

	jwt, err := auth.GenerateJWT("tenant@test.example.com", "tenantadmin", "", SnapshotTenantSlug, 1)
	if err != nil {
		t.Fatalf("snapshot: GenerateJWT failed: %v", err)
	}
	adminJWT, err := auth.GenerateJWT("super@test.example.com", "superadmin", "", "", 0)
	if err != nil {
		t.Fatalf("snapshot: GenerateJWT(superadmin) failed: %v", err)
	}
	operatorJWT, err := auth.GenerateJWT("operator@test.example.com", "operator", "", SnapshotTenantSlug, 2)
	if err != nil {
		t.Fatalf("snapshot: GenerateJWT(operator) failed: %v", err)
	}

	env := &Env{
		Server:      srv,
		JWT:         jwt,
		AdminJWT:    adminJWT,
		OperatorJWT: operatorJWT,
		Mongo:       client,
	}

	t.Cleanup(func() {
		srv.Close()
		nc.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		dropTenantDBs(cleanupCtx, t, client, SnapshotTenantSlug)
		_, _ = client.Database("account-mngr").Collection("tenants").
			DeleteOne(cleanupCtx, bson.M{"slug": SnapshotTenantSlug})
		_ = client.Disconnect(cleanupCtx)
	})

	return env
}

// TenantURL returns the absolute URL for a tenant-scoped path.
// Example: env.TenantURL("/device") -> http://127.0.0.1:port/api/tenants/snapshot-test/device
func (e *Env) TenantURL(path string) string {
	return e.Server.URL + "/api/tenants/" + SnapshotTenantSlug + path
}

// AdminURL returns an absolute URL for a SuperAdmin-only or non-tenant path.
func (e *Env) AdminURL(path string) string {
	return e.Server.URL + path
}

// Collection returns a handle on the snapshot tenant's _general DB collection.
// Tests use it to assert DB side effects after write requests.
func (e *Env) Collection(name string) *mongo.Collection {
	return e.Mongo.Database(fmt.Sprintf("tenant_%s_general", SnapshotTenantSlug)).Collection(name)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// resolveFixtureDir walks up from this source file to find the repo-root
// fixtures/ directory. Falls back to a relative "fixtures" path.
func resolveFixtureDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)
	for i := 0; i < 10; i++ {
		candidate := filepath.Join(dir, "fixtures")
		if abs, err := filepath.Abs(candidate); err == nil {
			if st, err := os.Stat(abs); err == nil && st.IsDir() {
				return abs
			}
		}
		dir = filepath.Join(dir, "..")
	}
	return "fixtures"
}

func loadFixture(ctx context.Context, t *testing.T, client *mongo.Client, path, coll string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("snapshot: cannot read fixture %s: %v", path, err)
	}

	// Parse each element of the JSON array as a bson.M via UnmarshalExtJSON,
	// which understands mongo-extended JSON ({"$date": "..."}, {"$oid": "..."}).
	// First parse as []json.RawMessage to get each element, then ext-json each.
	var rawDocs []json.RawMessage
	if err := json.Unmarshal(raw, &rawDocs); err != nil {
		t.Fatalf("snapshot: fixture %s is not a JSON array: %v", path, err)
	}
	if len(rawDocs) == 0 {
		return
	}
	collection := client.Database(fmt.Sprintf("tenant_%s_general", SnapshotTenantSlug)).Collection(coll)
	docsBson := make([]interface{}, 0, len(rawDocs))
	for i, rd := range rawDocs {
		var bd bson.M
		if err := bson.UnmarshalExtJSON(rd, false, &bd); err != nil {
			t.Fatalf("snapshot: fixture %s[%d] invalid ext JSON: %v", path, i, err)
		}
		// Strip any client-supplied _id so inserts don't conflict.
		delete(bd, "_id")
		docsBson = append(docsBson, bd)
	}
	if _, err := collection.InsertMany(ctx, docsBson); err != nil {
		t.Fatalf("snapshot: InsertMany into %s failed: %v", coll, err)
	}
}

func seedTenantRow(ctx context.Context, t *testing.T, client *mongo.Client) {
	t.Helper()
	_, err := client.Database("account-mngr").Collection("tenants").
		InsertOne(ctx, bson.M{
			"name":   "Snapshot Test Tenant",
			"slug":   SnapshotTenantSlug,
			"status": "active",
		})
	if err != nil {
		// Already present from a previous test in the same container; not fatal.
		return
	}
}

func dropTenantDBs(ctx context.Context, t *testing.T, client *mongo.Client, slug string) {
	_ = client.Database(fmt.Sprintf("tenant_%s_general", slug)).Drop(ctx)
	_ = client.Database(fmt.Sprintf("tenant_%s_usp", slug)).Drop(ctx)
}
