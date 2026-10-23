-- name: LockSavedViews :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_saved_views:' || sqlc.arg(workspace_id)::uuid::text, 0));

-- name: CreateSavedView :one
INSERT INTO saved_views (id, workspace_id, visibility, owner_member_id, team_id, name, filters, position, created_at, updated_at)
VALUES (@id, @workspace_id, @visibility, sqlc.narg(owner_member_id), sqlc.narg(team_id), @name, @filters, @position, @now, @now)
RETURNING *;

-- Personal views to their owner, workspace views to everyone, team views to the team's members and,
-- with all_teams, to owners, admins and keys.
-- name: ListSavedViewsFor :many
SELECT v.* FROM saved_views v
WHERE v.workspace_id = @workspace_id
  AND (v.visibility = 'workspace'
       OR (v.visibility = 'personal' AND v.owner_member_id = sqlc.narg(member_id)::uuid)
       OR (v.visibility = 'team' AND (@all_teams::bool OR EXISTS (
           SELECT 1 FROM team_members tm
           WHERE tm.workspace_id = v.workspace_id AND tm.team_id = v.team_id AND tm.member_id = sqlc.narg(member_id)::uuid))))
  AND (sqlc.narg(id)::uuid IS NULL OR v.id = sqlc.narg(id)::uuid)
ORDER BY CASE v.visibility WHEN 'personal' THEN 0 WHEN 'team' THEN 1 ELSE 2 END, v.position, v.name, v.id
LIMIT @lim;

-- name: UpdateSavedView :one
UPDATE saved_views SET name = @name, filters = @filters, position = @position, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteSavedView :execrows
DELETE FROM saved_views WHERE workspace_id = $1 AND id = $2;

-- name: SetSavedViewPositions :execrows
UPDATE saved_views v SET position = (o.ord - 1)::int, updated_at = @now
FROM unnest(@ids::uuid[]) WITH ORDINALITY AS o(id, ord)
WHERE v.workspace_id = @workspace_id AND v.id = o.id;

-- name: StripLabelFromSavedViews :exec
UPDATE saved_views SET filters = filters - 'label_id'
WHERE workspace_id = @workspace_id AND filters->>'label_id' = @label_id::text;

-- name: StripTeamFromSavedViews :exec
UPDATE saved_views SET filters = filters - 'team_id'
WHERE workspace_id = @workspace_id AND filters->>'team_id' = @team_id::text;

-- name: StripFieldFromSavedViews :exec
UPDATE saved_views SET filters = filters - ('field.' || @key::text)
WHERE workspace_id = @workspace_id AND filters ? ('field.' || @key::text);

-- name: ShareLockTeams :many
SELECT id FROM teams WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]) FOR SHARE;
