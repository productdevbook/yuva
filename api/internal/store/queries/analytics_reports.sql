-- name: GetAnalyticsSubscription :one
SELECT * FROM analytics_subscriptions
WHERE workspace_id = @workspace_id AND property_id = @property_id AND member_id = @member_id;

-- name: UpsertAnalyticsSubscription :one
INSERT INTO analytics_subscriptions (workspace_id, property_id, member_id, report, alerts, created_at, updated_at)
VALUES (@workspace_id, @property_id, @member_id, @report, @alerts, @now, @now)
ON CONFLICT (property_id, member_id) DO UPDATE SET report = excluded.report, alerts = excluded.alerts, updated_at = excluded.updated_at
RETURNING *;

-- name: ListAnalyticsReportWorkspaceIDs :many
-- The report and alert jobs find the live workspaces that have subscriptions; everything after it
-- is scoped by workspace.
SELECT DISTINCT s.workspace_id FROM analytics_subscriptions s
JOIN workspaces w ON w.id = s.workspace_id AND w.deleted_at IS NULL
ORDER BY s.workspace_id;

-- name: ListAnalyticsReportsDue :many
SELECT s.property_id, s.member_id, s.report, s.last_period, p.timezone
FROM analytics_subscriptions s
JOIN analytics_properties p ON p.workspace_id = s.workspace_id AND p.id = s.property_id
WHERE s.workspace_id = @workspace_id AND s.report <> 'none' AND p.deleted_at IS NULL
ORDER BY s.property_id, s.member_id;

-- name: ClaimAnalyticsReport :execrows
UPDATE analytics_subscriptions SET last_period = @period::date, last_report_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND property_id = @property_id AND member_id = @member_id
  AND report = @report AND (last_period IS NULL OR last_period < @period::date);

-- name: ListAnalyticsAlertProperties :many
SELECT DISTINCT p.id, p.timezone FROM analytics_subscriptions s
JOIN analytics_properties p ON p.workspace_id = s.workspace_id AND p.id = s.property_id
WHERE s.workspace_id = @workspace_id AND cardinality(s.alerts) > 0 AND p.deleted_at IS NULL
ORDER BY p.id;

-- name: ListAnalyticsSubscriberAddresses :many
-- Members subscribed to an alert kind (or to any report when kind is empty) who still see the
-- property: owners and admins, and agents of its linked inbox.
SELECT m.id AS member_id, pe.email, pe.locale, pe.name, s.alerts
FROM analytics_subscriptions s
JOIN members m ON m.workspace_id = s.workspace_id AND m.id = s.member_id
JOIN people pe ON pe.id = m.person_id
JOIN analytics_properties p ON p.workspace_id = s.workspace_id AND p.id = s.property_id
WHERE s.workspace_id = @workspace_id AND s.property_id = @property_id
  AND (@kind::text = '' OR @kind::text = ANY(s.alerts))
  AND ((m.role IN ('owner', 'admin') AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = p.workspace_id AND pmb.id = p.inbox_id AND pmb.owner_member_id IS NOT NULL)) OR EXISTS (
      SELECT 1 FROM member_inbox_access im WHERE im.workspace_id = m.workspace_id AND im.inbox_id = p.inbox_id AND im.member_id = m.id))

ORDER BY m.id;

-- name: CreateAnalyticsAlert :one
INSERT INTO analytics_alerts (id, workspace_id, property_id, kind, key, day, hours_from, hours_to, metric, current, baseline,
    goal_id, goal_name, referrer, text, created_at)
VALUES (@id, @workspace_id, @property_id, @kind, @key, @day, sqlc.narg(hours_from), sqlc.narg(hours_to), @metric, @current, @baseline,
    sqlc.narg(goal_id), sqlc.narg(goal_name), sqlc.narg(referrer), @text, @now)
ON CONFLICT (property_id, kind, key) DO NOTHING
RETURNING *;

-- name: SetAnalyticsAlertAnnotation :one
UPDATE analytics_alerts SET annotation_id = @annotation_id
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: ListAnalyticsAlerts :many
SELECT * FROM analytics_alerts
WHERE workspace_id = @workspace_id AND property_id = @property_id
  AND (sqlc.narg(before)::timestamptz IS NULL OR created_at < sqlc.narg(before)::timestamptz)
ORDER BY created_at DESC, id DESC
LIMIT 100;

-- name: CreateAnalyticsAlertAnnotation :one
INSERT INTO analytics_annotations (id, workspace_id, property_id, kind, starts_on, ends_on, text, alert_id, created_at, updated_at)
VALUES (@id, @workspace_id, @property_id, 'alert', @day, @day, @text, @alert_id, @now, @now)
RETURNING *;

-- name: TrimAnalyticsDailySupport :execrows
DELETE FROM analytics_daily_support WHERE ctid = ANY(ARRAY(
    SELECT t.ctid FROM analytics_daily_support t
    WHERE t.workspace_id = @workspace_id AND t.property_id = @property_id AND t.day < @before LIMIT 1000));

-- name: TrimAnalyticsSupportDays :execrows
DELETE FROM analytics_support_days WHERE ctid = ANY(ARRAY(
    SELECT t.ctid FROM analytics_support_days t
    WHERE t.workspace_id = @workspace_id AND t.property_id = @property_id AND t.day < @before LIMIT 1000));

-- name: DeleteAnalyticsSupportRollups :exec
WITH d AS (DELETE FROM analytics_daily_support WHERE analytics_daily_support.workspace_id = @workspace_id AND analytics_daily_support.property_id = @property_id)
DELETE FROM analytics_support_days WHERE analytics_support_days.workspace_id = @workspace_id AND analytics_support_days.property_id = @property_id;

-- name: GetMemberAddress :one
SELECT pe.email, pe.locale FROM members m JOIN people pe ON pe.id = m.person_id
WHERE m.workspace_id = @workspace_id AND m.id = @id;
