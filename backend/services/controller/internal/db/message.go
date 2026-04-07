package db

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UspMessage represents a stored USP message
type UspMessage struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Timestamp    time.Time           `bson:"timestamp" json:"timestamp"`
	DeviceSerial string              `bson:"device_serial" json:"device_serial"`
	Direction    string              `bson:"direction" json:"direction"` // "sent" or "received"
	Source       string              `bson:"source" json:"source"`       // "controller" or "device"
	MTP          string              `bson:"mtp" json:"mtp"`              // "mqtt", "ws", "stomp", or "unknown"
	MsgID        string              `bson:"msg_id" json:"msg_id"`
	MsgType      string              `bson:"msg_type" json:"msg_type"` // OPERATE, NOTIFY, etc.
	FullRecord   bson.M              `bson:"full_record" json:"full_record"` // JSON representation of the protobuf record
}

// UspMessageError represents a failed message parsing/storage attempt
type UspMessageError struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Timestamp    time.Time          `bson:"timestamp" json:"timestamp"`
	DeviceSerial string             `bson:"device_serial" json:"device_serial"`
	Subject      string             `bson:"subject" json:"subject"`
	RawData      []byte             `bson:"raw_data" json:"raw_data"`
	ErrorMessage string             `bson:"error_message" json:"error_message"`
	ErrorType    string             `bson:"error_type" json:"error_type"` // "parse_error", "validation_error", "storage_error"
}

var ErrorMessageNotFound = errors.New("Message not found")

// MessageFilters represents filter criteria for message history queries
type MessageFilters struct {
	MessageTypes  []string // e.g., ["OPERATE", "GET"] - exact match only
	Sources       []string // e.g., ["controller", "device", "unknown"]
	MTPs          []string // e.g., ["mqtt", "ws", "unknown"]
	MessageID     string   // Message ID to search for
	MessageIDExact bool    // true = exact match, false = partial (substring) match
}

// escapeRegex escapes special regex characters to make a simple substring search
func escapeRegex(s string) string {
	specialChars := []string{"\\", "^", "$", ".", "|", "?", "*", "+", "(", ")", "[", "]", "{", "}"}
	result := s
	for _, char := range specialChars {
		result = strings.ReplaceAll(result, char, "\\"+char)
	}
	return result
}

// StoreUspMessage stores a USP message in the database
func (t *TenantDB) StoreUspMessage(ctx context.Context, msg UspMessage) error {
	_, err := t.Messages().InsertOne(ctx, msg)
	if err != nil {
		log.Printf("Failed to store USP message: %v", err)
		return err
	}
	return nil
}

// GetMessageHistory retrieves message history for a device using cursor-based pagination
// fromTime and toTime are optional time filters. If nil, no time filtering is applied.
// filters is optional. If nil, no additional filtering is applied.
func (t *TenantDB) GetMessageHistory(ctx context.Context, deviceSerial string, limit int, cursorID string, fromTime, toTime *time.Time, filters *MessageFilters) ([]UspMessage, string, error) {
	filter := bson.M{"device_serial": deviceSerial}

	// Add timestamp filters
	if fromTime != nil || toTime != nil {
		timestampFilter := bson.M{}
		if fromTime != nil {
			timestampFilter["$gte"] = *fromTime
		}
		if toTime != nil {
			timestampFilter["$lte"] = *toTime
		}
		if len(timestampFilter) > 0 {
			filter["timestamp"] = timestampFilter
		}
	}

	// Add message filters if provided
	if filters != nil {
		// Add message type filter - EXACT MATCH ONLY
		// Empty array means "return nothing" for this filter
		if len(filters.MessageTypes) > 0 {
			filter["msg_type"] = bson.M{"$in": filters.MessageTypes}
		} else {
			// Explicitly empty array - return no results for this filter
			filter["msg_type"] = bson.M{"$in": []string{}}
		}

		// Add source filter - handle "unknown" separately
		// Empty array means "return nothing" for this filter
		if len(filters.Sources) > 0 {
			hasUnknown := false
			sources := []string{}
			for _, s := range filters.Sources {
				if s == "unknown" {
					hasUnknown = true
				} else {
					sources = append(sources, s)
				}
			}

			if hasUnknown && len(sources) > 0 {
				// Mix of known sources and unknown
				allSources := make([]interface{}, len(sources)+3)
				for i, s := range sources {
					allSources[i] = s
				}
				allSources[len(sources)] = "unknown"
				allSources[len(sources)+1] = nil
				allSources[len(sources)+2] = ""
				filter["source"] = bson.M{"$in": allSources}
			} else if hasUnknown {
				// Only unknown selected
				filter["source"] = bson.M{"$in": []interface{}{"unknown", nil, ""}}
			} else {
				// Only known sources selected
				filter["source"] = bson.M{"$in": sources}
			}
		} else {
			// Explicitly empty array - return no results for this filter
			filter["source"] = bson.M{"$in": []interface{}{}}
		}

		// Add MTP filter - handle "unknown" separately
		// Empty array means "return nothing" for this filter
		if len(filters.MTPs) > 0 {
			hasUnknown := false
			mtps := []string{}
			for _, m := range filters.MTPs {
				if m == "unknown" {
					hasUnknown = true
				} else {
					mtps = append(mtps, m)
				}
			}

			if hasUnknown && len(mtps) > 0 {
				// Mix of known MTPs and unknown
				allMtps := make([]interface{}, len(mtps)+3)
				for i, m := range mtps {
					allMtps[i] = m
				}
				allMtps[len(mtps)] = "unknown"
				allMtps[len(mtps)+1] = nil
				allMtps[len(mtps)+2] = ""
				filter["mtp"] = bson.M{"$in": allMtps}
			} else if hasUnknown {
				// Only unknown selected
				filter["mtp"] = bson.M{"$in": []interface{}{"unknown", nil, ""}}
			} else {
				// Only known MTPs selected
				filter["mtp"] = bson.M{"$in": mtps}
			}
		} else {
			// Explicitly empty array - return no results for this filter
			filter["mtp"] = bson.M{"$in": []interface{}{}}
		}

		// Add message ID filter - exact or partial (substring)
		if filters.MessageID != "" {
			if filters.MessageIDExact {
				// Exact match - uses index
				filter["msg_id"] = filters.MessageID
			} else {
				// Partial match - substring search (case-insensitive)
				// WARNING: $regex queries cannot efficiently use indexes and may cause full collection scans
				// on large datasets. For better performance, consider:
				// 1. Using exact match when possible
				// 2. Adding a text index on msg_id field for full-text search
				// 3. Limiting the dataset size with other filters (date range, device_serial, etc.)
				// Escape special regex characters to make it a simple substring search
				escaped := escapeRegex(filters.MessageID)
				filter["msg_id"] = bson.M{"$regex": escaped, "$options": "i"}
			}
		}
	}

	// If cursor provided, add it to filter
	if cursorID != "" {
		objectID, err := primitive.ObjectIDFromHex(cursorID)
		if err != nil {
			return nil, "", err
		}
		filter["_id"] = bson.M{"$lt": objectID} // Get messages before this ID (newest first)
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "_id", Value: -1}}). // Sort by _id descending (newest first)
		SetLimit(int64(limit + 1))                 // Fetch one extra to check if there's more

	cursor, err := t.Messages().Find(ctx, filter, opts)
	if err != nil {
		return nil, "", err
	}
	defer cursor.Close(ctx)

	var messages []UspMessage
	if err = cursor.All(ctx, &messages); err != nil {
		return nil, "", err
	}

	// Check if there are more messages
	// We fetched limit+1 to check if there are more
	var nextCursor string
	if len(messages) > limit {
		// We got more than limit, so there are more messages
		// Set cursor to the (limit+1)th message's ID (the extra one we fetched)
		nextCursor = messages[limit].ID.Hex()
		// Return only the first limit messages
		messages = messages[:limit]
	}
	// If len(messages) <= limit, we got all remaining messages, so no cursor needed

	return messages, nextCursor, nil
}

// GetMessageByMsgID retrieves a message by its message ID
func (t *TenantDB) GetMessageByMsgID(ctx context.Context, msgID string) (*UspMessage, error) {
	var result UspMessage
	err := t.Messages().FindOne(ctx, bson.M{"msg_id": msgID}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrorMessageNotFound
		}
		return nil, err
	}
	return &result, nil
}

// StoreUspMessageError stores a failed message parsing/storage attempt
func (t *TenantDB) StoreUspMessageError(ctx context.Context, errMsg UspMessageError) error {
	_, err := t.MessagesErrors().InsertOne(ctx, errMsg)
	if err != nil {
		log.Printf("Failed to store error message: %v", err)
		return err
	}
	return nil
}

// DeleteMessageHistory deletes all messages and error messages for a device
// Attempts to use MongoDB transaction if replica set is available, otherwise falls back to sequential deletes
// Returns the count of deleted messages and errors
func (t *TenantDB) DeleteMessageHistory(ctx context.Context, deviceSerial string) (int64, int64, error) {
	// Try to use transaction if replica set is available
	client := t.Usp.Client()
	session, err := client.StartSession()
	if err != nil {
		log.Printf("Failed to start session: %v", err)
		return t.deleteMessageHistoryWithoutTransaction(ctx, deviceSerial)
	}
	defer session.EndSession(ctx)

	var messagesCount, errorsCount int64

	// Try to execute with transaction
	err = mongo.WithSession(ctx, session, func(sc mongo.SessionContext) error {
		// Try to start transaction
		if err := session.StartTransaction(); err != nil {
			// If transaction fails (e.g., not a replica set), fall back to non-transactional
			log.Printf("Transaction not available (likely standalone MongoDB), using fallback: %v", err)
			return err
		}

		// Delete from messages collection
		messagesResult, err := t.Messages().DeleteMany(sc, bson.M{"device_serial": deviceSerial})
		if err != nil {
			session.AbortTransaction(sc)
			log.Printf("Failed to delete messages in transaction: %v", err)
			return err
		}
		messagesCount = messagesResult.DeletedCount

		// Delete from messages_errors collection
		errorsResult, err := t.MessagesErrors().DeleteMany(sc, bson.M{"device_serial": deviceSerial})
		if err != nil {
			session.AbortTransaction(sc)
			log.Printf("Failed to delete error messages in transaction: %v", err)
			return err
		}
		errorsCount = errorsResult.DeletedCount

		// Commit transaction
		if err := session.CommitTransaction(sc); err != nil {
			log.Printf("Failed to commit transaction: %v", err)
			return err
		}

		return nil
	})

	// If transaction failed (e.g., standalone MongoDB), use fallback
	if err != nil {
		log.Printf("Transaction not available, using fallback method for device %s", deviceSerial)
		return t.deleteMessageHistoryWithoutTransaction(ctx, deviceSerial)
	}

	log.Printf("Deleted %d messages and %d error messages for device %s (using transaction)",
		messagesCount, errorsCount, deviceSerial)

	return messagesCount, errorsCount, nil
}

// deleteMessageHistoryWithoutTransaction deletes messages without using transactions
// Used as fallback when MongoDB is not configured as a replica set
func (t *TenantDB) deleteMessageHistoryWithoutTransaction(ctx context.Context, deviceSerial string) (int64, int64, error) {
	// Delete from messages collection
	messagesResult, err := t.Messages().DeleteMany(ctx, bson.M{"device_serial": deviceSerial})
	if err != nil {
		log.Printf("Failed to delete messages: %v", err)
		return 0, 0, err
	}
	messagesCount := messagesResult.DeletedCount

	// Delete from messages_errors collection
	errorsResult, err := t.MessagesErrors().DeleteMany(ctx, bson.M{"device_serial": deviceSerial})
	if err != nil {
		log.Printf("Failed to delete error messages: %v", err)
		// Return messages count even if errors deletion failed
		// This is acceptable since they're separate collections
		return messagesCount, 0, err
	}
	errorsCount := errorsResult.DeletedCount

	log.Printf("Deleted %d messages and %d error messages for device %s (without transaction)",
		messagesCount, errorsCount, deviceSerial)

	return messagesCount, errorsCount, nil
}

