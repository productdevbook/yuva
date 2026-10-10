-- name: CreateMessage :one
WITH m AS (
    INSERT INTO messages (id, workspace_id, conversation_id, kind, direction, author_type, author_member_id,
                          author_contact_id, author_api_key_id, body, html, client_id, event, created_at, draft,
                          delivery_state, delivery_updated_at, via, mentions)
    VALUES (@id, @workspace_id, @conversation_id, @kind, @direction, @author_type, @author_member_id,
            @author_contact_id, sqlc.narg(author_api_key_id), @body, @html, @client_id, @event, @created_at, @draft,
            sqlc.narg(delivery_state),
            CASE WHEN sqlc.narg(delivery_state)::text IS NULL THEN NULL ELSE @created_at::timestamptz END,
            sqlc.narg(via), coalesce(sqlc.narg(mentions)::uuid[], '{}'))
    ON CONFLICT (workspace_id, conversation_id, client_id) DO NOTHING
    RETURNING *),
seen AS (
    UPDATE contacts SET last_active_at = greatest(coalesce(contacts.last_active_at, m.created_at), m.created_at)
    FROM m WHERE contacts.workspace_id = m.workspace_id AND contacts.id = m.author_contact_id)
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM m
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id;

-- name: GetMessageByClientID :one
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM messages m
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id
WHERE m.workspace_id = $1 AND m.conversation_id = $2 AND m.client_id = $3;

-- name: ListMessages :many
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM messages m
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id
WHERE m.workspace_id = @workspace_id AND m.conversation_id = @conversation_id
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (m.created_at, m.id) > (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY m.created_at, m.id
LIMIT @lim;

-- name: ListMessagesDesc :many
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM messages m
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id
WHERE m.workspace_id = @workspace_id AND m.conversation_id = @conversation_id
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (m.created_at, m.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY m.created_at DESC, m.id DESC
LIMIT @lim;

-- name: GetMessagePosition :one
SELECT id, created_at FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND id = $3;

-- name: GetLatestMessagePosition :one
SELECT id, created_at FROM messages WHERE workspace_id = $1 AND conversation_id = $2
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CreateAttachment :one
INSERT INTO attachments (id, workspace_id, conversation_id, message_id, storage_key, filename,
                         content_type, size_bytes, content_id, inline, created_at)
VALUES (@id, @workspace_id, @conversation_id, @message_id, @storage_key, @filename, @content_type,
        @size_bytes, sqlc.narg(content_id), @inline, @created_at)
RETURNING *;

-- name: ListAttachments :many
SELECT * FROM attachments
WHERE workspace_id = @workspace_id AND message_id = ANY(@message_ids::uuid[])
ORDER BY message_id, created_at, id;

-- name: GetAttachment :one
SELECT a.*, c.inbox_id FROM attachments a
JOIN conversations c ON c.workspace_id = a.workspace_id AND c.id = a.conversation_id
WHERE a.workspace_id = $1 AND a.id = $2;

-- name: GetMessage :one
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM messages m
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id
WHERE m.workspace_id = $1 AND m.id = $2;

-- name: LockMessage :one
SELECT id, conversation_id, draft FROM messages WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: SetMessageDelivery :one
WITH m AS (
    UPDATE messages SET delivery_state = @state::text, delivery_error = sqlc.narg(error), delivery_updated_at = @now::timestamptz
    WHERE messages.workspace_id = @workspace_id AND messages.id = @id
    RETURNING *)
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM m
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id;

-- name: UpdateDraft :one
WITH m AS (
    UPDATE messages SET body = @body, html = sqlc.narg(html)
    WHERE messages.workspace_id = @workspace_id AND messages.id = @id AND draft
    RETURNING *)
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM m
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id;

-- name: SendDraft :one
WITH m AS (
    UPDATE messages SET draft = false, created_at = @now::timestamptz,
        sent_by_member_id = sqlc.narg(sent_by_member_id), sent_by_api_key_id = sqlc.narg(sent_by_api_key_id),
        sent_via = sqlc.narg(sent_via),
        delivery_state = sqlc.narg(delivery_state),
        delivery_updated_at = CASE WHEN sqlc.narg(delivery_state)::text IS NULL THEN NULL ELSE @now::timestamptz END
    WHERE messages.workspace_id = @workspace_id AND messages.id = @id AND draft
    RETURNING *)
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM m
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id;

-- name: DeleteDraft :execrows
DELETE FROM messages WHERE workspace_id = $1 AND id = $2 AND draft;

-- name: ListAttachmentsOfMessage :many
SELECT * FROM attachments WHERE workspace_id = $1 AND message_id = $2 ORDER BY created_at, id;

-- name: CallerDraftInConversation :one
SELECT id FROM messages
WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND draft
  AND (author_api_key_id = sqlc.narg(api_key_id)
       OR (author_member_id = sqlc.narg(member_id) AND via IS NOT DISTINCT FROM sqlc.narg(via)))
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: ListDrafts :many
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM messages m
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id
WHERE m.workspace_id = @workspace_id AND m.draft
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id))
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR c.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (sqlc.narg(author_kind)::text IS NULL
       OR (sqlc.narg(author_kind)::text = 'bot' AND m.author_type = 'bot')
       OR (sqlc.narg(author_kind)::text = 'assistant' AND m.author_type = 'member' AND m.via IS NOT NULL)
       OR (sqlc.narg(author_kind)::text = 'member' AND m.author_type = 'member' AND m.via IS NULL))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (m.created_at, m.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY m.created_at DESC, m.id DESC
LIMIT @lim;

-- name: CountDrafts :one
SELECT count(*) FROM messages m
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
WHERE m.workspace_id = @workspace_id AND m.draft
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id))
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR c.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (sqlc.narg(author_kind)::text IS NULL
       OR (sqlc.narg(author_kind)::text = 'bot' AND m.author_type = 'bot')
       OR (sqlc.narg(author_kind)::text = 'assistant' AND m.author_type = 'member' AND m.via IS NOT NULL)
       OR (sqlc.narg(author_kind)::text = 'member' AND m.author_type = 'member' AND m.via IS NULL));

-- name: ListMentions :many
SELECT m.id, m.conversation_id, m.author_type, m.author_member_id, m.author_api_key_id, m.via,
       left(m.body, 1000)::text AS body, m.created_at,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       (r.last_read_at IS NOT NULL AND (r.last_read_at, r.last_read_message_id) >= (m.created_at, m.id))::bool AS seen
FROM messages m
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
LEFT JOIN conversation_reads r
       ON r.workspace_id = m.workspace_id AND r.conversation_id = m.conversation_id AND r.member_id = @member_id
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
WHERE m.workspace_id = @workspace_id AND m.kind = 'note' AND NOT m.draft
  AND m.mentions @> ARRAY[@member_id::uuid]
  AND m.author_member_id IS DISTINCT FROM @member_id::uuid
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @member_id))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (m.created_at, m.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY m.created_at DESC, m.id DESC
LIMIT @lim;

-- name: CountUnseenMentions :one
SELECT count(*) FROM messages m
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
LEFT JOIN conversation_reads r
       ON r.workspace_id = m.workspace_id AND r.conversation_id = m.conversation_id AND r.member_id = @member_id
WHERE m.workspace_id = @workspace_id AND m.kind = 'note' AND NOT m.draft
  AND m.mentions @> ARRAY[@member_id::uuid]
  AND m.author_member_id IS DISTINCT FROM @member_id::uuid
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @member_id))
  AND (r.last_read_at IS NULL OR (r.last_read_at, r.last_read_message_id) < (m.created_at, m.id));

-- name: ListConversationSummaries :many
SELECT id, inbox_id, subject, status, contact_id FROM conversations
WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);
