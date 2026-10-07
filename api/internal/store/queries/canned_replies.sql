-- name: CreateCannedReply :one
INSERT INTO canned_replies (id, workspace_id, shortcut, title, body, created_at, updated_at)
VALUES (@id, @workspace_id, @shortcut, @title, @body, @now, @now)
RETURNING *;

-- name: GetCannedReply :one
SELECT * FROM canned_replies WHERE workspace_id = $1 AND id = $2;

-- name: ListCannedReplies :many
SELECT * FROM canned_replies WHERE workspace_id = $1 ORDER BY shortcut;

-- name: UpdateCannedReply :one
UPDATE canned_replies SET shortcut = @shortcut, title = @title, body = @body, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteCannedReply :execrows
DELETE FROM canned_replies WHERE workspace_id = $1 AND id = $2;
