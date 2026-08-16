-- name: UpsertResearchEntry :exec
INSERT INTO research_entries (project, title, path, description, mtime)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (project, path) DO UPDATE SET
    title = excluded.title,
    description = excluded.description,
    mtime = excluded.mtime;

-- name: GetResearchEntry :one
SELECT * FROM research_entries
WHERE id = ? LIMIT 1;

-- name: ResearchEntriesByProject :many
SELECT * FROM research_entries
WHERE project = ?
ORDER BY path;

-- name: DeleteResearchEntry :exec
DELETE FROM research_entries
WHERE id = ?;

-- name: InsertResearchChunk :exec
INSERT INTO research_chunks (entry, heading, text, vector, dim, doc_id)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ResearchChunkByDocID :one
SELECT * FROM research_chunks
WHERE doc_id = ? LIMIT 1;

-- name: ResearchChunksByEntry :many
SELECT * FROM research_chunks
WHERE entry = ?
ORDER BY id;

-- name: DeleteResearchChunksByEntry :exec
DELETE FROM research_chunks
WHERE entry = ?;

-- name: GetEmbedCache :one
SELECT * FROM embed_cache
WHERE sha256 = ? LIMIT 1;

-- name: UpsertEmbedCache :exec
INSERT INTO embed_cache (sha256, vector, dim, model)
VALUES (?, ?, ?, ?)
ON CONFLICT (sha256) DO UPDATE SET
    vector = excluded.vector,
    dim = excluded.dim,
    model = excluded.model;

-- name: GetSessionState :one
SELECT * FROM session_state
WHERE session_id = ? LIMIT 1;

-- name: UpsertSessionState :exec
INSERT INTO session_state (session_id, transcript_offset, injected_entries, last_prompt)
VALUES (?, ?, ?, ?)
ON CONFLICT (session_id) DO UPDATE SET
    transcript_offset = excluded.transcript_offset,
    injected_entries = excluded.injected_entries,
    last_prompt = excluded.last_prompt;
