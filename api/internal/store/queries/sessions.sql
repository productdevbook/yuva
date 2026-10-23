-- name: CreateSession :exec
INSERT INTO sessions (id, person_id, token_hash, method, created_at, expires_at, last_seen_at, sso_workspace_id, sso_settings_id, user_agent)
VALUES (@id, @person_id, @token_hash, @method, @created_at, @expires_at, @created_at, sqlc.narg(sso_workspace_id), sqlc.narg(sso_settings_id), sqlc.narg(user_agent));

-- name: GetSessionByTokenHash :one
SELECT * FROM sessions WHERE token_hash = $1 AND expires_at > $2;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = @now::timestamptz
WHERE id = @id AND last_seen_at < @stale_before::timestamptz;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: ListSessions :many
SELECT id, method, user_agent, created_at, last_seen_at, expires_at FROM sessions
WHERE person_id = @person_id AND expires_at > @now::timestamptz
ORDER BY last_seen_at DESC, id;

-- name: DeleteOtherSessions :execrows
DELETE FROM sessions WHERE person_id = @person_id AND id <> @keep;
