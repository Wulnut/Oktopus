package api

import (
	"context"
	"log"
	"time"

	"github.com/leandrofars/oktopus/internal/db"
)

const minLockRetrySchedulerInterval = 15 * time.Second

func shouldMarkLockCommandForRetry(attemptCount, maxAttempts int) bool {
	if maxAttempts <= 0 {
		maxAttempts = db.DefaultLockCommandMaxAttempts
	}
	return attemptCount < maxAttempts
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

	decision := LockDecision{
		Status:        command.TargetStatus,
		ShouldCommand: true,
		CommandValue:  command.CommandValue,
	}
	if a.suppressLockIfBreakerTripped(ctx, tdb, tenantSlug, command.DeviceSN, decision.Status) {
		return
	}

	attempt, err := tdb.PrepareLockCommandResend(ctx, command.ID)
	if err != nil {
		log.Printf("lock_retry_scheduler: tenant %s prepare retry %s: %v", tenantSlug, command.ID.Hex(), err)
		return
	}
	decision.Status = attempt.TargetStatus
	decision.CommandValue = attempt.CommandValue

	if err := a.deliverLockCommand(ctx, tdb, attempt, decision, tenantSlug); err != nil {
		log.Printf("lock_retry_scheduler: tenant %s retry %s: %v", tenantSlug, attempt.DeviceSN, err)
	}
}
