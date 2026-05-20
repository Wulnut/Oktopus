package api

import (
	"context"
	"log"
	"time"

	"github.com/leandrofars/oktopus/internal/db"
)

const (
	minCampaignSchedulerInterval = 30 * time.Second
	schedulerTickTimeout         = 2 * time.Minute
)

// StartCampaignScheduler runs periodic checks for enabled campaigns with a time window
// and triggers RunCampaignBatch once per window occurrence.
func (a *Api) StartCampaignScheduler() {
	if !a.campaignSchedulerEnabled {
		log.Printf("campaign_scheduler: disabled")
		return
	}

	interval := a.campaignSchedulerInterval
	if interval < minCampaignSchedulerInterval {
		interval = minCampaignSchedulerInterval
	}

	log.Printf("campaign_scheduler: started (interval=%s)", interval)

	go func() {
		a.runCampaignSchedulerOnce()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			a.runCampaignSchedulerOnce()
		}
	}()
}

func (a *Api) runCampaignSchedulerOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), schedulerTickTimeout)
	defer cancel()

	tenants, err := a.db.FindAllTenants(ctx)
	if err != nil {
		log.Printf("campaign_scheduler: list tenants: %v", err)
		return
	}

	now := campaignNowUTC()
	for _, tenant := range tenants {
		if tenant.Slug == "" || tenant.Status != db.TenantStatusActive {
			continue
		}
		a.runCampaignSchedulerForTenant(ctx, tenant.Slug, now)
	}
}

func (a *Api) runCampaignSchedulerForTenant(ctx context.Context, tenantSlug string, now time.Time) {
	tdb := a.db.ForTenant(tenantSlug)
	campaigns, err := tdb.ListCampaigns(ctx)
	if err != nil {
		log.Printf("campaign_scheduler: tenant %s list campaigns: %v", tenantSlug, err)
		return
	}

	for _, campaign := range campaigns {
		if !campaign.Enabled || !campaignHasTimeWindow(campaign) {
			continue
		}

		windowKey := campaignWindowOccurrenceKey(campaign, now)
		if windowKey == "" {
			continue
		}

		// Skip if this window has already been processed (success / failed / skipped).
		if campaign.LastScheduledWindowKey == windowKey && db.IsScheduledBatchTerminal(campaign.ScheduledBatchStatus) {
			continue
		}

		acquired, err := tdb.TryAcquireScheduledBatch(ctx, campaign.ID, windowKey, now)
		if err != nil {
			log.Printf("campaign_scheduler: tenant %s campaign %s acquire: %v", tenantSlug, campaign.ID.Hex(), err)
			continue
		}
		if !acquired {
			continue
		}

		log.Printf("campaign_scheduler: tenant %s campaign %s starting scheduled batch (window=%s)", tenantSlug, campaign.ID.Hex(), windowKey)

		go func(c db.Campaign, slug, key string) {
			tdb := a.db.ForTenant(slug)
			result := a.runCampaignBatchLocked(tdb, c.ID, slug, "campaign_scheduled")

			status := db.ScheduledBatchSuccess
			switch {
			case result.Err != nil:
				status = db.ScheduledBatchFailed
			case result.Skipped:
				status = db.ScheduledBatchSkipped
			}

			completeCtx, completeCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer completeCancel()
			if err := tdb.CompleteScheduledBatch(completeCtx, c.ID, key, status, campaignNowUTC()); err != nil {
				log.Printf("campaign_scheduler: tenant %s campaign %s complete: %v", slug, c.ID.Hex(), err)
			}
			if status != db.ScheduledBatchSuccess {
				log.Printf("campaign_scheduler: tenant %s campaign %s batch finished status=%s (window=%s err=%v)",
					slug, c.ID.Hex(), status, key, result.Err)
			}
		}(campaign, tenantSlug, windowKey)
	}
}
