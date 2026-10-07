-- name: CreateMessage :one
INSERT INTO messages (id, workspace_id, conversation_id, kind, direction, author_type, author_member_id,
                      author_contact_id, body, html, client_id, event, created_at, delivery_state, delivery_updated_at)
VALUES (@id, @workspace_id, @conversation_id, @kind, @direction, @author_type, @author_member_id,
        @author_contact_id, @body, @html, @client_id, @event, @created_at, sqlc.narg(delivery_state),
        CASE WHEN sqlc.narg(delivery_state)::text IS NULL THEN NULL ELSE @created_at::timestamptz END)
ON CONFLICT (workspace_id, conversation_id, client_id) DO NOTHING
RETURNING id, workspace_id, conversation_id, kind, direction, author_type, author_member_id,
          author_contact_id, body, html, client_id, event, created_at, delivery_state, delivery_error, delivery_updated_at;

-- name: GetMessageByClientID :one
SELECT id, workspace_id, conversation_id, kind, direction, author_type, author_member_id,
       author_contact_id, body, html, client_id, event, created_at, delivery_state, delivery_error, delivery_updated_at
FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND client_id = $3;

-- name: ListMessages :many
SELECT id, workspace_id, conversation_id, kind, direction, author_type, author_member_id,
       author_contact_id, body, html, client_id, event, created_at, delivery_state, delivery_error, delivery_updated_at
FROM messages m
WHERE m.workspace_id = @workspace_id AND m.conversation_id = @conversation_id
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (m.created_at, m.id) > (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY m.created_at, m.id
LIMIT @lim;

-- name: ListMessagesDesc :many
SELECT id, workspace_id, conversation_id, kind, direction, author_type, author_member_id,
       author_contact_id, body, html, client_id, event, created_at, delivery_state, delivery_error, delivery_updated_at
FROM messages m
WHERE m.workspace_id = @workspace_id AND m.conversation_id = @conversation_id
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (m.created_at, m.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY m.created_at DESC, m.id DESC
LIMIT @lim;

-- name: GetMessagePosition :one
SELECT id, created_at FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND id = $3;

-- name: GetLatestMessagePosition :one
SELECT id, created_at FROM messages WHERE workspace_id = $1 AND conversation_id = $2
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CreateAttachment :one
INSERT INTO attachments (id, workspace_id, conversation_id, message_id, storage_key, filename,
                         content_type, size_bytes, content_id, inline, created_at)
VALUES (@id, @workspace_id, @conversation_id, @message_id, @storage_key, @filename, @content_type,
        @size_bytes, sqlc.narg(content_id), @inline, @created_at)
RETURNING *;

-- name: ListAttachments :many
SELECT * FROM attachments
WHERE workspace_id = @workspace_id AND message_id = ANY(@message_ids::uuid[])
ORDER BY message_id, created_at, id;

-- name: GetAttachment :one
SELECT a.*, c.inbox_id FROM attachments a
JOIN conversations c ON c.workspace_id = a.workspace_id AND c.id = a.conversation_id
WHERE a.workspace_id = $1 AND a.id = $2;

-- name: GetMessage :one
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at
FROM messages m WHERE m.workspace_id = $1 AND m.id = $2;

-- name: SetMessageDelivery :one
UPDATE messages SET delivery_state = @state::text, delivery_error = sqlc.narg(error), delivery_updated_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id
RETURNING id, workspace_id, conversation_id, kind, direction, author_type, author_member_id,
          author_contact_id, body, html, client_id, event, created_at, delivery_state, delivery_error, delivery_updated_at;

-- name: ListAttachmentsOfMessage :many
SELECT * FROM attachments WHERE workspace_id = $1 AND message_id = $2 ORDER BY created_at, id;
