package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/leandrofars/oktopus/internal/bridge"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/entity"
	local "github.com/leandrofars/oktopus/internal/nats"
	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const maxRetries = 3

// onConnectSem limits concurrent handleDeviceOnline goroutines.
var onConnectSem = make(chan struct{}, 50)

// StartCampaignEngine subscribes to device.v1.*.online events and runs campaign checks.
// The wildcard matches the tenant slug in the subject (device.v1.<tenant>.online).
func (a *Api) StartCampaignEngine() {
	sub, err := a.nc.Subscribe("device.v1.*.online", func(msg *nats.Msg) {
		// Extract tenant slug from subject: device.v1.<tenant>.online
		parts := strings.Split(msg.Subject, ".")
		if len(parts) < 4 || parts[2] == "" {
			log.Printf("campaign_engine: invalid subject format, skipping: %s", msg.Subject)
			return
		}
		tenantSlug := parts[2]
		tdb := a.db.ForTenant(tenantSlug)

		var device entity.Device
		if err := json.Unmarshal(msg.Data, &device); err != nil {
			log.Printf("campaign_engine: failed to unmarshal device event: %v", err)
			return
		}
		select {
		case onConnectSem <- struct{}{}:
			go func() {
				defer func() { <-onConnectSem }()
				a.handleDeviceOnline(tdb, device, tenantSlug)
			}()
		default:
			log.Printf("campaign_engine: too many concurrent checks, skipping device %s", device.SN)
		}
	})
	if err != nil {
		log.Printf("campaign_engine: failed to subscribe to device.v1.*.online: %v", err)
	} else {
		log.Printf("campaign_engine: subscribed to device.v1.*.online (sub=%s)", sub.Subject)
	}
}

// handleDeviceOnline checks for pending upgrade completions and new campaign upgrades.
func (a *Api) handleDeviceOnline(tdb *db.TenantDB, device entity.Device, tenantSlug string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Check for completion of a previous upgrade
	a.checkUpgradeCompletion(ctx, tdb, device)

	// Step 2: Check if a new upgrade is needed
	a.checkCampaignUpgrade(ctx, tdb, device, tenantSlug)
}

// checkUpgradeCompletion verifies if a pending/downloading upgrade succeeded after reboot.
func (a *Api) checkUpgradeCompletion(ctx context.Context, tdb *db.TenantDB, device entity.Device) {
	pendingLog, err := tdb.GetPendingUpgradeLog(ctx, device.SN)
	if err != nil {
		if err != mongo.ErrNoDocuments {
			log.Printf("campaign_engine: failed to get pending log for %s: %v", device.SN, err)
		}
		return
	}

	if device.Version == pendingLog.FirmwareBuildVer {
		tdb.UpdateUpgradeLogStatus(ctx, pendingLog.ID, "success", "")
		log.Printf("campaign_engine: device %s upgraded successfully to %s", device.SN, device.Version)
	} else {
		errMsg := fmt.Sprintf("version mismatch after upgrade: expected %s, got %s", pendingLog.FirmwareBuildVer, device.Version)
		tdb.UpdateUpgradeLogStatus(ctx, pendingLog.ID, "failed", errMsg)
		log.Printf("campaign_engine: device %s upgrade failed: %s", device.SN, errMsg)
	}
}

// checkCampaignUpgrade checks if a device needs a firmware upgrade based on its policy and campaign.
func (a *Api) checkCampaignUpgrade(ctx context.Context, tdb *db.TenantDB, device entity.Device, tenantSlug string) {
	policy, err := tdb.GetDeviceFWPolicy(ctx, device.SN)
	if err != nil {
		log.Printf("campaign_engine: failed to get FW policy for %s: %v", device.SN, err)
		return
	}

	switch policy.Policy {
	case "skip":
		return
	case "manual":
		a.handleManualPolicy(ctx, tdb, device, policy, tenantSlug)
	default: // "campaign"
		a.handleCampaignPolicy(ctx, tdb, device, tenantSlug)
	}
}

func (a *Api) handleManualPolicy(ctx context.Context, tdb *db.TenantDB, device entity.Device, policy db.DeviceFWPolicy, tenantSlug string) {
	if policy.ManualFirmwareID.IsZero() {
		return
	}

	fw, err := tdb.GetFirmware(ctx, policy.ManualFirmwareID)
	if err != nil {
		return
	}

	// Skip if device is already on the target version
	if device.Version == fw.BuildVersion {
		return
	}

	existingLog, err := tdb.GetLatestUpgradeLog(ctx, device.SN, fw.ID)
	if err == nil {
		switch existingLog.Status {
		case "pending", "downloading":
			if time.Since(existingLog.TriggeredAt) > 15*time.Minute {
				tdb.UpdateUpgradeLogStatus(ctx, existingLog.ID, "failed", "timed out after 15 minutes")
			} else {
				return
			}
		}
		// failed or expired — create a new attempt
	}

	a.triggerUpgrade(ctx, tdb, device, fw, primitive.NilObjectID, "manual", tenantSlug)
}

func (a *Api) handleCampaignPolicy(ctx context.Context, tdb *db.TenantDB, device entity.Device, tenantSlug string) {
	campaign, err := tdb.GetCampaignByHardware(ctx, device.Vendor, device.Model, device.HWVersion)
	if err != nil {
		return // No campaign for this hardware
	}

	if !campaign.Enabled {
		return
	}

	if !isWithinTimeWindow(campaign) {
		return
	}

	fw, err := tdb.GetFirmware(ctx, campaign.FirmwareID)
	if err != nil {
		log.Printf("campaign_engine: firmware %s not found for campaign %s: %v", campaign.FirmwareID.Hex(), campaign.ID.Hex(), err)
		return
	}

	// Skip if device already on this version
	if device.Version == fw.BuildVersion {
		return
	}

	existingLog, err := tdb.GetLatestUpgradeLog(ctx, device.SN, fw.ID)
	if err == nil {
		switch existingLog.Status {
		case "success":
			return
		case "pending", "downloading":
			if time.Since(existingLog.TriggeredAt) <= 15*time.Minute {
				return
			}
			tdb.UpdateUpgradeLogStatus(ctx, existingLog.ID, "failed", "timed out after 15 minutes")
		case "failed":
			if existingLog.RetryCount >= maxRetries {
				return
			}
		}
		// Create a new attempt
	}

	a.triggerUpgrade(ctx, tdb, device, fw, campaign.ID, "on_connect", tenantSlug)
}

func (a *Api) triggerUpgrade(ctx context.Context, tdb *db.TenantDB, device entity.Device, fw db.Firmware, campaignID primitive.ObjectID, triggerType, tenantSlug string) {
	logEntry := db.FirmwareUpgradeLog{
		DeviceSN:         device.SN,
		DeviceAlias:      device.Alias,
		FirmwareID:       fw.ID,
		FirmwareName:     fw.Name,
		FirmwareBuildVer: fw.BuildVersion,
		PreviousVersion:  device.Version,
		TriggerType:      triggerType,
		Status:           "pending",
	}
	if !campaignID.IsZero() {
		logEntry.CampaignID = campaignID
	}

	created, err := tdb.CreateUpgradeLog(ctx, logEntry)
	if err != nil {
		log.Printf("campaign_engine: failed to create upgrade log for %s: %v", device.SN, err)
		return
	}

	a.executeFirmwareUpgrade(tdb, device.SN, fw, created.ID, tenantSlug)
}

func (a *Api) executeFirmwareUpgrade(tdb *db.TenantDB, sn string, fw db.Firmware, logID primitive.ObjectID, tenantSlug string) {
	mtp, online := deviceStateOKNoWrite(a.nc, sn, tenantSlug)
	if !online {
		log.Printf("campaign_engine: device %s not online, skipping upgrade", sn)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tdb.UpdateUpgradeLogStatus(ctx, logID, "failed", "device not online")
		return
	}

	log.Printf("campaign_engine: triggering firmware upgrade for device %s to %s v%s", sn, fw.Name, fw.BuildVersion)

	fwErr := performFirmwareUpdate(sn, mtp, fw, a.nc, tenantSlug)
	if fwErr != nil {
		log.Printf("campaign_engine: firmware upgrade failed for %s: %v", sn, fwErr)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tdb.UpdateUpgradeLogStatus(ctx, logID, "failed", fwErr.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tdb.UpdateUpgradeLogStatus(ctx, logID, "downloading", "")
}

// RunCampaignBatch scans online devices matching a campaign and triggers upgrades.
func (a *Api) RunCampaignBatch(tdb *db.TenantDB, campaign db.Campaign, tenantSlug string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fw, err := tdb.GetFirmware(ctx, campaign.FirmwareID)
	if err != nil {
		log.Printf("campaign_engine: firmware not found for campaign %s: %v", campaign.ID.Hex(), err)
		return
	}

	devices, err := a.getMatchingOnlineDevices(campaign, tenantSlug)
	if err != nil {
		log.Printf("campaign_engine: failed to query devices for campaign %s: %v", campaign.ID.Hex(), err)
		return
	}

	var targets []entity.Device
	for _, d := range devices {
		policy, err := tdb.GetDeviceFWPolicy(ctx, d.SN)
		if err != nil {
			continue
		}
		if policy.Policy == "skip" {
			continue
		}
		if d.Version == fw.BuildVersion {
			continue
		}

		existingLog, err := tdb.GetLatestUpgradeLog(ctx, d.SN, fw.ID)
		if err == nil && (existingLog.Status == "success" || existingLog.Status == "pending" || existingLog.Status == "downloading") {
			continue
		}

		targets = append(targets, d)
	}

	if len(targets) == 0 {
		log.Printf("campaign_engine: no eligible devices for campaign %s", campaign.ID.Hex())
		return
	}

	concurrency := campaign.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}
	if concurrency > 50 {
		concurrency = 50
	}

	log.Printf("campaign_engine: starting batch upgrade for campaign %s (%d devices, concurrency %d)", campaign.ID.Hex(), len(targets), concurrency)

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, d := range targets {
		wg.Add(1)
		sem <- struct{}{}

		go func(device entity.Device) {
			defer wg.Done()
			defer func() { <-sem }()
			deviceCtx, deviceCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer deviceCancel()
			a.triggerUpgrade(deviceCtx, tdb, device, fw, campaign.ID, "campaign_start", tenantSlug)
		}(d)
	}

	wg.Wait()
	log.Printf("campaign_engine: batch upgrade completed for campaign %s", campaign.ID.Hex())
}

func (a *Api) getMatchingOnlineDevices(campaign db.Campaign, tenantSlug string) ([]entity.Device, error) {
	msg, err := bridge.NatsReqWithoutHttpSet[entity.DevicesList](
		local.NatsAdapterSubject(tenantSlug)+"devices",
		[]byte(""),
		a.nc,
	)
	if err != nil || msg == nil {
		return nil, fmt.Errorf("failed to query devices from adapter: %v", err)
	}

	var matched []entity.Device
	for _, d := range msg.Msg.Devices {
		if d.Status != entity.Online {
			continue
		}
		if d.Vendor == campaign.Vendor && d.Model == campaign.Model && d.HWVersion == campaign.HWVersion {
			matched = append(matched, d)
		}
	}
	return matched, nil
}

func isWithinTimeWindow(campaign db.Campaign) bool {
	if campaign.TimeWindowStart == "" || campaign.TimeWindowEnd == "" {
		return true
	}

	now := time.Now().UTC()
	currentMinutes := now.Hour()*60 + now.Minute()

	startMinutes, err := parseTimeHHMM(campaign.TimeWindowStart)
	if err != nil {
		return true
	}
	endMinutes, err := parseTimeHHMM(campaign.TimeWindowEnd)
	if err != nil {
		return true
	}

	if startMinutes <= endMinutes {
		return currentMinutes >= startMinutes && currentMinutes < endMinutes
	}
	return currentMinutes >= startMinutes || currentMinutes < endMinutes
}

func parseTimeHHMM(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}
