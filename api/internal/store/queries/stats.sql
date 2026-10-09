-- name: StatsReplies :many
SELECT m.sent_by_member_id, m.author_member_id, count(*) AS n
FROM messages m
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
WHERE m.workspace_id = @workspace_id AND m.created_at >= @since AND m.created_at <= @until
  AND m.kind = 'message' AND m.direction = 'out' AND NOT m.draft
  AND (m.author_type = 'member' OR m.sent_by_member_id IS NOT NULL)
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR c.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
GROUP BY 1, 2;

-- name: StatsClosed :many
SELECT m.author_member_id, count(*) AS n
FROM messages m
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
WHERE m.workspace_id = @workspace_id AND m.created_at >= @since AND m.created_at <= @until
  AND m.kind = 'event' AND m.event->>'type' = 'status_changed' AND m.event->>'status' = 'closed'
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR c.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
GROUP BY 1;

-- name: StatsFirstReplies :many
SELECT a.asked_at::timestamptz AS asked_at, min(o.created_at)::timestamptz AS replied_at
FROM (
    SELECT i.conversation_id, min(i.created_at) AS asked_at
    FROM messages i
    WHERE i.workspace_id = @workspace_id AND i.kind = 'message' AND i.direction = 'in'
      AND i.conversation_id IN (
          SELECT m.conversation_id
          FROM messages m
          JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
          WHERE m.workspace_id = @workspace_id AND m.created_at >= @since AND m.created_at <= @until
            AND m.kind = 'message' AND m.direction = 'out' AND NOT m.draft
            AND (m.author_type = 'member' OR m.sent_by_member_id IS NOT NULL)
            AND (sqlc.narg(inbox_id)::uuid IS NULL OR c.inbox_id = sqlc.narg(inbox_id)::uuid)
            AND (@all_inboxes::bool OR EXISTS (
                SELECT 1 FROM inbox_viewers iv
                WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid)))
    GROUP BY i.conversation_id
) a
JOIN messages o ON o.workspace_id = @workspace_id AND o.conversation_id = a.conversation_id
    AND o.kind = 'message' AND o.direction = 'out' AND NOT o.draft
    AND (o.author_type = 'member' OR o.sent_by_member_id IS NOT NULL)
    AND o.created_at > a.asked_at
GROUP BY a.conversation_id, a.asked_at
HAVING min(o.created_at) >= @since AND min(o.created_at) <= @until;
