-- name: ListLiveTriggered :many
SELECT id, delay_minutes::integer AS delay_minutes FROM broadcasts
WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id AND kind = 'triggered' AND state IN ('active', 'paused')
  AND broadcasts.trigger ->> 'event' = @event::text;

-- name: CountLiveTriggered :one
SELECT count(*) FROM broadcasts
WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id AND kind = 'triggered'
  AND state IN ('scheduled', 'active', 'paused') AND id <> @except_id::uuid;

-- name: EnrollContact :execrows
INSERT INTO triggered_enrollments (workspace_id, broadcast_id, contact_id, triggered_at, source, created_at, updated_at)
VALUES (@workspace_id, @broadcast_id, @contact_id, @triggered_at, 'trigger', @now, @now)
ON CONFLICT DO NOTHING;

-- A row from before external ids had a time counts from the contact's creation.
-- name: BackfillIdentifiedPage :many
SELECT x.contact_id, min(coalesce(x.created_at, c.created_at))::timestamptz AS triggered_at
FROM contact_external_ids x
JOIN contacts c ON c.workspace_id = x.workspace_id AND c.id = x.contact_id
WHERE x.workspace_id = @workspace_id AND x.inbox_id = @inbox_id AND x.contact_id > @after::uuid
GROUP BY x.contact_id
HAVING min(coalesce(x.created_at, c.created_at)) >= @window_from::timestamptz AND min(coalesce(x.created_at, c.created_at)) < @window_until::timestamptz
ORDER BY x.contact_id
LIMIT @lim;

-- name: CountBackfillIdentified :one
SELECT count(*) FROM (
    SELECT x.contact_id
    FROM contact_external_ids x
    JOIN contacts c ON c.workspace_id = x.workspace_id AND c.id = x.contact_id
    WHERE x.workspace_id = @workspace_id AND x.inbox_id = @inbox_id
    GROUP BY x.contact_id
    HAVING min(coalesce(x.created_at, c.created_at)) >= @window_from::timestamptz AND min(coalesce(x.created_at, c.created_at)) < @window_until::timestamptz
) t;

-- name: InsertBackfillEnrollments :execrows
INSERT INTO triggered_enrollments (workspace_id, broadcast_id, contact_id, triggered_at, source, created_at, updated_at)
SELECT @workspace_id, @broadcast_id, unnest(@contact_ids::uuid[]), unnest(@triggered_ats::timestamptz[]), 'backfill', @now, @now
ON CONFLICT DO NOTHING;

-- name: LockDueEnrollments :many
SELECT contact_id, triggered_at, source FROM triggered_enrollments
WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id AND state = 'waiting' AND triggered_at <= @due_before
ORDER BY triggered_at, contact_id
LIMIT @lim
FOR UPDATE SKIP LOCKED;

-- name: TriggeredCandidates :many
SELECT c.id AS contact_id, c.name, c.attributes, c.blocked, c.locale,
       coalesce((SELECT e.email FROM contact_emails e WHERE e.workspace_id = c.workspace_id AND e.contact_id = c.id
                 ORDER BY e.position LIMIT 1), '')::text AS email,
       EXISTS (SELECT 1 FROM inbox_mail_optouts o WHERE o.workspace_id = c.workspace_id AND o.inbox_id = @inbox_id AND o.contact_id = c.id)::bool AS stopped,
       coalesce(sub.list_id, '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS list_id, coalesce(sub.email, '')::text AS list_email,
       broadcast_audience_matches(c.attributes, c.locale, @conditions::jsonb, @locales::text[])::bool AS matches
FROM contacts c
LEFT JOIN LATERAL (
    SELECT s.list_id, s.email FROM list_subscriptions s
    WHERE s.workspace_id = c.workspace_id AND s.contact_id = c.id AND s.list_id = ANY(@list_ids::uuid[]) AND s.state = 'subscribed'
    ORDER BY array_position(@list_ids::uuid[], s.list_id)
    LIMIT 1
) sub ON true
WHERE c.workspace_id = @workspace_id AND c.id = ANY(@contact_ids::uuid[]);

-- name: SkipEnrollments :exec
UPDATE triggered_enrollments e SET state = 'skipped', skip_reason = r.reason, updated_at = @now
FROM (SELECT unnest(@contact_ids::uuid[]) AS contact_id, unnest(@reasons::text[]) AS reason) r
WHERE e.workspace_id = @workspace_id AND e.broadcast_id = @broadcast_id AND e.contact_id = r.contact_id AND e.state = 'waiting';

-- name: QueueEnrollments :exec
UPDATE triggered_enrollments e SET state = 'queued', delivery_id = d.id, updated_at = @now
FROM broadcast_deliveries d
WHERE e.workspace_id = @workspace_id AND e.broadcast_id = @broadcast_id AND e.contact_id = ANY(@contact_ids::uuid[]) AND e.state = 'waiting'
  AND d.workspace_id = e.workspace_id AND d.broadcast_id = e.broadcast_id AND d.contact_id = e.contact_id;

-- name: NextWaitingEnrollment :one
SELECT coalesce(min(triggered_at), 'epoch')::timestamptz AS triggered_at, count(*)::bigint AS waiting FROM triggered_enrollments
WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id AND state = 'waiting';

-- name: ListDueTriggered :many
SELECT b.id FROM broadcasts b
WHERE b.workspace_id = @workspace_id AND b.kind = 'triggered' AND b.state = 'active'
  AND EXISTS (SELECT 1 FROM triggered_enrollments e
              WHERE e.workspace_id = b.workspace_id AND e.broadcast_id = b.id AND e.state = 'waiting'
                AND e.triggered_at + b.delay_minutes * interval '1 minute' <= @now::timestamptz);

-- name: EnrollmentCounts :one
SELECT count(*)::bigint AS enrolled,
       count(*) FILTER (WHERE state = 'waiting')::bigint AS waiting,
       count(*) FILTER (WHERE skip_reason = 'no_address')::bigint AS no_address,
       count(*) FILTER (WHERE skip_reason = 'blocked')::bigint AS blocked,
       count(*) FILTER (WHERE skip_reason = 'suppressed')::bigint AS suppressed,
       count(*) FILTER (WHERE skip_reason = 'opted_out')::bigint AS opted_out,
       count(*) FILTER (WHERE skip_reason = 'stopped')::bigint AS stopped,
       count(*) FILTER (WHERE skip_reason = 'not_subscribed')::bigint AS not_subscribed,
       count(*) FILTER (WHERE skip_reason = 'not_matching')::bigint AS not_matching,
       count(*) FILTER (WHERE skip_reason = 'late')::bigint AS late,
       count(*) FILTER (WHERE skip_reason = 'removed')::bigint AS removed
FROM triggered_enrollments WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id;

-- name: ListEnrollments :many
SELECT e.*, c.name AS contact_name,
       coalesce((SELECT ce.email FROM contact_emails ce WHERE ce.workspace_id = e.workspace_id AND ce.contact_id = e.contact_id
                 ORDER BY ce.position LIMIT 1), '')::text AS contact_email
FROM triggered_enrollments e
JOIN contacts c ON c.workspace_id = e.workspace_id AND c.id = e.contact_id
WHERE e.workspace_id = @workspace_id AND e.broadcast_id = @broadcast_id
  AND (sqlc.narg(state)::text IS NULL OR e.state = sqlc.narg(state)::text)
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (@oldest_first::bool AND (e.triggered_at, e.contact_id) > (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
       OR (NOT @oldest_first::bool AND (e.triggered_at, e.contact_id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid)))
ORDER BY CASE WHEN @oldest_first::bool THEN e.triggered_at END, CASE WHEN @oldest_first::bool THEN e.contact_id END,
         e.triggered_at DESC, e.contact_id DESC
LIMIT @lim;

-- name: RemoveEnrollment :execrows
UPDATE triggered_enrollments SET state = 'skipped', skip_reason = 'removed', updated_at = @now
WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id AND contact_id = @contact_id AND state = 'waiting';

-- name: EnrollmentExists :one
SELECT EXISTS (
    SELECT 1 FROM triggered_enrollments WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id AND contact_id = @contact_id
) AS found;

-- name: CancelWaitingEnrollments :exec
UPDATE triggered_enrollments SET state = 'cancelled', updated_at = @now
WHERE workspace_id = @workspace_id AND broadcast_id = @broadcast_id AND state = 'waiting';

-- A merge keeps the target's enrollment of a triggered e-mail; the source's others move to it.
-- name: MoveContactEnrollments :exec
UPDATE triggered_enrollments e SET contact_id = @to_contact
WHERE e.workspace_id = @workspace_id AND e.contact_id = @from_contact
  AND NOT EXISTS (SELECT 1 FROM triggered_enrollments t
                  WHERE t.workspace_id = e.workspace_id AND t.broadcast_id = e.broadcast_id AND t.contact_id = @to_contact);

-- name: ListLiveTriggeredSummary :many
SELECT b.id, b.name, b.state, coalesce(b.basis, '')::text AS basis,
       (SELECT count(*) FROM broadcast_deliveries d WHERE d.workspace_id = b.workspace_id AND d.broadcast_id = b.id AND d.sent_at >= @since)::bigint AS sent,
       (SELECT count(*) FROM broadcast_deliveries d WHERE d.workspace_id = b.workspace_id AND d.broadcast_id = b.id AND d.bounced_at >= @since)::bigint AS bounced,
       (SELECT count(*) FROM broadcast_deliveries d WHERE d.workspace_id = b.workspace_id AND d.broadcast_id = b.id AND d.complained_at >= @since)::bigint AS complained
FROM broadcasts b
WHERE b.workspace_id = @workspace_id AND b.kind = 'triggered' AND b.state IN ('scheduled', 'active', 'paused')
ORDER BY b.created_at, b.id;
