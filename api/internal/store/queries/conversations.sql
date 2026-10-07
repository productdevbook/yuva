-- name: CreateConversation :one
INSERT INTO conversations (id, workspace_id, inbox_id, contact_id, channel_id, subject, priority,
                           assignee_id, spam, email_token, related_conversation_id, kind, feedback,
                           email_address, last_activity_at, created_at, updated_at)
VALUES (@id, @workspace_id, @inbox_id, @contact_id, @channel_id, @subject, @priority,
        @assignee_id, @spam, sqlc.narg(email_token), sqlc.narg(related_conversation_id),
        coalesce(sqlc.narg(kind)::text, 'conversation'), sqlc.narg(feedback), sqlc.narg(email_address), @now, @now, @now)
RETURNING *;

-- name: GetConversation :one
SELECT * FROM conversations WHERE workspace_id = $1 AND id = $2;

-- name: LockConversation :one
SELECT * FROM conversations WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: UpdateConversation :one
UPDATE conversations SET subject = @subject, status = @status, snooze_until = @snooze_until,
    priority = @priority, assignee_id = @assignee_id, spam = @spam, updated_at = @now
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
  AND (sqlc.narg(contact_id)::uuid IS NULL OR c.contact_id = sqlc.narg(contact_id)::uuid)
  AND c.spam = @spam::bool
  AND (sqlc.narg(kind)::text IS NULL OR c.kind = sqlc.narg(kind)::text)
  AND (sqlc.narg(category)::text IS NULL OR c.feedback->>'category' = sqlc.narg(category)::text)
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
  AND (sqlc.narg(q_not)::text IS NULL OR NOT (
      to_tsvector('simple', translate(c.subject, 'İı', 'ii')) @@ websearch_to_tsquery('simple', translate(sqlc.narg(q_not)::text, 'İı', 'ii'))
      OR EXISTS (
          SELECT 1 FROM messages m
          WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id
            AND m.search @@ websearch_to_tsquery('simple', translate(sqlc.narg(q_not)::text, 'İı', 'ii')))
      OR EXISTS (
          SELECT 1 FROM contacts ct
          WHERE ct.workspace_id = c.workspace_id AND ct.id = c.contact_id
            AND ct.search @@ websearch_to_tsquery('simple', translate(sqlc.narg(q_not)::text, 'İı', 'ii')))))
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

-- name: ListConversationPreviews :many
SELECT c.id AS conversation_id, m.id, m.kind, m.author_type, left(m.body, 1000)::text AS body, m.created_at
FROM conversations c
CROSS JOIN LATERAL (
    SELECT lm.id, lm.kind, lm.author_type, lm.body, lm.created_at FROM messages lm
    WHERE lm.workspace_id = c.workspace_id AND lm.conversation_id = c.id AND lm.kind = 'message'
    ORDER BY lm.created_at DESC, lm.id DESC
    LIMIT 1) m
WHERE c.workspace_id = @workspace_id AND c.id = ANY(@conversation_ids::uuid[]);

-- name: CountOpenConversations :many
SELECT c.inbox_id, coalesce(c.assignee_id = sqlc.narg(member_id)::uuid, false)::bool AS mine,
       (c.assignee_id IS NULL)::bool AS unassigned, c.spam, count(*) AS n
FROM conversations c
WHERE c.workspace_id = @workspace_id AND c.status = 'open'
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_members im
      WHERE im.workspace_id = c.workspace_id AND im.inbox_id = c.inbox_id AND im.member_id = sqlc.narg(member_id)::uuid))
GROUP BY 1, 2, 3, 4;

-- name: CountOpenConversationsByLabel :many
SELECT cl.label_id, count(*) AS n
FROM conversations c
JOIN conversation_labels cl ON cl.workspace_id = c.workspace_id AND cl.conversation_id = c.id
WHERE c.workspace_id = @workspace_id AND c.status = 'open' AND NOT c.spam
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_members im
      WHERE im.workspace_id = c.workspace_id AND im.inbox_id = c.inbox_id AND im.member_id = sqlc.narg(member_id)::uuid))
GROUP BY cl.label_id
ORDER BY cl.label_id;

-- name: CountOpenFeedback :many
SELECT (c.feedback->>'category')::text AS category, count(*) AS n
FROM conversations c
WHERE c.workspace_id = @workspace_id AND c.status = 'open' AND NOT c.spam AND c.kind = 'feedback'
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_members im
      WHERE im.workspace_id = c.workspace_id AND im.inbox_id = c.inbox_id AND im.member_id = sqlc.narg(member_id)::uuid))
GROUP BY 1
ORDER BY 1;

-- name: GetFirstPublicMessage :one
SELECT id FROM messages
WHERE workspace_id = $1 AND conversation_id = $2 AND kind = 'message'
ORDER BY created_at, id
LIMIT 1;

-- name: InboxAPIChannel :one
SELECT * FROM channels WHERE workspace_id = $1 AND inbox_id = $2 AND kind = 'api' ORDER BY created_at, id LIMIT 1;
