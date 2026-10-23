-- name: CreateCannedReply :one
INSERT INTO canned_replies (id, workspace_id, shortcut, title, body, saved_by_kind, saved_by_member_id, saved_by_api_key_id, created_at, updated_at)
VALUES (@id, @workspace_id, @shortcut, @title, @body, @saved_by_kind, sqlc.narg(saved_by_member_id), sqlc.narg(saved_by_api_key_id), @now, @now)
RETURNING *;

-- name: GetCannedReply :one
SELECT * FROM canned_replies WHERE workspace_id = $1 AND id = $2;

-- name: ListCannedReplies :many
SELECT * FROM canned_replies WHERE workspace_id = $1 ORDER BY shortcut;

-- name: UpdateCannedReply :one
UPDATE canned_replies SET shortcut = @shortcut, title = @title, body = @body, saved_by_kind = sqlc.narg(saved_by_kind),
    saved_by_member_id = sqlc.narg(saved_by_member_id), saved_by_api_key_id = sqlc.narg(saved_by_api_key_id), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteCannedReply :execrows
DELETE FROM canned_replies WHERE workspace_id = $1 AND id = $2;
