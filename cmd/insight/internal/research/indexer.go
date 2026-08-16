package research

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"

	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
)

// IndexQueries is the subset of db.Queries the corpus indexer needs
// to persist entry and chunk rows. *db.Queries satisfies it
// structurally.
type IndexQueries interface {
	UpsertResearchEntry(ctx context.Context, arg db.UpsertResearchEntryParams) error
	ResearchEntriesByProject(ctx context.Context, project string) ([]db.ResearchEntry, error)
	DeleteResearchEntry(ctx context.Context, id int64) error
	InsertResearchChunk(ctx context.Context, arg db.InsertResearchChunkParams) error
	DeleteResearchChunksByEntry(ctx context.Context, entry int64) error
	ResearchChunksByEntry(ctx context.Context, entry int64) ([]db.ResearchChunk, error)
}

// Embedding produces float32 vectors for text. *Embedder satisfies
// it structurally.
type Embedding interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Indexer maintains the Bleve text index and the
// research_entries/research_chunks rows for one corpus root. The
// Bleve index stays open across Index calls.
type Indexer struct {
	idx   bleve.Index
	q     IndexQueries
	embed Embedding
}

// NewIndexer opens (or creates) the Bleve index at bleveDir and
// returns an Indexer persisting through q with vectors from embed.
func NewIndexer(bleveDir string, q IndexQueries, embed Embedding) (*Indexer, error) {
	idx, err := bleve.Open(bleveDir)
	if errors.Is(err, bleve.ErrorIndexPathDoesNotExist) ||
		errors.Is(err, bleve.ErrorIndexMetaMissing) {
		idx, err = bleve.New(bleveDir, newIndexMapping())
	}

	if err != nil {
		return nil, fmt.Errorf("open bleve index: %w", err)
	}

	return &Indexer{idx: idx, q: q, embed: embed}, nil
}

// Close closes the Bleve index.
func (i *Indexer) Close() error {
	if err := i.idx.Close(); err != nil {
		return fmt.Errorf("close bleve index: %w", err)
	}

	return nil
}

// Search runs a Bleve search over the index.
func (i *Indexer) Search(req *bleve.SearchRequest) (*bleve.SearchResult, error) {
	res, err := i.idx.Search(req)
	if err != nil {
		return nil, fmt.Errorf("search bleve index: %w", err)
	}

	return res, nil
}

// Index lazily reindexes the corpus at
// <cwd>/.claude/skills/research for project. An entry whose mtime,
// title and description are unchanged is skipped entirely (no embed
// calls). Entries stored but absent from index.md are removed,
// cascading their chunks. A missing index.md means no corpus and
// returns nil.
func (i *Indexer) Index(ctx context.Context, project, cwd string) error {
	data, err := os.ReadFile(filepath.Join(cwd, ".claude", "skills", "research", "index.md"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("read index.md: %w", err)
	}

	parsed := parseIndex(data)

	live := make(map[string]bool, len(parsed))
	for _, e := range parsed {
		live[e.path] = true
	}

	stored, err := i.q.ResearchEntriesByProject(ctx, project)
	if err != nil {
		return fmt.Errorf("list research entries: %w", err)
	}

	if err := i.removeDeleted(ctx, project, live, stored); err != nil {
		return err
	}

	storedByPath := make(map[string]db.ResearchEntry, len(stored))
	for _, e := range stored {
		if live[e.Path] {
			storedByPath[e.Path] = e
		}
	}

	for _, e := range parsed {
		if err := i.syncEntry(ctx, project, cwd, e, storedByPath[e.path]); err != nil {
			return err
		}
	}

	return nil
}

// removeDeleted drops stored entries whose bullet is gone from
// index.md, cascading their chunks and Bleve docs.
func (i *Indexer) removeDeleted(ctx context.Context, project string,
	live map[string]bool, stored []db.ResearchEntry,
) error {
	for _, e := range stored {
		if live[e.Path] {
			continue
		}

		old, err := i.q.ResearchChunksByEntry(ctx, e.ID)
		if err != nil {
			return fmt.Errorf("list research chunks: %w", err)
		}

		if err := i.q.DeleteResearchEntry(ctx, e.ID); err != nil {
			return fmt.Errorf("delete research entry: %w", err)
		}

		if err := i.removeEntryDocs(project, e.Path, len(old)); err != nil {
			return err
		}
	}

	return nil
}

// syncEntry reindexes one entry unless its mtime, title and
// description are unchanged (a zero stored entry means the entry is
// new).
func (i *Indexer) syncEntry(ctx context.Context, project, cwd string, e indexEntry, stored db.ResearchEntry) error {
	path := filepath.Join(cwd, e.path)
	info, err := os.Stat(path)

	mtime := ""
	if err == nil {
		mtime = info.ModTime().UTC().Format(time.RFC3339)
	}

	if stored.Mtime == mtime && stored.Title == e.title && stored.Description == e.description {
		return nil // unchanged: no embed calls
	}

	return i.reindexEntry(ctx, project, path, e, mtime, stored.ID)
}

// reindexEntry replaces an entry's chunks and Bleve docs. The entry
// and all its sections are embedded before anything is persisted, so
// an embed failure leaves the previous rows and docs in place (stale
// but usable) and the next Index call retries the entry; the caller
// degrades silently per the architecture failure table.
func (i *Indexer) reindexEntry(ctx context.Context, project, path string,
	e indexEntry, mtime string, existingID int64,
) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read research doc: %w", err)
		}
	}

	sections := chunkSections(data)

	entryVec, err := i.embed.Embed(ctx, entryChunkText(e))
	if err != nil {
		return fmt.Errorf("embed entry: %w", err)
	}

	vecs, err := i.embedSections(ctx, sections)
	if err != nil {
		return err
	}

	if err := i.q.UpsertResearchEntry(ctx, db.UpsertResearchEntryParams{
		Project:     project,
		Title:       e.title,
		Path:        e.path,
		Description: e.description,
		Mtime:       mtime,
	}); err != nil {
		return fmt.Errorf("upsert research entry: %w", err)
	}

	id, err := i.entryID(ctx, project, e.path, existingID)
	if err != nil {
		return err
	}

	return i.writeSections(ctx, project, e, id, sections, vecs, entryVec)
}

// embedSections embeds every section. The error is returned before
// any row or doc is persisted (see reindexEntry).
func (i *Indexer) embedSections(ctx context.Context, sections []section) ([][]float32, error) {
	vecs := make([][]float32, len(sections))

	wg := sync.WaitGroup{}
	errs := make([]error, len(sections))

	for n, s := range sections {
		wg.Go(func() {
			text := strings.TrimSpace(s.heading + " " + s.body)

			vec, err := i.embed.Embed(ctx, text)
			if err != nil {
				errs[n] = err
			}

			vecs[n] = vec
		})
	}

	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("embed sections: %w", err)
	}

	return vecs, nil
}

// entryID resolves the research_entries id of an upserted entry.
// existingID is 0 for a new entry.
func (i *Indexer) entryID(ctx context.Context, project, path string, existingID int64) (int64, error) {
	if existingID != 0 {
		return existingID, nil
	}

	// UpsertResearchEntry is :exec, so a new entry's id is looked
	// up by path.
	entries, err := i.q.ResearchEntriesByProject(ctx, project)
	if err != nil {
		return 0, fmt.Errorf("list research entries: %w", err)
	}

	for _, en := range entries {
		if en.Path == path {
			return en.ID, nil
		}
	}

	return 0, fmt.Errorf("research entry %q not found after upsert", path)
}

// writeSections replaces an entry's chunk rows and Bleve docs. The
// entry's own chunk (title + description, doc ID of the Bleve entry
// doc) is stored beside the section chunks so the ranker can score
// entry-level matches.
func (i *Indexer) writeSections(ctx context.Context, project string,
	e indexEntry, id int64, sections []section, vecs [][]float32, entryVec []float32,
) error {
	old, err := i.q.ResearchChunksByEntry(ctx, id)
	if err != nil {
		return fmt.Errorf("list research chunks: %w", err)
	}

	if err := i.q.DeleteResearchChunksByEntry(ctx, id); err != nil {
		return fmt.Errorf("delete research chunks: %w", err)
	}

	for n := range old {
		if err := i.idx.Delete(sectionDocID(project, e.path, n)); err != nil {
			return fmt.Errorf("delete section doc: %w", err)
		}
	}

	for n, s := range sections {
		if err := i.q.InsertResearchChunk(ctx, db.InsertResearchChunkParams{
			Entry:   id,
			Heading: s.heading,
			Text:    s.body,
			Vector:  encodeVector(vecs[n]),
			Dim:     sql.NullInt64{Int64: int64(len(vecs[n])), Valid: true},
			DocID:   sectionDocID(project, e.path, n),
		}); err != nil {
			return fmt.Errorf("insert research chunk: %w", err)
		}

		if err := i.idx.Index(sectionDocID(project, e.path, n), map[string]any{
			"project": project,
			"heading": s.heading,
			"body":    s.body,
		}); err != nil {
			return fmt.Errorf("index section doc: %w", err)
		}
	}

	if err := i.q.InsertResearchChunk(ctx, db.InsertResearchChunkParams{
		Entry:   id,
		Heading: "",
		Text:    entryChunkText(e),
		Vector:  encodeVector(entryVec),
		Dim:     sql.NullInt64{Int64: int64(len(entryVec)), Valid: true},
		DocID:   entryDocID(project, e.path),
	}); err != nil {
		return fmt.Errorf("insert research chunk: %w", err)
	}

	if err := i.idx.Index(entryDocID(project, e.path), map[string]any{
		"project":     project,
		"title":       e.title,
		"description": e.description,
	}); err != nil {
		return fmt.Errorf("index entry doc: %w", err)
	}

	return nil
}

// removeEntryDocs drops the entry and its section docs from the
// Bleve index.
func (i *Indexer) removeEntryDocs(project, path string, sections int) error {
	if err := i.idx.Delete(entryDocID(project, path)); err != nil {
		return fmt.Errorf("delete entry doc: %w", err)
	}

	for n := range sections {
		if err := i.idx.Delete(sectionDocID(project, path, n)); err != nil {
			return fmt.Errorf("delete section doc: %w", err)
		}
	}

	return nil
}

// newIndexMapping is the minimal Bleve mapping: project is a keyword
// (term filtering), the rest are text fields.
func newIndexMapping() mapping.IndexMapping {
	m := mapping.NewIndexMapping()

	// AddFieldMappingsAt is required (not DefaultMapping.Fields):
	// v2.6 only applies explicit mappings found under Properties,
	// everything else falls back to the dynamic standard analyzer.
	m.DefaultMapping.AddFieldMappingsAt("project", mapping.NewKeywordFieldMapping())

	for _, name := range []string{"title", "description", "heading", "body"} {
		m.DefaultMapping.AddFieldMappingsAt(name, mapping.NewTextFieldMapping())
	}

	return m
}

// entryChunkText is the embedded text of an entry's own chunk: the
// index.md bullet as title and description.
func entryChunkText(e indexEntry) string {
	if e.description == "" {
		return e.title
	}

	return e.title + " — " + e.description
}

func entryDocID(project, path string) string {
	return "entry:" + project + ":" + path
}

func sectionDocID(project, path string, ordinal int) string {
	return fmt.Sprintf("section:%s:%s:%d", project, path, ordinal)
}
