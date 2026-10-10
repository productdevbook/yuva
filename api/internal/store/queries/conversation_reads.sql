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
        AND (r.last_read_at IS NULL OR (m.created_at, m.id) > (r.last_read_at, r.last_read_message_id)))
UNION
SELECT s.conversation_id FROM conversation_member_states s
WHERE s.workspace_id = @workspace_id AND s.member_id = @member_id AND s.conversation_id = ANY(@conversation_ids::uuid[])
  AND s.marked_unread_at IS NOT NULL;

-- name: MarkConversationUnread :exec
INSERT INTO conversation_member_states (workspace_id, member_id, conversation_id, marked_unread_at)
VALUES (@workspace_id, @member_id, @conversation_id, @now::timestamptz)
ON CONFLICT (workspace_id, member_id, conversation_id) DO UPDATE
SET marked_unread_at = coalesce(conversation_member_states.marked_unread_at, excluded.marked_unread_at);

-- name: ClearMarkedUnread :execrows
UPDATE conversation_member_states SET marked_unread_at = NULL
WHERE workspace_id = @workspace_id AND member_id = @member_id AND conversation_id = @conversation_id
  AND marked_unread_at IS NOT NULL;

-- name: PinConversation :one
INSERT INTO conversation_member_states (workspace_id, member_id, conversation_id, pinned_at)
VALUES (@workspace_id, @member_id, @conversation_id, @now::timestamptz)
ON CONFLICT (workspace_id, member_id, conversation_id) DO UPDATE
SET pinned_at = excluded.pinned_at
WHERE conversation_member_states.pinned_at IS NULL
RETURNING pinned_at::timestamptz;

-- name: GetConversationPin :one
SELECT pinned_at::timestamptz FROM conversation_member_states
WHERE workspace_id = @workspace_id AND member_id = @member_id AND conversation_id = @conversation_id AND pinned_at IS NOT NULL;

-- name: UnpinConversation :execrows
UPDATE conversation_member_states SET pinned_at = NULL
WHERE workspace_id = @workspace_id AND member_id = @member_id AND conversation_id = @conversation_id
  AND pinned_at IS NOT NULL;

-- name: ListPinnedConversations :many
SELECT conversation_id, pinned_at::timestamptz AS pinned_at FROM conversation_member_states
WHERE workspace_id = @workspace_id AND member_id = @member_id AND conversation_id = ANY(@conversation_ids::uuid[])
  AND pinned_at IS NOT NULL;
