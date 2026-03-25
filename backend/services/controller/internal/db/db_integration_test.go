//go:build integration

package db

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

var testDB Database

func TestMain(m *testing.M) {
	mongoURI := os.Getenv("MONGO_TEST_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27018"
	}

	ctx := context.Background()
	dbName := fmt.Sprintf("test_%d", time.Now().UnixNano())

	// Temporarily override NewDatabase to use a test DB name
	// Since NewDatabase uses hardcoded DB names, we connect directly
	testDB = NewDatabase(ctx, mongoURI)

	code := m.Run()

	// Cleanup: drop test databases
	testDB.client.Database("account-mngr").Drop(ctx)
	testDB.client.Database("general").Drop(ctx)
	testDB.client.Database("usp").Drop(ctx)
	_ = dbName

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
	created, err := testDB.CreateFirmware(context.Background(), fw)
	if err != nil {
		t.Fatalf("CreateFirmware failed: %v", err)
	}
	if created.ID.IsZero() {
		t.Error("Created firmware has zero ID")
	}

	got, err := testDB.GetFirmware(context.Background(), created.ID)
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
		fw := Firmware{Name: fmt.Sprintf("fw-list-%d-%d", time.Now().UnixNano(), i), BuildVersion: "1.0"}
		if _, err := testDB.CreateFirmware(context.Background(), fw); err != nil {
			t.Fatal(err)
		}
	}
	list, err := testDB.ListFirmware(context.Background())
	if err != nil {
		t.Fatalf("ListFirmware failed: %v", err)
	}
	if len(list) < 3 {
		t.Errorf("Expected at least 3 firmware entries, got %d", len(list))
	}
}

func TestUpdateFirmware_NonexistentID_Returns0Matched(t *testing.T) {
	matched, err := testDB.UpdateFirmware(context.Background(), primitive.NewObjectID(), Firmware{
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
	fw := Firmware{Name: fmt.Sprintf("fw-del-%d", time.Now().UnixNano()), BuildVersion: "1.0"}
	created, err := testDB.CreateFirmware(context.Background(), fw)
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.DeleteFirmware(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	_, err = testDB.GetFirmware(context.Background(), created.ID)
	if err == nil {
		t.Error("Expected error getting deleted firmware, got nil")
	}
}

// --- Firmware deletion does NOT cascade to campaigns (bug) ---

func TestDeleteFirmware_CampaignsNotCleaned(t *testing.T) {
	fw := Firmware{Name: fmt.Sprintf("fw-cascade-%d", time.Now().UnixNano()), BuildVersion: "2.0"}
	created, err := testDB.CreateFirmware(context.Background(), fw)
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
	createdCampaign, err := testDB.CreateCampaign(context.Background(), campaign)
	if err != nil {
		t.Fatal(err)
	}

	// Delete firmware
	if err := testDB.DeleteFirmware(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}

	// Campaign should be disabled or deleted -- but it won't be (known bug)
	got, err := testDB.GetCampaign(context.Background(), createdCampaign.ID)
	if err != nil {
		// Campaign was deleted -- that's one valid fix
		return
	}
	if got.Enabled {
		t.Error("BUG: Campaign still enabled after firmware deletion -- should be disabled or deleted")
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
	created, err := testDB.CreateCampaign(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := testDB.GetCampaign(context.Background(), created.ID)
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
	if _, err := testDB.CreateCampaign(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	// Same vendor+model+hw_version should fail
	_, err := testDB.CreateCampaign(context.Background(), c)
	if err == nil {
		t.Error("Expected duplicate key error, got nil")
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
	created, err := testDB.CreateUpgradeLog(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	err = testDB.UpdateUpgradeLogStatus(context.Background(), created.ID, "success", "")
	if err != nil {
		t.Fatal(err)
	}

	got, err := testDB.GetUpgradeLogByDeviceAndFirmware(context.Background(), log.DeviceSN, log.FirmwareID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "success" {
		t.Errorf("Expected status success, got %s", got.Status)
	}
}

func TestUpgradeLogUniqueIndex(t *testing.T) {
	fwID := primitive.NewObjectID()
	sn := fmt.Sprintf("SN-UNIQUE-%d", time.Now().UnixNano())
	log1 := FirmwareUpgradeLog{DeviceSN: sn, FirmwareID: fwID, Status: "pending", TriggeredAt: time.Now()}
	if _, err := testDB.CreateUpgradeLog(context.Background(), log1); err != nil {
		t.Fatal(err)
	}
	_, err := testDB.CreateUpgradeLog(context.Background(), log1)
	if err == nil {
		t.Error("Expected duplicate key error for same device_sn + firmware_id")
	}
}

func TestIncrementRetryAndResetStatus(t *testing.T) {
	fwID := primitive.NewObjectID()
	sn := fmt.Sprintf("SN-RETRY-%d", time.Now().UnixNano())
	log1 := FirmwareUpgradeLog{DeviceSN: sn, FirmwareID: fwID, Status: "failed", RetryCount: 0, TriggeredAt: time.Now()}
	created, err := testDB.CreateUpgradeLog(context.Background(), log1)
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.IncrementRetryAndResetStatus(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	got, err := testDB.GetUpgradeLogByDeviceAndFirmware(context.Background(), sn, fwID)
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
	policy, err := testDB.GetDeviceFWPolicy(context.Background(), "nonexistent-device")
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
	if err := testDB.SetDeviceFWPolicy(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got, err := testDB.GetDeviceFWPolicy(context.Background(), sn)
	if err != nil {
		t.Fatal(err)
	}
	if got.Policy != "skip" {
		t.Errorf("Expected policy=skip, got %s", got.Policy)
	}

	// Upsert again
	p.Policy = "manual"
	if err := testDB.SetDeviceFWPolicy(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got, err = testDB.GetDeviceFWPolicy(context.Background(), sn)
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
	if err := testDB.UpsertDeviceInfo(context.Background(), sn, info); err != nil {
		t.Fatal(err)
	}
	got, err := testDB.GetCachedDeviceInfo(context.Background(), sn)
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
	if err := testDB.StoreDeviceMetrics(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	since := time.Now().Add(-1 * time.Minute)
	history, err := testDB.GetDeviceMetricsHistory(context.Background(), m.DeviceSerial, since)
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
	u := User{Email: fmt.Sprintf("test-%d@test.com", time.Now().UnixNano()), Level: AdminUser}
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
	if err := testDB.AddTemplate(name, "tr-181", "template-content"); err != nil {
		t.Fatal(err)
	}
	got, err := testDB.FindTemplate(map[string]string{"name": name})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != name {
		t.Errorf("Name mismatch: %s", got.Name)
	}
}

// --- device_info has no TTL (documenting the issue) ---

func TestDeviceInfo_NoTTL(t *testing.T) {
	// This test documents that device_info has no TTL index.
	// After fix, this test should be updated to verify TTL exists.
	ctx := context.Background()
	cursor, err := testDB.deviceInfo.Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var indexes []map[string]interface{}
	if err := cursor.All(ctx, &indexes); err != nil {
		t.Fatal(err)
	}
	for _, idx := range indexes {
		if _, hasTTL := idx["expireAfterSeconds"]; hasTTL {
			// If this passes after fix, the TTL was added
			return
		}
	}
	t.Log("INFO: device_info collection has no TTL index -- cached data grows unboundedly")
}
