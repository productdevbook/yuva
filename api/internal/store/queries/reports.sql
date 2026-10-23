-- name: ReportFacts :many
-- Everything reports count that happened in [since, until), one row each, with the conversation's
-- inbox, channel kind and team and the member it is attributed to; secs is the duration of
-- first_reply and resolution. Counted as /v1/stats counts.
SELECT c.created_at::timestamptz AS at, c.id AS conversation_id, c.inbox_id, c.channel_kind, c.team_id, c.assignee_id AS member_id,
    'opened'::text AS metric, 0::bigint AS secs
FROM conversations c
WHERE c.workspace_id = @workspace_id AND c.last_activity_at >= @since::timestamptz
  AND c.created_at >= @since::timestamptz AND c.created_at < @until::timestamptz
  AND NOT c.spam AND c.trashed_at IS NULL AND c.imported_at IS NULL
UNION ALL
SELECT m.created_at, c.id, c.inbox_id, c.channel_kind, c.team_id, coalesce(m.sent_by_member_id, m.author_member_id), 'replies', 0
FROM messages m
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
WHERE m.workspace_id = @workspace_id AND m.created_at >= @since::timestamptz AND m.created_at < @until::timestamptz
  AND m.kind = 'message' AND m.direction = 'out' AND NOT m.draft AND m.imported_at IS NULL
  AND (m.author_type = 'member' OR m.sent_by_member_id IS NOT NULL) AND c.trashed_at IS NULL
UNION ALL
SELECT m.created_at, c.id, c.inbox_id, c.channel_kind, c.team_id, m.author_member_id,
    CASE WHEN m.event->>'status' = 'closed' THEN 'closed' ELSE 'reopened' END, 0
FROM messages m
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
WHERE m.workspace_id = @workspace_id AND m.created_at >= @since::timestamptz AND m.created_at < @until::timestamptz
  AND m.kind = 'event' AND m.event->>'type' = 'status_changed' AND m.imported_at IS NULL AND c.trashed_at IS NULL
  AND (m.event->>'status' = 'closed' OR (m.event->>'previous_status' = 'closed' AND m.event->>'status' <> 'closed'))
UNION ALL
SELECT f.replied_at, c.id, c.inbox_id, c.channel_kind, c.team_id, f.member_id, 'first_reply',
    floor(extract(epoch FROM f.replied_at - f.asked_at))::bigint
FROM (
    SELECT DISTINCT ON (a.conversation_id) a.conversation_id, a.asked_at, o.created_at AS replied_at,
        coalesce(o.sent_by_member_id, o.author_member_id) AS member_id
    FROM (
        SELECT i.conversation_id, min(i.created_at) AS asked_at
        FROM messages i
        WHERE i.workspace_id = @workspace_id AND i.kind = 'message' AND i.direction = 'in' AND i.imported_at IS NULL
          AND i.conversation_id IN (
              SELECT m.conversation_id
              FROM messages m
              WHERE m.workspace_id = @workspace_id AND m.created_at >= @since::timestamptz AND m.created_at < @until::timestamptz
                AND m.kind = 'message' AND m.direction = 'out' AND NOT m.draft AND m.imported_at IS NULL
                AND (m.author_type = 'member' OR m.sent_by_member_id IS NOT NULL))
        GROUP BY i.conversation_id
    ) a
    JOIN messages o ON o.workspace_id = @workspace_id AND o.conversation_id = a.conversation_id
        AND o.kind = 'message' AND o.direction = 'out' AND NOT o.draft AND o.imported_at IS NULL
        AND (o.author_type = 'member' OR o.sent_by_member_id IS NOT NULL)
        AND o.created_at > a.asked_at
    ORDER BY a.conversation_id, o.created_at, o.id
) f
JOIN conversations c ON c.workspace_id = @workspace_id AND c.id = f.conversation_id
WHERE f.replied_at >= @since::timestamptz AND f.replied_at < @until::timestamptz AND c.trashed_at IS NULL
UNION ALL
SELECT x.created_at, c.id, c.inbox_id, c.channel_kind, c.team_id, x.author_member_id, 'resolution',
    floor(extract(epoch FROM x.created_at - f.started_at))::bigint
FROM messages x
JOIN conversations c ON c.workspace_id = x.workspace_id AND c.id = x.conversation_id
JOIN LATERAL (
    SELECT max(p.created_at) AS closed_at
    FROM messages p
    WHERE p.workspace_id = @workspace_id AND p.conversation_id = x.conversation_id
      AND p.kind = 'event' AND p.event->>'type' = 'status_changed' AND p.event->>'status' = 'closed'
      AND p.imported_at IS NULL AND p.created_at < x.created_at
) pc ON true
JOIN LATERAL (
    SELECT min(i.created_at) AS started_at
    FROM messages i
    WHERE i.workspace_id = @workspace_id AND i.conversation_id = x.conversation_id
      AND i.kind = 'message' AND i.direction = 'in' AND NOT i.draft AND i.imported_at IS NULL
      AND (pc.closed_at IS NULL OR i.created_at > pc.closed_at) AND i.created_at <= x.created_at
) f ON f.started_at IS NOT NULL
WHERE x.workspace_id = @workspace_id AND x.created_at >= @since::timestamptz AND x.created_at < @until::timestamptz
  AND x.kind = 'event' AND x.event->>'type' = 'status_changed' AND x.event->>'status' = 'closed'
  AND x.imported_at IS NULL AND c.trashed_at IS NULL
UNION ALL
SELECT c.rated_at, c.id, c.inbox_id, c.channel_kind, c.team_id, c.assignee_id, 'rating_' || c.rating, 0
FROM conversations c
WHERE c.workspace_id = @workspace_id AND c.rated_at >= @since::timestamptz AND c.rated_at < @until::timestamptz
  AND NOT c.spam AND c.kind <> 'feedback' AND c.trashed_at IS NULL
UNION ALL
SELECT o.at, o.conversation_id, o.inbox_id, c.channel_kind, o.team_id, o.assignee_id, 'sla_' || o.target || '_' || o.outcome, 0
FROM sla_outcomes o
JOIN conversations c ON c.workspace_id = o.workspace_id AND c.id = o.conversation_id
WHERE o.workspace_id = @workspace_id AND o.at >= @since::timestamptz AND o.at < @until::timestamptz AND c.trashed_at IS NULL;

-- name: ListReportLabels :many
SELECT conversation_id, label_id FROM conversation_labels
WHERE workspace_id = @workspace_id AND conversation_id = ANY (@conversation_ids::uuid[]);

-- name: ReportEarliestActivity :one
-- The first instant anything reports count happened: a native conversation, a message that was not
-- imported (a reply in an imported thread counts) or an SLA outcome.
SELECT least(
    coalesce((SELECT min(c.created_at) FROM conversations c WHERE c.workspace_id = @workspace_id AND c.imported_at IS NULL), @now::timestamptz),
    coalesce((SELECT min(m.created_at) FROM messages m WHERE m.workspace_id = @workspace_id AND m.imported_at IS NULL), @now::timestamptz),
    coalesce((SELECT min(o.at) FROM sla_outcomes o WHERE o.workspace_id = @workspace_id), @now::timestamptz)
)::timestamptz AS at;

-- name: GetReportRollupState :one
SELECT * FROM report_rollup_state WHERE workspace_id = @workspace_id;

-- name: InitReportRollupState :exec
INSERT INTO report_rollup_state (workspace_id, built_until, recomputed_on, updated_at) VALUES (@workspace_id, @built_until, @recomputed_on::date, @now)
ON CONFLICT (workspace_id) DO NOTHING;

-- name: LockReportRollupState :one
SELECT * FROM report_rollup_state WHERE workspace_id = @workspace_id FOR UPDATE;

-- name: SetReportBuiltUntil :exec
UPDATE report_rollup_state SET built_until = @built_until, updated_at = @now WHERE workspace_id = @workspace_id;

-- name: SetReportRecompute :exec
UPDATE report_rollup_state SET recompute_at = sqlc.narg(recompute_at), recomputed_on = sqlc.narg(recomputed_on), updated_at = @now
WHERE workspace_id = @workspace_id;

-- name: DeleteReportRollups :exec
DELETE FROM report_rollups WHERE workspace_id = @workspace_id AND bucket >= @since::timestamptz AND bucket < @until::timestamptz;

-- name: ReportRows :many
-- The rollups before built (and before until) plus the tail rows counted live, summed per local
-- period, group, metric and bin over the inboxes the viewer sees. Label rows answer only when
-- by_label is set; deleted inboxes and labels drop out, deleted teams and members become none.
WITH r AS (
    SELECT rr.bucket, rr.inbox_id, rr.channel_kind, rr.team_id, rr.member_id, rr.label_id, rr.metric, rr.bin, rr.n
    FROM report_rollups rr
    WHERE rr.workspace_id = @workspace_id AND rr.bucket >= @since::timestamptz AND rr.bucket < @until::timestamptz
      AND rr.bucket < @built::timestamptz
    UNION ALL
    SELECT t.bucket, t.inbox_id, NULLIF(t.channel_kind, ''), NULLIF(t.team_id, '')::uuid, NULLIF(t.member_id, '')::uuid,
        NULLIF(t.label_id, '')::uuid, t.metric, t.bin, t.n
    FROM (
        SELECT unnest(@tail_bucket::timestamptz[]) AS bucket, unnest(@tail_inbox::uuid[]) AS inbox_id,
            unnest(@tail_channel::text[]) AS channel_kind, unnest(@tail_team::text[]) AS team_id,
            unnest(@tail_member::text[]) AS member_id, unnest(@tail_label::text[]) AS label_id,
            unnest(@tail_metric::text[]) AS metric, unnest(@tail_bin::smallint[]) AS bin, unnest(@tail_n::integer[]) AS n
    ) t
)
SELECT date_trunc(@grain::text, r.bucket AT TIME ZONE @timezone::text)::date AS period,
    (CASE @group_by::text
        WHEN 'inbox' THEN r.inbox_id::text
        WHEN 'channel' THEN coalesce(r.channel_kind, '')
        WHEN 'label' THEN r.label_id::text
        WHEN 'team' THEN coalesce(t.id::text, '')
        WHEN 'member' THEN coalesce(mb.id::text, '')
        ELSE '' END)::text AS grp,
    r.metric::text AS metric, r.bin::smallint AS bin, sum(r.n)::bigint AS n
FROM r
JOIN inboxes i ON i.workspace_id = @workspace_id AND i.id = r.inbox_id
LEFT JOIN teams t ON t.workspace_id = @workspace_id AND t.id = r.team_id
LEFT JOIN members mb ON mb.workspace_id = @workspace_id AND mb.id = r.member_id
LEFT JOIN labels l ON l.workspace_id = @workspace_id AND l.id = r.label_id
WHERE (r.label_id IS NOT NULL) = @by_label::bool AND (r.label_id IS NULL OR l.id IS NOT NULL)
  AND r.metric = ANY (@metrics::text[])
  AND (sqlc.narg(label_id)::uuid IS NULL OR r.label_id = sqlc.narg(label_id)::uuid)
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR r.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (sqlc.narg(channel_kind)::text IS NULL OR r.channel_kind = sqlc.narg(channel_kind)::text)
  AND (sqlc.narg(team_id)::uuid IS NULL OR t.id = sqlc.narg(team_id)::uuid)
  AND (sqlc.narg(member_id)::uuid IS NULL OR mb.id = sqlc.narg(member_id)::uuid)
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = @workspace_id AND iv.inbox_id = r.inbox_id AND iv.viewer_id = @viewer_id::uuid))
GROUP BY 1, 2, 3, 4;

-- name: ReportNames :many
SELECT id, name::text AS name FROM inboxes WHERE inboxes.workspace_id = @workspace_id AND inboxes.id = ANY (@ids::uuid[])
UNION ALL
SELECT id, name FROM labels WHERE labels.workspace_id = @workspace_id AND labels.id = ANY (@ids::uuid[])
UNION ALL
SELECT id, name FROM teams WHERE teams.workspace_id = @workspace_id AND teams.id = ANY (@ids::uuid[])
UNION ALL
SELECT m.id, coalesce(nullif(p.name, ''), p.email) FROM members m JOIN people p ON p.id = m.person_id
WHERE m.workspace_id = @workspace_id AND m.id = ANY (@ids::uuid[]);
