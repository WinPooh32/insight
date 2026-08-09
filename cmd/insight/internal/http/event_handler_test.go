package httphandler_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	"github.com/WinPooh32/insight/cmd/insight/internal/testutil"
)

const (
	healthPath   = "/health"
	sessionStart = "/hooks/v1/session-start"
	userPrompt   = "/hooks/v1/user-prompt-submit"
	messageDisp  = "/hooks/v1/message-display"
)

func TestHealthCheck(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestEnv(t)

	resp := testutil.Get(t, server, healthPath)
	defer resp.Body.Close()

	testutil.AssertStatus(t, resp, http.StatusOK)

	body := testutil.ReadBody(resp)
	if string(body) != `{"status":"ok"}` {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestHandleEvent(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestEnv(t)

	payload := map[string]any{
		testutil.TestSessionID: "test-session-123",
		"prompt":               "hello world",
	}

	resp := testutil.PostJSON(t, server, sessionStart, payload)
	testutil.AssertStatus(t, resp, http.StatusAccepted)

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", resp.Header.Get("Content-Type"))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	resp.Body.Close()

	// Verify response contains both id and status
	var result map[string]string
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["status"] != "received" {
		t.Errorf("expected status 'received', got %q", result["status"])
	}

	if result["id"] == "" {
		t.Error("expected non-empty id")
	}

	// Verify the response body format contains expected fields
	responseStr := string(bodyBytes)
	if len(responseStr) < 2 {
		t.Error("response body too short")
	}
}

func TestHandleEventMultipleTypes(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestEnv(t)

	// Test different event type endpoints
	endpoints := []string{
		sessionStart,
		userPrompt,
		messageDisp,
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()

			payload := map[string]any{
				testutil.TestSessionID: "test-session",
				"data":                 "test",
			}

			resp := testutil.PostJSON(t, server, endpoint, payload)
			defer resp.Body.Close()

			testutil.AssertStatus(t, resp, http.StatusAccepted)

			if resp.Header.Get("Content-Type") != "application/json" {
				t.Errorf("expected Content-Type application/json for %s", endpoint)
			}

			testutil.ReadBody(resp)
		})
	}
}

func TestHandleEventPayloadSizes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		size       int
		expectCode int
	}{
		{"900KB accepted", 900_000, http.StatusAccepted},
		{"1.1MB rejected", 1_100_000, http.StatusBadRequest},
		{"2.2MB rejected", 2_200_000, http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := testutil.NewTestEnv(t)

			var payload map[string]any
			if tc.size == 900_000 {
				// Use printable characters for large accepted payload
				payload = map[string]any{"data": strings.Repeat("A", tc.size)}
			} else {
				payload = map[string]any{"data": string(make([]byte, tc.size))}
			}

			resp := testutil.PostJSON(t, server, sessionStart, payload)
			defer resp.Body.Close()

			testutil.AssertStatus(t, resp, tc.expectCode)
			testutil.ReadBody(resp)
		})
	}
}

func TestHandleEventNoSessionID(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestEnv(t)

	// Post event without session_id
	payload := map[string]any{
		"prompt": "hello world",
		"data":   "test",
	}

	resp := testutil.PostJSON(t, server, sessionStart, payload)
	testutil.AssertStatus(t, resp, http.StatusAccepted)

	// Verify response still has id and status
	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	resp.Body.Close()

	if result["status"] != "received" {
		t.Errorf("expected status 'received', got %q", result["status"])
	}

	if result["id"] == "" {
		t.Error("expected non-empty id even without session_id")
	}
}

func TestHandleEventInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		payload      []byte
		expectCode   int
		expectLog    string
		useLogRouter bool
	}{
		{
			name:         "invalid JSON returns 400",
			payload:      []byte("invalid"),
			expectCode:   http.StatusBadRequest,
			expectLog:    "",
			useLogRouter: false,
		},
		{
			name:         "invalid JSON logs warning",
			payload:      []byte("invalid"),
			expectCode:   http.StatusBadRequest,
			expectLog:    "invalid JSON payload",
			useLogRouter: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				logBuf bytes.Buffer
				server *httptest.Server
			)

			if tc.useLogRouter {
				router := testutil.NewTestRouterWithLog(t, &logBuf)
				server = testutil.NewTestServer(t, router)
			} else {
				server = testutil.NewTestEnv(t)
			}

			resp := testutil.PostRaw(t, server, sessionStart, tc.payload)
			defer resp.Body.Close()

			testutil.AssertStatus(t, resp, tc.expectCode)
			testutil.ReadBody(resp)

			if tc.expectLog != "" && !strings.Contains(logBuf.String(), tc.expectLog) {
				t.Errorf("expected %q in logs, got: %s", tc.expectLog, logBuf.String())
			}
		})
	}
}

func TestKebabConversion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{testutil.EventSessionStart, "session-start"},
		{testutil.EventUserPrompt, "user-prompt-submit"},
		{"PreToolUse", "pre-tool-use"},
		{"PostToolUseFailure", "post-tool-use-failure"},
		{"ElicitationResult", "elicitation-result"},
		{"Stop", "stop"},
	}

	for _, tc := range tests {
		result := events.CamelToKebab(tc.input)
		if result != tc.expected {
			t.Errorf("CamelToKebab(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestHandleEventLogsSuccess(t *testing.T) {
	t.Parallel()

	var logBuf bytes.Buffer

	router := testutil.NewTestRouterWithLog(t, &logBuf)
	server := testutil.NewTestServer(t, router)

	payload := map[string]any{
		testutil.TestSessionID: "test-session-123",
		"prompt":               "hello world",
	}

	resp := testutil.PostJSON(t, server, sessionStart, payload)
	defer resp.Body.Close()

	testutil.ReadBody(resp)

	// Verify success logging (kills mutation .7 which removes InfoContext)
	if !strings.Contains(logBuf.String(), "hook event received") {
		t.Errorf("expected 'hook event received' in logs, got: %s", logBuf.String())
	}
}
