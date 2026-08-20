package index_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"

	"github.com/WinPooh32/insight/cmd/insight/internal/research/index"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
	"github.com/WinPooh32/insight/cmd/insight/internal/testutil"
)

// fakeEmbedder counts Embed calls and returns a deterministic
// 2-dim vector.
type fakeEmbedder struct {
	calls int
}

func (f *fakeEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	f.calls++

	return []float32{1, 0}, nil
}

// failingEmbedder simulates the embedding server being down.
type failingEmbedder struct{}

func (failingEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("embed server down")
}

// corpusIndex is a two-entry index.md with wrapped descriptions.
const corpusIndex = `# Research Index

- [codebase overview](.claude/skills/research/researches/codebase-overview.md) —
  Architecture, packages, and data flow
- [work items](.claude/skills/research/researches/work-items.md) —
  Open implementation items
`

const (
	overviewPath = ".claude/skills/research/researches/codebase-overview.md"
	itemsPath    = ".claude/skills/research/researches/work-items.md"

	overviewDoc = "# Codebase overview\n\nBody one covers architecture.\n\n## Layout\n\nPackages and layout.\n"
	itemsDoc    = "Preamble before any heading.\n\n## Items\n\nItem details.\n"
)

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

func newTestIndexer(t *testing.T, embed index.Embedding) (*index.Indexer, *db.Queries) {
	t.Helper()

	queries := testutil.NewTestStorage(t).Queries()

	idx, err := index.NewIndexer(t.TempDir(), queries, embed)
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}

	t.Cleanup(func() { idx.Close() })

	return idx, queries
}

// searchDocIDs returns the IDs of the docs in project matching the
// query word.
func searchDocIDs(t *testing.T, idx *index.Indexer, project, query string) []string {
	t.Helper()

	tq := bleve.NewTermQuery(project)
	tq.SetField("project")

	req := bleve.NewSearchRequestOptions(
		bleve.NewConjunctionQuery(bleve.NewMatchQuery(query), tq), 50, 0, false)

	res, err := idx.Search(req)
	if err != nil {
		t.Fatalf("bleve search: %v", err)
	}

	ids := make([]string, 0, len(res.Hits))
	for _, h := range res.Hits {
		ids = append(ids, h.ID)
	}

	return ids
}

func contains(ids []string, want string) bool {
	return slices.Contains(ids, want)
}

func TestIndexFresh(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cwd := writeCorpus(t, corpusIndex, map[string]string{
		overviewPath: overviewDoc,
		itemsPath:    itemsDoc,
	})

	embedder := &fakeEmbedder{calls: 0}
	idx, queries := newTestIndexer(t, embedder)

	if err := idx.Index(ctx, cwd, cwd); err != nil {
		t.Fatalf("index: %v", err)
	}

	// Entry rows for every index.md bullet.
	entries, err := queries.ResearchEntriesByProject(ctx, cwd)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2: %+v", len(entries), entries)
	}

	byPath := make(map[string]db.ResearchEntry, len(entries))
	for _, e := range entries {
		byPath[e.Path] = e
	}

	overview, ok := byPath[overviewPath]
	if !ok {
		t.Fatalf("missing entry row for %s", overviewPath)
	}

	if overview.Title != "codebase overview" || overview.Description != "Architecture, packages, and data flow" {
		t.Errorf("overview entry = %+v, want title/description from bullet", overview)
	}

	if overview.Mtime == "" {
		t.Errorf("overview entry mtime empty, want RFC3339")
	}

	if _, ok := byPath[itemsPath]; !ok {
		t.Fatalf("missing entry row for %s", itemsPath)
	}

	// Chunk rows: one per section plus the entry's own chunk,
	// little-endian vector, dim set.
	chunks, err := queries.ResearchChunksByEntry(ctx, overview.ID)
	if err != nil {
		t.Fatalf("list chunks: %v", err)
	}

	if len(chunks) != 3 {
		t.Fatalf("overview chunks = %d, want 3: %+v", len(chunks), chunks)
	}

	if chunks[0].Heading != "Codebase overview" || chunks[0].Text != "Body one covers architecture." {
		t.Errorf("chunk 0 = %+v", chunks[0])
	}

	if chunks[1].Heading != "Layout" || chunks[1].Text != "Packages and layout." {
		t.Errorf("chunk 1 = %+v", chunks[1])
	}

	// The entry chunk is the index.md bullet: title + description.
	if chunks[2].Heading != "" || chunks[2].Text != "codebase overview — Architecture, packages, and data flow" {
		t.Errorf("entry chunk = %+v", chunks[2])
	}

	wantVec := []byte{0, 0, 0x80, 0x3F, 0, 0, 0, 0} // float32 {1, 0} LE
	for _, c := range chunks {
		if !bytes.Equal(c.Vector, wantVec) || !c.Dim.Valid || c.Dim.Int64 != 2 {
			t.Errorf("chunk %d vector/dim = %x/%v, want %x/2", c.ID, c.Vector, c.Dim, wantVec)
		}
	}

	// Each chunk's doc_id is the exact Bleve doc ID of its text doc.
	if chunks[0].DocID != "section:"+cwd+":"+overviewPath+":0" {
		t.Errorf("chunk 0 doc_id = %q", chunks[0].DocID)
	}

	if chunks[2].DocID != "entry:"+cwd+":"+overviewPath {
		t.Errorf("entry chunk doc_id = %q", chunks[2].DocID)
	}

	// Bleve: the entry doc and a section doc are found, filtered by
	// project.
	ids := searchDocIDs(t, idx, cwd, "architecture")
	if !contains(ids, "entry:"+cwd+":"+overviewPath) {
		t.Errorf("search ids %v missing entry doc", ids)
	}

	if !contains(ids, "section:"+cwd+":"+overviewPath+":0") {
		t.Errorf("search ids %v missing section doc", ids)
	}

	// A section with an empty heading is indexed too.
	ids = searchDocIDs(t, idx, cwd, "preamble")
	if !contains(ids, "section:"+cwd+":"+itemsPath+":0") {
		t.Errorf("search ids %v missing preamble section doc", ids)
	}

	// Both entries (entry chunk + two sections each) embedded.
	if embedder.calls != 6 {
		t.Errorf("embed calls = %d, want 6", embedder.calls)
	}
}

func TestIndexUnchanged(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cwd := writeCorpus(t, corpusIndex, map[string]string{
		overviewPath: overviewDoc,
		itemsPath:    itemsDoc,
	})

	embedder := &fakeEmbedder{calls: 0}
	idx, _ := newTestIndexer(t, embedder)

	if err := idx.Index(ctx, cwd, cwd); err != nil {
		t.Fatalf("first index: %v", err)
	}

	first := embedder.calls

	if err := idx.Index(ctx, cwd, cwd); err != nil {
		t.Fatalf("second index: %v", err)
	}

	if embedder.calls != first {
		t.Errorf("embed calls = %d after unchanged index, want %d (no re-embed)", embedder.calls, first)
	}
}

func TestIndexEditDoc(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cwd := writeCorpus(t, corpusIndex, map[string]string{
		overviewPath: overviewDoc,
		itemsPath:    itemsDoc,
	})

	embedder := &fakeEmbedder{calls: 0}
	idx, queries := newTestIndexer(t, embedder)

	if err := idx.Index(ctx, cwd, cwd); err != nil {
		t.Fatalf("first index: %v", err)
	}

	// Edit the overview doc (third section) and bump its mtime.
	path := filepath.Join(cwd, overviewPath)

	newDoc := overviewDoc + "\n## Data flow\n\nEvents flow through the relay.\n"
	if err := os.WriteFile(path, []byte(newDoc), 0o600); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if err := idx.Index(ctx, cwd, cwd); err != nil {
		t.Fatalf("second index: %v", err)
	}

	// Only the edited entry re-embeds: its entry chunk plus the 3
	// sections.
	if embedder.calls != 6+4 {
		t.Errorf("embed calls = %d, want 10 (entry chunk + 3 sections of the edited doc)", embedder.calls)
	}

	entries, err := queries.ResearchEntriesByProject(ctx, cwd)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}

	byPath := make(map[string]db.ResearchEntry, len(entries))
	for _, e := range entries {
		byPath[e.Path] = e
	}

	chunks, err := queries.ResearchChunksByEntry(ctx, byPath[overviewPath].ID)
	if err != nil {
		t.Fatalf("list chunks: %v", err)
	}

	if len(chunks) != 4 {
		t.Fatalf("overview chunks = %d, want 4: %+v", len(chunks), chunks)
	}

	if chunks[2].Heading != "Data flow" || chunks[2].Text != "Events flow through the relay." {
		t.Errorf("chunk 2 = %+v", chunks[2])
	}

	// The other entry is untouched.
	items, err := queries.ResearchChunksByEntry(ctx, byPath[itemsPath].ID)
	if err != nil {
		t.Fatalf("list items chunks: %v", err)
	}

	if len(items) != 3 || items[0].Text != "Preamble before any heading." || items[1].Text != "Item details." {
		t.Errorf("items chunks changed: %+v", items)
	}
}

func TestIndexUnlistedFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	unlisted := ".claude/skills/research/researches/unlisted.md"
	cwd := writeCorpus(t, corpusIndex, map[string]string{
		overviewPath: overviewDoc,
		itemsPath:    itemsDoc,
		unlisted:     "# Unlisted\n\nUnlisted zebra content.\n",
	})

	embedder := &fakeEmbedder{calls: 0}
	idx, queries := newTestIndexer(t, embedder)

	if err := idx.Index(ctx, cwd, cwd); err != nil {
		t.Fatalf("index: %v", err)
	}

	entries, err := queries.ResearchEntriesByProject(ctx, cwd)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}

	for _, e := range entries {
		if e.Path == unlisted {
			t.Fatalf("unlisted file %s has an entry row: %+v", unlisted, e)
		}
	}

	// Its text is absent from the Bleve index.
	if ids := searchDocIDs(t, idx, cwd, "zebra"); len(ids) != 0 {
		t.Errorf("search ids %v, want none (unlisted file not indexed)", ids)
	}
}

func TestIndexRemoveBullet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cwd := writeCorpus(t, corpusIndex, map[string]string{
		overviewPath: overviewDoc,
		itemsPath:    itemsDoc,
	})

	embedder := &fakeEmbedder{calls: 0}
	idx, queries := newTestIndexer(t, embedder)

	if err := idx.Index(ctx, cwd, cwd); err != nil {
		t.Fatalf("first index: %v", err)
	}

	entries, err := queries.ResearchEntriesByProject(ctx, cwd)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}

	var itemsID int64

	for _, e := range entries {
		if e.Path == itemsPath {
			itemsID = e.ID
		}
	}

	if itemsID == 0 {
		t.Fatalf("items entry not found")
	}

	// Drop the work-items bullet.
	oneEntry := `# Research Index

- [codebase overview](.claude/skills/research/researches/codebase-overview.md) —
  Architecture, packages, and data flow
`

	indexPath := filepath.Join(cwd, ".claude", "skills", "research", "index.md")
	if err := os.WriteFile(indexPath, []byte(oneEntry), 0o600); err != nil {
		t.Fatalf("write index.md: %v", err)
	}

	if err := idx.Index(ctx, cwd, cwd); err != nil {
		t.Fatalf("second index: %v", err)
	}

	entries, err = queries.ResearchEntriesByProject(ctx, cwd)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}

	if len(entries) != 1 || entries[0].Path != overviewPath {
		t.Fatalf("entries after removal = %+v, want only the overview", entries)
	}

	chunks, err := queries.ResearchChunksByEntry(ctx, itemsID)
	if err != nil {
		t.Fatalf("list chunks: %v", err)
	}

	if len(chunks) != 0 {
		t.Errorf("chunks after removal = %d, want 0 (cascade)", len(chunks))
	}

	// The removed entry's docs are gone from Bleve; the remaining
	// entry's docs stay.
	if ids := searchDocIDs(t, idx, cwd, "preamble"); len(ids) != 0 {
		t.Errorf("search ids %v, want none (removed entry's docs)", ids)
	}

	ids := searchDocIDs(t, idx, cwd, "architecture")
	if !contains(ids, "entry:"+cwd+":"+overviewPath) {
		t.Errorf("search ids %v missing surviving entry doc", ids)
	}
}

func TestIndexMissingIndexMd(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cwd := t.TempDir()

	embedder := &fakeEmbedder{calls: 0}
	idx, _ := newTestIndexer(t, embedder)

	if err := idx.Index(ctx, cwd, cwd); err != nil {
		t.Fatalf("index without index.md: %v", err)
	}

	if embedder.calls != 0 {
		t.Errorf("embed calls = %d, want 0", embedder.calls)
	}
}

func TestIndexEmbedFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cwd := writeCorpus(t, corpusIndex, map[string]string{
		overviewPath: overviewDoc,
		itemsPath:    itemsDoc,
	})

	storageInst := testutil.NewTestStorage(t)

	failing, err := index.NewIndexer(t.TempDir(), storageInst.Queries(), failingEmbedder{})
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}

	t.Cleanup(func() { failing.Close() })

	if err := failing.Index(ctx, cwd, cwd); err == nil {
		t.Fatal("expected error with failing embedder")
	}

	// Nothing was persisted: the next Index retries from scratch.
	entries, err := storageInst.Queries().ResearchEntriesByProject(ctx, cwd)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("entries after failed index = %+v, want none", entries)
	}

	working := &fakeEmbedder{calls: 0}

	idx, err := index.NewIndexer(t.TempDir(), storageInst.Queries(), working)
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}

	t.Cleanup(func() { idx.Close() })

	if err := idx.Index(ctx, cwd, cwd); err != nil {
		t.Fatalf("index with working embedder: %v", err)
	}

	if working.calls != 6 {
		t.Errorf("embed calls = %d, want 6", working.calls)
	}
}
