package storage

import (
	"database/sql"

	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
)

// QueriesForTest exposes the sqlc-generated queries to the external test package.
func (s *SQLiteStorage) QueriesForTest() *db.Queries {
	return s.q
}

// DBForTest exposes the underlying sql.DB to the external test package.
func (s *SQLiteStorage) DBForTest() *sql.DB {
	return s.db
}
