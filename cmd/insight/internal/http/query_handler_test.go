package httphandler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	insighthttp "github.com/WinPooh32/insight/cmd/insight/internal/http"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"

	_ "modernc.org/sqlite"
)

const eventsPath = "/hooks/v1/events"

func setupTestQueryRouter(t *testing.T) http.Handler {
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

func TestListEvents(t *testing.T) {
	t.Parallel()

	router := setupTestQueryRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Post an event first
	payload := map[string]any{
		testSessionID: "test-session",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		server.URL+userPrompt, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if resp, err := http.DefaultClient.Do(req); err != nil {
		t.Fatalf("failed to post event: %v", err)
	} else {
		resp.Body.Close()
	}

	// List events
	req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+eventsPath, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to list events: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var events []db.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode events: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestSessionEvents(t *testing.T) {
	t.Parallel()

	router := setupTestQueryRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Post events for a session
	for i := range 3 {
		payload := map[string]any{
			testSessionID: "session-abc",
			"index":       float64(i),
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
			server.URL+messageDisp, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		req.Header.Set("Content-Type", "application/json")

		if resp, err := http.DefaultClient.Do(req); err != nil {
			t.Fatalf("failed to post event: %v", err)
		} else {
			resp.Body.Close()
		}
	}

	// Query session events
	sessionURL, _ := url.JoinPath(server.URL, eventsPath, "session", "session-abc")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, sessionURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get session events: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var events []db.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode events: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
}
