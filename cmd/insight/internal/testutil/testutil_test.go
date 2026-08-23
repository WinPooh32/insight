package testutil_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/testutil"
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

func TestNewTestServer(t *testing.T) {
	t.Parallel()

	srv := testutil.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestPostJSON(t *testing.T) {
	t.Parallel()

	var gotBody, gotContentType string

	srv := testutil.NewTestServer(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)

		gotBody = string(b)
		gotContentType = r.Header.Get("Content-Type")
	}))

	resp := testutil.PostJSON(t, srv, "/echo", map[string]string{"k": "v"})
	defer resp.Body.Close()

	if gotBody != `{"k":"v"}` {
		t.Errorf("body = %q, want %q", gotBody, `{"k":"v"}`)
	}

	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", gotContentType, "application/json")
	}
}

func TestPostRaw(t *testing.T) {
	t.Parallel()

	srv := testutil.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)

		_, _ = w.Write(b)
	}))

	resp := testutil.PostRaw(t, srv, "/echo", []byte("raw-bytes"))
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if string(body) != "raw-bytes" {
		t.Errorf("echoed body = %q, want %q", body, "raw-bytes")
	}
}

func TestAssertStatus(t *testing.T) {
	t.Parallel()

	srv := testutil.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	resp := testutil.PostRaw(t, srv, "/ok", []byte("x"))
	defer resp.Body.Close()

	testutil.AssertStatus(t, resp, http.StatusOK)

	var sub fakeTB

	testutil.AssertStatus(&sub, resp, http.StatusInternalServerError)

	if sub.err == "" {
		t.Error("AssertStatus reported no error on mismatch")
	}
}

// fakeTB captures Errorf output without failing the outer test.
type fakeTB struct {
	testing.TB
	err string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.err = fmt.Sprintf(format, args...)
}
