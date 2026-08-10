package storage_test

import (
	"context"
	"sync"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	"github.com/WinPooh32/insight/cmd/insight/internal/testutil"
)

func TestConcurrentStore(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)
	ctx := context.Background()

	const n = 50

	var wg sync.WaitGroup
	wg.Add(n)

	var (
		mu     sync.Mutex
		errors []error
	)

	for i := range n {
		go func(i int) {
			defer wg.Done()

			evt := events.NewEnvelope(testutil.EventUserPrompt, map[string]any{
				testutil.TestSessionID: "concurrent-session",
				"index":                float64(i), // float64 for JSON compatibility
			})
			if err := storageInst.Store(ctx, evt); err != nil {
				mu.Lock()

				errors = append(errors, err)
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	if len(errors) > 0 {
		t.Errorf("concurrent stores failed: %v", errors)
	}

	count, err := storageInst.Count(ctx)
	if err != nil {
		t.Fatalf("failed to get count: %v", err)
	}

	if count != int64(n) {
		t.Errorf("expected %d events, got %d", n, count)
	}
}
