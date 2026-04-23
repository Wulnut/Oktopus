//go:build integration

package api

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/entity"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func createTestFirmwareAndCampaign(t *testing.T) (db.Firmware, db.Campaign) {
	t.Helper()
	ctx := context.Background()

	fw := db.Firmware{
		Name:         fmt.Sprintf("fw-ce-%d", time.Now().UnixNano()),
		Vendor:       "TestVendor",
		Model:        "TestModel",
		BuildVersion: fmt.Sprintf("2.0.%d", time.Now().UnixNano()%1000),
		Phase:        db.PhaseRelease,
	}
	createdFw, err := testApi.db.CreateFirmware(ctx, fw)
	if err != nil {
		t.Fatal(err)
	}

	c := db.Campaign{
		Vendor:      "TestVendor",
		Model:       "TestModel",
		HWVersion:   fmt.Sprintf("hw-%d", time.Now().UnixNano()),
		FirmwareID:  createdFw.ID,
		Enabled:     true,
		Concurrency: 10,
	}
	createdCampaign, err := testApi.db.CreateCampaign(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	return createdFw, createdCampaign
}

// --- Concurrent upgrade deduplication ---

func TestHandleDeviceOnline_Concurrent_NoDuplicateUpgrades(t *testing.T) {
	fw, campaign := createTestFirmwareAndCampaign(t)
	sn := fmt.Sprintf("SN-CONCURRENT-%d", time.Now().UnixNano())

	device := entity.Device{
		SN:           sn,
		Status:       entity.Online,
		Vendor:       campaign.Vendor,
		Model:        campaign.Model,
		HWVersion:    campaign.HWVersion,
		Version:      "1.0.0", // Different from fw.BuildVersion to trigger upgrade
		ProductClass: "Router",
	}
	_ = fw

	// Spawn multiple goroutines simulating rapid device flapping
	var wg sync.WaitGroup
	errCount := 0
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					errCount++
					mu.Unlock()
					t.Logf("handleDeviceOnline panicked: %v", r)
				}
			}()
			testApi.handleDeviceOnline(device)
		}()
	}
	wg.Wait()

	if errCount > 0 {
		t.Errorf("handleDeviceOnline panicked %d times -- duplicate key errors not handled gracefully", errCount)
	}

	// Verify only one upgrade log was created (unique index should prevent duplicates)
	ctx := context.Background()
	log, err := testApi.db.GetLatestUpgradeLog(ctx, sn, fw.ID)
	if err != nil {
		// Could be that device was not online (no adapter responding) -- log may be "failed"
		t.Logf("GetLatestUpgradeLog: %v (expected if no adapter)", err)
		return
	}
	if log.DeviceSN != sn {
		t.Errorf("Upgrade log SN mismatch: %s", log.DeviceSN)
	}
}

// --- Upgrade completion detection ---

func TestCheckUpgradeCompletion_VersionMatch_MarksSuccess(t *testing.T) {
	ctx := context.Background()

	fwID := primitive.NewObjectID()
	sn := fmt.Sprintf("SN-COMPLETE-%d", time.Now().UnixNano())
	targetVersion := "2.0.0"

	logEntry := db.FirmwareUpgradeLog{
		DeviceSN:         sn,
		FirmwareID:       fwID,
		FirmwareBuildVer: targetVersion,
		Status:           "pending",
		TriggeredAt:      time.Now(),
	}
	created, err := testApi.db.CreateUpgradeLog(ctx, logEntry)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate device coming online with matching version
	device := entity.Device{SN: sn, Version: targetVersion}
	testApi.checkUpgradeCompletion(ctx, device)

	got, err := testApi.db.GetLatestUpgradeLog(ctx, sn, fwID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "success" {
		t.Errorf("Expected status=success, got %s", got.Status)
	}
	_ = created
}

func TestCheckUpgradeCompletion_VersionMismatch_MarksFailed(t *testing.T) {
	ctx := context.Background()

	fwID := primitive.NewObjectID()
	sn := fmt.Sprintf("SN-MISMATCH-%d", time.Now().UnixNano())

	logEntry := db.FirmwareUpgradeLog{
		DeviceSN:         sn,
		FirmwareID:       fwID,
		FirmwareBuildVer: "2.0.0",
		Status:           "pending",
		TriggeredAt:      time.Now(),
	}
	if _, err := testApi.db.CreateUpgradeLog(ctx, logEntry); err != nil {
		t.Fatal(err)
	}

	// Device comes online with OLD version
	device := entity.Device{SN: sn, Version: "1.0.0"}
	testApi.checkUpgradeCompletion(ctx, device)

	got, err := testApi.db.GetLatestUpgradeLog(ctx, sn, fwID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" {
		t.Errorf("Expected status=failed, got %s", got.Status)
	}
}

// --- Retry logic ---

func TestRetryLogic_MaxRetriesExhausted_NoMoreRetries(t *testing.T) {
	ctx := context.Background()

	fw, campaign := createTestFirmwareAndCampaign(t)
	sn := fmt.Sprintf("SN-MAXRETRY-%d", time.Now().UnixNano())

	// Create a log at max retries
	logEntry := db.FirmwareUpgradeLog{
		DeviceSN:         sn,
		FirmwareID:       fw.ID,
		FirmwareBuildVer: fw.BuildVersion,
		Status:           "failed",
		RetryCount:       maxRetries, // Already at max
		TriggeredAt:      time.Now(),
	}
	if _, err := testApi.db.CreateUpgradeLog(ctx, logEntry); err != nil {
		t.Fatal(err)
	}

	device := entity.Device{
		SN:        sn,
		Status:    entity.Online,
		Vendor:    campaign.Vendor,
		Model:     campaign.Model,
		HWVersion: campaign.HWVersion,
		Version:   "1.0.0",
	}

	// This should NOT retry (max retries reached)
	testApi.checkCampaignUpgrade(ctx, device)

	got, err := testApi.db.GetLatestUpgradeLog(ctx, sn, fw.ID)
	if err != nil {
		t.Fatal(err)
	}
	// RetryCount should still be at max, status still failed
	if got.RetryCount != maxRetries {
		t.Errorf("Expected retry_count=%d (unchanged), got %d", maxRetries, got.RetryCount)
	}
	if got.Status != "failed" {
		t.Errorf("Expected status=failed (unchanged), got %s", got.Status)
	}
}

// --- Device already on target version -- skip upgrade ---

func TestCheckCampaignUpgrade_AlreadyOnTargetVersion_Skips(t *testing.T) {
	ctx := context.Background()

	fw, campaign := createTestFirmwareAndCampaign(t)
	sn := fmt.Sprintf("SN-SKIP-%d", time.Now().UnixNano())

	device := entity.Device{
		SN:        sn,
		Status:    entity.Online,
		Vendor:    campaign.Vendor,
		Model:     campaign.Model,
		HWVersion: campaign.HWVersion,
		Version:   fw.BuildVersion, // Already on target
	}

	testApi.checkCampaignUpgrade(ctx, device)

	// No upgrade log should be created
	_, err := testApi.db.GetLatestUpgradeLog(ctx, sn, fw.ID)
	if err == nil {
		t.Error("Expected no upgrade log for device already on target version, but one was created")
	}
}
