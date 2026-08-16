package httphandler

import (
	"net/http"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
)

// Router returns an http.Handler with the UserPromptSubmit and
// PreToolUse injection routes registered. When injection is nil, the
// returned handler serves no routes.
func Router(injection *InjectionHandler) http.Handler {
	mux := http.NewServeMux()

	if injection != nil {
		mux.HandleFunc("POST "+events.HookEndpoint(userPromptSubmit), injection.UserPromptSubmit)
		mux.HandleFunc("POST "+events.HookEndpoint(preToolUse), injection.PreToolUse)
	}

	return mux
}
