//go:build integration

package db

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var testDB Database
var testTDB *TenantDB

func TestMain(m *testing.M) {
	mongoURI := os.Getenv("MONGO_TEST_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27018"
	}

	ctx := context.Background()

	testDB = NewDatabase(ctx, mongoURI)

	// Provision a test tenant
	testTenantSlug := fmt.Sprintf("test_%d", time.Now().UnixNano())
	if err := testDB.ProvisionTenantDBs(ctx, testTenantSlug); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to provision tenant DBs: %v\n", err)
		os.Exit(1)
	}
	testTDB = testDB.ForTenant(testTenantSlug)

	code := m.Run()

	// Cleanup: drop test databases
	testDB.client.Database("account-mngr").Drop(ctx)
	_ = testDB.DropTenantDBs(ctx, testTenantSlug)

	os.Exit(code)
}

// --- Firmware CRUD ---

func TestCreateAndGetFirmware(t *testing.T) {
	fw := Firmware{
		Name:         fmt.Sprintf("fw-test-%d", time.Now().UnixNano()),
		Vendor:       "TestVendor",
		Model:        "TestModel",
		BuildVersion: "1.0.0",
		Phase:        PhaseInternalTesting,
	}
	created, err := testTDB.CreateFirmware(context.Background(), fw)
	if err != nil {
		t.Fatalf("CreateFirmware failed: %v", err)
	}
	if created.ID.IsZero() {
		t.Error("Created firmware has zero ID")
	}

	got, err := testTDB.GetFirmware(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetFirmware failed: %v", err)
	}
	if got.Name != fw.Name {
		t.Errorf("Name mismatch: %s vs %s", got.Name, fw.Name)
	}
	if got.Vendor != fw.Vendor {
		t.Errorf("Vendor mismatch: %s vs %s", got.Vendor, fw.Vendor)
	}
}

func TestCreateAndListFirmware(t *testing.T) {
	for i := 0; i < 3; i++ {
		fw := Firmware{
			Name:         fmt.Sprintf("fw-list-%d-%d", time.Now().UnixNano(), i),
			Vendor:       fmt.Sprintf("Vendor-%d", i),
			Model:        fmt.Sprintf("Model-%d", i),
			HWVersion:    fmt.Sprintf("hw-%d", i),
			BuildVersion: fmt.Sprintf("1.0.%d", i),
		}
		if _, err := testTDB.CreateFirmware(context.Background(), fw); err != nil {
			t.Fatal(err)
		}
	}
	list, err := testTDB.ListFirmware(context.Background())
	if err != nil {
		t.Fatalf("ListFirmware failed: %v", err)
	}
	if len(list) < 3 {
		t.Errorf("Expected at least 3 firmware entries, got %d", len(list))
	}
}

func TestUpdateFirmware_NonexistentID_Returns0Matched(t *testing.T) {
	matched, err := testTDB.UpdateFirmware(context.Background(), primitive.NewObjectID(), Firmware{
		Name: "nonexistent", BuildVersion: "1.0",
	})
	if err != nil {
		t.Fatalf("UpdateFirmware error: %v", err)
	}
	if matched != 0 {
		t.Errorf("Expected 0 matched for nonexistent ID, got %d", matched)
	}
}

func TestCreateAndDeleteFirmware(t *testing.T) {
	fw := Firmware{
		Name:         fmt.Sprintf("fw-del-%d", time.Now().UnixNano()),
		Vendor:       "DelVendor",
		Model:        "DelModel",
		HWVersion:    "hw-del",
		BuildVersion: fmt.Sprintf("1.0.%d", time.Now().UnixNano()),
	}
	created, err := testTDB.CreateFirmware(context.Background(), fw)
	if err != nil {
		t.Fatal(err)
	}
	if err := testTDB.DeleteFirmware(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	_, err = testTDB.GetFirmware(context.Background(), created.ID)
	if err == nil {
		t.Error("Expected error getting deleted firmware, got nil")
	}
}

// --- Firmware deletion does NOT cascade to campaigns (bug) ---

func TestDeleteFirmware_CampaignsNotCleaned(t *testing.T) {
	fw := Firmware{Name: fmt.Sprintf("fw-cascade-%d", time.Now().UnixNano()), BuildVersion: "2.0"}
	created, err := testTDB.CreateFirmware(context.Background(), fw)
	if err != nil {
		t.Fatal(err)
	}

	campaign := Campaign{
		Vendor:      "V",
		Model:       "M",
		HWVersion:   fmt.Sprintf("hw-%d", time.Now().UnixNano()),
		FirmwareID:  created.ID,
		Enabled:     true,
		Concurrency: 10,
	}
	createdCampaign, err := testTDB.CreateCampaign(context.Background(), campaign)
	if err != nil {
		t.Fatal(err)
	}

	// Delete firmware and cascade to campaigns (same as API handler does)
	if err := testTDB.DeleteFirmware(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	if err := testTDB.DisableCampaignsByFirmware(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}

	// Campaign should be disabled after firmware deletion
	got, err := testTDB.GetCampaign(context.Background(), createdCampaign.ID)
	if err != nil {
		// Campaign was deleted -- that's one valid fix
		return
	}
	if got.Enabled {
		t.Error("Campaign still enabled after firmware deletion -- DisableCampaignsByFirmware did not work")
	}
}

// --- ONT lock policy CRUD ---

func TestLockPolicyUpsert_MutualExclusionBySN(t *testing.T) {
	ctx := context.Background()
	sn := fmt.Sprintf("LOCK-%d", time.Now().UnixNano())

	_, err := testTDB.UpsertLockPolicy(ctx, LockPolicy{
		SN:             sn,
		PolicyType:     LockPolicyWhitelist,
		AllowedIPRange: "10.10.0.0/16",
		Status:         true,
	})
	if err != nil {
		t.Fatalf("create whitelist policy: %v", err)
	}

	_, err = testTDB.UpsertLockPolicy(ctx, LockPolicy{
		SN:             sn,
		PolicyType:     LockPolicyWhitelist,
		AllowedIPRange: "10.20.0.0/16",
		Description:    "updated policy",
		Status:         true,
	})
	if err != nil {
		t.Fatalf("replace whitelist policy: %v", err)
	}

	got, err := testTDB.GetLockPolicy(ctx, sn)
	if err != nil {
		t.Fatalf("get lock policy: %v", err)
	}
	if got.AllowedIPRange != "10.20.0.0/16" {
		t.Fatalf("expected updated CIDR to replace original, got %s", got.AllowedIPRange)
	}

	policies, err := testTDB.ListLockPolicies(ctx, "")
	if err != nil {
		t.Fatalf("list lock policies: %v", err)
	}
	count := 0
	for _, policy := range policies {
		if policy.SN == NormalizeSN(sn) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one active policy for SN %s, got %d", sn, count)
	}
}

func TestLockPolicyTenantIsolation_AllowsSameSNInDifferentTenants(t *testing.T) {
	ctx := context.Background()
	otherSlug := fmt.Sprintf("lock_other_%d", time.Now().UnixNano())
	if err := testDB.ProvisionTenantDBs(ctx, otherSlug); err != nil {
		t.Fatalf("provision other tenant: %v", err)
	}
	defer testDB.DropTenantDBs(ctx, otherSlug)

	otherTDB := testDB.ForTenant(otherSlug)
	sn := fmt.Sprintf("TENANT-%d", time.Now().UnixNano())

	_, err := testTDB.UpsertLockPolicy(ctx, LockPolicy{
		SN:             sn,
		PolicyType:     LockPolicyWhitelist,
		AllowedIPRange: "10.10.0.0/16",
		Status:         true,
	})
	if err != nil {
		t.Fatalf("create policy in first tenant: %v", err)
	}

	_, err = otherTDB.UpsertLockPolicy(ctx, LockPolicy{
		SN:             sn,
		PolicyType:     LockPolicyWhitelist,
		AllowedIPRange: "10.30.0.0/16",
		Description:    "second tenant policy",
		Status:         true,
	})
	if err != nil {
		t.Fatalf("create policy in second tenant: %v", err)
	}

	first, err := testTDB.GetLockPolicy(ctx, sn)
	if err != nil {
		t.Fatalf("get first tenant policy: %v", err)
	}
	second, err := otherTDB.GetLockPolicy(ctx, sn)
	if err != nil {
		t.Fatalf("get second tenant policy: %v", err)
	}

	if first.PolicyType != LockPolicyWhitelist {
		t.Fatalf("expected first tenant whitelist, got %s", first.PolicyType)
	}
	if second.AllowedIPRange != "10.30.0.0/16" {
		t.Fatalf("expected second tenant CIDR 10.30.0.0/16, got %s", second.AllowedIPRange)
	}
}

// --- Campaign CRUD ---

func TestCreateAndGetCampaign(t *testing.T) {
	c := Campaign{
		Vendor:      "CampaignVendor",
		Model:       "CampaignModel",
		HWVersion:   fmt.Sprintf("hw-%d", time.Now().UnixNano()),
		FirmwareID:  primitive.NewObjectID(),
		Enabled:     true,
		Concurrency: 5,
	}
	created, err := testTDB.CreateCampaign(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := testTDB.GetCampaign(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Vendor != c.Vendor {
		t.Errorf("Vendor mismatch: %s vs %s", got.Vendor, c.Vendor)
	}
	if got.Concurrency != 5 {
		t.Errorf("Concurrency mismatch: %d vs 5", got.Concurrency)
	}
}

func TestCampaignUniqueConstraint(t *testing.T) {
	hw := fmt.Sprintf("hw-unique-%d", time.Now().UnixNano())
	c := Campaign{Vendor: "V", Model: "M", HWVersion: hw, FirmwareID: primitive.NewObjectID(), Concurrency: 10}
	if _, err := testTDB.CreateCampaign(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	// Same vendor+model+hw_version should fail
	_, err := testTDB.CreateCampaign(context.Background(), c)
	if err == nil {
		t.Error("Expected duplicate key error, got nil")
	}
}

func TestCampaignUniqueConstraint_CaseInsensitive(t *testing.T) {
	hw := fmt.Sprintf("hw-ci-%d", time.Now().UnixNano())
	first := Campaign{Vendor: "Vendor", Model: "Model", HWVersion: hw, FirmwareID: primitive.NewObjectID(), Concurrency: 10}
	if _, err := testTDB.CreateCampaign(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	// Case-only difference must still be rejected by the case-insensitive unique index.
	second := Campaign{Vendor: "vendor", Model: "MODEL", HWVersion: hw, FirmwareID: primitive.NewObjectID(), Concurrency: 10}
	if _, err := testTDB.CreateCampaign(context.Background(), second); err == nil {
		t.Error("Expected duplicate key error for case-only difference, got nil")
	}
}

func TestGetCampaignByHardware_CaseAndWhitespaceInsensitive(t *testing.T) {
	hw := fmt.Sprintf("hw-lookup-%d", time.Now().UnixNano())
	c := Campaign{Vendor: "Huawei", Model: "HG8145", HWVersion: hw, FirmwareID: primitive.NewObjectID(), Concurrency: 10}
	if _, err := testTDB.CreateCampaign(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	got, err := testTDB.GetCampaignByHardware(context.Background(), "  HUAWEI  ", "hg8145", hw)
	if err != nil {
		t.Fatalf("expected case+whitespace-insensitive lookup to find campaign, got %v", err)
	}
	if got.Vendor != "Huawei" {
		t.Errorf("stored vendor mismatch: got %q want %q", got.Vendor, "Huawei")
	}
}

func TestCreateCampaign_TrimsHardwareFields(t *testing.T) {
	hw := fmt.Sprintf("  hw-trim-%d  ", time.Now().UnixNano())
	c := Campaign{Vendor: "  Vendor  ", Model: "  Model  ", HWVersion: hw, FirmwareID: primitive.NewObjectID(), Concurrency: 10}
	created, err := testTDB.CreateCampaign(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if created.Vendor != "Vendor" || created.Model != "Model" {
		t.Errorf("CreateCampaign did not trim: got vendor=%q model=%q", created.Vendor, created.Model)
	}
	if got := created.HWVersion; got[0] == ' ' || got[len(got)-1] == ' ' {
		t.Errorf("CreateCampaign did not trim hw_version: %q", got)
	}
}

// --- Upgrade Log ---

func TestCreateUpgradeLog_And_UpdateStatus(t *testing.T) {
	log := FirmwareUpgradeLog{
		DeviceSN:         "SN-TEST",
		FirmwareID:       primitive.NewObjectID(),
		FirmwareName:     "test-fw",
		FirmwareBuildVer: "1.0",
		TriggerType:      "on_connect",
		Status:           "pending",
		TriggeredAt:      time.Now(),
	}
	created, err := testTDB.CreateUpgradeLog(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	err = testTDB.UpdateUpgradeLogStatus(context.Background(), created.ID, "success", "")
	if err != nil {
		t.Fatal(err)
	}

	got, err := testTDB.GetLatestUpgradeLog(context.Background(), log.DeviceSN, log.FirmwareID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "success" {
		t.Errorf("Expected status success, got %s", got.Status)
	}
}

func TestUpgradeLogOnlyOneActiveAttemptPerDeviceFirmware(t *testing.T) {
	ctx := context.Background()
	fwID := primitive.NewObjectID()
	sn := fmt.Sprintf("SN-ACTIVE-UNIQUE-%d", time.Now().UnixNano())
	log1 := FirmwareUpgradeLog{DeviceSN: sn, FirmwareID: fwID, Status: "pending"}
	created, err := testTDB.CreateUpgradeLog(ctx, log1)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := testTDB.CreateUpgradeLog(ctx, log1); err == nil {
		t.Fatal("expected a duplicate-key error for a second active attempt")
	}

	if err := testTDB.UpdateUpgradeLogStatus(ctx, created.ID, "failed", "test failure"); err != nil {
		t.Fatal(err)
	}
	if _, err := testTDB.CreateUpgradeLog(ctx, log1); err != nil {
		t.Fatalf("terminal history must not block a later retry: %v", err)
	}
}

func TestUpgradeLogConcurrentCreateClaimsSingleActiveAttempt(t *testing.T) {
	ctx := context.Background()
	fwID := primitive.NewObjectID()
	sn := fmt.Sprintf("SN-ACTIVE-RACE-%d", time.Now().UnixNano())
	const workers = 12

	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := testTDB.CreateUpgradeLog(ctx, FirmwareUpgradeLog{
				DeviceSN:   sn,
				FirmwareID: fwID,
				Status:     "pending",
			})
			results <- err
		}()
	}

	succeeded := 0
	for i := 0; i < workers; i++ {
		if err := <-results; err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("expected exactly one active attempt, got %d successful inserts", succeeded)
	}
}

func TestMigrateActiveUpgradeAttemptsKeepsNewestLegacyAttempt(t *testing.T) {
	ctx := context.Background()
	fwID := primitive.NewObjectID()
	sn := fmt.Sprintf("SN-ACTIVE-MIGRATE-%d", time.Now().UnixNano())
	olderID := primitive.NewObjectID()
	newerID := primitive.NewObjectID()
	_, err := testTDB.UpgradeLogs().InsertMany(ctx, []interface{}{
		bson.M{
			"_id": olderID, "device_sn": sn, "firmware_id": fwID,
			"status": "pending", "triggered_at": time.Now().Add(-time.Minute),
		},
		bson.M{
			"_id": newerID, "device_sn": sn, "firmware_id": fwID,
			"status": "downloading", "triggered_at": time.Now(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := migrateActiveUpgradeAttempts(ctx, testTDB.UpgradeLogs()); err != nil {
		t.Fatal(err)
	}

	activeCount, err := testTDB.UpgradeLogs().CountDocuments(ctx, bson.M{
		"device_sn": sn,
		"status":    bson.M{"$in": []string{"pending", "downloading"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatalf("expected one migrated active attempt, got %d", activeCount)
	}
	var newer FirmwareUpgradeLog
	if err := testTDB.UpgradeLogs().FindOne(ctx, bson.M{"_id": newerID}).Decode(&newer); err != nil {
		t.Fatal(err)
	}
	if newer.ActiveAttemptKey == "" {
		t.Fatal("newest legacy attempt did not receive the active key")
	}
	var older FirmwareUpgradeLog
	if err := testTDB.UpgradeLogs().FindOne(ctx, bson.M{"_id": olderID}).Decode(&older); err != nil {
		t.Fatal(err)
	}
	if older.Status != "failed" || older.CompletedAt.IsZero() {
		t.Fatalf("older duplicate was not terminalized: status=%q completed_at=%v", older.Status, older.CompletedAt)
	}
}

func TestIncrementRetryAndResetStatus(t *testing.T) {
	fwID := primitive.NewObjectID()
	sn := fmt.Sprintf("SN-RETRY-%d", time.Now().UnixNano())
	log1 := FirmwareUpgradeLog{DeviceSN: sn, FirmwareID: fwID, Status: "failed", RetryCount: 0, TriggeredAt: time.Now()}
	created, err := testTDB.CreateUpgradeLog(context.Background(), log1)
	if err != nil {
		t.Fatal(err)
	}
	if err := testTDB.IncrementRetryAndResetStatus(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	got, err := testTDB.GetLatestUpgradeLog(context.Background(), sn, fwID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RetryCount != 1 {
		t.Errorf("Expected retry_count=1, got %d", got.RetryCount)
	}
	if got.Status != "pending" {
		t.Errorf("Expected status=pending, got %s", got.Status)
	}
}

// --- FW Policy ---

func TestGetDeviceFWPolicy_NoDocument_ReturnsDefault(t *testing.T) {
	policy, err := testTDB.GetDeviceFWPolicy(context.Background(), "nonexistent-device")
	if err != nil {
		t.Fatal(err)
	}
	if policy.Policy != "campaign" {
		t.Errorf("Expected default policy 'campaign', got '%s'", policy.Policy)
	}
}

func TestUpsertDeviceFWPolicy(t *testing.T) {
	sn := fmt.Sprintf("SN-POLICY-%d", time.Now().UnixNano())
	p := DeviceFWPolicy{DeviceSN: sn, Policy: "skip"}
	if err := testTDB.SetDeviceFWPolicy(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got, err := testTDB.GetDeviceFWPolicy(context.Background(), sn)
	if err != nil {
		t.Fatal(err)
	}
	if got.Policy != "skip" {
		t.Errorf("Expected policy=skip, got %s", got.Policy)
	}

	// Upsert again
	p.Policy = "manual"
	if err := testTDB.SetDeviceFWPolicy(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got, err = testTDB.GetDeviceFWPolicy(context.Background(), sn)
	if err != nil {
		t.Fatal(err)
	}
	if got.Policy != "manual" {
		t.Errorf("Expected policy=manual after upsert, got %s", got.Policy)
	}
}

// --- Device Info Cache ---

func TestCachedDeviceInfo_UpsertAndGet(t *testing.T) {
	sn := fmt.Sprintf("SN-CACHE-%d", time.Now().UnixNano())
	info := []byte(`{"Manufacturer":"Test","ModelName":"M1"}`)
	if err := testTDB.UpsertDeviceInfo(context.Background(), sn, info); err != nil {
		t.Fatal(err)
	}
	got, err := testTDB.GetCachedDeviceInfo(context.Background(), sn)
	if err != nil {
		t.Fatal(err)
	}
	if got.InfoRaw != string(info) {
		t.Errorf("Info mismatch: %s", got.InfoRaw)
	}
}

// --- Metrics ---

func TestStoreAndGetMetrics(t *testing.T) {
	m := DeviceMetrics{
		DeviceSerial: fmt.Sprintf("SN-METRICS-%d", time.Now().UnixNano()),
		Timestamp:    time.Now(),
	}
	if err := testTDB.StoreDeviceMetrics(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	since := time.Now().Add(-1 * time.Minute)
	history, err := testTDB.GetDeviceMetricsHistory(context.Background(), m.DeviceSerial, since)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) == 0 {
		t.Error("Expected at least 1 metric entry")
	}
}

// --- User (testing the log.Fatal issue) ---
// NOTE: FindAllUsers calls log.Fatal on cursor.All error.
// We cannot test this without refactoring -- log.Fatal calls os.Exit.
// This test documents the expected behavior after fix.
func TestFindAllUsers_ReturnsUsers(t *testing.T) {
	// Register a user first
	u := User{Email: fmt.Sprintf("test-%d@test.com", time.Now().UnixNano()), Level: TenantAdmin}
	u.Password = "testpassword123"
	if err := testDB.RegisterUser(u); err != nil {
		t.Fatal(err)
	}
	users, err := testDB.FindAllUsers()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) == 0 {
		t.Error("Expected at least 1 user")
	}
}

// --- Template CRUD ---

func TestAddAndFindTemplate(t *testing.T) {
	name := fmt.Sprintf("tmpl-%d", time.Now().UnixNano())
	ctx := context.Background()
	if err := testTDB.AddTemplate(ctx, name, "tr-181", "template-content"); err != nil {
		t.Fatal(err)
	}
	got, err := testTDB.FindTemplate(ctx, map[string]string{"name": name})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != name {
		t.Errorf("Name mismatch: %s", got.Name)
	}
}

// --- device_info cache retention ---

func TestDeviceInfoHasUpdatedAtTTL(t *testing.T) {
	ctx := context.Background()
	cursor, err := testTDB.DeviceInfo().Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var indexes []bson.M
	if err := cursor.All(ctx, &indexes); err != nil {
		t.Fatal(err)
	}
	for _, idx := range indexes {
		key, _ := idx["key"].(bson.M)
		if _, ok := key["updated_at"]; ok {
			if _, hasTTL := idx["expireAfterSeconds"]; !hasTTL {
				t.Fatalf("updated_at index is not a TTL index: %#v", idx)
			}
			return
		}
	}
	t.Fatal("device_info must have a TTL index on updated_at")
}
