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
