-- +goose Up
-- session_state gains last_prompt: the last UserPromptSubmit prompt,
-- persisted at UPS time so PreToolUse (which does not carry the prompt)
-- can re-use it as a query segment. Rebuilt to add the column.
CREATE TABLE session_state_new (
    session_id        TEXT    PRIMARY KEY,
    transcript_offset INTEGER NOT NULL DEFAULT 0,
    injected_entries  TEXT    NOT NULL DEFAULT '[]',
    last_prompt       TEXT    NOT NULL DEFAULT ''
);

INSERT INTO session_state_new (session_id, transcript_offset, injected_entries)
SELECT session_id,
       transcript_offset,
       injected_entries
FROM session_state;

DROP TABLE session_state;
ALTER TABLE session_state_new RENAME TO session_state;

-- +goose Down
CREATE TABLE session_state_new (
    session_id        TEXT    PRIMARY KEY,
    transcript_offset INTEGER NOT NULL DEFAULT 0,
    injected_entries  TEXT    NOT NULL DEFAULT '[]'
);

INSERT INTO session_state_new (session_id, transcript_offset, injected_entries)
SELECT session_id,
       transcript_offset,
       injected_entries
FROM session_state;

DROP TABLE session_state;
ALTER TABLE session_state_new RENAME TO session_state;
