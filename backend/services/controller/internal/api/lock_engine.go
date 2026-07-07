package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/leandrofars/oktopus/internal/cwmp"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/entity"
	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	lockParameterPath = "Device.X_TELKOMSEL_OntLock.Lock"
	lockWanIPPath     = "Device.X_TELKOMSEL_OntLock.InternetWanIP"
)

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

	if input.Policy != nil && input.Policy.Status && input.Policy.PolicyType == db.LockPolicyBlacklist {
		return LockDecision{
			Status:        db.LockStatusLocked,
			Reason:        db.LockReasonBlacklisted,
			ShouldCommand: true,
			CommandValue:  "1",
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
		go a.handleLockDeviceOnline(tdb, device, tenantSlug)
	})
	if err != nil {
		log.Printf("lock_engine: failed to subscribe to device.v1.*.online: %v", err)
	} else {
		log.Printf("lock_engine: subscribed to device.v1.*.online (sub=%s)", sub.Subject)
	}
}

func (a *Api) handleLockDeviceOnline(tdb *db.TenantDB, device entity.Device, tenantSlug string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reportedIP := a.lockReportedIP(ctx, device, tenantSlug)
	policy, found, err := lockCache.GetLockPolicy(ctx, tenantSlug, device.SN)
	var policyPtr *db.LockPolicy
	if err == nil && found {
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

	if decision.Status == db.LockStatusPending {
		_ = tdb.RecordUnauthorizedDevice(ctx, db.UnauthorizedDevice{
			SN:         device.SN,
			ReportedIP: reportedIP,
			Reason:     decision.Reason,
			Status:     decision.Status,
		})
	}

	a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
		SN:     device.SN,
		Action: "evaluate",
		Status: decision.Status,
		Details: bson.M{
			"reason":      decision.Reason,
			"reported_ip": reportedIP,
		},
	})

	if !decision.ShouldCommand {
		return
	}

	if _, err := a.sendLockCommand(ctx, tdb, device.SN, reportedIP, decision, tenantSlug); err != nil {
		log.Printf("lock_engine: send lock command %s: %v", device.SN, err)
	}
}

func (a *Api) lockReportedIP(ctx context.Context, device entity.Device, tenantSlug string) string {
	if device.Cwmp != entity.Online {
		return ""
	}
	resp, err := cwmpGetValues(device.SN, []string{lockWanIPPath}, a.nc, tenantSlug)
	if err != nil {
		log.Printf("lock_engine: get reported IP for %s: %v", device.SN, err)
		return ""
	}
	for _, param := range resp.ParameterList {
		if param.Name == lockWanIPPath {
			return strings.TrimSpace(param.Value)
		}
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

func (a *Api) deliverLockCommand(ctx context.Context, tdb *db.TenantDB, attempt db.LockCommandAttempt, decision LockDecision, tenantSlug string) error {
	payload := cwmp.SetParameterValues(lockParameterPath, decision.CommandValue)
	qw := &quietResponseWriter{status: http.StatusOK}
	_, _, err := cwmpInteraction[cwmp.SetParameterValuesResponse](attempt.DeviceSN, []byte(payload), qw, a.nc, tenantSlug)
	if err != nil {
		a.markLockCommandOutcome(ctx, tdb, attempt, err)
		return err
	}
	if qw.status != http.StatusOK {
		err = fmt.Errorf("cwmp request failed with status %d: %s", qw.status, strings.TrimSpace(string(qw.body)))
		a.markLockCommandOutcome(ctx, tdb, attempt, err)
		return err
	}

	_ = tdb.UpdateLockCommandStatus(ctx, attempt.ID, db.LockCommandSuccess, "")
	return nil
}

func (a *Api) markLockCommandOutcome(ctx context.Context, tdb *db.TenantDB, attempt db.LockCommandAttempt, err error) {
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

func (a *Api) recordLockAudit(ctx context.Context, tdb *db.TenantDB, tenantSlug string, logEntry db.LockAuditLog) {
	if err := tdb.CreateLockAuditLog(ctx, logEntry); err != nil {
		log.Printf("lock_engine: create audit log: %v", err)
		return
	}
	if err := lockAuditSink.PublishLockAudit(ctx, tenantSlug, logEntry); err != nil {
		log.Printf("lock_engine: publish audit sink: %v", err)
	}
}
