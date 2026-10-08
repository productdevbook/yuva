-- name: MarkConversationRead :one
INSERT INTO conversation_reads (workspace_id, member_id, conversation_id, last_read_message_id, last_read_at, updated_at)
VALUES (@workspace_id, @member_id, @conversation_id, @message_id, @message_at, @now)
ON CONFLICT (workspace_id, member_id, conversation_id) DO UPDATE
SET last_read_message_id = excluded.last_read_message_id, last_read_at = excluded.last_read_at, updated_at = excluded.updated_at
WHERE (conversation_reads.last_read_at, conversation_reads.last_read_message_id) < (excluded.last_read_at, excluded.last_read_message_id)
RETURNING *;

-- name: GetConversationRead :one
SELECT * FROM conversation_reads WHERE workspace_id = $1 AND member_id = $2 AND conversation_id = $3;

-- name: ListUnreadConversations :many
SELECT c.id FROM conversations c
LEFT JOIN conversation_reads r
       ON r.workspace_id = c.workspace_id AND r.conversation_id = c.id AND r.member_id = @member_id
WHERE c.workspace_id = @workspace_id AND c.id = ANY(@conversation_ids::uuid[])
  AND EXISTS (
      SELECT 1 FROM messages m
      WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id
        AND m.kind IN ('message', 'note') AND NOT m.draft
        AND m.author_member_id IS DISTINCT FROM @member_id::uuid
        AND (r.last_read_at IS NULL OR (m.created_at, m.id) > (r.last_read_at, r.last_read_message_id)));
