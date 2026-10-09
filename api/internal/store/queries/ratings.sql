-- name: RateConversation :one
UPDATE conversations SET rating = @rating::text, rating_comment = sqlc.narg(comment), rated_at = @now::timestamptz, updated_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id AND status = 'closed' AND closed_at = @closed_at
  AND (rated_at IS NULL OR rated_at < closed_at)
RETURNING *;

-- name: ClaimRatingRequest :one
UPDATE conversations SET rating_requested_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id AND status = 'closed' AND closed_at = @closed_at AND NOT spam
  AND (rating_requested_at IS NULL OR rating_requested_at < closed_at)
  AND (rated_at IS NULL OR rated_at < closed_at)
RETURNING *;

-- name: ConversationHasReply :one
SELECT EXISTS (
    SELECT 1 FROM messages
    WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id
      AND kind = 'message' AND direction = 'out' AND NOT draft AND author_type IN ('member', 'bot')
) AS replied;

-- name: StatsRatings :many
SELECT c.inbox_id, c.rating::text AS rating, count(*) AS n
FROM conversations c
WHERE c.workspace_id = @workspace_id AND c.rated_at >= @since::timestamptz AND c.rated_at <= @until::timestamptz AND NOT c.spam
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR c.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
GROUP BY 1, 2
ORDER BY 1, 2;
