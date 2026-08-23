package testutil

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/storage"
)

// NewTestStorage creates a SQLiteStorage instance backed by a temp dir.
// The storage is closed when the test completes.
func NewTestStorage(tb testing.TB) *storage.SQLiteStorage {
	tb.Helper()

	tmpDir := tb.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := slog.New(slog.DiscardHandler)

	storageInst, err := storage.NewSQLiteStorage(context.Background(), dbPath, logger)
	if err != nil {
		tb.Fatalf("failed to create storage: %v", err)
	}

	tb.Cleanup(func() { storageInst.Close() })

	return storageInst
}
