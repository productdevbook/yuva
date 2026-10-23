-- name: CreateTeam :one
INSERT INTO teams (id, workspace_id, name, description, assignment_mode, assignment_cap, created_at, updated_at)
VALUES (@id, @workspace_id, @name, @description, @assignment_mode, @assignment_cap, @now, @now)
RETURNING *;

-- name: GetTeam :one
SELECT * FROM teams WHERE workspace_id = $1 AND id = $2;

-- name: LockTeam :one
SELECT * FROM teams WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- Agents and keys limited to inboxes see the teams they belong to and the teams granted an inbox
-- they see.
-- name: ListTeams :many
SELECT t.* FROM teams t
WHERE t.workspace_id = @workspace_id
  AND (sqlc.narg(id)::uuid IS NULL OR t.id = sqlc.narg(id)::uuid)
  AND (@all_teams::bool
       OR EXISTS (
           SELECT 1 FROM team_members tm
           WHERE tm.workspace_id = t.workspace_id AND tm.team_id = t.id AND tm.member_id = @viewer_id)
       OR EXISTS (
           SELECT 1 FROM team_inboxes ti
           JOIN inbox_viewers iv ON iv.workspace_id = ti.workspace_id AND iv.inbox_id = ti.inbox_id
           WHERE ti.workspace_id = t.workspace_id AND ti.team_id = t.id AND iv.viewer_id = @viewer_id))
ORDER BY t.name, t.id;

-- name: UpdateTeam :one
UPDATE teams SET name = @name, description = @description, assignment_mode = @assignment_mode, assignment_cap = @assignment_cap, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: TouchTeam :one
UPDATE teams SET updated_at = @now WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteTeam :execrows
DELETE FROM teams WHERE workspace_id = $1 AND id = $2;

-- name: ListTeamMemberIDs :many
SELECT team_id, member_id FROM team_members
WHERE workspace_id = @workspace_id AND team_id = ANY(@team_ids::uuid[])
ORDER BY team_id, created_at, member_id;

-- name: ListTeamInboxIDs :many
SELECT team_id, inbox_id FROM team_inboxes
WHERE workspace_id = @workspace_id AND team_id = ANY(@team_ids::uuid[])
ORDER BY team_id, created_at, inbox_id;

-- name: AddTeamMember :execrows
INSERT INTO team_members (workspace_id, team_id, member_id, created_at)
VALUES (@workspace_id, @team_id, @member_id, @now)
ON CONFLICT DO NOTHING;

-- name: RemoveTeamMember :execrows
DELETE FROM team_members WHERE workspace_id = $1 AND team_id = $2 AND member_id = $3;

-- name: CountTeamMembers :one
SELECT count(*) FROM team_members WHERE workspace_id = $1 AND team_id = $2;

-- name: AddTeamInbox :execrows
INSERT INTO team_inboxes (workspace_id, team_id, inbox_id, created_at)
VALUES (@workspace_id, @team_id, @inbox_id, @now)
ON CONFLICT DO NOTHING;

-- name: RemoveTeamInbox :execrows
DELETE FROM team_inboxes WHERE workspace_id = $1 AND team_id = $2 AND inbox_id = $3;

-- name: CountTeamInboxes :one
SELECT count(*) FROM team_inboxes WHERE workspace_id = $1 AND team_id = $2;

-- name: TeamHasInbox :one
SELECT EXISTS (
    SELECT 1 FROM team_inboxes WHERE workspace_id = @workspace_id AND team_id = @team_id AND inbox_id = @inbox_id
) AS access;

-- name: ListMemberTeamIDs :many
SELECT team_id FROM team_members WHERE workspace_id = @workspace_id AND member_id = @member_id ORDER BY team_id;

-- Changes to inbox access take this lock first, so two of them never miss each other's flip of a
-- member's effective access.
-- name: LockInboxAccess :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_inbox_access:' || sqlc.arg(workspace_id)::uuid::text, 0));

-- name: ListMembersInboxAccess :many
SELECT DISTINCT member_id, inbox_id FROM member_inbox_access
WHERE workspace_id = @workspace_id AND member_id = ANY(@member_ids::uuid[])
ORDER BY member_id, inbox_id;

-- Taken before the conversation's lock when a team is set, so a concurrent delete or revoke of the
-- team (which locks the row FOR UPDATE first) either waits or is seen.
-- name: ShareLockTeam :one
SELECT id FROM teams WHERE workspace_id = $1 AND id = $2 FOR SHARE;

-- name: ClearConversationsTeam :many
UPDATE conversations SET team_id = NULL, updated_at = @now
WHERE workspace_id = @workspace_id AND team_id = @team_id
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR inbox_id = sqlc.narg(inbox_id)::uuid)
RETURNING *;
