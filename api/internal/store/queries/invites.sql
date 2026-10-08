-- name: UpsertInvite :one
INSERT INTO invites (id, workspace_id, email, role, locale, invited_by, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (workspace_id, email) DO UPDATE SET
    role = excluded.role,
    locale = excluded.locale,
    invited_by = excluded.invited_by,
    created_at = excluded.created_at,
    expires_at = excluded.expires_at
RETURNING *;

-- name: ListInvites :many
SELECT * FROM invites WHERE workspace_id = $1 AND expires_at > $2 ORDER BY created_at, id;

-- name: DeleteInvite :execrows
DELETE FROM invites WHERE workspace_id = $1 AND id = $2;

-- name: TakePendingInvites :many
DELETE FROM invites WHERE email = @email AND expires_at > @now
  AND workspace_id IN (SELECT id FROM workspaces WHERE deleted_at IS NULL)
RETURNING *;

-- name: LatestPendingInvite :one
SELECT i.* FROM invites i JOIN workspaces w ON w.id = i.workspace_id
WHERE i.email = @email AND i.expires_at > @now AND w.deleted_at IS NULL
ORDER BY i.created_at DESC LIMIT 1;
