package httphandler_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/events"
	insighthttp "github.com/WinPooh32/insight/cmd/insight/internal/http"
	"github.com/WinPooh32/insight/cmd/insight/internal/research"
	"github.com/WinPooh32/insight/cmd/insight/internal/testutil"
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
	server *httptest.Server
	cwd    string
}

func newInjectionFixture(t *testing.T, index string, docs map[string]string, emb research.Embedding) *injectionFixture {
	t.Helper()

	storage := testutil.NewTestStorage(t)
	queries := storage.Queries()
	cwd := writeCorpus(t, index, docs)

	indexer, err := research.NewIndexer(t.TempDir(), queries, emb)
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}

	t.Cleanup(func() { indexer.Close() })

	logger := slog.New(slog.DiscardHandler)
	ranker := research.NewRanker(indexer, queries, emb)
	eventHandler := insighthttp.NewEventHandler(storage, nil, logger)
	injection := insighthttp.NewInjectionHandler(indexer, ranker, queries, eventHandler, logger)
	router := insighthttp.Router(storage, nil, logger, injection)

	return &injectionFixture{server: testutil.NewTestServer(t, router), cwd: cwd}
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

	resp := testutil.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), upsPayload(fx.cwd, "s1", "zebra"))
	defer resp.Body.Close()

	testutil.AssertStatus(t, resp, http.StatusOK)

	event, ctx := additionalContext(t, testutil.ReadBody(resp))
	if event != "UserPromptSubmit" {
		t.Errorf("hookEventName = %q, want UserPromptSubmit", event)
	}

	want := "- [Alpha Doc](" + raPath + ") — alpha desc\n- [Beta Doc](" + rbPath + ") — beta desc"
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

	first := testutil.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), payload)
	defer first.Body.Close()

	testutil.AssertStatus(t, first, http.StatusOK)

	if strings.TrimSpace(string(testutil.ReadBody(first))) == "" {
		t.Fatalf("first UPS returned an empty body, want an offer")
	}

	second := testutil.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), payload)
	defer second.Body.Close()

	testutil.AssertStatus(t, second, http.StatusOK)

	if body := string(testutil.ReadBody(second)); body != "" {
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

	ups := testutil.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), upsPayload(fx.cwd, "s1", "zebra"))
	defer ups.Body.Close()

	testutil.AssertStatus(t, ups, http.StatusOK)

	event, ctx := additionalContext(t, testutil.ReadBody(ups))
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

	resp := testutil.PostJSON(t, fx.server, events.HookEndpoint("PreToolUse"), ptu)
	defer resp.Body.Close()

	testutil.AssertStatus(t, resp, http.StatusOK)

	event, ctx = additionalContext(t, testutil.ReadBody(resp))
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

	resp := testutil.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), upsPayload(fx.cwd, "s1", "zebra"))
	defer resp.Body.Close()

	testutil.AssertStatus(t, resp, http.StatusOK)

	if body := string(testutil.ReadBody(resp)); body != "" {
		t.Errorf("UPS body = %q, want empty (below the score floor)", body)
	}
}

func TestInjectionEmbedderDown(t *testing.T) {
	t.Parallel()

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n",
		map[string]string{raPath: "# Alpha\n\nzebra body\n"},
		failingEmbedder{})

	resp := testutil.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), upsPayload(fx.cwd, "s1", "zebra"))
	defer resp.Body.Close()

	testutil.AssertStatus(t, resp, http.StatusOK)

	if body := string(testutil.ReadBody(resp)); body != "" {
		t.Errorf("UPS body = %q, want empty (embedder down degrades to 200 empty)", body)
	}
}

func TestInjectionStoresEvents(t *testing.T) {
	t.Parallel()

	emb := &mapEmbedder{
		def:     []float32{0, 1},
		vectors: map[string][]float32{"zebra": {1, 0}, "Alpha zebra body": {1, 0}},
	}

	fx := newInjectionFixture(t,
		"- [Alpha Doc]("+raPath+") — alpha desc\n",
		map[string]string{raPath: "# Alpha\n\nzebra body\n"},
		emb)

	ups := testutil.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), upsPayload(fx.cwd, "s1", "zebra"))
	defer ups.Body.Close()

	testutil.AssertStatus(t, ups, http.StatusOK)
	testutil.ReadBody(ups)

	ptu := testutil.PostJSON(t, fx.server, events.HookEndpoint("PreToolUse"), map[string]any{
		"session_id": "s1",
		"cwd":        fx.cwd,
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "ls"},
	})
	defer ptu.Body.Close()

	testutil.AssertStatus(t, ptu, http.StatusOK)
	testutil.ReadBody(ptu)

	resp := testutil.Get(t, fx.server, "/hooks/v1/events/session/s1")
	defer resp.Body.Close()

	testutil.AssertStatus(t, resp, http.StatusOK)

	var evts []map[string]any
	if err := json.Unmarshal(testutil.ReadBody(resp), &evts); err != nil {
		t.Fatalf("decode events: %v", err)
	}

	if len(evts) != 2 {
		t.Fatalf("stored %d events, want 2", len(evts))
	}

	seen := map[string]bool{}

	for _, e := range evts {
		if et, ok := e["event_type"].(string); ok {
			seen[et] = true
		}
	}

	if !seen["UserPromptSubmit"] || !seen["PreToolUse"] {
		t.Errorf("stored event types = %v, want UserPromptSubmit and PreToolUse", seen)
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
		resp := testutil.PostJSON(t, fx.server, events.HookEndpoint("UserPromptSubmit"), payload)
		defer resp.Body.Close()

		testutil.AssertStatus(t, resp, http.StatusOK)

		if body := string(testutil.ReadBody(resp)); body != "" {
			t.Errorf("%s: UPS body = %q, want empty", name, body)
		}
	}
}
