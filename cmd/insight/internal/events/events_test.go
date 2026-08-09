package events_test

import (
	"encoding/json"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
)

func TestCamelToKebab(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"single lowercase", "a", "a"},
		{"single uppercase", "A", "a"},
		{"all lowercase", "lowercase", "lowercase"},
		{"all uppercase", "UPPERCASE", "u-p-p-e-r-c-a-s-e"},
		{"two capitals consecutive", "HTMLParser", "h-t-m-l-parser"},
		{"SessionStart", "SessionStart", "session-start"},
		{"UserPromptSubmit", "UserPromptSubmit", "user-prompt-submit"},
		{"PreToolUse", "PreToolUse", "pre-tool-use"},
		{"PostToolUseFailure", "PostToolUseFailure", "post-tool-use-failure"},
		{"Stop", "Stop", "stop"},
		{"ElicitationResult", "ElicitationResult", "elicitation-result"},
		{"AWSService", "AWSService", "a-w-s-service"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := events.CamelToKebab(tc.input)
			if result != tc.expected {
				t.Errorf("CamelToKebab(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestHookEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		event    string
		expected string
	}{
		{"SessionStart", "SessionStart", "/hooks/v1/session-start"},
		{"UserPromptSubmit", "UserPromptSubmit", "/hooks/v1/user-prompt-submit"},
		{"PreToolUse", "PreToolUse", "/hooks/v1/pre-tool-use"},
		{"Stop", "Stop", "/hooks/v1/stop"},
		{"ElicitationResult", "ElicitationResult", "/hooks/v1/elicitation-result"},
		{"empty string", "", "/hooks/v1/"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := events.HookEndpoint(tc.event)
			if result != tc.expected {
				t.Errorf("HookEndpoint(%q) = %q, want %q", tc.event, result, tc.expected)
			}
		})
	}
}

func TestNewEnvelopeWithoutSessionID(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"prompt": "hello world",
		"data":   "test",
	}

	envelope := events.NewEnvelope("SessionStart", payload)

	if envelope.EventType != "SessionStart" {
		t.Errorf("expected EventType 'SessionStart', got %q", envelope.EventType)
	}

	if envelope.SessionID != "" {
		t.Errorf("expected empty SessionID, got %q", envelope.SessionID)
	}

	if envelope.ID == "" {
		t.Error("expected non-empty ID")
	}

	if envelope.Received.IsZero() {
		t.Error("expected non-zero Received time")
	}
}

func TestNewEnvelopeWithSessionID(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"session_id": "my-session-123",
		"prompt":     "hello",
	}

	envelope := events.NewEnvelope("UserPromptSubmit", payload)

	if envelope.SessionID != "my-session-123" {
		t.Errorf("expected SessionID 'my-session-123', got %q", envelope.SessionID)
	}

	if envelope.EventType != "UserPromptSubmit" {
		t.Errorf("expected EventType 'UserPromptSubmit', got %q", envelope.EventType)
	}
}

func TestToJSON(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"session_id": "test-session",
		"prompt":     "hello world",
		"count":      float64(42),
	}

	envelope := events.NewEnvelope("SessionStart", payload)

	jsonBytes, err := envelope.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() returned error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if result["session_id"] != "test-session" {
		t.Errorf("expected session_id 'test-session', got %v", result["session_id"])
	}

	if result["prompt"] != "hello world" {
		t.Errorf("expected prompt 'hello world', got %v", result["prompt"])
	}
}

func TestToJSONComplexPayload(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"session_id": "complex-session",
		"nested": map[string]any{
			"key1": "value1",
			"key2": float64(123),
			"nested_deep": map[string]any{
				"deep_key": "deep_value",
			},
		},
		"array":   []any{"item1", "item2", float64(3)},
		"boolean": true,
		"null":    nil,
	}

	envelope := events.NewEnvelope("PreToolUse", payload)

	jsonBytes, err := envelope.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() returned error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if result["session_id"] != "complex-session" {
		t.Errorf("expected session_id 'complex-session', got %v", result["session_id"])
	}

	nested, ok := result["nested"].(map[string]any)
	if !ok {
		t.Fatal("expected nested to be a map")
	}

	if nested["key1"] != "value1" {
		t.Errorf("expected nested.key1 'value1', got %v", nested["key1"])
	}

	arr, ok := result["array"].([]any)
	if !ok {
		t.Fatal("expected array to be a slice")
	}

	if len(arr) != 3 {
		t.Errorf("expected array length 3, got %d", len(arr))
	}
}

func TestHookEvents(t *testing.T) {
	t.Parallel()

	events := events.HookEvents()

	if len(events) == 0 {
		t.Error("expected non-empty HookEvents list")
	}

	// Verify known events are present
	knownEvents := map[string]bool{
		"SessionStart":     false,
		"SessionEnd":       false,
		"UserPromptSubmit": false,
		"PreToolUse":       false,
		"Stop":             false,
	}

	for _, event := range events {
		if _, ok := knownEvents[event]; ok {
			knownEvents[event] = true
		}
	}

	for event, found := range knownEvents {
		if !found {
			t.Errorf("expected %q in HookEvents", event)
		}
	}
}
