package events_test

import (
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
)

func TestAllowList_EmptyAllowsAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     []string
		eventType string
	}{
		{"nil input", nil, "SessionStart"},
		{"empty input", []string{}, "SessionStart"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := events.NewAllowList(tc.input)
			if !a.Allows(tc.eventType) {
				t.Errorf("expected empty allowlist to allow %q", tc.eventType)
			}
		})
	}
}

func TestAllowList_AllowsListedEvents(t *testing.T) {
	t.Parallel()

	a := events.NewAllowList([]string{"SessionStart", "UserPromptSubmit", "Stop"})

	allowed := []string{"SessionStart", "UserPromptSubmit", "Stop"}
	for _, e := range allowed {
		t.Run(e, func(t *testing.T) {
			t.Parallel()

			if !a.Allows(e) {
				t.Errorf("expected allowlist to allow %q", e)
			}
		})
	}
}

func TestAllowList_DeniesUnlistedEvents(t *testing.T) {
	t.Parallel()

	a := events.NewAllowList([]string{"SessionStart", "Stop"})

	denied := []string{"UserPromptSubmit", "MessageDisplay", "Notification"}
	for _, e := range denied {
		t.Run(e, func(t *testing.T) {
			t.Parallel()

			if a.Allows(e) {
				t.Errorf("expected allowlist to deny %q", e)
			}
		})
	}
}

func TestAllowList_CaseSensitive(t *testing.T) {
	t.Parallel()

	a := events.NewAllowList([]string{"SessionStart"})

	if a.Allows("sessionstart") {
		t.Error("expected allowlist to be case sensitive")
	}

	if a.Allows("session-start") {
		t.Error("expected allowlist to be case sensitive")
	}

	if !a.Allows("SessionStart") {
		t.Error("expected exact match to be allowed")
	}
}
