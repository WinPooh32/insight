package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/migrations"
	"github.com/pressly/goose/v3"

	// Register the SQLite driver for use by database/sql.
	_ "modernc.org/sqlite"
)

const storageDirPerm = 0o755

// SQLiteStorage wraps sqlc-generated queries with SQLite.
type SQLiteStorage struct {
	q   *db.Queries
	db  *sql.DB
	log *slog.Logger
}

// NewSQLiteStorage creates a new SQLite storage at the given path.
func NewSQLiteStorage(ctx context.Context, dbPath string, logger *slog.Logger) (*SQLiteStorage, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), storageDirPerm); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	sdb, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite needs a single writer connection to avoid SQLITE_BUSY.
	sdb.SetMaxOpenConns(1)

	// Enable WAL mode for better concurrent read performance.
	if _, err := sdb.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		sdb.Close()
		return nil, fmt.Errorf("set journal mode: %w", err)
	}

	// Enforce foreign keys so ON DELETE CASCADE in migrations works.
	if _, err := sdb.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		sdb.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Run goose migrations from embedded SQL files.
	goose.SetBaseFS(migrations.Embed)

	if err := goose.SetDialect("sqlite"); err != nil {
		sdb.Close()
		return nil, fmt.Errorf("set dialect: %w", err)
	}

	if err := goose.Up(sdb, ".", goose.WithAllowMissing()); err != nil {
		sdb.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	q := db.New(sdb)

	logger.Info("sqlite storage initialized", "path", dbPath)

	return &SQLiteStorage{
		q:   q,
		db:  sdb,
		log: logger,
	}, nil
}

// Queries returns the sqlc queries backed by the storage's pool.
// Consumers that need the same database (e.g. the research embedder)
// must use this instead of opening a second pool on the same file,
// which risks SQLITE_BUSY against the single-writer connection.
func (s *SQLiteStorage) Queries() *db.Queries {
	return s.q
}

// Close closes the SQLite database connection.
func (s *SQLiteStorage) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close db: %w", err)
	}

	return nil
}
