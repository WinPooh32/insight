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

	insighthttp "github.com/WinPooh32/insight/cmd/insight/internal/http"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage"

	// sqlite driver required for storage initialization.
	_ "modernc.org/sqlite"
)

// Common test constants.
const (
	TestSessionID     = "session_id"
	EventSessionStart = "SessionStart"
	EventUserPrompt   = "UserPromptSubmit"
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

// NewTestStorageWithLog creates a SQLiteStorage instance with a custom log buffer.
func NewTestStorageWithLog(tb testing.TB, logBuf *bytes.Buffer) *storage.SQLiteStorage {
	tb.Helper()

	tmpDir := tb.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := slog.New(slog.NewTextHandler(logBuf, nil))

	storageInst, err := storage.NewSQLiteStorage(context.Background(), dbPath, logger)
	if err != nil {
		tb.Fatalf("failed to create storage: %v", err)
	}

	tb.Cleanup(func() { storageInst.Close() })

	return storageInst
}

// NewTestRouter creates an HTTP router backed by a temp storage.
func NewTestRouter(tb testing.TB) http.Handler {
	tb.Helper()

	storageInst := NewTestStorage(tb)

	return insighthttp.Router(storageInst, slog.New(slog.DiscardHandler))
}

// NewTestRouterWithLog creates an HTTP router and captures logs.
func NewTestRouterWithLog(tb testing.TB, logBuf *bytes.Buffer) http.Handler {
	tb.Helper()

	storageInst := NewTestStorageWithLog(tb, logBuf)

	return insighthttp.Router(storageInst, slog.New(slog.NewTextHandler(logBuf, nil)))
}

// NewTestServer creates an httptest.Server from a router and registers cleanup.
func NewTestServer(tb testing.TB, router http.Handler) *httptest.Server {
	tb.Helper()

	server := httptest.NewServer(router)
	tb.Cleanup(server.Close)

	return server
}

// NewTestEnv creates a test environment and returns an httptest.Server.
func NewTestEnv(tb testing.TB) *httptest.Server {
	tb.Helper()

	router := NewTestRouter(tb)

	return NewTestServer(tb, router)
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

// PostRaw sends a POST request with raw bytes and returns the response.
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

// Get sends a GET request and returns the response.
func Get(tb testing.TB, server *httptest.Server, endpoint string) *http.Response {
	tb.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+endpoint, nil)
	if err != nil {
		tb.Fatalf("create request: %v", err)
	}

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
