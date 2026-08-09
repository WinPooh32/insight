package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
)

// NewSQLiteStorageMemory creates an in-memory SQLite storage (useful for tests).
func NewSQLiteStorageMemory(_ context.Context, logger *slog.Logger) (*SQLiteStorage, error) {
	sdb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("open memory sqlite: %w", err)
	}

	q := db.New(sdb)

	return &SQLiteStorage{
		q:   q,
		db:  sdb,
		log: logger,
	}, nil
}
