-- name: CreateStatusMonitor :one
INSERT INTO status_monitors (id, workspace_id, page_id, component_id, kind, name, token_hash, token_prefix, interval_seconds, grace_seconds,
    failure_state, failure_threshold, recovery_threshold, on_failure, paused, state, due_at, target, timeout_seconds, http_method,
    http_header_names, http_headers, http_body, status_min, status_max, keyword, follow_redirects, cert_warn_days, check_at,
    changed_at, created_at, updated_at)
VALUES (@id, @workspace_id, @page_id, @component_id, @kind, @name, sqlc.narg(token_hash), sqlc.narg(token_prefix), sqlc.narg(interval_seconds), sqlc.narg(grace_seconds),
    @failure_state, @failure_threshold, @recovery_threshold, @on_failure, @paused, 'pending', sqlc.narg(due_at), sqlc.narg(target),
    sqlc.narg(timeout_seconds), sqlc.narg(http_method), sqlc.arg(http_header_names)::text[], sqlc.narg(http_headers), sqlc.narg(http_body),
    sqlc.narg(status_min), sqlc.narg(status_max), sqlc.narg(keyword), @follow_redirects, sqlc.narg(cert_warn_days), sqlc.narg(check_at),
    @now, @now, @now)
RETURNING *;

-- name: CountStatusMonitors :one
SELECT count(*)::bigint FROM status_monitors WHERE workspace_id = @workspace_id;

-- name: GetStatusMonitor :one
SELECT * FROM status_monitors WHERE workspace_id = @workspace_id AND page_id = @page_id AND id = @id;

-- name: GetStatusMonitorByID :one
SELECT * FROM status_monitors WHERE workspace_id = @workspace_id AND id = @id;

-- name: LockStatusMonitor :one
SELECT * FROM status_monitors WHERE workspace_id = @workspace_id AND id = @id FOR UPDATE;

-- name: ListStatusMonitors :many
SELECT * FROM status_monitors WHERE workspace_id = @workspace_id AND page_id = @page_id ORDER BY created_at, id;

-- name: FindStatusMonitorByToken :one
-- The documented exception: a heartbeat or push finds its monitor by its token before the workspace is known.
SELECT workspace_id, id, kind FROM status_monitors WHERE token_hash = @token_hash;

-- name: UpdateStatusMonitorSettings :one
UPDATE status_monitors SET name = @name, component_id = @component_id, failure_state = @failure_state, failure_threshold = @failure_threshold,
    recovery_threshold = @recovery_threshold, on_failure = @on_failure, interval_seconds = sqlc.narg(interval_seconds),
    grace_seconds = sqlc.narg(grace_seconds), due_at = sqlc.narg(due_at), due_extended = @due_extended, target = sqlc.narg(target),
    timeout_seconds = sqlc.narg(timeout_seconds), http_method = sqlc.narg(http_method), http_header_names = sqlc.arg(http_header_names)::text[],
    http_headers = sqlc.narg(http_headers), http_body = sqlc.narg(http_body), status_min = sqlc.narg(status_min), status_max = sqlc.narg(status_max),
    keyword = sqlc.narg(keyword), follow_redirects = @follow_redirects, cert_warn_days = sqlc.narg(cert_warn_days),
    cert_expires_at = sqlc.narg(cert_expires_at), check_at = sqlc.narg(check_at), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: SetStatusMonitorPaused :one
UPDATE status_monitors SET paused = @paused, state = @state, down_state = sqlc.narg(down_state), consecutive_failures = 0,
    consecutive_successes = 0, due_at = sqlc.narg(due_at), due_extended = false, check_at = sqlc.narg(check_at), incident_id = NULL,
    changed_at = @changed_at, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: SetStatusMonitorToken :one
UPDATE status_monitors SET token_hash = @token_hash, token_prefix = @token_prefix, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: SetStatusMonitorResult :one
UPDATE status_monitors SET state = @state, down_state = sqlc.narg(down_state), consecutive_failures = @consecutive_failures,
    consecutive_successes = @consecutive_successes, last_result_at = @at::timestamptz, last_message = sqlc.narg(last_message),
    due_at = sqlc.narg(due_at), due_extended = @due_extended, incident_id = sqlc.narg(incident_id), changed_at = @changed_at, updated_at = @at
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: ExtendStatusMonitorDue :exec
UPDATE status_monitors SET due_at = @due_at::timestamptz, due_extended = true
WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteStatusMonitor :execrows
DELETE FROM status_monitors WHERE workspace_id = @workspace_id AND page_id = @page_id AND id = @id;

-- name: ListDueStatusMonitors :many
-- The documented exception: the sweep finds the heartbeats whose beat is overdue in every workspace at once.
SELECT workspace_id, id FROM status_monitors
WHERE due_at IS NOT NULL AND NOT paused AND due_at <= @now::timestamptz
ORDER BY due_at
LIMIT 500;

-- name: FailingStatusMonitors :many
-- A monitor is overridden, and holds nothing, once a member or key set its component's state after the monitor's last change.
SELECT m.id, m.component_id, m.down_state::text AS state, m.changed_at FROM status_monitors m
WHERE m.workspace_id = @workspace_id AND m.component_id = ANY(@component_ids::uuid[]) AND m.state = 'down' AND NOT m.paused
  AND NOT EXISTS (
    SELECT 1 FROM status_component_states s
    WHERE s.workspace_id = m.workspace_id AND s.component_id = m.component_id AND s.source IN ('member', 'api_key') AND s.started_at > m.changed_at)
ORDER BY m.changed_at, m.id;

-- name: CreateStatusCheck :exec
INSERT INTO status_checks (id, workspace_id, monitor_id, at, ok, source, state, message, error, duration_ms)
VALUES (@id, @workspace_id, @monitor_id, @at, @ok, @source, sqlc.narg(state), sqlc.narg(message), sqlc.narg(error), sqlc.narg(duration_ms));

-- name: ListStatusChecks :many
SELECT * FROM status_checks
WHERE workspace_id = @workspace_id AND monitor_id = @monitor_id AND NOT pending
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (at, id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY at DESC, id DESC
LIMIT @lim;

-- name: DeleteOldStatusChecks :execrows
DELETE FROM status_checks WHERE id IN (
    SELECT c.id FROM status_checks c
    WHERE c.workspace_id = @workspace_id AND (c.at < @before OR (c.pending AND c.at < @stale_before::timestamptz)) LIMIT 10000);

-- name: AnyDueStatusChecks :one
-- The documented exception: the sweep looks for due outside checks in every workspace at once.
SELECT EXISTS (SELECT 1 FROM status_monitors WHERE check_at IS NOT NULL AND NOT paused AND check_at < @until::timestamptz)::boolean;

-- name: ClaimDueStatusChecks :many
-- The documented exception: the sweep claims the outside checks due before until in every workspace
-- at once, oldest first while their timeouts fit in budget seconds, and moves each to its next slot,
-- interval apart at an offset its id spreads over the interval.
WITH due AS (
    SELECT c.workspace_id, c.id, c.check_at, c.timeout_seconds
    FROM status_monitors c
    WHERE c.check_at IS NOT NULL AND NOT c.paused AND c.check_at < @until::timestamptz
    ORDER BY c.check_at, c.id
    LIMIT @budget::int
    FOR UPDATE SKIP LOCKED
), fit AS (
    SELECT due.workspace_id, due.id, due.check_at,
        sum(coalesce(due.timeout_seconds, 30)) OVER (ORDER BY due.check_at, due.id) AS load
    FROM due
)
UPDATE status_monitors m SET check_at = to_timestamp(
    (floor((date_part('epoch', @until::timestamptz) - mod(abs(hashtext(m.id::text)::bigint), m.interval_seconds)) / m.interval_seconds) + 1)
        * m.interval_seconds + mod(abs(hashtext(m.id::text)::bigint), m.interval_seconds))
FROM fit
WHERE m.workspace_id = fit.workspace_id AND m.id = fit.id AND fit.load <= @budget::int
RETURNING m.workspace_id, m.id, fit.check_at::timestamptz AS due;

-- name: CreatePendingStatusCheck :exec
INSERT INTO status_checks (id, workspace_id, monitor_id, at, ok, source, error, duration_ms, status_code, round_id, pending)
VALUES (@id, @workspace_id, @monitor_id, @at, @ok, @source, sqlc.narg(error), sqlc.narg(duration_ms), sqlc.narg(status_code), @round_id, true);

-- name: ListRoundStatusChecks :many
-- The documented exception: a round is judged over the checks of every workspace it ran.
SELECT workspace_id, id, monitor_id, ok, source, error, round_dropped FROM status_checks WHERE round_id = @round_id AND pending ORDER BY at, id;

-- name: DecideStatusRound :exec
-- The documented exception: a round's judgement is stored on its checks in every workspace.
UPDATE status_checks SET round_dropped = @dropped WHERE round_id = @round_id AND pending AND round_dropped IS NULL;

-- name: FinishStatusCheck :execrows
UPDATE status_checks SET pending = false, state = sqlc.narg(state)
WHERE workspace_id = @workspace_id AND id = @id AND pending;

-- name: DeleteStatusCheck :exec
DELETE FROM status_checks WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeletePendingStatusChecks :exec
DELETE FROM status_checks WHERE workspace_id = @workspace_id AND monitor_id = @monitor_id AND pending;

-- name: SetStatusMonitorCert :exec
UPDATE status_monitors SET cert_expires_at = sqlc.narg(cert_expires_at)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: StatusCheckBuckets :many
-- A bucket starts at a multiple of bucket_seconds since the epoch; p50 and p95 are -1 without a measured success.
SELECT to_timestamp(floor(date_part('epoch', at) / sqlc.arg(bucket_seconds)::int) * sqlc.arg(bucket_seconds)::int)::timestamptz AS start,
    count(*)::int AS checks, count(*) FILTER (WHERE NOT ok)::int AS failures,
    coalesce((percentile_cont(0.5) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE ok AND duration_ms IS NOT NULL))::int, -1)::int AS p50,
    coalesce((percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE ok AND duration_ms IS NOT NULL))::int, -1)::int AS p95
FROM status_checks
WHERE workspace_id = @workspace_id AND monitor_id = @monitor_id AND NOT pending AND at >= @since::timestamptz AND at < @until::timestamptz
GROUP BY 1
ORDER BY 1;

-- name: StatusCheckTotals :one
SELECT count(*)::int AS checks, count(*) FILTER (WHERE NOT ok)::int AS failures,
    coalesce((percentile_cont(0.5) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE ok AND duration_ms IS NOT NULL))::int, -1)::int AS p50,
    coalesce((percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE ok AND duration_ms IS NOT NULL))::int, -1)::int AS p95
FROM status_checks
WHERE workspace_id = @workspace_id AND monitor_id = @monitor_id AND NOT pending AND at >= @since::timestamptz AND at < @until::timestamptz;

-- name: StatusResponseComponents :many
-- interval_seconds is the longest interval of the component's http and tcp monitors and of its push
-- monitors with a measured success since @since; a push monitor without one does not count.
SELECT m.component_id, max(coalesce(m.interval_seconds, 60))::int AS interval_seconds FROM status_monitors m
WHERE m.workspace_id = @workspace_id AND m.page_id = @page_id AND m.component_id = ANY(@component_ids::uuid[])
  AND (m.kind IN ('http', 'tcp') OR (m.kind = 'push' AND EXISTS (
    SELECT 1 FROM status_checks c WHERE c.workspace_id = m.workspace_id AND c.monitor_id = m.id
      AND NOT c.pending AND c.ok AND c.duration_ms IS NOT NULL AND c.at >= @since::timestamptz)))
GROUP BY m.component_id;

-- name: StatusResponseBuckets :many
-- Step n covers since + n * step_seconds; only measured successes of http, tcp and push monitors count.
SELECT m.component_id, floor(date_part('epoch', c.at - @since::timestamptz) / sqlc.arg(step_seconds)::int)::int AS step,
    (percentile_cont(0.5) WITHIN GROUP (ORDER BY c.duration_ms))::int AS p50
FROM status_checks c
JOIN status_monitors m ON m.workspace_id = c.workspace_id AND m.id = c.monitor_id
WHERE c.workspace_id = @workspace_id AND m.page_id = @page_id AND m.kind IN ('http', 'tcp', 'push') AND m.component_id = ANY(@component_ids::uuid[])
  AND NOT c.pending AND c.ok AND c.duration_ms IS NOT NULL AND c.at >= @since::timestamptz AND c.at < @until::timestamptz
GROUP BY 1, 2;

-- name: StatusResponseTotals :many
SELECT m.component_id, (percentile_cont(0.5) WITHIN GROUP (ORDER BY c.duration_ms))::int AS p50,
    (percentile_cont(0.95) WITHIN GROUP (ORDER BY c.duration_ms))::int AS p95
FROM status_checks c
JOIN status_monitors m ON m.workspace_id = c.workspace_id AND m.id = c.monitor_id
WHERE c.workspace_id = @workspace_id AND m.page_id = @page_id AND m.kind IN ('http', 'tcp', 'push') AND m.component_id = ANY(@component_ids::uuid[])
  AND NOT c.pending AND c.ok AND c.duration_ms IS NOT NULL AND c.at >= @since::timestamptz AND c.at < @until::timestamptz
GROUP BY 1;
