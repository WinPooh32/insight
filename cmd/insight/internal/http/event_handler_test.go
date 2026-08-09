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
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	insighthttp "github.com/WinPooh32/insight/cmd/insight/internal/http"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage"

	_ "modernc.org/sqlite"
)

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

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["status"] != "received" {
		t.Errorf("expected status 'received', got %q", result["status"])
	}

	if result["id"] == "" {
		t.Error("expected non-empty id")
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
