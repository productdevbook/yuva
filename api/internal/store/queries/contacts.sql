-- name: CreateContact :one
INSERT INTO contacts (id, workspace_id, name, attributes, blocked, created_by, created_at, updated_at)
VALUES (@id, @workspace_id, @name, @attributes, @blocked, sqlc.narg(created_by), @now, @now)
RETURNING id, workspace_id, name, attributes, blocked, created_at, updated_at, locale, typed_email;

-- name: GetContact :one
SELECT id, workspace_id, name, attributes, blocked, created_at, updated_at, locale, typed_email
FROM contacts WHERE workspace_id = $1 AND id = $2;

-- name: LockContact :one
SELECT id, workspace_id, name, attributes, blocked, created_at, updated_at, locale, typed_email
FROM contacts WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: UpdateContact :one
UPDATE contacts SET name = @name, attributes = @attributes, blocked = @blocked, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING id, workspace_id, name, attributes, blocked, created_at, updated_at, locale, typed_email;

-- name: DeleteContact :execrows
DELETE FROM contacts WHERE workspace_id = $1 AND id = $2;

-- name: RefreshContactSearch :exec
UPDATE contacts c SET search = to_tsvector('simple', translate(concat_ws(' ',
    c.name,
    (SELECT string_agg(e.email || ' ' || translate(e.email, '@.', '  '), ' ')
     FROM contact_emails e WHERE e.workspace_id = c.workspace_id AND e.contact_id = c.id),
    (SELECT string_agg(x.external_id, ' ')
     FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
), 'İı', 'ii'))
WHERE c.workspace_id = $1 AND c.id = $2;

-- name: ListContacts :many
SELECT id, workspace_id, name, attributes, blocked, created_at, updated_at, locale, typed_email
FROM contacts c
WHERE c.workspace_id = @workspace_id
  AND (sqlc.narg(viewer_id)::uuid IS NULL OR (
      EXISTS (SELECT 1 FROM conversations cv JOIN inbox_viewers iv
              ON iv.workspace_id = cv.workspace_id AND iv.inbox_id = cv.inbox_id AND iv.viewer_id = sqlc.narg(viewer_id)::uuid
              WHERE cv.workspace_id = c.workspace_id AND cv.contact_id = c.id)
   OR EXISTS (SELECT 1 FROM contact_external_ids x JOIN inbox_viewers iv
              ON iv.workspace_id = x.workspace_id AND iv.inbox_id = x.inbox_id AND iv.viewer_id = sqlc.narg(viewer_id)::uuid
              WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
   OR (c.created_by = sqlc.narg(viewer_id)::uuid
       AND NOT EXISTS (SELECT 1 FROM conversations cv WHERE cv.workspace_id = c.workspace_id AND cv.contact_id = c.id)
       AND NOT EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id))))
  AND (NOT EXISTS (SELECT 1 FROM conversations cv JOIN inboxes ci ON ci.workspace_id = cv.workspace_id AND ci.id = cv.inbox_id
                   WHERE cv.workspace_id = c.workspace_id AND cv.contact_id = c.id AND ci.personal)
       OR EXISTS (SELECT 1 FROM conversations cv JOIN inboxes ci ON ci.workspace_id = cv.workspace_id AND ci.id = cv.inbox_id
                  WHERE cv.workspace_id = c.workspace_id AND cv.contact_id = c.id
                    AND (NOT ci.personal OR EXISTS (
                        SELECT 1 FROM inbox_viewers iv
                        WHERE iv.workspace_id = ci.workspace_id AND iv.inbox_id = ci.id AND iv.viewer_id = @personal_viewer_id::uuid)))
       OR EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id))
  AND (sqlc.narg(q)::text IS NULL OR c.search @@ websearch_to_tsquery('simple', translate(sqlc.narg(q)::text, 'İı', 'ii'))
       OR c.search @@ to_tsquery('simple', sqlc.narg(prefix)::text))
  AND (sqlc.narg(kind)::text IS NULL OR (sqlc.narg(kind)::text = 'known') = (
      EXISTS (SELECT 1 FROM contact_emails e WHERE e.workspace_id = c.workspace_id AND e.contact_id = c.id)
      OR EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)))
  AND (sqlc.narg(has_open)::bool IS NULL OR sqlc.narg(has_open)::bool = EXISTS (
      SELECT 1 FROM conversations cv
      WHERE cv.workspace_id = c.workspace_id AND cv.contact_id = c.id AND cv.status = 'open' AND NOT cv.spam AND cv.trashed_at IS NULL
        AND (sqlc.narg(viewer_id)::uuid IS NULL OR EXISTS (
            SELECT 1 FROM inbox_viewers iv
            WHERE iv.workspace_id = cv.workspace_id AND iv.inbox_id = cv.inbox_id AND iv.viewer_id = sqlc.narg(viewer_id)::uuid))))
  AND (sqlc.narg(inbox_id)::uuid IS NULL
       OR EXISTS (SELECT 1 FROM conversations cv WHERE cv.workspace_id = c.workspace_id AND cv.contact_id = c.id AND cv.inbox_id = sqlc.narg(inbox_id)::uuid)
       OR EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id AND x.inbox_id = sqlc.narg(inbox_id)::uuid))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (c.created_at, c.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @lim;

-- name: ListContactsByActivity :many
SELECT id, workspace_id, name, attributes, blocked, created_at, updated_at, locale, typed_email,
       coalesce(c.last_active_at, c.created_at)::timestamptz AS active_at
FROM contacts c
WHERE c.workspace_id = @workspace_id
  AND (sqlc.narg(viewer_id)::uuid IS NULL OR (
      EXISTS (SELECT 1 FROM conversations cv JOIN inbox_viewers iv
              ON iv.workspace_id = cv.workspace_id AND iv.inbox_id = cv.inbox_id AND iv.viewer_id = sqlc.narg(viewer_id)::uuid
              WHERE cv.workspace_id = c.workspace_id AND cv.contact_id = c.id)
   OR EXISTS (SELECT 1 FROM contact_external_ids x JOIN inbox_viewers iv
              ON iv.workspace_id = x.workspace_id AND iv.inbox_id = x.inbox_id AND iv.viewer_id = sqlc.narg(viewer_id)::uuid
              WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)
   OR (c.created_by = sqlc.narg(viewer_id)::uuid
       AND NOT EXISTS (SELECT 1 FROM conversations cv WHERE cv.workspace_id = c.workspace_id AND cv.contact_id = c.id)
       AND NOT EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id))))
  AND (NOT EXISTS (SELECT 1 FROM conversations cv JOIN inboxes ci ON ci.workspace_id = cv.workspace_id AND ci.id = cv.inbox_id
                   WHERE cv.workspace_id = c.workspace_id AND cv.contact_id = c.id AND ci.personal)
       OR EXISTS (SELECT 1 FROM conversations cv JOIN inboxes ci ON ci.workspace_id = cv.workspace_id AND ci.id = cv.inbox_id
                  WHERE cv.workspace_id = c.workspace_id AND cv.contact_id = c.id
                    AND (NOT ci.personal OR EXISTS (
                        SELECT 1 FROM inbox_viewers iv
                        WHERE iv.workspace_id = ci.workspace_id AND iv.inbox_id = ci.id AND iv.viewer_id = @personal_viewer_id::uuid)))
       OR EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id))
  AND (sqlc.narg(q)::text IS NULL OR c.search @@ websearch_to_tsquery('simple', translate(sqlc.narg(q)::text, 'İı', 'ii'))
       OR c.search @@ to_tsquery('simple', sqlc.narg(prefix)::text))
  AND (sqlc.narg(kind)::text IS NULL OR (sqlc.narg(kind)::text = 'known') = (
      EXISTS (SELECT 1 FROM contact_emails e WHERE e.workspace_id = c.workspace_id AND e.contact_id = c.id)
      OR EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id)))
  AND (sqlc.narg(has_open)::bool IS NULL OR sqlc.narg(has_open)::bool = EXISTS (
      SELECT 1 FROM conversations cv
      WHERE cv.workspace_id = c.workspace_id AND cv.contact_id = c.id AND cv.status = 'open' AND NOT cv.spam AND cv.trashed_at IS NULL
        AND (sqlc.narg(viewer_id)::uuid IS NULL OR EXISTS (
            SELECT 1 FROM inbox_viewers iv
            WHERE iv.workspace_id = cv.workspace_id AND iv.inbox_id = cv.inbox_id AND iv.viewer_id = sqlc.narg(viewer_id)::uuid))))
  AND (sqlc.narg(inbox_id)::uuid IS NULL
       OR EXISTS (SELECT 1 FROM conversations cv WHERE cv.workspace_id = c.workspace_id AND cv.contact_id = c.id AND cv.inbox_id = sqlc.narg(inbox_id)::uuid)
       OR EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id AND x.inbox_id = sqlc.narg(inbox_id)::uuid))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (coalesce(c.last_active_at, c.created_at), c.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY coalesce(c.last_active_at, c.created_at) DESC, c.id DESC
LIMIT @lim;

-- name: ListContactInboxes :many
SELECT t.contact_id, i.id AS inbox_id, i.name
FROM (
    SELECT cv.contact_id, cv.inbox_id FROM conversations cv
    WHERE cv.workspace_id = @workspace_id AND cv.contact_id = ANY(@contact_ids::uuid[])
    UNION
    SELECT x.contact_id, x.inbox_id FROM contact_external_ids x
    WHERE x.workspace_id = @workspace_id AND x.contact_id = ANY(@contact_ids::uuid[])
) t
JOIN inboxes i ON i.workspace_id = @workspace_id AND i.id = t.inbox_id
WHERE ((@all_inboxes::bool AND NOT i.personal) OR EXISTS (
    SELECT 1 FROM inbox_viewers iv
    WHERE iv.workspace_id = i.workspace_id AND iv.inbox_id = i.id AND iv.viewer_id = @viewer_id::uuid))
ORDER BY t.contact_id, i.name, i.id;

-- name: ListContactActivity :many
SELECT c.id, c.last_active_at, count(cv.id) AS conversations,
       count(cv.id) FILTER (WHERE cv.status = 'open') AS open_conversations,
       coalesce(max(coalesce(cv.last_message_at, cv.created_at)), c.created_at)::timestamptz AS last_conversation_at
FROM contacts c
LEFT JOIN conversations cv ON cv.workspace_id = c.workspace_id AND cv.contact_id = c.id AND NOT cv.spam AND cv.trashed_at IS NULL
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = cv.workspace_id AND pmb.id = cv.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = cv.workspace_id AND iv.inbox_id = cv.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND conversation_visible_to(cv.draft_of, sqlc.narg(viewer_member_id)::uuid)
WHERE c.workspace_id = @workspace_id AND c.id = ANY(@ids::uuid[])
GROUP BY c.id, c.last_active_at;

-- name: ContactFirstReplies :many
SELECT a.asked_at::timestamptz AS asked_at, min(o.created_at)::timestamptz AS replied_at
FROM (
    SELECT i.conversation_id, min(i.created_at) AS asked_at
    FROM messages i
    JOIN conversations cv ON cv.workspace_id = i.workspace_id AND cv.id = i.conversation_id
    WHERE i.workspace_id = @workspace_id AND cv.contact_id = @contact_id AND NOT cv.spam AND cv.trashed_at IS NULL
      AND i.kind = 'message' AND i.direction = 'in'
      AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = cv.workspace_id AND pmb.id = cv.inbox_id AND pmb.personal)) OR EXISTS (
          SELECT 1 FROM inbox_viewers iv
          WHERE iv.workspace_id = cv.workspace_id AND iv.inbox_id = cv.inbox_id AND iv.viewer_id = @viewer_id::uuid))
    GROUP BY i.conversation_id
) a
JOIN messages o ON o.workspace_id = @workspace_id AND o.conversation_id = a.conversation_id
    AND o.kind = 'message' AND o.direction = 'out' AND NOT o.draft
    AND (o.author_type = 'member' OR o.sent_by_member_id IS NOT NULL)
    AND o.created_at > a.asked_at
GROUP BY a.conversation_id, a.asked_at;

-- name: ContactRatings :many
SELECT cv.rating::text AS rating, count(*) AS n
FROM conversations cv
WHERE cv.workspace_id = @workspace_id AND cv.contact_id = @contact_id AND NOT cv.spam AND cv.kind <> 'feedback' AND cv.rating IS NOT NULL AND cv.trashed_at IS NULL
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = cv.workspace_id AND pmb.id = cv.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = cv.workspace_id AND iv.inbox_id = cv.inbox_id AND iv.viewer_id = @viewer_id::uuid))
GROUP BY 1;

-- name: ListContactEmails :many
SELECT contact_id, email FROM contact_emails
WHERE workspace_id = @workspace_id AND contact_id = ANY(@contact_ids::uuid[])
ORDER BY contact_id, position;

-- name: ListContactExternalIDs :many
SELECT contact_id, inbox_id, external_id FROM contact_external_ids
WHERE workspace_id = @workspace_id AND contact_id = ANY(@contact_ids::uuid[])
ORDER BY contact_id, inbox_id, external_id;

-- name: DeleteContactEmails :exec
DELETE FROM contact_emails WHERE workspace_id = $1 AND contact_id = $2;

-- name: AddContactEmail :exec
INSERT INTO contact_emails (workspace_id, contact_id, email, position) VALUES ($1, $2, $3, $4);

-- name: DeleteContactExternalIDs :exec
DELETE FROM contact_external_ids WHERE workspace_id = $1 AND contact_id = $2;

-- name: AddContactExternalID :exec
INSERT INTO contact_external_ids (workspace_id, inbox_id, external_id, contact_id) VALUES ($1, $2, $3, $4);

-- name: GetContactIDByExternalID :one
SELECT contact_id FROM contact_external_ids WHERE workspace_id = $1 AND inbox_id = $2 AND external_id = $3;

-- name: ListContactStorageKeys :many
-- Files of the contact's conversations and of messages they wrote in anyone else's: both go when
-- the contact does.
SELECT a.storage_key FROM attachments a
JOIN conversations c ON c.workspace_id = a.workspace_id AND c.id = a.conversation_id
JOIN messages m ON m.workspace_id = a.workspace_id AND m.id = a.message_id
WHERE a.workspace_id = $1 AND (c.contact_id = $2 OR m.author_contact_id = $2);

-- name: ListContactSummaries :many
SELECT c.id, c.name,
       coalesce((SELECT e.email FROM contact_emails e
                 WHERE e.workspace_id = c.workspace_id AND e.contact_id = c.id
                 ORDER BY e.position LIMIT 1), '')::text AS email
FROM contacts c
WHERE c.workspace_id = @workspace_id AND c.id = ANY(@ids::uuid[]);

-- name: ContactExists :one
SELECT EXISTS (SELECT 1 FROM contacts WHERE workspace_id = $1 AND id = $2) AS found;

-- name: ContactPersonalOnly :one
SELECT (EXISTS (SELECT 1 FROM conversations cv JOIN inboxes ci ON ci.workspace_id = cv.workspace_id AND ci.id = cv.inbox_id
               WHERE cv.workspace_id = @workspace_id AND cv.contact_id = @contact_id AND ci.personal)
   AND NOT EXISTS (SELECT 1 FROM conversations cv JOIN inboxes ci ON ci.workspace_id = cv.workspace_id AND ci.id = cv.inbox_id
                   WHERE cv.workspace_id = @workspace_id AND cv.contact_id = @contact_id
                     AND (NOT ci.personal OR EXISTS (
                         SELECT 1 FROM inbox_viewers iv
                         WHERE iv.workspace_id = ci.workspace_id AND iv.inbox_id = ci.id AND iv.viewer_id = @viewer_id)))
   AND NOT EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = @workspace_id AND x.contact_id = @contact_id))::bool
    AS hidden;

-- name: ContactVisibleToViewer :one
SELECT coalesce(EXISTS (SELECT 1 FROM conversations cv JOIN inbox_viewers iv
               ON iv.workspace_id = cv.workspace_id AND iv.inbox_id = cv.inbox_id AND iv.viewer_id = @viewer_id
               WHERE cv.workspace_id = @workspace_id AND cv.contact_id = @contact_id)
    OR EXISTS (SELECT 1 FROM contact_external_ids x JOIN inbox_viewers iv
               ON iv.workspace_id = x.workspace_id AND iv.inbox_id = x.inbox_id AND iv.viewer_id = @viewer_id
               WHERE x.workspace_id = @workspace_id AND x.contact_id = @contact_id)
    OR (EXISTS (SELECT 1 FROM contacts ct WHERE ct.workspace_id = @workspace_id AND ct.id = @contact_id AND ct.created_by = @viewer_id)
        AND NOT EXISTS (SELECT 1 FROM conversations cv WHERE cv.workspace_id = @workspace_id AND cv.contact_id = @contact_id)
        AND NOT EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = @workspace_id AND x.contact_id = @contact_id)), false)::bool
    AS visible;

-- name: MoveContactExternalIDs :exec
UPDATE contact_external_ids SET contact_id = @to_contact
WHERE workspace_id = @workspace_id AND contact_id = @from_contact;
