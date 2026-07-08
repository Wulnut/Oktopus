package usp_handler

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/OktopUSP/oktopus/backend/services/mtp/adapter/internal/db"
	"github.com/OktopUSP/oktopus/backend/services/mtp/adapter/internal/nats"
	"github.com/OktopUSP/oktopus/backend/services/mtp/adapter/internal/usp/usp_msg"
	"github.com/OktopUSP/oktopus/backend/services/mtp/adapter/internal/usp/usp_record"
	"google.golang.org/protobuf/proto"
)

func (h *Handler) HandleDeviceInfo(device, tenantSlug, subject string, data []byte, mtp string, ack func()) {
	defer ack()
	log.Printf("Device %s info, mtp: %s, tenant: %s", device, mtp, tenantSlug)

	deviceInfo, err := parseDeviceInfoMsg(device, subject, data, getMtp(mtp))
	if err != nil {
		log.Printf("REJECTED: device %s on subject %s: %v", device, subject, err)
		return
	}
	if deviceInfo.SN == "" {
		log.Printf("WARNING: empty SN for device %s on subject %s, skipping", device, subject)
		return
	}

	deviceInfo.TenantID = tenantSlug
	if deviceExists, _ := h.db.DeviceExists(deviceInfo.SN); !deviceExists {
		fmtDeviceInfo, _ := json.Marshal(deviceInfo)
		h.nc.Publish("device.v1."+tenantSlug+".new", fmtDeviceInfo)
	}
	err = h.db.CreateDevice(deviceInfo)
	if err != nil {
		log.Printf("Failed to create device: %v", err)
		return
	}
	// Publish online event only after info validation succeeds.
	onlineData, _ := json.Marshal(deviceInfo)
	h.nc.Publish("device.v1."+tenantSlug+".online", onlineData)
}

func getMtp(mtp string) db.MTP {
	switch mtp {
	case nats.MQTT_STREAM_NAME:
		return db.MQTT
	case nats.WS_STREAM_NAME:
		return db.WEBSOCKETS
	case nats.STOMP_STREAM_NAME:
		return db.STOMP
	default:
		return db.UNDEFINED
	}
}

func parseDeviceInfoMsg(sn, subject string, data []byte, mtp db.MTP) (db.Device, error) {
	var record usp_record.Record
	var message usp_msg.Msg

	if err := proto.Unmarshal(data, &record); err != nil {
		return db.Device{}, fmt.Errorf("invalid USP record: %w", err)
	}

	noSessionContext := record.GetNoSessionContext()
	if noSessionContext == nil {
		recordType := recordTypeName(&record)
		return db.Device{}, fmt.Errorf("expected NoSessionContext record, got %s", recordType)
	}

	if err := proto.Unmarshal(noSessionContext.Payload, &message); err != nil {
		return db.Device{}, fmt.Errorf("invalid USP message payload: %w", err)
	}

	response, ok := message.Body.MsgBody.(*usp_msg.Body_Response)
	if !ok {
		msgType := ""
		if message.Header != nil {
			msgType = message.Header.MsgType.String()
		}
		return db.Device{}, fmt.Errorf("expected GET response, got %s", msgType)
	}

	getResp := response.Response.GetGetResp()
	if getResp == nil {
		return db.Device{}, fmt.Errorf("expected GetResp body")
	}

	vendor, err := getRespParam(getResp.ReqPathResults, 0, "Manufacturer")
	if err != nil {
		return db.Device{}, err
	}
	model, err := getRespParam(getResp.ReqPathResults, 1, "ModelName")
	if err != nil {
		return db.Device{}, err
	}
	version, err := getRespParam(getResp.ReqPathResults, 2, "SoftwareVersion")
	if err != nil {
		return db.Device{}, err
	}
	if _, err := getRespParam(getResp.ReqPathResults, 3, "SerialNumber"); err != nil {
		return db.Device{}, err
	}
	productClass, err := getRespParam(getResp.ReqPathResults, 4, "ProductClass")
	if err != nil {
		return db.Device{}, err
	}
	hwVersion, err := getRespParam(getResp.ReqPathResults, 5, "HardwareVersion")
	if err != nil {
		return db.Device{}, err
	}

	var device db.Device
	device.Vendor = vendor
	device.Model = model
	device.Version = version
	device.ProductClass = productClass
	device.HWVersion = hwVersion
	device.SN = sn
	switch mtp {
	case db.MQTT:
		device.Mqtt = db.Online
	case db.WEBSOCKETS:
		device.Websockets = db.Online
	case db.STOMP:
		device.Stomp = db.Online
	}
	device.Status = db.Online

	return device, nil
}

func recordTypeName(record *usp_record.Record) string {
	switch {
	case record.GetSessionContext() != nil:
		return "SessionContext"
	case record.GetWebsocketConnect() != nil:
		return "WebSocketConnect"
	case record.GetMqttConnect() != nil:
		return "MQTTConnect"
	case record.GetStompConnect() != nil:
		return "STOMPConnect"
	case record.GetDisconnect() != nil:
		return "Disconnect"
	default:
		return "unknown"
	}
}

func getRespParam(results []*usp_msg.GetResp_RequestedPathResult, idx int, key string) (string, error) {
	if idx >= len(results) {
		return "", fmt.Errorf("missing ReqPathResults[%d] (%s)", idx, key)
	}
	resolved := results[idx].ResolvedPathResults
	if len(resolved) == 0 {
		return "", fmt.Errorf("empty ResolvedPathResults for %s", key)
	}
	val, ok := resolved[0].ResultParams[key]
	if !ok || val == "" {
		return "", fmt.Errorf("missing parameter %s", key)
	}
	return val, nil
}
