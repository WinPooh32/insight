package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

	"github.com/WinPooh32/insight/cmd/insight/internal/config"
	insighthttp "github.com/WinPooh32/insight/cmd/insight/internal/http"
	"github.com/WinPooh32/insight/cmd/insight/internal/research"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage"
	migrations "github.com/WinPooh32/insight/cmd/insight/internal/storage/migrations"
)

const (
	dirPerm         = 0o755
	readTimeout     = 115 * time.Second
	writeTimeout    = 115 * time.Second
	idleTimeout     = 160 * time.Second
	shutdownTimeout = 5 * time.Second
	maxHeaderBytes  = 4096
)

// Run wires dependencies and starts the hooks relay service.
func Run(ctx context.Context) error {
	cfg := config.DefaultConfig()
	logger := cfg.Logger

	sdb, err := openDatabase(ctx, cfg)
	if err != nil {
		return err
	}
	defer sdb.Close()

	if err := migrate(ctx, sdb, logger); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	srv, stor, indexer, err := createServer(ctx, cfg)
	if err != nil {
		return err
	}
	defer stor.Close()

	go func() {
		<-ctx.Done()
		logger.Info("shutting down server...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown error", "error", err)
		}

		if err := indexer.Close(); err != nil {
			logger.Error("close research index", "error", err)
		}
	}()

	logger.Info("starting hooks relay service",
		"addr", cfg.Addr(),
		"data_dir", cfg.DataDir,
	)

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: %w", err)
	}

	return nil
}

func openDatabase(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	if err := os.MkdirAll(cfg.DataDir, dirPerm); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	sdb, err := sql.Open("sqlite", cfg.DBPath())
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := sdb.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		sdb.Close()
		return nil, fmt.Errorf("set journal mode: %w", err)
	}

	return sdb, nil
}

func createServer(
	ctx context.Context, cfg config.Config,
) (*http.Server, *storage.SQLiteStorage, *research.Indexer, error) {
	stor, err := storage.NewSQLiteStorage(ctx, cfg.DBPath(), cfg.Logger)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create storage: %w", err)
	}

	queries := stor.Queries()
	embedder := research.NewEmbedder(cfg.EmbedBaseURL, cfg.EmbedModel, cfg.EmbedAPIKey, queries)

	bleveDir := filepath.Join(cfg.DataDir, "research")
	if err := os.MkdirAll(bleveDir, dirPerm); err != nil {
		stor.Close()
		return nil, nil, nil, fmt.Errorf("create research index dir: %w", err)
	}

	indexer, err := research.NewIndexer(bleveDir, queries, embedder)
	if err != nil {
		stor.Close()
		return nil, nil, nil, fmt.Errorf("open research index: %w", err)
	}

	ranker := research.NewRanker(indexer, queries, embedder)
	injection := insighthttp.NewInjectionHandler(indexer, ranker, queries, cfg.Logger)

	server := &http.Server{
		Addr:                         cfg.Addr(),
		Handler:                      insighthttp.Router(injection),
		ReadHeaderTimeout:            readTimeout,
		ReadTimeout:                  readTimeout,
		WriteTimeout:                 writeTimeout,
		IdleTimeout:                  idleTimeout,
		MaxHeaderBytes:               maxHeaderBytes,
		DisableGeneralOptionsHandler: false,
		TLSConfig:                    nil,
		TLSNextProto:                 nil,
		ConnState:                    nil,
		ConnContext:                  nil,
		ErrorLog:                     nil,
		BaseContext:                  nil,
		HTTP2:                        nil,
		Protocols:                    nil,
	}

	return server, stor, indexer, nil
}

func migrate(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	provider, err := goose.NewProvider(
		database.DialectSQLite3,
		db,
		migrations.Embed,
		goose.WithVerbose(true),
	)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	for _, r := range results {
		logger.Info("migrated", "file", r.Source.Path, "duration", r.Duration)
	}

	return nil
}
