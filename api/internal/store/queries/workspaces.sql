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
SELECT * FROM workspaces WHERE name = $1 AND deleted_at IS NULL ORDER BY created_at, id;

-- name: SetWorkspaceRetention :one
UPDATE workspaces SET retention_days = sqlc.narg(retention_days)
WHERE id = @id
RETURNING *;

-- name: ListWorkspaceRetention :many
SELECT id, retention_days::integer AS retention_days FROM workspaces WHERE retention_days IS NOT NULL ORDER BY id;

-- name: MarkWorkspaceDeleted :execrows
UPDATE workspaces SET deleted_at = @now WHERE id = @id AND deleted_at IS NULL;

-- name: IsWorkspaceLive :one
SELECT EXISTS (SELECT 1 FROM workspaces WHERE id = $1 AND deleted_at IS NULL) AS live;

-- name: GetDeletedWorkspace :one
SELECT * FROM workspaces WHERE id = $1 AND deleted_at IS NOT NULL;

-- name: ListDeletedWorkspaceIDs :many
SELECT id FROM workspaces WHERE deleted_at IS NOT NULL ORDER BY deleted_at, id;

-- name: PurgeWorkspace :execrows
DELETE FROM workspaces WHERE id = $1 AND deleted_at IS NOT NULL;

-- name: ListWorkspaceConversationIDs :many
SELECT id FROM conversations WHERE workspace_id = @workspace_id ORDER BY id LIMIT @max_rows;

-- name: CountWorkspaceConversations :one
SELECT count(*) FROM conversations WHERE workspace_id = $1;

-- name: DeleteWorkspaceContacts :execrows
DELETE FROM contacts c WHERE c.workspace_id = @workspace_id
  AND c.id IN (SELECT x.id FROM contacts x WHERE x.workspace_id = @workspace_id LIMIT @max_rows);

-- name: DeleteWorkspaceWebhookEndpoints :execrows
DELETE FROM webhook_endpoints WHERE workspace_id = $1;

-- Push subscriptions belong to a person (see "Hosting for others later"): those of members who
-- are left with no other workspace go with the workspace.
-- name: DeleteLonelyMembersPushSubscriptions :execrows
DELETE FROM push_subscriptions ps
WHERE ps.person_id IN (SELECT m.person_id FROM members m WHERE m.workspace_id = @workspace_id)
  AND NOT EXISTS (
      SELECT 1 FROM members o JOIN workspaces w ON w.id = o.workspace_id
      WHERE o.person_id = ps.person_id AND o.workspace_id <> @workspace_id AND w.deleted_at IS NULL
  );

-- name: ListOwnedWorkspaces :many
SELECT w.id, w.name FROM members m JOIN workspaces w ON w.id = m.workspace_id
WHERE m.person_id = $1 AND m.role = 'owner' AND w.deleted_at IS NULL
ORDER BY w.id;
