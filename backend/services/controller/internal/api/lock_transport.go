package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/leandrofars/oktopus/internal/cwmp"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/entity"
	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
	"github.com/leandrofars/oktopus/internal/usp/usp_utils"
	"github.com/nats-io/nats.go"
)

const uspErrCodePathNotInSchema = 7026

type uspErrorBody struct {
	ErrCode uint32 `json:"err_code"`
	ErrMsg  string `json:"err_msg"`
}

func parseUSPErrorBody(body []byte) error {
	if len(body) == 0 {
		return nil
	}
	var uspErr uspErrorBody
	if err := json.Unmarshal(body, &uspErr); err != nil || uspErr.ErrCode == 0 {
		return nil
	}
	return fmt.Errorf("usp error %d: %s", uspErr.ErrCode, uspErr.ErrMsg)
}

// lockDeviceMTP returns the preferred MTP protocol name for a device.
// CWMP is checked first (primary control plane for ONT Lock), then MQTT,
// WebSocket, and STOMP.
func lockDeviceMTP(device entity.Device) string {
	if device.Cwmp == entity.Online {
		return "cwmp"
	}
	if device.Mqtt == entity.Online {
		return entity.Mqtt
	}
	if device.Websockets == entity.Online {
		return entity.Websockets
	}
	if device.Stomp == entity.Online {
		return entity.Stomp
	}
	return ""
}

// deliverLockCommandByMTP dispatches a lock/unlock command via the device's
// active MTP protocol. CWMP devices use SetParameterValues (SOAP);
// USP devices (MQTT/WS/STOMP) use a USP Set message.
func (a *Api) deliverLockCommandByMTP(attempt db.LockCommandAttempt, decision LockDecision, tenantSlug string) error {
	device, online := a.getOnlineDeviceNoWrite2(attempt.DeviceSN, tenantSlug)
	if !online {
		return fmt.Errorf("device %s is not online", attempt.DeviceSN)
	}

	mtp := lockDeviceMTP(device)
	if mtp == "" {
		return fmt.Errorf("device %s has no active MTP", attempt.DeviceSN)
	}

	if mtp == "cwmp" {
		return a.deliverLockCommandCWMP(attempt, decision, tenantSlug)
	}
	return a.deliverLockCommandUSP(attempt, decision, tenantSlug, mtp)
}

func (a *Api) deliverLockCommandCWMP(attempt db.LockCommandAttempt, decision LockDecision, tenantSlug string) error {
	payload := cwmp.SetParameterValues(lockParameterPath, decision.CommandValue)
	qw := &quietResponseWriter{status: http.StatusOK}
	_, _, err := cwmpInteraction[cwmp.SetParameterValuesResponse](attempt.DeviceSN, []byte(payload), qw, a.nc, tenantSlug)
	if err != nil {
		return err
	}
	if qw.status != http.StatusOK {
		return fmt.Errorf("cwmp request failed with status %d: %s", qw.status, strings.TrimSpace(string(qw.body)))
	}
	return nil
}

func (a *Api) deliverLockCommandUSP(attempt db.LockCommandAttempt, decision LockDecision, tenantSlug, mtp string) error {
	setMsg := usp_utils.NewSetMsg(usp_msg.Set{
		AllowPartial: false,
		UpdateObjs: []*usp_msg.Set_UpdateObject{
			{
				ObjPath: "Device.X_TELKOMSEL_OntLock.",
				ParamSettings: []*usp_msg.Set_UpdateParamSetting{
					{
						Param:    "Lock",
						Value:    decision.CommandValue,
						Required: true,
					},
				},
			},
		},
	})
	_, err := sendUspMsgNoHTTP(setMsg, attempt.DeviceSN, a.nc, mtp, tenantSlug)
	return err
}

// sendUspMsgNoHTTP sends a USP message without writing to a real http.ResponseWriter.
// It uses quietResponseWriter to capture status/body, mirroring the pattern
// used in cwmp_datamodel.go for headless CWMP interactions.
func sendUspMsgNoHTTP(msg usp_msg.Msg, sn string, nc *nats.Conn, mtp, tenantSlug string) ([]byte, error) {
	qw := &quietResponseWriter{status: http.StatusOK}
	if err := sendUspMsg(msg, sn, qw, nc, mtp, tenantSlug); err != nil {
		return nil, err
	}
	if qw.status == http.StatusGatewayTimeout {
		return nil, fmt.Errorf("usp request timeout")
	}
	if qw.status != http.StatusOK {
		return nil, fmt.Errorf("usp request failed with status %d: %s", qw.status, strings.TrimSpace(string(qw.body)))
	}
	if uspErr := parseUSPErrorBody(qw.body); uspErr != nil {
		return nil, uspErr
	}
	return qw.body, nil
}

// uspGetValue fetches a single parameter value from a USP device via Get message.
func uspGetValue(sn, paramPath, mtp string, nc *nats.Conn, tenantSlug string) (string, error) {
	getMsg := usp_utils.NewGetMsg(usp_msg.Get{
		ParamPaths: []string{paramPath},
	})
	data, err := sendUspMsgNoHTTP(getMsg, sn, nc, mtp, tenantSlug)
	if err != nil {
		return "", err
	}

	var getResp uspGetResponseJSON
	if err := json.Unmarshal(data, &getResp); err != nil {
		return "", fmt.Errorf("parse USP GetResp: %w", err)
	}
	for _, rp := range getResp.ReqPathResults {
		for _, rpr := range rp.ResolvedPathResults {
			for k, v := range rpr.ResultParams {
				if k == paramPath || strings.HasSuffix(k, paramPath) {
					return v, nil
				}
			}
		}
	}
	return "", fmt.Errorf("parameter %s not found in USP response", paramPath)
}

// uspGetResponseJSON is a minimal JSON decoder for USP GetResp.
type uspGetResponseJSON struct {
	ReqPathResults []struct {
		ResolvedPathResults []struct {
			ResolvedPath string            `json:"resolved_path"`
			ResultParams map[string]string `json:"result_params"`
		} `json:"resolved_path_results"`
	} `json:"req_path_results"`
}

// getOnlineDeviceNoWrite2 is a convenience wrapper for getOnlineDeviceNoWrite.
func (a *Api) getOnlineDeviceNoWrite2(sn, tenantSlug string) (entity.Device, bool) {
	return a.getOnlineDeviceNoWrite(nil, tenantSlug, sn)
}
