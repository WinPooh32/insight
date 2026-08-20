package httphandler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/WinPooh32/insight/cmd/insight/internal/research"
)

// injectionDeadline bounds the whole injection phase. The hook must
// fit Claude Code's 30 s budget, so this sits below it.
const injectionDeadline = 110 * time.Second

// contextHeader introduces the injected research links in the
// additionalContext message.
const contextHeader = "These researches may be relevant to the task:\n"

// maxPayloadBytes bounds the request body the injection endpoints read.
const maxPayloadBytes = 1 << 20 // 1 MB

// Hook event types served by the injection endpoints.
const (
	userPromptSubmit = "UserPromptSubmit"
	preToolUse       = "PreToolUse"
)

// InjectionHandler serves the UserPromptSubmit and PreToolUse hooks.
// It offers the top relevant research entries as additionalContext.
// Every failure mode degrades to a 200 with an empty body.
type InjectionHandler struct {
	indexer *research.Indexer
	ranker  *research.Ranker
	session research.SessionQueries
	log     *slog.Logger
}

// NewInjectionHandler creates an InjectionHandler. session must share
// the storage's database pool (e.g. SQLiteStorage.Queries()).
func NewInjectionHandler(
	indexer *research.Indexer, ranker *research.Ranker, session research.SessionQueries, logger *slog.Logger,
) *InjectionHandler {
	return &InjectionHandler{
		indexer: indexer,
		ranker:  ranker,
		session: session,
		log:     logger,
	}
}

// upsPayload is the UserPromptSubmit hook payload.
type upsPayload struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	Prompt         string `json:"prompt"`
}

// ptuPayload is the PreToolUse hook payload.
type ptuPayload struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	Cwd            string          `json:"cwd"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
}

// UserPromptSubmit handles the UserPromptSubmit injection hook.
func (h *InjectionHandler) UserPromptSubmit(w http.ResponseWriter, r *http.Request) {
	var p upsPayload
	if !h.decode(r, &p) {
		return
	}

	if p.SessionID == "" || p.Cwd == "" {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), injectionDeadline)
	defer cancel()

	if err := h.indexer.Index(ctx, p.Cwd, p.Cwd); err != nil {
		h.log.WarnContext(ctx, "index research corpus", "error", err)
	}

	var segments []string
	if p.Prompt != "" {
		segments = append(segments, p.Prompt)
	}

	h.persistLastPrompt(ctx, p.SessionID, p.Prompt)

	h.respond(ctx, w, userPromptSubmit, p.Cwd, p.SessionID, segments)
}

// PreToolUse handles the PreToolUse injection hook.
func (h *InjectionHandler) PreToolUse(w http.ResponseWriter, r *http.Request) {
	var p ptuPayload
	if !h.decode(r, &p) {
		return
	}

	if p.SessionID == "" || p.Cwd == "" {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), injectionDeadline)
	defer cancel()

	if err := h.indexer.Index(ctx, p.Cwd, p.Cwd); err != nil {
		h.log.WarnContext(ctx, "index research corpus", "error", err)
	}

	segments := h.ptuSegments(ctx, p)

	h.respond(ctx, w, preToolUse, p.Cwd, p.SessionID, segments)
}

// decode reads the bounded request body into p. It returns false when
// the body is missing, oversized, or not valid JSON; the caller
// returns a 200 empty body in that case.
func (h *InjectionHandler) decode(r *http.Request, p any) bool {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxPayloadBytes+1))
	if err != nil || len(raw) > maxPayloadBytes {
		return false
	}

	if err := json.Unmarshal(raw, p); err != nil {
		return false
	}

	return true
}

// ptuSegments builds the PreToolUse query segments: the persisted
// last prompt, the new transcript delta, and the compacted tool
// input, each included when non-empty.
func (h *InjectionHandler) ptuSegments(ctx context.Context, p ptuPayload) []string {
	var segments []string

	if last := h.lastPrompt(ctx, p.SessionID); last != "" {
		segments = append(segments, last)
	}

	delta, err := research.NewTranscript(p.TranscriptPath, h.session).Delta(ctx, p.SessionID)
	if err != nil {
		h.log.WarnContext(ctx, "read transcript delta", "error", err)
	} else if delta != "" {
		segments = append(segments, delta)
	}

	if len(p.ToolInput) > 0 {
		var buf bytes.Buffer
		if err := json.Compact(&buf, p.ToolInput); err == nil && buf.Len() > 0 {
			segments = append(segments, buf.String())
		}
	}

	return segments
}

// lastPrompt returns the persisted last prompt for sessionID, or ""
// when the session has no row.
func (h *InjectionHandler) lastPrompt(ctx context.Context, sessionID string) string {
	state, err := h.session.GetSessionState(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}

	if err != nil {
		h.log.WarnContext(ctx, "get session state", "error", err)
		return ""
	}

	return state.LastPrompt
}

// persistLastPrompt stores the last prompt for sessionID, preserving
// the transcript offset and injected entries.
func (h *InjectionHandler) persistLastPrompt(ctx context.Context, sessionID, prompt string) {
	if err := research.PersistLastPrompt(ctx, h.session, sessionID, prompt); err != nil {
		h.log.WarnContext(ctx, "persist last prompt", "error", err)
	}
}

// respond ranks the segments and writes the additionalContext
// response when entries are offered. Every failure degrades to a 200
// empty body.
func (h *InjectionHandler) respond(
	ctx context.Context, w http.ResponseWriter, event, cwd, sessionID string, segments []string,
) {
	if len(segments) == 0 {
		return
	}

	ranked, err := h.ranker.Rank(ctx, cwd, sessionID, segments)
	if err != nil {
		h.log.WarnContext(ctx, "rank research entries", "error", err)
		return
	}

	if len(ranked) == 0 {
		return
	}

	lines := make([]string, 0, len(ranked))
	for _, e := range ranked {
		lines = append(lines, "- ["+e.Title+"]("+e.Path+") — "+e.Description)
	}

	content := contextHeader + strings.Join(lines, "\n")

	h.log.InfoContext(ctx, "inject additional context",
		"event_type", event, "session_id", sessionID,
		"content", content)

	resp := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     event,
			"additionalContext": content,
		},
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.WarnContext(ctx, "encode injection response", "error", err)
	}
}
