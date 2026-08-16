package httphandler

import (
	"log/slog"
	"net/http"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage"
)

// Router returns an http.Handler with all routes registered. When
// injection is non-nil, the UserPromptSubmit and PreToolUse routes are
// served by it (storing the event and offering research context) instead
// of the generic ingest handler.
func Router(
	storage storage.Storage, filter events.AllowList, logger *slog.Logger, injection *InjectionHandler,
) http.Handler {
	mux := http.NewServeMux()

	eventHandler := NewEventHandler(storage, filter, logger)
	queryHandler := NewQueryHandler(storage, logger)

	// Health check
	mux.HandleFunc("GET /health", healthCheck)

	// Event query endpoints
	mux.HandleFunc("GET /hooks/v1/events", queryHandler.ListEvents)
	mux.HandleFunc("GET /hooks/v1/events/session/{session_id}", queryHandler.SessionEvents)

	// Injection endpoints for UPS and PTU.
	if injection != nil {
		mux.HandleFunc("POST "+events.HookEndpoint(userPromptSubmit), injection.UserPromptSubmit)
		mux.HandleFunc("POST "+events.HookEndpoint(preToolUse), injection.PreToolUse)
	}

	// Register POST endpoints for all hook events, skipping the
	// injection-served types when an injection handler is present.
	for _, eventType := range events.HookEvents() {
		// Capture loop variable
		et := eventType
		if injection != nil && (et == userPromptSubmit || et == preToolUse) {
			continue
		}

		path := events.HookEndpoint(et)
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			eventHandler.HandleEvent(w, r, et)
		})
	}

	return mux
}
