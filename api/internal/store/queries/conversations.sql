-- name: CreateConversation :one
INSERT INTO conversations (id, workspace_id, inbox_id, contact_id, channel_id, subject, priority,
                           assignee_id, spam, email_token, related_conversation_id, kind, feedback,
                           email_address, page_url, page_title, help_article_id, channel_kind, last_activity_at, created_at, updated_at)
VALUES (@id, @workspace_id, @inbox_id, @contact_id, @channel_id, @subject, @priority,
        @assignee_id, @spam, sqlc.narg(email_token), sqlc.narg(related_conversation_id),
        coalesce(sqlc.narg(kind)::text, 'conversation'), sqlc.narg(feedback), sqlc.narg(email_address),
        sqlc.narg(page_url), sqlc.narg(page_title), sqlc.narg(help_article_id),
        (SELECT ch.kind FROM channels ch WHERE ch.workspace_id = @workspace_id AND ch.id = @channel_id),
        @now, @now, @now)
RETURNING *;

-- name: GetConversation :one
SELECT * FROM conversations WHERE workspace_id = $1 AND id = $2;

-- name: LockConversation :one
SELECT * FROM conversations WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: UpdateConversation :one
UPDATE conversations SET subject = @subject, status = @status, snooze_until = @snooze_until,
    priority = @priority, assignee_id = @assignee_id, spam = @spam, feedback_progress = @feedback_progress, updated_at = @now,
    closed_at = CASE WHEN @status = 'closed' AND status <> 'closed' THEN @now ELSE closed_at END,
    assign_waiting_since = CASE WHEN @status <> 'open' OR @assignee_id::uuid IS NOT NULL OR @spam::bool THEN NULL ELSE assign_waiting_since END,
    assign_after_rules = assign_after_rules AND @status = 'open' AND @assignee_id::uuid IS NULL AND NOT @spam::bool
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
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id))
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR c.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (sqlc.narg(contact_id)::uuid IS NULL OR c.contact_id = sqlc.narg(contact_id)::uuid)
  AND (sqlc.narg(broadcast_id)::uuid IS NULL OR c.broadcast_id = sqlc.narg(broadcast_id)::uuid)
  AND (sqlc.narg(spam)::bool IS NULL OR c.spam = sqlc.narg(spam)::bool)
  AND (c.trashed_at IS NOT NULL) = @trashed::bool
  AND (sqlc.narg(kinds)::text[] IS NULL OR c.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(channel_kinds)::text[] IS NULL OR c.channel_kind = ANY(sqlc.narg(channel_kinds)::text[]))
  AND (sqlc.narg(site)::text IS NULL OR left(c.page_url, length(sqlc.narg(site)::text) + 1) = sqlc.narg(site)::text || '/')
  AND (sqlc.narg(category)::text IS NULL OR c.feedback->>'category' = sqlc.narg(category)::text)
  AND (sqlc.narg(page)::text IS NULL OR c.page_url = sqlc.narg(page)::text)
  AND (sqlc.narg(statuses)::text[] IS NULL OR c.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(idea_id)::uuid IS NULL OR EXISTS (
      SELECT 1 FROM roadmap_idea_conversations ri
      WHERE ri.workspace_id = c.workspace_id AND ri.conversation_id = c.id AND ri.idea_id = sqlc.narg(idea_id)::uuid))
  AND (NOT @no_idea::bool OR (c.kind = 'feedback' AND NOT EXISTS (
      SELECT 1 FROM roadmap_idea_conversations ri
      WHERE ri.workspace_id = c.workspace_id AND ri.conversation_id = c.id)))
  AND (NOT @unassigned::bool OR c.assignee_id IS NULL)
  AND (sqlc.narg(assignee_id)::uuid IS NULL OR c.assignee_id = sqlc.narg(assignee_id)::uuid)
  AND (sqlc.narg(team_id)::uuid IS NULL OR c.team_id = sqlc.narg(team_id)::uuid)
  AND (sqlc.narg(teams_of)::uuid IS NULL OR c.team_id IN (
      SELECT tm.team_id FROM team_members tm
      WHERE tm.workspace_id = c.workspace_id AND tm.member_id = sqlc.narg(teams_of)::uuid))
  AND (sqlc.narg(label_id)::uuid IS NULL OR EXISTS (
      SELECT 1 FROM conversation_labels cl
      WHERE cl.workspace_id = c.workspace_id AND cl.conversation_id = c.id AND cl.label_id = sqlc.narg(label_id)::uuid))
  AND (sqlc.narg(fields)::jsonb IS NULL OR c.fields @> sqlc.narg(fields)::jsonb)
  AND (sqlc.narg(pinned)::bool IS NULL OR sqlc.narg(pinned)::bool = EXISTS (
      SELECT 1 FROM conversation_member_states s
      WHERE s.workspace_id = c.workspace_id AND s.conversation_id = c.id
        AND s.member_id = sqlc.narg(pinned_by)::uuid AND s.pinned_at IS NOT NULL))
  AND (sqlc.narg(purpose)::text IS NULL OR EXISTS (
      SELECT 1 FROM inboxes pi
      WHERE pi.workspace_id = c.workspace_id AND pi.id = c.inbox_id AND pi.purpose = sqlc.narg(purpose)::text))
  AND (sqlc.narg(starred)::bool IS NULL OR sqlc.narg(starred)::bool = EXISTS (
      SELECT 1 FROM conversation_stars st
      WHERE st.workspace_id = c.workspace_id AND st.conversation_id = c.id AND st.member_id = sqlc.narg(starred_by)::uuid))
  AND (sqlc.narg(sent_by)::uuid IS NULL OR EXISTS (
      SELECT 1 FROM messages sm
      WHERE sm.workspace_id = c.workspace_id AND sm.conversation_id = c.id AND sm.kind = 'message' AND sm.direction = 'out'
        AND NOT sm.draft AND (sm.author_member_id = sqlc.narg(sent_by)::uuid OR sm.sent_by_member_id = sqlc.narg(sent_by)::uuid)))
  AND (sqlc.narg(q)::text IS NULL OR (
      to_tsvector('simple', translate(c.subject, 'İı', 'ii')) @@ websearch_to_tsquery('simple', translate(sqlc.narg(q)::text, 'İı', 'ii'))
      OR EXISTS (
          SELECT 1 FROM messages m
          WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND NOT m.draft
            AND m.search @@ websearch_to_tsquery('simple', translate(sqlc.narg(q)::text, 'İı', 'ii')))
      OR EXISTS (
          SELECT 1 FROM contacts ct
          WHERE ct.workspace_id = c.workspace_id AND ct.id = c.contact_id
            AND ct.search @@ websearch_to_tsquery('simple', translate(sqlc.narg(q)::text, 'İı', 'ii')))))
  AND (sqlc.narg(q_not)::text IS NULL OR NOT (
      to_tsvector('simple', translate(c.subject, 'İı', 'ii')) @@ websearch_to_tsquery('simple', translate(sqlc.narg(q_not)::text, 'İı', 'ii'))
      OR EXISTS (
          SELECT 1 FROM messages m
          WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND NOT m.draft
            AND m.search @@ websearch_to_tsquery('simple', translate(sqlc.narg(q_not)::text, 'İı', 'ii')))
      OR EXISTS (
          SELECT 1 FROM contacts ct
          WHERE ct.workspace_id = c.workspace_id AND ct.id = c.contact_id
            AND ct.search @@ websearch_to_tsquery('simple', translate(sqlc.narg(q_not)::text, 'İı', 'ii')))))
  AND (sqlc.narg(sla_states)::text[] IS NULL OR EXISTS (
      SELECT 1 FROM conversation_sla s
      WHERE s.workspace_id = c.workspace_id AND s.conversation_id = c.id AND s.dropped_at IS NULL
        AND (CASE
            WHEN c.sla_due_at IS NOT NULL AND c.sla_due_at <= @sla_now::timestamptz THEN 'breached'
            WHEN c.sla_due_at IS NOT NULL AND s.warn_minutes > 0
                AND c.sla_due_at - s.warn_minutes * interval '1 minute' <= @sla_now::timestamptz THEN 'due_soon'
            WHEN c.sla_due_at IS NOT NULL THEN 'on_track'
            WHEN s.paused_at IS NOT NULL THEN 'paused'
            WHEN s.first_reply_breached_at IS NOT NULL OR s.next_reply_breached_at IS NOT NULL OR s.resolution_breached_at IS NOT NULL THEN 'breached'
            WHEN (s.first_reply_target_s IS NOT NULL AND s.first_reply_met_at IS NOT NULL)
                OR (s.next_reply_target_s IS NOT NULL AND s.next_reply_met_at IS NOT NULL)
                OR (s.resolution_target_s IS NOT NULL AND s.resolution_met_at IS NOT NULL) THEN 'met'
            ELSE 'on_track' END) = ANY (sqlc.narg(sla_states)::text[])))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR CASE
      WHEN NOT @by_sla::bool THEN (c.last_activity_at, c.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
      WHEN sqlc.narg(cursor_due)::timestamptz IS NULL THEN c.sla_due_at IS NULL
          AND (c.last_activity_at, c.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
      ELSE c.sla_due_at IS NULL OR c.sla_due_at > sqlc.narg(cursor_due)::timestamptz
          OR (c.sla_due_at = sqlc.narg(cursor_due)::timestamptz
              AND (c.last_activity_at, c.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid)) END)
ORDER BY CASE WHEN @by_sla::bool THEN c.sla_due_at END ASC NULLS LAST, c.last_activity_at DESC, c.id DESC
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
SELECT c.id AS conversation_id, m.id, m.kind, m.author_type, left(m.body, 1000)::text AS body, m.created_at,
       (m.redacted_at IS NOT NULL)::bool AS redacted
FROM conversations c
CROSS JOIN LATERAL (
    SELECT lm.id, lm.kind, lm.author_type, lm.body, lm.created_at, lm.redacted_at FROM messages lm
    WHERE lm.workspace_id = c.workspace_id AND lm.conversation_id = c.id AND lm.kind = 'message' AND NOT lm.draft
    ORDER BY lm.created_at DESC, lm.id DESC
    LIMIT 1) m
WHERE c.workspace_id = @workspace_id AND c.id = ANY(@conversation_ids::uuid[]);

-- name: CountOpenConversations :many
SELECT c.inbox_id, c.assignee_id, c.team_id, c.spam, count(*) AS n
FROM conversations c
WHERE c.workspace_id = @workspace_id AND c.status = 'open' AND c.trashed_at IS NULL
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND (sqlc.narg(kinds)::text[] IS NULL OR c.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(purpose)::text IS NULL OR EXISTS (
      SELECT 1 FROM inboxes pi
      WHERE pi.workspace_id = c.workspace_id AND pi.id = c.inbox_id AND pi.purpose = sqlc.narg(purpose)::text))
GROUP BY 1, 2, 3, 4;

-- name: CountOpenConversationsByLabel :many
SELECT cl.label_id, count(*) AS n
FROM conversations c
JOIN conversation_labels cl ON cl.workspace_id = c.workspace_id AND cl.conversation_id = c.id
WHERE c.workspace_id = @workspace_id AND c.status = 'open' AND c.trashed_at IS NULL AND NOT c.spam
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND (sqlc.narg(kinds)::text[] IS NULL OR c.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(purpose)::text IS NULL OR EXISTS (
      SELECT 1 FROM inboxes pi
      WHERE pi.workspace_id = c.workspace_id AND pi.id = c.inbox_id AND pi.purpose = sqlc.narg(purpose)::text))
GROUP BY cl.label_id
ORDER BY cl.label_id;

-- name: CountOpenFeedback :many
SELECT (c.feedback->>'category')::text AS category, count(*) AS n
FROM conversations c
WHERE c.workspace_id = @workspace_id AND c.status = 'open' AND c.trashed_at IS NULL AND NOT c.spam AND c.kind = 'feedback'
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND (sqlc.narg(kinds)::text[] IS NULL OR c.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(purpose)::text IS NULL OR EXISTS (
      SELECT 1 FROM inboxes pi
      WHERE pi.workspace_id = c.workspace_id AND pi.id = c.inbox_id AND pi.purpose = sqlc.narg(purpose)::text))
GROUP BY 1
ORDER BY 1;

-- name: CountStarredConversations :one
SELECT count(*) FROM conversations c
JOIN conversation_stars st ON st.workspace_id = c.workspace_id AND st.conversation_id = c.id AND st.member_id = @member_id
WHERE c.workspace_id = @workspace_id AND c.status = 'open' AND c.trashed_at IS NULL AND NOT c.spam
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND (sqlc.narg(kinds)::text[] IS NULL OR c.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(purpose)::text IS NULL OR EXISTS (
      SELECT 1 FROM inboxes pi
      WHERE pi.workspace_id = c.workspace_id AND pi.id = c.inbox_id AND pi.purpose = sqlc.narg(purpose)::text));

-- name: GetFirstPublicMessage :one
SELECT id FROM messages
WHERE workspace_id = $1 AND conversation_id = $2 AND kind = 'message' AND NOT draft
ORDER BY created_at, id
LIMIT 1;

-- name: InboxAPIChannel :one
SELECT * FROM channels WHERE workspace_id = $1 AND inbox_id = $2 AND kind = 'api' ORDER BY created_at, id LIMIT 1;

-- name: ListExpiredConversationIDs :many
SELECT id FROM conversations
WHERE workspace_id = @workspace_id AND status = 'closed' AND trashed_at IS NULL
  AND updated_at < @before AND last_activity_at < @before
ORDER BY updated_at, id
LIMIT @max_rows;

-- name: ListConversationsStorageKeys :many
SELECT storage_key FROM attachments WHERE workspace_id = @workspace_id AND conversation_id = ANY(@ids::uuid[]);

-- name: DeleteConversations :execrows
DELETE FROM conversations WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);

-- name: MoveConversation :one
UPDATE conversations SET inbox_id = @inbox_id, channel_id = sqlc.narg(channel_id), assignee_id = sqlc.narg(assignee_id),
    team_id = sqlc.narg(team_id), updated_at = @now,
    assign_waiting_since = CASE WHEN sqlc.narg(assignee_id)::uuid IS NOT NULL THEN NULL ELSE assign_waiting_since END
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: InboxChannelOfKind :one
SELECT * FROM channels WHERE workspace_id = $1 AND inbox_id = $2 AND kind = $3 ORDER BY created_at, id LIMIT 1;

-- name: SetFeedbackFirstMessage :one
UPDATE conversations c SET feedback = c.feedback || jsonb_build_object(
    'text', @text::text,
    'attachment_count', (SELECT count(*) FROM attachments a WHERE a.workspace_id = c.workspace_id AND a.message_id = @message_id::uuid))
WHERE c.workspace_id = @workspace_id AND c.id = @id AND c.kind = 'feedback'
RETURNING *;

-- The trash's 30 days run on the database's clock (at replaces it in tests), as rate limits do.
-- name: TrashConversation :one
UPDATE conversations SET trashed_at = coalesce(sqlc.narg(at)::timestamptz, clock_timestamp()), trashed_by_member_id = sqlc.narg(member_id), trashed_by_api_key_id = sqlc.narg(api_key_id), updated_at = @now::timestamptz,
    assign_waiting_since = NULL, assign_after_rules = false
WHERE workspace_id = @workspace_id AND id = @id AND trashed_at IS NULL
RETURNING *;

-- name: RestoreConversation :one
UPDATE conversations SET trashed_at = NULL, trashed_by_member_id = NULL, trashed_by_api_key_id = NULL, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id AND trashed_at > coalesce(sqlc.narg(at)::timestamptz, clock_timestamp()) - interval '720 hours'
RETURNING *;

-- Locked, so a restore that commits first keeps its conversation and one that comes later finds it gone.
-- name: ListExpiredTrash :many
SELECT id, inbox_id, contact_id FROM conversations
WHERE workspace_id = @workspace_id AND trashed_at <= coalesce(sqlc.narg(at)::timestamptz, clock_timestamp()) - interval '720 hours'
ORDER BY trashed_at, id
LIMIT @max_rows
FOR UPDATE;

-- name: ListWorkspacesWithExpiredTrash :many
SELECT w.id FROM workspaces w
WHERE w.deleted_at IS NULL
  AND EXISTS (SELECT 1 FROM conversations c WHERE c.workspace_id = w.id AND c.trashed_at <= coalesce(sqlc.narg(at)::timestamptz, clock_timestamp()) - interval '720 hours')
ORDER BY w.id;

-- name: SetConversationTeam :one
UPDATE conversations SET team_id = sqlc.narg(team_id), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: ListContactConversationIDsAll :many
SELECT id FROM conversations WHERE workspace_id = @workspace_id AND contact_id = @contact_id;

-- name: ListInboxConversationIDs :many
SELECT id FROM conversations WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id;

-- name: LockConversationIDs :many
SELECT id FROM conversations WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]) ORDER BY id FOR UPDATE;
