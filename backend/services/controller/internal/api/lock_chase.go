package api

import (
	"context"
	"log"
	"time"

	"github.com/leandrofars/oktopus/internal/bridge"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/entity"
	local "github.com/leandrofars/oktopus/internal/nats"
)

func (a *Api) chaseLockForSN(tenantSlug, sn string) {
	go func() {
		a.acquireLockSem()
		defer a.releaseLockSem()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		device, online := a.getOnlineDeviceNoWrite(ctx, tenantSlug, sn)
		if !online {
			return
		}
		tdb := a.db.ForTenant(tenantSlug)
		a.evaluateAndMaybeCommand(ctx, tdb, device, tenantSlug, lockTriggerChase, "")
	}()
}

func (a *Api) chaseLockForSNs(tenantSlug string, sns []string) {
	seen := map[string]struct{}{}
	for _, sn := range sns {
		normalized := db.NormalizeSN(sn)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		a.chaseLockForSN(tenantSlug, normalized)
	}
}

func (a *Api) getOnlineDeviceNoWrite(ctx context.Context, tenantSlug, sn string) (entity.Device, bool) {
	msg, err := bridge.NatsReqWithoutHttpSet[entity.Device](
		local.NatsAdapterSubject(tenantSlug)+sn+".device",
		[]byte(""),
		a.nc,
	)
	if err != nil || msg == nil {
		return entity.Device{}, false
	}
	if msg.Msg.Status != entity.Online {
		return entity.Device{}, false
	}
	return msg.Msg, true
}

func (a *Api) chaseLockAfterPolicyDelete(tenantSlug, sn string) {
	a.chaseLockForSN(tenantSlug, sn)
}

// chaseLockAfterConfigUpdate re-evaluates all currently online devices for a
// tenant. Used after whitelist batch import (scenario 1) and config changes.
// Each device evaluation runs concurrently with semaphore control so that a
// large online fleet does not starve the DB connection pool.
func (a *Api) chaseLockAfterConfigUpdate(tenantSlug string) {
	list, err := getDevicesNoHTTP(map[string]interface{}{"status": entity.Online}, a.nc, tenantSlug)
	if err != nil {
		log.Printf("lock_chase: list online devices tenant %s: %v", tenantSlug, err)
		return
	}
	tdb := a.db.ForTenant(tenantSlug)
	for _, device := range list.Devices {
		go func(dev entity.Device) {
			a.acquireLockSem()
			defer a.releaseLockSem()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			a.evaluateAndMaybeCommand(ctx, tdb, dev, tenantSlug, lockTriggerChase, "")
		}(device)
	}
}
