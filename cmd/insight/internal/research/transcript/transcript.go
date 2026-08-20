package transcript

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
)

// SessionQueries is the subset of db.Queries the transcript reader
// needs to persist per-session state. *db.Queries satisfies it
// structurally.
type SessionQueries interface {
	GetSessionState(ctx context.Context, sessionID string) (db.SessionState, error)
	UpsertSessionState(ctx context.Context, arg db.UpsertSessionStateParams) error
}

// SessionStore adapts *db.Queries to SessionQueries so layers that must
// not import the db package (e.g. the http handler) can persist session
// state. It keeps the concrete db type out of those layers.
type SessionStore struct{ q *db.Queries }

// NewSessionStore wraps q for use as a SessionQueries.
func NewSessionStore(q *db.Queries) *SessionStore { return &SessionStore{q: q} }

func (s *SessionStore) GetSessionState(ctx context.Context, sessionID string) (db.SessionState, error) {
	state, err := s.q.GetSessionState(ctx, sessionID)
	if err != nil {
		return db.SessionState{}, fmt.Errorf("get session state: %w", err)
	}

	return state, nil
}

func (s *SessionStore) UpsertSessionState(ctx context.Context, arg db.UpsertSessionStateParams) error {
	if err := s.q.UpsertSessionState(ctx, arg); err != nil {
		return fmt.Errorf("upsert session state: %w", err)
	}

	return nil
}

// Transcript delta-parses a Claude Code JSONL transcript. Each session
// keeps a byte offset in session_state, so Delta only reads the lines
// appended since the previous call. Main transcript only; subagent
// (sidechain) entries are skipped.
type Transcript struct {
	path    string
	session SessionQueries
}

// NewTranscript creates a Transcript reading the JSONL file at path.
// session persists per-session offsets; it must share the storage's
// database pool.
func NewTranscript(path string, session SessionQueries) *Transcript {
	return &Transcript{path: path, session: session}
}

// PersistLastPrompt stores the last prompt for sessionID, preserving
// the transcript offset and injected entries. It seeds a fresh row
// when the session has none.
func PersistLastPrompt(ctx context.Context, session SessionQueries, sessionID, prompt string) error {
	state, err := session.GetSessionState(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		// Seed a valid injected set so the ranker's dedup can parse
		// the row on the same request.
		state = db.SessionState{SessionID: sessionID, TranscriptOffset: 0, InjectedEntries: "[]", LastPrompt: ""}
	} else if err != nil {
		return fmt.Errorf("load session state: %w", err)
	}

	state.LastPrompt = prompt

	if err := session.UpsertSessionState(ctx, db.UpsertSessionStateParams(state)); err != nil {
		return fmt.Errorf("save session state: %w", err)
	}

	return nil
}

// proseCapTokens is the approximate token budget kept from each end
// of an over-long text or thinking block.
// ponytail: tokens approximated as 4 chars; use a real tokenizer if
// the embedding budget ever needs exactness.
const proseCapTokens = 512

// proseCapChars is the per-side char budget behind proseCapTokens.
const proseCapChars = proseCapTokens * 4

// Delta returns the assistant prose (text and thinking) blocks
// appended to the transcript since the previous Delta call for
// sessionID and persists the new offset. Only top-level entries with
// type "assistant" and isSidechain false contribute; within them
// "text" and "thinking" content blocks are concatenated in block
// order, each capped by capProse (tool and other blocks excluded). A
// trailing partial line (the file is still being written) is left
// unconsumed. If the file shrank (compaction) the offset resets to 0.
// The returned text may be "".
func (t *Transcript) Delta(ctx context.Context, sessionID string) (string, error) {
	state, err := t.session.GetSessionState(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		state = db.SessionState{SessionID: sessionID, TranscriptOffset: 0, InjectedEntries: "[]", LastPrompt: ""}
	} else if err != nil {
		return "", fmt.Errorf("get session state: %w", err)
	}

	data, offset, err := readDelta(t.path, state.TranscriptOffset)
	if err != nil {
		return "", err
	}

	if len(data) == 0 {
		return "", nil
	}

	var b strings.Builder

	for line := range bytes.Lines(data) {
		if prose := assistantProse(line); prose != "" {
			b.WriteString(prose)
		}
	}

	if err := t.persist(ctx, state, offset); err != nil {
		return "", err
	}

	return b.String(), nil
}

// readDelta returns the complete transcript lines (raw bytes, each
// ending in '\n') appended at offset together with the new offset. If
// the file shrank below offset (compaction) the read restarts from 0.
// A trailing partial line is not consumed: it is excluded from the
// returned bytes and from the new offset.
func readDelta(path string, offset int64) ([]byte, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("stat transcript: %w", err)
	}

	if info.Size() < offset {
		offset = 0 // file shrank (compaction); re-read from the start
	}

	if info.Size() == offset {
		return nil, offset, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, 0, fmt.Errorf("seek transcript: %w", err)
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, 0, fmt.Errorf("read transcript: %w", err)
	}

	// Consume complete lines only; the tail after the last '\n' is a
	// possibly-partial line and must wait for the next read.
	if idx := bytes.LastIndexByte(data, '\n'); idx >= 0 {
		data = data[:idx+1]
	} else {
		data = nil
	}

	return data, offset + int64(len(data)), nil
}

// persist upserts the session row with the new offset.
// UpsertSessionState is a full overwrite, so the existing
// injected_entries value is re-written unchanged.
func (t *Transcript) persist(ctx context.Context, state db.SessionState, offset int64) error {
	if err := t.session.UpsertSessionState(ctx, db.UpsertSessionStateParams{
		SessionID:        state.SessionID,
		TranscriptOffset: offset,
		InjectedEntries:  state.InjectedEntries,
		LastPrompt:       state.LastPrompt,
	}); err != nil {
		return fmt.Errorf("upsert session state: %w", err)
	}

	return nil
}

// capProse returns s unchanged when it fits the head+tail budget,
// otherwise the first and last proseCapChars of s joined by an
// elision marker, each side cut back to a word boundary.
func capProse(s string) string {
	if len(s) <= 2*proseCapChars {
		return s
	}

	head := s[:proseCapChars]
	if i := strings.LastIndexByte(head, ' '); i > 0 {
		head = head[:i]
	}

	tail := s[len(s)-proseCapChars:]
	if i := strings.IndexByte(tail, ' '); i >= 0 {
		tail = tail[i+1:]
	}

	return head + "\n…\n" + tail
}

// assistantProse returns the concatenated "text" and "thinking"
// blocks of a main (non-sidechain) assistant entry line, each capped
// by capProse. It returns "" for every other line type, sidechain
// lines, unparsable lines, and entries without such blocks.
func assistantProse(line []byte) string {
	var entry struct {
		Type        string `json:"type"`
		IsSidechain bool   `json:"isSidechain"`
		Message     struct {
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Thinking string `json:"thinking"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &entry); err != nil {
		return ""
	}

	if entry.Type != "assistant" || entry.IsSidechain {
		return ""
	}

	var b strings.Builder

	for _, block := range entry.Message.Content {
		switch block.Type {
		case "text":
			b.WriteString(capProse(block.Text))
		case "thinking":
			b.WriteString(capProse(block.Thinking))
		}
	}

	return b.String()
}
