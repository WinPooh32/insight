package rank_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/research/index"
	"github.com/WinPooh32/insight/cmd/insight/internal/research/rank"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
	"github.com/WinPooh32/insight/cmd/insight/internal/testutil"
)

// queryWord is the word the BM25 pre-filter looks for; every test
// corpus contains it where the fake embedder makes it score.
const queryWord = "zebra"

// mapEmbedder returns the vector mapped to the exact embedded text;
// unknown texts get def. It programs the cosine outcomes of Rank and
// doubles as the indexer's embedder.
type mapEmbedder struct {
	vectors map[string][]float32
	def     []float32
}

func (m *mapEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if v, ok := m.vectors[text]; ok {
		return v, nil
	}

	return m.def, nil
}

// writeCorpus creates a temp cwd with the index.md content ("" = no
// file) and the given docs, and returns the cwd.
func writeCorpus(t *testing.T, index string, docs map[string]string) string {
	t.Helper()

	cwd := t.TempDir()

	if index != "" {
		dir := filepath.Join(cwd, ".claude", "skills", "research")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir research dir: %v", err)
		}

		if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(index), 0o600); err != nil {
			t.Fatalf("write index.md: %v", err)
		}
	}

	for path, content := range docs {
		full := filepath.Join(cwd, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", path, err)
		}

		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	return cwd
}

// newRankFixture indexes the corpus into a real Bleve index and a
// temp DB, and returns a Ranker over it plus the queries for
// assertions.
func newRankFixture(t *testing.T, indexMd string, docs map[string]string,
	emb *mapEmbedder,
) (*rank.Ranker, *db.Queries) {
	t.Helper()

	ctx := context.Background()

	cwd := writeCorpus(t, indexMd, docs)

	queries := testutil.NewTestStorage(t).Queries()

	idx, err := index.NewIndexer(t.TempDir(), queries, emb)
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}

	t.Cleanup(func() { idx.Close() })

	if err := idx.Index(ctx, "proj", cwd); err != nil {
		t.Fatalf("index: %v", err)
	}

	return rank.NewRanker(idx, queries, emb), queries
}

// rankPaths returns the returned entry paths in order.
func rankPaths(t *testing.T, r *rank.Ranker, session string, segments ...string) []string {
	t.Helper()

	got, err := r.Rank(context.Background(), "proj", session, segments)
	if err != nil {
		t.Fatalf("rank: %v", err)
	}

	paths := make([]string, len(got))
	for i, e := range got {
		paths[i] = e.Path
	}

	return paths
}

const (
	raPath = ".claude/skills/research/researches/ra.md"
	rbPath = ".claude/skills/research/researches/rb.md"
	rcPath = ".claude/skills/research/researches/rc.md"
	rdPath = ".claude/skills/research/researches/rd.md"
	rePath = ".claude/skills/research/researches/re.md"
)

func entryIndex(paths ...string) string {
	var index string

	var indexSb90 strings.Builder
	for i, p := range paths {
		fmt.Fprintf(&indexSb90, "- [Doc %d](%s) — desc %d\n", i, p, i)
	}

	index += indexSb90.String()

	return index
}

func TestRankSectionMatchOnce(t *testing.T) {
	t.Parallel()

	// One entry, two sections whose texts both sit close to the
	// query vector: the entry is scored once, by the best section.
	emb := &mapEmbedder{
		def: []float32{0, 1},
		vectors: map[string][]float32{
			queryWord:                {1, 0},
			"One Zebra section one.": {1, 0},
			"Two Zebra section two.": {1, 0.1},
		},
	}

	r, _ := newRankFixture(t, entryIndex(raPath), map[string]string{
		raPath: "# One\n\nZebra section one.\n\n## Two\n\nZebra section two.\n",
	}, emb)

	got, err := r.Rank(context.Background(), "proj", "s", []string{queryWord})
	if err != nil {
		t.Fatalf("rank: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("rank = %d entries, want 1 (one per doc, not per section): %+v", len(got), got)
	}

	if got[0].Path != raPath || got[0].Title != "Doc 0" {
		t.Errorf("entry = %+v, want %s / Doc 0", got[0], raPath)
	}

	if math.Abs(got[0].Score-1.0) > 1e-6 {
		t.Errorf("score = %v, want 1 (best section vector equals the query vector)", got[0].Score)
	}
}

func TestRankEntryLevelMatch(t *testing.T) {
	t.Parallel()

	// The query word only occurs in the index.md bullet (title +
	// description), i.e. the entry's own chunk. No section matches.
	emb := &mapEmbedder{
		def: []float32{0, 1},
		vectors: map[string][]float32{
			queryWord:                          {1, 0},
			"Zebra habitat — Zebra range data": {1, 0},
		},
	}

	index := "- [Zebra habitat](" + raPath + ") — Zebra range data\n"

	r, _ := newRankFixture(t, index, map[string]string{
		raPath: "# Topic\n\nIrrelevant body.\n",
	}, emb)

	got, err := r.Rank(context.Background(), "proj", "s", []string{queryWord})
	if err != nil {
		t.Fatalf("rank: %v", err)
	}

	if len(got) != 1 || got[0].Path != raPath {
		t.Fatalf("rank = %+v, want the entry via its own chunk", got)
	}

	if math.Abs(got[0].Score-1.0) > 1e-6 {
		t.Errorf("score = %v, want 1", got[0].Score)
	}
}

func TestRankTop3(t *testing.T) {
	t.Parallel()

	// Five entries with distinct scores, all above the floor:
	// exactly 3 come back, in score order.
	x := []float32{0, 0.5, 1, 1.5, 2}
	paths := []string{raPath, rbPath, rcPath, rdPath, rePath}

	emb := &mapEmbedder{def: []float32{0, 1}, vectors: map[string][]float32{
		queryWord: {1, 0},
	}}

	docs := make(map[string]string, len(paths))

	for i, p := range paths {
		emb.vectors[fmt.Sprintf("zebra body %d", i)] = []float32{1, x[i]}
		docs[p] = fmt.Sprintf("zebra body %d\n", i)
	}

	r, _ := newRankFixture(t, entryIndex(paths...), docs, emb)

	got, err := r.Rank(context.Background(), "proj", "s", []string{queryWord})
	if err != nil {
		t.Fatalf("rank: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("rank = %d entries, want 3: %+v", len(got), got)
	}

	want := []string{raPath, rbPath, rcPath}
	for i, p := range want {
		if got[i].Path != p {
			t.Errorf("rank[%d] = %s, want %s", i, got[i].Path, p)
		}
	}

	for i := 1; i < len(got); i++ {
		if got[i-1].Score <= got[i].Score {
			t.Errorf("scores not descending: %v after %v", got[i].Score, got[i-1].Score)
		}
	}
}

func TestRankFloor(t *testing.T) {
	t.Parallel()

	// Every candidate vector is orthogonal to the query (cosine 0,
	// below the floor): empty result, and the session row is never
	// written.
	emb := &mapEmbedder{
		def:     []float32{0, 1},
		vectors: map[string][]float32{queryWord: {1, 0}},
	}

	r, queries := newRankFixture(t, entryIndex(raPath), map[string]string{
		raPath: "# Topic\n\nZebra body.\n",
	}, emb)

	got, err := r.Rank(context.Background(), "proj", "s", []string{queryWord})
	if err != nil {
		t.Fatalf("rank: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("rank = %+v, want empty (all candidates below the floor)", got)
	}

	if _, err := queries.GetSessionState(context.Background(), "s"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("session row = %v, want none (nothing returned, no write)", err)
	}
}

func TestRankDedup(t *testing.T) {
	t.Parallel()

	// Both entries pass the floor; the first Rank returns both and
	// records them. The same query in the same session must not
	// offer either again; a different session gets both.
	emb := &mapEmbedder{
		def: []float32{0, 1},
		vectors: map[string][]float32{
			queryWord: {1, 0},
			"zebra A": {1, 0},
			"zebra B": {1, 0.5},
		},
	}

	r, _ := newRankFixture(t, entryIndex(raPath, rbPath), map[string]string{
		raPath: "zebra A\n",
		rbPath: "zebra B\n",
	}, emb)

	first := rankPaths(t, r, "s", queryWord)
	if len(first) != 2 || first[0] != raPath || first[1] != rbPath {
		t.Fatalf("first rank = %v, want [%s %s]", first, raPath, rbPath)
	}

	second := rankPaths(t, r, "s", queryWord)
	if len(second) != 0 {
		t.Fatalf("second rank in the same session = %v, want empty (both already injected)", second)
	}

	other := rankPaths(t, r, "s2", queryWord)
	if len(other) != 2 || other[0] != raPath || other[1] != rbPath {
		t.Fatalf("rank in a different session = %v, want [%s %s]", other, raPath, rbPath)
	}
}

func TestRankPreservesTranscriptOffset(t *testing.T) {
	t.Parallel()

	emb := &mapEmbedder{
		def:     []float32{0, 1},
		vectors: map[string][]float32{queryWord: {1, 0}, "zebra A": {1, 0}},
	}

	r, queries := newRankFixture(t, entryIndex(raPath), map[string]string{
		raPath: "zebra A\n",
	}, emb)

	ctx := context.Background()

	// The transcript reader owns the same row: seed a non-zero
	// offset.
	if err := queries.UpsertSessionState(ctx, db.UpsertSessionStateParams{
		SessionID:        "s",
		TranscriptOffset: 42,
		InjectedEntries:  "[]",
		LastPrompt:       "",
	}); err != nil {
		t.Fatalf("seed session state: %v", err)
	}

	got := rankPaths(t, r, "s", queryWord)
	if len(got) != 1 || got[0] != raPath {
		t.Fatalf("rank = %v, want [%s]", got, raPath)
	}

	state, err := queries.GetSessionState(ctx, "s")
	if err != nil {
		t.Fatalf("get session state: %v", err)
	}

	if state.TranscriptOffset != 42 {
		t.Errorf("transcript offset = %d, want 42 (preserved)", state.TranscriptOffset)
	}

	if state.InjectedEntries != `["`+raPath+`"]` {
		t.Errorf("injected entries = %s, want [\"%s\"]", state.InjectedEntries, raPath)
	}
}

func TestRankNoSessionRow(t *testing.T) {
	t.Parallel()

	emb := &mapEmbedder{
		def:     []float32{0, 1},
		vectors: map[string][]float32{queryWord: {1, 0}, "zebra A": {1, 0}},
	}

	r, queries := newRankFixture(t, entryIndex(raPath), map[string]string{
		raPath: "zebra A\n",
	}, emb)

	got := rankPaths(t, r, "fresh", queryWord)
	if len(got) != 1 || got[0] != raPath {
		t.Fatalf("rank = %v, want [%s]", got, raPath)
	}

	state, err := queries.GetSessionState(context.Background(), "fresh")
	if err != nil {
		t.Fatalf("get session state: %v", err)
	}

	if state.TranscriptOffset != 0 {
		t.Errorf("transcript offset = %d, want 0", state.TranscriptOffset)
	}

	if state.InjectedEntries != `["`+raPath+`"]` {
		t.Errorf("injected entries = %s, want [\"%s\"]", state.InjectedEntries, raPath)
	}
}
