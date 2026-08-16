package research_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/research"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
	"github.com/WinPooh32/insight/cmd/insight/internal/testutil"
)

// Transcript line fixtures.
const (
	fixtureAssistant = `{"type":"assistant","isSidechain":false,"message":{"role":"assistant","content":[` +
		`{"type":"thinking","thinking":"secret-thought"},{"type":"text","text":"hello"}]}}`
	fixtureUser     = `{"type":"user","isSidechain":false,"message":{"role":"user","content":"hi"}}`
	fixtureThinking = `{"type":"assistant","isSidechain":false,"message":{"role":"assistant","content":[` +
		`{"type":"thinking","thinking":"inner-mono"}]}}`
	fixtureMixed = `{"type":"assistant","isSidechain":false,"message":{"role":"assistant","content":[` +
		`{"type":"text","text":"world"},{"type":"thinking","thinking":"again"},` +
		`{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}`
	fixtureSidechain = `{"type":"assistant","isSidechain":true,"message":{"role":"assistant","content":[` +
		`{"type":"text","text":"subagent-text"}]}}`
)

func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	return path
}

func appendTranscript(t *testing.T, path, data string) {
	t.Helper()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open transcript: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(data); err != nil {
		t.Fatalf("append transcript: %v", err)
	}
}

func delta(t *testing.T, tr *research.Transcript, sessionID string) string {
	t.Helper()

	text, err := tr.Delta(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("delta: %v", err)
	}

	return text
}

func TestTranscriptDelta(t *testing.T) {
	t.Parallel()

	path := writeTranscript(t, fixtureAssistant, fixtureUser)

	storageInst := testutil.NewTestStorage(t)
	tr := research.NewTranscript(path, storageInst.Queries())

	// First read: all assistant text, thinking excluded.
	if got := delta(t, tr, "s1"); got != "hello" {
		t.Errorf("first delta = %q, want %q", got, "hello")
	}

	// Second read, unchanged file: nothing new.
	if got := delta(t, tr, "s1"); got != "" {
		t.Errorf("second delta = %q, want empty", got)
	}

	// Append: user line, thinking-only entry, mixed entry, sidechain.
	appendTranscript(t, path, fixtureUser+"\n"+fixtureThinking+"\n"+fixtureMixed+"\n"+fixtureSidechain+"\n")

	if got := delta(t, tr, "s1"); got != "world" {
		t.Errorf("third delta = %q, want %q", got, "world")
	}
}

func TestTranscriptShrinkResetsOffset(t *testing.T) {
	t.Parallel()

	path := writeTranscript(t, fixtureAssistant, fixtureUser, fixtureThinking, fixtureMixed)

	storageInst := testutil.NewTestStorage(t)
	tr := research.NewTranscript(path, storageInst.Queries())

	if got := delta(t, tr, "s1"); got != "helloworld" {
		t.Fatalf("first delta = %q, want %q", got, "helloworld")
	}

	// Compaction: the file is now shorter than the stored offset.
	if err := os.WriteFile(path, []byte(fixtureUser+"\n"+fixtureAssistant+"\n"), 0o600); err != nil {
		t.Fatalf("rewrite transcript: %v", err)
	}

	if got := delta(t, tr, "s1"); got != "hello" {
		t.Errorf("delta after shrink = %q, want %q", got, "hello")
	}
}

func TestTranscriptPartialTrailingLine(t *testing.T) {
	t.Parallel()

	path := writeTranscript(t, fixtureAssistant)

	storageInst := testutil.NewTestStorage(t)
	tr := research.NewTranscript(path, storageInst.Queries())

	if got := delta(t, tr, "s1"); got != "hello" {
		t.Fatalf("first delta = %q, want %q", got, "hello")
	}

	// A line still being written: no trailing newline.
	appendTranscript(t, path, fixtureMixed)

	if got := delta(t, tr, "s1"); got != "" {
		t.Errorf("delta with partial line = %q, want empty", got)
	}

	// The offset must sit at the last complete line, so the partial
	// line is re-read in full once it is finished.
	row, err := storageInst.Queries().GetSessionState(context.Background(), "s1")
	if err != nil {
		t.Fatalf("get session state: %v", err)
	}

	base := len(fixtureAssistant) + 1
	if row.TranscriptOffset != int64(base) {
		t.Fatalf("offset = %d, want %d (last complete line)", row.TranscriptOffset, base)
	}

	appendTranscript(t, path, "\n")

	if got := delta(t, tr, "s1"); got != "world" {
		t.Errorf("delta after line completed = %q, want %q", got, "world")
	}
}

func TestTranscriptPreservesInjectedEntries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	path := writeTranscript(t, fixtureAssistant)

	storageInst := testutil.NewTestStorage(t)
	queries := storageInst.Queries()

	// Pre-existing session row: Delta must keep injected_entries.
	if err := queries.UpsertSessionState(ctx, db.UpsertSessionStateParams{
		SessionID:        "s1",
		TranscriptOffset: 0,
		InjectedEntries:  `["docs/a.md"]`,
		LastPrompt:       "",
	}); err != nil {
		t.Fatalf("seed session state: %v", err)
	}

	tr := research.NewTranscript(path, queries)

	if got := delta(t, tr, "s1"); got != "hello" {
		t.Fatalf("delta = %q, want %q", got, "hello")
	}

	row, err := queries.GetSessionState(ctx, "s1")
	if err != nil {
		t.Fatalf("get session state: %v", err)
	}

	if row.InjectedEntries != `["docs/a.md"]` {
		t.Errorf("injected_entries = %q, want unchanged", row.InjectedEntries)
	}

	if row.TranscriptOffset != int64(len(fixtureAssistant)+1) {
		t.Errorf("offset = %d, want %d", row.TranscriptOffset, len(fixtureAssistant)+1)
	}
}

func TestTranscriptMissingFile(t *testing.T) {
	t.Parallel()

	storageInst := testutil.NewTestStorage(t)
	tr := research.NewTranscript(filepath.Join(t.TempDir(), "absent.jsonl"), storageInst.Queries())

	if _, err := tr.Delta(context.Background(), "s1"); err == nil {
		t.Error("expected error for missing transcript file")
	}
}
