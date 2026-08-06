# Use PostgreSQL for Primary Database

- **Status:** accepted
- **Date:** 2026-08-04

## Context

The project needs persistent storage. As the application grows beyond a simple
CLI, structured data must be stored reliably — configuration, state, and
application data.

Requirements:
- Relational data model with ACID guarantees
- First-class JSON support for semi-structured data
- Mature Go driver ecosystem
- Low operational overhead for a small team

## Options Considered

- **SQLite** — Zero-config, file-based. Great for local-first tools, but lacks
  concurrency under write load and has limited remote access patterns.
- **PostgreSQL** — Battle-tested relational database with JSONB, full-text search, and strong concurrency support.
- **MySQL** — Widely available but weaker JSON support and less consistent semantics than PostgreSQL.

## Decision

We will use **PostgreSQL** as the primary database.

## Consequences

- **Positive:** Rich data types (JSONB, arrays), strong consistency, well-maintained Go drivers (`pgx`).
- **Positive:** Team members likely already familiar with PostgreSQL; easy to find operational guidance.
- **Negative:** Requires a running database server — not zero-config like
  SQLite. Local development needs Docker or a local install.
- **Negative:** One more dependency to manage in CI/CD pipelines.
