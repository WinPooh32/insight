package storage_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
	"github.com/WinPooh32/insight/cmd/insight/internal/testutil"
)

func TestResearchEntriesRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	storageInst := testutil.NewTestStorage(t)
	q := storageInst.QueriesForTest()

	mtime := "2026-08-16T00:00:00Z"
	if err := q.UpsertResearchEntry(ctx, db.UpsertResearchEntryParams{
		Project:     "proj",
		Title:       "Doc",
		Path:        "docs/a.md",
		Description: "a doc",
		Mtime:       mtime,
	}); err != nil {
		t.Fatalf("upsert research entry: %v", err)
	}

	entries, err := q.ResearchEntriesByProject(ctx, "proj")
	if err != nil {
		t.Fatalf("list research entries: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 research entry, got %d", len(entries))
	}

	got := entries[0]
	if got.Project != "proj" || got.Title != "Doc" || got.Path != "docs/a.md" {
		t.Errorf("research entry mismatch: %+v", got)
	}

	if got.Description != "a doc" || got.Mtime != mtime {
		t.Errorf("research entry mismatch: %+v", got)
	}

	entry, err := q.GetResearchEntry(ctx, got.ID)
	if err != nil {
		t.Fatalf("get research entry: %v", err)
	}

	if entry.ID != got.ID {
		t.Errorf("expected entry id %d, got %d", got.ID, entry.ID)
	}
}

func TestResearchChunksRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	storageInst := testutil.NewTestStorage(t)
	q := storageInst.QueriesForTest()

	if err := q.UpsertResearchEntry(ctx, db.UpsertResearchEntryParams{
		Project:     "proj",
		Title:       "Doc",
		Path:        "docs/a.md",
		Description: "",
		Mtime:       "2026-08-16T00:00:00Z",
	}); err != nil {
		t.Fatalf("upsert research entry: %v", err)
	}

	entries, err := q.ResearchEntriesByProject(ctx, "proj")
	if err != nil {
		t.Fatalf("list research entries: %v", err)
	}

	vector := []byte{1, 2, 3}
	if err := q.InsertResearchChunk(ctx, db.InsertResearchChunkParams{
		Entry:   entries[0].ID,
		Heading: "Heading",
		Text:    "chunk text",
		Vector:  vector,
		Dim:     sql.NullInt64{Int64: 3, Valid: true},
		DocID:   "section:proj:docs/a.md:0",
	}); err != nil {
		t.Fatalf("insert research chunk: %v", err)
	}

	chunks, err := q.ResearchChunksByEntry(ctx, entries[0].ID)
	if err != nil {
		t.Fatalf("list research chunks: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 research chunk, got %d", len(chunks))
	}

	got := chunks[0]
	if got.Heading != "Heading" || got.Text != "chunk text" {
		t.Errorf("research chunk mismatch: %+v", got)
	}

	if !bytes.Equal(got.Vector, vector) || !got.Dim.Valid || got.Dim.Int64 != 3 {
		t.Errorf("research chunk mismatch: %+v", got)
	}

	if got.DocID != "section:proj:docs/a.md:0" {
		t.Errorf("research chunk doc_id = %q, want section:proj:docs/a.md:0", got.DocID)
	}

	byDoc, err := q.ResearchChunkByDocID(ctx, "section:proj:docs/a.md:0")
	if err != nil {
		t.Fatalf("get research chunk by doc id: %v", err)
	}

	if byDoc.ID != got.ID {
		t.Errorf("expected chunk id %d, got %d", got.ID, byDoc.ID)
	}

	if _, err := q.ResearchChunkByDocID(ctx, "entry:proj:docs/a.md"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows for unknown doc id, got %v", err)
	}

	if err := q.DeleteResearchChunksByEntry(ctx, entries[0].ID); err != nil {
		t.Fatalf("delete research chunks: %v", err)
	}

	chunks, err = q.ResearchChunksByEntry(ctx, entries[0].ID)
	if err != nil {
		t.Fatalf("list research chunks after delete: %v", err)
	}

	if len(chunks) != 0 {
		t.Errorf("expected 0 research chunks after delete, got %d", len(chunks))
	}
}

func TestEmbedCacheRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	storageInst := testutil.NewTestStorage(t)
	q := storageInst.QueriesForTest()

	vector := []byte{4, 5, 6}
	if err := q.UpsertEmbedCache(ctx, db.UpsertEmbedCacheParams{
		Sha256: "abc123",
		Vector: vector,
		Dim:    3,
		Model:  "test-model",
	}); err != nil {
		t.Fatalf("upsert embed cache: %v", err)
	}

	cached, err := q.GetEmbedCache(ctx, "abc123")
	if err != nil {
		t.Fatalf("get embed cache: %v", err)
	}

	if cached.Sha256 != "abc123" || cached.Dim != 3 || cached.Model != "test-model" {
		t.Errorf("embed cache mismatch: %+v", cached)
	}

	if !bytes.Equal(cached.Vector, vector) {
		t.Errorf("embed cache mismatch: %+v", cached)
	}
}

func TestSessionStateRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	storageInst := testutil.NewTestStorage(t)
	q := storageInst.QueriesForTest()

	if err := q.UpsertSessionState(ctx, db.UpsertSessionStateParams{
		SessionID:        "sess-1",
		TranscriptOffset: 42,
		InjectedEntries:  `["docs/a.md"]`,
		LastPrompt:       "",
	}); err != nil {
		t.Fatalf("upsert session state: %v", err)
	}

	state, err := q.GetSessionState(ctx, "sess-1")
	if err != nil {
		t.Fatalf("get session state: %v", err)
	}

	if state.SessionID != "sess-1" || state.TranscriptOffset != 42 {
		t.Errorf("session state mismatch: %+v", state)
	}

	if state.InjectedEntries != `["docs/a.md"]` {
		t.Errorf("session state mismatch: %+v", state)
	}
}

func TestResearchEntryDeleteCascades(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	storageInst := testutil.NewTestStorage(t)
	q := storageInst.QueriesForTest()

	if err := q.UpsertResearchEntry(ctx, db.UpsertResearchEntryParams{
		Project:     "proj",
		Title:       "Doc",
		Path:        "docs/a.md",
		Description: "",
		Mtime:       "2026-08-16T00:00:00Z",
	}); err != nil {
		t.Fatalf("upsert research entry: %v", err)
	}

	entries, err := q.ResearchEntriesByProject(ctx, "proj")
	if err != nil {
		t.Fatalf("list research entries: %v", err)
	}

	entryID := entries[0].ID

	if err := q.InsertResearchChunk(ctx, db.InsertResearchChunkParams{
		Entry:   entryID,
		Heading: "",
		Text:    "chunk text",
		Vector:  nil,
		Dim:     sql.NullInt64{Int64: 0, Valid: false},
		DocID:   "section:proj:docs/a.md:0",
	}); err != nil {
		t.Fatalf("insert research chunk: %v", err)
	}

	// Deleting the entry must remove its chunks (ON DELETE CASCADE).
	if err := q.DeleteResearchEntry(ctx, entryID); err != nil {
		t.Fatalf("delete research entry: %v", err)
	}

	entries, err = q.ResearchEntriesByProject(ctx, "proj")
	if err != nil {
		t.Fatalf("list research entries after delete: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 research entries after delete, got %d", len(entries))
	}

	chunks, err := q.ResearchChunksByEntry(ctx, entryID)
	if err != nil {
		t.Fatalf("list research chunks after delete: %v", err)
	}

	if len(chunks) != 0 {
		t.Errorf("expected 0 research chunks after entry delete, got %d", len(chunks))
	}
}
