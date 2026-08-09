-- name: InsertEvent :exec
INSERT INTO events (id, event_type, received, payload, session_id)
VALUES (?, ?, ?, ?, ?);

-- name: GetEvent :one
SELECT * FROM events
WHERE id = ? LIMIT 1;

-- name: RecentEvents :many
SELECT * FROM events
ORDER BY received DESC
LIMIT ?
OFFSET ?;

-- name: EventsBySession :many
SELECT * FROM events
WHERE session_id = ?
ORDER BY received;

-- name: EventsByType :many
SELECT * FROM events
WHERE event_type = ?
ORDER BY received DESC
LIMIT ?
OFFSET ?;

-- name: EventCount :one
SELECT count(*) FROM events;
