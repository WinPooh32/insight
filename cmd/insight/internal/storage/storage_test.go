package storage_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage"

	_ "modernc.org/sqlite"
)

const (
	testSessionID     = "session_id"
	eventSessionStart = "SessionStart"
	eventUserPrompt   = "UserPromptSubmit"
)

func setupTestStorage(t *testing.T) *storage.SQLiteStorage {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := slog.New(slog.DiscardHandler)

	storageInst, err := storage.NewSQLiteStorage(context.Background(), dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	t.Cleanup(func() { storageInst.Close() })

	return storageInst
}

func TestStoreAndRecent(t *testing.T) {
	t.Parallel()

	storageInst := setupTestStorage(t)

	ctx := context.Background()

	// Store an event
	evt := events.NewEnvelope(eventSessionStart, map[string]any{
		testSessionID: "test-session-1",
		"prompt":      "hello",
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

	if eventList[0].EventType != eventSessionStart {
		t.Errorf("expected event type %q, got %q", eventSessionStart, eventList[0].EventType)
	}
}

func TestBySession(t *testing.T) {
	t.Parallel()

	storageInst := setupTestStorage(t)

	ctx := context.Background()

	// Store events for two sessions
	for _, sessionID := range []string{"session-a", "session-b", "session-a"} {
		evt := events.NewEnvelope(eventUserPrompt, map[string]any{
			testSessionID: sessionID,
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

func TestByType(t *testing.T) {
	t.Parallel()

	storageInst := setupTestStorage(t)

	ctx := context.Background()

	// Store events of different types
	for _, eventType := range []string{eventSessionStart, eventUserPrompt, eventUserPrompt} {
		evt := events.NewEnvelope(eventType, map[string]any{})
		if err := storageInst.Store(ctx, evt); err != nil {
			t.Fatalf("failed to store event: %v", err)
		}
	}

	// Query by type
	eventList, err := storageInst.ByType(ctx, eventUserPrompt, 10, 0)
	if err != nil {
		t.Fatalf("failed to query by type: %v", err)
	}

	if len(eventList) != 2 {
		t.Fatalf("expected 2 %s events, got %d", eventUserPrompt, len(eventList))
	}
}

func TestCount(t *testing.T) {
	t.Parallel()

	storageInst := setupTestStorage(t)

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
	evt := events.NewEnvelope(eventSessionStart, map[string]any{})
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
