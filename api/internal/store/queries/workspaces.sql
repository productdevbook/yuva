-- name: CreateWorkspace :one
INSERT INTO workspaces (id, name) VALUES ($1, $2)
RETURNING *;

-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id = $1;

-- name: LockWorkspace :one
SELECT id FROM workspaces WHERE id = $1 FOR UPDATE;

-- name: CountWorkspaces :one
SELECT count(*) FROM workspaces;

-- name: LockBootstrap :exec
SELECT pg_advisory_xact_lock(hashtext('yuva.bootstrap'));

-- name: ListWorkspaceIDs :many
SELECT id FROM workspaces ORDER BY id;

-- name: ListWorkspacesByName :many
SELECT * FROM workspaces WHERE name = $1 ORDER BY created_at, id;

-- name: SetWorkspaceRetention :one
UPDATE workspaces SET retention_days = sqlc.narg(retention_days)
WHERE id = @id
RETURNING *;

-- name: ListWorkspaceRetention :many
SELECT id, retention_days::integer AS retention_days FROM workspaces WHERE retention_days IS NOT NULL ORDER BY id;
