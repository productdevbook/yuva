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
SELECT sqlc.embed(c), sched.at AS next_scheduled_at FROM conversations c
LEFT JOIN LATERAL (
    SELECT sm.scheduled_at AS at FROM messages sm
    WHERE sqlc.narg(scheduled_by)::uuid IS NOT NULL AND sm.workspace_id = c.workspace_id
      AND sm.scheduled_by = sqlc.narg(scheduled_by)::uuid AND sm.conversation_id = c.id AND sm.scheduled_at IS NOT NULL
    ORDER BY sm.scheduled_at LIMIT 1) sched ON true
WHERE c.workspace_id = @workspace_id
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = c.workspace_id AND pmb.id = c.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id))
  AND conversation_visible_to(c.draft_of, sqlc.narg(viewer_member_id)::uuid)
  AND (sqlc.narg(scheduled_by)::uuid IS NULL OR sched.at IS NOT NULL)
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
  AND NOT @op_impossible::bool
  AND (NOT @op_has_hits::bool OR c.id = ANY(@op_hit_ids::uuid[]))
  AND NOT (c.id = ANY(coalesce(@op_miss_ids::uuid[], '{}')))
  AND (coalesce(cardinality(@op_from_vals::text[]), 0) = 0 OR NOT EXISTS (
      SELECT 1 FROM (
          SELECT (@op_from_vals::text[])[i] AS v, (@op_from_neg::bool[])[i] AS neg
          FROM generate_series(1, cardinality(@op_from_vals::text[])) AS i) o
      WHERE o.neg = EXISTS (
          SELECT 1 FROM messages m
          LEFT JOIN message_emails me ON me.workspace_id = m.workspace_id AND me.message_id = m.id
          WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward')
            AND (me.from_address ILIKE o.v
              OR (m.author_contact_id IS NOT NULL AND EXISTS (
                  SELECT 1 FROM contacts ct
                  WHERE ct.workspace_id = m.workspace_id AND ct.id = m.author_contact_id
                    AND (ct.name ILIKE o.v OR EXISTS (
                        SELECT 1 FROM contact_emails ce
                        WHERE ce.workspace_id = ct.workspace_id AND ce.contact_id = ct.id AND ce.email ILIKE o.v))))
              OR (m.author_member_id IS NOT NULL AND EXISTS (
                  SELECT 1 FROM members mb JOIN people pe ON pe.id = mb.person_id
                  WHERE mb.workspace_id = m.workspace_id AND mb.id = m.author_member_id
                    AND (pe.name ILIKE o.v OR pe.email ILIKE o.v)))))))
  AND (coalesce(cardinality(@op_to_vals::text[]), 0) = 0 OR NOT EXISTS (
      SELECT 1 FROM (
          SELECT (@op_to_vals::text[])[i] AS v, (@op_to_neg::bool[])[i] AS neg
          FROM generate_series(1, cardinality(@op_to_vals::text[])) AS i) o
      WHERE o.neg = EXISTS (
          SELECT 1 FROM messages m
          JOIN message_emails me ON me.workspace_id = m.workspace_id AND me.message_id = m.id
          WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward')
            AND address_text(me.to_addresses) ILIKE o.v)))
  AND (coalesce(cardinality(@op_cc_vals::text[]), 0) = 0 OR NOT EXISTS (
      SELECT 1 FROM (
          SELECT (@op_cc_vals::text[])[i] AS v, (@op_cc_neg::bool[])[i] AS neg
          FROM generate_series(1, cardinality(@op_cc_vals::text[])) AS i) o
      WHERE o.neg = EXISTS (
          SELECT 1 FROM messages m
          JOIN message_emails me ON me.workspace_id = m.workspace_id AND me.message_id = m.id
          WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward')
            AND address_text(me.cc_addresses) ILIKE o.v)))
  AND (coalesce(cardinality(@op_subject_vals::text[]), 0) = 0 OR NOT EXISTS (
      SELECT 1 FROM (
          SELECT (@op_subject_vals::text[])[i] AS v, (@op_subject_neg::bool[])[i] AS neg
          FROM generate_series(1, cardinality(@op_subject_vals::text[])) AS i) o
      WHERE o.neg = to_tsvector('simple', translate(c.subject, 'İı', 'ii')) @@ websearch_to_tsquery('simple', translate(o.v, 'İı', 'ii'))))
  AND ((sqlc.narg(op_after)::timestamptz IS NULL AND sqlc.narg(op_before)::timestamptz IS NULL) OR EXISTS (
      SELECT 1 FROM messages m
      WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward')
        AND (sqlc.narg(op_after)::timestamptz IS NULL OR m.created_at >= sqlc.narg(op_after)::timestamptz)
        AND (sqlc.narg(op_before)::timestamptz IS NULL OR m.created_at < sqlc.narg(op_before)::timestamptz)))
  AND (sqlc.narg(op_not_after)::timestamptz IS NULL OR NOT EXISTS (
      SELECT 1 FROM messages m WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward') AND m.created_at >= sqlc.narg(op_not_after)::timestamptz))
  AND (sqlc.narg(op_not_before)::timestamptz IS NULL OR NOT EXISTS (
      SELECT 1 FROM messages m WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward') AND m.created_at < sqlc.narg(op_not_before)::timestamptz))
  AND (sqlc.narg(op_from_me)::bool IS NULL OR sqlc.narg(op_from_me)::bool = EXISTS (
      SELECT 1 FROM messages m
      WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward')
        AND (m.author_member_id = sqlc.narg(viewer_member_id)::uuid OR m.sent_by_member_id = sqlc.narg(viewer_member_id)::uuid OR m.on_behalf_of_member_id = sqlc.narg(viewer_member_id)::uuid)))
  AND (sqlc.narg(op_to_me)::bool IS NULL OR sqlc.narg(op_to_me)::bool = EXISTS (
      SELECT 1 FROM messages m
      JOIN message_emails me ON me.workspace_id = m.workspace_id AND me.message_id = m.id
      WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward')
        AND me.to_addresses && ARRAY(
            SELECT pe.email FROM members mb JOIN people pe ON pe.id = mb.person_id
            WHERE mb.workspace_id = @workspace_id AND mb.id = sqlc.narg(viewer_member_id)::uuid
            UNION
            SELECT ma.address FROM member_addresses ma
            WHERE ma.workspace_id = @workspace_id AND ma.member_id = sqlc.narg(viewer_member_id)::uuid AND ma.verified_at IS NOT NULL)))
  AND (sqlc.narg(op_has_attachment)::bool IS NULL OR sqlc.narg(op_has_attachment)::bool = EXISTS (
      SELECT 1 FROM attachments a JOIN messages m ON m.workspace_id = a.workspace_id AND m.id = a.message_id
      WHERE a.workspace_id = c.workspace_id AND a.conversation_id = c.id AND NOT a.inline
        AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward')))
  AND (sqlc.narg(op_is_starred)::bool IS NULL OR sqlc.narg(op_is_starred)::bool = EXISTS (
      SELECT 1 FROM conversation_stars st
      WHERE st.workspace_id = c.workspace_id AND st.conversation_id = c.id AND st.member_id = sqlc.narg(viewer_member_id)::uuid))
  AND (sqlc.narg(op_in_starred)::bool IS NULL OR sqlc.narg(op_in_starred)::bool = (EXISTS (SELECT 1 FROM inboxes mi WHERE mi.workspace_id = c.workspace_id AND mi.id = c.inbox_id AND mi.purpose = 'mail') AND EXISTS (
      SELECT 1 FROM conversation_stars st
      WHERE st.workspace_id = c.workspace_id AND st.conversation_id = c.id AND st.member_id = sqlc.narg(viewer_member_id)::uuid)))
  AND (sqlc.narg(op_in_inbox)::bool IS NULL OR sqlc.narg(op_in_inbox)::bool = (EXISTS (SELECT 1 FROM inboxes mi WHERE mi.workspace_id = c.workspace_id AND mi.id = c.inbox_id AND mi.purpose = 'mail') AND c.status = 'open' AND c.trashed_at IS NULL))
  AND (sqlc.narg(op_in_snoozed)::bool IS NULL OR sqlc.narg(op_in_snoozed)::bool = (EXISTS (SELECT 1 FROM inboxes mi WHERE mi.workspace_id = c.workspace_id AND mi.id = c.inbox_id AND mi.purpose = 'mail') AND c.status = 'snoozed' AND c.trashed_at IS NULL))
  AND (sqlc.narg(op_in_archive)::bool IS NULL OR sqlc.narg(op_in_archive)::bool = (EXISTS (SELECT 1 FROM inboxes mi WHERE mi.workspace_id = c.workspace_id AND mi.id = c.inbox_id AND mi.purpose = 'mail') AND c.status = 'closed' AND c.trashed_at IS NULL))
  AND (sqlc.narg(op_in_trash)::bool IS NULL OR sqlc.narg(op_in_trash)::bool = (EXISTS (SELECT 1 FROM inboxes mi WHERE mi.workspace_id = c.workspace_id AND mi.id = c.inbox_id AND mi.purpose = 'mail') AND c.trashed_at IS NOT NULL))
  AND (sqlc.narg(op_in_sent)::bool IS NULL OR sqlc.narg(op_in_sent)::bool = (EXISTS (SELECT 1 FROM inboxes mi WHERE mi.workspace_id = c.workspace_id AND mi.id = c.inbox_id AND mi.purpose = 'mail') AND EXISTS (
      SELECT 1 FROM messages m
      WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND m.kind = 'message' AND m.direction = 'out'
        AND NOT m.draft AND (m.author_member_id = sqlc.narg(viewer_member_id)::uuid OR m.sent_by_member_id = sqlc.narg(viewer_member_id)::uuid OR m.on_behalf_of_member_id = sqlc.narg(viewer_member_id)::uuid))))
  AND (sqlc.narg(op_in_drafts)::bool IS NULL OR sqlc.narg(op_in_drafts)::bool = (EXISTS (SELECT 1 FROM inboxes mi WHERE mi.workspace_id = c.workspace_id AND mi.id = c.inbox_id AND mi.purpose = 'mail') AND EXISTS (
      SELECT 1 FROM messages m
      WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND m.draft AND m.scheduled_at IS NULL
        AND m.author_type = 'member' AND m.author_member_id = sqlc.narg(viewer_member_id)::uuid)))
  AND (sqlc.narg(op_in_scheduled)::bool IS NULL OR sqlc.narg(op_in_scheduled)::bool = (EXISTS (SELECT 1 FROM inboxes mi WHERE mi.workspace_id = c.workspace_id AND mi.id = c.inbox_id AND mi.purpose = 'mail') AND EXISTS (
      SELECT 1 FROM messages m
      WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id
        AND m.scheduled_by = sqlc.narg(viewer_member_id)::uuid AND m.scheduled_at IS NOT NULL)))
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
      WHEN sqlc.narg(scheduled_by)::uuid IS NOT NULL THEN sched.at > sqlc.narg(cursor_due)::timestamptz
          OR (sched.at = sqlc.narg(cursor_due)::timestamptz
              AND (c.last_activity_at, c.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
      WHEN NOT @by_sla::bool THEN (c.last_activity_at, c.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
      WHEN sqlc.narg(cursor_due)::timestamptz IS NULL THEN c.sla_due_at IS NULL
          AND (c.last_activity_at, c.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
      ELSE c.sla_due_at IS NULL OR c.sla_due_at > sqlc.narg(cursor_due)::timestamptz
          OR (c.sla_due_at = sqlc.narg(cursor_due)::timestamptz
              AND (c.last_activity_at, c.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid)) END)
ORDER BY CASE WHEN @by_sla::bool THEN c.sla_due_at END ASC NULLS LAST, sched.at ASC NULLS LAST, c.last_activity_at DESC, c.id DESC
LIMIT @lim;

-- name: ListFromMatchConversations :many
SELECT m.conversation_id FROM message_emails me
JOIN messages m ON m.workspace_id = me.workspace_id AND m.id = me.message_id
WHERE me.workspace_id = @workspace_id AND me.from_address ILIKE @pattern::text
  AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward')
UNION
SELECT c.id FROM contacts ct
JOIN conversations c ON c.workspace_id = ct.workspace_id AND c.contact_id = ct.id
WHERE ct.workspace_id = @workspace_id AND ct.name ILIKE @pattern::text
  AND EXISTS (
      SELECT 1 FROM messages m
      WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND m.author_contact_id = ct.id
        AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward'))
UNION
SELECT c.id FROM contact_emails ce
JOIN conversations c ON c.workspace_id = ce.workspace_id AND c.contact_id = ce.contact_id
WHERE ce.workspace_id = @workspace_id AND ce.email ILIKE @pattern::text
  AND EXISTS (
      SELECT 1 FROM messages m
      WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND m.author_contact_id = ce.contact_id
        AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward'))
UNION
SELECT m.conversation_id FROM members mb
JOIN people pe ON pe.id = mb.person_id
JOIN messages m ON m.workspace_id = mb.workspace_id AND m.author_member_id = mb.id
WHERE mb.workspace_id = @workspace_id AND (pe.name ILIKE @pattern::text OR pe.email ILIKE @pattern::text)
  AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward')
LIMIT @max_rows;

-- name: ListToMatchConversations :many
SELECT DISTINCT m.conversation_id FROM message_emails me
JOIN messages m ON m.workspace_id = me.workspace_id AND m.id = me.message_id
WHERE me.workspace_id = @workspace_id AND address_text(me.to_addresses) ILIKE @pattern::text
  AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward')
LIMIT @max_rows;

-- name: ListCcMatchConversations :many
SELECT DISTINCT m.conversation_id FROM message_emails me
JOIN messages m ON m.workspace_id = me.workspace_id AND m.id = me.message_id
WHERE me.workspace_id = @workspace_id AND address_text(me.cc_addresses) ILIKE @pattern::text
  AND NOT m.draft AND m.redacted_at IS NULL AND m.kind IN ('message', 'forward')
LIMIT @max_rows;

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
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = c.workspace_id AND pmb.id = c.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND conversation_visible_to(c.draft_of, sqlc.narg(viewer_member_id)::uuid)
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
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = c.workspace_id AND pmb.id = c.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND conversation_visible_to(c.draft_of, sqlc.narg(viewer_member_id)::uuid)
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
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = c.workspace_id AND pmb.id = c.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND conversation_visible_to(c.draft_of, sqlc.narg(viewer_member_id)::uuid)
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
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = c.workspace_id AND pmb.id = c.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND conversation_visible_to(c.draft_of, sqlc.narg(viewer_member_id)::uuid)
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

-- name: SetComposedConversation :one
UPDATE conversations SET subject = @subject, contact_id = @contact_id, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteDraftContact :execrows
DELETE FROM contacts c
WHERE c.workspace_id = @workspace_id AND c.id = @id AND c.created_at >= @since
  AND c.name = '' AND c.attributes = '{}'::jsonb AND NOT c.blocked AND c.created_by IS NULL
  AND (SELECT count(*) FROM contact_emails x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id) <= 1
  AND NOT EXISTS (SELECT 1 FROM conversations x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM messages x WHERE x.workspace_id = c.workspace_id AND x.author_contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM contact_notes x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM list_subscriptions x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM chat_visitors x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM contact_sessions x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM push_devices x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM broadcast_deliveries x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM email_confirmations x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM discussion_posts x WHERE x.workspace_id = c.workspace_id AND x.author_contact_id = c.id);

-- name: ListDraftConversationIDs :many
SELECT id FROM conversations WHERE workspace_id = @workspace_id AND draft_of = @member_id ORDER BY id;

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

-- name: ListConversationsWithAttachments :many
SELECT c.id FROM conversations c
WHERE c.workspace_id = @workspace_id AND c.id = ANY(@conversation_ids::uuid[])
  AND EXISTS (
      SELECT 1 FROM attachments a
      JOIN messages m ON m.workspace_id = a.workspace_id AND m.id = a.message_id
      WHERE a.workspace_id = c.workspace_id AND a.conversation_id = c.id AND NOT a.inline
        AND NOT m.draft AND m.redacted_at IS NULL AND (@include_notes::bool OR m.kind <> 'note'));

-- name: SetConversationDraftOf :exec
UPDATE conversations SET draft_of = sqlc.narg(draft_of)::uuid WHERE workspace_id = @workspace_id AND id = @id;

-- The trash a caller may delete for good: their own personal mailboxes, and for managers and keys
-- every other inbox they see. bound is the database's clock unless at replaces it in tests.
-- name: CountDeletableTrash :one
WITH b AS (SELECT coalesce(sqlc.narg(at)::timestamptz, statement_timestamp())::timestamptz AS bound)
SELECT b.bound::timestamptz AS bound, (
    SELECT count(*) FROM conversations c
    JOIN inboxes i ON i.workspace_id = c.workspace_id AND i.id = c.inbox_id
    WHERE c.workspace_id = @workspace_id AND c.trashed_at <= b.bound
      AND conversation_visible_to(c.draft_of, sqlc.narg(viewer_member_id)::uuid)
      AND ((i.personal AND i.owner_member_id = sqlc.narg(owner_member_id)::uuid)
        OR (NOT i.personal AND @manager::bool AND (@all_inboxes::bool OR EXISTS (
            SELECT 1 FROM inbox_viewers iv
            WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))))
)::bigint AS n
FROM b;

-- name: ListDeletableTrash :many
SELECT c.id, c.inbox_id, c.contact_id FROM conversations c
JOIN inboxes i ON i.workspace_id = c.workspace_id AND i.id = c.inbox_id
WHERE c.workspace_id = @workspace_id AND c.trashed_at <= @until::timestamptz
  AND conversation_visible_to(c.draft_of, sqlc.narg(viewer_member_id)::uuid)
  AND ((i.personal AND i.owner_member_id = sqlc.narg(owner_member_id)::uuid)
    OR (NOT i.personal AND @manager::bool AND (@all_inboxes::bool OR EXISTS (
        SELECT 1 FROM inbox_viewers iv
        WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))))
ORDER BY c.id
LIMIT @max_rows
FOR UPDATE OF c;
