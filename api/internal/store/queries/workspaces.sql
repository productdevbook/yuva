-- name: CreateWorkspace :one
INSERT INTO workspaces (id, name) VALUES ($1, $2)
RETURNING *;

-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id = $1;
