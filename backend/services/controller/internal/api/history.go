package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/utils"
)

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
func (a *Api) deviceClearHistory(w http.ResponseWriter, r *http.Request) {
	// Extract device serial from URL: /api/device/{sn}/history
	vars := mux.Vars(r)
	deviceSerial := vars["sn"]

	// Delete messages and errors
	messagesCount, errorsCount, err := a.db.DeleteMessageHistory(r.Context(), deviceSerial)
	if err != nil {
		log.Printf("Failed to delete message history: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(utils.Marshall(err.Error()))
		return
	}

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

