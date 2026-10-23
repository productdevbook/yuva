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
    auto_reply_interval_hours = excluded.auto_reply_interval_hours,
    verified_at = CASE WHEN email_channels.address = excluded.address THEN email_channels.verified_at END,
    verified_by = CASE WHEN email_channels.address = excluded.address THEN email_channels.verified_by END,
    verify_problem = CASE WHEN email_channels.address = excluded.address THEN email_channels.verify_problem END,
    verify_checked_at = CASE WHEN email_channels.address = excluded.address THEN email_channels.verify_checked_at END,
    verify_code_hash = CASE WHEN email_channels.address = excluded.address THEN email_channels.verify_code_hash END,
    verify_code_expires_at = CASE WHEN email_channels.address = excluded.address THEN email_channels.verify_code_expires_at END,
    verify_message_id = CASE WHEN email_channels.address = excluded.address THEN email_channels.verify_message_id END,
    verify_relay_message_id = CASE WHEN email_channels.address = excluded.address THEN email_channels.verify_relay_message_id END
RETURNING *;

-- name: GetEmailChannel :one
SELECT * FROM email_channels WHERE workspace_id = $1 AND channel_id = $2;

-- name: ListEmailChannels :many
SELECT * FROM email_channels WHERE workspace_id = @workspace_id AND channel_id = ANY(@channel_ids::uuid[]);

-- Inbound mail names only its recipient, so the channel is found before the workspace is known
-- (see "Hosting for others later" in docs/architecture.md); only a proven address routes.
-- name: FindEmailChannelByAddress :one
SELECT e.*, c.inbox_id, c.name AS channel_name FROM email_channels e
JOIN channels c ON c.workspace_id = e.workspace_id AND c.id = e.channel_id
JOIN workspaces w ON w.id = e.workspace_id
WHERE e.address = $1 AND e.verified_at IS NOT NULL AND w.deleted_at IS NULL;

-- A forwarding address names only its token (see "Hosting for others later" in docs/architecture.md).
-- name: FindEmailChannelByForwardToken :one
SELECT e.*, c.inbox_id, c.name AS channel_name FROM email_channels e
JOIN channels c ON c.workspace_id = e.workspace_id AND c.id = e.channel_id
JOIN workspaces w ON w.id = e.workspace_id
WHERE e.forward_token = $1 AND w.deleted_at IS NULL;

-- name: GetIngestChannel :one
SELECT e.*, c.inbox_id, c.name AS channel_name FROM email_channels e
JOIN channels c ON c.workspace_id = e.workspace_id AND c.id = e.channel_id
WHERE e.workspace_id = $1 AND e.channel_id = $2;

-- name: RotateForwardToken :one
UPDATE email_channels SET forward_token = DEFAULT WHERE workspace_id = $1 AND channel_id = $2
RETURNING *;

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
SELECT e.message_id, e.direction, e.header_message_id, e.from_address, e.to_addresses, e.cc_addresses, e.bcc_addresses, e.subject, e.quoted,
       (e.raw_key IS NOT NULL)::bool AS has_raw, e.auto, e.dmarc,
       (e.direction = 'in' AND NOT EXISTS (
           SELECT 1 FROM contact_emails ce
           WHERE ce.workspace_id = e.workspace_id AND ce.contact_id = coalesce(m.author_contact_id, c.contact_id)
             AND ce.email = e.from_address
       ))::bool AS unverified_sender
FROM message_emails e
JOIN conversations c ON c.workspace_id = e.workspace_id AND c.id = e.conversation_id
JOIN messages m ON m.workspace_id = e.workspace_id AND m.id = e.message_id
WHERE e.workspace_id = @workspace_id AND e.message_id = ANY(@message_ids::uuid[]);

-- name: FindInboundEmailByHeader :one
SELECT message_id, conversation_id FROM message_emails
WHERE workspace_id = $1 AND channel_id = $2 AND direction = 'in' AND header_message_id = $3;

-- name: FindConversationByHeaders :one
SELECT c.* FROM message_emails e
JOIN conversations c ON c.workspace_id = e.workspace_id AND c.id = e.conversation_id
WHERE e.workspace_id = @workspace_id AND c.inbox_id = @inbox_id AND c.trashed_at IS NULL
  AND (e.header_message_id = ANY(@ids::text[]) OR e.relay_message_id = ANY(@ids::text[]))
ORDER BY (c.contact_id = @contact_id) DESC, e.created_at DESC, e.message_id DESC
LIMIT 1;

-- name: FindConversationByEmailToken :one
SELECT * FROM conversations WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id AND email_token = ANY(@tokens::text[])
  AND trashed_at IS NULL
ORDER BY (contact_id = @contact_id) DESC, created_at DESC
LIMIT 1;

-- name: SetConversationEmailToken :one
UPDATE conversations SET email_token = coalesce(email_token, @token::text)
WHERE workspace_id = @workspace_id AND id = @id
RETURNING email_token::text;

-- Forwards and their answers are a separate thread with someone else; replies never name them.
-- name: LatestThreadEmail :one
SELECT e.header_message_id, e.in_reply_to, e.references_ids, e.from_address, e.direction FROM message_emails e
JOIN messages m ON m.workspace_id = e.workspace_id AND m.id = e.message_id
WHERE e.workspace_id = @workspace_id AND e.conversation_id = @conversation_id AND e.message_id <> @exclude_message_id
  AND m.kind <> 'forward'
ORDER BY e.created_at DESC, e.message_id DESC
LIMIT 1;

-- Replies go only to an address of the conversation's contact, never to another sender in the thread.
-- name: ContactReplyAddress :one
SELECT e.from_address FROM message_emails e
JOIN conversations c ON c.workspace_id = e.workspace_id AND c.id = e.conversation_id
JOIN contact_emails ce ON ce.workspace_id = e.workspace_id AND ce.contact_id = c.contact_id AND ce.email = e.from_address
WHERE e.workspace_id = $1 AND e.conversation_id = $2 AND e.direction = 'in'
ORDER BY e.created_at DESC, e.message_id DESC
LIMIT 1;

-- name: SetRelayMessageID :exec
UPDATE message_emails SET relay_message_id = @relay_message_id
WHERE workspace_id = @workspace_id AND message_id = @message_id AND direction = 'out';

-- name: FindOutboundEmailByHeader :one
SELECT e.message_id, e.conversation_id, e.header_message_id, e.relay_message_id, e.to_addresses, e.cc_addresses, e.bcc_addresses, e.shared FROM message_emails e
WHERE e.workspace_id = @workspace_id AND e.direction = 'out' AND e.imported_at IS NULL
  AND (e.header_message_id = @header_message_id OR e.relay_message_id = @header_message_id)
LIMIT 1;

-- Automatic mail is left out so that a refused, stored report cannot block the genuine one.
-- name: MessageIDSharedBeyond :one
SELECT EXISTS (
  SELECT 1 FROM message_emails e
  WHERE e.workspace_id = @workspace_id
    AND (e.in_reply_to = ANY(@ids::text[]) OR e.references_ids && @ids::text[])
    AND CASE WHEN e.direction = 'out'
      THEN e.shared OR e.to_addresses <> ARRAY[@email::text] OR cardinality(e.cc_addresses) > 0 OR cardinality(e.bcc_addresses) > 0
      ELSE NOT e.auto AND (e.shared OR e.from_address <> @email::text OR cardinality(e.to_addresses) > 1 OR cardinality(e.cc_addresses) > 0)
    END
)::bool AS shared;

-- SES reports name only our Message-ID, so the message is found before the workspace is known
-- (see "Hosting for others later" in docs/architecture.md).
-- name: FindOutboundEmailAnyWorkspace :one
SELECT e.workspace_id, e.message_id, e.conversation_id, e.to_addresses, e.cc_addresses, e.bcc_addresses, e.channel_id FROM message_emails e
JOIN workspaces w ON w.id = e.workspace_id
WHERE e.direction = 'out' AND e.imported_at IS NULL AND e.header_message_id = $1 AND w.deleted_at IS NULL
LIMIT 1;

-- name: CountRecentConversations :one
SELECT count(*) FROM conversations
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND contact_id = @contact_id AND created_at > @since;

-- name: LatestContactConversation :one
SELECT * FROM conversations
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND contact_id = @contact_id AND trashed_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CountRecentInboundEmails :one
SELECT count(*) FROM message_emails e
JOIN conversations c ON c.workspace_id = e.workspace_id AND c.id = e.conversation_id
JOIN messages m ON m.workspace_id = e.workspace_id AND m.id = e.message_id
WHERE e.workspace_id = @workspace_id AND coalesce(m.author_contact_id, c.contact_id) = @contact_id::uuid
  AND e.direction = 'in' AND (e.dmarc = 'pass') = @authenticated::bool AND e.created_at > @since;

-- name: IsEmailChannelSender :one
SELECT EXISTS (
  SELECT 1 FROM email_channels
  WHERE workspace_id = @workspace_id AND verified_at IS NOT NULL AND (address = @address::text OR from_address = @address::text)
);

-- name: IsWorkspaceEmailAddress :one
SELECT EXISTS (
  SELECT 1 FROM email_channels
  WHERE workspace_id = @workspace_id AND verified_at IS NOT NULL
    AND (address = ANY(@addresses::text[]) OR from_address = @address::text)
);

-- name: LockEmailAddress :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_email_address:' || @address::text, 0));

-- name: LockEmailChannel :one
SELECT * FROM email_channels WHERE workspace_id = $1 AND channel_id = $2 FOR UPDATE;

-- name: CountWorkspaceVerificationSends :one
SELECT coalesce(sum(verify_sends), 0)::bigint FROM email_channels
WHERE workspace_id = @workspace_id AND verify_window_at > @since;

-- name: StartEmailVerificationCode :one
UPDATE email_channels SET verify_code_hash = @code_hash, verify_code_expires_at = @expires_at, verify_attempts = 0,
    verify_message_id = @message_id, verify_relay_message_id = NULL, verify_sent_at = @now,
    verify_sends = CASE WHEN verify_window_at > @window_since THEN verify_sends + 1 ELSE 1 END,
    verify_window_at = CASE WHEN verify_window_at > @window_since THEN verify_window_at ELSE @now END
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND address = @address AND verified_at IS NULL
RETURNING *;

-- name: SetEmailVerificationRelayID :exec
UPDATE email_channels SET verify_relay_message_id = @relay_message_id
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND verify_message_id = @message_id;

-- name: FailEmailVerificationCode :exec
UPDATE email_channels SET verify_attempts = verify_attempts + 1,
    verify_code_hash = CASE WHEN verify_attempts + 1 >= @max_attempts::int THEN NULL ELSE verify_code_hash END
WHERE workspace_id = @workspace_id AND channel_id = @channel_id;

-- name: SetEmailVerifyProblem :one
UPDATE email_channels SET verify_problem = @problem, verify_checked_at = @now
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND verified_at IS NULL
RETURNING *;

-- A proof takes the address from the channel that held it, in whichever workspace: the one write
-- across workspaces, keyed by the address alone like the ingress lookup (see "E-mail" in
-- docs/architecture.md).
-- name: ReleaseEmailAddress :many
UPDATE email_channels SET verified_at = NULL, verified_by = NULL, verify_problem = 'taken', verify_checked_at = @now
WHERE address = @address AND verified_at IS NOT NULL AND NOT (workspace_id = @workspace_id AND channel_id = @channel_id)
RETURNING workspace_id, channel_id;

-- name: MarkEmailChannelVerified :one
UPDATE email_channels SET verified_at = @now, verified_by = @method, verify_problem = NULL, verify_checked_at = @now,
    verify_code_hash = NULL, verify_code_expires_at = NULL, verify_message_id = NULL, verify_relay_message_id = NULL
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND address = @address
RETURNING *;

-- name: ClaimAutoReply :one
INSERT INTO email_auto_replies (workspace_id, channel_id, contact_id, sent_at)
VALUES (@workspace_id, @channel_id, @contact_id, @now)
ON CONFLICT (workspace_id, channel_id, contact_id) DO UPDATE SET sent_at = excluded.sent_at
WHERE email_auto_replies.sent_at <= @not_after
RETURNING sent_at;

-- name: SuppressEmail :exec
INSERT INTO email_suppressions (workspace_id, email, reason, detail, created_at)
VALUES (@workspace_id, @email, @reason, @detail, @now)
ON CONFLICT (workspace_id, email) DO UPDATE SET reason = excluded.reason, detail = excluded.detail, scope = 'all', created_at = excluded.created_at;

-- name: IsEmailSuppressed :one
SELECT EXISTS (SELECT 1 FROM email_suppressions WHERE workspace_id = $1 AND email = $2 AND scope = 'all') AS suppressed;

-- Stops only confirmation mails; any other suppression replaces it, and a complaint is never turned back into a bounce.
-- name: SuppressConfirmations :exec
INSERT INTO email_suppressions (workspace_id, email, reason, detail, scope, created_at)
VALUES (@workspace_id, @email, @reason, @detail, 'confirmations', @now)
ON CONFLICT (workspace_id, email) DO UPDATE SET reason = excluded.reason, detail = excluded.detail, created_at = excluded.created_at
WHERE email_suppressions.scope = 'confirmations' AND email_suppressions.reason = 'bounce';

-- name: EmailSuppressionReason :one
SELECT reason FROM email_suppressions
WHERE workspace_id = @workspace_id AND email = @email
  AND NOT (scope = 'confirmations' AND reason = 'bounce' AND created_at <= @expired);

-- name: DeleteExpiredConfirmationBounces :execrows
DELETE FROM email_suppressions
WHERE workspace_id = @workspace_id AND scope = 'confirmations' AND reason = 'bounce' AND created_at <= @before;

-- name: ListContactSuppressions :many
SELECT e.contact_id, s.email, s.reason, s.detail, s.created_at FROM email_suppressions s
JOIN contact_emails e ON e.workspace_id = s.workspace_id AND e.email = s.email
WHERE s.workspace_id = @workspace_id AND e.contact_id = ANY(@contact_ids::uuid[]) AND s.scope <> 'confirmations'
ORDER BY e.contact_id, e.position;

-- name: ClearSuppression :exec
DELETE FROM email_suppressions WHERE workspace_id = $1 AND email = $2;

-- name: GetContactIDByEmail :one
SELECT contact_id FROM contact_emails WHERE workspace_id = $1 AND email = $2;

-- name: ListContactRawKeys :many
SELECT e.raw_key::text FROM message_emails e
JOIN conversations c ON c.workspace_id = e.workspace_id AND c.id = e.conversation_id
JOIN messages m ON m.workspace_id = e.workspace_id AND m.id = e.message_id
WHERE e.workspace_id = $1 AND (c.contact_id = $2 OR m.author_contact_id = $2) AND e.raw_key IS NOT NULL;

-- name: ListInboxRawKeys :many
SELECT e.raw_key::text FROM message_emails e
JOIN conversations c ON c.workspace_id = e.workspace_id AND c.id = e.conversation_id
WHERE e.workspace_id = $1 AND c.inbox_id = $2 AND e.raw_key IS NOT NULL;

-- name: ListConversationsRawKeys :many
SELECT raw_key::text FROM message_emails
WHERE workspace_id = @workspace_id AND conversation_id = ANY(@ids::uuid[]) AND raw_key IS NOT NULL;

-- name: ListExpiredRawKeys :many
SELECT raw_key::text FROM message_emails
WHERE workspace_id = @workspace_id AND raw_key IS NOT NULL AND coalesce(imported_at, created_at) < @before::timestamptz
ORDER BY coalesce(imported_at, created_at)
LIMIT @max_rows;

-- name: ClearRawKeys :execrows
UPDATE message_emails SET raw_key = NULL, raw_size = NULL
WHERE workspace_id = @workspace_id AND raw_key = ANY(@keys::text[]);

-- name: SetEmailChannelBroadcast :one
UPDATE email_channels SET broadcast_smtp_host = sqlc.narg(host), broadcast_smtp_port = sqlc.narg(port),
    broadcast_smtp_username = sqlc.narg(username), broadcast_smtp_password = sqlc.narg(password),
    broadcast_smtp_tls = sqlc.narg(tls), broadcast_rate = @rate
WHERE workspace_id = @workspace_id AND channel_id = @channel_id
RETURNING *;

-- name: CreateEmailChannelDkimKey :one
UPDATE email_channels SET dkim_selector = @selector, dkim_private_key = @private_key, dkim_public_key = @public_key,
    dkim_enabled = false
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND dkim_private_key IS NULL
RETURNING *;

-- name: DeleteEmailChannelDkimKey :execrows
UPDATE email_channels SET dkim_selector = NULL, dkim_private_key = NULL, dkim_public_key = NULL, dkim_enabled = false
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND dkim_private_key IS NOT NULL;

-- name: SetEmailChannelDkimEnabled :one
UPDATE email_channels SET dkim_enabled = @enabled
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND (dkim_private_key IS NOT NULL OR NOT @enabled)
RETURNING *;
