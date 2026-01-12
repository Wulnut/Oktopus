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
func (d *Database) StoreUspMessage(ctx context.Context, msg UspMessage) error {
	_, err := d.messages.InsertOne(ctx, msg)
	if err != nil {
		log.Printf("Failed to store USP message: %v", err)
		return err
	}
	return nil
}

// GetMessageHistory retrieves message history for a device using cursor-based pagination
// fromTime and toTime are optional time filters. If nil, no time filtering is applied.
// filters is optional. If nil, no additional filtering is applied.
func (d *Database) GetMessageHistory(ctx context.Context, deviceSerial string, limit int, cursorID string, fromTime, toTime *time.Time, filters *MessageFilters) ([]UspMessage, string, error) {
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

				// Mix of known sources and unknown - use $in with all values including nil and empty
				allSources := make([]interface{}, len(sources)+2)
				for i, s := range sources {
					allSources[i] = s
				}
				allSources[len(sources)] = nil
				allSources[len(sources)+1] = ""
				filter["source"] = bson.M{"$in": allSources}
			} else if hasUnknown {
				// Only unknown selected
				filter["source"] = bson.M{"$in": []interface{}{nil, ""}}
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
				// Mix of known MTPs and unknown - use $in with all values including nil and empty
				allMtps := make([]interface{}, len(mtps)+2)
				for i, m := range mtps {
					allMtps[i] = m
				}
				allMtps[len(mtps)] = nil
				allMtps[len(mtps)+1] = ""
				filter["mtp"] = bson.M{"$in": allMtps}
			} else if hasUnknown {
				// Only unknown selected
				filter["mtp"] = bson.M{"$in": []interface{}{nil, ""}}
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

	cursor, err := d.messages.Find(ctx, filter, opts)
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
func (d *Database) GetMessageByMsgID(ctx context.Context, msgID string) (*UspMessage, error) {
	var result UspMessage
	err := d.messages.FindOne(ctx, bson.M{"msg_id": msgID}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrorMessageNotFound
		}
		return nil, err
	}
	return &result, nil
}

// StoreUspMessageError stores a failed message parsing/storage attempt
func (d *Database) StoreUspMessageError(ctx context.Context, errMsg UspMessageError) error {
	_, err := d.messagesErrors.InsertOne(ctx, errMsg)
	if err != nil {
		log.Printf("Failed to store error message: %v", err)
		return err
	}
	return nil
}

// DeleteMessageHistory deletes all messages and error messages for a device
// Returns the count of deleted messages and errors
func (d *Database) DeleteMessageHistory(ctx context.Context, deviceSerial string) (int64, int64, error) {
	// Delete from messages collection
	messagesResult, err := d.messages.DeleteMany(ctx, bson.M{"device_serial": deviceSerial})
	if err != nil {
		log.Printf("Failed to delete messages: %v", err)
		return 0, 0, err
	}
	
	// Delete from messages_errors collection
	errorsResult, err := d.messagesErrors.DeleteMany(ctx, bson.M{"device_serial": deviceSerial})
	if err != nil {
		log.Printf("Failed to delete error messages: %v", err)
		return messagesResult.DeletedCount, 0, err
	}
	
	log.Printf("Deleted %d messages and %d error messages for device %s", 
		messagesResult.DeletedCount, errorsResult.DeletedCount, deviceSerial)
	
	return messagesResult.DeletedCount, errorsResult.DeletedCount, nil
}

