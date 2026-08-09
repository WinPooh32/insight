package httphandler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	insighthttp "github.com/WinPooh32/insight/cmd/insight/internal/http"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage"

	_ "modernc.org/sqlite"
)

func setupTestRouterWithLog(t *testing.T, logBuf *bytes.Buffer) http.Handler {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	var logger *slog.Logger
	if logBuf != nil {
		logger = slog.New(slog.NewTextHandler(logBuf, nil))
	} else {
		logger = slog.New(slog.DiscardHandler)
	}

	storageInst, err := storage.NewSQLiteStorage(context.Background(), dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	t.Cleanup(func() { storageInst.Close() })

	return insighthttp.Router(storageInst, logger)
}

const (
	healthPath   = "/health"
	sessionStart = "/hooks/v1/session-start"
	userPrompt   = "/hooks/v1/user-prompt-submit"
	messageDisp  = "/hooks/v1/message-display"
)

const (
	testSessionID     = "session_id"
	eventSessionStart = "SessionStart"
	eventUserPrompt   = "UserPromptSubmit"
)

func setupTestRouter(t *testing.T) http.Handler {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := slog.New(slog.DiscardHandler)

	storageInst, err := storage.NewSQLiteStorage(context.Background(), dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	t.Cleanup(func() { storageInst.Close() })

	return insighthttp.Router(storageInst, logger)
}

func TestHealthCheck(t *testing.T) {
	t.Parallel()

	router := setupTestRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+healthPath, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"status":"ok"}` {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestHandleEvent(t *testing.T) {
	t.Parallel()

	router := setupTestRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	payload := map[string]any{
		testSessionID: "test-session-123",
		"prompt":      "hello world",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		server.URL+sessionStart, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("expected status 202, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", resp.Header.Get("Content-Type"))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

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

	router := setupTestRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Test different event type endpoints
	endpoints := []string{
		sessionStart,
		userPrompt,
		messageDisp,
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			payload := map[string]any{
				testSessionID: "test-session",
				"data":        "test",
			}
			body, _ := json.Marshal(payload)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
				server.URL+endpoint, bytes.NewReader(body))
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("failed to post event: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusAccepted {
				t.Errorf("expected status 202 for %s, got %d", endpoint, resp.StatusCode)
			}

			if resp.Header.Get("Content-Type") != "application/json" {
				t.Errorf("expected Content-Type application/json for %s", endpoint)
			}
		})
	}
}

func TestHandleEventMaxPayloadSize(t *testing.T) {
	t.Parallel()

	router := setupTestRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Create a payload larger than 1MB (maxPayloadBytes)
	largePayload := make(map[string]any)
	largePayload["data"] = string(make([]byte, 1_100_000))
	body, _ := json.Marshal(largePayload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		server.URL+sessionStart, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for oversized payload, got %d", resp.StatusCode)
	}
}

func TestHandleEventLargePayloadAccepted(t *testing.T) {
	t.Parallel()

	router := setupTestRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Create a payload just under 1MB (maxPayloadBytes = 1MB)
	// Use printable characters to avoid JSON escaping overhead.
	// This should be accepted. Kills mutations that reduce maxPayloadBytes.
	data := strings.Repeat("A", 900_000) // ~900KB of 'A's, under 1MB after JSON encoding
	largePayload := map[string]any{"data": data}
	body, _ := json.Marshal(largePayload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		server.URL+sessionStart, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("expected status 202 for large-but-valid payload, got %d", resp.StatusCode)
	}
}

func TestHandleEventNoSessionID(t *testing.T) {
	t.Parallel()

	router := setupTestRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Post event without session_id
	payload := map[string]any{
		"prompt": "hello world",
		"data":   "test",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		server.URL+sessionStart, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("expected status 202, got %d", resp.StatusCode)
	}

	// Verify response still has id and status
	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["status"] != "received" {
		t.Errorf("expected status 'received', got %q", result["status"])
	}

	if result["id"] == "" {
		t.Error("expected non-empty id even without session_id")
	}
}

func TestHandleEventInvalidJSON(t *testing.T) {
	t.Parallel()

	router := setupTestRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		server.URL+sessionStart, bytes.NewReader([]byte("invalid")))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestKebabConversion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{eventSessionStart, "session-start"},
		{eventUserPrompt, "user-prompt-submit"},
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

	router := setupTestRouterWithLog(t, &logBuf)

	server := httptest.NewServer(router)
	defer server.Close()

	payload := map[string]any{
		testSessionID: "test-session-123",
		"prompt":      "hello world",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
		server.URL+sessionStart, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to post event: %v", err)
	}
	defer resp.Body.Close()

	// Verify success logging (kills mutation .7 which removes InfoContext)
	if !strings.Contains(logBuf.String(), "hook event received") {
		t.Errorf("expected 'hook event received' in logs, got: %s", logBuf.String())
	}
}

func TestHandleEventLogsInvalidJSON(t *testing.T) {
	t.Parallel()

	var logBuf bytes.Buffer

	router := setupTestRouterWithLog(t, &logBuf)

	server := httptest.NewServer(router)
	defer server.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
		server.URL+sessionStart, bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}

	// Verify invalid JSON warning was logged (kills mutation .11 which removes WarnContext)
	if !strings.Contains(logBuf.String(), "invalid JSON payload") {
		t.Errorf("expected 'invalid JSON payload' in logs, got: %s", logBuf.String())
	}
}

func TestHandleEventPayload2MBRejected(t *testing.T) {
	t.Parallel()

	router := setupTestRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Create a 2MB payload (exceeds 1MB maxPayloadBytes)
	// This should be rejected even if maxPayloadBytes is mutated to 2<<20 or 1<<21
	largePayload := make(map[string]any)
	largePayload["data"] = string(make([]byte, 2_200_000))
	body, _ := json.Marshal(largePayload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		server.URL+sessionStart, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for 2MB payload, got %d", resp.StatusCode)
	}
}
