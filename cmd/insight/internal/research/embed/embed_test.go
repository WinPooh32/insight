package embed_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/research/embed"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/testutil"
)

// newFakeEmbedServer returns an OpenAI-compatible fake that answers
// /v1/embeddings with a model-dependent vector, counting requests and
// recording the Authorization header of each call.
func newFakeEmbedServer(t *testing.T, hits *atomic.Int64, auths *[]string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)

		*auths = append(*auths, r.Header.Get("Authorization"))

		if r.Method != http.MethodPost {
			t.Errorf("request method = %s, want POST", r.Method)
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}

		var req struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		vec := []float32{1, 2, 3}
		if req.Model == "b" {
			vec = []float32{4, 5, 6}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": vec}},
		})
	}))
	t.Cleanup(server.Close)

	return server
}

func sha256Text(t *testing.T, text string) string {
	t.Helper()

	sum := sha256.Sum256([]byte(text))

	return hex.EncodeToString(sum[:])
}

func TestEmbedCacheHit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var (
		hits  atomic.Int64
		auths []string
	)

	server := newFakeEmbedServer(t, &hits, &auths)

	storageInst := testutil.NewTestStorage(t)
	embedder := embed.NewEmbedder(server.URL, "a", "", storageInst.Queries())

	first, err := embedder.Embed(ctx, "hello")
	if err != nil {
		t.Fatalf("first embed: %v", err)
	}

	second, err := embedder.Embed(ctx, "hello")
	if err != nil {
		t.Fatalf("second embed: %v", err)
	}

	if got := hits.Load(); got != 1 {
		t.Errorf("server hits = %d, want 1 (second embed must be a cache hit)", got)
	}

	if !equalFloat32(first, second) {
		t.Errorf("cache hit vector %v != original %v", second, first)
	}

	// Empty API key: no Authorization header sent.
	for _, a := range auths {
		if a != "" {
			t.Errorf("Authorization header = %q, want empty", a)
		}
	}
}

func TestEmbedDistinctTexts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var (
		hits  atomic.Int64
		auths []string
	)

	server := newFakeEmbedServer(t, &hits, &auths)

	storageInst := testutil.NewTestStorage(t)
	embedder := embed.NewEmbedder(server.URL, "a", "key-1", storageInst.Queries())

	for _, text := range []string{"alpha", "beta"} {
		if _, err := embedder.Embed(ctx, text); err != nil {
			t.Fatalf("embed %q: %v", text, err)
		}
	}

	if got := hits.Load(); got != 2 {
		t.Errorf("server hits = %d, want 2 (each distinct text embedded once)", got)
	}

	// Non-empty API key: Bearer header on every call.
	for _, a := range auths {
		if a != "Bearer key-1" {
			t.Errorf("Authorization header = %q, want %q", a, "Bearer key-1")
		}
	}
}

func TestEmbedModelChange(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var (
		hits  atomic.Int64
		auths []string
	)

	server := newFakeEmbedServer(t, &hits, &auths)

	storageInst := testutil.NewTestStorage(t)

	a := embed.NewEmbedder(server.URL, "a", "", storageInst.Queries())

	va, err := a.Embed(ctx, "hello")
	if err != nil {
		t.Fatalf("embed with model a: %v", err)
	}

	// A different model must not reuse the cached row.
	b := embed.NewEmbedder(server.URL, "b", "", storageInst.Queries())

	vb, err := b.Embed(ctx, "hello")
	if err != nil {
		t.Fatalf("embed with model b: %v", err)
	}

	if got := hits.Load(); got != 2 {
		t.Errorf("server hits = %d, want 2 (model change must re-embed)", got)
	}

	if !equalFloat32(va, []float32{1, 2, 3}) {
		t.Errorf("model a vector = %v, want [1 2 3]", va)
	}

	if !equalFloat32(vb, []float32{4, 5, 6}) {
		t.Errorf("model b vector = %v, want [4 5 6]", vb)
	}

	// The upsert overwrote the old row.
	row, err := storageInst.Queries().GetEmbedCache(ctx, sha256Text(t, "hello"))
	if err != nil {
		t.Fatalf("get embed cache: %v", err)
	}

	if row.Model != "b" {
		t.Errorf("cached model = %q, want b", row.Model)
	}
}

func TestEmbedServerDown(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	storageInst := testutil.NewTestStorage(t)
	embedder := embed.NewEmbedder("http://127.0.0.1:1", "a", "", storageInst.Queries())

	vec, err := embedder.Embed(ctx, "hello")
	if err == nil {
		t.Fatalf("expected error with server down, got vector %v", vec)
	}
}

func TestEmbedAPIError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	storageInst := testutil.NewTestStorage(t)

	for name, body := range map[string]string{
		"status 500": `{"error": "boom"}`,
		"empty data": `{"data": []}`,
		"bad json":   `{not json`,
		"empty vec":  `{"data": [{"embedding": []}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if name == "status 500" {
					http.Error(w, body, http.StatusInternalServerError)

					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(server.Close)

			embedder := embed.NewEmbedder(server.URL, "a", "", storageInst.Queries())
			if _, err := embedder.Embed(ctx, "hello"); err == nil {
				t.Errorf("expected error for %s", name)
			}
		})
	}
}

func TestEmbedCacheCorrupt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	storageInst := testutil.NewTestStorage(t)

	// A BLOB whose length is not a multiple of 4 cannot decode.
	if err := storageInst.Queries().UpsertEmbedCache(ctx, db.UpsertEmbedCacheParams{
		Sha256: sha256Text(t, "corrupt"),
		Vector: []byte{1, 2, 3},
		Dim:    1,
		Model:  "a",
	}); err != nil {
		t.Fatalf("upsert corrupt row: %v", err)
	}

	embedder := embed.NewEmbedder("http://127.0.0.1:1", "a", "", storageInst.Queries())
	if _, err := embedder.Embed(ctx, "corrupt"); err == nil {
		t.Error("expected error for corrupt cache row")
	}
}

func TestVectorRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	storageInst := testutil.NewTestStorage(t)

	var (
		hits  atomic.Int64
		auths []string
	)

	server := newFakeEmbedServer(t, &hits, &auths)
	embedder := embed.NewEmbedder(server.URL, "a", "", storageInst.Queries())

	// The fake returns [1 2 3] for model "a".
	vec, err := embedder.Embed(ctx, "hello")
	if err != nil {
		t.Fatalf("embed: %v", err)
	}

	// The BLOB must be little-endian float32: 1.0, 2.0 and 3.0.
	want := []byte{
		0x00, 0x00, 0x80, 0x3F, // 1.0
		0x00, 0x00, 0x00, 0x40, // 2.0
		0x00, 0x00, 0x40, 0x40, // 3.0
	}

	row, err := storageInst.Queries().GetEmbedCache(ctx, sha256Text(t, "hello"))
	if err != nil {
		t.Fatalf("get embed cache: %v", err)
	}

	if row.Dim != 3 {
		t.Errorf("cached dim = %d, want 3", row.Dim)
	}

	if !bytes.Equal(row.Vector, want) {
		t.Errorf("cached vector = %x, want %x", row.Vector, want)
	}

	// A fresh embedder must decode the same BLOB bit-identically
	// (cache hit, no new request).
	again := embed.NewEmbedder(server.URL, "a", "", storageInst.Queries())

	got, err := again.Embed(ctx, "hello")
	if err != nil {
		t.Fatalf("re-embed: %v", err)
	}

	if hits := hits.Load(); hits != 1 {
		t.Errorf("server hits = %d, want 1", hits)
	}

	if !equalFloat32(got, vec) {
		t.Errorf("decoded vector = %v, want %v", got, vec)
	}
}

func equalFloat32(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if math.Float32bits(a[i]) != math.Float32bits(b[i]) {
			return false
		}
	}

	return true
}
