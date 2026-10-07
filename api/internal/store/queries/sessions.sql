-- name: CreateSession :exec
INSERT INTO sessions (id, person_id, token_hash, method, created_at, expires_at, last_seen_at)
VALUES ($1, $2, $3, $4, $5, $6, $5);

-- name: GetSessionByTokenHash :one
SELECT * FROM sessions WHERE token_hash = $1 AND expires_at > $2;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = @now::timestamptz
WHERE id = @id AND last_seen_at < @stale_before::timestamptz;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;
