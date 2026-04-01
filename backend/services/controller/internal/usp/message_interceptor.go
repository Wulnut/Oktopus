package usp

import (
	"context"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/leandrofars/oktopus/internal/bridge"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/entity"
	local "github.com/leandrofars/oktopus/internal/nats"
	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
	"github.com/leandrofars/oktopus/internal/usp/usp_record"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

// Device MTP cache with TTL
type deviceMTPCache struct {
	mu    sync.RWMutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	mtp       string
	expiresAt time.Time
}

var mtpCache = &deviceMTPCache{
	cache: make(map[string]cacheEntry),
}

const cacheTTL = 30 * time.Second // Cache device MTP for 30 seconds

// getDeviceMTP gets the MTP for a device, using cache if available
func getDeviceMTP(ctx context.Context, nc *nats.Conn, deviceSerial string) string {
	// Check cache first
	mtpCache.mu.RLock()
	if entry, ok := mtpCache.cache[deviceSerial]; ok {
		if time.Now().Before(entry.expiresAt) {
			mtpCache.mu.RUnlock()
			return entry.mtp
		}
		// Cache expired, remove it
		delete(mtpCache.cache, deviceSerial)
	}
	mtpCache.mu.RUnlock()

	// Query device info via NATS
	// TODO: pass tenant slug when message interceptor becomes tenant-aware
	msg, err := bridge.NatsReqWithoutHttpSet[entity.Device](
		local.NatsAdapterSubject("default")+deviceSerial+".device",
		[]byte(""),
		nc,
	)
	if err != nil || msg == nil {
		// If query fails, return "unknown" and cache it briefly to avoid repeated queries
		mtpCache.mu.Lock()
		mtpCache.cache[deviceSerial] = cacheEntry{
			mtp:       "unknown",
			expiresAt: time.Now().Add(5 * time.Second), // Cache "unknown" for shorter time
		}
		mtpCache.mu.Unlock()
		return "unknown"
	}

	device := msg.Msg
	var mtp string

	// Check which MTPs are online
	// Note: If multiple MTPs are active, we can't determine which one was used
	// for a specific message since all adapters publish to device.usp.v1.{sn}.api
	// We return the first active MTP found (priority: MQTT > WS > STOMP)
	// This is a limitation when devices are connected via multiple MTPs simultaneously
	var activeMTPs []string
	if device.Mqtt == entity.Online {
		activeMTPs = append(activeMTPs, entity.Mqtt)
	}
	if device.Websockets == entity.Online {
		activeMTPs = append(activeMTPs, entity.Websockets)
	}
	if device.Stomp == entity.Online {
		activeMTPs = append(activeMTPs, entity.Stomp)
	}

	if len(activeMTPs) == 0 {
		mtp = "unknown"
	} else if len(activeMTPs) == 1 {
		mtp = activeMTPs[0]
	} else {
		// Multiple MTPs active: return first one (priority order)
		// Could also return comma-separated list like "mqtt,stomp" if needed
		mtp = activeMTPs[0]
		// Log warning for debugging
		log.Printf("Device %s has multiple active MTPs: %v, using %s for message history", deviceSerial, activeMTPs, mtp)
	}

	// Cache the result
	mtpCache.mu.Lock()
	mtpCache.cache[deviceSerial] = cacheEntry{
		mtp:       mtp,
		expiresAt: time.Now().Add(cacheTTL),
	}
	mtpCache.mu.Unlock()

	return mtp
}

// StartMessageInterceptor subscribes to NATS subjects to intercept all USP messages
// IMPORTANT: Use Subscribe() (not QueueSubscribe()) to ensure all subscribers receive messages
func StartMessageInterceptor(ctx context.Context, nc *nats.Conn, d *db.TenantDB, controllerID string) {
	// Subscribe to all adapter-to-device subjects (messages being sent TO devices)
	// Pattern: "{mtp}-adapter.usp.v1.{sn}.api" where mtp in [mqtt, ws, stomp]
	patterns := []string{
		"mqtt-adapter.usp.v1.>",
		"ws-adapter.usp.v1.>",
		"stomp-adapter.usp.v1.>",
	}

	for _, pattern := range patterns {
		// Use Subscribe() to ensure both interceptor and existing handlers receive messages
		_, err := nc.Subscribe(pattern, func(msg *nats.Msg) {
			// Process asynchronously to avoid blocking
			go handleSentMessage(ctx, msg, d, controllerID)
		})
		if err != nil {
			log.Printf("Failed to subscribe to %s: %v", pattern, err)
		}
	}

	// Subscribe to all device-to-controller subjects (messages being received FROM devices)
	// Pattern: "device.usp.v1.{sn}.api" (used by WS adapter and some MQTT messages)
	_, err := nc.Subscribe("device.usp.v1.>", func(msg *nats.Msg) {
		// Process asynchronously to avoid blocking
		go handleReceivedMessage(ctx, msg, nc, d, controllerID)
	})
	if err != nil {
		log.Printf("Failed to subscribe to device.usp.v1.>: %v", err)
	}

	// Also subscribe to MTP-specific subjects for received messages
	// MQTT adapter routes some messages (e.g., from controller topic) to mqtt.usp.v1.{device}.info
	// WS adapter routes to ws.usp.v1.{device}.info for info messages
	// STOMP adapter routes to stomp.usp.v1.{device}.info for info messages
	mtpPatterns := []string{
		"mqtt.usp.v1.>",
		"ws.usp.v1.>",
		"stomp.usp.v1.>",
	}

	for _, pattern := range mtpPatterns {
		_, err := nc.Subscribe(pattern, func(msg *nats.Msg) {
			// Process asynchronously to avoid blocking
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
	// Option 1: Use regex (more robust)
	re := regexp.MustCompile(`\.usp\.v1\.(.+?)\.api$`)
	matches := re.FindStringSubmatch(subject)
	if len(matches) >= 2 {
		return matches[1] // Serial number (can contain dots)
	}

	// Option 2: Remove known prefixes/suffixes (fallback)
	prefixPatterns := []string{
		"device.usp.v1.",
		"mqtt-adapter.usp.v1.",
		"ws-adapter.usp.v1.",
		"stomp-adapter.usp.v1.",
	}
	for _, prefix := range prefixPatterns {
		if strings.HasPrefix(subject, prefix) {
			serial := strings.TrimPrefix(subject, prefix)
			serial = strings.TrimSuffix(serial, ".api")
			return serial
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

	// For device subjects: look up device MTP from device registry
	if strings.HasPrefix(subject, "device.usp.v1.") {
		if nc != nil && deviceSerial != "" && deviceSerial != "unknown" {
			return getDeviceMTP(ctx, nc, deviceSerial)
		}
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

