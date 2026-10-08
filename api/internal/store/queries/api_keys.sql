-- name: CreateAPIKey :one
INSERT INTO api_keys (id, workspace_id, name, prefix, secret_hash, created_by, scopes, inbox_limited, expires_at,
                      bot_name, bot_avatar_url)
VALUES (@id, @workspace_id, @name, @prefix, @secret_hash, sqlc.narg(created_by), @scopes, @inbox_limited,
        sqlc.narg(expires_at), sqlc.narg(bot_name), sqlc.narg(bot_avatar_url))
RETURNING *;

-- name: AddAPIKeyInbox :exec
INSERT INTO api_key_inboxes (workspace_id, api_key_id, inbox_id) VALUES ($1, $2, $3);

-- name: ListAPIKeyInboxes :many
SELECT api_key_id, inbox_id FROM api_key_inboxes
WHERE workspace_id = @workspace_id AND api_key_id = ANY(@api_key_ids::uuid[])
ORDER BY api_key_id, inbox_id;

-- name: UpdateAPIKey :one
UPDATE api_keys SET name = @name, bot_name = sqlc.narg(bot_name), bot_avatar_url = sqlc.narg(bot_avatar_url)
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: GetAPIKey :one
SELECT * FROM api_keys WHERE workspace_id = $1 AND id = $2;

-- name: ListAPIKeys :many
SELECT * FROM api_keys WHERE workspace_id = $1 ORDER BY created_at, id;

-- name: RevokeAPIKey :execrows
UPDATE api_keys SET revoked_at = coalesce(revoked_at, @now::timestamptz)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: GetActiveAPIKeyByHash :one
SELECT k.*, w.bots_may_send FROM api_keys k JOIN workspaces w ON w.id = k.workspace_id
WHERE k.secret_hash = $1 AND k.revoked_at IS NULL AND w.deleted_at IS NULL;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id
  AND (last_used_at IS NULL OR last_used_at < @stale_before::timestamptz);

-- name: GetAPIKeyByID :one
SELECT * FROM api_keys WHERE id = $1;
