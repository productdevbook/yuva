-- name: CreateInbox :one
INSERT INTO inboxes (id, workspace_id, name, slug, purpose, branding, default_locale, timezone, mode,
                     expected_reply_minutes, business_hours, identity_secret, ask_for_rating, rating_since,
                     assignment_mode, assignment_cap, created_at, updated_at)
VALUES (@id, @workspace_id, @name, @slug, @purpose, @branding, @default_locale, @timezone, @mode,
        @expected_reply_minutes, @business_hours, @identity_secret, coalesce(sqlc.narg(ask_for_rating)::bool, false),
        CASE WHEN sqlc.narg(ask_for_rating)::bool THEN @now::timestamptz END, @assignment_mode, @assignment_cap, @now, @now)
RETURNING *;

-- name: GetInbox :one
SELECT * FROM inboxes WHERE workspace_id = $1 AND id = $2;

-- name: LockInbox :one
SELECT * FROM inboxes WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: ShareInbox :one
SELECT * FROM inboxes WHERE workspace_id = $1 AND id = $2 FOR SHARE;

-- name: ListInboxes :many
SELECT i.* FROM inboxes i
WHERE i.workspace_id = @workspace_id
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = i.workspace_id AND iv.inbox_id = i.id AND iv.viewer_id = @viewer_id))
ORDER BY i.name, i.id;

-- name: UpdateInbox :one
UPDATE inboxes SET name = @name, slug = @slug, purpose = @purpose, branding = @branding, default_locale = @default_locale,
    timezone = @timezone, mode = @mode, expected_reply_minutes = @expected_reply_minutes,
    business_hours = @business_hours, ask_for_rating = @ask_for_rating,
    assignment_mode = @assignment_mode, assignment_cap = @assignment_cap,
    rating_since = CASE WHEN NOT @ask_for_rating::bool THEN NULL WHEN ask_for_rating THEN rating_since ELSE @now::timestamptz END,
    updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: SetInboxSignature :one
UPDATE inboxes SET signature = @signature, signature_image_ids = @image_ids::uuid[], updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: SetInboxIdentitySecret :execrows
UPDATE inboxes SET identity_secret = @identity_secret, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteInbox :execrows
DELETE FROM inboxes WHERE workspace_id = $1 AND id = $2;

-- name: HasInboxAccess :one
SELECT EXISTS (
    SELECT 1 FROM member_inbox_access WHERE workspace_id = $1 AND inbox_id = $2 AND member_id = $3
) AS access;

-- name: GrantInboxAccess :execrows
INSERT INTO inbox_members (workspace_id, inbox_id, member_id, created_at)
VALUES (@workspace_id, @inbox_id, @member_id, @now)
ON CONFLICT DO NOTHING;

-- name: RevokeInboxAccess :execrows
DELETE FROM inbox_members WHERE workspace_id = $1 AND inbox_id = $2 AND member_id = $3;

-- name: ListInboxMembers :many
SELECT m.id, m.workspace_id, m.person_id, m.role, m.title, m.created_at, p.email, p.name,
       bool_or(a.team_id IS NULL)::bool AS direct,
       coalesce(array_agg(DISTINCT a.team_id ORDER BY a.team_id) FILTER (WHERE a.team_id IS NOT NULL), '{}')::uuid[] AS team_ids
FROM member_inbox_access a
JOIN members m ON m.workspace_id = a.workspace_id AND m.id = a.member_id
JOIN people p ON p.id = m.person_id
WHERE a.workspace_id = @workspace_id AND a.inbox_id = @inbox_id
GROUP BY m.id, m.workspace_id, m.person_id, m.role, m.title, m.created_at, p.email, p.name
ORDER BY m.created_at, m.id;

-- name: ListInboxStorageKeys :many
SELECT a.storage_key FROM attachments a
JOIN conversations c ON c.workspace_id = a.workspace_id AND c.id = a.conversation_id
WHERE a.workspace_id = $1 AND c.inbox_id = $2;

-- name: ListViewerInboxIDs :many
SELECT DISTINCT inbox_id FROM inbox_viewers WHERE workspace_id = @workspace_id AND viewer_id = @viewer_id;

-- name: ViewerHasInbox :one
SELECT EXISTS (
    SELECT 1 FROM inbox_viewers WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id AND viewer_id = @viewer_id
) AS access;

-- name: SetInboxFeedbackCategories :one
UPDATE inboxes SET feedback_categories = @feedback_categories, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: MoveFeedbackToOther :execrows
UPDATE conversations SET feedback = feedback || '{"category": "other"}'::jsonb
WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id AND kind = 'feedback'
  AND NOT (feedback->>'category' = ANY(@keys::text[]));

-- name: ShareInboxFeedbackCategories :one
SELECT feedback_categories FROM inboxes WHERE workspace_id = $1 AND id = $2 FOR SHARE;

-- name: ShareInboxPurpose :one
SELECT purpose FROM inboxes WHERE workspace_id = @workspace_id AND id = @id FOR SHARE;

-- name: ListInboxPurposes :many
SELECT id, purpose FROM inboxes WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);

-- name: InboxSupportLinks :one
SELECT
    coalesce((SELECT array_agg(DISTINCT ch.kind ORDER BY ch.kind) FROM channels ch
              WHERE ch.workspace_id = @workspace_id AND ch.inbox_id = @inbox_id AND ch.kind <> 'email'), '{}')::text[] AS channel_kinds,
    coalesce((SELECT array_agg(DISTINCT c.channel_kind ORDER BY c.channel_kind) FROM conversations c
              WHERE c.workspace_id = @workspace_id AND c.inbox_id = @inbox_id AND c.trashed_at IS NULL
                AND c.channel_kind <> 'email'), '{}')::text[] AS conversation_kinds,
    EXISTS (SELECT 1 FROM help_centers h WHERE h.workspace_id = @workspace_id AND h.inbox_id = @inbox_id)::bool AS help_center,
    EXISTS (SELECT 1 FROM status_pages sp WHERE sp.workspace_id = @workspace_id AND sp.inbox_id = @inbox_id)::bool AS status_page,
    EXISTS (SELECT 1 FROM roadmap_boards rb WHERE rb.workspace_id = @workspace_id AND rb.inbox_id = @inbox_id)::bool AS roadmap_board,
    EXISTS (SELECT 1 FROM changelogs cl WHERE cl.workspace_id = @workspace_id AND cl.inbox_id = @inbox_id)::bool AS whats_new,
    EXISTS (SELECT 1 FROM sla_policies p WHERE p.workspace_id = @workspace_id AND @inbox_id::uuid = ANY (p.inbox_ids))::bool AS sla_policy;

-- name: ListInboxSLAConversations :many
SELECT s.conversation_id FROM conversation_sla s
JOIN conversations c ON c.workspace_id = s.workspace_id AND c.id = s.conversation_id
WHERE s.workspace_id = @workspace_id AND c.inbox_id = @inbox_id AND s.dropped_at IS NULL
ORDER BY s.conversation_id;
