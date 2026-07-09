package api

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/leandrofars/oktopus/internal/config"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/entity"
	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
	"github.com/leandrofars/oktopus/internal/usp/usp_record"
	"github.com/leandrofars/oktopus/internal/usp/usp_utils"
	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/bson"
	"google.golang.org/protobuf/proto"
)

const (
	lockSubscriptionObjPath = "Device.LocalAgent.Subscription."
	lockNotifTypeValueChange = "ValueChange"
)

// Manual verification (staging SN 081074000888): after deploy with
// LOCK_NOTIFY_ENABLED=true, change WAN IP (or inject USP ValueChange Notify)
// and confirm lock_audit_logs details.trigger == "ip_change_notify".

// isLockWanIPValueChange reports whether a Notify ValueChange path refers to
// Device.X_TELKOMSEL_OntLock.InternetWanIP (full path or suffix match).
func isLockWanIPValueChange(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if path == lockWanIPPath {
		return true
	}
	return strings.HasSuffix(path, ".InternetWanIP") || path == "InternetWanIP"
}

// extractIPFromValueChange returns the trimmed ParamValue from a ValueChange.
func extractIPFromValueChange(vc *usp_msg.Notify_ValueChange) string {
	if vc == nil {
		return ""
	}
	return strings.TrimSpace(vc.GetParamValue())
}

// buildLockIPValueChangeSubscriptionAdd builds a USP Add for a ValueChange
// subscription on InternetWanIP. Matches the LCM OperationComplete pattern
// (Enable + NotifType + ReferenceList); Persistent is optional (Required=false).
func buildLockIPValueChangeSubscriptionAdd() usp_msg.Add {
	return usp_msg.Add{
		AllowPartial: true,
		CreateObjs: []*usp_msg.Add_CreateObject{{
			ObjPath: lockSubscriptionObjPath,
			ParamSettings: []*usp_msg.Add_CreateParamSetting{
				{Param: "Enable", Value: "true", Required: true},
				{Param: "NotifType", Value: lockNotifTypeValueChange, Required: true},
				{Param: "ReferenceList", Value: lockWanIPPath, Required: true},
				{Param: "Persistent", Value: "true", Required: false},
			},
		}},
	}
}

// StartLockNotifyHandler subscribes to inbound USP MTP subjects and handles
// ValueChange Notify for InternetWanIP. No-op when NotifyEnabled is false.
// Duplicate Subscribe alongside the message interceptor is intentional.
func (a *Api) StartLockNotifyHandler(cfg config.LockScale) {
	a.lockNotifyEnabled = cfg.NotifyEnabled
	if !cfg.NotifyEnabled {
		log.Printf("lock_notify: disabled")
		return
	}

	patterns := []string{
		"device.usp.v1.>",
		"mqtt.usp.v1.>",
		"ws.usp.v1.>",
		"stomp.usp.v1.>",
	}
	for _, pattern := range patterns {
		p := pattern
		_, err := a.nc.Subscribe(p, func(msg *nats.Msg) {
			a.handleLockNotifyNATS(msg)
		})
		if err != nil {
			log.Printf("lock_notify: subscribe %s: %v", p, err)
			continue
		}
		log.Printf("lock_notify: subscribed to %s", p)
	}
}

// ensureLockIPValueChangeSubscription creates (or confirms) a USP ValueChange
// subscription after a successful OntLock probe on USP MTP. CWMP is skipped.
// On 7026/schema miss → unsupported; transient → leave on poll (NotifyOKAt unset).
func (a *Api) ensureLockIPValueChangeSubscription(ctx context.Context, device entity.Device, tenantSlug, mtp string) {
	if !a.lockNotifyEnabled || mtp == "" || mtp == "cwmp" {
		return
	}

	if a.lockIPValueChangeSubscriptionExists(device.SN, mtp, tenantSlug) {
		a.touchLockNotifyOKAt(ctx, tenantSlug, device.SN)
		return
	}

	addMsg := usp_utils.NewCreateMsg(buildLockIPValueChangeSubscriptionAdd())
	_, err := sendUspMsgNoHTTP(addMsg, device.SN, a.nc, mtp, tenantSlug)
	if err != nil {
		switch classifyLockProbeError(err) {
		case lockProbeUnsupported:
			tdb := a.db.ForTenant(tenantSlug)
			detail := err.Error()
			if upsertErr := tdb.UpsertUnsupportedLockDevice(ctx, db.UnsupportedLockDevice{
				SN:            device.SN,
				Reason:        db.LockUnsupportedReasonPath,
				Detail:        detail,
				LastCheckedAt: time.Now(),
			}); upsertErr != nil {
				log.Printf("lock_notify: upsert unsupported %s: %v", device.SN, upsertErr)
			}
			a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
				SN:     device.SN,
				Action: "unsupported",
				Details: bson.M{
					"reason": db.LockUnsupportedReasonPath,
					"detail": detail,
					"source": "notify_subscribe",
				},
			})
		default:
			log.Printf("lock_notify: subscribe transient %s: %v", device.SN, err)
		}
		return
	}

	a.touchLockNotifyOKAt(ctx, tenantSlug, device.SN)
	log.Printf("lock_notify: subscribed ValueChange InternetWanIP for %s (mtp=%s)", device.SN, mtp)
}

func (a *Api) lockIPValueChangeSubscriptionExists(sn, mtp, tenantSlug string) bool {
	searchPath := `Device.LocalAgent.Subscription.[Enable=="True"&&NotifType=="ValueChange"&&ReferenceList=="` + lockWanIPPath + `"].`
	getMsg := usp_utils.NewGetMsg(usp_msg.Get{
		ParamPaths: []string{searchPath},
		MaxDepth:   1,
	})
	data, err := sendUspMsgNoHTTP(getMsg, sn, a.nc, mtp, tenantSlug)
	if err != nil {
		// Treat lookup failure as "not found" and attempt Add.
		return false
	}
	var getResp uspGetResponseJSON
	if err := json.Unmarshal(data, &getResp); err != nil {
		return false
	}
	for _, rp := range getResp.ReqPathResults {
		if len(rp.ResolvedPathResults) > 0 {
			return true
		}
	}
	return false
}

func (a *Api) touchLockNotifyOKAt(ctx context.Context, tenantSlug, sn string) {
	state, _, err := lockStateStore.Get(ctx, tenantSlug, sn)
	if err != nil {
		log.Printf("lock_notify: get state %s: %v", sn, err)
		state = lockDeviceState{}
	}
	now := time.Now()
	state.NotifyOKAt = now
	state.UpdatedAt = now
	if err := lockStateStore.Put(ctx, tenantSlug, sn, state); err != nil {
		log.Printf("lock_notify: put NotifyOKAt %s: %v", sn, err)
	}
}

func (a *Api) handleLockNotifyNATS(msg *nats.Msg) {
	if !a.lockNotifyEnabled || len(msg.Data) < 2 {
		return
	}

	tenantSlug, sn := lockNotifyTenantAndSN(msg.Subject)
	if tenantSlug == "" || sn == "" || sn == "unknown" {
		return
	}

	var record usp_record.Record
	if err := proto.Unmarshal(msg.Data, &record); err != nil {
		return
	}
	noSession := record.GetNoSessionContext()
	if noSession == nil {
		return
	}
	var uspMsg usp_msg.Msg
	if err := proto.Unmarshal(noSession.Payload, &uspMsg); err != nil {
		return
	}
	if uspMsg.Header == nil || uspMsg.Header.MsgType != usp_msg.Header_NOTIFY {
		return
	}
	req := uspMsg.Body.GetRequest()
	if req == nil || req.GetNotify() == nil {
		return
	}
	vc := req.GetNotify().GetValueChange()
	if vc == nil || !isLockWanIPValueChange(vc.GetParamPath()) {
		return
	}

	newIP := extractIPFromValueChange(vc)
	go a.handleLockIPValueChangeNotify(tenantSlug, sn, newIP)
}

func (a *Api) handleLockIPValueChangeNotify(tenantSlug, sn, newIP string) {
	a.acquireLockSem()
	defer a.releaseLockSem()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	a.touchLockNotifyOKAt(ctx, tenantSlug, sn)

	device, online := a.getOnlineDeviceNoWrite(ctx, tenantSlug, sn)
	if !online {
		log.Printf("lock_notify: device %s not online, skip evaluate", sn)
		return
	}

	if newIP == "" {
		newIP = a.lockReportedIP(ctx, device, tenantSlug)
	}

	tdb := a.db.ForTenant(tenantSlug)
	a.evaluateAndMaybeCommand(ctx, tdb, device, tenantSlug, lockTriggerIPChangeNotify, newIP)
}

// lockNotifyTenantAndSN parses <prefix>.usp.v1.<tenant>.<sn>.<type>.
func lockNotifyTenantAndSN(subject string) (tenant, sn string) {
	parts := strings.Split(subject, ".")
	for i, p := range parts {
		if p == "v1" && i+2 < len(parts) {
			return parts[i+1], parts[i+2]
		}
	}
	return "", ""
}
