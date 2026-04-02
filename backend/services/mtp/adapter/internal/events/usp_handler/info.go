package usp_handler

import (
	"encoding/json"
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
	deviceInfo := parseDeviceInfoMsg(device, subject, data, getMtp(mtp))
	deviceInfo.TenantID = tenantSlug
	if deviceExists, _ := h.db.DeviceExists(deviceInfo.SN); !deviceExists {
		fmtDeviceInfo, _ := json.Marshal(deviceInfo)
		h.nc.Publish("device.v1."+tenantSlug+".new", fmtDeviceInfo)
	}
	err := h.db.CreateDevice(deviceInfo)
	if err != nil {
		log.Printf("Failed to create device: %v", err)
	}
	// Publish online event for every connect (used by controller for campaign checks)
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

func parseDeviceInfoMsg(sn, subject string, data []byte, mtp db.MTP) db.Device {
	var record usp_record.Record
	var message usp_msg.Msg

	err := proto.Unmarshal(data, &record)
	if err != nil {
		log.Printf("ERROR: Failed to unmarshal record for device %s, subject: %s: %v", sn, subject, err)
		log.Fatal(err)
	}
	
	// Check what type of record we have
	noSessionContext := record.GetNoSessionContext()
	if noSessionContext == nil {
		// Determine record type for error message
		recordType := "unknown"
		if record.GetSessionContext() != nil {
			recordType = "SessionContext"
		} else if record.GetWebsocketConnect() != nil {
			recordType = "WebSocketConnect"
		} else if record.GetMqttConnect() != nil {
			recordType = "MQTTConnect"
		} else if record.GetStompConnect() != nil {
			recordType = "STOMPConnect"
		} else if record.GetDisconnect() != nil {
			recordType = "Disconnect"
		}
		
		// ERROR: .info handler only expects NoSessionContext records (USP messages)
		// Connection management records should be handled by .async handler
		log.Printf("ERROR: Received non-NoSessionContext record on .info subject for device %s. Record type: %s. Ignoring.", 
			sn, recordType)
		return db.Device{}
	}
	
	err = proto.Unmarshal(noSessionContext.Payload, &message)
	if err != nil {
		log.Printf("ERROR: Failed to unmarshal message payload for device %s, subject: %s: %v", sn, subject, err)
		log.Fatal(err)
	}

	msgType := message.Header.MsgType.String()

	// Check if message is a response, not a request
	response, ok := message.Body.MsgBody.(*usp_msg.Body_Response)
	if !ok {
		log.Printf("WARNING: Message on subject %s for device %s is not a response (it's a request). Message type: %s. Skipping device info parsing.", subject, sn, msgType)
		// Return a minimal device struct with just the serial number and MTP status
		var device db.Device
		device.SN = sn
		switch db.MTP(mtp) {
		case db.MQTT:
			device.Mqtt = db.Online
		case db.WEBSOCKETS:
			device.Websockets = db.Online
		case db.STOMP:
			device.Stomp = db.Online
		}
		device.Status = db.Online
		return device
	}

	var device db.Device
	msg := response.Response.GetGetResp()
	if msg == nil {
		log.Printf("WARNING: Response on subject %s for device %s is not a GET response (message type: %s). Skipping device info parsing.", 
			subject, sn, msgType)
		// Return a minimal device struct with just the serial number and MTP status
		device.SN = sn
		switch db.MTP(mtp) {
		case db.MQTT:
			device.Mqtt = db.Online
		case db.WEBSOCKETS:
			device.Websockets = db.Online
		case db.STOMP:
			device.Stomp = db.Online
		}
		device.Status = db.Online
		return device
	}

	device.Vendor = msg.ReqPathResults[0].ResolvedPathResults[0].ResultParams["Manufacturer"]
	device.Model = msg.ReqPathResults[1].ResolvedPathResults[0].ResultParams["ModelName"]
	device.Version = msg.ReqPathResults[2].ResolvedPathResults[0].ResultParams["SoftwareVersion"]
	device.ProductClass = msg.ReqPathResults[4].ResolvedPathResults[0].ResultParams["ProductClass"]
	device.HWVersion = msg.ReqPathResults[5].ResolvedPathResults[0].ResultParams["HardwareVersion"]
	device.SN = sn
	switch db.MTP(mtp) {
	case db.MQTT:
		device.Mqtt = db.Online
	case db.WEBSOCKETS:
		device.Websockets = db.Online
	case db.STOMP:
		device.Stomp = db.Online
	}

	device.Status = db.Online

	return device
}
