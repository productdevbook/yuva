-- name: HomeInbox :many
WITH open AS (
    SELECT c.id, c.inbox_id, c.subject, c.contact_id, c.assignee_id, c.channel_kind
    FROM conversations c
    WHERE c.workspace_id = @workspace_id AND c.status = 'open' AND NOT c.spam AND c.trashed_at IS NULL
      AND (sqlc.narg(inbox_id)::uuid IS NULL OR c.inbox_id = sqlc.narg(inbox_id)::uuid)
      AND NOT EXISTS (SELECT 1 FROM inboxes mi WHERE mi.workspace_id = c.workspace_id AND mi.id = c.inbox_id AND mi.purpose = 'mail')
      AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = c.workspace_id AND pmb.id = c.inbox_id AND pmb.personal)) OR EXISTS (
          SELECT 1 FROM inbox_viewers iv
          WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
      AND conversation_visible_to(c.draft_of, sqlc.narg(viewer_member_id)::uuid)
), waiting AS (
    SELECT o.id, o.inbox_id, o.subject, o.contact_id, o.channel_kind, w.since
    FROM open o
    CROSS JOIN LATERAL (
        SELECT min(m.created_at)::timestamptz AS since FROM messages m
        WHERE m.workspace_id = @workspace_id AND m.conversation_id = o.id AND m.kind = 'message' AND m.direction = 'in'
          AND m.created_at > coalesce((
              SELECT max(r.created_at) FROM messages r
              WHERE r.workspace_id = @workspace_id AND r.conversation_id = o.id AND r.kind = 'message' AND r.direction = 'out'
                AND NOT r.draft AND r.author_type IN ('member', 'bot')), '-infinity'::timestamptz)
    ) w
    WHERE w.since IS NOT NULL
), oldest AS (
    SELECT * FROM waiting ORDER BY since, id LIMIT @waiting_limit
)
SELECT n.open, n.unassigned, n.waiting, o.id AS oldest_id, o.inbox_id AS oldest_inbox_id, o.subject AS oldest_subject,
    o.channel_kind AS oldest_channel_kind, o.since AS oldest_since, coalesce(pv.body, '')::text AS oldest_preview,
    ct.id AS oldest_contact_id, ct.name AS oldest_contact_name, ce.email AS oldest_contact_email
FROM (SELECT (SELECT count(*) FROM open)::bigint AS open,
             (SELECT count(*) FROM open WHERE assignee_id IS NULL)::bigint AS unassigned,
             (SELECT count(*) FROM waiting)::bigint AS waiting) n
LEFT JOIN oldest o ON true
LEFT JOIN LATERAL (
    SELECT left(m.body, 2000)::text AS body FROM messages m
    WHERE m.workspace_id = @workspace_id AND m.conversation_id = o.id AND m.kind = 'message' AND m.direction = 'in' AND NOT m.draft
    ORDER BY m.created_at DESC, m.id DESC
    LIMIT 1
) pv ON true
LEFT JOIN contacts ct ON ct.workspace_id = @workspace_id AND ct.id = o.contact_id
LEFT JOIN contact_emails ce ON ce.workspace_id = @workspace_id AND ce.contact_id = o.contact_id
    AND ce.position = (SELECT min(e.position) FROM contact_emails e WHERE e.workspace_id = @workspace_id AND e.contact_id = o.contact_id)
ORDER BY o.since, o.id;

-- name: HomeOpenLoad :many
SELECT i.id AS inbox_id, c.assignee_id, coalesce(c.kind = 'feedback', false)::bool AS feedback, count(c.id) AS n
FROM inboxes i
LEFT JOIN conversations c ON c.workspace_id = i.workspace_id AND c.inbox_id = i.id AND c.status = 'open' AND NOT c.spam AND c.trashed_at IS NULL
    AND conversation_visible_to(c.draft_of, sqlc.narg(viewer_member_id)::uuid)
WHERE i.workspace_id = @workspace_id AND i.purpose = 'support'
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR i.id = sqlc.narg(inbox_id)::uuid)
  AND ((@all_inboxes::bool AND NOT i.personal) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = i.workspace_id AND iv.inbox_id = i.id AND iv.viewer_id = @viewer_id::uuid))
GROUP BY i.id, c.assignee_id, 3
ORDER BY i.name, i.id;

-- name: HomeConversationsByDay :many
SELECT c.inbox_id, (c.created_at AT TIME ZONE @timezone::text)::date AS day, count(*) AS n
FROM conversations c
WHERE c.workspace_id = @workspace_id AND c.last_activity_at >= @since::timestamptz
  AND c.created_at >= @since::timestamptz AND c.created_at <= @until::timestamptz AND NOT c.spam AND c.trashed_at IS NULL
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR c.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND NOT EXISTS (SELECT 1 FROM inboxes mi WHERE mi.workspace_id = c.workspace_id AND mi.id = c.inbox_id AND mi.purpose = 'mail')
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = c.workspace_id AND pmb.id = c.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND conversation_visible_to(c.draft_of, sqlc.narg(viewer_member_id)::uuid)
GROUP BY 1, 2;

-- name: HomeRatingsByDay :many
SELECT c.inbox_id, (c.rated_at AT TIME ZONE @timezone::text)::date AS day, c.rating::text AS rating, count(*) AS n
FROM conversations c
WHERE c.workspace_id = @workspace_id AND c.rated_at >= @since::timestamptz AND c.rated_at <= @until::timestamptz
  AND NOT c.spam AND c.kind <> 'feedback' AND c.trashed_at IS NULL
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR c.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND NOT EXISTS (SELECT 1 FROM inboxes mi WHERE mi.workspace_id = c.workspace_id AND mi.id = c.inbox_id AND mi.purpose = 'mail')
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = c.workspace_id AND pmb.id = c.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id::uuid))
GROUP BY 1, 2, 3;

-- name: HomeStatusComponents :many
SELECT p.id AS page_id, p.inbox_id, p.name AS page_name, p.timezone, p.published,
    c.id AS component_id, c.name AS component_name, c.state AS component_state
FROM status_pages p
LEFT JOIN status_components c ON c.workspace_id = p.workspace_id AND c.page_id = p.id AND NOT c.hidden
LEFT JOIN status_component_groups g ON g.workspace_id = c.workspace_id AND g.id = c.group_id
WHERE p.workspace_id = @workspace_id
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR p.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = p.workspace_id AND pmb.id = p.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = p.workspace_id AND iv.inbox_id = p.inbox_id AND iv.viewer_id = @viewer_id::uuid))
ORDER BY p.created_at, p.id, g.position NULLS FIRST, g.name NULLS FIRST, g.id NULLS FIRST, c.position, c.name, c.id;

-- name: HomeIdeas :many
SELECT d.id, d.inbox_id, d.title, d.vote_count, d.stage::text AS stage
FROM discussions d
WHERE d.workspace_id = @workspace_id AND d.kind = 'idea' AND d.state = 'published' AND d.stage NOT IN ('shipped', 'closed')
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR d.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = d.workspace_id AND pmb.id = d.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = d.workspace_id AND iv.inbox_id = d.inbox_id AND iv.viewer_id = @viewer_id::uuid))
ORDER BY d.vote_count DESC, d.last_activity_at DESC, d.id DESC
LIMIT @lim;

-- name: HomeUpcoming :many
WITH visible AS (
    SELECT i.id FROM inboxes i
    WHERE i.workspace_id = @workspace_id
      AND (sqlc.narg(inbox_id)::uuid IS NULL OR i.id = sqlc.narg(inbox_id)::uuid)
      AND ((@all_inboxes::bool AND NOT i.personal) OR EXISTS (
          SELECT 1 FROM inbox_viewers iv
          WHERE iv.workspace_id = i.workspace_id AND iv.inbox_id = i.id AND iv.viewer_id = @viewer_id::uuid))
)
SELECT 'broadcast'::text AS kind, b.id, b.inbox_id, b.scheduled_at::timestamptz AS at, b.name AS title, NULL::uuid AS parent_id,
    ''::text AS locale, coalesce((b.counts->>'recipients')::bigint, 0)::bigint AS recipients
FROM broadcasts b
WHERE @with_broadcasts::bool AND b.workspace_id = @workspace_id AND b.state = 'scheduled' AND b.scheduled_at IS NOT NULL
  AND b.inbox_id IN (SELECT id FROM visible)
UNION ALL
SELECT si.kind, si.id, p.inbox_id,
    CASE WHEN si.kind = 'maintenance' THEN si.scheduled_start ELSE coalesce(si.started_at, si.published_at) END,
    si.title, p.id, '', 0
FROM status_incidents si
JOIN status_pages p ON p.workspace_id = si.workspace_id AND p.id = si.page_id
WHERE si.workspace_id = @workspace_id AND si.published_at IS NOT NULL AND si.resolved_at IS NULL
  AND si.state NOT IN ('resolved', 'completed')
  AND p.inbox_id IN (SELECT id FROM visible)
UNION ALL
SELECT 'whats_new', e.id, cl.inbox_id, e.publish_at, coalesce(t.title, ''), cl.id, cl.default_locale, 0
FROM changelog_entries e
JOIN changelogs cl ON cl.workspace_id = e.workspace_id AND cl.id = e.changelog_id
LEFT JOIN LATERAL (
    SELECT tr.title FROM changelog_entry_translations tr
    WHERE tr.workspace_id = e.workspace_id AND tr.entry_id = e.id
    ORDER BY tr.locale = cl.default_locale DESC, tr.locale
    LIMIT 1
) t ON true
WHERE @with_whats_new::bool AND e.workspace_id = @workspace_id AND e.state = 'scheduled'
  AND cl.inbox_id IN (SELECT id FROM visible)
ORDER BY 4, 2
LIMIT @lim;

-- name: HomeLastBroadcast :one
SELECT * FROM broadcasts b
WHERE b.workspace_id = @workspace_id AND b.state = 'sent'
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR b.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = b.workspace_id AND pmb.id = b.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = b.workspace_id AND iv.inbox_id = b.inbox_id AND iv.viewer_id = @viewer_id::uuid))
ORDER BY b.finished_at DESC NULLS LAST, b.id DESC
LIMIT 1;

-- name: ListSetupParts :many
WITH visible AS (
    SELECT i.id, i.name, i.created_at, i.purpose FROM inboxes i
    WHERE i.workspace_id = @workspace_id
      AND ((@all_inboxes::bool AND NOT i.personal) OR EXISTS (
          SELECT 1 FROM inbox_viewers iv
          WHERE iv.workspace_id = i.workspace_id AND iv.inbox_id = i.id AND iv.viewer_id = @viewer_id::uuid))
)
SELECT 'inbox'::text AS part, v.id, NULL::uuid AS inbox_id, v.name, ''::text AS kind, v.created_at,
    '{}'::text[] AS domains, '{}'::text[] AS platforms, false AS docs,
    row_number() OVER (ORDER BY v.name, v.id) AS pos, v.purpose
FROM visible v
WHERE @with_inboxes::bool
UNION ALL
SELECT 'channel', ch.id, ch.inbox_id, ch.name, ch.kind, ch.created_at, '{}', '{}',
    ch.kind = 'chat' AND (
        EXISTS (SELECT 1 FROM page_ratings r WHERE r.workspace_id = ch.workspace_id AND r.channel_id = ch.id)
        OR EXISTS (SELECT 1 FROM conversations c WHERE c.workspace_id = ch.workspace_id AND c.channel_id = ch.id AND c.page_url IS NOT NULL)
        OR EXISTS (SELECT 1 FROM discussions d WHERE d.workspace_id = ch.workspace_id AND d.inbox_id = ch.inbox_id AND d.channel_id = ch.id AND d.kind = 'page')),
    row_number() OVER (ORDER BY ch.created_at, ch.id), ''
FROM channels ch
WHERE @with_inboxes::bool AND @with_channels::bool AND ch.workspace_id = @workspace_id AND ch.inbox_id IN (SELECT id FROM visible)
UNION ALL
SELECT 'property', p.id, p.inbox_id, p.name, '', p.created_at, p.domains, p.platforms, false,
    row_number() OVER (ORDER BY p.created_at, p.id), ''
FROM analytics_properties p
WHERE @with_properties::bool AND p.workspace_id = @workspace_id AND p.deleted_at IS NULL
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = p.workspace_id AND pmb.id = p.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = p.workspace_id AND iv.inbox_id = p.inbox_id AND iv.viewer_id = @viewer_id::uuid))
UNION ALL
SELECT 'list', l.id, l.inbox_id, l.name, '', l.created_at, '{}', '{}', false,
    row_number() OVER (ORDER BY l.created_at DESC, l.id DESC), ''
FROM mailing_lists l
WHERE @with_lists::bool AND l.workspace_id = @workspace_id AND l.archived_at IS NULL AND l.kind = 'list'
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = l.workspace_id AND pmb.id = l.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = l.workspace_id AND iv.inbox_id = l.inbox_id AND iv.viewer_id = @viewer_id::uuid))
ORDER BY 1, 10;
