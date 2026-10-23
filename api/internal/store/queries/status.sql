-- name: CreateStatusPage :one
INSERT INTO status_pages (id, workspace_id, inbox_id, name, description, slug, logo_url, logo_dark_url, accent_color, site_url, support_url,
    locale, timezone, history_days, show_uptime, show_response_times, show_name, show_follow, published, changed_at, created_at, updated_at)
VALUES (@id, @workspace_id, @inbox_id, @name, @description, @slug, sqlc.narg(logo_url), sqlc.narg(logo_dark_url), sqlc.narg(accent_color), sqlc.narg(site_url),
    sqlc.narg(support_url), @locale, @timezone, @history_days, @show_uptime, @show_response_times, @show_name, @show_follow, @published, @now, @now, @now)
RETURNING *;

-- name: GetStatusPage :one
SELECT * FROM status_pages WHERE workspace_id = $1 AND id = $2;

-- name: LockStatusPage :one
SELECT * FROM status_pages WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: GetInboxStatusPage :one
SELECT * FROM status_pages WHERE workspace_id = $1 AND inbox_id = $2;

-- name: StatusSlugTaken :one
-- The documented exception: slugs, current and former, are unique across the server, so availability is checked across workspaces.
SELECT (EXISTS (SELECT 1 FROM status_pages p WHERE p.slug = @slug AND p.id <> @id)
    OR EXISTS (SELECT 1 FROM status_page_slugs f WHERE f.slug = @slug AND f.page_id <> @id))::bool;

-- name: AddStatusFormerSlug :exec
INSERT INTO status_page_slugs (slug, workspace_id, page_id, created_at) VALUES (@slug, @workspace_id, @page_id, @now);

-- name: ReleaseStatusFormerSlug :exec
DELETE FROM status_page_slugs WHERE workspace_id = @workspace_id AND page_id = @page_id AND slug = @slug;

-- name: FindStatusPageByFormerSlug :one
-- The documented exception: a public page is found by a former slug before its workspace is known.
SELECT p.* FROM status_page_slugs s
JOIN status_pages p ON p.workspace_id = s.workspace_id AND p.id = s.page_id
WHERE s.slug = $1;

-- name: FindStatusPageBySlug :one
-- The documented exception: a public page is found by its slug before its workspace is known.
SELECT * FROM status_pages WHERE slug = $1;

-- name: UpdateStatusPage :one
UPDATE status_pages SET name = @name, description = @description, slug = @slug, logo_url = sqlc.narg(logo_url),
    logo_dark_url = sqlc.narg(logo_dark_url),
    accent_color = sqlc.narg(accent_color), site_url = sqlc.narg(site_url), support_url = sqlc.narg(support_url),
    locale = @locale, timezone = @timezone, history_days = @history_days, show_uptime = @show_uptime,
    show_response_times = @show_response_times, show_name = @show_name, show_follow = @show_follow, published = @published,
    changed_at = greatest(changed_at + interval '1 microsecond', @now::timestamptz), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: TouchStatusPage :exec
UPDATE status_pages SET changed_at = greatest(changed_at + interval '1 microsecond', @now::timestamptz)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteStatusPage :execrows
DELETE FROM status_pages WHERE workspace_id = $1 AND id = $2;

-- name: ListStatusPages :many
SELECT * FROM status_pages p
WHERE p.workspace_id = @workspace_id
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR p.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = p.workspace_id AND iv.inbox_id = p.inbox_id AND iv.viewer_id = @viewer_id))
ORDER BY p.created_at, p.id;

-- name: CreateStatusGroup :one
INSERT INTO status_component_groups (id, workspace_id, page_id, name, position, created_at, updated_at)
VALUES (@id, @workspace_id, @page_id, @name, @position, @now, @now)
RETURNING *;

-- name: GetStatusGroup :one
SELECT * FROM status_component_groups WHERE workspace_id = @workspace_id AND page_id = @page_id AND id = @id;

-- name: UpdateStatusGroup :one
UPDATE status_component_groups SET name = @name, position = @position, updated_at = @now
WHERE workspace_id = @workspace_id AND page_id = @page_id AND id = @id
RETURNING *;

-- name: DeleteStatusGroup :execrows
DELETE FROM status_component_groups WHERE workspace_id = @workspace_id AND page_id = @page_id AND id = @id;

-- name: ListStatusGroups :many
SELECT * FROM status_component_groups WHERE workspace_id = @workspace_id AND page_id = @page_id ORDER BY position, name, id;

-- name: CountStatusComponents :one
SELECT count(*)::bigint FROM status_components WHERE workspace_id = @workspace_id AND page_id = @page_id;

-- name: CreateStatusComponent :one
INSERT INTO status_components (id, workspace_id, page_id, group_id, name, description, position, hidden, state, created_at, updated_at)
VALUES (@id, @workspace_id, @page_id, sqlc.narg(group_id), @name, @description, @position, @hidden, @state, @now, @now)
RETURNING *;

-- name: GetStatusComponent :one
SELECT * FROM status_components WHERE workspace_id = @workspace_id AND page_id = @page_id AND id = @id;

-- name: LockStatusComponent :one
SELECT * FROM status_components WHERE workspace_id = @workspace_id AND page_id = @page_id AND id = @id FOR UPDATE;

-- name: UpdateStatusComponent :one
UPDATE status_components SET group_id = sqlc.narg(group_id), name = @name, description = @description, position = @position,
    hidden = @hidden, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: SetStatusComponentState :one
UPDATE status_components SET state = @state, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteStatusComponent :execrows
DELETE FROM status_components WHERE workspace_id = @workspace_id AND page_id = @page_id AND id = @id;

-- name: ListStatusComponents :many
SELECT c.* FROM status_components c
LEFT JOIN status_component_groups g ON g.workspace_id = c.workspace_id AND g.id = c.group_id
WHERE c.workspace_id = @workspace_id AND c.page_id = @page_id
ORDER BY g.position NULLS FIRST, g.name NULLS FIRST, g.id NULLS FIRST, c.position, c.name, c.id;

-- name: ListStatusComponentsByIDs :many
SELECT * FROM status_components WHERE workspace_id = @workspace_id AND page_id = @page_id AND id = ANY(@ids::uuid[]);

-- name: EndStatusComponentState :exec
UPDATE status_component_states SET ended_at = greatest(started_at, @now::timestamptz)
WHERE workspace_id = @workspace_id AND component_id = @component_id AND ended_at IS NULL;

-- name: HandOverStatusComponentState :exec
UPDATE status_component_states SET source = @source, source_id = sqlc.narg(source_id)
WHERE workspace_id = @workspace_id AND component_id = @component_id AND ended_at IS NULL
  AND (source <> @source OR source_id IS DISTINCT FROM sqlc.narg(source_id)::uuid);

-- name: StartStatusComponentState :one
INSERT INTO status_component_states (id, workspace_id, component_id, state, source, source_id, started_at)
VALUES (@id, @workspace_id, @component_id, @state, @source, sqlc.narg(source_id), @now)
RETURNING *;

-- name: OpenStatusComponentStates :many
SELECT * FROM status_component_states
WHERE workspace_id = @workspace_id AND component_id = ANY(@component_ids::uuid[]) AND ended_at IS NULL;

-- name: StatusComponentSpans :many
SELECT * FROM status_component_states
WHERE workspace_id = @workspace_id AND component_id = ANY(@component_ids::uuid[])
  AND (ended_at IS NULL OR ended_at > @since::timestamptz)
ORDER BY component_id, started_at;

-- name: CreateStatusIncident :one
INSERT INTO status_incidents (id, workspace_id, page_id, kind, title, impact, state, scheduled_start, scheduled_end,
    published_at, started_at, created_by, notify, created_at, updated_at)
VALUES (@id, @workspace_id, @page_id, @kind, @title, @impact, @state, sqlc.narg(scheduled_start), sqlc.narg(scheduled_end),
    sqlc.narg(published_at), sqlc.narg(started_at), sqlc.narg(created_by), @notify, @now, @now)
RETURNING *;

-- name: GetStatusIncident :one
SELECT * FROM status_incidents WHERE workspace_id = @workspace_id AND page_id = @page_id AND id = @id;

-- name: LockStatusIncident :one
SELECT * FROM status_incidents WHERE workspace_id = @workspace_id AND page_id = @page_id AND id = @id FOR UPDATE;

-- name: UpdateStatusIncident :one
UPDATE status_incidents SET title = @title, impact = @impact, state = @state, scheduled_start = sqlc.narg(scheduled_start),
    scheduled_end = sqlc.narg(scheduled_end), published_at = sqlc.narg(published_at), started_at = sqlc.narg(started_at),
    resolved_at = sqlc.narg(resolved_at), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteStatusIncident :execrows
DELETE FROM status_incidents WHERE workspace_id = @workspace_id AND page_id = @page_id AND id = @id AND published_at IS NULL;

-- name: ListStatusIncidents :many
SELECT * FROM status_incidents
WHERE workspace_id = @workspace_id AND page_id = @page_id
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind)::text)
  AND (NOT @open_only::bool OR resolved_at IS NULL)
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (created_at, id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: ListPublicStatusIncidents :many
SELECT * FROM status_incidents
WHERE workspace_id = @workspace_id AND page_id = @page_id AND published_at IS NOT NULL
  AND (resolved_at IS NULL OR resolved_at >= @since::timestamptz)
ORDER BY coalesce(scheduled_start, published_at) DESC, id DESC
LIMIT @lim;

-- name: ListStatusIncidentsByIDs :many
SELECT * FROM status_incidents WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);

-- name: ClearStatusIncidentComponents :exec
DELETE FROM status_incident_components WHERE workspace_id = @workspace_id AND incident_id = @incident_id;

-- name: AddStatusIncidentComponent :exec
INSERT INTO status_incident_components (workspace_id, incident_id, component_id, state)
VALUES (@workspace_id, @incident_id, @component_id, @state);

-- name: SetStatusIncidentComponentState :exec
UPDATE status_incident_components SET state = @state
WHERE workspace_id = @workspace_id AND incident_id = @incident_id AND component_id = @component_id;

-- name: ListStatusIncidentComponents :many
SELECT * FROM status_incident_components WHERE workspace_id = @workspace_id AND incident_id = ANY(@incident_ids::uuid[])
ORDER BY incident_id, component_id;

-- name: ActiveStatusHolders :many
SELECT ic.component_id, ic.state, i.id AS incident_id
FROM status_incident_components ic
JOIN status_incidents i ON i.workspace_id = ic.workspace_id AND i.id = ic.incident_id
WHERE ic.workspace_id = @workspace_id AND ic.component_id = ANY(@component_ids::uuid[])
  AND i.published_at IS NOT NULL AND i.state IN ('investigating', 'identified', 'monitoring', 'in_progress')
  AND i.id <> @except_id;

-- name: CreateStatusIncidentUpdate :one
INSERT INTO status_incident_updates (id, workspace_id, incident_id, state, body, author_member_id, author_key_id, notify, created_at)
VALUES (@id, @workspace_id, @incident_id, @state, @body, sqlc.narg(author_member_id), sqlc.narg(author_key_id), @notify, @now)
RETURNING *;

-- name: EditStatusIncidentUpdate :one
UPDATE status_incident_updates SET body = @body, edited_at = sqlc.narg(edited_at)
WHERE workspace_id = @workspace_id AND incident_id = @incident_id AND id = @id
RETURNING *;

-- name: ListStatusIncidentUpdates :many
SELECT * FROM status_incident_updates WHERE workspace_id = @workspace_id AND incident_id = ANY(@incident_ids::uuid[])
ORDER BY created_at DESC, id DESC;

-- name: ClientStatusIncidents :many
SELECT i.id, i.kind, i.title, i.impact, i.state,
    coalesce((SELECT max(u.created_at) FROM status_incident_updates u
              WHERE u.workspace_id = i.workspace_id AND u.incident_id = i.id), i.published_at)::timestamptz AS last_update_at
FROM status_incidents i
WHERE i.workspace_id = @workspace_id AND i.page_id = @page_id AND i.published_at IS NOT NULL
  AND i.state IN ('investigating', 'identified', 'monitoring', 'in_progress')
ORDER BY i.published_at DESC, i.id DESC
LIMIT 20;

-- name: LinkStatusIncidentConversation :exec
INSERT INTO status_incident_conversations (workspace_id, incident_id, conversation_id, linked_by_member_id, linked_by_api_key_id, linked_at)
VALUES (@workspace_id, @incident_id, @conversation_id, sqlc.narg(linked_by_member_id), sqlc.narg(linked_by_api_key_id), @now)
ON CONFLICT DO NOTHING;

-- name: UnlinkStatusIncidentConversation :exec
DELETE FROM status_incident_conversations
WHERE workspace_id = @workspace_id AND incident_id = @incident_id AND conversation_id = @conversation_id;

-- name: ListStatusIncidentConversations :many
SELECT sqlc.embed(c), l.linked_at FROM status_incident_conversations l
JOIN conversations c ON c.workspace_id = l.workspace_id AND c.id = l.conversation_id
WHERE l.workspace_id = @workspace_id AND l.incident_id = @incident_id AND c.trashed_at IS NULL
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (l.linked_at, l.conversation_id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY l.linked_at DESC, l.conversation_id DESC
LIMIT @lim;

-- name: ListStatusReplyTargets :many
SELECT c.id FROM status_incident_conversations l
JOIN conversations c ON c.workspace_id = l.workspace_id AND c.id = l.conversation_id
WHERE l.workspace_id = @workspace_id AND l.incident_id = @incident_id AND c.status <> 'closed' AND NOT c.spam AND c.trashed_at IS NULL
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id))
ORDER BY l.linked_at, c.id;

-- name: ListConversationIncidents :many
SELECT sqlc.embed(i), p.name AS page_name, p.slug AS page_slug, p.custom_domain AS page_domain, p.domain_state AS page_domain_state, p.cf_status AS page_cf_status, p.cf_ssl_status AS page_cf_ssl_status, l.linked_at FROM status_incident_conversations l
JOIN status_incidents i ON i.workspace_id = l.workspace_id AND i.id = l.incident_id
JOIN status_pages p ON p.workspace_id = i.workspace_id AND p.id = i.page_id
WHERE l.workspace_id = @workspace_id AND l.conversation_id = @conversation_id
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = p.workspace_id AND iv.inbox_id = p.inbox_id AND iv.viewer_id = @viewer_id))
ORDER BY l.linked_at DESC, i.id DESC;

-- name: StatusIncidentConversationLinked :one
SELECT EXISTS (SELECT 1 FROM status_incident_conversations
    WHERE workspace_id = @workspace_id AND incident_id = @incident_id AND conversation_id = @conversation_id)::bool;

-- name: CountStatusIncidentConversations :one
SELECT count(*)::int FROM status_incident_conversations WHERE workspace_id = @workspace_id AND incident_id = @incident_id;

-- name: CreateLinkedReply :one
INSERT INTO linked_replies (workspace_id, id, incident_id, idea_id, body, close, member_id, api_key_id, grant_id, via, mcp_only, queued, created_at)
VALUES (@workspace_id, @id, sqlc.narg(incident_id), sqlc.narg(idea_id), @body, @close, sqlc.narg(member_id), sqlc.narg(api_key_id), sqlc.narg(grant_id), @via, @mcp_only, @queued, @created_at)
RETURNING *;

-- name: GetIncidentLinkedReply :one
SELECT * FROM linked_replies WHERE workspace_id = @workspace_id AND incident_id = @incident_id AND id = @id;

-- name: GetIdeaLinkedReply :one
SELECT * FROM linked_replies WHERE workspace_id = @workspace_id AND idea_id = @idea_id AND id = @id;

-- name: GetLinkedReply :one
SELECT * FROM linked_replies WHERE workspace_id = @workspace_id AND id = @id;

-- name: AddLinkedReplyTargets :exec
INSERT INTO linked_reply_targets (workspace_id, reply_id, conversation_id)
SELECT @workspace_id, @reply_id, unnest(@conversation_ids::uuid[]);

-- name: FinishLinkedReplyTarget :exec
UPDATE linked_reply_targets SET outcome = @outcome, reason = sqlc.narg(reason), finished_at = @now
WHERE workspace_id = @workspace_id AND reply_id = @reply_id AND conversation_id = @conversation_id AND outcome = 'pending';

-- name: LinkedReplyTargetOutcome :one
SELECT outcome FROM linked_reply_targets
WHERE workspace_id = @workspace_id AND reply_id = @reply_id AND conversation_id = @conversation_id;

-- name: CountLinkedReplyTargets :many
SELECT t.outcome, count(*)::int AS n FROM linked_reply_targets t
JOIN conversations c ON c.workspace_id = t.workspace_id AND c.id = t.conversation_id
WHERE t.workspace_id = @workspace_id AND t.reply_id = @reply_id
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id))
GROUP BY t.outcome;

-- name: ListLinkedReplyTargetOutcomes :many
SELECT t.conversation_id, t.outcome, t.reason, t.finished_at FROM linked_reply_targets t
JOIN conversations c ON c.workspace_id = t.workspace_id AND c.id = t.conversation_id
WHERE t.workspace_id = @workspace_id AND t.reply_id = @reply_id
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id))
  AND (sqlc.narg(cursor_id)::uuid IS NULL OR t.conversation_id > sqlc.narg(cursor_id)::uuid)
ORDER BY t.conversation_id
LIMIT @lim;
-- name: SetStatusPageEmail :one
UPDATE status_pages SET mailing_list_id = sqlc.narg(mailing_list_id), email_enabled = @email_enabled,
    changed_at = greatest(changed_at + interval '1 microsecond', @now::timestamptz), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: GetStatusPageByList :one
SELECT * FROM status_pages WHERE workspace_id = @workspace_id AND mailing_list_id = @mailing_list_id;

-- name: ListStatusPagesByLists :many
SELECT id, mailing_list_id FROM status_pages WHERE workspace_id = @workspace_id AND mailing_list_id = ANY(@list_ids::uuid[]);

-- name: MailingListNameTaken :one
SELECT EXISTS (SELECT 1 FROM mailing_lists WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id AND name = @name)::bool;

-- name: ClearSubscriptionComponents :exec
DELETE FROM status_subscriber_components WHERE workspace_id = @workspace_id AND subscription_id = @subscription_id;

-- name: SetSubscriptionChoseComponents :exec
UPDATE list_subscriptions SET chose_components = @chose WHERE workspace_id = @workspace_id AND id = @id;

-- name: AddSubscriptionComponent :exec
INSERT INTO status_subscriber_components (workspace_id, subscription_id, component_id)
VALUES (@workspace_id, @subscription_id, @component_id)
ON CONFLICT DO NOTHING;

-- name: ListSubscriptionComponents :many
SELECT subscription_id, component_id FROM status_subscriber_components
WHERE workspace_id = @workspace_id AND subscription_id = ANY(@subscription_ids::uuid[])
ORDER BY subscription_id, component_id;

-- Contacts made by a status page request that nobody confirmed: nothing else refers to them.
-- name: DeleteUnconfirmedContacts :execrows
DELETE FROM contacts c
WHERE c.workspace_id = @workspace_id AND c.id = ANY(@ids::uuid[]) AND c.person_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM contact_emails x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM conversations x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM list_subscriptions x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM contact_notes x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM chat_visitors x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM broadcast_deliveries x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id);
