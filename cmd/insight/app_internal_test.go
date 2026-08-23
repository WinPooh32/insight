package main

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/config"

	_ "modernc.org/sqlite"
)

func newTestConfig(t *testing.T) config.Config {
	t.Helper()

	return config.Config{
		Port:         0,
		Host:         "",
		DataDir:      t.TempDir(),
		Logger:       slog.New(slog.DiscardHandler),
		EmbedBaseURL: "",
		EmbedModel:   "",
		EmbedAPIKey:  "",
	}
}

func TestOpenDatabase(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig(t)

	ctx := context.Background()

	db, err := openDatabase(ctx, cfg)
	if err != nil {
		t.Fatalf("openDatabase: %v", err)
	}

	t.Cleanup(func() { _ = db.Close() })

	if _, err := os.Stat(cfg.DBPath()); err != nil {
		t.Errorf("DB file missing at %s: %v", cfg.DBPath(), err)
	}

	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}

	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestMigrate(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig(t)

	ctx := context.Background()

	db, err := openDatabase(ctx, cfg)
	if err != nil {
		t.Fatalf("openDatabase: %v", err)
	}

	t.Cleanup(func() { _ = db.Close() })

	if err := migrate(ctx, db, cfg.Logger); err != nil {
		t.Fatalf("first migrate: %v", err)
	}

	// goose migrations must be idempotent.
	if err := migrate(ctx, db, cfg.Logger); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table'").Scan(&count); err != nil {
		t.Fatalf("count tables: %v", err)
	}

	if count <= 0 {
		t.Error("no tables created by migrations")
	}
}
