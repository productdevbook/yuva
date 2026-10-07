-- name: CreateChannel :one
INSERT INTO channels (id, workspace_id, inbox_id, kind, name, settings, created_at, updated_at)
VALUES (@id, @workspace_id, @inbox_id, @kind, @name, @settings, @now, @now)
RETURNING *;

-- name: GetChannel :one
SELECT * FROM channels WHERE workspace_id = $1 AND id = $2;

-- name: ListChannels :many
SELECT * FROM channels WHERE workspace_id = $1 AND inbox_id = $2 ORDER BY created_at, id;

-- name: UpdateChannel :one
UPDATE channels SET name = @name, settings = @settings, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteChannel :exec
DELETE FROM channels WHERE workspace_id = $1 AND id = $2;
