package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TenantCACert represents a CA certificate attached to a tenant for device authentication.
type TenantCACert struct {
	ID      primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Label   string             `bson:"label"         json:"label"`
	PEM     string             `bson:"pem"           json:"pem"`
	NotAfter time.Time         `bson:"not_after"     json:"not_after"`
	AddedAt  time.Time         `bson:"added_at"      json:"added_at"`
}

// TenantAuthPolicy defines the authentication requirements for a tenant.
type TenantAuthPolicy struct {
	PasswordRequired bool `bson:"password_required" json:"password_required"`
	CertRequired     bool `bson:"cert_required"     json:"cert_required"`
}

// TenantStatus represents the state of a tenant.
type TenantStatus string

const (
	TenantStatusActive   TenantStatus = "active"
	TenantStatusDisabled TenantStatus = "disabled"
)

// Tenant represents a tenant in the multi-tenant system.
type Tenant struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name       string             `bson:"name"          json:"name"`
	Slug       string             `bson:"slug"          json:"slug"`
	Status     TenantStatus       `bson:"status"        json:"status"`
	AuthPolicy TenantAuthPolicy   `bson:"auth_policy"   json:"auth_policy"`
	CACerts    []TenantCACert     `bson:"ca_certs"      json:"ca_certs"`
	CreatedAt  time.Time          `bson:"created_at"    json:"created_at"`
	UpdatedAt  time.Time          `bson:"updated_at"    json:"updated_at"`
}

// TenantDB provides access to per-tenant MongoDB databases (general + usp).
type TenantDB struct {
	General *mongo.Database
	Usp     *mongo.Database
}

// --- Collection accessors ---

func (t *TenantDB) Templates() *mongo.Collection   { return t.General.Collection("templates") }
func (t *TenantDB) Firmware() *mongo.Collection     { return t.General.Collection("firmware") }
func (t *TenantDB) Scripts() *mongo.Collection      { return t.General.Collection("scripts") }
func (t *TenantDB) ScriptExecs() *mongo.Collection  { return t.General.Collection("script_executions") }
func (t *TenantDB) MassActions() *mongo.Collection  { return t.General.Collection("mass_actions") }
func (t *TenantDB) DeviceInfo() *mongo.Collection   { return t.General.Collection("device_info") }
func (t *TenantDB) Campaigns() *mongo.Collection    { return t.General.Collection("campaigns") }
func (t *TenantDB) FWPolicies() *mongo.Collection   { return t.General.Collection("fw_policies") }
func (t *TenantDB) UpgradeLogs() *mongo.Collection  { return t.General.Collection("upgrade_logs") }
func (t *TenantDB) Messages() *mongo.Collection     { return t.Usp.Collection("messages") }
func (t *TenantDB) MessagesErrors() *mongo.Collection { return t.Usp.Collection("messages_errors") }
func (t *TenantDB) Metrics() *mongo.Collection       { return t.Usp.Collection("device_metrics") }

// --- ForTenant returns a TenantDB scoped to a given tenant slug ---

func (d *Database) ForTenant(slug string) *TenantDB {
	generalName := fmt.Sprintf("tenant_%s_general", slug)
	uspName := fmt.Sprintf("tenant_%s_usp", slug)
	return &TenantDB{
		General: d.client.Database(generalName),
		Usp:     d.client.Database(uspName),
	}
}

// --- Tenant CRUD operations (on the shared tenants collection) ---

func (d *Database) CreateTenant(ctx context.Context, t Tenant) (Tenant, error) {
	t.ID = primitive.NewObjectID()
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	if t.Status == "" {
		t.Status = TenantStatusActive
	}
	if t.CACerts == nil {
		t.CACerts = []TenantCACert{}
	}
	_, err := d.tenants.InsertOne(ctx, t)
	return t, err
}

func (d *Database) FindTenant(ctx context.Context, slug string) (Tenant, error) {
	var t Tenant
	err := d.tenants.FindOne(ctx, bson.M{"slug": slug}).Decode(&t)
	return t, err
}

func (d *Database) FindTenantByID(ctx context.Context, id primitive.ObjectID) (Tenant, error) {
	var t Tenant
	err := d.tenants.FindOne(ctx, bson.M{"_id": id}).Decode(&t)
	return t, err
}

func (d *Database) FindAllTenants(ctx context.Context) ([]Tenant, error) {
	cursor, err := d.tenants.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var results []Tenant
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (d *Database) UpdateTenant(ctx context.Context, id primitive.ObjectID, t Tenant) error {
	_, err := d.tenants.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"name":        t.Name,
			"slug":        t.Slug,
			"status":      t.Status,
			"auth_policy": t.AuthPolicy,
			"updated_at":  time.Now(),
		}})
	return err
}

func (d *Database) DeleteTenant(ctx context.Context, id primitive.ObjectID) error {
	_, err := d.tenants.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// --- CA cert operations ---

func (d *Database) AddCACert(ctx context.Context, tenantID primitive.ObjectID, cert TenantCACert) error {
	cert.ID = primitive.NewObjectID()
	cert.AddedAt = time.Now()
	_, err := d.tenants.UpdateOne(ctx,
		bson.M{"_id": tenantID},
		bson.M{
			"$push": bson.M{"ca_certs": cert},
			"$set":  bson.M{"updated_at": time.Now()},
		})
	return err
}

func (d *Database) RemoveCACert(ctx context.Context, tenantID, certID primitive.ObjectID) error {
	_, err := d.tenants.UpdateOne(ctx,
		bson.M{"_id": tenantID},
		bson.M{
			"$pull": bson.M{"ca_certs": bson.M{"_id": certID}},
			"$set":  bson.M{"updated_at": time.Now()},
		})
	return err
}

// --- Tenant database provisioning ---

// ProvisionTenantDBs creates all collection indexes for a tenant's databases.
func (d *Database) ProvisionTenantDBs(ctx context.Context, slug string) error {
	tdb := d.ForTenant(slug)

	// --- general database collections ---

	// templates: unique name
	_, err := tdb.Templates().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("templates index: %w", err)
	}

	// firmware: unique compound (vendor, model, hw_version, build_version)
	_, err = tdb.Firmware().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "vendor", Value: 1},
			{Key: "model", Value: 1},
			{Key: "hw_version", Value: 1},
			{Key: "build_version", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("firmware index: %w", err)
	}

	// scripts: unique name
	_, err = tdb.Scripts().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("scripts index: %w", err)
	}

	// script_executions
	_, err = tdb.ScriptExecs().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "created_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(2592000), // 30 days
		},
		{
			Keys: bson.D{{Key: "script_id", Value: 1}, {Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "device_sn", Value: 1}, {Key: "created_at", Value: -1}},
		},
	})
	if err != nil {
		return fmt.Errorf("script_executions indexes: %w", err)
	}

	// mass_actions
	_, err = tdb.MassActions().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "created_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(7776000), // 90 days
		},
		{
			Keys: bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: -1}},
		},
	})
	if err != nil {
		return fmt.Errorf("mass_actions indexes: %w", err)
	}

	// device_info: unique device_sn
	_, err = tdb.DeviceInfo().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "device_sn", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("device_info index: %w", err)
	}

	// campaigns: unique (vendor, model, hw_version)
	_, err = tdb.Campaigns().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "vendor", Value: 1}, {Key: "model", Value: 1}, {Key: "hw_version", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("campaigns index: %w", err)
	}

	// fw_policies: unique device_sn
	_, err = tdb.FWPolicies().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "device_sn", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("fw_policies index: %w", err)
	}

	// upgrade_logs
	_, err = tdb.UpgradeLogs().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "campaign_id", Value: 1}, {Key: "triggered_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "device_sn", Value: 1}, {Key: "triggered_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "device_sn", Value: 1}, {Key: "firmware_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "device_sn", Value: 1}, {Key: "status", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "triggered_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(7776000), // 90 days
		},
	})
	if err != nil {
		return fmt.Errorf("upgrade_logs indexes: %w", err)
	}

	// --- usp database collections ---

	// messages
	err = createMessageIndexes(ctx, tdb.Messages())
	if err != nil {
		return fmt.Errorf("messages indexes: %w", err)
	}

	// messages_errors
	err = createMessageErrorIndexes(ctx, tdb.MessagesErrors())
	if err != nil {
		return fmt.Errorf("messages_errors indexes: %w", err)
	}

	// device_metrics
	_, err = tdb.Metrics().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "timestamp", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(604800), // 7 days
		},
		{
			Keys: bson.D{{Key: "device_serial", Value: 1}, {Key: "timestamp", Value: -1}},
		},
	})
	if err != nil {
		return fmt.Errorf("device_metrics indexes: %w", err)
	}

	log.Printf("Provisioned databases for tenant %q", slug)
	return nil
}

// DropTenantDBs drops all databases for a tenant.
func (d *Database) DropTenantDBs(ctx context.Context, slug string) error {
	generalName := fmt.Sprintf("tenant_%s_general", slug)
	uspName := fmt.Sprintf("tenant_%s_usp", slug)

	if err := d.client.Database(generalName).Drop(ctx); err != nil {
		return fmt.Errorf("drop %s: %w", generalName, err)
	}
	if err := d.client.Database(uspName).Drop(ctx); err != nil {
		return fmt.Errorf("drop %s: %w", uspName, err)
	}
	log.Printf("Dropped databases for tenant %q", slug)
	return nil
}
