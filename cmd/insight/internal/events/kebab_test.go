package events_test

import (
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
)

func TestCamelToKebab(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "single upper", in: "A", want: "a"},
		{name: "camel", in: "UserPromptSubmit", want: "user-prompt-submit"},
		{name: "session start", in: "SessionStart", want: "session-start"},
		{name: "post tool use", in: "PostToolUse", want: "post-tool-use"},
		{name: "already kebab", in: "already-kebab", want: "already-kebab"},
		// Consecutive caps each get a hyphen: "HTTP" -> "h-t-t-p".
		{name: "consecutive caps", in: "HTTPServer", want: "h-t-t-p-server"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := events.CamelToKebab(tt.in); got != tt.want {
				t.Errorf("CamelToKebab(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHookEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		eventType string
		want      string
	}{
		{name: "session start", eventType: "SessionStart", want: "/hooks/v1/session-start"},
		{name: "user prompt submit", eventType: "UserPromptSubmit", want: "/hooks/v1/user-prompt-submit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := events.HookEndpoint(tt.eventType); got != tt.want {
				t.Errorf("HookEndpoint(%q) = %q, want %q", tt.eventType, got, tt.want)
			}
		})
	}
}
