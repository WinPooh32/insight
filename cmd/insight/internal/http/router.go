package httphandler

import (
	"log/slog"
	"net/http"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage"
)

// Router returns an http.Handler with all routes registered.
func Router(storage storage.Storage, filter events.AllowList, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	eventHandler := NewEventHandler(storage, filter, logger)
	queryHandler := NewQueryHandler(storage, logger)

	// Health check
	mux.HandleFunc("GET /health", healthCheck)

	// Event query endpoints
	mux.HandleFunc("GET /hooks/v1/events", queryHandler.ListEvents)
	mux.HandleFunc("GET /hooks/v1/events/session/{session_id}", queryHandler.SessionEvents)

	// Register POST endpoints for all hook events
	for _, eventType := range events.HookEvents() {
		// Capture loop variable
		et := eventType
		path := events.HookEndpoint(et)
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			eventHandler.HandleEvent(w, r, et)
		})
	}

	return mux
}
