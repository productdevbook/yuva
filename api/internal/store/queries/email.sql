-- name: CreateEmailChannel :one
INSERT INTO email_channels (workspace_id, channel_id, address, display_name, from_address, smtp_host, smtp_port,
                            smtp_username, smtp_password, smtp_tls, auto_reply_enabled, auto_reply_text,
                            auto_reply_interval_hours)
VALUES (@workspace_id, @channel_id, @address, @display_name, @from_address, @smtp_host, @smtp_port,
        @smtp_username, @smtp_password, @smtp_tls, @auto_reply_enabled, @auto_reply_text, @auto_reply_interval_hours)
ON CONFLICT (workspace_id, channel_id) DO UPDATE SET
    address = excluded.address, display_name = excluded.display_name, from_address = excluded.from_address,
    smtp_host = excluded.smtp_host, smtp_port = excluded.smtp_port, smtp_username = excluded.smtp_username,
    smtp_password = excluded.smtp_password, smtp_tls = excluded.smtp_tls,
    auto_reply_enabled = excluded.auto_reply_enabled, auto_reply_text = excluded.auto_reply_text,
    auto_reply_interval_hours = excluded.auto_reply_interval_hours
RETURNING *;

-- name: GetEmailChannel :one
SELECT * FROM email_channels WHERE workspace_id = $1 AND channel_id = $2;

-- name: ListEmailChannels :many
SELECT * FROM email_channels WHERE workspace_id = @workspace_id AND channel_id = ANY(@channel_ids::uuid[]);

-- Inbound mail names only its recipient, so the channel is found before the workspace is known
-- (see "Hosting for others later" in docs/architecture.md).
-- name: FindEmailChannelByAddress :one
SELECT e.*, c.inbox_id, c.name AS channel_name FROM email_channels e
JOIN channels c ON c.workspace_id = e.workspace_id AND c.id = e.channel_id
JOIN workspaces w ON w.id = e.workspace_id
WHERE e.address = $1 AND w.deleted_at IS NULL;

-- name: CreateMessageEmail :exec
INSERT INTO message_emails (workspace_id, message_id, conversation_id, channel_id, direction, header_message_id,
                            in_reply_to, references_ids, from_address, to_addresses, cc_addresses, subject,
                            full_text, full_html, quoted, headers, raw_key, raw_size, authentication_results,
                            dmarc, auto, created_at)
VALUES (@workspace_id, @message_id, @conversation_id, @channel_id, @direction, @header_message_id,
        @in_reply_to, @references_ids, @from_address, @to_addresses, @cc_addresses, @subject,
        @full_text, @full_html, @quoted, @headers, @raw_key, @raw_size, @authentication_results,
        @dmarc, @auto, @created_at);

-- name: GetMessageEmail :one
SELECT * FROM message_emails WHERE workspace_id = $1 AND message_id = $2;

-- name: ListMessageEmails :many
SELECT e.message_id, e.direction, e.header_message_id, e.from_address, e.to_addresses, e.cc_addresses, e.subject, e.quoted,
       (e.raw_key IS NOT NULL)::bool AS has_raw, e.auto, e.dmarc,
       (e.direction = 'in' AND NOT EXISTS (
           SELECT 1 FROM contact_emails ce
           WHERE ce.workspace_id = e.workspace_id AND ce.contact_id = c.contact_id AND ce.email = e.from_address
       ))::bool AS unverified_sender
FROM message_emails e
JOIN conversations c ON c.workspace_id = e.workspace_id AND c.id = e.conversation_id
WHERE e.workspace_id = @workspace_id AND e.message_id = ANY(@message_ids::uuid[]);

-- name: FindInboundEmailByHeader :one
SELECT message_id, conversation_id FROM message_emails
WHERE workspace_id = $1 AND channel_id = $2 AND direction = 'in' AND header_message_id = $3;

-- name: FindConversationByHeaders :one
SELECT c.* FROM message_emails e
JOIN conversations c ON c.workspace_id = e.workspace_id AND c.id = e.conversation_id
WHERE e.workspace_id = @workspace_id AND c.inbox_id = @inbox_id AND e.header_message_id = ANY(@ids::text[])
ORDER BY (c.contact_id = @contact_id) DESC, e.created_at DESC, e.message_id DESC
LIMIT 1;

-- name: FindConversationByEmailToken :one
SELECT * FROM conversations WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id AND email_token = ANY(@tokens::text[])
ORDER BY (contact_id = @contact_id) DESC, created_at DESC
LIMIT 1;

-- name: SetConversationEmailToken :one
UPDATE conversations SET email_token = coalesce(email_token, @token::text)
WHERE workspace_id = @workspace_id AND id = @id
RETURNING email_token::text;

-- name: LatestThreadEmail :one
SELECT header_message_id, in_reply_to, references_ids, from_address, direction FROM message_emails
WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND message_id <> @exclude_message_id
ORDER BY created_at DESC, message_id DESC
LIMIT 1;

-- Replies go only to an address of the conversation's contact, never to another sender in the thread.
-- name: ContactReplyAddress :one
SELECT e.from_address FROM message_emails e
JOIN conversations c ON c.workspace_id = e.workspace_id AND c.id = e.conversation_id
JOIN contact_emails ce ON ce.workspace_id = e.workspace_id AND ce.contact_id = c.contact_id AND ce.email = e.from_address
WHERE e.workspace_id = $1 AND e.conversation_id = $2 AND e.direction = 'in'
ORDER BY e.created_at DESC, e.message_id DESC
LIMIT 1;

-- name: FindOutboundEmailByHeader :one
SELECT e.message_id, e.conversation_id, e.to_addresses FROM message_emails e
WHERE e.workspace_id = $1 AND e.direction = 'out' AND e.header_message_id = $2;

-- SES reports name only our Message-ID, so the message is found before the workspace is known
-- (see "Hosting for others later" in docs/architecture.md).
-- name: FindOutboundEmailAnyWorkspace :one
SELECT e.workspace_id, e.message_id, e.conversation_id, e.to_addresses FROM message_emails e
JOIN workspaces w ON w.id = e.workspace_id
WHERE e.direction = 'out' AND e.header_message_id = $1 AND w.deleted_at IS NULL
LIMIT 1;

-- name: CountRecentConversations :one
SELECT count(*) FROM conversations
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND contact_id = @contact_id AND created_at > @since;

-- name: LatestContactConversation :one
SELECT * FROM conversations
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND contact_id = @contact_id
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CountRecentInboundEmails :one
SELECT count(*) FROM message_emails e
JOIN conversations c ON c.workspace_id = e.workspace_id AND c.id = e.conversation_id
WHERE e.workspace_id = @workspace_id AND c.contact_id = @contact_id AND e.direction = 'in' AND e.created_at > @since;

-- Mail from an address the server sends from is dropped, whichever workspace it belongs to.
-- name: IsEmailChannelSender :one
SELECT EXISTS (SELECT 1 FROM email_channels WHERE address = @address::text OR from_address = @address::text);

-- name: ClaimAutoReply :one
INSERT INTO email_auto_replies (workspace_id, channel_id, contact_id, sent_at)
VALUES (@workspace_id, @channel_id, @contact_id, @now)
ON CONFLICT (workspace_id, channel_id, contact_id) DO UPDATE SET sent_at = excluded.sent_at
WHERE email_auto_replies.sent_at <= @not_after
RETURNING sent_at;

-- name: SuppressEmail :exec
INSERT INTO email_suppressions (workspace_id, email, reason, detail, created_at)
VALUES (@workspace_id, @email, @reason, @detail, @now)
ON CONFLICT (workspace_id, email) DO UPDATE SET reason = excluded.reason, detail = excluded.detail, created_at = excluded.created_at;

-- name: IsEmailSuppressed :one
SELECT EXISTS (SELECT 1 FROM email_suppressions WHERE workspace_id = $1 AND email = $2) AS suppressed;

-- name: ListContactSuppressions :many
SELECT e.contact_id, s.email, s.reason, s.detail, s.created_at FROM email_suppressions s
JOIN contact_emails e ON e.workspace_id = s.workspace_id AND e.email = s.email
WHERE s.workspace_id = @workspace_id AND e.contact_id = ANY(@contact_ids::uuid[])
ORDER BY e.contact_id, e.position;

-- name: ClearSuppression :exec
DELETE FROM email_suppressions WHERE workspace_id = $1 AND email = $2;

-- name: GetContactIDByEmail :one
SELECT contact_id FROM contact_emails WHERE workspace_id = $1 AND email = $2;

-- name: SetConversationSpam :exec
UPDATE conversations SET spam = @spam WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListContactRawKeys :many
SELECT e.raw_key::text FROM message_emails e
JOIN conversations c ON c.workspace_id = e.workspace_id AND c.id = e.conversation_id
WHERE e.workspace_id = $1 AND c.contact_id = $2 AND e.raw_key IS NOT NULL;

-- name: ListInboxRawKeys :many
SELECT e.raw_key::text FROM message_emails e
JOIN conversations c ON c.workspace_id = e.workspace_id AND c.id = e.conversation_id
WHERE e.workspace_id = $1 AND c.inbox_id = $2 AND e.raw_key IS NOT NULL;

-- name: ListConversationsRawKeys :many
SELECT raw_key::text FROM message_emails
WHERE workspace_id = @workspace_id AND conversation_id = ANY(@ids::uuid[]) AND raw_key IS NOT NULL;

-- name: ListExpiredRawKeys :many
SELECT raw_key::text FROM message_emails
WHERE workspace_id = @workspace_id AND raw_key IS NOT NULL AND created_at < @before
ORDER BY created_at
LIMIT @max_rows;

-- name: ClearRawKeys :execrows
UPDATE message_emails SET raw_key = NULL, raw_size = NULL
WHERE workspace_id = @workspace_id AND raw_key = ANY(@keys::text[]);
