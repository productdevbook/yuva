-- name: CreateMacro :one
INSERT INTO macros (id, workspace_id, owner_member_id, name, shortcut, body, actions, position, created_at, updated_at)
VALUES (@id, @workspace_id, sqlc.narg(owner_member_id), @name, @shortcut, sqlc.narg(body), @actions, @position, @now, @now)
RETURNING *;

-- name: GetMacroFor :one
SELECT * FROM macros
WHERE workspace_id = @workspace_id AND id = @id
  AND (owner_member_id IS NULL OR owner_member_id = sqlc.narg(member_id)::uuid);

-- name: ListMacrosFor :many
SELECT * FROM macros
WHERE workspace_id = @workspace_id AND (owner_member_id IS NULL OR owner_member_id = sqlc.narg(member_id)::uuid)
ORDER BY owner_member_id IS NULL, position, name, id;

-- name: UpdateMacro :one
UPDATE macros SET name = @name, shortcut = @shortcut, body = sqlc.narg(body), actions = @actions, position = @position, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteMacro :execrows
DELETE FROM macros WHERE workspace_id = $1 AND id = $2;

-- name: StripLabelFromMacros :exec
UPDATE macros SET actions = jsonb_set(jsonb_set(actions,
        '{add_labels}', COALESCE(actions->'add_labels', '[]'::jsonb) - @label_id::text),
        '{remove_labels}', COALESCE(actions->'remove_labels', '[]'::jsonb) - @label_id::text)
WHERE workspace_id = @workspace_id
  AND (actions->'add_labels' ? @label_id::text OR actions->'remove_labels' ? @label_id::text);

-- name: ClearMacroAssignee :exec
UPDATE macros SET actions = actions - 'assignee_id'
WHERE workspace_id = @workspace_id AND actions->>'assignee_id' = @member_id::text;

-- name: LockLabels :many
SELECT id FROM labels WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]) FOR SHARE;

-- name: LockMemberShared :many
SELECT id FROM members WHERE workspace_id = @workspace_id AND id = @id FOR SHARE;
