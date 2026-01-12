package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/utils"
)

// Allowed message types for validation
var allowedMessageTypes = map[string]bool{
	"GET": true, "GET_RESP": true, "SET": true, "SET_RESP": true,
	"ADD": true, "ADD_RESP": true, "DELETE": true, "DELETE_RESP": true,
	"OPERATE": true, "OPERATE_RESP": true, "NOTIFY": true, "NOTIFY_RESP": true,
	"STOMPConnect": true, "MQTTConnect": true, "Disconnect": true, "WebSocketConnect": true,
	"GET_SUPPORTED_DM": true, "GET_SUPPORTED_DM_RESP": true,
	"GET_INSTANCES": true, "GET_INSTANCES_RESP": true,
	"GET_SUPPORTED_PROTO": true, "GET_SUPPORTED_PROTO_RESP": true,
	"REGISTER": true, "REGISTER_RESP": true,
	"DEREGISTER": true, "DEREGISTER_RESP": true,
	"ERROR": true, "SessionContext": true, "UNKNOWN_RECORD": true,
}

// Allowed sources for validation
var allowedSources = map[string]bool{
	"controller": true,
	"device":     true,
	"unknown":    true,
}

// Allowed MTPs for validation
var allowedMTPs = map[string]bool{
	"mqtt":    true,
	"ws":      true,
	"stomp":   true,
	"unknown": true,
}

// deviceMessageHistory retrieves message history for a device
func (a *Api) deviceMessageHistory(w http.ResponseWriter, r *http.Request) {
	// Extract device serial from URL: /api/device/{sn}/history
	vars := mux.Vars(r)
	deviceSerial := vars["sn"]

	// Parse query params: limit (default 50), cursor (optional), from (optional), to (optional)
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}
	cursor := r.URL.Query().Get("cursor")

	// Parse date filters
	// datetime-local format: "2006-01-02T15:04" (no timezone, treated as UTC)
	var fromTime *time.Time
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		// Parse as UTC since datetime-local doesn't include timezone info
		parsed, err := time.Parse("2006-01-02T15:04", fromStr)
		if err == nil {
			// Treat as UTC
			parsed = time.Date(parsed.Year(), parsed.Month(), parsed.Day(),
				parsed.Hour(), parsed.Minute(), 0, 0, time.UTC)
			fromTime = &parsed
		}
	}

	var toTime *time.Time
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		// Parse as UTC since datetime-local doesn't include timezone info
		parsed, err := time.Parse("2006-01-02T15:04", toStr)
		if err == nil {
			// Treat as UTC and set to end of the selected minute (59 seconds, 999 milliseconds)
			// This ensures we include all messages up to and including the selected minute
			parsed = time.Date(parsed.Year(), parsed.Month(), parsed.Day(),
				parsed.Hour(), parsed.Minute(), 59, 999000000, time.UTC)
			toTime = &parsed
		}
	}

	// Validate date range: fromTime must be <= toTime
	if fromTime != nil && toTime != nil && fromTime.After(*toTime) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(utils.Marshall("Invalid date range: 'from' date must be before or equal to 'to' date"))
		return
	}

	// Parse filter query parameters
	var filters *db.MessageFilters
	msgTypes := r.URL.Query()["msg_type"]
	sources := r.URL.Query()["source"]
	mtps := r.URL.Query()["mtp"]
	msgID := r.URL.Query().Get("msg_id")
	msgIDExact := r.URL.Query().Get("msg_id_exact") == "true"

	// Filter out empty strings from arrays (they indicate "empty" filter)
	// Empty arrays after filtering mean "return nothing" for that filter category
	filterEmptyStrings := func(arr []string) []string {
		result := []string{}
		for _, s := range arr {
			if s != "" {
				result = append(result, s)
			}
		}
		return result
	}

	msgTypesFiltered := filterEmptyStrings(msgTypes)
	sourcesFiltered := filterEmptyStrings(sources)
	mtpsFiltered := filterEmptyStrings(mtps)

	// Validate filter values against allowed sets
	// Validate message types
	for _, msgType := range msgTypesFiltered {
		if !allowedMessageTypes[msgType] {
			w.WriteHeader(http.StatusBadRequest)
			w.Write(utils.Marshall("Invalid message type: " + msgType))
			return
		}
	}

	// Validate sources
	for _, source := range sourcesFiltered {
		if !allowedSources[source] {
			w.WriteHeader(http.StatusBadRequest)
			w.Write(utils.Marshall("Invalid source: " + source))
			return
		}
	}

	// Validate MTPs
	for _, mtp := range mtpsFiltered {
		if !allowedMTPs[mtp] {
			w.WriteHeader(http.StatusBadRequest)
			w.Write(utils.Marshall("Invalid MTP: " + mtp))
			return
		}
	}

	// Validate message ID length (prevent extremely long strings)
	if len(msgID) > 500 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(utils.Marshall("Message ID too long (max 500 characters)"))
		return
	}

	// Sanitize message ID: remove any potentially dangerous characters for regex
	// (This is already handled by escapeRegex, but we validate length here)
	if msgID != "" && strings.ContainsAny(msgID, "\x00\n\r") {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(utils.Marshall("Invalid message ID: contains invalid characters"))
		return
	}

	// Create filters struct if any filter parameter is present
	// If parameter exists but is empty (after filtering), it means "return nothing"
	_, hasMsgType := r.URL.Query()["msg_type"]
	_, hasSource := r.URL.Query()["source"]
	_, hasMtp := r.URL.Query()["mtp"]
	hasMsgID := msgID != ""

	if hasMsgType || hasSource || hasMtp || hasMsgID {
		filters = &db.MessageFilters{
			MessageTypes:  msgTypesFiltered,
			Sources:       sourcesFiltered,
			MTPs:          mtpsFiltered,
			MessageID:     msgID,
			MessageIDExact: msgIDExact,
		}
	}

	// Call db.GetMessageHistory() with cursor-based pagination, date filters, and message filters
	messages, nextCursor, err := a.db.GetMessageHistory(r.Context(), deviceSerial, limit, cursor, fromTime, toTime, filters)
	if err != nil {
		log.Printf("Failed to get message history: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(utils.Marshall(err.Error()))
		return
	}

	// Return JSON response
	response := map[string]interface{}{
		"messages":    messages,
		"next_cursor": nextCursor,
		"has_more":    nextCursor != "",
	}

	// Set content type and encode JSON
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(utils.Marshall("Failed to encode response"))
		return
	}
}

// deviceClearHistory deletes all message history for a device
// TODO: Add proper role-based authorization (e.g., admin-only or device owner check)
// Currently, any authenticated user can delete any device's history
func (a *Api) deviceClearHistory(w http.ResponseWriter, r *http.Request) {
	// Extract device serial from URL: /api/device/{sn}/history
	vars := mux.Vars(r)
	deviceSerial := vars["sn"]

	// Get user email from context (set by authentication middleware)
	userEmail := r.Context().Value("email")
	if userEmail == nil {
		// This should not happen if middleware is working correctly, but check anyway
		log.Printf("Warning: Clear History called without authenticated user")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(utils.Marshall("Unauthorized: authentication required"))
		return
	}

	// Log the action for audit purposes
	log.Printf("User %v clearing message history for device %s", userEmail, deviceSerial)

	// TODO: Add authorization check here
	// Example: Check if user is admin or owns the device
	// if !isAdmin(userEmail) && !ownsDevice(userEmail, deviceSerial) {
	//     w.WriteHeader(http.StatusForbidden)
	//     w.Write(utils.Marshall("Forbidden: insufficient permissions"))
	//     return
	// }

	// Delete messages and errors
	messagesCount, errorsCount, err := a.db.DeleteMessageHistory(r.Context(), deviceSerial)
	if err != nil {
		log.Printf("Failed to delete message history for device %s by user %v: %v", deviceSerial, userEmail, err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(utils.Marshall(err.Error()))
		return
	}

	// Log successful deletion
	log.Printf("User %v successfully deleted %d messages and %d errors for device %s",
		userEmail, messagesCount, errorsCount, deviceSerial)

	// Return success response with counts
	response := map[string]interface{}{
		"success":        true,
		"messages_count": messagesCount,
		"errors_count":   errorsCount,
		"total_count":    messagesCount + errorsCount,
	}

	// Set content type and encode JSON
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(utils.Marshall("Failed to encode response"))
		return
	}
}

