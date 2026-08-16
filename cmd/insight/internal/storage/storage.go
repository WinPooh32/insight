package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/migrations"
	"github.com/pressly/goose/v3"

	// Register the SQLite driver for use by database/sql.
	_ "modernc.org/sqlite"
)

const storageDirPerm = 0o755

// Storage provides an interface for storing and querying hook events.
type Storage interface {
	Store(ctx context.Context, evt events.Envelope) error
	Recent(ctx context.Context, limit int) ([]events.StoredEvent, error)
	BySession(ctx context.Context, sessionID string) ([]events.StoredEvent, error)
	ByType(ctx context.Context, eventType string, limit int, offset int) ([]events.StoredEvent, error)
	Count(ctx context.Context) (int64, error)
	Close() error
}

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

// Store persists a hook event to SQLite.
func (s *SQLiteStorage) Store(ctx context.Context, evt events.Envelope) error {
	payload, err := evt.ToJSON()
	if err != nil {
		s.log.ErrorContext(ctx, "failed to marshal payload", "error", err)
		return fmt.Errorf("marshal payload: %w", err)
	}

	sessionID := sql.NullString{
		Valid:  evt.SessionID != "",
		String: evt.SessionID,
	}

	err = s.q.InsertEvent(ctx, db.InsertEventParams{
		ID:        evt.ID,
		EventType: evt.EventType,
		Received:  evt.Received.Format(time.RFC3339Nano),
		Payload:   string(payload),
		SessionID: sessionID,
	})
	if err != nil {
		s.log.ErrorContext(ctx, "failed to store event",
			"error", err,
			"event_id", evt.ID,
		)

		return fmt.Errorf("insert event: %w", err)
	}

	s.log.InfoContext(ctx, "event stored",
		"event_type", evt.EventType,
		"session_id", evt.SessionID,
	)

	return nil
}

// Recent returns the most recent events.
func (s *SQLiteStorage) Recent(ctx context.Context, limit int) ([]events.StoredEvent, error) {
	if limit < 0 {
		return nil, fmt.Errorf("limit must be non-negative, got %d", limit)
	}

	evts, err := s.q.RecentEvents(ctx, db.RecentEventsParams{
		Limit:  int64(limit),
		Offset: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("recent events: %w", err)
	}

	return toStoredEvents(evts), nil
}

// BySession returns all events for a given session.
func (s *SQLiteStorage) BySession(ctx context.Context, sessionID string) ([]events.StoredEvent, error) {
	evts, err := s.q.EventsBySession(ctx, sql.NullString{
		Valid:  true,
		String: sessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("events by session: %w", err)
	}

	return toStoredEvents(evts), nil
}

// ByType returns events filtered by type with pagination.
func (s *SQLiteStorage) ByType(
	ctx context.Context, eventType string, limit int, offset int,
) ([]events.StoredEvent, error) {
	if limit < 0 || offset < 0 {
		return nil, fmt.Errorf("limit and offset must be non-negative, got limit=%d offset=%d", limit, offset)
	}

	evts, err := s.q.EventsByType(ctx, db.EventsByTypeParams{
		EventType: eventType,
		Limit:     int64(limit),
		Offset:    int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("events by type: %w", err)
	}

	return toStoredEvents(evts), nil
}

// Count returns the total number of stored events.
func (s *SQLiteStorage) Count(ctx context.Context) (int64, error) {
	count, err := s.q.EventCount(ctx)
	if err != nil {
		return 0, fmt.Errorf("event count: %w", err)
	}

	return count, nil
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

// toStoredEvents converts sqlc-generated db.Event to domain types.
func toStoredEvents(evts []db.Event) []events.StoredEvent {
	if len(evts) == 0 {
		return nil
	}

	result := make([]events.StoredEvent, len(evts))
	for i := range evts {
		var sessionID *string
		if evts[i].SessionID.Valid {
			sessionID = &evts[i].SessionID.String
		}

		result[i] = events.StoredEvent{
			ID:        evts[i].ID,
			EventType: evts[i].EventType,
			Received:  evts[i].Received,
			Payload:   evts[i].Payload,
			SessionID: sessionID,
		}
	}

	return result
}
