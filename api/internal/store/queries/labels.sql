-- name: CreateLabel :one
INSERT INTO labels (id, workspace_id, name, color, created_at) VALUES (@id, @workspace_id, @name, @color, @now)
RETURNING *;

-- name: GetLabel :one
SELECT * FROM labels WHERE workspace_id = $1 AND id = $2;

-- name: ListLabels :many
SELECT * FROM labels WHERE workspace_id = $1 ORDER BY name, id;

-- name: UpdateLabel :one
UPDATE labels SET name = @name, color = @color WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteLabel :execrows
DELETE FROM labels WHERE workspace_id = $1 AND id = $2;

-- name: CountLabels :one
SELECT count(*) FROM labels WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);
