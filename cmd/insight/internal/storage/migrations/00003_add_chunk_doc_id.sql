-- +goose Up
-- research_chunks gains the exact Bleve doc ID of its text doc so the
-- ranker can map a pre-filter hit to its vector row. Existing rows are
-- section chunks only (entry chunks land with the ranker), inserted in
-- ordinal order, so their doc IDs are reconstructed from the entry's
-- project/path and the row's position within the entry.
CREATE TABLE research_chunks_new (
    id      INTEGER PRIMARY KEY,
    entry   INTEGER NOT NULL REFERENCES research_entries(id) ON DELETE CASCADE,
    heading TEXT    NOT NULL DEFAULT '',
    text    TEXT    NOT NULL,
    vector  BLOB,
    dim     INTEGER,
    doc_id  TEXT    UNIQUE NOT NULL
);

INSERT INTO research_chunks_new (id, entry, heading, text, vector, dim, doc_id)
SELECT c.id,
       c.entry,
       c.heading,
       c.text,
       c.vector,
       c.dim,
       'section:' || e.project || ':' || e.path || ':' || (c.id - (
           SELECT min(c2.id) FROM research_chunks c2 WHERE c2.entry = c.entry
       ))
FROM research_chunks c
JOIN research_entries e ON e.id = c.entry;

DROP TABLE research_chunks;
ALTER TABLE research_chunks_new RENAME TO research_chunks;
CREATE INDEX idx_research_chunks_entry ON research_chunks(entry);

-- +goose Down
DROP TABLE IF EXISTS research_chunks;
CREATE TABLE research_chunks (
    id      INTEGER PRIMARY KEY,
    entry   INTEGER NOT NULL REFERENCES research_entries(id) ON DELETE CASCADE,
    heading TEXT    NOT NULL DEFAULT '',
    text    TEXT    NOT NULL,
    vector  BLOB,
    dim     INTEGER
);
CREATE INDEX idx_research_chunks_entry ON research_chunks(entry);
