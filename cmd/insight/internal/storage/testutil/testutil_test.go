package testutil_test

import (
	"testing"

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
