-- name: CreateContactNote :one
WITH n AS (
    INSERT INTO contact_notes (id, workspace_id, contact_id, author_member_id, author_api_key_id, body, created_at)
    VALUES (@id, @workspace_id, @contact_id, sqlc.narg(author_member_id), sqlc.narg(author_api_key_id), @body, @now)
    RETURNING *)
SELECT n.id, n.workspace_id, n.contact_id, n.author_member_id, n.author_api_key_id, n.body, n.created_at,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url
FROM n LEFT JOIN api_keys ak ON ak.workspace_id = n.workspace_id AND ak.id = n.author_api_key_id;

-- name: ListContactNotes :many
SELECT n.id, n.workspace_id, n.contact_id, n.author_member_id, n.author_api_key_id, n.body, n.created_at,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url
FROM contact_notes n LEFT JOIN api_keys ak ON ak.workspace_id = n.workspace_id AND ak.id = n.author_api_key_id
WHERE n.workspace_id = @workspace_id AND n.contact_id = @contact_id
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (n.created_at, n.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY n.created_at DESC, n.id DESC
LIMIT @lim;

-- name: LockContactNote :one
SELECT * FROM contact_notes WHERE workspace_id = @workspace_id AND contact_id = @contact_id AND id = @id FOR UPDATE;

-- name: DeleteContactNote :exec
DELETE FROM contact_notes WHERE workspace_id = @workspace_id AND id = @id;
