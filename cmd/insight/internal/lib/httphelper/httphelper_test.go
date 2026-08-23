package httphelper_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/lib/httphelper"
)

func TestNewTestServer(t *testing.T) {
	t.Parallel()

	srv := httphelper.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

	srv := httphelper.NewTestServer(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)

		gotBody = string(b)
		gotContentType = r.Header.Get("Content-Type")
	}))

	resp := httphelper.PostJSON(t, srv, "/echo", map[string]string{"k": "v"})
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

	srv := httphelper.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)

		_, _ = w.Write(b)
	}))

	resp := httphelper.PostRaw(t, srv, "/echo", []byte("raw-bytes"))
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

	srv := httphelper.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	resp := httphelper.PostRaw(t, srv, "/ok", []byte("x"))
	defer resp.Body.Close()

	httphelper.AssertStatus(t, resp, http.StatusOK)

	var sub fakeTB

	httphelper.AssertStatus(&sub, resp, http.StatusInternalServerError)

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
