package storage_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage"
	"github.com/WinPooh32/insight/cmd/insight/internal/testutil"

	_ "modernc.org/sqlite"
)

func TestStoreAndRecent(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)

	ctx := context.Background()

	// Store an event
	evt := events.NewEnvelope(testutil.EventSessionStart, map[string]any{
		testutil.TestSessionID: "test-session-1",
		"prompt":               "hello",
	})
	if err := storageInst.Store(ctx, evt); err != nil {
		t.Fatalf("failed to store event: %v", err)
	}

	// Fetch recent events
	eventList, err := storageInst.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent events: %v", err)
	}

	if len(eventList) != 1 {
		t.Fatalf("expected 1 event, got %d", len(eventList))
	}

	if eventList[0].EventType != testutil.EventSessionStart {
		t.Errorf("expected event type %q, got %q", testutil.EventSessionStart, eventList[0].EventType)
	}
}

func TestStoreEmptySessionID(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)

	ctx := context.Background()

	// Store event without session_id
	evt := events.NewEnvelope(testutil.EventSessionStart, map[string]any{
		"prompt": "hello",
	})
	if err := storageInst.Store(ctx, evt); err != nil {
		t.Fatalf("failed to store event: %v", err)
	}

	// Fetch and verify session_id is empty/NULL
	eventList, err := storageInst.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent events: %v", err)
	}

	if len(eventList) != 1 {
		t.Fatalf("expected 1 event, got %d", len(eventList))
	}

	// The session_id should be nil (NULL in DB)
	if eventList[0].SessionID != nil {
		t.Errorf("expected nil session_id, got %q", *eventList[0].SessionID)
	}
}

func TestRecentLimitBoundary(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)

	ctx := context.Background()

	// Store an event
	evt := events.NewEnvelope(testutil.EventSessionStart, map[string]any{})
	if err := storageInst.Store(ctx, evt); err != nil {
		t.Fatalf("failed to store event: %v", err)
	}

	tests := []struct {
		name      string
		limit     int
		expectLen int
		expectErr bool
	}{
		{"limit=0 returns empty", 0, 0, false},
		{"limit=-1 returns error", -1, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			eventList, err := storageInst.Recent(ctx, tc.limit)
			if tc.expectErr {
				if err == nil {
					t.Error("expected error for negative limit")
				}

				return
			}

			if err != nil {
				t.Fatalf("failed to get recent events: %v", err)
			}

			if len(eventList) != tc.expectLen {
				t.Errorf("expected %d events, got %d", tc.expectLen, len(eventList))
			}
		})
	}
}

func TestDataIntegrity(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)

	ctx := context.Background()

	// Store event with known values
	evt := events.NewEnvelope(testutil.EventUserPrompt, map[string]any{
		testutil.TestSessionID: "integrity-session",
		"prompt":               "test payload",
		"nested":               map[string]any{"key": "value"},
	})
	if err := storageInst.Store(ctx, evt); err != nil {
		t.Fatalf("failed to store event: %v", err)
	}

	// Fetch and verify data integrity
	eventList, err := storageInst.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent events: %v", err)
	}

	if len(eventList) != 1 {
		t.Fatalf("expected 1 event, got %d", len(eventList))
	}

	stored := eventList[0]

	if stored.EventType != testutil.EventUserPrompt {
		t.Errorf("expected event type %q, got %q", testutil.EventUserPrompt, stored.EventType)
	}

	if stored.SessionID == nil || *stored.SessionID != "integrity-session" {
		t.Errorf("expected session_id 'integrity-session', got %v", stored.SessionID)
	}

	// Verify payload was stored
	if stored.Payload == "" {
		t.Error("expected non-empty payload")
	}
}

func TestBySession(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)

	ctx := context.Background()

	// Store events for two sessions
	for _, sessionID := range []string{"session-a", "session-b", "session-a"} {
		evt := events.NewEnvelope(testutil.EventUserPrompt, map[string]any{
			testutil.TestSessionID: sessionID,
		})
		if err := storageInst.Store(ctx, evt); err != nil {
			t.Fatalf("failed to store event: %v", err)
		}
	}

	// Query session-a
	eventList, err := storageInst.BySession(ctx, "session-a")
	if err != nil {
		t.Fatalf("failed to query by session: %v", err)
	}

	if len(eventList) != 2 {
		t.Fatalf("expected 2 events for session-a, got %d", len(eventList))
	}
}

func TestQueryEmptyResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		q    func(*storage.SQLiteStorage, context.Context) ([]events.StoredEvent, error)
	}{
		{
			name: "BySession nonexistent",
			q: func(s *storage.SQLiteStorage, ctx context.Context) ([]events.StoredEvent, error) {
				return s.BySession(ctx, "nonexistent-session")
			},
		},
		{
			name: "ByType nonexistent",
			q: func(s *storage.SQLiteStorage, ctx context.Context) ([]events.StoredEvent, error) {
				return s.ByType(ctx, "NonExistentType", 10, 0)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storageInst := testutil.NewTestStorage(t)
			ctx := context.Background()

			eventList, err := tc.q(storageInst, ctx)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if len(eventList) != 0 {
				t.Errorf("expected 0 events, got %d", len(eventList))
			}
		})
	}
}

func TestByType(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)

	ctx := context.Background()

	// Store events of different types
	for _, eventType := range []string{testutil.EventSessionStart, testutil.EventUserPrompt, testutil.EventUserPrompt} {
		evt := events.NewEnvelope(eventType, map[string]any{})
		if err := storageInst.Store(ctx, evt); err != nil {
			t.Fatalf("failed to store event: %v", err)
		}
	}

	// Query by type
	eventList, err := storageInst.ByType(ctx, testutil.EventUserPrompt, 10, 0)
	if err != nil {
		t.Fatalf("failed to query by type: %v", err)
	}

	if len(eventList) != 2 {
		t.Fatalf("expected 2 %s events, got %d", testutil.EventUserPrompt, len(eventList))
	}
}

func TestCount(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)

	ctx := context.Background()

	// Initial count should be 0
	count, err := storageInst.Count(ctx)
	if err != nil {
		t.Fatalf("failed to get count: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 events, got %d", count)
	}

	// Store an event and check count
	evt := events.NewEnvelope(testutil.EventSessionStart, map[string]any{})
	if err := storageInst.Store(ctx, evt); err != nil {
		t.Fatalf("failed to store event: %v", err)
	}

	count, err = storageInst.Count(ctx)
	if err != nil {
		t.Fatalf("failed to get count: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 event, got %d", count)
	}
}

func TestCountAfterMultipleStores(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)

	ctx := context.Background()

	// Store multiple events and verify count each time
	for i := range 5 {
		evt := events.NewEnvelope(testutil.EventUserPrompt, map[string]any{
			"index": strconv.Itoa(i),
		})
		if err := storageInst.Store(ctx, evt); err != nil {
			t.Fatalf("failed to store event %d: %v", i, err)
		}

		count, err := storageInst.Count(ctx)
		if err != nil {
			t.Fatalf("failed to get count after store %d: %v", i, err)
		}

		if count != int64(i+1) {
			t.Errorf("expected count %d after %d stores, got %d", i+1, i+1, count)
		}
	}
}

func TestNewSQLiteStorage(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := slog.New(slog.DiscardHandler)

	storageInst, err := storage.NewSQLiteStorage(context.Background(), dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storageInst.Close()

	// Verify the DB file was created
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected DB file to exist: %v", err)
	}
}

func TestStorageAfterClose(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := slog.New(slog.DiscardHandler)

	storageInst, err := storage.NewSQLiteStorage(context.Background(), dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Store an event before close
	evt := events.NewEnvelope(testutil.EventSessionStart, map[string]any{testutil.TestSessionID: "test"})
	if err := storageInst.Store(ctx, evt); err != nil {
		t.Fatalf("failed to store event before close: %v", err)
	}

	// Close the storage
	if err := storageInst.Close(); err != nil {
		t.Fatalf("failed to close storage: %v", err)
	}

	// Operations after close should fail
	_, err = storageInst.Recent(ctx, 10)
	if err == nil {
		t.Error("expected error when using storage after close")
	}

	_, err = storageInst.BySession(ctx, "test-session")
	if err == nil {
		t.Error("expected error when querying after close")
	}

	_, err = storageInst.ByType(ctx, "SessionStart", 10, 0)
	if err == nil {
		t.Error("expected error when querying by type after close")
	}

	count, err := storageInst.Count(ctx)
	if err == nil {
		t.Error("expected error when counting after close")
	}

	if count != 0 {
		t.Errorf("expected count 0 on error, got %d", count)
	}

	err = storageInst.Store(ctx, evt)
	if err == nil {
		t.Error("expected error when storing after close")
	}
}

func TestStorageLogging(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	var logBuf bytes.Buffer

	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	storageInst, err := storage.NewSQLiteStorage(context.Background(), dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storageInst.Close()

	// Verify that initialization was logged (kills mutation .17 which removes logger.Info)
	if !strings.Contains(logBuf.String(), "sqlite storage initialized") {
		t.Errorf("expected 'sqlite storage initialized' in log output, got: %s", logBuf.String())
	}

	// Store an event
	ctx := context.Background()

	evt := events.NewEnvelope(testutil.EventSessionStart, map[string]any{testutil.TestSessionID: "log-test"})
	if err := storageInst.Store(ctx, evt); err != nil {
		t.Fatalf("failed to store event: %v", err)
	}

	// Verify that "event stored" was logged (kills mutation .22 which removes InfoContext)
	if !strings.Contains(logBuf.String(), "event stored") {
		t.Errorf("expected 'event stored' in log output, got: %s", logBuf.String())
	}
}

func TestStoreLogsErrorAfterClose(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	var logBuf bytes.Buffer

	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	storageInst, err := storage.NewSQLiteStorage(context.Background(), dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Close the storage
	if err := storageInst.Close(); err != nil {
		t.Fatalf("failed to close storage: %v", err)
	}

	// Store after close should fail and log an error
	evt := events.NewEnvelope(testutil.EventSessionStart, map[string]any{testutil.TestSessionID: "log-test"})
	_ = storageInst.Store(ctx, evt)

	// Verify that "failed to store event" was logged (kills mutation .24 which removes ErrorContext)
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "failed to store event") {
		t.Errorf("expected 'failed to store event' in log output, got: %s", logOutput)
	}
}

func TestCountReturnsZeroOnError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := slog.New(slog.DiscardHandler)

	storageInst, err := storage.NewSQLiteStorage(context.Background(), dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	// Close to force error on next Count
	if err := storageInst.Close(); err != nil {
		t.Fatalf("failed to close storage: %v", err)
	}

	// Count after close should return error with count == 0
	// (kills mutations .13 and .15 which change the return value from 0 to -1/1)
	count, err := storageInst.Count(context.Background())
	if err == nil {
		t.Fatal("expected error from Count after close")
	}

	if count != 0 {
		t.Errorf("expected count 0 on error, got %d", count)
	}
}

func TestRecentOffsetZero(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)

	ctx := context.Background()

	// Store 5 events
	for i := range 5 {
		evt := events.NewEnvelope(testutil.EventUserPrompt, map[string]any{
			testutil.TestSessionID: "offset-test",
			"index":                strconv.Itoa(i),
		})
		if err := storageInst.Store(ctx, evt); err != nil {
			t.Fatalf("failed to store event %d: %v", i, err)
		}
	}

	// Recent with limit=3 should return exactly 3 (starting from offset 0)
	// Mutation .12 changes Offset from 0 to -1 which may change SQLite behavior
	eventList, err := storageInst.Recent(ctx, 3)
	if err != nil {
		t.Fatalf("failed to get recent events: %v", err)
	}

	if len(eventList) != 3 {
		t.Errorf("expected 3 recent events with limit=3, got %d", len(eventList))
	}

	// Recent with limit=100 should return all 5
	eventList, err = storageInst.Recent(ctx, 100)
	if err != nil {
		t.Fatalf("failed to get recent events: %v", err)
	}

	if len(eventList) != 5 {
		t.Errorf("expected 5 recent events with limit=100, got %d", len(eventList))
	}
}

func TestByTypeNegativeLimit(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)

	ctx := context.Background()

	// Negative limit should return an error
	_, err := storageInst.ByType(ctx, "TestType", -1, 0)
	if err == nil {
		t.Error("expected error for negative limit in ByType")
	}

	// Negative offset should return an error
	_, err = storageInst.ByType(ctx, "TestType", 10, -1)
	if err == nil {
		t.Error("expected error for negative offset in ByType")
	}
}

func TestByTypeOffsetPagination(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)

	ctx := context.Background()

	// Store 4 events with distinct identifiers
	for i := range 4 {
		evt := events.NewEnvelope(testutil.EventUserPrompt, map[string]any{
			testutil.TestSessionID: "pagination-test",
			"index":                strconv.Itoa(i),
		})
		if err := storageInst.Store(ctx, evt); err != nil {
			t.Fatalf("failed to store event %d: %v", i, err)
		}
	}

	// Get first page with offset=0, limit=2
	page1, err := storageInst.ByType(ctx, testutil.EventUserPrompt, 2, 0)
	if err != nil {
		t.Fatalf("failed to get first page: %v", err)
	}

	if len(page1) != 2 {
		t.Errorf("expected 2 events on first page, got %d", len(page1))
	}

	// Get second page with offset=2, limit=2
	page2, err := storageInst.ByType(ctx, testutil.EventUserPrompt, 2, 2)
	if err != nil {
		t.Fatalf("failed to get second page: %v", err)
	}

	if len(page2) != 2 {
		t.Errorf("expected 2 events on second page, got %d", len(page2))
	}

	// Pages should have different events
	if page1[0].ID == page2[0].ID {
		t.Error("expected different events on different pages")
	}

	// Third page should be empty
	page3, err := storageInst.ByType(ctx, testutil.EventUserPrompt, 2, 4)
	if err != nil {
		t.Fatalf("failed to get third page: %v", err)
	}

	if len(page3) != 0 {
		t.Errorf("expected 0 events on third page, got %d", len(page3))
	}
}
