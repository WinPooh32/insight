package storage_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/storage"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/testutil"

	_ "modernc.org/sqlite"
)

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

// captureLog records the slog records emitted through it.
type captureLog struct {
	mu   sync.Mutex
	recs []slog.Record
}

func (c *captureLog) Enabled(context.Context, slog.Level) bool { return true }

func (c *captureLog) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.recs = append(c.recs, r)

	return nil
}

func (c *captureLog) WithAttrs([]slog.Attr) slog.Handler { return c }

func (c *captureLog) WithGroup(string) slog.Handler { return c }

func TestNewSQLiteStorageBadParentDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	blocker := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("failed to create blocker file: %v", err)
	}

	dbPath := filepath.Join(blocker, "test.db")

	logger := slog.New(slog.DiscardHandler)

	if _, err := storage.NewSQLiteStorage(context.Background(), dbPath, logger); err == nil {
		t.Fatal("expected error when db parent path is a file")
	}
}

func TestNewSQLiteStorageLogsInit(t *testing.T) {
	t.Parallel()

	h := &captureLog{mu: sync.Mutex{}, recs: nil}
	logger := slog.New(h)

	dbPath := filepath.Join(t.TempDir(), "test.db")

	storageInst, err := storage.NewSQLiteStorage(context.Background(), dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storageInst.Close()

	h.mu.Lock()
	defer h.mu.Unlock()

	for _, r := range h.recs {
		if r.Message == "sqlite storage initialized" {
			return
		}
	}

	t.Errorf("no %q log record emitted", "sqlite storage initialized")
}

// The pool is capped at one connection so every statement runs on the
// connection that enabled the per-connection foreign_keys pragma; a
// larger pool would let writes run on fresh connections where
// foreign_keys is OFF and ON DELETE CASCADE silently stops firing.
func TestStorageSingleConnectionPool(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)

	if got := storageInst.DBForTest().Stats().MaxOpenConnections; got != 1 {
		t.Errorf("MaxOpenConnections = %d, want 1", got)
	}
}

func TestQueries(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)

	if storageInst.Queries() == nil {
		t.Fatal("expected non-nil queries")
	}
}
