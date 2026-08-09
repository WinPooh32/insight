package httphandler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/WinPooh32/insight/cmd/insight/internal/storage"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
)

const (
	defaultEventLimit = 50
	maxEventLimit     = 1000
)

// QueryHandler handles GET requests for querying events.
type QueryHandler struct {
	storage storage.Storage
	log     *slog.Logger
}

// NewQueryHandler creates a new QueryHandler with the given storage and logger.
func NewQueryHandler(storage storage.Storage, logger *slog.Logger) *QueryHandler {
	return &QueryHandler{
		storage: storage,
		log:     logger,
	}
}

// ListEvents returns paginated recent events.
func (h *QueryHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	limit := defaultEventLimit

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	if limit <= 0 || limit > maxEventLimit {
		limit = defaultEventLimit
	}

	offset := 0

	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil {
			offset = parsed
		}
	}

	var (
		eventList []db.Event
		err       error
	)

	if eventType := r.URL.Query().Get("event_type"); eventType != "" {
		eventList, err = h.storage.ByType(r.Context(), eventType, limit, offset)
	} else {
		eventList, err = h.storage.Recent(r.Context(), limit)
	}

	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to query events", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(eventList); err != nil {
		h.log.ErrorContext(r.Context(), "failed to encode events", "error", err)
	}
}

// SessionEvents returns all events for a given session.
func (h *QueryHandler) SessionEvents(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	if sessionID == "" {
		http.Error(w, `{"error":"session_id required"}`, http.StatusBadRequest)
		return
	}

	eventList, err := h.storage.BySession(r.Context(), sessionID)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to query session events",
			"error", err,
			"session_id", sessionID,
		)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(eventList); err != nil {
		h.log.ErrorContext(r.Context(), "failed to encode events", "error", err)
	}
}
