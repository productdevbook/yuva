-- name: SaveChatChannel :one
INSERT INTO chat_channels (workspace_id, channel_id, public_key, allowed_origins, allow_anonymous, ask_email_offline,
                           greeting, launcher_position, launcher_color, platforms)
VALUES (@workspace_id, @channel_id, @public_key, @allowed_origins, @allow_anonymous, @ask_email_offline,
        @greeting, sqlc.narg(launcher_position), sqlc.narg(launcher_color), @platforms)
ON CONFLICT (workspace_id, channel_id) DO UPDATE SET
    allowed_origins = excluded.allowed_origins, allow_anonymous = excluded.allow_anonymous,
    ask_email_offline = excluded.ask_email_offline, greeting = excluded.greeting,
    launcher_position = excluded.launcher_position, launcher_color = excluded.launcher_color,
    platforms = excluded.platforms
RETURNING *;

-- name: GetChatChannel :one
SELECT * FROM chat_channels WHERE workspace_id = $1 AND channel_id = $2;

-- name: ListChatChannels :many
SELECT * FROM chat_channels WHERE workspace_id = @workspace_id AND channel_id = ANY(@channel_ids::uuid[]);

-- name: GetSessionChannel :one
SELECT cc.*, c.kind FROM chat_channels cc
JOIN channels c ON c.workspace_id = cc.workspace_id AND c.id = cc.channel_id
WHERE cc.workspace_id = $1 AND cc.channel_id = $2;

-- name: SetChatChannelKey :one
UPDATE chat_channels SET public_key = @public_key
WHERE workspace_id = @workspace_id AND channel_id = @channel_id
RETURNING *;

-- The widget names only the channel's public key, so the channel is found before the workspace is
-- known (see "Hosting for others later" in docs/architecture.md).
-- name: FindChatChannelByKey :one
SELECT cc.*, c.inbox_id, c.kind FROM chat_channels cc
JOIN channels c ON c.workspace_id = cc.workspace_id AND c.id = cc.channel_id
JOIN workspaces w ON w.id = cc.workspace_id
WHERE cc.public_key = $1 AND w.deleted_at IS NULL;

-- A CORS preflight carries neither the key nor the session (see "Hosting for others later").
-- name: AnyChatChannelAllowsOrigin :one
SELECT EXISTS (
    SELECT 1 FROM chat_channels cc JOIN workspaces w ON w.id = cc.workspace_id
    WHERE cc.allowed_origins @> ARRAY[@origin::text] AND w.deleted_at IS NULL
) AS allowed;

-- name: CreateContactSession :one
INSERT INTO contact_sessions (id, workspace_id, channel_id, inbox_id, contact_id, token_hash, identified,
                              created_at, expires_at, last_seen_at)
VALUES (@id, @workspace_id, @channel_id, @inbox_id, @contact_id, @token_hash, @identified, @now, @expires_at, @now)
RETURNING *;

-- A contact session token names no workspace (see "Hosting for others later").
-- name: GetContactSessionByTokenHash :one
SELECT s.*, ct.blocked AS contact_blocked FROM contact_sessions s
JOIN contacts ct ON ct.workspace_id = s.workspace_id AND ct.id = s.contact_id
JOIN workspaces w ON w.id = s.workspace_id
WHERE s.token_hash = $1 AND s.expires_at > $2 AND w.deleted_at IS NULL;

-- name: TouchContactSession :exec
UPDATE contact_sessions SET last_seen_at = @now::timestamptz, expires_at = @expires_at::timestamptz
WHERE workspace_id = @workspace_id AND id = @id AND last_seen_at < @stale_before::timestamptz;

-- name: SeeContactSession :exec
UPDATE contact_sessions SET last_seen_at = greatest(last_seen_at, @now::timestamptz)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteContactSession :exec
DELETE FROM contact_sessions WHERE workspace_id = $1 AND id = $2;

-- name: ContactLastSeen :one
SELECT coalesce(max(last_seen_at), 'epoch')::timestamptz AS last_seen FROM contact_sessions WHERE workspace_id = $1 AND contact_id = $2;

-- name: GetChatVisitor :one
SELECT contact_id FROM chat_visitors WHERE workspace_id = $1 AND inbox_id = $2 AND visitor_hash = $3;

-- name: CreateChatVisitor :exec
INSERT INTO chat_visitors (workspace_id, inbox_id, visitor_hash, contact_id, created_at) VALUES ($1, $2, $3, $4, $5);

-- name: DeleteChatVisitor :exec
DELETE FROM chat_visitors WHERE workspace_id = $1 AND inbox_id = $2 AND visitor_hash = $3;

-- name: IsChatVisitor :one
SELECT EXISTS (SELECT 1 FROM chat_visitors WHERE workspace_id = $1 AND contact_id = $2) AS visitor;

-- name: MoveContactConversations :many
UPDATE conversations SET contact_id = @to_contact, updated_at = @now
WHERE workspace_id = @workspace_id AND contact_id = @from_contact
RETURNING *;

-- name: MoveContactMessages :exec
UPDATE messages SET author_contact_id = @to_contact
WHERE workspace_id = @workspace_id AND author_contact_id = @from_contact;

-- name: MoveContactEmails :exec
UPDATE contact_emails e SET contact_id = @to_contact::uuid,
    position = e.position + (SELECT count(*) FROM contact_emails x WHERE x.workspace_id = @workspace_id::uuid AND x.contact_id = @to_contact::uuid)
WHERE e.workspace_id = @workspace_id::uuid AND e.contact_id = @from_contact::uuid;

-- name: SetContactIdentity :one
UPDATE contacts SET name = @name, locale = sqlc.narg(locale), attributes = @attributes, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING id, workspace_id, name, attributes, blocked, created_at, updated_at, locale, typed_email;

-- name: SetContactTypedEmail :one
UPDATE contacts SET typed_email = sqlc.narg(typed_email), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING id, workspace_id, name, attributes, blocked, created_at, updated_at, locale, typed_email;

-- name: CountContactEmails :one
SELECT count(*) FROM contact_emails WHERE workspace_id = $1 AND contact_id = $2;

-- name: ListContactConversations :many
SELECT c.* FROM conversations c
WHERE c.workspace_id = @workspace_id AND c.inbox_id = @inbox_id AND c.contact_id = @contact_id
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (coalesce(c.last_message_at, c.created_at), c.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY coalesce(c.last_message_at, c.created_at) DESC, c.id DESC
LIMIT @lim;

-- name: ListContactConversationIDs :many
SELECT id, status FROM conversations WHERE workspace_id = $1 AND inbox_id = $2 AND contact_id = $3;

-- name: ListUnreadForContact :many
SELECT c.id FROM conversations c
LEFT JOIN contact_reads r ON r.workspace_id = c.workspace_id AND r.conversation_id = c.id
WHERE c.workspace_id = @workspace_id AND c.id = ANY(@conversation_ids::uuid[])
  AND EXISTS (
      SELECT 1 FROM messages m
      WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id
        AND m.kind = 'message' AND m.direction = 'out' AND NOT m.draft
        AND (r.last_read_at IS NULL OR (m.created_at, m.id) > (r.last_read_at, r.last_read_message_id)));

-- name: ListMemberReadPositions :many
SELECT r.conversation_id, max(r.last_read_at)::timestamptz AS read_at
FROM conversation_reads r
JOIN conversations c ON c.workspace_id = r.workspace_id AND c.id = r.conversation_id
JOIN inboxes i ON i.workspace_id = c.workspace_id AND i.id = c.inbox_id
WHERE r.workspace_id = @workspace_id AND r.conversation_id = ANY(@conversation_ids::uuid[]) AND i.mode = 'live'
GROUP BY r.conversation_id;

-- name: ListPublicMessages :many
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM messages m
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id
WHERE m.workspace_id = @workspace_id AND m.conversation_id = @conversation_id AND m.kind = 'message' AND NOT m.draft
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (m.created_at, m.id) > (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY m.created_at, m.id
LIMIT @lim;

-- name: ListPublicMessagesDesc :many
SELECT m.id, m.workspace_id, m.conversation_id, m.kind, m.direction, m.author_type, m.author_member_id,
       m.author_contact_id, m.body, m.html, m.client_id, m.event, m.created_at, m.delivery_state, m.delivery_error,
       m.delivery_updated_at, m.author_api_key_id, m.draft, m.sent_by_member_id, m.sent_by_api_key_id, m.via, m.sent_via,
       coalesce(ak.bot_name, ak.name, '')::text AS bot_name, coalesce(ak.bot_avatar_url, '')::text AS bot_avatar_url,
       coalesce(sk.bot_name, sk.name, '')::text AS sent_by_bot_name
FROM messages m
LEFT JOIN api_keys ak ON ak.workspace_id = m.workspace_id AND ak.id = m.author_api_key_id
LEFT JOIN api_keys sk ON sk.workspace_id = m.workspace_id AND sk.id = m.sent_by_api_key_id
WHERE m.workspace_id = @workspace_id AND m.conversation_id = @conversation_id AND m.kind = 'message' AND NOT m.draft
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (m.created_at, m.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY m.created_at DESC, m.id DESC
LIMIT @lim;

-- name: GetLatestPublicMessagePosition :one
SELECT id, created_at FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND kind = 'message' AND NOT draft
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: MarkContactRead :one
INSERT INTO contact_reads (workspace_id, conversation_id, last_read_message_id, last_read_at, updated_at)
VALUES (@workspace_id, @conversation_id, @message_id, @message_at, @now)
ON CONFLICT (workspace_id, conversation_id) DO UPDATE
SET last_read_message_id = excluded.last_read_message_id, last_read_at = excluded.last_read_at, updated_at = excluded.updated_at
WHERE (contact_reads.last_read_at, contact_reads.last_read_message_id) < (excluded.last_read_at, excluded.last_read_message_id)
RETURNING *;

-- name: GetContactRead :one
SELECT * FROM contact_reads WHERE workspace_id = $1 AND conversation_id = $2;

-- name: GetClientAttachment :one
SELECT a.* FROM attachments a
JOIN messages m ON m.workspace_id = a.workspace_id AND m.id = a.message_id
JOIN conversations c ON c.workspace_id = a.workspace_id AND c.id = a.conversation_id
WHERE a.workspace_id = @workspace_id AND a.id = @id AND m.kind = 'message' AND NOT m.draft
  AND c.inbox_id = @inbox_id AND c.contact_id = @contact_id;

-- name: ListMemberNames :many
SELECT m.id, p.name FROM members m JOIN people p ON p.id = m.person_id
WHERE m.workspace_id = @workspace_id AND m.id = ANY(@ids::uuid[]);

-- name: OpenConnection :exec
INSERT INTO realtime_connections (id, workspace_id, member_id, contact_id, seen_at)
VALUES (@id, @workspace_id, sqlc.narg(member_id), sqlc.narg(contact_id), @now);

-- name: SeeConnection :exec
UPDATE realtime_connections SET seen_at = @now WHERE workspace_id = @workspace_id AND id = @id;

-- name: CloseConnection :exec
DELETE FROM realtime_connections WHERE workspace_id = $1 AND id = $2;

-- name: DeleteStaleConnections :execrows
DELETE FROM realtime_connections WHERE workspace_id = @workspace_id AND seen_at < @before;

-- name: ContactConnected :one
SELECT EXISTS (
    SELECT 1 FROM realtime_connections WHERE workspace_id = @workspace_id AND contact_id = @contact_id AND seen_at > @fresh_after
) AS connected;

-- name: ListAvailableMembers :many
SELECT DISTINCT ON (m.id) m.id, p.name, m.created_at FROM realtime_connections rc
JOIN members m ON m.workspace_id = rc.workspace_id AND m.id = rc.member_id
JOIN people p ON p.id = m.person_id
WHERE rc.workspace_id = @workspace_id AND rc.seen_at > @fresh_after AND p.availability = 'auto'
  AND (m.role IN ('owner', 'admin') OR EXISTS (
      SELECT 1 FROM inbox_members im
      WHERE im.workspace_id = m.workspace_id AND im.inbox_id = @inbox_id AND im.member_id = m.id))
ORDER BY m.id;

-- name: ListPendingReplies :many
SELECT m.id, m.created_at FROM messages m
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
LEFT JOIN contact_reads r ON r.workspace_id = m.workspace_id AND r.conversation_id = m.conversation_id
WHERE m.workspace_id = @workspace_id AND m.conversation_id = @conversation_id
  AND m.kind = 'message' AND m.direction = 'out' AND m.author_type = 'member' AND m.delivery_state IS NULL AND NOT m.draft
  AND (r.last_read_at IS NULL OR (m.created_at, m.id) > (r.last_read_at, r.last_read_message_id))
  AND (c.continuity_through IS NULL OR m.created_at > c.continuity_through)
ORDER BY m.created_at, m.id
LIMIT 50;

-- name: ListConversationsWithPendingReplies :many
SELECT DISTINCT c.id FROM conversations c
JOIN messages m ON m.workspace_id = c.workspace_id AND m.conversation_id = c.id
LEFT JOIN contact_reads r ON r.workspace_id = c.workspace_id AND r.conversation_id = c.id
WHERE c.workspace_id = @workspace_id AND c.contact_id = @contact_id
  AND m.kind = 'message' AND m.direction = 'out' AND m.author_type = 'member' AND m.delivery_state IS NULL AND NOT m.draft
  AND (r.last_read_at IS NULL OR (m.created_at, m.id) > (r.last_read_at, r.last_read_message_id))
  AND (c.continuity_through IS NULL OR m.created_at > c.continuity_through);

-- name: ClaimContinuity :exec
UPDATE conversations SET continuity_through = @through::timestamptz, continuity_sent_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id;

-- name: InboxEmailChannel :one
SELECT e.* FROM email_channels e
JOIN channels c ON c.workspace_id = e.workspace_id AND c.id = e.channel_id
WHERE e.workspace_id = $1 AND c.inbox_id = $2
ORDER BY (e.address LIKE '*@%'), c.created_at, c.id
LIMIT 1;

-- name: ListPersonWorkspaceIDs :many
SELECT m.workspace_id FROM members m JOIN workspaces w ON w.id = m.workspace_id
WHERE m.person_id = $1 AND w.deleted_at IS NULL;

-- name: SetPersonAvailability :exec
UPDATE people SET availability = $2 WHERE id = $1;

-- name: FindContactMessageByClientID :one
SELECT m.id, m.conversation_id FROM messages m
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
WHERE m.workspace_id = @workspace_id AND c.inbox_id = @inbox_id AND c.contact_id = @contact_id
  AND m.author_contact_id = @contact_id AND m.client_id = @client_id::text
ORDER BY m.created_at DESC
LIMIT 1;

-- name: FindThreadSentTo :one
SELECT c.contact_id FROM message_emails e
JOIN conversations c ON c.workspace_id = e.workspace_id AND c.id = e.conversation_id
WHERE e.workspace_id = @workspace_id AND c.inbox_id = @inbox_id AND e.direction = 'out'
  AND e.header_message_id = ANY(@ids::text[]) AND @address::text = ANY(e.to_addresses)
ORDER BY e.created_at DESC, e.message_id DESC
LIMIT 1;

-- name: DeleteExpiredContactSessions :execrows
DELETE FROM contact_sessions WHERE workspace_id = @workspace_id AND expires_at < @before;

-- name: CreateEmailConfirmation :exec
INSERT INTO email_confirmations (workspace_id, id, contact_id, inbox_id, email, token_hash, created_at, expires_at)
VALUES (@workspace_id, @id, @contact_id, @inbox_id, @email, @token_hash, @now, @expires_at);

-- name: CountEmailConfirmations :one
SELECT count(*) FROM email_confirmations WHERE workspace_id = @workspace_id AND contact_id = @contact_id AND created_at > @since;

-- name: GetEmailConfirmation :one
SELECT contact_id, inbox_id, email FROM email_confirmations
WHERE workspace_id = @workspace_id AND token_hash = @token_hash AND expires_at > @now;

-- name: TakeEmailConfirmation :one
DELETE FROM email_confirmations WHERE workspace_id = @workspace_id AND token_hash = @token_hash AND expires_at > @now
RETURNING contact_id, inbox_id, email;

-- name: DeleteContactEmailConfirmations :exec
DELETE FROM email_confirmations WHERE workspace_id = @workspace_id AND (contact_id = @contact_id OR expires_at <= @now);

-- name: DeleteExpiredIdentityTokenIDs :exec
DELETE FROM identity_token_ids WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id AND expires_at <= @now;

-- name: UseIdentityTokenID :execrows
INSERT INTO identity_token_ids (workspace_id, inbox_id, jti, expires_at)
VALUES (@workspace_id, @inbox_id, @jti, @expires_at)
ON CONFLICT DO NOTHING;
