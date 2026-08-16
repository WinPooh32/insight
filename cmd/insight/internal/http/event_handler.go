package httphandler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage"
)

const maxPayloadBytes = 1 << 20 // 1 MB

// Store outcomes. errFiltered is a normal outcome (the event type is
// not in the allow list), not a failure.
var (
	errFiltered = errors.New("event type filtered")
	errDecode   = errors.New("decode payload")
	errStore    = errors.New("store event")
)

// EventHandler handles POST requests for hook events.
type EventHandler struct {
	storage     storage.Storage
	eventFilter events.AllowList
	log         *slog.Logger
}

// NewEventHandler creates a new EventHandler with the given storage, filter, and logger.
func NewEventHandler(storage storage.Storage, filter events.AllowList, logger *slog.Logger) *EventHandler {
	return &EventHandler{
		storage:     storage,
		eventFilter: filter,
		log:         logger,
	}
}

// Store decodes the request body into a hook event and, when the event
// type is allowed by the filter, stores it. It does not write a
// response. It returns the decoded envelope and an error: errFiltered
// when the type is filtered (nothing stored), errDecode on a decode
// failure, or errStore on a storage failure.
func (h *EventHandler) Store(w http.ResponseWriter, r *http.Request, eventType string) (events.Envelope, error) {
	var payload map[string]any
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPayloadBytes)).Decode(&payload); err != nil {
		h.log.WarnContext(r.Context(), "invalid JSON payload",
			"event_type", eventType,
			"error", err,
		)

		return events.Envelope{}, fmt.Errorf("%w: %w", errDecode, err)
	}

	envelope := events.NewEnvelope(eventType, payload)

	if !h.eventFilter.Allows(eventType) {
		h.log.InfoContext(r.Context(), "event filtered",
			"event_type", eventType,
		)

		return envelope, errFiltered
	}

	if err := h.storage.Store(r.Context(), envelope); err != nil {
		h.log.ErrorContext(r.Context(), "failed to store event",
			"event_type", eventType,
			"error", err,
		)

		return envelope, fmt.Errorf("%w: %w", errStore, err)
	}

	h.log.InfoContext(r.Context(), "hook event received",
		"event_type", eventType,
		"session_id", envelope.SessionID,
	)

	return envelope, nil
}

// HandleEvent processes an incoming hook event POST request.
func (h *EventHandler) HandleEvent(w http.ResponseWriter, r *http.Request, eventType string) {
	envelope, err := h.Store(w, r, eventType)

	switch {
	case errors.Is(err, errFiltered):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ignored"}`)
	case errors.Is(err, errDecode):
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
	case err != nil:
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, `{"id":"%s","status":"received"}`, envelope.ID)
	}
}
