package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/utils"
)

// deviceMessageHistory retrieves message history for a device
func (a *Api) deviceMessageHistory(w http.ResponseWriter, r *http.Request) {
	// Extract device serial from URL: /api/device/{sn}/history
	vars := mux.Vars(r)
	deviceSerial := vars["sn"]

	// Parse query params: limit (default 50), cursor (optional)
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}
	cursor := r.URL.Query().Get("cursor")

	// Call db.GetMessageHistory() with cursor-based pagination
	messages, nextCursor, err := a.db.GetMessageHistory(r.Context(), deviceSerial, limit, cursor)
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

