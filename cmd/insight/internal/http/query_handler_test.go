package httphandler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
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

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", resp.Header.Get("Content-Type"))
	}

	var events []db.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode events: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestListEventsLimitBoundary(t *testing.T) {
	t.Parallel()

	router := setupTestQueryRouter(t)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	// Post several events
	for i := range 5 {
		payload := map[string]any{
			testSessionID: "test-session",
			"index":       float64(i),
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
			server.URL+userPrompt, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	numEvents := 5

	tests := []struct {
		name       string
		limit      string
		expectLen  int
		expectCode int
	}{
		{"limit=0 uses default", "0", numEvents, http.StatusOK},
		{"limit=1001 uses default", "1001", numEvents, http.StatusOK},
		{"limit=abc uses default", "abc", numEvents, http.StatusOK},
		{"limit=2 returns 2", "2", 2, http.StatusOK},
		{"limit=5 returns all 5", "5", numEvents, http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			url := server.URL + eventsPath + "?limit=" + tc.limit

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("failed to list events: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectCode {
				t.Errorf("expected status %d, got %d", tc.expectCode, resp.StatusCode)
			}

			var events []db.Event
			if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
				t.Fatalf("failed to decode events: %v", err)
			}

			if len(events) != tc.expectLen {
				t.Errorf("expected %d events, got %d", tc.expectLen, len(events))
			}
		})
	}
}

func TestListEventsEventTypeFilter(t *testing.T) {
	t.Parallel()

	router := setupTestQueryRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Post events of different types
	payload1 := map[string]any{testSessionID: "sess-1"}

	body1, err := json.Marshal(payload1)
	if err != nil {
		t.Fatalf("failed to marshal payload1: %v", err)
	}

	req1, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
		server.URL+sessionStart, bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	resp1, _ := http.DefaultClient.Do(req1)
	resp1.Body.Close()

	for range 2 {
		payload2 := map[string]any{testSessionID: "sess-1", "prompt": "hello"}

		body2, err := json.Marshal(payload2)
		if err != nil {
			t.Fatalf("failed to marshal payload2: %v", err)
		}

		req2, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
			server.URL+userPrompt, bytes.NewReader(body2))
		req2.Header.Set("Content-Type", "application/json")
		resp2, _ := http.DefaultClient.Do(req2)
		resp2.Body.Close()
	}

	// Filter by SessionStart
	getURL := server.URL + eventsPath + "?event_type=SessionStart"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, getURL, nil)
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
		t.Errorf("expected 1 SessionStart event, got %d", len(events))
	}

	if len(events) > 0 && events[0].EventType != "SessionStart" {
		t.Errorf("expected event type SessionStart, got %q", events[0].EventType)
	}
}

func TestListEventsOffset(t *testing.T) {
	t.Parallel()

	router := setupTestQueryRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Post 3 events
	for range 3 {
		payload := map[string]any{testSessionID: "test-session"}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
			server.URL+messageDisp, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	// Get with offset=1, limit=2
	getURL := server.URL + eventsPath + "?limit=2&offset=1"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, getURL, nil)
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

	if len(events) != 2 {
		t.Errorf("expected 2 events with offset=1 limit=2, got %d", len(events))
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

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", resp.Header.Get("Content-Type"))
	}

	var events []db.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode events: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
}

func TestSessionEventsEmptySessionID(t *testing.T) {
	t.Parallel()

	router := setupTestQueryRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Query session events with empty session_id
	// The router pattern {session_id} won't match empty path, so we get 404
	sessionURL, _ := url.JoinPath(server.URL, eventsPath, "session", "")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, sessionURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get session events: %v", err)
	}
	defer resp.Body.Close()

	// The router doesn't match empty {session_id}, so we get 404
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestListEventsDefaultLimitExact(t *testing.T) {
	t.Parallel()

	router := setupTestQueryRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Post 60 events to test default limit of 50
	for range 60 {
		payload := map[string]any{testSessionID: "test-session"}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
			server.URL+messageDisp, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	// No limit → default of 50
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+eventsPath, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to list events: %v", err)
	}
	defer resp.Body.Close()

	var events []db.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode events: %v", err)
	}

	if len(events) != 50 {
		t.Errorf("expected exactly 50 events (default limit), got %d", len(events))
	}
}

func TestListEventsLimitMaxBoundary(t *testing.T) {
	t.Parallel()

	router := setupTestQueryRouter(t)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	// Post 60 events to test max limit boundary
	for range 60 {
		payload := map[string]any{testSessionID: "test-session"}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
			server.URL+messageDisp, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	tests := []struct {
		name      string
		limit     string
		expectLen int
	}{
		{"limit=1000 uses 1000 (not default)", "1000", 60},
		{"limit=1001 uses default 50", "1001", 50},
		{"limit=999 uses 999", "999", 60},
		{"limit=3 returns 3", "3", 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			getURL := server.URL + eventsPath + "?limit=" + tc.limit

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, getURL, nil)
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

			if len(events) != tc.expectLen {
				t.Errorf("expected %d events, got %d", tc.expectLen, len(events))
			}
		})
	}
}

func TestListEventsOffsetSkipsEvents(t *testing.T) {
	t.Parallel()

	router := setupTestQueryRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Post 3 UserPromptSubmit events to test offset with ByType
	for range 3 {
		payload := map[string]any{testSessionID: "test-session"}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
			server.URL+userPrompt, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	// Get first event without offset (using event_type to trigger ByType which supports offset)
	getURL := server.URL + eventsPath + "?event_type=UserPromptSubmit&limit=1"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, getURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp1, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to list events: %v", err)
	}

	var events1 []db.Event
	if err := json.NewDecoder(resp1.Body).Decode(&events1); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	resp1.Body.Close()

	if len(events1) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events1))
	}

	// Get second event with offset=1
	getURL = server.URL + eventsPath + "?event_type=UserPromptSubmit&limit=1&offset=1"

	req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, getURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to list events: %v", err)
	}

	var events2 []db.Event
	if err := json.NewDecoder(resp2.Body).Decode(&events2); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	resp2.Body.Close()

	if len(events2) != 1 {
		t.Fatalf("expected 1 event with offset=1, got %d", len(events2))
	}

	// The events should be different (offset should skip the first)
	if events1[0].ID == events2[0].ID {
		t.Error("offset=1 should return a different event than offset=0")
	}
}

func TestListEventsEventTypeFilterAllMatch(t *testing.T) {
	t.Parallel()

	router := setupTestQueryRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Post events of different types
	for range 3 {
		payload := map[string]any{testSessionID: "sess-1"}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
			server.URL+sessionStart, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	for range 2 {
		payload := map[string]any{testSessionID: "sess-1", "prompt": "hello"}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
			server.URL+userPrompt, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	// Filter by UserPromptSubmit and verify ALL match
	getURL := server.URL + eventsPath + "?event_type=UserPromptSubmit&limit=10"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, getURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to list events: %v", err)
	}
	defer resp.Body.Close()

	var events []db.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode events: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 UserPromptSubmit events, got %d", len(events))
	}

	// Verify ALL returned events match the filter
	for _, e := range events {
		if e.EventType != "UserPromptSubmit" {
			t.Errorf("expected all events to be UserPromptSubmit, got %q", e.EventType)
		}
	}
}

func TestListEventsDefaultOffset(t *testing.T) {
	t.Parallel()

	router := setupTestQueryRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	// Post 3 events
	for range 3 {
		payload := map[string]any{testSessionID: "test-session"}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
			server.URL+messageDisp, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	// Get events without offset - should return all 3 (offset defaults to 0)
	// Mutation .19 changes default offset from 0 to -1
	getURL := server.URL + eventsPath + "?limit=10"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, getURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to list events: %v", err)
	}
	defer resp.Body.Close()

	var events []db.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode events: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("expected 3 events with default offset, got %d", len(events))
	}
}

func TestHandleEventInvalidJSONBody(t *testing.T) {
	t.Parallel()

	router := setupTestRouter(t)

	server := httptest.NewServer(router)
	defer server.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
		server.URL+sessionStart, bytes.NewReader([]byte("not json at all")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}

	// Verify the error response body contains expected error message
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "invalid JSON") {
		t.Errorf("expected 'invalid JSON' in response body, got: %s", string(body))
	}
}
