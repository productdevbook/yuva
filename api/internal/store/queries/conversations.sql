-- name: CreateConversation :one
INSERT INTO conversations (id, workspace_id, inbox_id, contact_id, channel_id, subject, priority,
                           assignee_id, last_activity_at, created_at, updated_at)
VALUES (@id, @workspace_id, @inbox_id, @contact_id, @channel_id, @subject, @priority,
        @assignee_id, @now, @now, @now)
RETURNING *;

-- name: GetConversation :one
SELECT * FROM conversations WHERE workspace_id = $1 AND id = $2;

-- name: LockConversation :one
SELECT * FROM conversations WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: UpdateConversation :one
UPDATE conversations SET subject = @subject, status = @status, snooze_until = @snooze_until,
    priority = @priority, assignee_id = @assignee_id, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: TouchConversation :exec
UPDATE conversations SET
    last_activity_at = greatest(last_activity_at, @now::timestamptz),
    last_message_at = CASE WHEN @is_message::bool THEN greatest(coalesce(last_message_at, @now::timestamptz), @now::timestamptz) ELSE last_message_at END,
    updated_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListConversations :many
SELECT c.* FROM conversations c
WHERE c.workspace_id = @workspace_id
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_members im
      WHERE im.workspace_id = c.workspace_id AND im.inbox_id = c.inbox_id AND im.member_id = @member_id))
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR c.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status)::text)
  AND (NOT @unassigned::bool OR c.assignee_id IS NULL)
  AND (sqlc.narg(assignee_id)::uuid IS NULL OR c.assignee_id = sqlc.narg(assignee_id)::uuid)
  AND (sqlc.narg(label_id)::uuid IS NULL OR EXISTS (
      SELECT 1 FROM conversation_labels cl
      WHERE cl.workspace_id = c.workspace_id AND cl.conversation_id = c.id AND cl.label_id = sqlc.narg(label_id)::uuid))
  AND (sqlc.narg(q)::text IS NULL OR (
      to_tsvector('simple', translate(c.subject, 'İı', 'ii')) @@ websearch_to_tsquery('simple', translate(sqlc.narg(q)::text, 'İı', 'ii'))
      OR EXISTS (
          SELECT 1 FROM messages m
          WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id
            AND m.search @@ websearch_to_tsquery('simple', translate(sqlc.narg(q)::text, 'İı', 'ii')))
      OR EXISTS (
          SELECT 1 FROM contacts ct
          WHERE ct.workspace_id = c.workspace_id AND ct.id = c.contact_id
            AND ct.search @@ websearch_to_tsquery('simple', translate(sqlc.narg(q)::text, 'İı', 'ii')))))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (c.last_activity_at, c.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY c.last_activity_at DESC, c.id DESC
LIMIT @lim;

-- name: ListConversationLabels :many
SELECT conversation_id, label_id FROM conversation_labels
WHERE workspace_id = @workspace_id AND conversation_id = ANY(@conversation_ids::uuid[])
ORDER BY conversation_id, label_id;

-- name: AddConversationLabel :exec
INSERT INTO conversation_labels (workspace_id, conversation_id, label_id) VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: RemoveConversationLabel :exec
DELETE FROM conversation_labels WHERE workspace_id = $1 AND conversation_id = $2 AND label_id = $3;
