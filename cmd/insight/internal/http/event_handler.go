package httphandler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage"
)

const maxPayloadBytes = 1 << 20 // 1 MB

// EventHandler handles POST requests for hook events.
type EventHandler struct {
	storage storage.Storage
	log     *slog.Logger
}

// NewEventHandler creates a new EventHandler with the given storage and logger.
func NewEventHandler(storage storage.Storage, logger *slog.Logger) *EventHandler {
	return &EventHandler{
		storage: storage,
		log:     logger,
	}
}

// HandleEvent processes an incoming hook event POST request.
func (h *EventHandler) HandleEvent(w http.ResponseWriter, r *http.Request, eventType string) {
	var payload map[string]any
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPayloadBytes)).Decode(&payload); err != nil {
		h.log.WarnContext(r.Context(), "invalid JSON payload",
			"event_type", eventType,
			"error", err,
		)
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)

		return
	}

	envelope := events.NewEnvelope(eventType, payload)

	if err := h.storage.Store(r.Context(), envelope); err != nil {
		h.log.ErrorContext(r.Context(), "failed to store event",
			"event_type", eventType,
			"error", err,
		)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)

		return
	}

	h.log.InfoContext(r.Context(), "hook event received",
		"event_type", eventType,
		"session_id", envelope.SessionID,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"id":"%s","status":"received"}`, envelope.ID)
}
