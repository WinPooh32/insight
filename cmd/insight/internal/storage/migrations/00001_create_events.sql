-- +goose Up
CREATE TABLE events (
    id         TEXT    PRIMARY KEY,
    event_type TEXT    NOT NULL,
    received   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    payload    TEXT    NOT NULL,
    session_id TEXT
);

CREATE INDEX idx_events_session_id ON events(session_id);
CREATE INDEX idx_events_event_type ON events(event_type);
CREATE INDEX idx_events_received ON events(received);

-- +goose Down
DROP INDEX IF EXISTS idx_events_session_id;
DROP INDEX IF EXISTS idx_events_event_type;
DROP INDEX IF EXISTS idx_events_received;
DROP TABLE IF EXISTS events;
