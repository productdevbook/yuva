-- name: CreateContact :one
INSERT INTO contacts (id, workspace_id, name, attributes, blocked, created_at, updated_at)
VALUES (@id, @workspace_id, @name, @attributes, @blocked, @now, @now)
RETURNING id, workspace_id, name, attributes, blocked, created_at, updated_at;

-- name: GetContact :one
SELECT id, workspace_id, name, attributes, blocked, created_at, updated_at
FROM contacts WHERE workspace_id = $1 AND id = $2;

-- name: LockContact :one
SELECT id, workspace_id, name, attributes, blocked, created_at, updated_at
FROM contacts WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: UpdateContact :one
UPDATE contacts SET name = @name, attributes = @attributes, blocked = @blocked, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING id, workspace_id, name, attributes, blocked, created_at, updated_at;

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
SELECT id, workspace_id, name, attributes, blocked, created_at, updated_at
FROM contacts c
WHERE c.workspace_id = @workspace_id
  AND (sqlc.narg(q)::text IS NULL OR c.search @@ websearch_to_tsquery('simple', translate(sqlc.narg(q)::text, 'İı', 'ii')))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (c.created_at, c.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @lim;

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
SELECT a.storage_key FROM attachments a
JOIN conversations c ON c.workspace_id = a.workspace_id AND c.id = a.conversation_id
WHERE a.workspace_id = $1 AND c.contact_id = $2;
