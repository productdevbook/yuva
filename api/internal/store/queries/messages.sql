-- name: CreateMessage :one
INSERT INTO messages (id, workspace_id, conversation_id, kind, direction, author_type, author_member_id,
                      author_contact_id, body, html, client_id, event, created_at)
VALUES (@id, @workspace_id, @conversation_id, @kind, @direction, @author_type, @author_member_id,
        @author_contact_id, @body, @html, @client_id, @event, @created_at)
ON CONFLICT (workspace_id, conversation_id, client_id) DO NOTHING
RETURNING id, workspace_id, conversation_id, kind, direction, author_type, author_member_id,
          author_contact_id, body, html, client_id, event, created_at;

-- name: GetMessageByClientID :one
SELECT id, workspace_id, conversation_id, kind, direction, author_type, author_member_id,
       author_contact_id, body, html, client_id, event, created_at
FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND client_id = $3;

-- name: ListMessages :many
SELECT id, workspace_id, conversation_id, kind, direction, author_type, author_member_id,
       author_contact_id, body, html, client_id, event, created_at
FROM messages m
WHERE m.workspace_id = @workspace_id AND m.conversation_id = @conversation_id
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (m.created_at, m.id) > (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY m.created_at, m.id
LIMIT @lim;

-- name: CreateAttachment :one
INSERT INTO attachments (id, workspace_id, conversation_id, message_id, storage_key, filename,
                         content_type, size_bytes, created_at)
VALUES (@id, @workspace_id, @conversation_id, @message_id, @storage_key, @filename, @content_type,
        @size_bytes, @created_at)
RETURNING *;

-- name: ListAttachments :many
SELECT * FROM attachments
WHERE workspace_id = @workspace_id AND message_id = ANY(@message_ids::uuid[])
ORDER BY message_id, created_at, id;

-- name: GetAttachment :one
SELECT a.*, c.inbox_id FROM attachments a
JOIN conversations c ON c.workspace_id = a.workspace_id AND c.id = a.conversation_id
WHERE a.workspace_id = $1 AND a.id = $2;
