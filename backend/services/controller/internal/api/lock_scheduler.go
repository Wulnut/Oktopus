package api

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/leandrofars/oktopus/internal/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const minLockRetrySchedulerInterval = 15 * time.Second

func shouldMarkLockCommandForRetry(attemptCount, maxAttempts int) bool {
	if maxAttempts <= 0 {
		maxAttempts = db.DefaultLockCommandMaxAttempts
	}
	return attemptCount < maxAttempts
}

// retryCooldownMessage returns an explanatory failure message when a retry for
// target must be drained without being claimed. Redis failures soft-degrade to
// the existing retry behavior.
func (a *Api) retryCooldownMessage(ctx context.Context, tenantSlug, sn string, target db.DeviceLockStatus) (string, bool) {
	if !a.lockBackoff.Enabled {
		return "", false
	}
	state, found, err := lockStateStore.Get(ctx, tenantSlug, sn)
	if err != nil {
		log.Printf("lock_backoff: get state before retry %s: %v", sn, err)
		return "", false
	}
	if !found {
		return "", false
	}
	b := backoffFor(state.CommandBackoffs, target)
	if b == nil || !time.Now().Before(b.CooldownUntil) {
		return "", false
	}
	return fmt.Sprintf("suppressed by device cooldown until %s", b.CooldownUntil.Format(time.RFC3339)), true
}

// StartLockRetryScheduler retries lock commands marked for retry after command timeout.
func (a *Api) StartLockRetryScheduler() {
	if !a.lockRetryEnabled {
		log.Printf("lock_retry_scheduler: disabled")
		return
	}

	interval := a.lockRetryInterval
	if interval < minLockRetrySchedulerInterval {
		interval = minLockRetrySchedulerInterval
	}

	log.Printf("lock_retry_scheduler: started (interval=%s timeout=%s max_attempts=%d)",
		interval, a.lockCommandTimeout, a.lockMaxAttempts)

	go func() {
		a.runLockRetrySchedulerOnce()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			a.runLockRetrySchedulerOnce()
		}
	}()
}

func (a *Api) runLockRetrySchedulerOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	tenants, err := a.db.FindAllTenants(ctx)
	if err != nil {
		log.Printf("lock_retry_scheduler: list tenants: %v", err)
		return
	}

	retryBefore := time.Now().Add(-a.lockCommandTimeout)
	for _, tenant := range tenants {
		if tenant.Slug == "" || tenant.Status != db.TenantStatusActive {
			continue
		}
		a.runLockRetrySchedulerForTenant(ctx, tenant.Slug, retryBefore)
	}
}

func (a *Api) runLockRetrySchedulerForTenant(ctx context.Context, tenantSlug string, retryBefore time.Time) {
	tdb := a.db.ForTenant(tenantSlug)
	commands, err := tdb.ListRetryableLockCommands(ctx, retryBefore, 50)
	if err != nil {
		log.Printf("lock_retry_scheduler: tenant %s list retryable: %v", tenantSlug, err)
		return
	}
	for _, command := range commands {
		go a.retryLockCommand(tenantSlug, command)
	}
}

func (a *Api) retryLockCommand(tenantSlug string, command db.LockCommandAttempt) {
	a.acquireLockSem()
	defer a.releaseLockSem()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tdb := a.db.ForTenant(tenantSlug)
	if !shouldMarkLockCommandForRetry(command.AttemptCount, a.lockMaxAttempts) {
		_ = tdb.UpdateLockCommandStatus(ctx, command.ID, db.LockCommandFailed, command.Error)
		return
	}

	device, online := a.getOnlineDeviceNoWrite(ctx, tenantSlug, command.DeviceSN)
	if !online {
		return
	}

	unlock, ok, err := lockStateStore.TryLock(ctx, tenantSlug, command.DeviceSN, 0)
	if err != nil {
		log.Printf("lock_retry_scheduler: tenant %s try lock %s: %v", tenantSlug, command.DeviceSN, err)
	}
	if !ok {
		return
	}
	defer unlock()

	if _, proceed := a.gateLockCapability(ctx, tdb, device, tenantSlug, lockTriggerChase); !proceed {
		return
	}

	decision, reportedIP, err := a.resolveCurrentLockDecision(ctx, tdb, device, tenantSlug, "")
	if err != nil {
		log.Printf("lock_retry_scheduler: tenant %s resolve current decision %s: %v", tenantSlug, command.DeviceSN, err)
		return
	}
	if !decision.ShouldCommand {
		_ = tdb.UpdateLockCommandStatus(ctx, command.ID, db.LockCommandFailed, "retry superseded: current policy requires no command")
		return
	}
	if a.suppressLockIfBreakerTripped(ctx, tdb, tenantSlug, command.DeviceSN, decision.Status) {
		return
	}
	if msg, suppress := a.retryCooldownMessage(ctx, tenantSlug, command.DeviceSN, decision.Status); suppress {
		_ = tdb.UpdateLockCommandStatus(ctx, command.ID, db.LockCommandFailed, msg)
		return
	}

	attempt, err := tdb.PrepareLockCommandResend(ctx, command.ID, command.UpdatedAt, reportedIP, decision.Status, decision.CommandValue)
	if err != nil {
		// mongo.ErrNoDocuments means another controller already claimed it.
		if err != mongo.ErrNoDocuments {
			log.Printf("lock_retry_scheduler: tenant %s prepare retry %s: %v", tenantSlug, command.ID.Hex(), err)
		}
		return
	}

	a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
		SN:     attempt.DeviceSN,
		Action: "retry_re_evaluated",
		Status: decision.Status,
		Details: bson.M{
			"previous_status": command.TargetStatus,
			"reported_ip":     reportedIP,
			"attempt_count":   attempt.AttemptCount,
		},
	})

	if err := a.deliverLockCommand(ctx, tdb, attempt, decision, tenantSlug); err != nil {
		log.Printf("lock_retry_scheduler: tenant %s retry %s: %v", tenantSlug, attempt.DeviceSN, err)
		return
	}

	state, found, stateErr := lockStateStore.Get(ctx, tenantSlug, attempt.DeviceSN)
	if stateErr != nil || !found {
		state = lockDeviceState{}
	}
	state = successfulLockDeviceState(state, reportedIP, decision.Status, decision.CommandValue, time.Now())
	if err := lockStateStore.Put(ctx, tenantSlug, attempt.DeviceSN, state); err != nil {
		log.Printf("lock_retry_scheduler: tenant %s put state %s: %v", tenantSlug, attempt.DeviceSN, err)
	}
}
