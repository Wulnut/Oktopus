package usp_handler

import (
	"log"
	"strings"

	usp_record "github.com/OktopUSP/oktopus/backend/services/mtp/adapter/internal/usp/usp_record"
	"google.golang.org/protobuf/proto"
)

// HandleDeviceAsync handles asynchronous messages from devices (STOMPConnect, MQTTConnect, Disconnect, NOTIFY)
func (h Handler) HandleDeviceAsync(device, subject string, data []byte, mtp string, ack func()) {
	defer ack()

	var record usp_record.Record
	if err := proto.Unmarshal(data, &record); err != nil {
		log.Printf("[ASYNC] ERROR: Failed to unmarshal record for device %s: %v", device, err)
		return
	}

	// Forward all records to .api for interceptor
	// The interceptor handles both USP messages (with NoSessionContext) and connection records (without NoSessionContext)
	recordType := "USP message"
	if record.GetNoSessionContext() == nil {
		if record.GetMqttConnect() != nil {
			recordType = "MQTTConnect"
		} else if record.GetStompConnect() != nil {
			recordType = "STOMPConnect"
		} else if record.GetDisconnect() != nil {
			recordType = "Disconnect"
		} else {
			recordType = "connection record"
		}
	}

	log.Printf("[ASYNC] Received %s from device %s (mtp: %s), forwarding to .api", recordType, device, mtp)
	// Extract tenant slug from subject (e.g., mqtt-adapter.usp.v1.<tenant>.<sn>.async)
	parts := strings.Split(subject, ".")
	tenantSlug := ""
	if len(parts) >= 4 {
		tenantSlug = parts[3]
	}

	// Publish to MTP-specific .api subject to preserve MTP information for interceptor
	// The interceptor subscribes to mqtt.usp.v1.>, stomp.usp.v1.>, etc. and can extract MTP from subject
	publish_subject := mtp + ".usp.v1." + tenantSlug + "." + device + ".api"
	log.Printf("[ASYNC] Publishing %s to subject: %s, size: %d bytes", recordType, publish_subject, len(data))
	if err := h.nc.Publish(publish_subject, data); err != nil {
		log.Printf("[ASYNC] ERROR: Failed to publish %s for device %s to %s: %v", recordType, device, publish_subject, err)
	} else {
		log.Printf("[ASYNC] SUCCESS: Published %s for device %s to %s", recordType, device, publish_subject)
	}
}

