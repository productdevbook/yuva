-- name: CreateMailingList :one
INSERT INTO mailing_lists (id, workspace_id, inbox_id, channel_id, kind, name, description, double_opt_in, public, commercial, welcome, created_at, updated_at)
VALUES (@id, @workspace_id, @inbox_id, sqlc.narg(channel_id), @kind, @name, @description, @double_opt_in, @public, @commercial, @welcome, @now, @now)
RETURNING *;

-- name: GetMailingList :one
SELECT * FROM mailing_lists WHERE workspace_id = $1 AND id = $2;

-- name: LockMailingList :one
SELECT * FROM mailing_lists WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: UpdateMailingList :one
UPDATE mailing_lists SET name = @name, description = @description, double_opt_in = @double_opt_in, public = @public,
    commercial = @commercial, welcome = @welcome, channel_id = sqlc.narg(channel_id), archived_at = sqlc.narg(archived_at),
    updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteMailingList :execrows
DELETE FROM mailing_lists WHERE workspace_id = $1 AND id = $2;

-- name: ListMailingLists :many
SELECT * FROM mailing_lists l
WHERE l.workspace_id = @workspace_id
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR l.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND @archived::bool = (l.archived_at IS NOT NULL)
  AND l.kind = @kind
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = l.workspace_id AND iv.inbox_id = l.inbox_id AND iv.viewer_id = @viewer_id))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (l.created_at, l.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY l.created_at DESC, l.id DESC
LIMIT @lim;

-- name: ListPublicMailingLists :many
SELECT * FROM mailing_lists
WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id AND public AND archived_at IS NULL AND kind = 'list'
ORDER BY name, id;

-- name: ListInboxMailingLists :many
SELECT * FROM mailing_lists WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id ORDER BY name, id;

-- name: ListMailingListsByIDs :many
SELECT * FROM mailing_lists WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);

-- name: CountSubscriptions :many
SELECT list_id,
       count(*) FILTER (WHERE state = 'subscribed')::bigint AS subscribed,
       count(*) FILTER (WHERE state = 'pending')::bigint AS pending,
       count(*) FILTER (WHERE state = 'unsubscribed')::bigint AS unsubscribed
FROM list_subscriptions
WHERE workspace_id = @workspace_id AND list_id = ANY(@list_ids::uuid[])
GROUP BY list_id;

-- name: MailingListGrowth :many
SELECT x.day::date AS day, sum(x.n_new)::bigint AS new, sum(x.n_confirmed)::bigint AS confirmed, sum(x.n_unsubscribed)::bigint AS unsubscribed
FROM (
    SELECT (s.created_at AT TIME ZONE 'UTC')::date AS day, 1 AS n_new, 0 AS n_confirmed, 0 AS n_unsubscribed
    FROM list_subscriptions s WHERE s.workspace_id = @workspace_id AND s.list_id = @list_id AND s.created_at >= @since
    UNION ALL
    SELECT (e.created_at AT TIME ZONE 'UTC')::date, 0,
           CASE WHEN e.kind = 'confirmed' THEN 1 ELSE 0 END,
           CASE WHEN e.kind IN ('unsubscribed', 'complained') THEN 1 ELSE 0 END
    FROM consent_events e
    JOIN list_subscriptions s ON s.workspace_id = e.workspace_id AND s.id = e.subscription_id
    WHERE e.workspace_id = @workspace_id AND s.list_id = @list_id AND e.created_at >= @since
      AND e.kind IN ('confirmed', 'unsubscribed', 'complained')
) x
GROUP BY x.day
ORDER BY x.day;

-- name: MailingListConfirmations :one
SELECT count(*) FILTER (WHERE confirmed_at IS NOT NULL)::bigint AS confirmed,
       count(*) FILTER (WHERE confirmed_at IS NOT NULL OR state = 'pending' OR confirmations_sent > 0)::bigint AS asked
FROM list_subscriptions
WHERE workspace_id = @workspace_id AND list_id = @list_id AND created_at >= @since;

-- name: CreateSubscription :one
INSERT INTO list_subscriptions (id, workspace_id, list_id, contact_id, email, state, source, consent_at, confirmed_at, created_at, updated_at)
VALUES (@id, @workspace_id, @list_id, @contact_id, @email, @state, @source, sqlc.narg(consent_at), sqlc.narg(confirmed_at), @now, @now)
RETURNING *;

-- name: GetSubscription :one
SELECT * FROM list_subscriptions WHERE workspace_id = $1 AND id = $2;

-- name: LockSubscription :one
SELECT * FROM list_subscriptions WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: LockSubscriptionByEmail :one
SELECT * FROM list_subscriptions WHERE workspace_id = @workspace_id AND list_id = @list_id AND email = @email FOR UPDATE;

-- name: SaveSubscriptionState :one
UPDATE list_subscriptions SET state = @state, consent_at = sqlc.narg(consent_at), confirmed_at = sqlc.narg(confirmed_at),
    unsubscribed_at = sqlc.narg(unsubscribed_at), unsubscribe_reason = sqlc.narg(unsubscribe_reason),
    unsubscribe_broadcast_id = sqlc.narg(unsubscribe_broadcast_id), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: MarkConfirmationSent :exec
UPDATE list_subscriptions SET confirmation_sent_at = @now::timestamptz, confirmations_sent = confirmations_sent + 1, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteSubscription :execrows
DELETE FROM list_subscriptions WHERE workspace_id = $1 AND id = $2;

-- name: ListSubscriptions :many
SELECT * FROM list_subscriptions s
WHERE s.workspace_id = @workspace_id AND s.list_id = @list_id
  AND (sqlc.narg(state)::text IS NULL OR s.state = sqlc.narg(state)::text)
  AND (sqlc.narg(q)::text IS NULL OR strpos(s.email, lower(sqlc.narg(q)::text)) > 0 OR EXISTS (
      SELECT 1 FROM contacts c
      WHERE c.workspace_id = s.workspace_id AND c.id = s.contact_id AND strpos(lower(c.name), lower(sqlc.narg(q)::text)) > 0))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (s.created_at, s.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY s.created_at DESC, s.id DESC
LIMIT @lim;

-- name: ListContactSubscriptions :many
SELECT * FROM list_subscriptions WHERE workspace_id = $1 AND contact_id = $2 ORDER BY created_at DESC, id DESC;

-- name: LockContactListSubscriptions :many
SELECT * FROM list_subscriptions
WHERE workspace_id = @workspace_id AND list_id = @list_id AND contact_id = @contact_id
ORDER BY created_at, id
FOR UPDATE;

-- name: LockEmailSubscriptions :many
SELECT * FROM list_subscriptions WHERE workspace_id = @workspace_id AND email = @email ORDER BY created_at, id FOR UPDATE;

-- name: SubscriptionContactByEmail :one
SELECT contact_id FROM list_subscriptions WHERE workspace_id = @workspace_id AND email = @email ORDER BY created_at, id LIMIT 1;

-- name: ContactUnidentified :one
SELECT (c.person_id IS NULL AND NOT EXISTS (
    SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id))::bool
FROM contacts c WHERE c.workspace_id = @workspace_id AND c.id = @id;

-- name: MoveContactSubscriptions :exec
UPDATE list_subscriptions SET contact_id = @to_contact WHERE workspace_id = @workspace_id AND contact_id = @from_contact;

-- name: AddConsentEvent :exec
INSERT INTO consent_events (id, workspace_id, subscription_id, kind, source, ip, user_agent, page_url, consent_text,
    member_id, api_key_id, import_id, broadcast_id, detail, created_at)
VALUES (@id, @workspace_id, @subscription_id, @kind, @source, sqlc.narg(ip), sqlc.narg(user_agent), sqlc.narg(page_url),
    sqlc.narg(consent_text), sqlc.narg(member_id), sqlc.narg(api_key_id), sqlc.narg(import_id), sqlc.narg(broadcast_id), @detail, @created_at);

-- name: ListConsentEvents :many
SELECT * FROM consent_events WHERE workspace_id = $1 AND subscription_id = $2 ORDER BY created_at, id;

-- name: ListConsentRecords :many
SELECT e.id AS event_id, e.kind, e.source AS event_source, e.ip, e.user_agent, e.page_url, e.consent_text, e.member_id,
       e.api_key_id, e.detail, e.created_at AS event_at,
       s.id AS subscription_id, s.contact_id, s.email, s.state, s.source, s.consent_at, s.unsubscribed_at, s.unsubscribe_reason,
       l.id AS list_id, l.name AS list_name, l.commercial
FROM consent_events e
JOIN list_subscriptions s ON s.workspace_id = e.workspace_id AND s.id = e.subscription_id
JOIN mailing_lists l ON l.workspace_id = s.workspace_id AND l.id = s.list_id
WHERE e.workspace_id = @workspace_id
  AND (sqlc.narg(list_id)::uuid IS NULL OR s.list_id = sqlc.narg(list_id)::uuid)
  AND (sqlc.narg(contact_id)::uuid IS NULL OR s.contact_id = sqlc.narg(contact_id)::uuid)
  AND (sqlc.narg(since)::timestamptz IS NULL OR e.created_at >= sqlc.narg(since)::timestamptz)
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = l.workspace_id AND iv.inbox_id = l.inbox_id AND iv.viewer_id = @viewer_id))
  AND (sqlc.narg(after_at)::timestamptz IS NULL
       OR (e.created_at, e.id) > (sqlc.narg(after_at)::timestamptz, sqlc.narg(after_id)::uuid))
ORDER BY e.created_at, e.id
LIMIT @lim;

-- name: ListIYSRecords :many
SELECT s.id, s.email, s.state, s.source, s.consent_at, s.unsubscribed_at, s.created_at
FROM list_subscriptions s
WHERE s.workspace_id = @workspace_id AND s.list_id = @list_id AND s.state IN ('subscribed', 'unsubscribed')
  AND (sqlc.narg(since)::timestamptz IS NULL
       OR (CASE WHEN s.state = 'subscribed' THEN s.consent_at ELSE s.unsubscribed_at END) >= sqlc.narg(since)::timestamptz)
  AND (sqlc.narg(after_at)::timestamptz IS NULL
       OR (s.created_at, s.id) > (sqlc.narg(after_at)::timestamptz, sqlc.narg(after_id)::uuid))
ORDER BY s.created_at, s.id
LIMIT @lim;

-- name: GetOptOutKey :one
SELECT optout_key FROM workspace_keys WHERE workspace_id = @workspace_id;

-- name: KeepOptOutKey :exec
INSERT INTO workspace_keys (workspace_id, optout_key, created_at) VALUES (@workspace_id, @optout_key, @now)
ON CONFLICT (workspace_id) DO NOTHING;

-- name: HasOptOutsWithoutKey :one
SELECT (EXISTS (SELECT 1 FROM email_optouts o WHERE o.workspace_id = @workspace_id)
   AND NOT EXISTS (SELECT 1 FROM workspace_keys k WHERE k.workspace_id = @workspace_id))::bool AS unkept;

-- name: AddOptOut :exec
INSERT INTO email_optouts (workspace_id, email_hash, reason, created_at) VALUES (@workspace_id, @email_hash, @reason, @now)
ON CONFLICT (workspace_id, email_hash) DO NOTHING;

-- name: IsOptedOut :one
SELECT EXISTS (SELECT 1 FROM email_optouts WHERE workspace_id = @workspace_id AND email_hash = @email_hash) AS opted_out;

-- name: DeleteOptOut :exec
DELETE FROM email_optouts WHERE workspace_id = @workspace_id AND email_hash = @email_hash;

-- name: ContactObjectionEmails :many
SELECT s.email FROM list_subscriptions s
JOIN mailing_lists l ON l.workspace_id = s.workspace_id AND l.id = s.list_id
WHERE s.workspace_id = @workspace_id AND s.contact_id = @contact_id AND l.kind = 'list'
  AND (s.state = 'unsubscribed' OR (s.state = 'pending' AND s.unsubscribe_reason IS NOT NULL))
UNION
SELECT x.email FROM email_suppressions x
JOIN contact_emails e ON e.workspace_id = x.workspace_id AND e.email = x.email
WHERE x.workspace_id = @workspace_id AND e.contact_id = @contact_id AND x.reason = 'complaint';

-- name: ListUnsubscribedEmails :many
SELECT email FROM list_subscriptions
WHERE workspace_id = @workspace_id AND list_id = @list_id
  AND (state = 'unsubscribed' OR (state = 'pending' AND unsubscribe_reason IS NOT NULL));

-- name: IsEmailSuppressedAnyScope :one
SELECT EXISTS (SELECT 1 FROM email_suppressions WHERE workspace_id = $1 AND email = $2 AND scope <> 'confirmations') AS suppressed;

-- Pending subscriptions that never reached consent are deleted; one that was unsubscribed before
-- and asked again goes back to unsubscribed, so its objection is not lost.
-- name: ExpirePendingSubscriptions :many
WITH expired AS (
    SELECT s.id, s.contact_id,
           (SELECT l.kind = 'status' FROM mailing_lists l WHERE l.workspace_id = s.workspace_id AND l.id = s.list_id)::bool AS status,
           (SELECT max(e.created_at) FROM consent_events e
            WHERE e.workspace_id = s.workspace_id AND e.subscription_id = s.id AND e.kind IN ('unsubscribed', 'complained')) AS objected_at
    FROM list_subscriptions s
    WHERE s.workspace_id = @workspace_id AND s.state = 'pending' AND s.updated_at < @before
    ORDER BY s.updated_at
    LIMIT @lim
), reverted AS (
    UPDATE list_subscriptions s SET state = 'unsubscribed', unsubscribed_at = x.objected_at, updated_at = @now
    FROM expired x
    WHERE s.workspace_id = @workspace_id AND s.id = x.id AND x.objected_at IS NOT NULL
    RETURNING s.id
), deleted AS (
    DELETE FROM list_subscriptions s
    USING expired x
    WHERE s.workspace_id = @workspace_id AND s.id = x.id AND x.objected_at IS NULL
    RETURNING s.id
)
SELECT x.contact_id, (x.status AND d.id IS NOT NULL)::bool AS unconfirmed_status
FROM expired x
LEFT JOIN deleted d ON d.id = x.id;

-- name: ListExpiredUnsubscribed :many
SELECT s.id, s.email, l.kind FROM list_subscriptions s
JOIN mailing_lists l ON l.workspace_id = s.workspace_id AND l.id = s.list_id
WHERE s.workspace_id = @workspace_id AND s.state = 'unsubscribed' AND s.unsubscribed_at < @before::timestamptz
ORDER BY s.unsubscribed_at
LIMIT @lim;

-- name: DeleteSubscriptions :execrows
DELETE FROM list_subscriptions WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);

-- name: SetInboxSender :one
UPDATE inboxes SET sender_legal_name = sqlc.narg(legal_name), sender_postal_address = sqlc.narg(postal_address),
    sender_registration = sqlc.narg(registration), sender_contact = sqlc.narg(contact), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;
