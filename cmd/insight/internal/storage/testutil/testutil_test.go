package testutil_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/storage"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/testutil"
)

func TestNewTestStorage(t *testing.T) {
	t.Parallel()

	s := testutil.NewTestStorage(t)

	if s == nil {
		t.Fatal("NewTestStorage returned nil")
	}

	if s.Queries() == nil {
		t.Error("Queries() returned nil")
	}
}

func TestNewTestStorageUsableAndClosedAfterCleanup(t *testing.T) {
	t.Parallel()

	var s *storage.SQLiteStorage

	t.Run("create", func(t *testing.T) {
		s = testutil.NewTestStorage(t)

		if _, err := s.Queries().GetSessionState(context.Background(), "missing"); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("GetSessionState on fresh db: err = %v, want %v", err, sql.ErrNoRows)
		}
	})

	_, err := s.Queries().GetSessionState(context.Background(), "missing")
	if !strings.Contains(err.Error(), "database is closed") {
		t.Errorf("GetSessionState after cleanup: err = %v, want closed db error", err)
	}
}

func TestNewTestStorageFatalsOnCreateError(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	// Make the parent dir a regular file so MkdirAll inside NewSQLiteStorage fails.
	if err := os.WriteFile(filepath.Join(base, "blocked"), nil, 0o600); err != nil {
		t.Fatalf("create blocked file: %v", err)
	}

	sub := fakeTB{TB: nil, dir: filepath.Join(base, "blocked", "db"), fatal: ""}

	done := make(chan struct{})
	go func() {
		defer close(done)

		testutil.NewTestStorage(&sub)
	}()

	<-done

	if sub.fatal == "" {
		t.Error("NewTestStorage did not fatal on storage creation error")
	}
}

// fakeTB records Fatalf (and exits the goroutine, like a real TB) and serves
// a TempDir whose parent is a regular file, so storage creation fails.
type fakeTB struct {
	testing.TB
	dir   string
	fatal string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) TempDir() string { return f.dir }

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.fatal = fmt.Sprintf(format, args...)

	runtime.Goexit()
}
