# Architecture — Research-Link Injection

- **Status:** agreed (design), not yet implemented
- **Date:** 2026-08-16
- **Related:**
  [ADR 0001](adr/0001-hooks-relay-service.md) (hook relay service),
  [ADR 0002](adr/0002-bleve-text-index-for-research-search.md) (Bleve text-only index, vectors in SQLite)

## Overview

Insight scores a project's research corpus against what the agent is doing.
It injects the top-3 research links into the Claude Code context as
`additionalContext`. Delivery uses two `type: "http"` hooks pointing at an
insight endpoint. When nothing is relevant — or a dependency is down — the
hook returns an empty body and the prompt proceeds.

## Injection

| Hook               | Query segments (each embedded separately)                                                 |
| ------------------ | ----------------------------------------------------------------------------------------- |
| `UserPromptSubmit` | {user prompt}                                                                             |
| `PreToolUse`       | {user prompt} ∪ {assistant text blocks since the last tool call} ∪ {current `tool_input`} |

- Assistant **text** blocks only — thinking blocks are excluded.
- The transcript is delta-parsed via a per-session byte offset on
  `transcript_path`. If the file shrinks (compaction) the offset resets to 0;
  the embedding hash-cache absorbs the re-read. Main transcript only; subagent
  transcripts are out of scope.
- Output: up to 3 lines in `index.md` format, one per research doc no matter
  how many of its sections matched:

  ```text
  - [title](path) — description
  ```

  Paths are emitted exactly as stored in `index.md` (repo-root-relative =
  cwd-relative). Per-session dedup: an entry already injected in the session
  is never offered again.

## Relevance pipeline (per request)

1. Resolve the corpus for `cwd`: `<cwd>/.claude/skills/research/index.md`
   entries plus their files.
2. **Lazy mtime reindex** when files changed:
  - **Bleve text index** (pure Go, built *without* the `vectors` tag): one
     document per entry (title + description) and one per markdown section,
     with a project field for term filtering. BM25 and analyzers come free.
  - **SQLite `research_chunks`**: chunk text + vector BLOB + dimension,
     embedded via the configured embedder.
3. Embed the request's query segments — **content-hash cache** in SQLite;
   each text block is embedded once per model.
4. **Pre-filter**: Bleve BM25 query over the concatenated signal text,
   project-filtered, size-capped → candidate chunks.
5. **Rank**: candidate vectors from SQLite, cosine against each segment
   vector, **max across segments** in Go.
6. Score floor → top 3 → per-session dedup → `additionalContext`.
7. Nothing passes the floor → empty body.

## Storage

- Bleve index directory (text-only).
- SQLite tables:
  - `research_entries` — project, title, path, description, mtime
  - `research_chunks` — entry, heading, text, vector BLOB, dim
  - `embed_cache` — sha256(text) → vector, dim, model
  - `session_state` — transcript byte offset, injected-entry set

## Embedder

Any OpenAI-compatible `/v1/embeddings` endpoint (model-agnostic).

- `EMBED_BASE_URL` — default `http://localhost:8080/v1` (llama.cpp)
- `EMBED_MODEL`
- `EMBED_API_KEY` — empty for local serving

The cloud API is a config choice, **never an automatic fallback** — session
prompts and tool inputs going to the cloud must be an explicit operator
decision. Dimension is stored per vector; changing `EMBED_MODEL` invalidates
all chunk vectors and the embed cache (full re-embed).

## Failure modes

| Failure                   | Behavior                                                     |
| ------------------------- | ------------------------------------------------------------ |
| Insight down / hook >30 s | Claude Code discards hook output; prompt proceeds (built-in) |
| Embedding server down     | Silent — no injection, no keyword fallback                   |

The embedding server is therefore a hard runtime dependency of this feature.

## Constraints and rationale

- **No Bleve kNN** ([ADR 0002](adr/0002-bleve-text-index-for-research-search.md)).
  Native Bleve vector search requires the FAISS C++ shared
  library (CGO, build-time link, pinned faiss checkpoints) — incompatible with
  the pure-Go build ADR 0001 relies on. Brute-force cosine in Go is
  microseconds at this corpus scale, so vectors live in SQLite and only the
  text half of Bleve is used.
  **Upgrade path:** when the corpus grows until brute force is measurable,
  swap the ranker to native Bleve kNN and accept the FAISS/CGO cost.
- **ADR note to record:** "zero external dependencies" (ADR 0001) is amended
  to "optional localhost embedding server; cloud embeddings only by explicit
  config". The indexer itself adds no constraint — Bleve text-only is a
  pure-Go dependency, same category as `modernc.org/sqlite`.

## Open assumptions

- **A1** — PreToolUse hook matcher = `Read|Edit|Write|Bash|Grep|Glob`
  (file-touching tools only; browser/TodoWrite inputs cannot match research).
  Trivially widened in `settings.json`.
- **A2** — emitted paths are exactly what `index.md` stores.
- **A3** — only files listed in `index.md` are eligible for injection; the
  index is the curation surface.
