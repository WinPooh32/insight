package httphelper_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
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

	var gotBody, gotContentType, gotPath string

	srv := httphelper.NewTestServer(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)

		gotBody = string(b)
		gotContentType = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
	}))

	resp := httphelper.PostJSON(t, srv, "/echo", map[string]string{"k": "v"})
	defer resp.Body.Close()

	if gotBody != `{"k":"v"}` {
		t.Errorf("body = %q, want %q", gotBody, `{"k":"v"}`)
	}

	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", gotContentType, "application/json")
	}

	if gotPath != "/echo" {
		t.Errorf("path = %q, want %q", gotPath, "/echo")
	}
}

func TestPostRaw(t *testing.T) {
	t.Parallel()

	var gotContentType, gotPath string

	srv := httphelper.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)

		gotContentType = r.Header.Get("Content-Type")
		gotPath = r.URL.Path

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

	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", gotContentType, "application/json")
	}

	if gotPath != "/echo" {
		t.Errorf("path = %q, want %q", gotPath, "/echo")
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

func TestPostJSONFatals(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

	t.Run("marshal", func(t *testing.T) {
		t.Parallel()

		srv := httphelper.NewTestServer(t, handler)

		if fatal := callFatal(func(tb testing.TB) {
			tb.Helper()

			closePostJSON(tb, srv, "/echo", make(chan int))
		}); fatal == "" {
			t.Error("PostJSON did not fatal on unmarshalable payload")
		}
	})

	t.Run("create request", func(t *testing.T) {
		t.Parallel()

		srv := httphelper.NewTestServer(t, handler)

		if fatal := callFatal(func(tb testing.TB) {
			tb.Helper()

			closePostJSON(tb, srv, "\n", map[string]string{})
		}); fatal == "" {
			t.Error("PostJSON did not fatal on invalid endpoint")
		}
	})

	t.Run("execute request", func(t *testing.T) {
		t.Parallel()

		srv := httphelper.NewTestServer(t, handler)

		srv.Close()

		if fatal := callFatal(func(tb testing.TB) {
			tb.Helper()

			closePostJSON(tb, srv, "/echo", map[string]string{})
		}); fatal == "" {
			t.Error("PostJSON did not fatal on closed server")
		}
	})
}

func closePostJSON(tb testing.TB, srv *httptest.Server, endpoint string, payload any) {
	tb.Helper()

	resp := httphelper.PostJSON(tb, srv, endpoint, payload)
	if resp != nil {
		_ = resp.Body.Close()
	}
}

func TestPostRawFatals(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

	t.Run("create request", func(t *testing.T) {
		t.Parallel()

		srv := httphelper.NewTestServer(t, handler)

		if fatal := callFatal(func(tb testing.TB) {
			tb.Helper()

			closePostRaw(tb, srv, "\n", []byte("x"))
		}); fatal == "" {
			t.Error("PostRaw did not fatal on invalid endpoint")
		}
	})

	t.Run("execute request", func(t *testing.T) {
		t.Parallel()

		srv := httphelper.NewTestServer(t, handler)

		srv.Close()

		if fatal := callFatal(func(tb testing.TB) {
			tb.Helper()

			closePostRaw(tb, srv, "/echo", []byte("x"))
		}); fatal == "" {
			t.Error("PostRaw did not fatal on closed server")
		}
	})
}

func closePostRaw(tb testing.TB, srv *httptest.Server, endpoint string, body []byte) {
	tb.Helper()

	resp := httphelper.PostRaw(tb, srv, endpoint, body)
	if resp != nil {
		_ = resp.Body.Close()
	}
}

// callFatal runs fn with a fakeTB in a goroutine (Fatalf exits it, like a
// real TB) and returns the recorded fatal message.
func callFatal(fn func(tb testing.TB)) string {
	var sub fakeTB

	done := make(chan struct{})
	go func() {
		defer close(done)

		fn(&sub)
	}()

	<-done

	return sub.fatal
}

func TestNewTestServerClosesOnCleanup(t *testing.T) {
	t.Parallel()

	var srv *httptest.Server

	t.Run("create", func(t *testing.T) {
		srv = httphelper.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()

		t.Error("request after cleanup succeeded, want server closed")
	}
}

func TestReadBodyClosesBody(t *testing.T) {
	t.Parallel()

	srv := httphelper.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("abc"))
	}))

	resp := httphelper.PostRaw(t, srv, "/echo", []byte("x"))
	defer resp.Body.Close()

	if got := httphelper.ReadBody(resp); string(got) != "abc" {
		t.Errorf("ReadBody = %q, want %q", got, "abc")
	}

	if _, err := io.ReadFull(resp.Body, make([]byte, 1)); err == nil || errors.Is(err, io.EOF) {
		t.Errorf("read after ReadBody: err = %v, want closed body error", err)
	}
}

// fakeTB captures Errorf and Fatalf output without failing the outer test.
type fakeTB struct {
	testing.TB
	err   string
	fatal string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.err = fmt.Sprintf(format, args...)
}

func (f *fakeTB) Fatal(args ...any) {
	f.fatal = fmt.Sprint(args...)
}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.fatal = fmt.Sprintf(format, args...)

	runtime.Goexit()
}
