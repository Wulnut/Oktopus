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

	ontLockObjectPath = "X_TELKOMSEL_OntLock."

	lockTriggerOnline         = "online"
	lockTriggerChase          = "chase"
	lockTriggerIPChangePoll   = "ip_change_poll"
	lockTriggerIPChangeNotify = "ip_change_notify"
)

// lockCWMPRootPrefix returns the CWMP root for OntLock paths based on the
// device's stored data model. TR-098 devices use "InternetGatewayDevice.";
// TR-181 (and unknown) devices use "Device.".
func lockCWMPRootPrefix(dataModel string) string {
	if dataModel == cwmpDataModelTR098 {
		return cwmpRootTR098
	}
	return cwmpRootTR181
}

// lockParamPathCWMP returns the OntLock.Lock parameter path adapted for the
// device's CWMP data model root.
func lockParamPathCWMP(dataModel string) string {
	return lockCWMPRootPrefix(dataModel) + ontLockObjectPath + "Lock"
}

// lockWanIPPathCWMP returns the OntLock.InternetWanIP parameter path adapted
// for the device's CWMP data model root.
func lockWanIPPathCWMP(dataModel string) string {
	return lockCWMPRootPrefix(dataModel) + ontLockObjectPath + "InternetWanIP"
}

// lockOppositeDataModel returns the data model root opposite to the given one.
// Used when a device's OntLock vendor extension lives under a different root
// than its reported data model (e.g. a TR-098 device exposing OntLock under Device.).
func lockOppositeDataModel(dataModel string) string {
	if dataModel == cwmpDataModelTR098 {
		return cwmpDataModelTR181
	}
	return cwmpDataModelTR098
}

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
	for _, cidr := range strings.Split(cidrValue, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// hostCIDR returns a single-host CIDR for the given bare IP address.
// IPv4 -> "/32", IPv6 -> "/128". If the input is not a valid bare IP,
// returns it unchanged so Validate() can catch it.
func hostCIDR(ip string) string {
	trimmed := strings.TrimSpace(ip)
	parsed := net.ParseIP(trimmed)
	if parsed == nil {
		return ip
	}
	if ipv4 := parsed.To4(); ipv4 != nil {
		return ipv4.String() + "/32"
	}
	return trimmed + "/128"
}

// normalizeReportedIP sanitizes a raw IP string from a device report.
// Handles: multiple space/comma-separated addresses (returns first valid),
// IPv6 zone IDs (strips "%eth0" suffix), empty/garbage input (returns "").
func normalizeReportedIP(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	candidates := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t'
	})
	for _, c := range candidates {
		if idx := strings.Index(c, "%"); idx > 0 {
			c = c[:idx]
		}
		if net.ParseIP(c) != nil {
			return c
		}
	}
	return ""
}

// mergeCIDRByFamily merges new CIDRs into an existing policy's AllowedIPRange.
// A new CIDR replaces all existing CIDRs in the same IP family while preserving
// CIDRs from the other family.
func mergeCIDRByFamily(existing, newCIDRs string) string {
	result := strings.TrimSpace(existing)
	for _, newCIDR := range strings.Split(newCIDRs, ",") {
		newCIDR = strings.TrimSpace(newCIDR)
		if newCIDR == "" {
			continue
		}
		result = mergeSingleCIDRByFamily(result, newCIDR)
	}
	return result
}

func mergeSingleCIDRByFamily(existing, newCIDR string) string {
	newFamily, validNew := cidrFamily(newCIDR)
	if !validNew {
		if existing == "" {
			return newCIDR
		}
		return existing + "," + newCIDR
	}

	parts := make([]string, 0, 2)
	replaced := false
	for _, cidr := range strings.Split(existing, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		family, valid := cidrFamily(cidr)
		if valid && family == newFamily {
			if !replaced {
				parts = append(parts, newCIDR)
				replaced = true
			}
			continue
		}
		parts = append(parts, cidr)
	}
	if !replaced {
		parts = append(parts, newCIDR)
	}
	return strings.Join(parts, ",")
}

func cidrFamily(cidr string) (int, bool) {
	_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return 0, false
	}
	_, bits := network.Mask.Size()
	return bits, bits == 32 || bits == 128
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
		// Re-evaluate all already-online devices so that devices that were
		// online before the controller (re)started are not missed (they will
		// not emit a new "online" event). Runs after a short delay to let NATS
		// subscriptions settle.
		go func() {
			time.Sleep(5 * time.Second)
			a.chaseLockOnStartup()
		}()
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

// shouldSkipFreshLockCommand keeps the legacy same-status optimization unless
// a recorded failure shows that the target may not actually be converged. That
// failure must be allowed to probe again after its cooldown expires.
func (a *Api) shouldSkipFreshLockCommand(trigger string, found bool, prev lockDeviceState, decisionStatus db.DeviceLockStatus, shouldCommand bool) bool {
	if a.lockBackoff.Enabled && backoffFor(prev.CommandBackoffs, decisionStatus) != nil {
		return false
	}
	return shouldSkipLockCommand(trigger, found, prev.LastStatus, decisionStatus, shouldCommand)
}

func nextLockEvaluationState(prev lockDeviceState, found bool, reportedIP string, status db.DeviceLockStatus, now time.Time) lockDeviceState {
	next := lockDeviceState{
		LastIP:     reportedIP,
		LastStatus: status,
		UpdatedAt:  now,
	}
	if found {
		next.LastCommand = prev.LastCommand
		next.NotifyOKAt = prev.NotifyOKAt
		next.CommandBackoffs = prev.CommandBackoffs
	}
	return next
}

func successfulLockDeviceState(state lockDeviceState, reportedIP string, status db.DeviceLockStatus, commandValue string, now time.Time) lockDeviceState {
	state.LastIP = reportedIP
	state.LastStatus = status
	state.LastCommand = commandValue
	state.UpdatedAt = now
	state.CommandBackoffs = nil
	return state
}

// evaluateFreshCooldown reports whether a fresh command for target is blocked
// by an active per-device cooldown. Redis failures soft-degrade to allow.
func (a *Api) evaluateFreshCooldown(ctx context.Context, tenantSlug, sn string, target db.DeviceLockStatus) bool {
	if !a.lockBackoff.Enabled {
		return false
	}
	state, found, err := lockStateStore.Get(ctx, tenantSlug, sn)
	if err != nil {
		log.Printf("lock_backoff: get state before fresh command %s: %v", sn, err)
		return false
	}
	return found && isBackoffCoolingDown(state.CommandBackoffs, target, time.Now())
}

// evaluateAndMaybeCommand is the shared ONT Lock evaluate pipeline.
// online/chase force-send when ShouldCommand; poll/notify skip when Redis
// last_status matches the new decision. reportedIP empty → fetch via USP/CWMP.
//
// Capability gate: opt_out before TryLock; after TryLock, probe only when
// shouldProbeOntLockCapability allows (online always re-probes; poll/chase/notify
// skip without probe when an unsupported row already exists). Unsupported/transient skip evaluate.
func (a *Api) evaluateAndMaybeCommand(ctx context.Context, tdb *db.TenantDB, device entity.Device, tenantSlug, trigger, reportedIP string) {
	reportedIP = normalizeReportedIP(reportedIP)
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

	decision, reportedIP, err := a.resolveCurrentLockDecision(ctx, tdb, device, tenantSlug, reportedIP)
	if err != nil {
		log.Printf("lock_engine: resolve decision %s: %v", device.SN, err)
		return
	}

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
	nextState := nextLockEvaluationState(prev, found, reportedIP, decision.Status, now)
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

	if a.evaluateFreshCooldown(ctx, tenantSlug, device.SN, decision.Status) {
		return
	}

	if a.shouldSkipFreshLockCommand(trigger, found, prev, decision.Status, decision.ShouldCommand) {
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
	// Still persist evaluated state so poll/notify can skip redundant work
	// (same as the !ShouldCommand / shouldSkip early returns above).
	if a.suppressLockIfBreakerTripped(ctx, tdb, tenantSlug, device.SN, decision.Status) {
		if err := lockStateStore.Put(ctx, tenantSlug, device.SN, nextState); err != nil {
			log.Printf("lock_engine: put device state %s: %v", device.SN, err)
		}
		return
	}

	if _, err := a.sendLockCommand(ctx, tdb, device.SN, reportedIP, decision, tenantSlug); err != nil {
		log.Printf("lock_engine: send lock command %s: %v", device.SN, err)
		return
	}

	nextState = successfulLockDeviceState(nextState, reportedIP, decision.Status, decision.CommandValue, now)
	if err := lockStateStore.Put(ctx, tenantSlug, device.SN, nextState); err != nil {
		log.Printf("lock_engine: put device state %s: %v", device.SN, err)
	}
}

// resolveCurrentLockDecision reads the device's current WAN IP, policy and
// global config. Retry paths use the same resolver so a historical command is
// never replayed after policy or configuration changes.
func (a *Api) resolveCurrentLockDecision(ctx context.Context, tdb *db.TenantDB, device entity.Device, tenantSlug, reportedIP string) (LockDecision, string, error) {
	reportedIP = normalizeReportedIP(reportedIP)
	if reportedIP == "" {
		reportedIP = normalizeReportedIP(a.lockReportedIP(ctx, device, tenantSlug))
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
		return LockDecision{}, reportedIP, err
	}

	return EvaluateLockDecision(LockEvaluationInput{
		SN:         device.SN,
		ReportedIP: reportedIP,
		Config:     cfg,
		Policy:     policyPtr,
	}), reportedIP, nil
}

// lockReportedIP fetches the WAN IP via the device's active transport protocol.
// CWMP devices use GetParameterValues; USP devices (MQTT/WS/STOMP) use a USP
// Get message. Returns empty string if the IP cannot be retrieved.
func (a *Api) lockReportedIP(ctx context.Context, device entity.Device, tenantSlug string) string {
	mtp := lockDeviceMTP(device)

	if mtp == "cwmp" {
		for _, dm := range []string{device.DataModel, lockOppositeDataModel(device.DataModel)} {
			wanPath := lockWanIPPathCWMP(dm)
			resp, err := cwmpGetValues(device.SN, []string{wanPath}, a.nc, tenantSlug)
			if err != nil {
				continue
			}
			for _, param := range resp.ParameterList {
				if param.Name == wanPath {
					return strings.TrimSpace(param.Value)
				}
			}
		}
		log.Printf("lock_engine: get reported IP (cwmp) for %s: OntLock not found under either root", device.SN)
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

// deliverLockCommand dispatches the lock/unlock command via the device's active
// MTP protocol, then records the per-device failure backoff (on retryable
// failure) or clears all device backoff (on success). It is the single command
// outcome boundary used by both fresh commands and retries.
func (a *Api) deliverLockCommand(ctx context.Context, tdb *db.TenantDB, attempt db.LockCommandAttempt, decision LockDecision, tenantSlug string) error {
	if err := a.deliverLockCommandByMTP(attempt, decision, tenantSlug); err != nil {
		a.handleLockCommandFailure(ctx, tdb, attempt, decision.Status, err, tenantSlug)
		return err
	}

	_ = tdb.UpdateLockCommandStatus(ctx, attempt.ID, db.LockCommandSuccess, "")
	a.handleLockCommandSuccess(ctx, tdb, attempt.DeviceSN, decision.Status, tenantSlug)
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

// handleLockCommandFailure records a delivery failure. Permanent schema/path
// errors keep the existing failed-status behavior and do NOT count toward
// backoff. Retryable errors record the per-device backoff transition in Redis;
// when the target threshold is reached the Mongo row is marked failed (leaving
// the retry queue) with the cooldown deadline in its error text. Below the
// threshold, the legacy retry/fail behavior is preserved. When backoff is
// disabled, this is equivalent to the former markLockCommandOutcome.
func (a *Api) handleLockCommandFailure(ctx context.Context, tdb *db.TenantDB, attempt db.LockCommandAttempt, target db.DeviceLockStatus, err error, tenantSlug string) {
	if isPermanentLockCommandError(err) {
		if tdb != nil {
			_ = tdb.UpdateLockCommandStatus(ctx, attempt.ID, db.LockCommandFailed, err.Error())
		}
		return
	}
	cooledDown := false
	deadline := time.Time{}
	if a.lockBackoff.Enabled {
		cooledDown, deadline = a.recordDeviceBackoffFailure(ctx, tdb, tenantSlug, attempt.DeviceSN, target, err)
	}
	if cooledDown {
		if tdb != nil {
			_ = tdb.UpdateLockCommandStatus(ctx, attempt.ID, db.LockCommandFailed,
				formatBackoffCommandError(err.Error(), deadline))
		}
		return
	}
	maxAttempts := a.lockMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = db.DefaultLockCommandMaxAttempts
	}
	if shouldMarkLockCommandForRetry(attempt.AttemptCount, maxAttempts) {
		if tdb != nil {
			_ = tdb.MarkLockCommandForRetry(ctx, attempt.ID, err.Error())
		}
		return
	}
	if tdb != nil {
		_ = tdb.UpdateLockCommandStatus(ctx, attempt.ID, db.LockCommandFailed, err.Error())
	}
}

// recordDeviceBackoffFailure records one retryable failure in the device's Redis
// state (read-modify-write preserving existing device fields). Returns cooled=true
// and the deadline when this failure enters or re-enters cooldown, and emits the
// device_command_backoff_started transition log + audit. Redis errors are logged
// and never block.
func (a *Api) recordDeviceBackoffFailure(ctx context.Context, tdb *db.TenantDB, tenantSlug, sn string, target db.DeviceLockStatus, err error) (bool, time.Time) {
	if !supportsPersistentLockBackoff(lockStateStore) {
		return false, time.Time{}
	}
	state, found, errGet := lockStateStore.Get(ctx, tenantSlug, sn)
	if errGet != nil {
		log.Printf("lock_backoff: get state %s: %v", sn, errGet)
		return false, time.Time{}
	}
	var prev map[db.DeviceLockStatus]*lockCommandBackoff
	if found {
		prev = state.CommandBackoffs
	}
	now := time.Now()
	next, cooled := applyBackoffFailure(prev, target, now, a.lockBackoff.Threshold, a.lockBackoff.Cooldown, err.Error())
	state.CommandBackoffs = next
	state.UpdatedAt = now
	if errPut := lockStateStore.Put(ctx, tenantSlug, sn, state); errPut != nil {
		log.Printf("lock_backoff: put state %s: %v", sn, errPut)
		return false, time.Time{}
	}
	if !cooled {
		return false, time.Time{}
	}
	b := next[target]
	log.Printf("lock_command_backoff: tenant=%s sn=%s target=%s failures=%d until=%s error=%s",
		tenantSlug, sn, target, b.ConsecutiveFailures, b.CooldownUntil.Format(time.RFC3339), truncateBackoffError(err.Error()))
	if tdb != nil {
		a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
			SN:     sn,
			Action: "device_command_backoff_started",
			Status: target,
			Details: bson.M{
				"target_status":        target,
				"consecutive_failures": b.ConsecutiveFailures,
				"cooldown_until":       b.CooldownUntil,
				"last_error":           truncateBackoffError(err.Error()),
			},
		})
	}
	return true, b.CooldownUntil
}

// handleLockCommandSuccess clears all per-device backoff after a successful
// delivery and emits device_command_backoff_recovered when backoff was active.
// Redis errors are logged and never block; success is never converted to failure.
func (a *Api) handleLockCommandSuccess(ctx context.Context, tdb *db.TenantDB, sn string, target db.DeviceLockStatus, tenantSlug string) {
	state, found, err := lockStateStore.Get(ctx, tenantSlug, sn)
	if err != nil {
		log.Printf("lock_backoff: get state on success %s: %v", sn, err)
		return
	}
	if !found || len(state.CommandBackoffs) == 0 {
		return
	}
	prevMax := maxBackoffFailures(state.CommandBackoffs)
	cleared := make([]string, 0, len(state.CommandBackoffs))
	for k := range state.CommandBackoffs {
		cleared = append(cleared, string(k))
	}
	state.CommandBackoffs = nil
	state.UpdatedAt = time.Now()
	if errPut := lockStateStore.Put(ctx, tenantSlug, sn, state); errPut != nil {
		log.Printf("lock_backoff: put state on success %s: %v", sn, errPut)
		return
	}
	log.Printf("lock_command_backoff: recovered tenant=%s sn=%s successful_target=%s previous_failures=%d",
		tenantSlug, sn, target, prevMax)
	if tdb != nil {
		a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
			SN:     sn,
			Action: "device_command_backoff_recovered",
			Status: target,
			Details: bson.M{
				"successful_target_status": target,
				"cleared_targets":          cleared,
				"previous_max_failures":    prevMax,
			},
		})
	}
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
	persisted, err := tdb.CreateLockAuditLog(ctx, logEntry)
	if err != nil {
		log.Printf("lock_engine: create audit log: %v", err)
		return
	}
	if err := lockAuditSink.PublishLockAudit(ctx, tenantSlug, persisted); err != nil {
		log.Printf("lock_engine: publish audit sink: %v", err)
	}
}
