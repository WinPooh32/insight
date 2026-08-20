package rank

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"

	"github.com/WinPooh32/insight/cmd/insight/internal/research/vec"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
)

const (
	// scoreFloor is the minimum cosine similarity for an entry to be
	// offered. Local embedding models (the llama.cpp default) produce
	// flatter, less concentrated similarity distributions than
	// well-trained cloud models, so the floor sits lower (0.30) than
	// the ~0.5 typical of cloud embedders. If an operator sees too
	// many (or too few) offers with a given EMBED_MODEL, this const is
	// the knob to turn.
	scoreFloor = 0.30

	// rankSize caps the BM25 pre-filter hits.
	rankSize = 50

	// rankTop is the number of entries offered per request.
	rankTop = 3
)

// RankedEntry is one research doc offered for injection.
// Path, Title and Description are the index.md fields the
// injection endpoint emits.
type RankedEntry struct {
	Path        string
	Title       string
	Description string
	Score       float64
}

// Searcher searches a Bleve index. *index.Indexer satisfies it.
type Searcher interface {
	Search(req *bleve.SearchRequest) (*bleve.SearchResult, error)
}

// Embedding produces float32 vectors for text. *embed.Embedder
// satisfies it structurally.
type Embedding interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Queries is the subset of db.Queries the ranker needs to load
// candidate vectors, entry rows and the per-session injected set.
// *db.Queries satisfies it structurally.
type Queries interface {
	ResearchChunkByDocID(ctx context.Context, docID string) (db.ResearchChunk, error)
	ResearchEntriesByProject(ctx context.Context, project string) ([]db.ResearchEntry, error)
	GetSessionState(ctx context.Context, sessionID string) (db.SessionState, error)
	UpsertSessionState(ctx context.Context, arg db.UpsertSessionStateParams) error
}

// Ranker scores a project's research entries against query segments:
// BM25 pre-filter, cosine in Go (max across segments), score floor,
// top 3, per-session dedup.
type Ranker struct {
	search Searcher
	q      Queries
	embed  Embedding
}

// NewRanker creates a Ranker over the searcher's Bleve index,
// persisting per-session state through q and embedding segments via
// embed (a caching *embed.Embedder keeps repeats free).
func NewRanker(search Searcher, q Queries, embed Embedding) *Ranker {
	return &Ranker{search: search, q: q, embed: embed}
}

// Rank returns up to rankTop of the project's research entries most
// relevant to the query segments, best first, excluding entries
// already injected in sessionID. Each segment is embedded separately;
// a project-filtered, size-capped BM25 pre-filter selects candidate
// chunks, each scored by the max cosine across segment vectors, and
// each entry by the max across its chunks. Entries below scoreFloor
// are dropped. The returned paths are appended to the session's
// injected set with the transcript offset preserved; when nothing is
// returned the row is left untouched. Embed and storage errors are
// returned; the caller degrades silently (no keyword fallback).
func (r *Ranker) Rank(ctx context.Context, project, sessionID string, segments []string) ([]RankedEntry, error) {
	if len(segments) == 0 {
		return nil, nil
	}

	segVecs, err := r.embedSegments(ctx, segments)
	if err != nil {
		return nil, err
	}

	docIDs, err := r.prefilter(project, segments)
	if err != nil {
		return nil, err
	}

	entryScores, err := r.scoreEntries(ctx, docIDs, segVecs)
	if err != nil {
		return nil, err
	}

	ranked, err := r.topEntries(ctx, project, entryScores)
	if err != nil {
		return nil, err
	}

	ranked, err = r.dedup(ctx, sessionID, ranked)
	if err != nil {
		return nil, err
	}

	return ranked, nil
}

// embedSegments embeds each query segment. The content-hash cache in
// the embedder makes repeated texts free.
func (r *Ranker) embedSegments(ctx context.Context, segments []string) ([][]float32, error) {
	vecs := make([][]float32, len(segments))

	wg := sync.WaitGroup{}
	errs := make([]error, len(segments))

	for n, s := range segments {
		wg.Go(func() {
			vec, err := r.embed.Embed(ctx, s)
			if err != nil {
				errs[n] = err
				return
			}

			vecs[n] = vec
		})
	}

	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("embed query segments: %w", err)
	}

	return vecs, nil
}

// prefilter runs the project-filtered BM25 pre-filter over the
// segments and returns the candidate doc IDs.
func (r *Ranker) prefilter(project string, segments []string) ([]string, error) {
	// The segments form one disjunction (bleve v2 has no
	// MatchAnyQuery): a doc is a candidate when any segment matches;
	// cosine does the ranking.
	disjuncts := make([]query.Query, len(segments))
	for n, s := range segments {
		disjuncts[n] = bleve.NewMatchQuery(s)
	}

	tq := bleve.NewTermQuery(project)
	tq.SetField("project")

	req := bleve.NewSearchRequestOptions(
		bleve.NewConjunctionQuery(bleve.NewDisjunctionQuery(disjuncts...), tq), rankSize, 0, false)

	res, err := r.search.Search(req)
	if err != nil {
		return nil, fmt.Errorf("pre-filter search: %w", err)
	}

	ids := make([]string, 0, len(res.Hits))
	for _, h := range res.Hits {
		ids = append(ids, h.ID)
	}

	return ids, nil
}

// scoreEntries loads the candidate chunks' vectors and returns the
// best cosine score per entry: max across segment vectors per chunk,
// max across chunks per entry.
func (r *Ranker) scoreEntries(ctx context.Context, docIDs []string, segVecs [][]float32) (map[int64]float64, error) {
	scores := make(map[int64]float64)

	for _, id := range docIDs {
		chunk, err := r.q.ResearchChunkByDocID(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			continue // doc deleted since the index was built
		}

		if err != nil {
			return nil, fmt.Errorf("load chunk %s: %w", id, err)
		}

		vec, err := vec.Decode(chunk.Vector)
		if err != nil {
			return nil, fmt.Errorf("decode chunk %s vector: %w", id, err)
		}

		score := 0.0
		for _, sv := range segVecs {
			if c := cosine(vec, sv); c > score {
				score = c
			}
		}

		if score > scores[chunk.Entry] {
			scores[chunk.Entry] = score
		}
	}

	return scores, nil
}

// topEntries joins the scores with the entry rows, applies the score
// floor and returns the top rankTop entries, best first.
func (r *Ranker) topEntries(ctx context.Context, project string, entryScores map[int64]float64) ([]RankedEntry, error) {
	entries, err := r.q.ResearchEntriesByProject(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("list research entries: %w", err)
	}

	byID := make(map[int64]db.ResearchEntry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}

	ranked := make([]RankedEntry, 0, len(entryScores))
	for id, score := range entryScores {
		if score < scoreFloor {
			continue
		}

		e, ok := byID[id]
		if !ok {
			continue
		}

		ranked = append(ranked, RankedEntry{
			Path:        e.Path,
			Title:       e.Title,
			Description: e.Description,
			Score:       score,
		})
	}

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}

		return ranked[i].Path < ranked[j].Path
	})

	if len(ranked) > rankTop {
		ranked = ranked[:rankTop]
	}

	return ranked, nil
}

// dedup drops entries already injected in sessionID and records the
// survivors in the session's injected set, preserving the transcript
// offset. The row is only written when something is returned.
func (r *Ranker) dedup(ctx context.Context, sessionID string, ranked []RankedEntry) ([]RankedEntry, error) {
	state, err := r.q.GetSessionState(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		state = db.SessionState{SessionID: sessionID, TranscriptOffset: 0, InjectedEntries: "[]", LastPrompt: ""}
	} else if err != nil {
		return nil, fmt.Errorf("get session state: %w", err)
	}

	injected, err := parseInjected(state.InjectedEntries)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(injected))
	for _, p := range injected {
		seen[p] = true
	}

	kept := make([]RankedEntry, 0, len(ranked))
	for _, e := range ranked {
		if !seen[e.Path] {
			seen[e.Path] = true
			kept = append(kept, e)
		}
	}

	if len(kept) == 0 {
		return kept, nil // nothing new: leave the row untouched
	}

	if err := r.record(ctx, state, injected, kept); err != nil {
		return nil, err
	}

	return kept, nil
}

// record appends the returned paths to the session's injected set
// and persists the row. UpsertSessionState is a full overwrite, so
// the transcript offset read at the start of dedup is re-written
// unchanged.
func (r *Ranker) record(ctx context.Context, state db.SessionState, injected []string, kept []RankedEntry) error {
	for _, e := range kept {
		injected = append(injected, e.Path)
	}

	data, err := json.Marshal(injected)
	if err != nil {
		return fmt.Errorf("marshal injected entries: %w", err)
	}

	if err := r.q.UpsertSessionState(ctx, db.UpsertSessionStateParams{
		SessionID:        state.SessionID,
		TranscriptOffset: state.TranscriptOffset,
		InjectedEntries:  string(data),
		LastPrompt:       state.LastPrompt,
	}); err != nil {
		return fmt.Errorf("upsert session state: %w", err)
	}

	return nil
}

// parseInjected decodes the JSON array of entry paths stored in
// session_state.injected_entries.
func parseInjected(raw string) ([]string, error) {
	var injected []string
	if err := json.Unmarshal([]byte(raw), &injected); err != nil {
		return nil, fmt.Errorf("parse injected entries: %w", err)
	}

	return injected, nil
}

// cosine returns the cosine similarity of a and b. A zero-norm
// vector, or a dimension mismatch (stale vectors after an
// EMBED_MODEL change), scores 0.
func cosine(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}

	if na == 0 || nb == 0 {
		return 0
	}

	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
