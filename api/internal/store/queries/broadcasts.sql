-- name: CreateBroadcast :one
INSERT INTO broadcasts (id, workspace_id, inbox_id, channel_id, name, subject, preheader, design_id, audience,
                        click_tracking, utm, utm_campaign, created_by, pace_per_hour, pace_per_day, pace_per_month,
                        changelog_entry_id, created_at, updated_at)
VALUES (@id, @workspace_id, @inbox_id, sqlc.narg(channel_id), @name, @subject, @preheader, sqlc.narg(design_id), @audience,
        @click_tracking, @utm, @utm_campaign, sqlc.narg(created_by), sqlc.narg(pace_per_hour), sqlc.narg(pace_per_day),
        sqlc.narg(pace_per_month), sqlc.narg(changelog_entry_id), @now, @now)
RETURNING *;

-- name: GetBroadcast :one
SELECT * FROM broadcasts WHERE workspace_id = $1 AND id = $2;

-- name: LockBroadcast :one
SELECT * FROM broadcasts WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: SaveBroadcast :one
UPDATE broadcasts SET channel_id = sqlc.narg(channel_id), name = @name, subject = @subject, preheader = @preheader,
    design_id = sqlc.narg(design_id), design_version = sqlc.narg(design_version), document = sqlc.narg(document),
    audience = @audience, click_tracking = @click_tracking, utm = @utm, utm_campaign = @utm_campaign, state = @state, pause_reason = sqlc.narg(pause_reason),
    state_detail = sqlc.narg(state_detail), scheduled_at = sqlc.narg(scheduled_at), waiting_until = sqlc.narg(waiting_until),
    started_at = sqlc.narg(started_at), resumed_at = sqlc.narg(resumed_at), finished_at = sqlc.narg(finished_at),
    last_test_at = sqlc.narg(last_test_at), snapshot_after = sqlc.narg(snapshot_after), snapshot_done = @snapshot_done,
    sender_failing_since = sqlc.narg(sender_failing_since), scheduled_by = sqlc.narg(scheduled_by),
    scheduled_by_key = sqlc.narg(scheduled_by_key), updated_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: SetBroadcastPace :one
UPDATE broadcasts SET pace_per_hour = sqlc.narg(per_hour), pace_per_day = sqlc.narg(per_day),
    pace_per_month = sqlc.narg(per_month), pace_next_at = sqlc.narg(next_at), updated_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- One mail of a paced broadcast may go out now: takes the slot and sets the next one from the pace
-- stored on the row, so concurrent senders and pace changes cannot double a slot.
-- name: ClaimBroadcastPaceSlot :execrows
UPDATE broadcasts
SET pace_next_at = (@now::timestamptz) + make_interval(secs => greatest(
        coalesce(3600.0 / pace_per_hour, 0), coalesce(86400.0 / pace_per_day, 0),
        coalesce(2592000.0 / pace_per_month, 0))::float8)
WHERE workspace_id = @workspace_id AND id = @id AND state = 'sending'
  AND (pace_next_at IS NULL OR pace_next_at <= @now::timestamptz);

-- name: MarkBroadcastTested :exec
UPDATE broadcasts SET last_test_at = @now::timestamptz WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteBroadcast :execrows
DELETE FROM broadcasts WHERE workspace_id = $1 AND id = $2;

-- name: ListBroadcasts :many
SELECT * FROM broadcasts b
WHERE b.workspace_id = @workspace_id
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR b.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (sqlc.narg(states)::text[] IS NULL OR b.state = ANY(sqlc.narg(states)::text[]))
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = b.workspace_id AND pmb.id = b.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = b.workspace_id AND iv.inbox_id = b.inbox_id AND iv.viewer_id = @viewer_id))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (b.created_at, b.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY b.created_at DESC, b.id DESC
LIMIT @lim;

-- name: CountBroadcasts :one
SELECT count(*) FROM broadcasts WHERE workspace_id = $1;

-- Counts are merged by adding each key, so concurrent batches, replies and complaints never lose one.
-- name: AddBroadcastCounts :exec
UPDATE broadcasts b SET counts = coalesce((
    SELECT jsonb_object_agg(k, coalesce((b.counts ->> k)::bigint, 0) + coalesce((@delta::jsonb ->> k)::bigint, 0))
    FROM (SELECT jsonb_object_keys(b.counts) UNION SELECT jsonb_object_keys(@delta::jsonb)) AS keys (k)
), '{}'::jsonb)
WHERE b.workspace_id = @workspace_id AND b.id = @id;

-- name: SetBroadcastCounts :exec
UPDATE broadcasts SET counts = counts || @counts::jsonb WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListSendingBroadcastsOfChannel :many
SELECT * FROM broadcasts
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND state = 'sending'
ORDER BY started_at, id;

-- name: ListRunningBroadcasts :many
SELECT * FROM broadcasts
WHERE workspace_id = @workspace_id AND state IN ('scheduled', 'preparing', 'sending')
ORDER BY created_at, id;

-- name: SetBroadcastsWaiting :exec
UPDATE broadcasts SET waiting_until = sqlc.narg(until)
WHERE workspace_id = @workspace_id AND state = 'sending';

-- name: ListLatestBroadcasts :many
SELECT * FROM broadcasts WHERE workspace_id = @workspace_id ORDER BY created_at DESC, id DESC LIMIT @lim;

-- name: BroadcastUsesList :one
SELECT EXISTS (
    SELECT 1 FROM broadcasts
    WHERE workspace_id = @workspace_id AND state IN ('scheduled', 'preparing', 'sending', 'paused')
      AND audience -> 'list_ids' @> jsonb_build_array(@list_id::text)
) AS used;

-- name: ListBroadcastSenderDomains :many
SELECT DISTINCT lower(split_part(coalesce(e.from_address, e.address), '@', 2))::text AS domain
FROM broadcasts b
JOIN email_channels e ON e.workspace_id = b.workspace_id AND e.channel_id = b.channel_id
WHERE b.workspace_id = @workspace_id AND b.started_at > @since;

-- One mail per contact: the subscription of the first list in the broadcast's order. A contact
-- matches when every condition and one of the languages match; suppressed and blocked rows are
-- returned so they can be counted.
-- name: BroadcastAudiencePage :many
SELECT DISTINCT ON (s.contact_id)
    s.contact_id, s.email, s.list_id, c.name, c.attributes, c.blocked,
    EXISTS (SELECT 1 FROM email_suppressions x WHERE x.workspace_id = s.workspace_id AND x.email = s.email AND x.scope <> 'confirmations')::bool AS suppressed,
    (count(*) OVER (PARTITION BY s.contact_id))::bigint AS subscriptions,
    ((cardinality(@locales::text[]) = 0 OR EXISTS (
        SELECT 1 FROM unnest(@locales::text[]) AS l (tag)
        WHERE lower(coalesce(c.locale, '')) = lower(l.tag) OR lower(coalesce(c.locale, '')) LIKE lower(l.tag) || '-%'))
     AND NOT EXISTS (
        SELECT 1 FROM jsonb_to_recordset(@conditions::jsonb) AS k (attribute text, op text, value text, "values" text[])
        WHERE NOT coalesce(CASE k.op
            WHEN 'exists' THEN c.attributes -> k.attribute IS NOT NULL
            WHEN 'not_exists' THEN c.attributes -> k.attribute IS NULL
            WHEN 'eq' THEN c.attributes ->> k.attribute = k.value
            WHEN 'neq' THEN (c.attributes ->> k.attribute) IS DISTINCT FROM k.value
            WHEN 'contains' THEN strpos(lower(c.attributes ->> k.attribute), lower(k.value)) > 0
            WHEN 'in' THEN c.attributes ->> k.attribute = ANY(k."values")
            WHEN 'gt' THEN CASE
                WHEN (c.attributes ->> k.attribute) ~ '^-?[0-9]{1,30}(\.[0-9]{1,30})?$' AND k.value ~ '^-?[0-9]{1,30}(\.[0-9]{1,30})?$'
                THEN (c.attributes ->> k.attribute)::numeric > k.value::numeric
                ELSE (c.attributes ->> k.attribute) > k.value END
            WHEN 'lt' THEN CASE
                WHEN (c.attributes ->> k.attribute) ~ '^-?[0-9]{1,30}(\.[0-9]{1,30})?$' AND k.value ~ '^-?[0-9]{1,30}(\.[0-9]{1,30})?$'
                THEN (c.attributes ->> k.attribute)::numeric < k.value::numeric
                ELSE (c.attributes ->> k.attribute) < k.value END
            ELSE false END, false)))::bool AS matches
FROM list_subscriptions s
JOIN contacts c ON c.workspace_id = s.workspace_id AND c.id = s.contact_id
WHERE s.workspace_id = @workspace_id AND s.list_id = ANY(@list_ids::uuid[]) AND s.state = 'subscribed'
  AND s.contact_id > @after::uuid
ORDER BY s.contact_id, array_position(@list_ids::uuid[], s.list_id)
LIMIT @lim;

-- name: BroadcastAudienceLeftOut :one
SELECT count(DISTINCT s.contact_id) FILTER (WHERE s.state = 'unsubscribed')::bigint AS unsubscribed,
       count(DISTINCT s.contact_id) FILTER (WHERE s.state = 'pending')::bigint AS pending
FROM list_subscriptions s
WHERE s.workspace_id = @workspace_id AND s.list_id = ANY(@list_ids::uuid[]) AND s.state <> 'subscribed'
  AND NOT EXISTS (
      SELECT 1 FROM list_subscriptions o
      WHERE o.workspace_id = s.workspace_id AND o.contact_id = s.contact_id AND o.list_id = ANY(@list_ids::uuid[])
        AND o.state = 'subscribed');

-- name: ListOptOutHashes :many
SELECT email_hash FROM email_optouts WHERE workspace_id = @workspace_id AND email_hash = ANY(@hashes::bytea[]);

-- name: InsertBroadcastDeliveries :execrows
INSERT INTO broadcast_deliveries (id, workspace_id, broadcast_id, contact_id, list_id, email, email_token, header_message_id, created_at)
SELECT unnest(@ids::uuid[]), @workspace_id, @broadcast_id, unnest(@contact_ids::uuid[]), unnest(@list_ids::uuid[]),
       unnest(@emails::text[]), unnest(@tokens::text[]), unnest(@headers::text[]), @now
ON CONFLICT DO NOTHING;

-- name: DueBroadcastDeliveries :many
SELECT * FROM broadcast_deliveries
WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id AND state = 'queued'
  AND (next_attempt_at IS NULL OR next_attempt_at <= @now::timestamptz)
ORDER BY next_attempt_at NULLS FIRST, id
LIMIT @lim;

-- name: QueuedBroadcastDeliveries :one
SELECT count(*)::bigint AS queued,
       coalesce(CASE WHEN bool_or(next_attempt_at IS NULL) THEN NULL ELSE min(next_attempt_at) END, 'epoch')::timestamptz AS next_attempt
FROM broadcast_deliveries
WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id AND state = 'queued';

-- name: SaveBroadcastDelivery :exec
UPDATE broadcast_deliveries SET state = @state, skip_reason = sqlc.narg(skip_reason), attempts = @attempts,
    next_attempt_at = sqlc.narg(next_attempt_at), error = sqlc.narg(error), sent_at = sqlc.narg(sent_at),
    bounced_at = sqlc.narg(bounced_at), relay_message_id = coalesce(sqlc.narg(relay_message_id), relay_message_id)
WHERE workspace_id = @workspace_id AND id = @id
  AND (state = 'queued' OR (state = 'skipped' AND skip_reason = 'cancelled' AND @state::text <> 'queued'));

-- name: SkipQueuedDeliveries :execrows
UPDATE broadcast_deliveries SET state = 'skipped', skip_reason = 'cancelled', next_attempt_at = NULL
WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id AND state = 'queued';

-- name: RecountBroadcast :one
SELECT count(*)::bigint AS recipients,
       count(*) FILTER (WHERE state = 'queued')::bigint AS queued,
       count(*) FILTER (WHERE sent_at IS NOT NULL)::bigint AS sent,
       count(*) FILTER (WHERE state = 'failed')::bigint AS failed,
       count(*) FILTER (WHERE state = 'bounced')::bigint AS bounced,
       count(*) FILTER (WHERE complained_at IS NOT NULL)::bigint AS complained,
       count(*) FILTER (WHERE unsubscribed_at IS NOT NULL)::bigint AS unsubscribed,
       count(*) FILTER (WHERE replied_at IS NOT NULL)::bigint AS replied,
       count(*) FILTER (WHERE auto_replied_at IS NOT NULL)::bigint AS auto_replied,
       count(*) FILTER (WHERE clicked_at IS NOT NULL)::bigint AS clicked,
       count(*) FILTER (WHERE skip_reason = 'suppressed')::bigint AS skipped_suppressed,
       count(*) FILTER (WHERE skip_reason = 'opted_out')::bigint AS skipped_opted_out,
       count(*) FILTER (WHERE skip_reason = 'unsubscribed')::bigint AS skipped_unsubscribed,
       count(*) FILTER (WHERE skip_reason = 'cancelled')::bigint AS skipped_cancelled
FROM broadcast_deliveries WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id;

-- SES reports name only our Message-ID, so the delivery is found before the workspace is known
-- (see "Hosting for others later" in docs/architecture.md).
-- name: FindBroadcastDeliveryAnyWorkspace :one
SELECT d.* FROM broadcast_deliveries d
JOIN workspaces w ON w.id = d.workspace_id
WHERE d.header_message_id = @header_message_id AND w.deleted_at IS NULL;

-- name: FindBroadcastDeliveryByHeader :one
SELECT * FROM broadcast_deliveries
WHERE workspace_id = @workspace_id AND (header_message_id = @header_message_id OR relay_message_id = @header_message_id)
LIMIT 1;

-- name: FindBroadcastDeliveryByTokens :one
SELECT d.* FROM broadcast_deliveries d
JOIN broadcasts b ON b.workspace_id = d.workspace_id AND b.id = d.broadcast_id
WHERE d.workspace_id = @workspace_id AND b.inbox_id = @inbox_id
  AND (d.email_token = ANY(@tokens::text[]) OR d.relay_message_id = ANY(@ids::text[]))
ORDER BY (d.contact_id = @contact_id) DESC, d.created_at DESC
LIMIT 1;

-- name: GetBroadcastDelivery :one
SELECT * FROM broadcast_deliveries WHERE workspace_id = $1 AND id = $2;

-- name: MarkDeliveryBounced :execrows
UPDATE broadcast_deliveries SET state = 'bounced', bounced_at = @now::timestamptz, next_attempt_at = NULL,
    error = sqlc.narg(error)
WHERE workspace_id = @workspace_id AND id = @id AND state IN ('queued', 'sent');

-- name: MarkDeliveryComplained :execrows
UPDATE broadcast_deliveries SET state = CASE WHEN state = 'bounced' THEN state ELSE 'complained' END, complained_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id AND complained_at IS NULL;

-- name: MarkDeliveryReplied :execrows
UPDATE broadcast_deliveries SET replied_at = @now::timestamptz, conversation_id = coalesce(conversation_id, sqlc.narg(conversation_id))
WHERE workspace_id = @workspace_id AND id = @id AND replied_at IS NULL;

-- name: MarkDeliveryAutoReplied :execrows
UPDATE broadcast_deliveries SET auto_replied_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id AND auto_replied_at IS NULL;

-- name: MarkDeliveryUnsubscribed :execrows
UPDATE broadcast_deliveries SET unsubscribed_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id AND contact_id = @contact_id AND unsubscribed_at IS NULL;

-- name: MarkDeliveryClicked :execrows
UPDATE broadcast_deliveries SET clicked_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id AND clicked_at IS NULL;

-- name: ListBroadcastDeliveries :many
SELECT d.*, c.name AS contact_name FROM broadcast_deliveries d
JOIN contacts c ON c.workspace_id = d.workspace_id AND c.id = d.contact_id
WHERE d.workspace_id = @workspace_id AND d.broadcast_id = @broadcast_id
  AND (sqlc.narg(state)::text IS NULL OR d.state = sqlc.narg(state)::text)
  AND (sqlc.narg(replied)::bool IS NULL OR sqlc.narg(replied)::bool = (d.replied_at IS NOT NULL))
  AND (sqlc.narg(q)::text IS NULL OR strpos(d.email, lower(sqlc.narg(q)::text)) > 0
       OR strpos(lower(c.name), lower(sqlc.narg(q)::text)) > 0)
  AND (sqlc.narg(after)::uuid IS NULL OR d.id > sqlc.narg(after)::uuid)
ORDER BY d.id
LIMIT @lim;

-- Since the broadcast started or was last resumed: mails tried (sent or refused), bounces,
-- mails sent and complaints.
-- name: BroadcastRates :one
SELECT count(*) FILTER (WHERE sent_at >= @since OR (state IN ('bounced', 'failed') AND coalesce(bounced_at, sent_at, @since) >= @since))::bigint AS tried,
       count(*) FILTER (WHERE state = 'bounced' AND bounced_at >= @since)::bigint AS bounced,
       count(*) FILTER (WHERE sent_at >= @since)::bigint AS sent,
       count(*) FILTER (WHERE complained_at >= @since)::bigint AS complained
FROM broadcast_deliveries WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id;

-- name: BroadcastTimeline :many
SELECT (CASE WHEN e.t < @hourly_until::timestamptz THEN date_trunc('hour', e.t AT TIME ZONE 'UTC')
             ELSE date_trunc('day', e.t AT TIME ZONE 'UTC') END AT TIME ZONE 'UTC')::timestamptz AS at,
       count(*) FILTER (WHERE e.kind = 'sent')::bigint AS sent,
       count(*) FILTER (WHERE e.kind = 'bounced')::bigint AS bounced,
       count(*) FILTER (WHERE e.kind = 'complained')::bigint AS complained,
       count(*) FILTER (WHERE e.kind = 'unsubscribed')::bigint AS unsubscribed,
       count(*) FILTER (WHERE e.kind = 'replied')::bigint AS replied,
       count(*) FILTER (WHERE e.kind = 'clicked')::bigint AS clicked
FROM (
    SELECT d.sent_at AS t, 'sent'::text AS kind FROM broadcast_deliveries d
    WHERE d.workspace_id = @workspace_id AND d.broadcast_id = @broadcast_id AND d.sent_at IS NOT NULL
    UNION ALL
    SELECT d.bounced_at AS t, 'bounced'::text AS kind FROM broadcast_deliveries d
    WHERE d.workspace_id = @workspace_id AND d.broadcast_id = @broadcast_id AND d.bounced_at IS NOT NULL
    UNION ALL
    SELECT d.complained_at AS t, 'complained'::text AS kind FROM broadcast_deliveries d
    WHERE d.workspace_id = @workspace_id AND d.broadcast_id = @broadcast_id AND d.complained_at IS NOT NULL
    UNION ALL
    SELECT d.unsubscribed_at AS t, 'unsubscribed'::text AS kind FROM broadcast_deliveries d
    WHERE d.workspace_id = @workspace_id AND d.broadcast_id = @broadcast_id AND d.unsubscribed_at IS NOT NULL
    UNION ALL
    SELECT d.replied_at AS t, 'replied'::text AS kind FROM broadcast_deliveries d
    WHERE d.workspace_id = @workspace_id AND d.broadcast_id = @broadcast_id AND d.replied_at IS NOT NULL
    UNION ALL
    SELECT d.clicked_at AS t, 'clicked'::text AS kind FROM broadcast_deliveries d
    WHERE d.workspace_id = @workspace_id AND d.broadcast_id = @broadcast_id AND d.clicked_at IS NOT NULL
) e
GROUP BY 1
ORDER BY 1;

-- name: ListBroadcastReplies :many
SELECT d.conversation_id::uuid AS conversation_id, d.contact_id, c.name AS contact_name, d.replied_at::timestamptz AS replied_at
FROM broadcast_deliveries d
JOIN contacts c ON c.workspace_id = d.workspace_id AND c.id = d.contact_id
WHERE d.workspace_id = @workspace_id AND d.broadcast_id = @broadcast_id AND d.replied_at IS NOT NULL AND d.conversation_id IS NOT NULL
ORDER BY d.replied_at, d.id
LIMIT 50;

-- name: InsertBroadcastLink :exec
INSERT INTO broadcast_links (workspace_id, broadcast_id, position, url) VALUES (@workspace_id, @broadcast_id, @position, @url)
ON CONFLICT (workspace_id, broadcast_id, position) DO UPDATE SET url = excluded.url, clicks = 0;

-- name: DeleteBroadcastLinks :exec
DELETE FROM broadcast_links WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id;

-- name: ListBroadcastLinks :many
SELECT * FROM broadcast_links WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id ORDER BY clicks DESC, position;

-- name: GetBroadcastLink :one
SELECT * FROM broadcast_links WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id AND position = @position;

-- name: RecordBroadcastClick :execrows
INSERT INTO broadcast_clicks (workspace_id, delivery_id, position, first_at) VALUES (@workspace_id, @delivery_id, @position, @now)
ON CONFLICT DO NOTHING;

-- name: CountRecentBroadcastClicks :one
SELECT count(*) FROM broadcast_clicks WHERE workspace_id = @workspace_id AND delivery_id = @delivery_id AND first_at > @since;

-- name: CountBroadcastLinkClick :exec
UPDATE broadcast_links SET clicks = clicks + 1 WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id AND position = @position;

-- name: AddSendingDay :exec
INSERT INTO broadcast_sending_days (workspace_id, day, sent, bounced, complained)
VALUES (@workspace_id, @day, @sent, @bounced, @complained)
ON CONFLICT (workspace_id, day) DO UPDATE SET
    sent = broadcast_sending_days.sent + excluded.sent,
    bounced = broadcast_sending_days.bounced + excluded.bounced,
    complained = broadcast_sending_days.complained + excluded.complained;

-- name: ListSendingDays :many
SELECT * FROM broadcast_sending_days WHERE workspace_id = @workspace_id AND day >= @since ORDER BY day;

-- name: DeleteOldSendingDays :exec
DELETE FROM broadcast_sending_days WHERE workspace_id = @workspace_id AND day < @before;

-- name: DeleteOldBroadcastDeliveries :execrows
DELETE FROM broadcast_deliveries WHERE (workspace_id, id) IN (
    SELECT d.workspace_id, d.id FROM broadcast_deliveries d
    JOIN broadcasts b ON b.workspace_id = d.workspace_id AND b.id = d.broadcast_id
    WHERE d.workspace_id = @workspace_id AND b.finished_at < @before
    LIMIT @lim);

-- name: SetWorkspaceBroadcastsStatus :one
UPDATE workspaces SET broadcasts_status = @status, broadcasts_status_reason = sqlc.narg(reason), broadcasts_status_at = @now::timestamptz
WHERE id = @id
RETURNING *;

-- name: SetWorkspaceBroadcastDailyLimit :one
UPDATE workspaces SET broadcast_daily_limit = sqlc.narg(daily_limit) WHERE id = @id RETURNING *;

-- name: SetWorkspaceBroadcastsByKeys :one
UPDATE workspaces SET broadcasts_by_keys = @by_keys WHERE id = @id RETURNING *;

-- name: AddBroadcastUsage :exec
INSERT INTO usage_counters (workspace_id, month, broadcast_recipients)
VALUES (@workspace_id, @month, @recipients)
ON CONFLICT (workspace_id, month) DO UPDATE SET broadcast_recipients = usage_counters.broadcast_recipients + excluded.broadcast_recipients;

-- name: SetConversationBroadcast :one
UPDATE conversations SET broadcast_id = @broadcast_id WHERE workspace_id = @workspace_id AND id = @id RETURNING *;

-- A complaint about a broadcast stops broadcasts only; it never narrows a suppression of all mail.
-- name: SuppressEmailScoped :exec
INSERT INTO email_suppressions (workspace_id, email, reason, detail, scope, created_at)
VALUES (@workspace_id, @email, @reason, @detail, @scope, @now)
ON CONFLICT (workspace_id, email) DO UPDATE SET reason = excluded.reason, detail = excluded.detail,
    scope = excluded.scope, created_at = excluded.created_at
WHERE email_suppressions.scope <> 'all' OR excluded.scope = 'all';

-- name: IsContactSubscribed :one
SELECT EXISTS (
    SELECT 1 FROM list_subscriptions
    WHERE workspace_id = @workspace_id AND contact_id = @contact_id AND email = @email
      AND list_id = ANY(@list_ids::uuid[]) AND state = 'subscribed'
) AS subscribed;

-- name: CountWorkspaceMembersByIDs :many
SELECT m.id, p.email FROM members m JOIN people p ON p.id = m.person_id
WHERE m.workspace_id = @workspace_id AND m.id = ANY(@ids::uuid[]);

-- name: CountBroadcastReplyConversations :one
SELECT count(*) FROM conversations
WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id AND created_at >= @since AND created_at < @until AND trashed_at IS NULL;
