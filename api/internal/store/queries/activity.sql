-- name: ListActivity :many
WITH visible AS (
    SELECT i.id, i.purpose, i.personal, i.owner_member_id FROM inboxes i
    WHERE i.workspace_id = @workspace_id
      AND (sqlc.narg(inbox_id)::uuid IS NULL OR i.id = sqlc.narg(inbox_id)::uuid)
      AND ((@all_inboxes::bool AND NOT i.personal) OR EXISTS (
          SELECT 1 FROM inbox_viewers iv
          WHERE iv.workspace_id = i.workspace_id AND iv.inbox_id = i.id AND iv.viewer_id = @viewer_id::uuid))
), items AS (
    (SELECT CASE WHEN m.event->>'type' = 'rated' THEN 'conversation.rated' ELSE 'conversation.closed' END::text AS kind,
        m.id, m.created_at AS at, c.inbox_id,
        CASE WHEN m.event->>'type' = 'rated' THEN NULL ELSE m.author_member_id END::uuid AS actor_id,
        'conversation'::text AS target_type, c.id AS target_id, coalesce(c.subject, '')::text AS title, NULL::uuid AS parent_id,
        coalesce(m.event->>'rating', '')::text AS rating, ''::text AS state, ''::text AS locale
    FROM messages m
    JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
    WHERE m.workspace_id = @workspace_id AND m.kind = 'event' AND m.imported_at IS NULL
      AND ((m.event->>'type' = 'status_changed' AND m.event->>'status' = 'closed')
           OR (m.event->>'type' = 'rated' AND c.kind <> 'feedback'))
      AND c.inbox_id IN (SELECT v.id FROM visible v WHERE v.purpose <> 'mail')
      AND NOT c.spam AND c.trashed_at IS NULL
      AND conversation_visible_to(c.draft_of, sqlc.narg(viewer_member_id)::uuid)
      AND (sqlc.narg(cursor_at)::timestamptz IS NULL
           OR (m.created_at, m.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
    ORDER BY m.created_at DESC, m.id DESC
    LIMIT @lim)
    UNION ALL
    (SELECT 'incident.updated', u.id, u.created_at, p.inbox_id, u.author_member_id,
        'incident', si.id, si.title, p.id, '', u.state, ''
    FROM status_incident_updates u
    JOIN status_incidents si ON si.workspace_id = u.workspace_id AND si.id = u.incident_id
    JOIN status_pages p ON p.workspace_id = si.workspace_id AND p.id = si.page_id
    WHERE u.workspace_id = @workspace_id AND si.published_at IS NOT NULL
      AND p.inbox_id IN (SELECT v.id FROM visible v)
      AND (sqlc.narg(cursor_at)::timestamptz IS NULL
           OR (u.created_at, u.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
    ORDER BY u.created_at DESC, u.id DESC
    LIMIT @lim)
    UNION ALL
    (SELECT 'article.published', r.id, r.created_at, hc.inbox_id, r.author_member_id,
        'article', a.id, r.title, hc.id, '', '', r.locale
    FROM help_article_revisions r
    JOIN help_articles a ON a.workspace_id = r.workspace_id AND a.id = r.article_id
    JOIN help_centers hc ON hc.workspace_id = a.workspace_id AND hc.id = a.center_id
    WHERE @with_help::bool AND r.workspace_id = @workspace_id AND r.kind = 'publish'
      AND hc.inbox_id IN (SELECT v.id FROM visible v)
      AND (sqlc.narg(cursor_at)::timestamptz IS NULL
           OR (r.created_at, r.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
    ORDER BY r.created_at DESC, r.id DESC
    LIMIT @lim)
    UNION ALL
    (SELECT 'broadcast.sent', b.id, b.finished_at, b.inbox_id, b.scheduled_by,
        'broadcast', b.id, b.name, NULL, '', '', ''
    FROM broadcasts b
    WHERE @with_broadcasts::bool AND b.workspace_id = @workspace_id AND b.state = 'sent' AND b.finished_at IS NOT NULL
      AND b.inbox_id IN (SELECT v.id FROM visible v WHERE NOT v.personal OR v.owner_member_id = @viewer_id::uuid)
      AND (sqlc.narg(cursor_at)::timestamptz IS NULL
           OR (b.finished_at, b.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
    ORDER BY b.finished_at DESC, b.id DESC
    LIMIT @lim)
)
SELECT it.kind, it.id, it.at::timestamptz AS at, it.inbox_id, it.target_type, it.target_id, it.title, it.parent_id,
    it.rating, it.state, it.locale, mb.id AS actor_member_id, pe.name AS actor_name
FROM items it
LEFT JOIN members mb ON mb.workspace_id = @workspace_id AND mb.id = it.actor_id
LEFT JOIN people pe ON pe.id = mb.person_id
ORDER BY it.at DESC, it.id DESC
LIMIT @lim;
