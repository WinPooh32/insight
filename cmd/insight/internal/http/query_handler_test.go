package httphandler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	"github.com/WinPooh32/insight/cmd/insight/internal/testutil"
)

const eventsPath = "/hooks/v1/events"

func TestListEvents(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestEnv(t)

	// Post an event first
	{
		resp := testutil.PostJSON(t, server, userPrompt, map[string]any{
			testutil.TestSessionID: "test-session",
		})
		resp.Body.Close()
	}

	// List events
	resp := testutil.Get(t, server, eventsPath)
	testutil.AssertStatus(t, resp, http.StatusOK)

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", resp.Header.Get("Content-Type"))
	}

	var eventList []events.StoredEvent
	if err := json.NewDecoder(resp.Body).Decode(&eventList); err != nil {
		t.Fatalf("failed to decode events: %v", err)
	}

	resp.Body.Close()

	if len(eventList) != 1 {
		t.Errorf("expected 1 event, got %d", len(eventList))
	}
}

func TestListEventsLimits(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestEnv(t)

	// Post 60 events to test all limit boundaries
	for range 60 {
		resp := testutil.PostJSON(t, server, messageDisp, map[string]any{
			testutil.TestSessionID: "test-session",
		})
		resp.Body.Close()
	}

	tests := []struct {
		name      string
		limit     string
		expectLen int
	}{
		{"limit=0 uses default", "0", 50},
		{"limit=1001 uses default", "1001", 50},
		{"limit=abc uses default", "abc", 50},
		{"limit=1000 uses 1000", "1000", 60},
		{"limit=999 uses 999", "999", 60},
		{"limit=3 returns 3", "3", 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			getURL := eventsPath + "?limit=" + tc.limit
			resp := testutil.Get(t, server, getURL)
			testutil.AssertStatus(t, resp, http.StatusOK)

			var eventList []events.StoredEvent
			if err := json.NewDecoder(resp.Body).Decode(&eventList); err != nil {
				t.Fatalf("failed to decode events: %v", err)
			}

			resp.Body.Close()

			if len(eventList) != tc.expectLen {
				t.Errorf("expected %d events, got %d", tc.expectLen, len(eventList))
			}
		})
	}
}

func TestListEventsEventTypeFilter(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestEnv(t)

	// Post 1 SessionStart + 2 UserPromptSubmit
	{
		resp := testutil.PostJSON(t, server, sessionStart, map[string]any{
			testutil.TestSessionID: "sess-1",
		})
		resp.Body.Close()
	}

	for range 2 {
		resp := testutil.PostJSON(t, server, userPrompt, map[string]any{
			testutil.TestSessionID: "sess-1",
			"prompt":               "hello",
		})
		resp.Body.Close()
	}

	tests := []struct {
		name       string
		filter     string
		expectN    int
		expectType string
	}{
		{"filter SessionStart returns 1", "SessionStart", 1, "SessionStart"},
		{"filter UserPromptSubmit returns 2", "UserPromptSubmit", 2, "UserPromptSubmit"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			getURL := eventsPath + "?event_type=" + tc.filter
			resp := testutil.Get(t, server, getURL)
			testutil.AssertStatus(t, resp, http.StatusOK)

			var eventList []events.StoredEvent
			if err := json.NewDecoder(resp.Body).Decode(&eventList); err != nil {
				t.Fatalf("failed to decode events: %v", err)
			}

			resp.Body.Close()

			if len(eventList) != tc.expectN {
				t.Errorf("expected %d %s events, got %d", tc.expectN, tc.filter, len(eventList))
			}

			// Verify ALL returned events match the filter
			for _, e := range eventList {
				if e.EventType != tc.expectType {
					t.Errorf("expected all events to be %s, got %q", tc.expectType, e.EventType)
				}
			}
		})
	}
}

func TestListEventsOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		postN     int
		getURL    string
		expectLen int
	}{
		{
			name:      "offset=1 limit=2 returns 2",
			postN:     3,
			getURL:    eventsPath + "?limit=2&offset=1",
			expectLen: 2,
		},
		{
			name:      "default offset returns all",
			postN:     3,
			getURL:    eventsPath + "?limit=10",
			expectLen: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := testutil.NewTestEnv(t)

			// Post events
			for range tc.postN {
				resp := testutil.PostJSON(t, server, messageDisp, map[string]any{
					testutil.TestSessionID: "test-session",
				})
				resp.Body.Close()
			}

			resp := testutil.Get(t, server, tc.getURL)
			testutil.AssertStatus(t, resp, http.StatusOK)

			var eventList []events.StoredEvent
			if err := json.NewDecoder(resp.Body).Decode(&eventList); err != nil {
				t.Fatalf("failed to decode events: %v", err)
			}

			resp.Body.Close()

			if len(eventList) != tc.expectLen {
				t.Errorf("expected %d events, got %d", tc.expectLen, len(eventList))
			}
		})
	}
}

func TestSessionEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		postEvents   bool
		sessionID    string
		expectCode   int
		expectEvents int
	}{
		{
			name:         "existing session returns events",
			postEvents:   true,
			sessionID:    "session-abc",
			expectCode:   http.StatusOK,
			expectEvents: 3,
		},
		{
			name:         "empty session ID returns 404",
			postEvents:   false,
			sessionID:    "",
			expectCode:   http.StatusNotFound,
			expectEvents: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := testutil.NewTestEnv(t)

			if tc.postEvents {
				for i := range 3 {
					resp := testutil.PostJSON(t, server, messageDisp, map[string]any{
						testutil.TestSessionID: tc.sessionID,
						"index":                float64(i),
					})
					resp.Body.Close()
				}
			}

			sessionURL, _ := url.JoinPath(server.URL, eventsPath, "session", tc.sessionID)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, sessionURL, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("failed to get session events: %v", err)
			}

			defer resp.Body.Close()

			if resp.StatusCode != tc.expectCode {
				t.Errorf("expected status %d, got %d", tc.expectCode, resp.StatusCode)
			}

			if tc.expectCode != http.StatusOK {
				return
			}

			if resp.Header.Get("Content-Type") != "application/json" {
				t.Errorf("expected Content-Type application/json, got %q", resp.Header.Get("Content-Type"))
			}

			var eventList []events.StoredEvent
			if err := json.NewDecoder(resp.Body).Decode(&eventList); err != nil {
				t.Fatalf("failed to decode events: %v", err)
			}

			if len(eventList) != tc.expectEvents {
				t.Errorf("expected %d events, got %d", tc.expectEvents, len(eventList))
			}
		})
	}
}

func TestListEventsDefaultLimitExact(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestEnv(t)

	// Post 60 events to test default limit of 50
	for range 60 {
		resp := testutil.PostJSON(t, server, messageDisp, map[string]any{
			testutil.TestSessionID: "test-session",
		})
		resp.Body.Close()
	}

	// No limit → default of 50
	resp := testutil.Get(t, server, eventsPath)

	var eventList []events.StoredEvent
	if err := json.NewDecoder(resp.Body).Decode(&eventList); err != nil {
		t.Fatalf("failed to decode events: %v", err)
	}

	resp.Body.Close()

	if len(eventList) != 50 {
		t.Errorf("expected exactly 50 events (default limit), got %d", len(eventList))
	}
}

func TestListEventsOffsetSkipsEvents(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestEnv(t)

	// Post 3 UserPromptSubmit events to test offset with ByType
	for range 3 {
		resp := testutil.PostJSON(t, server, userPrompt, map[string]any{
			testutil.TestSessionID: "test-session",
		})
		resp.Body.Close()
	}

	// Get first event without offset (using event_type to trigger ByType which supports offset)
	resp1 := testutil.Get(t, server, eventsPath+"?event_type=UserPromptSubmit&limit=1")

	var eventList1 []events.StoredEvent
	if err := json.NewDecoder(resp1.Body).Decode(&eventList1); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	resp1.Body.Close()

	if len(eventList1) != 1 {
		t.Fatalf("expected 1 event, got %d", len(eventList1))
	}

	// Get second event with offset=1
	resp2 := testutil.Get(t, server, eventsPath+"?event_type=UserPromptSubmit&limit=1&offset=1")

	var eventList2 []events.StoredEvent
	if err := json.NewDecoder(resp2.Body).Decode(&eventList2); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	resp2.Body.Close()

	if len(eventList2) != 1 {
		t.Fatalf("expected 1 event with offset=1, got %d", len(eventList2))
	}

	// The events should be different (offset should skip the first)
	if eventList1[0].ID == eventList2[0].ID {
		t.Error("offset=1 should return a different event than offset=0")
	}
}

func TestHandleEventInvalidJSONBody(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestEnv(t)

	resp := testutil.PostRaw(t, server, sessionStart, []byte("not json at all"))
	defer resp.Body.Close()

	testutil.AssertStatus(t, resp, http.StatusBadRequest)

	body := testutil.ReadBody(resp)
	if !strings.Contains(string(body), "invalid JSON") {
		t.Errorf("expected 'invalid JSON' in response body, got: %s", string(body))
	}
}
