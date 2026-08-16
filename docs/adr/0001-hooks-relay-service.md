# Use local HTTP relay service for Claude Code hook events

- **Status:** accepted
- **Date:** 2026-08-07

## Context

Claude Code fires hooks at key lifecycle points (session start, tool use, errors,
etc.). We need to capture these events for observability and debugging. The hooks
framework supports `type: "http"` hooks that POST to a local endpoint.

### Constraints

- Must run on localhost with an optional localhost embedding server; cloud
  embeddings only by explicit configuration (amended 2026-08-16; see
  [Amendment](#amendment))
- Must support querying events by session, type, and time range after the fact
- Should be type-safe to avoid runtime errors from schema drift

## Options Considered

- **File-based logging** — Write each hook to an append-only JSON file. Simple but
  no query capability; would need a separate tool to analyze.
- **External SaaS** — POST to an external analytics service. Adds network dependency
  and latency to the hook pipeline; privacy concerns with session data.
- **Local HTTP service with SQLite** — Standalone server on localhost that receives
  hooks via HTTP and stores them in SQLite. Self-contained, queryable, zero external
  deps.

## Decision

Use a standalone Go HTTP service on localhost (`:8765`) with SQLite storage. SQL
queries are type-checked at compile time via sqlc. Migrations are managed by goose
with embedded SQL files.

## Consequences

- Hook events are queryable immediately via `GET /hooks/v1/events` without post-processing
- SQLite serializes concurrent writes; acceptable for hook volume but not suitable
  for high-throughput use
- Events accumulate indefinitely; no retention policy (future work)
- Service binds to localhost only by default; not suitable for multi-machine setups
- Pure Go SQLite (`modernc.org/sqlite`) eliminates system-level SQLite dependency

## Amendment

- **Date:** 2026-08-16
- The "zero external dependencies" constraint is amended to "optional
  localhost embedding server; cloud embeddings only by explicit
  configuration" (see [architecture](../architecture.md), "Constraints and
  rationale").
- Bleve text-only ([ADR 0002](0002-bleve-text-index-for-research-search.md))
  is a pure-Go dependency in the same category as `modernc.org/sqlite` — no
  new constraint.
- **Date:** 2026-08-16
- The service is re-scoped to the research-link injection endpoints
  (`POST /hooks/v1/user-prompt-submit`, `POST /hooks/v1/pre-tool-use`) only.
  Event storage, the event query API (`GET /hooks/v1/events`), and the
  health endpoint are removed; the Consequences about event queryability
  and retention are superseded.
