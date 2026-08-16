package storage

import "github.com/WinPooh32/insight/cmd/insight/internal/storage/db"

// QueriesForTest exposes the sqlc-generated queries to the external test package.
func (s *SQLiteStorage) QueriesForTest() *db.Queries {
	return s.q
}
