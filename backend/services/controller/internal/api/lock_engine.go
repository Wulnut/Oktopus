package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/leandrofars/oktopus/internal/config"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/entity"
	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	lockParameterPath = "Device.X_TELKOMSEL_OntLock.Lock"
	lockWanIPPath     = "Device.X_TELKOMSEL_OntLock.InternetWanIP"

	lockTriggerOnline         = "online"
	lockTriggerChase          = "chase"
	lockTriggerIPChangePoll   = "ip_change_poll"
	lockTriggerIPChangeNotify = "ip_change_notify"
)

// lockEngineConcurrency limits simultaneous device evaluations to protect
// the database connection pool during burst events (e.g. 10k devices online).
const lockEngineConcurrency = 50

type LockEvaluationInput struct {
	SN         string
	ReportedIP string
	Config     db.LockConfig
	Policy     *db.LockPolicy
}

type LockDecision struct {
	Status        db.DeviceLockStatus `json:"status"`
	Reason        db.LockReason       `json:"reason"`
	ShouldCommand bool                `json:"should_command"`
	CommandValue  string              `json:"command_value,omitempty"`
}

type lockEventSink interface {
	PublishLockAudit(ctx context.Context, tenantSlug string, log db.LockAuditLog) error
}

type lockPolicyCache interface {
	GetLockPolicy(ctx context.Context, tenantSlug, sn string) (db.LockPolicy, bool, error)
	PutLockPolicy(ctx context.Context, tenantSlug string, policy db.LockPolicy) error
	DeleteLockPolicy(ctx context.Context, tenantSlug, sn string) error
}

type noopLockEventSink struct{}

func (noopLockEventSink) PublishLockAudit(context.Context, string, db.LockAuditLog) error { return nil }

type noopLockPolicyCache struct{}

func (noopLockPolicyCache) GetLockPolicy(context.Context, string, string) (db.LockPolicy, bool, error) {
	return db.LockPolicy{}, false, nil
}
func (noopLockPolicyCache) PutLockPolicy(context.Context, string, db.LockPolicy) error { return nil }
func (noopLockPolicyCache) DeleteLockPolicy(context.Context, string, string) error     { return nil }

var lockAuditSink lockEventSink = noopLockEventSink{}
var lockCache lockPolicyCache = noopLockPolicyCache{}

func EvaluateLockDecision(input LockEvaluationInput) LockDecision {
	if !input.Config.MasterEnabled {
		return LockDecision{
			Status:        db.LockStatusUnlocked,
			Reason:        db.LockReasonMasterDisabled,
			ShouldCommand: true,
			CommandValue:  "0",
		}
	}

	if input.Policy != nil && input.Policy.Status && input.Policy.PolicyType == db.LockPolicyWhitelist {
		if ipInCIDR(input.ReportedIP, input.Policy.AllowedIPRange) {
			return LockDecision{
				Status:        db.LockStatusUnlocked,
				Reason:        db.LockReasonAuthorized,
				ShouldCommand: true,
				CommandValue:  "0",
			}
		}
	}

	if input.Config.AutoLockEnabled {
		return LockDecision{
			Status:        db.LockStatusLocked,
			Reason:        db.LockReasonUnauthorized,
			ShouldCommand: true,
			CommandValue:  "1",
		}
	}

	return LockDecision{
		Status: db.LockStatusPending,
		Reason: db.LockReasonUnauthorized,
	}
}

func ipInCIDR(ipValue, cidrValue string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipValue))
	if ip == nil {
		return false
	}
	_, network, err := net.ParseCIDR(strings.TrimSpace(cidrValue))
	if err != nil {
		return false
	}
	return network.Contains(ip)
}

// InitLockEngine initializes the circuit breaker and concurrency semaphore
// from config values. Called once during startup.
func (a *Api) InitLockEngine(cbConfig config.LockCircuitBreaker) {
	breaker := newLockCircuitBreaker()
	breaker.setEnabled(cbConfig.Enabled)
	breaker.setThreshold(cbConfig.Threshold, cbConfig.WindowSec)
	a.lockCircuitBreaker = breaker

	if a.lockEngineSem == nil {
		a.lockEngineSem = make(chan struct{}, lockEngineConcurrency)
	}
}

func (a *Api) StartLockEngine() {
	sub, err := a.nc.Subscribe("device.v1.*.online", func(msg *nats.Msg) {
		parts := strings.Split(msg.Subject, ".")
		if len(parts) < 4 || parts[2] == "" {
			log.Printf("lock_engine: invalid subject format, skipping: %s", msg.Subject)
			return
		}
		tenantSlug := parts[2]
		tdb := a.db.ForTenant(tenantSlug)

		var device entity.Device
		if err := json.Unmarshal(msg.Data, &device); err != nil {
			log.Printf("lock_engine: failed to unmarshal device event: %v", err)
			return
		}
		go a.tryHandleLockDeviceOnline(tdb, device, tenantSlug)
	})
	if err != nil {
		log.Printf("lock_engine: failed to subscribe to device.v1.*.online: %v", err)
	} else {
		log.Printf("lock_engine: subscribed to device.v1.*.online (sub=%s)", sub.Subject)
	}
}

// tryHandleLockDeviceOnline acquires the concurrency semaphore before
// evaluating a device. This prevents DB connection pool exhaustion during
// mass-online events.
func (a *Api) tryHandleLockDeviceOnline(tdb *db.TenantDB, device entity.Device, tenantSlug string) {
	a.acquireLockSem()
	defer a.releaseLockSem()
	a.handleLockDeviceOnline(tdb, device, tenantSlug)
}

func (a *Api) acquireLockSem() {
	if a.lockEngineSem != nil {
		a.lockEngineSem <- struct{}{}
	}
}

func (a *Api) releaseLockSem() {
	if a.lockEngineSem != nil {
		<-a.lockEngineSem
	}
}

func (a *Api) handleLockDeviceOnline(tdb *db.TenantDB, device entity.Device, tenantSlug string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a.evaluateAndMaybeCommand(ctx, tdb, device, tenantSlug, lockTriggerOnline, "")
}

// shouldSkipLockCommand is true when ShouldCommand but the trigger is a
// status-diff path (poll/notify) and Redis already recorded the same status.
// online/chase always force-converge (never skip).
func shouldSkipLockCommand(trigger string, found bool, prevStatus, decisionStatus db.DeviceLockStatus, shouldCommand bool) bool {
	if !shouldCommand {
		return false
	}
	forceConverge := trigger == lockTriggerOnline || trigger == lockTriggerChase
	if forceConverge {
		return false
	}
	return found && prevStatus == decisionStatus
}

// evaluateAndMaybeCommand is the shared ONT Lock evaluate pipeline.
// online/chase force-send when ShouldCommand; poll/notify skip when Redis
// last_status matches the new decision. reportedIP empty → fetch via USP/CWMP.
//
// Capability gate: opt_out before TryLock; after TryLock, probe only when
// shouldProbeOntLockCapability allows (online always re-probes; poll/chase/notify
// skip without probe when an unsupported row already exists). Unsupported/transient skip evaluate.
func (a *Api) evaluateAndMaybeCommand(ctx context.Context, tdb *db.TenantDB, device entity.Device, tenantSlug, trigger, reportedIP string) {
	if a.unsupportedLockOptedOut(ctx, tdb, device.SN) {
		return
	}

	unlock, ok, err := lockStateStore.TryLock(ctx, tenantSlug, device.SN, 0)
	if err != nil {
		log.Printf("lock_engine: try lock %s: %v", device.SN, err)
	}
	if !ok {
		return
	}
	defer unlock()

	if !a.gateLockCapability(ctx, tdb, device, tenantSlug, trigger) {
		return
	}

	// Subscribe only on online (re-probe path). Poll/notify/chase must not re-Add.
	// Defer runs before unlock (LIFO) so subscribe's NotifyOKAt touch takes the
	// per-SN lock after this evaluate's state Put.
	if a.lockNotifyEnabled && trigger == lockTriggerOnline {
		if mtp := lockDeviceMTP(device); mtp != "" && mtp != "cwmp" {
			defer func() {
				go a.ensureLockIPValueChangeSubscription(context.Background(), device, tenantSlug, mtp)
			}()
		}
	}

	if reportedIP == "" {
		reportedIP = a.lockReportedIP(ctx, device, tenantSlug)
	}

	policy, foundPolicy, err := lockCache.GetLockPolicy(ctx, tenantSlug, device.SN)
	var policyPtr *db.LockPolicy
	if err == nil && foundPolicy {
		policyPtr = &policy
	} else {
		policy, err = tdb.GetLockPolicy(ctx, device.SN)
		if err == nil {
			policyPtr = &policy
			_ = lockCache.PutLockPolicy(ctx, tenantSlug, policy)
		} else if err != mongo.ErrNoDocuments {
			log.Printf("lock_engine: get policy %s: %v", device.SN, err)
		}
	}

	cfg, err := tdb.GetLockConfig(ctx)
	if err != nil {
		log.Printf("lock_engine: get config: %v", err)
		return
	}

	decision := EvaluateLockDecision(LockEvaluationInput{
		SN:         device.SN,
		ReportedIP: reportedIP,
		Config:     cfg,
		Policy:     policyPtr,
	})

	prev, found, err := lockStateStore.Get(ctx, tenantSlug, device.SN)
	if err != nil {
		log.Printf("lock_engine: get device state %s: %v", device.SN, err)
		found = false
	}

	// Record unauthorized devices (both LOCKED and PENDING). The state guard
	// skips redundant upserts when IP and status are unchanged since the last
	// evaluate (e.g. IP poll every 60s on a steady-state locked device).
	shouldRecord := decision.Reason == db.LockReasonUnauthorized
	if shouldRecord && found && prev.LastIP == reportedIP && prev.LastStatus == decision.Status {
		shouldRecord = false
	}
	if shouldRecord {
		_ = tdb.RecordUnauthorizedDevice(ctx, db.UnauthorizedDevice{
			SN:         device.SN,
			ReportedIP: reportedIP,
			Reason:     decision.Reason,
			Status:     decision.Status,
		})
	}

	// Self-cleanup: when a previously non-unlocked device becomes authorized
	// (e.g. IP drifted into a whitelist CIDR), remove its unauthorized entry
	// and leave an audit trail.
	if found && prev.LastStatus != db.LockStatusUnlocked && decision.Reason == db.LockReasonAuthorized {
		if deleted, _ := tdb.DeleteUnauthorizedDevices(ctx, []string{device.SN}); deleted > 0 {
			a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
				SN:     device.SN,
				Action: "unauthorized_auto_resolved",
				Details: bson.M{
					"source":      "evaluate_ip_authorized",
					"reported_ip": reportedIP,
				},
			})
		}
	}

	details := bson.M{
		"reason":      decision.Reason,
		"reported_ip": reportedIP,
		"trigger":     trigger,
	}
	if found {
		details["previous_status"] = prev.LastStatus
		details["previous_ip"] = prev.LastIP
	}

	now := time.Now()
	nextState := lockDeviceState{
		LastIP:     reportedIP,
		LastStatus: decision.Status,
		UpdatedAt:  now,
	}
	if found {
		nextState.LastCommand = prev.LastCommand
		nextState.NotifyOKAt = prev.NotifyOKAt
	}
	if trigger == lockTriggerIPChangeNotify {
		nextState.NotifyOKAt = now
	}

	if !decision.ShouldCommand {
		a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
			SN:      device.SN,
			Action:  "evaluate",
			Status:  decision.Status,
			Details: details,
		})
		if err := lockStateStore.Put(ctx, tenantSlug, device.SN, nextState); err != nil {
			log.Printf("lock_engine: put device state %s: %v", device.SN, err)
		}
		return
	}

	if shouldSkipLockCommand(trigger, found, prev.LastStatus, decision.Status, decision.ShouldCommand) {
		details["command_skipped"] = true
		a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
			SN:      device.SN,
			Action:  "evaluate",
			Status:  decision.Status,
			Details: details,
		})
		if err := lockStateStore.Put(ctx, tenantSlug, device.SN, nextState); err != nil {
			log.Printf("lock_engine: put device state %s: %v", device.SN, err)
		}
		return
	}

	a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
		SN:      device.SN,
		Action:  "evaluate",
		Status:  decision.Status,
		Details: details,
	})

	// Circuit breaker: suppress LOCK commands when tripped to prevent
	// mass-lock accidents (e.g. misconfigured whitelist). UNLOCK is never
	// suppressed. Successful deliveries are counted in deliverLockCommand.
	if a.suppressLockIfBreakerTripped(ctx, tdb, tenantSlug, device.SN, decision.Status) {
		return
	}

	if _, err := a.sendLockCommand(ctx, tdb, device.SN, reportedIP, decision, tenantSlug); err != nil {
		log.Printf("lock_engine: send lock command %s: %v", device.SN, err)
		return
	}

	nextState.LastCommand = decision.CommandValue
	if err := lockStateStore.Put(ctx, tenantSlug, device.SN, nextState); err != nil {
		log.Printf("lock_engine: put device state %s: %v", device.SN, err)
	}
}

// lockReportedIP fetches the WAN IP via the device's active transport protocol.
// CWMP devices use GetParameterValues; USP devices (MQTT/WS/STOMP) use a USP
// Get message. Returns empty string if the IP cannot be retrieved.
func (a *Api) lockReportedIP(ctx context.Context, device entity.Device, tenantSlug string) string {
	mtp := lockDeviceMTP(device)

	if mtp == "cwmp" {
		resp, err := cwmpGetValues(device.SN, []string{lockWanIPPath}, a.nc, tenantSlug)
		if err != nil {
			log.Printf("lock_engine: get reported IP (cwmp) for %s: %v", device.SN, err)
			return ""
		}
		for _, param := range resp.ParameterList {
			if param.Name == lockWanIPPath {
				return strings.TrimSpace(param.Value)
			}
		}
		return ""
	}

	if mtp != "" {
		ip, err := uspGetValue(device.SN, lockWanIPPath, mtp, a.nc, tenantSlug)
		if err != nil {
			log.Printf("lock_engine: get reported IP (usp/%s) for %s: %v", mtp, device.SN, err)
			return ""
		}
		return strings.TrimSpace(ip)
	}

	return ""
}

func (a *Api) sendLockCommand(ctx context.Context, tdb *db.TenantDB, sn, reportedIP string, decision LockDecision, tenantSlug string) (db.LockCommandAttempt, error) {
	attempt, err := tdb.CreateLockCommandAttempt(ctx, db.LockCommandAttempt{
		DeviceSN:     db.NormalizeSN(sn),
		ReportedIP:   reportedIP,
		TargetStatus: decision.Status,
		CommandValue: decision.CommandValue,
	})
	if err != nil {
		return attempt, err
	}
	if err := a.deliverLockCommand(ctx, tdb, attempt, decision, tenantSlug); err != nil {
		return attempt, err
	}
	return attempt, nil
}

// deliverLockCommand dispatches the lock/unlock command via the device's
// active MTP protocol. It delegates to deliverLockCommandByMTP which
// auto-detects CWMP vs USP. On failure, the command is marked for retry
// or failure based on attempt count.
func (a *Api) deliverLockCommand(ctx context.Context, tdb *db.TenantDB, attempt db.LockCommandAttempt, decision LockDecision, tenantSlug string) error {
	if err := a.deliverLockCommandByMTP(attempt, decision, tenantSlug); err != nil {
		a.markLockCommandOutcome(ctx, tdb, attempt, err)
		return err
	}

	_ = tdb.UpdateLockCommandStatus(ctx, attempt.ID, db.LockCommandSuccess, "")
	if decision.Status == db.LockStatusLocked && a.lockCircuitBreaker != nil {
		a.lockCircuitBreaker.recordLock(tenantSlug)
	}
	return nil
}

// suppressLockIfBreakerTripped returns true when a LOCK command should be
// suppressed because the per-tenant circuit breaker is open.
func (a *Api) suppressLockIfBreakerTripped(ctx context.Context, tdb *db.TenantDB, tenantSlug, sn string, status db.DeviceLockStatus) bool {
	if status != db.LockStatusLocked || a.lockCircuitBreaker == nil {
		return false
	}
	if !a.lockCircuitBreaker.isTripped(tenantSlug) {
		return false
	}
	log.Printf("lock_engine: circuit breaker tripped for tenant %s, suppressing LOCK for %s",
		tenantSlug, sn)
	a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
		SN:     sn,
		Action: "circuit_breaker_suppressed",
		Status: status,
	})
	return true
}

func (a *Api) markLockCommandOutcome(ctx context.Context, tdb *db.TenantDB, attempt db.LockCommandAttempt, err error) {
	if isPermanentLockCommandError(err) {
		_ = tdb.UpdateLockCommandStatus(ctx, attempt.ID, db.LockCommandFailed, err.Error())
		return
	}
	maxAttempts := a.lockMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = db.DefaultLockCommandMaxAttempts
	}
	if shouldMarkLockCommandForRetry(attempt.AttemptCount, maxAttempts) {
		_ = tdb.MarkLockCommandForRetry(ctx, attempt.ID, err.Error())
		return
	}
	_ = tdb.UpdateLockCommandStatus(ctx, attempt.ID, db.LockCommandFailed, err.Error())
}

func isPermanentLockCommandError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "does not exist in the schema") ||
		strings.Contains(msg, fmt.Sprintf("usp error %d:", uspErrCodePathNotInSchema))
}

func (a *Api) recordLockAudit(ctx context.Context, tdb *db.TenantDB, tenantSlug string, logEntry db.LockAuditLog) {
	if err := tdb.CreateLockAuditLog(ctx, logEntry); err != nil {
		log.Printf("lock_engine: create audit log: %v", err)
		return
	}
	if err := lockAuditSink.PublishLockAudit(ctx, tenantSlug, logEntry); err != nil {
		log.Printf("lock_engine: publish audit sink: %v", err)
	}
}
