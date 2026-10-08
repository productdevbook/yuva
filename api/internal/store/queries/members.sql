-- name: CreateMember :one
INSERT INTO members (id, workspace_id, person_id, role) VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id, person_id) DO NOTHING
RETURNING *;

-- name: GetMember :one
SELECT m.id, m.workspace_id, m.person_id, m.role, m.created_at, p.email, p.name
FROM members m JOIN people p ON p.id = m.person_id
WHERE m.workspace_id = $1 AND m.id = $2;

-- name: GetMemberByPerson :one
SELECT m.* FROM members m JOIN workspaces w ON w.id = m.workspace_id
WHERE m.workspace_id = $1 AND m.person_id = $2 AND w.deleted_at IS NULL;

-- name: MemberExistsByEmail :one
SELECT EXISTS (
    SELECT 1 FROM members m JOIN people p ON p.id = m.person_id
    WHERE m.workspace_id = $1 AND p.email = $2
) AS member;

-- name: ListMembers :many
SELECT m.id, m.workspace_id, m.person_id, m.role, m.created_at, p.email, p.name
FROM members m JOIN people p ON p.id = m.person_id
WHERE m.workspace_id = $1
ORDER BY m.created_at, m.id;

-- name: ListMemberships :many
SELECT m.id AS member_id, m.role, w.id AS workspace_id, w.name AS workspace_name, w.created_at AS workspace_created_at,
       w.retention_days AS workspace_retention_days, w.bots_may_send AS workspace_bots_may_send
FROM members m JOIN workspaces w ON w.id = m.workspace_id
WHERE m.person_id = $1 AND w.deleted_at IS NULL
ORDER BY m.created_at, m.id;

-- name: CountOwners :one
SELECT count(*) FROM members WHERE workspace_id = $1 AND role = 'owner';

-- name: UpdateMemberRole :exec
UPDATE members SET role = $3 WHERE workspace_id = $1 AND id = $2;

-- name: DeleteMember :exec
DELETE FROM members WHERE workspace_id = $1 AND id = $2;
