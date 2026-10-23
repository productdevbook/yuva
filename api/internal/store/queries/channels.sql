-- name: CreateChannel :one
INSERT INTO channels (id, workspace_id, inbox_id, kind, name, settings, attachment_max_bytes, attachment_types, created_at, updated_at)
VALUES (@id, @workspace_id, @inbox_id, @kind, @name, @settings, sqlc.narg(attachment_max_bytes), sqlc.narg(attachment_types)::text[], @now, @now)
RETURNING *;

-- name: GetChannel :one
SELECT * FROM channels WHERE workspace_id = $1 AND id = $2;

-- name: ListChannels :many
SELECT * FROM channels WHERE workspace_id = $1 AND inbox_id = $2 ORDER BY created_at, id;

-- name: UpdateChannel :one
UPDATE channels SET name = @name, settings = @settings,
    attachment_max_bytes = CASE WHEN @set_attachment_max_bytes::bool THEN sqlc.narg(attachment_max_bytes) ELSE attachment_max_bytes END,
    attachment_types = CASE WHEN @set_attachment_types::bool THEN sqlc.narg(attachment_types)::text[] ELSE attachment_types END,
    updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteChannel :exec
DELETE FROM channels WHERE workspace_id = $1 AND id = $2;

-- name: GetChannelAttachmentLimits :one
SELECT attachment_max_bytes, attachment_types FROM channels WHERE workspace_id = $1 AND id = $2;
