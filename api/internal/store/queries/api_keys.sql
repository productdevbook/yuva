-- name: CreateAPIKey :one
INSERT INTO api_keys (id, workspace_id, name, prefix, secret_hash, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListAPIKeys :many
SELECT * FROM api_keys WHERE workspace_id = $1 ORDER BY created_at, id;

-- name: RevokeAPIKey :execrows
UPDATE api_keys SET revoked_at = coalesce(revoked_at, @now::timestamptz)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: GetActiveAPIKeyByHash :one
SELECT * FROM api_keys WHERE secret_hash = $1 AND revoked_at IS NULL;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id
  AND (last_used_at IS NULL OR last_used_at < @stale_before::timestamptz);

-- name: GetAPIKeyByID :one
SELECT * FROM api_keys WHERE id = $1;
