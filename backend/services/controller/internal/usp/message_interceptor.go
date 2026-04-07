package usp

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/entity"
	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
	"github.com/leandrofars/oktopus/internal/usp/usp_record"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

// extractTenantSlug extracts tenant slug from NATS subject.
// Format: <prefix>.usp.v1.<tenant>.<serial>.<type>
func extractTenantSlug(subject string) string {
	parts := strings.Split(subject, ".")
	for i, p := range parts {
		if p == "v1" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return "default"
}

// StartMessageInterceptor subscribes to NATS subjects to intercept all USP messages
// IMPORTANT: Use Subscribe() (not QueueSubscribe()) to ensure all subscribers receive messages
func StartMessageInterceptor(ctx context.Context, nc *nats.Conn, database *db.Database, controllerID string) {
	// Subscribe to all adapter-to-device subjects (messages being sent TO devices)
	patterns := []string{
		"mqtt-adapter.usp.v1.>",
		"ws-adapter.usp.v1.>",
		"stomp-adapter.usp.v1.>",
	}

	for _, pattern := range patterns {
		_, err := nc.Subscribe(pattern, func(msg *nats.Msg) {
			tenantSlug := extractTenantSlug(msg.Subject)
			d := database.ForTenant(tenantSlug)
			go handleSentMessage(ctx, msg, d, controllerID)
		})
		if err != nil {
			log.Printf("Failed to subscribe to %s: %v", pattern, err)
		}
	}

	// Subscribe to all device-to-controller subjects
	sub, err := nc.Subscribe("device.usp.v1.>", func(msg *nats.Msg) {
		log.Printf("[interceptor-RECV] subject=%s size=%d", msg.Subject, len(msg.Data))
		tenantSlug := extractTenantSlug(msg.Subject)
		d := database.ForTenant(tenantSlug)
		go handleReceivedMessage(ctx, msg, nc, d, controllerID)
	})
	if err != nil {
		log.Printf("Failed to subscribe to device.usp.v1.>: %v", err)
	} else {
		log.Printf("Subscribed to device.usp.v1.> (sub=%v, valid=%v)", sub.Subject, sub.IsValid())
	}

	// Subscribe to MTP-specific subjects for received messages
	mtpPatterns := []string{
		"mqtt.usp.v1.>",
		"ws.usp.v1.>",
		"stomp.usp.v1.>",
	}

	for _, pattern := range mtpPatterns {
		_, err := nc.Subscribe(pattern, func(msg *nats.Msg) {
			tenantSlug := extractTenantSlug(msg.Subject)
			d := database.ForTenant(tenantSlug)
			go handleReceivedMessage(ctx, msg, nc, d, controllerID)
		})
		if err != nil {
			log.Printf("Failed to subscribe to %s: %v", pattern, err)
		}
	}

	log.Println("Message interceptor started")
}

// handleSentMessage processes messages being sent TO devices
func handleSentMessage(ctx context.Context, msg *nats.Msg, d *db.TenantDB, controllerID string) {
	deviceSerial := extractDeviceSerial(msg.Subject)

	// Parse record
	var record usp_record.Record
	if err := proto.Unmarshal(msg.Data, &record); err != nil {
		log.Printf("Failed to parse record: %v", err)
		storeError(ctx, d, msg, deviceSerial, "parse_error", err.Error())
		return
	}

	// Parse message from record payload
	var uspMsg usp_msg.Msg
	noSessionContext := record.GetNoSessionContext()
	if noSessionContext == nil {
		log.Printf("Record does not have NoSessionContext")
		storeError(ctx, d, msg, deviceSerial, "parse_error", "record does not have NoSessionContext")
		return
	}

	if err := proto.Unmarshal(noSessionContext.Payload, &uspMsg); err != nil {
		log.Printf("Failed to parse message: %v", err)
		storeError(ctx, d, msg, deviceSerial, "parse_error", err.Error())
		return
	}

	// Validate message
	if err := validateMessage(uspMsg, deviceSerial); err != nil {
		log.Printf("Message validation failed: %v", err)
		storeError(ctx, d, msg, deviceSerial, "validation_error", err.Error())
		return
	}

	// Detect source for sent messages
	source := detectSourceForSent(record, controllerID)

	// Extract MTP from subject (for sent messages, MTP is in the subject)
	mtp := extractMTP(ctx, msg.Subject, nil, "")

	// Store message
	if err := StoreUspMessage(ctx, d, uspMsg, record, deviceSerial, "sent", source, mtp); err != nil {
		log.Printf("Failed to store message: %v", err)
		storeError(ctx, d, msg, deviceSerial, "storage_error", err.Error())
	}
}

// handleReceivedMessage processes messages being received FROM devices
func handleReceivedMessage(ctx context.Context, msg *nats.Msg, nc *nats.Conn, d *db.TenantDB, controllerID string) {
	deviceSerial := extractDeviceSerial(msg.Subject)

	// Skip non-protobuf messages (e.g., status messages which are just "0" or "1")
	if len(msg.Data) < 2 {
		return
	}

	// Parse record
	var record usp_record.Record
	if err := proto.Unmarshal(msg.Data, &record); err != nil {
		log.Printf("ERROR: Failed to parse record for device %s: %v", deviceSerial, err)
		storeError(ctx, d, msg, deviceSerial, "parse_error", err.Error())
		return
	}

	// Detect source for received messages
	source := detectSourceForReceived(record, controllerID)

	// Extract MTP from subject (for device subjects, look up device MTP)
	mtp := extractMTP(ctx, msg.Subject, nc, deviceSerial)

	// Check if record has NoSessionContext (USP message) or is a connection management record
	noSessionContext := record.GetNoSessionContext()
	if noSessionContext != nil {
		// This is a USP message (NOTIFY, REGISTER, DEREGISTER, etc.)
		var uspMsg usp_msg.Msg
		if err := proto.Unmarshal(noSessionContext.Payload, &uspMsg); err != nil {
			log.Printf("ERROR: Failed to parse USP message for device %s: %v", deviceSerial, err)
			storeError(ctx, d, msg, deviceSerial, "parse_error", err.Error())
			return
		}

		// If MTP is unknown (response on device.usp.v1.*), look up from matching sent message
		if mtp == "unknown" && uspMsg.Header != nil && uspMsg.Header.MsgId != "" {
			if sentMsg, err := d.FindMessageByMsgID(ctx, deviceSerial, uspMsg.Header.MsgId); err == nil && sentMsg.MTP != "" && sentMsg.MTP != "unknown" {
				mtp = sentMsg.MTP
			}
		}

		// Validate message
		if err := validateMessage(uspMsg, deviceSerial); err != nil {
			log.Printf("ERROR: Message validation failed for device %s: %v", deviceSerial, err)
			storeError(ctx, d, msg, deviceSerial, "validation_error", err.Error())
			return
		}

		// Store message
		if err := StoreUspMessage(ctx, d, uspMsg, record, deviceSerial, "received", source, mtp); err != nil {
			log.Printf("ERROR: Failed to store message for device %s: %v", deviceSerial, err)
			storeError(ctx, d, msg, deviceSerial, "storage_error", err.Error())
		}
	} else {
		// This is a connection management record (MQTTConnect, STOMPConnect, Disconnect, etc.)
		recordType := "UNKNOWN_RECORD"
		if record.GetMqttConnect() != nil {
			recordType = "MQTTConnect"
		} else if record.GetStompConnect() != nil {
			recordType = "STOMPConnect"
		} else if record.GetWebsocketConnect() != nil {
			recordType = "WebSocketConnect"
		} else if record.GetDisconnect() != nil {
			recordType = "Disconnect"
		} else if record.GetSessionContext() != nil {
			recordType = "SessionContext"
		}

		if err := StoreUspRecord(ctx, d, record, deviceSerial, "received", source, mtp, recordType); err != nil {
			log.Printf("ERROR: Failed to store record for device %s: %v", deviceSerial, err)
			storeError(ctx, d, msg, deviceSerial, "storage_error", err.Error())
		}
	}
}

// detectSourceForSent determines source for messages sent TO device
func detectSourceForSent(record usp_record.Record, controllerID string) string {
	// Cannot distinguish between server and third-party when FromId is the same
	// Both localhost and remote clients can use the same FromId
	// Return generic "controller" source since we can't determine the actual source
	return "controller"
}

// detectSourceForReceived determines source for messages received FROM device
func detectSourceForReceived(record usp_record.Record, controllerID string) string {
	// Message is on device.usp.v1.{sn}.api, so it's FROM device
	// The subject tells us it's from device, regardless of FromId/ToId values
	return "device"
}

// extractDeviceSerial extracts device serial from NATS subject
// Pattern: "device.usp.v1.{serial}.api" or "{mtp}-adapter.usp.v1.{serial}.api"
// Serial can contain dots, so we need to use regex or prefix/suffix removal
func extractDeviceSerial(subject string) string {
	// Subject format: <prefix>.usp.v1.<tenant>.<serial>.<type>
	// Examples:
	//   device.usp.v1.prpl-test.mb_an7583_prpl401.api
	//   stomp.usp.v1.prpl-test.mb_an7583_prpl401.info
	//   mqtt-adapter.usp.v1.prpl-test.mb_an7583_prpl401.api
	parts := strings.Split(subject, ".")

	// Find "v1" position, then tenant is at v1+1, serial at v1+2
	for i, p := range parts {
		if p == "v1" && i+3 < len(parts) {
			// parts[i+1] = tenant, parts[i+2] = serial, parts[i+3] = type
			return parts[i+2]
		}
	}

	return "unknown"
}

// extractMTP extracts MTP (Media Type) from NATS subject
// Pattern: "{mtp}-adapter.usp.v1.{sn}.api" or "device.usp.v1.{sn}.api" or "{mtp}.usp.v1.{sn}.{type}"
// For device subjects, it looks up the device MTP from the device registry
func extractMTP(ctx context.Context, subject string, nc *nats.Conn, deviceSerial string) string {
	// For adapter subjects: extract MTP from prefix
	if strings.HasPrefix(subject, "mqtt-adapter.usp.v1.") {
		return entity.Mqtt
	}
	if strings.HasPrefix(subject, "ws-adapter.usp.v1.") {
		return entity.Websockets
	}
	if strings.HasPrefix(subject, "stomp-adapter.usp.v1.") {
		return entity.Stomp
	}

	// For MTP-specific subjects (mqtt.usp.v1., stomp.usp.v1., ws.usp.v1.): extract MTP from prefix
	if strings.HasPrefix(subject, "mqtt.usp.v1.") {
		return entity.Mqtt
	}
	if strings.HasPrefix(subject, "stomp.usp.v1.") {
		return entity.Stomp
	}
	if strings.HasPrefix(subject, "ws.usp.v1.") {
		return entity.Websockets
	}

	// For device subjects: MTP can't be determined from subject alone.
	// The sent message already captured MTP from the adapter-specific subject.
	if strings.HasPrefix(subject, "device.usp.v1.") {
		return "unknown"
	}

	return "unknown"
}

// storeError stores a failed message parsing/storage attempt
func storeError(ctx context.Context, d *db.TenantDB, msg *nats.Msg, deviceSerial, errorType, errorMessage string) {
	errMsg := db.UspMessageError{
		Timestamp:    time.Now(),
		DeviceSerial: deviceSerial,
		Subject:      msg.Subject,
		RawData:      msg.Data,
		ErrorMessage: errorMessage,
		ErrorType:    errorType,
	}
	if err := d.StoreUspMessageError(ctx, errMsg); err != nil {
		log.Printf("Failed to store error message: %v", err)
	}
}

