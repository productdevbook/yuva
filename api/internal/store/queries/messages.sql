-- name: CreateMessage :one
WITH m AS (
    INSERT INTO messages (id, workspace_id, conversation_id, kind, direction, author_type, author_member_id,
                          author_contact_id, author_api_key_id, body, html, client_id, event, created_at, draft,
                          delivery_state, delivery_updated_at, via, mentions, help_cards, rule, forward)
    VALUES (@id, @workspace_id, @conversation_id, @kind, @direction, @author_type, @author_member_id,
            @author_contact_id, sqlc.narg(author_api_key_id), @body, @html, @client_id, @event, @created_at, @draft,
            sqlc.narg(delivery_state),
            CASE WHEN sqlc.narg(delivery_state)::text IS NULL THEN NULL ELSE @created_at::timestamptz END,
            sqlc.narg(via), coalesce(sqlc.narg(mentions)::uuid[], '{}'), sqlc.narg(help_cards)::jsonb, sqlc.narg(rule)::jsonb,
            sqlc.narg(forward)::jsonb)
    ON CONFLICT (workspace_id, conversation_id, client_id) DO NOTHING
    RETURNING *),
seen AS (
    UPDATE contacts SET last_active_at = greatest(coalesce(contacts.last_active_at, m.created_at), m.created_at)
    FROM m WHERE contacts.workspace_id = m.workspace_id AND contacts.id = m.author_contact_id)
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions, m.imported_at, m.imported_author, m.help_cards,
       m.redacted_at, m.redacted_by_member_id, m.redacted_by_api_key_id, m.rule, m.forward,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM m
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id;

-- name: GetMessageByClientID :one
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions, m.imported_at, m.imported_author, m.help_cards,
       m.redacted_at, m.redacted_by_member_id, m.redacted_by_api_key_id, m.rule, m.forward,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM messages m
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id
WHERE m.workspace_id = $1 AND m.conversation_id = $2 AND m.client_id = $3;

-- name: ListMessages :many
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions, m.imported_at, m.imported_author, m.help_cards,
       m.redacted_at, m.redacted_by_member_id, m.redacted_by_api_key_id, m.rule, m.forward,
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
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions, m.imported_at, m.imported_author, m.help_cards,
       m.redacted_at, m.redacted_by_member_id, m.redacted_by_api_key_id, m.rule, m.forward,
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
SELECT a.*, c.inbox_id, c.trashed_at FROM attachments a
JOIN conversations c ON c.workspace_id = a.workspace_id AND c.id = a.conversation_id
WHERE a.workspace_id = $1 AND a.id = $2;

-- name: GetMessage :one
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions, m.imported_at, m.imported_author, m.help_cards,
       m.redacted_at, m.redacted_by_member_id, m.redacted_by_api_key_id, m.rule, m.forward,
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
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions, m.imported_at, m.imported_author, m.help_cards,
       m.redacted_at, m.redacted_by_member_id, m.redacted_by_api_key_id, m.rule, m.forward,
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
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions, m.imported_at, m.imported_author, m.help_cards,
       m.redacted_at, m.redacted_by_member_id, m.redacted_by_api_key_id, m.rule, m.forward,
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
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions, m.imported_at, m.imported_author, m.help_cards,
       m.redacted_at, m.redacted_by_member_id, m.redacted_by_api_key_id, m.rule, m.forward,
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
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions, m.imported_at, m.imported_author, m.help_cards,
       m.redacted_at, m.redacted_by_member_id, m.redacted_by_api_key_id, m.rule, m.forward,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM messages m
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id
WHERE m.workspace_id = @workspace_id AND m.draft AND c.trashed_at IS NULL
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
WHERE m.workspace_id = @workspace_id AND m.draft AND c.trashed_at IS NULL
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
       v.seen::bool AS seen
FROM member_mentions v
JOIN messages m ON m.workspace_id = v.workspace_id AND m.id = v.message_id
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
WHERE v.workspace_id = @workspace_id AND v.member_id = @member_id AND v.mentions @> ARRAY[@member_id::uuid]
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = v.workspace_id AND iv.inbox_id = v.inbox_id AND iv.viewer_id = @member_id))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (v.created_at, v.message_id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY v.created_at DESC, v.message_id DESC
LIMIT @lim;

-- name: CountUnseenMentions :one
SELECT count(*) FROM member_mentions v
WHERE v.workspace_id = @workspace_id AND v.member_id = @member_id AND v.mentions @> ARRAY[@member_id::uuid]
  AND NOT v.seen
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = v.workspace_id AND iv.inbox_id = v.inbox_id AND iv.viewer_id = @member_id));

-- name: GetMentionNote :one
SELECT v.message_id, v.conversation_id FROM member_mentions v
WHERE v.workspace_id = @workspace_id AND v.member_id = @member_id AND v.mentions @> ARRAY[@member_id::uuid]
  AND v.message_id = @id;

-- name: MarkMentionSeen :execrows
INSERT INTO mention_seen (workspace_id, member_id, message_id, seen_at)
SELECT v.workspace_id, v.member_id, v.message_id, @now
FROM member_mentions v
WHERE v.workspace_id = @workspace_id AND v.member_id = @member_id AND v.message_id = @id AND NOT v.seen
ON CONFLICT DO NOTHING;

-- name: MarkMentionsSeen :execrows
INSERT INTO mention_seen (workspace_id, member_id, message_id, seen_at)
SELECT v.workspace_id, v.member_id, v.message_id, @now
FROM member_mentions v
WHERE v.workspace_id = @workspace_id AND v.member_id = @member_id AND v.mentions @> ARRAY[@member_id::uuid]
  AND NOT v.seen
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = v.workspace_id AND iv.inbox_id = v.inbox_id AND iv.viewer_id = @member_id))
ON CONFLICT DO NOTHING;

-- name: ListConversationSummaries :many
SELECT id, inbox_id, subject, status, contact_id FROM conversations
WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);

-- name: RedactMessage :one
WITH m AS (
    UPDATE messages SET body = '', html = NULL, help_cards = NULL, mentions = '{}',
        redacted_at = @now::timestamptz, redacted_by_member_id = sqlc.narg(member_id), redacted_by_api_key_id = sqlc.narg(api_key_id),
        delivery_state = CASE WHEN delivery_state = 'queued' THEN 'failed' ELSE delivery_state END,
        delivery_error = CASE WHEN delivery_state = 'queued' THEN 'removed before it was sent' ELSE delivery_error END,
        delivery_updated_at = CASE WHEN delivery_state = 'queued' THEN @now::timestamptz ELSE delivery_updated_at END
    WHERE messages.workspace_id = @workspace_id AND messages.id = @id AND messages.redacted_at IS NULL
    RETURNING *)
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via, m.mentions, m.imported_at, m.imported_author, m.help_cards,
       m.redacted_at, m.redacted_by_member_id, m.redacted_by_api_key_id, m.rule, m.forward,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM m
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id;

-- name: DeleteMessageAttachments :many
DELETE FROM attachments WHERE workspace_id = @workspace_id AND message_id = @message_id
RETURNING storage_key;

-- name: RedactMessageEmail :many
UPDATE message_emails me SET subject = '', full_text = '', full_html = NULL, quoted = false, headers = '{}',
    raw_key = NULL, raw_size = NULL
FROM (SELECT raw_key FROM message_emails WHERE workspace_id = @workspace_id AND message_id = @message_id FOR UPDATE) old
WHERE me.workspace_id = @workspace_id AND me.message_id = @message_id
RETURNING old.raw_key;

-- name: ListLaterInboundEmails :many
SELECT me.message_id, me.full_text, me.full_html, me.raw_key, me.in_reply_to, me.references_ids, m.body, m.html FROM message_emails me
JOIN messages m ON m.workspace_id = me.workspace_id AND m.id = me.message_id
WHERE me.workspace_id = @workspace_id AND me.conversation_id = @conversation_id AND me.direction = 'in'
  AND me.message_id <> @message_id AND me.created_at >= @since::timestamptz AND m.redacted_at IS NULL
  AND (me.quoted OR me.raw_key IS NOT NULL)
ORDER BY me.created_at, me.message_id;

-- name: DropEmailQuotes :exec
UPDATE message_emails SET full_text = @full_text, full_html = sqlc.narg(full_html), quoted = false, raw_key = NULL, raw_size = NULL
WHERE workspace_id = @workspace_id AND message_id = @message_id;

-- name: ClearFeedbackText :one
UPDATE conversations SET feedback = feedback || '{"text": "", "attachment_count": 0}'::jsonb, updated_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id AND kind = 'feedback'
RETURNING *;

-- name: ClearConversationSubject :one
UPDATE conversations SET subject = '', updated_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: ClearEmailSubjects :exec
UPDATE message_emails SET subject = ''
WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND strpos(lower(subject), lower(@subject::text)) > 0;

-- Waits for a redaction of these messages in progress, then reports which are redacted.
-- name: LockMessagesShared :many
SELECT id, (redacted_at IS NOT NULL)::bool AS redacted FROM messages
WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[])
ORDER BY id
FOR SHARE;

-- name: LockMessagesForSend :many
SELECT id FROM messages WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[])
ORDER BY id
FOR UPDATE;

-- name: ListForwardableMessageIDs :many
SELECT id FROM (
    SELECT id, created_at FROM messages
    WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND kind = 'message' AND NOT draft
      AND redacted_at IS NULL AND created_at <= @before::timestamptz
    ORDER BY created_at DESC, id DESC
    LIMIT @lim) latest
ORDER BY created_at, id;

-- name: SetMessageForward :exec
UPDATE messages SET forward = @forward::jsonb WHERE workspace_id = @workspace_id AND id = @id AND kind = 'forward';
