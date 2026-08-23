package httphandler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	insighthttp "github.com/WinPooh32/insight/cmd/insight/internal/http"
	"github.com/WinPooh32/insight/cmd/insight/internal/lib/httphelper"
	"github.com/WinPooh32/insight/cmd/insight/internal/research/index"
	"github.com/WinPooh32/insight/cmd/insight/internal/research/rank"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/db"
	"github.com/WinPooh32/insight/cmd/insight/internal/storage/testutil"
)

const (
	raPath = ".claude/skills/research/researches/ra.md"
	rbPath = ".claude/skills/research/researches/rb.md"
)

// mapEmbedder returns the vector mapped to the exact embedded text;
// unknown texts get def. It programs the cosine outcomes of Rank.
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

// failingEmbedder errors on every call to exercise the embedder-down
// degradation path.
type failingEmbedder struct{}

func (failingEmbedder) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("embedder down")
}

// writeCorpus creates a temp cwd with the index.md content and the
// given docs, and returns the cwd.
func writeCorpus(t *testing.T, index string, docs map[string]string) string {
	t.Helper()

	cwd := t.TempDir()

	dir := filepath.Join(cwd, ".claude", "skills", "research")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir research dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(index), 0o600); err != nil {
		t.Fatalf("write index.md: %v", err)
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

// injectionFixture wires a real Bleve index, a real temp DB, and a
// temp corpus behind an InjectionHandler served by a test HTTP server.
type injectionFixture struct {
	server  *httptest.Server
	cwd     string
	queries *db.Queries
}

func newInjectionFixture(t *testing.T, indexMd string, docs map[string]string, emb index.Embedding) *injectionFixture {
	t.Helper()

	storage := testutil.NewTestStorage(t)
	queries := storage.Queries()
	cwd := writeCorpus(t, indexMd, docs)

	indexer, err := index.NewIndexer(t.TempDir(), queries, emb)
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}

	t.Cleanup(func() { indexer.Close() })

	logger := slog.New(slog.DiscardHandler)
	ranker := rank.NewRanker(indexer, queries, emb)
	injection := insighthttp.NewInjectionHandler(indexer, ranker, queries, logger)
	router := insighthttp.Router(injection)

	return &injectionFixture{
		server: httphelper.NewTestServer(t, router), cwd: cwd, queries: queries,
	}
}

// sizedUpsBody returns a valid UserPromptSubmit body of exactly size
// bytes for session, padding the pad field.
func sizedUpsBody(session, cwd string, size int) []byte {
	base := fmt.Sprintf(`{"session_id":%q,"cwd":%q,"prompt":"zebra","pad":""}`, session, cwd)
	pad := size - len(base)

	return []byte(base[:len(base)-2] + strings.Repeat("a", pad) + `"}`)
}

// upsPayload is the UserPromptSubmit request body.
func upsPayload(cwd, session, prompt string) map[string]any {
	return map[string]any{
		"session_id": session,
		"cwd":        cwd,
		"prompt":     prompt,
	}
}

// additionalContext decodes the hookSpecificOutput.additionalContext
// field of an injection response.
func additionalContext(t *testing.T, body []byte) (event, ctx string) {
	t.Helper()

	var got struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode response: %v (body %q)", err, body)
	}

	return got.HookSpecificOutput.HookEventName, got.HookSpecificOutput.AdditionalContext
}

func TestInjectionUpsRelevant(t *testing.T) {
	t.Parallel()

	emb := &mapEmbedder{
		def: []float32{0, 1},
		vectors: map[string][]float32{
			"zebra":            {1, 0},
			"Alpha zebra body": {1, 0},
			"Beta zebra body":  {1, 0.1},
		},
	}

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n- [Beta Doc]("+rbPath+") — beta desc\n",
		map[string]string{
			raPath: "# Alpha\n\nzebra body\n",
			rbPath: "# Beta\n\nzebra body\n",
		},
		emb)

	resp := httphelper.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), upsPayload(fx.cwd, "s1", "zebra"))
	defer resp.Body.Close()

	httphelper.AssertStatus(t, resp, http.StatusOK)

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	event, ctx := additionalContext(t, httphelper.ReadBody(resp))
	if event != "UserPromptSubmit" {
		t.Errorf("hookEventName = %q, want UserPromptSubmit", event)
	}

	want := "These researches may be relevant to the task:\n" +
		"- [Alpha Doc](" + raPath + ") — alpha desc\n- [Beta Doc](" + rbPath + ") — beta desc"
	if ctx != want {
		t.Errorf("additionalContext = %q, want %q", ctx, want)
	}
}

func TestInjectionUpsDedup(t *testing.T) {
	t.Parallel()

	emb := &mapEmbedder{
		def:     []float32{0, 1},
		vectors: map[string][]float32{"zebra": {1, 0}, "Alpha zebra body": {1, 0}},
	}

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n",
		map[string]string{raPath: "# Alpha\n\nzebra body\n"},
		emb)

	payload := upsPayload(fx.cwd, "s1", "zebra")

	first := httphelper.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), payload)
	defer first.Body.Close()

	httphelper.AssertStatus(t, first, http.StatusOK)

	if strings.TrimSpace(string(httphelper.ReadBody(first))) == "" {
		t.Fatalf("first UPS returned an empty body, want an offer")
	}

	second := httphelper.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), payload)
	defer second.Body.Close()

	httphelper.AssertStatus(t, second, http.StatusOK)

	if body := string(httphelper.ReadBody(second)); body != "" {
		t.Errorf("second UPS body = %q, want empty (already injected)", body)
	}
}

func TestInjectionPtuDifferentEntry(t *testing.T) {
	t.Parallel()

	// The UPS prompt matches only Alpha; the transcript delta matches
	// only Beta. After the UPS injects Alpha, the PTU must offer Beta.
	emb := &mapEmbedder{
		def: []float32{0, 1},
		vectors: map[string][]float32{
			"zebra":            {1, 0},
			"Alpha zebra body": {1, 0},
			"Beta yak body":    {0, 1},
			"yak":              {0, 1},
		},
	}

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n- [Beta Doc]("+rbPath+") — beta desc\n",
		map[string]string{
			raPath: "# Alpha\n\nzebra body\n",
			rbPath: "# Beta\n\nyak body\n",
		},
		emb)

	ups := httphelper.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), upsPayload(fx.cwd, "s1", "zebra"))
	defer ups.Body.Close()

	httphelper.AssertStatus(t, ups, http.StatusOK)

	event, ctx := additionalContext(t, httphelper.ReadBody(ups))
	if event != "UserPromptSubmit" || !strings.Contains(ctx, raPath) {
		t.Fatalf("UPS = %q / %q, want Alpha offered", event, ctx)
	}

	// Append an assistant line matching Beta before the PTU.
	transcript := filepath.Join(t.TempDir(), "transcript.jsonl")

	assistant := `{"type":"assistant","isSidechain":false,"message":{"content":[{"type":"text","text":"yak"}]}}`
	if err := os.WriteFile(transcript, []byte(assistant+"\n"), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	ptu := map[string]any{
		"session_id":      "s1",
		"transcript_path": transcript,
		"cwd":             fx.cwd,
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "ls"},
	}

	resp := httphelper.PostJSON(t, fx.server, events.HookEndpoint("PreToolUse"), ptu)
	defer resp.Body.Close()

	httphelper.AssertStatus(t, resp, http.StatusOK)

	event, ctx = additionalContext(t, httphelper.ReadBody(resp))
	if event != "PreToolUse" {
		t.Errorf("hookEventName = %q, want PreToolUse", event)
	}

	if !strings.Contains(ctx, rbPath) {
		t.Errorf("PTU additionalContext = %q, want Beta offered", ctx)
	}

	if strings.Contains(ctx, raPath) {
		t.Errorf("PTU additionalContext = %q, must not re-offer Alpha (already injected)", ctx)
	}
}

func TestInjectionUpsIrrelevant(t *testing.T) {
	t.Parallel()

	// The prompt matches the doc by BM25 but scores below the floor.
	emb := &mapEmbedder{
		def:     []float32{0, 1},
		vectors: map[string][]float32{"zebra": {1, 0}, "Alpha zebra body": {0.2, 0.98}},
	}

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n",
		map[string]string{raPath: "# Alpha\n\nzebra body\n"},
		emb)

	resp := httphelper.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), upsPayload(fx.cwd, "s1", "zebra"))
	defer resp.Body.Close()

	httphelper.AssertStatus(t, resp, http.StatusOK)

	if body := string(httphelper.ReadBody(resp)); body != "" {
		t.Errorf("UPS body = %q, want empty (below the score floor)", body)
	}
}

func TestInjectionEmbedderDown(t *testing.T) {
	t.Parallel()

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n",
		map[string]string{raPath: "# Alpha\n\nzebra body\n"},
		failingEmbedder{})

	resp := httphelper.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), upsPayload(fx.cwd, "s1", "zebra"))
	defer resp.Body.Close()

	httphelper.AssertStatus(t, resp, http.StatusOK)

	if body := string(httphelper.ReadBody(resp)); body != "" {
		t.Errorf("UPS body = %q, want empty (embedder down degrades to 200 empty)", body)
	}
}

func TestInjectionMissingFields(t *testing.T) {
	t.Parallel()

	emb := &mapEmbedder{
		def:     []float32{0, 1},
		vectors: map[string][]float32{"zebra": {1, 0}, "Alpha zebra body": {1, 0}},
	}

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n",
		map[string]string{raPath: "# Alpha\n\nzebra body\n"},
		emb)

	for name, payload := range map[string]map[string]any{
		"no_session_id": {"cwd": fx.cwd, "prompt": "zebra"},
		"no_cwd":        {"session_id": "s1", "prompt": "zebra"},
	} {
		resp := httphelper.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), payload)
		defer resp.Body.Close()

		httphelper.AssertStatus(t, resp, http.StatusOK)

		if body := string(httphelper.ReadBody(resp)); body != "" {
			t.Errorf("%s: UPS body = %q, want empty", name, body)
		}
	}
}

func TestInjectionPayloadSize(t *testing.T) {
	t.Parallel()

	emb := &mapEmbedder{
		def:     []float32{0, 1},
		vectors: map[string][]float32{"zebra": {1, 0}, "Alpha zebra body": {1, 0}},
	}

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n",
		map[string]string{raPath: "# Alpha\n\nzebra body\n"},
		emb)

	maxSize := 1 << 20 // must match httphandler.maxPayloadBytes

	cases := []struct {
		name, session string
		size          int
		wantEmpty     bool
	}{
		{"at limit", "s1", maxSize, false},
		{"over limit", "s2", maxSize + 1, true},
		{"half size", "s3", 3 << 18, false},  // above a halved limit
		{"double size", "s4", 5 << 18, true}, // below a doubled limit
	}

	for _, tc := range cases {
		body := sizedUpsBody(tc.session, fx.cwd, tc.size)

		resp := httphelper.PostRaw(t, fx.server, events.HookEndpoint("UserPromptSubmit"), body)
		defer resp.Body.Close()

		httphelper.AssertStatus(t, resp, http.StatusOK)

		empty := strings.TrimSpace(string(httphelper.ReadBody(resp))) == ""
		if empty != tc.wantEmpty {
			t.Errorf("%s: empty body = %v, want %v", tc.name, empty, tc.wantEmpty)
		}
	}
}

func TestInjectionMalformedPayload(t *testing.T) {
	t.Parallel()

	emb := &mapEmbedder{
		def:     []float32{0, 1},
		vectors: map[string][]float32{"zebra": {1, 0}, "Alpha zebra body": {1, 0}},
	}

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n",
		map[string]string{raPath: "# Alpha\n\nzebra body\n"},
		emb)

	// Valid fields but a truncated object: unmarshal fills the payload
	// before failing, so only decode's failure path keeps it empty.
	body := []byte(`{"session_id":"s1","cwd":"` + fx.cwd + `","prompt":"zebra"`)

	resp := httphelper.PostRaw(t, fx.server, events.HookEndpoint("UserPromptSubmit"), body)
	defer resp.Body.Close()

	httphelper.AssertStatus(t, resp, http.StatusOK)

	if b := string(httphelper.ReadBody(resp)); b != "" {
		t.Errorf("UPS body = %q, want empty (malformed payload)", b)
	}
}

func TestInjectionPtuToolInput(t *testing.T) {
	t.Parallel()

	emb := &mapEmbedder{
		def: []float32{0, 1},
		vectors: map[string][]float32{
			`{"command":"zebra"}`: {1, 0},
			"Alpha zebra body":    {1, 0},
		},
	}

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n",
		map[string]string{raPath: "# Alpha\n\nzebra body\n"},
		emb)

	ptu := map[string]any{
		"session_id": "s1",
		"cwd":        fx.cwd,
		"tool_input": map[string]any{"command": "zebra"},
	}

	resp := httphelper.PostJSON(t, fx.server, events.HookEndpoint("PreToolUse"), ptu)
	defer resp.Body.Close()

	httphelper.AssertStatus(t, resp, http.StatusOK)

	event, ctx := additionalContext(t, httphelper.ReadBody(resp))
	if event != "PreToolUse" || !strings.Contains(ctx, raPath) {
		t.Fatalf("PTU = %q / %q, want Alpha offered from the tool input", event, ctx)
	}
}

func TestInjectionPtuLastPrompt(t *testing.T) {
	t.Parallel()

	emb := &mapEmbedder{
		def:     []float32{0, 1},
		vectors: map[string][]float32{"zebra": {1, 0}, "Alpha zebra body": {1, 0}},
	}

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n",
		map[string]string{raPath: "# Alpha\n\nzebra body\n"},
		emb)

	// Seed a persisted prompt; with no transcript or tool input it is
	// the PTU's only query segment.
	if err := fx.queries.UpsertSessionState(context.Background(), db.UpsertSessionStateParams{
		SessionID: "s1", TranscriptOffset: 0, InjectedEntries: "[]", LastPrompt: "zebra",
	}); err != nil {
		t.Fatalf("seed session state: %v", err)
	}

	ptu := map[string]any{"session_id": "s1", "cwd": fx.cwd}

	resp := httphelper.PostJSON(t, fx.server, events.HookEndpoint("PreToolUse"), ptu)
	defer resp.Body.Close()

	httphelper.AssertStatus(t, resp, http.StatusOK)

	event, ctx := additionalContext(t, httphelper.ReadBody(resp))
	if event != "PreToolUse" || !strings.Contains(ctx, raPath) {
		t.Fatalf("PTU = %q / %q, want Alpha offered from the persisted prompt", event, ctx)
	}
}

func TestInjectionUpsPersistsPrompt(t *testing.T) {
	t.Parallel()

	emb := &mapEmbedder{
		def: []float32{0, 1},
		vectors: map[string][]float32{
			"zebra":            {1, 0},
			"Alpha zebra body": {1, 0},
			"yak":              {0, 1},
			"Beta yak body":    {0, 1},
		},
	}

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n- [Beta Doc]("+rbPath+") — beta desc\n",
		map[string]string{
			raPath: "# Alpha\n\nzebra body\n",
			rbPath: "# Beta\n\nyak body\n",
		},
		emb)

	// Seed a stale prompt: the UPS must overwrite it, so the follow-up
	// PTU (whose only segment is the persisted prompt) finds only the
	// already-injected Alpha.
	if err := fx.queries.UpsertSessionState(context.Background(), db.UpsertSessionStateParams{
		SessionID: "s1", TranscriptOffset: 0, InjectedEntries: "[]", LastPrompt: "yak",
	}); err != nil {
		t.Fatalf("seed session state: %v", err)
	}

	ups := httphelper.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), upsPayload(fx.cwd, "s1", "zebra"))
	defer ups.Body.Close()

	httphelper.AssertStatus(t, ups, http.StatusOK)

	if _, ctx := additionalContext(t, httphelper.ReadBody(ups)); !strings.Contains(ctx, raPath) {
		t.Fatalf("UPS = %q, want Alpha offered", ctx)
	}

	ptu := map[string]any{"session_id": "s1", "cwd": fx.cwd}

	resp := httphelper.PostJSON(t, fx.server, events.HookEndpoint("PreToolUse"), ptu)
	defer resp.Body.Close()

	httphelper.AssertStatus(t, resp, http.StatusOK)

	if body := string(httphelper.ReadBody(resp)); body != "" {
		t.Errorf("PTU body = %q, want empty (the UPS persisted its prompt)", body)
	}
}

func TestInjectionPtuMissingSession(t *testing.T) {
	t.Parallel()

	emb := &mapEmbedder{
		def:     []float32{0, 1},
		vectors: map[string][]float32{"yak": {0, 1}, "Beta yak body": {0, 1}},
	}

	fx := newInjectionFixture(t,
		"- [Beta Doc]("+rbPath+") — beta desc\n",
		map[string]string{rbPath: "# Beta\n\nyak body\n"},
		emb)

	transcript := filepath.Join(t.TempDir(), "transcript.jsonl")

	assistant := `{"type":"assistant","isSidechain":false,"message":{"content":[{"type":"text","text":"yak"}]}}`
	if err := os.WriteFile(transcript, []byte(assistant+"\n"), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	ptu := map[string]any{
		"cwd":             fx.cwd,
		"transcript_path": transcript,
	}

	resp := httphelper.PostJSON(t, fx.server, events.HookEndpoint("PreToolUse"), ptu)
	defer resp.Body.Close()

	httphelper.AssertStatus(t, resp, http.StatusOK)

	if body := string(httphelper.ReadBody(resp)); body != "" {
		t.Errorf("PTU body = %q, want empty (missing session_id)", body)
	}
}

func TestInjectionUpsMissingCwdLeavesNoState(t *testing.T) {
	t.Parallel()

	emb := &mapEmbedder{
		def:     []float32{0, 1},
		vectors: map[string][]float32{"zebra": {1, 0}, "Alpha zebra body": {1, 0}},
	}

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n",
		map[string]string{raPath: "# Alpha\n\nzebra body\n"},
		emb)

	payload := map[string]any{"session_id": "s1", "prompt": "zebra"}

	resp := httphelper.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), payload)
	defer resp.Body.Close()

	httphelper.AssertStatus(t, resp, http.StatusOK)

	if body := string(httphelper.ReadBody(resp)); body != "" {
		t.Errorf("UPS body = %q, want empty (missing cwd)", body)
	}

	// A rejected request must not create session state.
	if _, err := fx.queries.GetSessionState(context.Background(), "s1"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetSessionState(s1) = %v, want sql.ErrNoRows", err)
	}
}
