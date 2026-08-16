-- +goose Up
CREATE TABLE research_entries (
    id          INTEGER PRIMARY KEY,
    project     TEXT    NOT NULL,
    title       TEXT    NOT NULL,
    path        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    mtime       TEXT    NOT NULL,
    UNIQUE(project, path)
);

CREATE TABLE research_chunks (
    id      INTEGER PRIMARY KEY,
    entry   INTEGER NOT NULL REFERENCES research_entries(id) ON DELETE CASCADE,
    heading TEXT    NOT NULL DEFAULT '',
    text    TEXT    NOT NULL,
    vector  BLOB,
    dim     INTEGER
);

CREATE INDEX idx_research_chunks_entry ON research_chunks(entry);

CREATE TABLE embed_cache (
    sha256 TEXT    PRIMARY KEY,
    vector BLOB    NOT NULL,
    dim    INTEGER NOT NULL,
    model  TEXT    NOT NULL
);

CREATE TABLE session_state (
    session_id        TEXT    PRIMARY KEY,
    transcript_offset INTEGER NOT NULL DEFAULT 0,
    injected_entries  TEXT    NOT NULL DEFAULT '[]'
);

-- +goose Down
DROP TABLE IF EXISTS session_state;
DROP TABLE IF EXISTS embed_cache;
DROP INDEX IF EXISTS idx_research_chunks_entry;
DROP TABLE IF EXISTS research_chunks;
DROP TABLE IF EXISTS research_entries;
