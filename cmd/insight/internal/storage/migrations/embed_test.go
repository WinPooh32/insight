package migrations_test

import (
	"io/fs"
	"testing"

	migrations "github.com/WinPooh32/insight/cmd/insight/internal/storage/migrations"
)

func TestEmbedContainsSQLFiles(t *testing.T) {
	t.Parallel()

	matches, err := fs.Glob(migrations.Embed, "*.sql")
	if err != nil {
		t.Fatalf("fs.Glob: %v", err)
	}

	if len(matches) == 0 {
		t.Error("no .sql files matched in embedded FS")
	}
}
