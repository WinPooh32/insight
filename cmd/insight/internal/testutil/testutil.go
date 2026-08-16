package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/storage"

	// sqlite driver required for storage initialization.
	_ "modernc.org/sqlite"
)

// NewTestStorage creates a SQLiteStorage instance backed by a temp dir.
// The storage is closed when the test completes.
func NewTestStorage(tb testing.TB) *storage.SQLiteStorage {
	tb.Helper()

	tmpDir := tb.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := slog.New(slog.DiscardHandler)

	storageInst, err := storage.NewSQLiteStorage(context.Background(), dbPath, logger)
	if err != nil {
		tb.Fatalf("failed to create storage: %v", err)
	}

	tb.Cleanup(func() { storageInst.Close() })

	return storageInst
}

// NewTestServer creates an httptest.Server from a router and registers cleanup.
func NewTestServer(tb testing.TB, router http.Handler) *httptest.Server {
	tb.Helper()

	server := httptest.NewServer(router)
	tb.Cleanup(server.Close)

	return server
}

// PostJSON sends a POST request with JSON payload and returns the response.
// Caller is responsible for closing resp.Body.
func PostJSON(tb testing.TB, server *httptest.Server, endpoint string, payload any) *http.Response {
	tb.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		tb.Fatalf("marshal payload: %v", err)
	}

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, server.URL+endpoint, bytes.NewReader(body),
	)
	if err != nil {
		tb.Fatalf("create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		tb.Fatalf("execute request: %v", err)
	}

	return resp
}

// PostRaw sends a POST request with a raw body and returns the
// response. Caller is responsible for closing resp.Body.
func PostRaw(tb testing.TB, server *httptest.Server, endpoint string, body []byte) *http.Response {
	tb.Helper()

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, server.URL+endpoint, bytes.NewReader(body),
	)
	if err != nil {
		tb.Fatalf("create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		tb.Fatalf("execute request: %v", err)
	}

	return resp
}

// AssertStatus checks the response status code.
func AssertStatus(tb testing.TB, resp *http.Response, expected int) {
	tb.Helper()

	if resp.StatusCode != expected {
		tb.Errorf("expected status %d, got %d", expected, resp.StatusCode)
	}
}

// ReadBody reads and closes the response body, returning the bytes.
func ReadBody(resp *http.Response) []byte {
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	return body
}
