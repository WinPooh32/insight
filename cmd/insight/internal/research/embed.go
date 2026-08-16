// Package research implements research-link injection: an embedding
// client with a content-hash cache, corpus indexing, transcript
// parsing, and relevance ranking.
package research

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
)

const (
	// requestTimeout bounds one embeddings API call; the whole hook must
	// fit Claude Code's 120 s budget.
	requestTimeout = 110 * time.Second

	// errBodyLimit caps error body text included in API errors.
	errBodyLimit = 512

	// float32Size is the size of one float32 in the cache BLOB.
	float32Size = 4
)

// CacheQueries is the subset of db.Queries the embedder needs to
// persist results. *db.Queries satisfies it structurally.
type CacheQueries interface {
	GetEmbedCache(ctx context.Context, sha256 string) (db.EmbedCache, error)
	UpsertEmbedCache(ctx context.Context, arg db.UpsertEmbedCacheParams) error
}

// Embedder produces float32 vectors for text via an OpenAI-compatible
// /v1/embeddings endpoint. Results are cached in embed_cache keyed by
// sha256(text); a row is reused only when its model matches, so a
// model change re-embeds every text.
type Embedder struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
	cache   CacheQueries
}

// NewEmbedder creates an Embedder. baseURL is the API base (e.g.
// http://localhost:8080/v1); apiKey may be empty for local servers.
// cache must share the storage's database pool.
func NewEmbedder(baseURL, model, apiKey string, cache CacheQueries) *Embedder {
	return &Embedder{
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
		client: &http.Client{
			Timeout:       requestTimeout,
			Transport:     nil,
			CheckRedirect: nil,
			Jar:           nil,
		},
		cache: cache,
	}
}

// Embed returns the embedding vector for text, serving a cache hit
// when the text was already embedded with the same model. API and
// cache failures are returned as errors; there is no fallback.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	key := sha256Text(text)

	row, err := e.cache.GetEmbedCache(ctx, key)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get embed cache: %w", err)
	}

	if err == nil && row.Model == e.model {
		return decodeVector(row.Vector)
	}

	vec, err := e.callAPI(ctx, text)
	if err != nil {
		return nil, err
	}

	if err := e.cache.UpsertEmbedCache(ctx, db.UpsertEmbedCacheParams{
		Sha256: key,
		Vector: encodeVector(vec),
		Dim:    int64(len(vec)),
		Model:  e.model,
	}); err != nil {
		return nil, fmt.Errorf("upsert embed cache: %w", err)
	}

	return vec, nil
}

// callAPI posts the text to {baseURL}/embeddings and returns the
// vector. The Bearer header is set only when an API key is configured.
func (e *Embedder) callAPI(ctx context.Context, text string) ([]float32, error) {
	payload, err := json.Marshal(map[string]string{
		"model": e.model,
		"input": text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embeddings request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, errBodyLimit))
		return nil, fmt.Errorf("embeddings API: status %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}

	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, errors.New("embeddings API: empty data")
	}

	return result.Data[0].Embedding, nil
}

// sha256Text returns the hex sha256 of text, the embed_cache key.
func sha256Text(text string) string {
	sum := sha256.Sum256([]byte(text))

	return hex.EncodeToString(sum[:])
}

// encodeVector serializes a float32 vector to little-endian bytes.
func encodeVector(vec []float32) []byte {
	buf := make([]byte, float32Size*len(vec))
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[float32Size*i:], math.Float32bits(v))
	}

	return buf
}

// decodeVector reverses encodeVector.
func decodeVector(buf []byte) ([]float32, error) {
	if len(buf)%float32Size != 0 {
		return nil, fmt.Errorf("vector blob of %d bytes is not a multiple of %d", len(buf), float32Size)
	}

	vec := make([]float32, len(buf)/float32Size)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[float32Size*i:]))
	}

	return vec, nil
}
