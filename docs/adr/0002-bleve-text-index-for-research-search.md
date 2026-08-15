# Use Bleve text-only index for research search, vectors in SQLite

- **Status:** accepted
- **Date:** 2026-08-16

## Context

The research-link injection feature ([architecture](../architecture.md))
ranks a per-project research corpus (entries + markdown sections). It matches
the corpus against the agent's current prompt, recent assistant text, and the
pending tool call. The pipeline is a keyword pre-filter followed by vector
ranking. Chunk embeddings come from an external OpenAI-compatible server,
e.g. llama.cpp on localhost.

### Constraints

- ADR 0001 requires a pure-Go build with no system-level dependencies
  (`modernc.org/sqlite` was chosen specifically to avoid system SQLite).
- The corpus is small today (single research document) and expected to grow
  slowly — hundreds of chunks at most for the foreseeable future.
- Chunk vectors must be re-embeddable when the embedding model changes.

## Options Considered

- **Bleve with native kNN (FAISS)** — Full native hybrid search: BM25 +
  HNSW kNN in one index. However, Bleve vector search mandates the FAISS C++
  shared library: CGO build-time link (`-lfaiss_c`), the `vectors` build tag,
  and a pin to select `blevesearch/faiss` checkpoints. There is no pure-Go
  code path. This breaks the pure-Go build constraint on every build and
  runtime machine.
- **Bleve text-only + SQLite vectors + Go cosine** — Bleve built *without*
  the `vectors` tag provides BM25, analyzers, and a queryable index
  directory in pure Go. Chunk vectors are stored as BLOBs in SQLite; ranking
  is brute-force cosine over pre-filtered candidates, computed in Go.
- **All-SQLite, no Bleve** — Hand-rolled token overlap pre-filter plus Go
  cosine. Zero new dependencies, but gives up real tokenization, stemming,
  and a queryable index.

## Decision

Use Bleve (v2, pure Go, no `vectors` tag) as the text pre-filter index and
store chunk vectors in SQLite with Go-side brute-force cosine ranking.

## Consequences

- Pure-Go build is preserved; no system libraries or CGO toolchain required.
- BM25 scoring, analyzers, and a queryable index come free from Bleve.
- Index state is split across two stores (Bleve text index + SQLite
  vectors); the lazy mtime reindex must keep both in sync.
- There is no native hybrid scoring; the max-merge across query segments is
  hand-rolled in Go.
- Brute-force cosine is O(candidates × dim) per request. At the current
  corpus scale this is microseconds. **Upgrade path:** when the corpus grows
  until brute force is measurable, swap the ranker to native Bleve kNN and
  accept the FAISS/CGO cost (new ADR superseding this one).
- New pure-Go dependency: `github.com/blevesearch/bleve/v2`.
