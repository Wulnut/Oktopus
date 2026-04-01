package usp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
	"github.com/leandrofars/oktopus/internal/usp/usp_record"
	"go.mongodb.org/mongo-driver/bson"
	"google.golang.org/protobuf/encoding/protojson"
)

// validateMessage validates a USP message before storage
func validateMessage(msg usp_msg.Msg, deviceSerial string) error {
	if deviceSerial == "" || deviceSerial == "unknown" {
		return fmt.Errorf("invalid device serial: %s", deviceSerial)
	}
	if msg.Header == nil {
		return fmt.Errorf("message header is nil")
	}
	if msg.Header.MsgId == "" {
		return fmt.Errorf("message ID is empty")
	}
	return nil
}

// StoreUspMessage stores a USP message with source detection
// source: "controller" (sent TO device - cannot distinguish localhost vs remote), "device" (received FROM device)
// mtp: "mqtt", "ws", "stomp", or "unknown" (extracted from NATS subject)
func StoreUspMessage(ctx context.Context, d *db.TenantDB, msg usp_msg.Msg, record usp_record.Record, deviceSerial, direction, source, mtp string) error {
	// Convert protobuf record to JSON
	// Using protojson to convert the record to JSON format
	protojsonMarshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
		UseProtoNames:   true,
	}
	
	recordJSONBytes, err := protojsonMarshaler.Marshal(&record)
	if err != nil {
		return fmt.Errorf("failed to marshal record to JSON: %w", err)
	}

	// Parse JSON bytes into bson.M for MongoDB storage
	var recordJSON bson.M
	if err := json.Unmarshal(recordJSONBytes, &recordJSON); err != nil {
		return fmt.Errorf("failed to unmarshal JSON to bson.M: %w", err)
	}

	// Convert the payload (USP message) to JSON and replace the base64 string
	// The payload is in no_session_context.payload or session_context_record.payload
	if noSessionContext, ok := recordJSON["no_session_context"].(map[string]interface{}); ok {
		if _, ok := noSessionContext["payload"].(string); ok {
			// Convert the message (which we already have parsed) to JSON
			msgJSONBytes, err := protojsonMarshaler.Marshal(&msg)
			if err != nil {
				return fmt.Errorf("failed to marshal message to JSON: %w", err)
			}
			var msgJSON bson.M
			if err := json.Unmarshal(msgJSONBytes, &msgJSON); err != nil {
				return fmt.Errorf("failed to unmarshal message JSON to bson.M: %w", err)
			}
			// Replace base64 payload with JSON object
			noSessionContext["payload"] = msgJSON
		}
	}
	// Handle session_context_record if needed (for future support)
	if sessionContext, ok := recordJSON["session_context_record"].(map[string]interface{}); ok {
		if _, ok := sessionContext["payload"].([]interface{}); ok {
			// For session context, payload is an array of bytes
			// Convert each payload segment if needed (for now, leave as is or convert)
			// This is less common, so we'll handle it if needed
		}
	}

	// Extract msg_id and msg_type from header
	msgID := msg.Header.MsgId
	msgType := msg.Header.MsgType.String()

	// Create UspMessage struct
	uspMsg := db.UspMessage{
		Timestamp:    time.Now(),
		DeviceSerial: deviceSerial,
		Direction:    direction,
		Source:       source,
		MTP:          mtp,
		MsgID:        msgID,
		MsgType:      msgType,
		FullRecord:   recordJSON, // Store as JSON (bson.M) with payload as JSON
	}

	// Store in database
	if err := d.StoreUspMessage(ctx, uspMsg); err != nil {
		log.Printf("Failed to store USP message: %v", err)
		return err
	}

	return nil
}

// StoreUspRecord stores a USP record (including Connect/Disconnect records without NoSessionContext)
// For records without NoSessionContext, recordType is used as msg_type and a generated ID is used as msg_id
func StoreUspRecord(ctx context.Context, d *db.TenantDB, record usp_record.Record, deviceSerial, direction, source, mtp, recordType string) error {
	// Convert protobuf record to JSON
	protojsonMarshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
		UseProtoNames:   true,
	}

	recordJSONBytes, err := protojsonMarshaler.Marshal(&record)
	if err != nil {
		return fmt.Errorf("failed to marshal record to JSON: %w", err)
	}

	// Parse JSON bytes into bson.M for MongoDB storage
	var recordJSON bson.M
	if err := json.Unmarshal(recordJSONBytes, &recordJSON); err != nil {
		return fmt.Errorf("failed to unmarshal JSON to bson.M: %w", err)
	}

	// For records with NoSessionContext, try to extract msg_id from payload if already converted to JSON
	msgID := fmt.Sprintf("%s-%d", recordType, time.Now().UnixNano())
	if noSessionContext, ok := recordJSON["no_session_context"].(map[string]interface{}); ok {
		if payload, ok := noSessionContext["payload"].(map[string]interface{}); ok {
			if header, ok := payload["header"].(map[string]interface{}); ok {
				if id, ok := header["msg_id"].(string); ok && id != "" {
					msgID = id
				}
			}
		}
	}

	// Create UspMessage struct
	uspMsg := db.UspMessage{
		Timestamp:    time.Now(),
		DeviceSerial: deviceSerial,
		Direction:    direction,
		Source:       source,
		MTP:          mtp,
		MsgID:        msgID,
		MsgType:      recordType,
		FullRecord:   recordJSON,
	}

	// Store in database
	if err := d.StoreUspMessage(ctx, uspMsg); err != nil {
		log.Printf("Failed to store USP record: %v", err)
		return err
	}

	return nil
}

