package api

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/leandrofars/oktopus/internal/config"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/entity"
)

const minLockIPPollInterval = 30 * time.Second
const defaultLockNotifyHealth = 120 * time.Second

// shouldSkipIPPollForNotifyHealth is true when NotifyOKAt is recent enough that
// the poller should defer to the Notify path (Task 6 sets NotifyOKAt).
func shouldSkipIPPollForNotifyHealth(notifyOKAt, now time.Time, health time.Duration) bool {
	if health <= 0 || notifyOKAt.IsZero() {
		return false
	}
	return !notifyOKAt.After(now) && now.Sub(notifyOKAt) < health
}

// shouldSkipIPPollForSameIP is true when Redis already recorded this WAN IP.
func shouldSkipIPPollForSameIP(lastIP, currentIP string) bool {
	return currentIP != "" && lastIP == currentIP
}

func hasPendingLockBackoff(enabled bool, state lockDeviceState) bool {
	return enabled && len(state.CommandBackoffs) > 0
}

// shouldSkipIPPollForUnsupported skips any SN with a lock_unsupported_devices
// row (unsupported or opt_out) so those devices never enter evaluate from poll.
func shouldSkipIPPollForUnsupported(hasRow, _ bool) bool {
	return hasRow
}

// StartLockIPPoller periodically re-evaluates online devices when WAN IP changes
// or when a recorded command failure needs a cooldown/recovery probe.
// Disabled unless LOCK_IP_POLL_ENABLED is true. Interval is floored at 30s.
//
// Same-IP skip requires Redis LastIP. With noopLockDeviceStateStore (Redis off),
// Get always returns empty LastIP so same-IP never skips — every poll may evaluate.
func (a *Api) StartLockIPPoller(cfg config.LockScale) {
	if !cfg.IPPollEnabled {
		log.Printf("lock_ip_poller: disabled")
		return
	}

	interval := cfg.IPPollInterval
	if interval < minLockIPPollInterval {
		interval = minLockIPPollInterval
	}
	notifyHealth := cfg.NotifyHealth
	if notifyHealth <= 0 {
		notifyHealth = defaultLockNotifyHealth
	}

	log.Printf("lock_ip_poller: started (interval=%s notify_health=%s)", interval, notifyHealth)

	go func() {
		// Short delay so online/chase startup work settles before the first poll.
		time.Sleep(5 * time.Second)
		a.runLockIPPollRound(notifyHealth)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			a.runLockIPPollRound(notifyHealth)
		}
	}()
}

func (a *Api) runLockIPPollRound(notifyHealth time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	tenants, err := a.db.FindAllTenants(ctx)
	if err != nil {
		log.Printf("lock_ip_poller: list tenants: %v", err)
		return
	}

	var wg sync.WaitGroup
	for _, tenant := range tenants {
		if tenant.Slug == "" || tenant.Status != db.TenantStatusActive {
			continue
		}
		a.pollLockIPForTenant(tenant.Slug, notifyHealth, &wg)
	}
	wg.Wait()
}

func (a *Api) pollLockIPForTenant(tenantSlug string, notifyHealth time.Duration, wg *sync.WaitGroup) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	tdb := a.db.ForTenant(tenantSlug)
	cfg, err := tdb.GetLockConfig(ctx)
	if err != nil {
		log.Printf("lock_ip_poller: tenant %s get config: %v", tenantSlug, err)
		return
	}
	if !cfg.MasterEnabled {
		return
	}

	list, err := getDevicesNoHTTP(map[string]interface{}{"status": entity.Online}, a.nc, tenantSlug)
	if err != nil {
		log.Printf("lock_ip_poller: tenant %s list online devices: %v", tenantSlug, err)
		return
	}

	for _, device := range list.Devices {
		wg.Add(1)
		go func(d entity.Device) {
			defer wg.Done()
			a.pollLockIPForDevice(tdb, d, tenantSlug, notifyHealth)
		}(device)
	}
}

func (a *Api) pollLockIPForDevice(tdb *db.TenantDB, device entity.Device, tenantSlug string, notifyHealth time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Cheap skips before acquireLockSem so unsupported / healthy-notify devices
	// do not consume evaluate concurrency slots.
	hasRow, optOut := a.unsupportedLockRowState(ctx, tdb, device.SN)
	if shouldSkipIPPollForUnsupported(hasRow, optOut) {
		return
	}

	state, _, err := lockStateStore.Get(ctx, tenantSlug, device.SN)
	if err != nil {
		log.Printf("lock_ip_poller: tenant %s get state %s: %v", tenantSlug, device.SN, err)
		// Soft-degrade: continue without NotifyHealth / LastIP skip.
		state = lockDeviceState{}
	}
	hasBackoff := hasPendingLockBackoff(a.lockBackoff.Enabled, state)
	if !hasBackoff && shouldSkipIPPollForNotifyHealth(state.NotifyOKAt, time.Now(), notifyHealth) {
		return
	}

	a.acquireLockSem()
	defer a.releaseLockSem()

	ip := a.lockReportedIP(ctx, device, tenantSlug)
	if ip == "" {
		return
	}
	if !hasBackoff && shouldSkipIPPollForSameIP(state.LastIP, ip) {
		return
	}

	a.evaluateAndMaybeCommand(ctx, tdb, device, tenantSlug, lockTriggerIPChangePoll, ip)
}
