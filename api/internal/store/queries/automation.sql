-- name: LockAutomationRules :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_automation_rules:' || sqlc.arg(workspace_id)::uuid::text, 0));

-- name: ListAutomationRules :many
SELECT * FROM automation_rules WHERE workspace_id = @workspace_id ORDER BY position, created_at, id;

-- name: GetAutomationRule :one
SELECT * FROM automation_rules WHERE workspace_id = @workspace_id AND id = @id;

-- name: LockAutomationRule :one
SELECT * FROM automation_rules WHERE workspace_id = @workspace_id AND id = @id FOR UPDATE;

-- name: ShareLockAutomationRule :one
SELECT * FROM automation_rules WHERE workspace_id = @workspace_id AND id = @id FOR SHARE;

-- name: CountAutomationRules :one
SELECT count(*) FROM automation_rules WHERE workspace_id = @workspace_id;

-- name: CreateAutomationRule :one
INSERT INTO automation_rules (id, workspace_id, inbox_id, name, enabled, position, trigger, idle_minutes, conditions, actions,
    saved_by_kind, saved_by_member_id, saved_by_api_key_id, created_at, updated_at)
VALUES (@id, @workspace_id, sqlc.narg(inbox_id), @name, @enabled,
    (SELECT coalesce(max(position) + 1, 0) FROM automation_rules WHERE workspace_id = @workspace_id),
    @trigger, sqlc.narg(idle_minutes), @conditions, @actions,
    @saved_by_kind, sqlc.narg(saved_by_member_id), sqlc.narg(saved_by_api_key_id), @now, @now)
RETURNING *;

-- name: UpdateAutomationRule :one
UPDATE automation_rules SET inbox_id = sqlc.narg(inbox_id), name = @name, enabled = @enabled, trigger = @trigger,
    idle_minutes = sqlc.narg(idle_minutes), conditions = @conditions, actions = @actions, saved_by_kind = @saved_by_kind,
    saved_by_member_id = sqlc.narg(saved_by_member_id), saved_by_api_key_id = sqlc.narg(saved_by_api_key_id), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: SetAutomationRulePosition :exec
UPDATE automation_rules SET position = @position WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteAutomationRule :execrows
DELETE FROM automation_rules WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListEnabledAutomationTriggers :many
SELECT DISTINCT trigger FROM automation_rules WHERE workspace_id = @workspace_id AND enabled;

-- name: ListEnabledAutomationRules :many
SELECT * FROM automation_rules
WHERE workspace_id = @workspace_id AND enabled AND trigger = @trigger
  AND (sqlc.narg(idle_minutes)::int IS NULL OR idle_minutes = sqlc.narg(idle_minutes)::int)
ORDER BY position, created_at, id;

-- name: ListIdleAutomationMinutes :many
SELECT DISTINCT idle_minutes::int AS minutes FROM automation_rules
WHERE workspace_id = @workspace_id AND enabled AND trigger = 'idle'
ORDER BY 1;

-- The lookback bounds the scan; a conversation whose quiet period began before it is not picked up.
-- name: ListIdleConversations :many
SELECT id, last_activity_at FROM conversations
WHERE workspace_id = @workspace_id AND status IN ('open', 'pending') AND NOT spam AND trashed_at IS NULL
  AND last_activity_at <= @idle_before AND last_activity_at > @not_before
ORDER BY last_activity_at, id
LIMIT @lim;

-- name: InsertAutomationRun :one
INSERT INTO automation_runs (id, workspace_id, rule_id, conversation_id, trigger, trigger_key, outcome, error, detail, actions, created_at)
VALUES (@id, @workspace_id, @rule_id, @conversation_id, @trigger, @trigger_key, @outcome, sqlc.narg(error), sqlc.narg(detail), @actions, @now)
ON CONFLICT (workspace_id, rule_id, trigger_key) DO NOTHING
RETURNING id;

-- name: SetAutomationRunActions :exec
UPDATE automation_runs SET actions = @actions WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListAutomationRuns :many
SELECT r.* FROM automation_runs r
JOIN conversations c ON c.workspace_id = r.workspace_id AND c.id = r.conversation_id
WHERE r.workspace_id = @workspace_id AND r.rule_id = @rule_id AND c.trashed_at IS NULL
  AND (sqlc.narg(outcome)::text IS NULL OR r.outcome = sqlc.narg(outcome)::text)
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = c.workspace_id AND pmb.id = c.inbox_id AND pmb.owner_member_id IS NOT NULL)) OR c.inbox_id = ANY(@inbox_ids::uuid[]))

  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (r.created_at, r.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY r.created_at DESC, r.id DESC
LIMIT @lim;

-- name: DeleteOldAutomationRuns :execrows
DELETE FROM automation_runs r
WHERE r.workspace_id = sqlc.arg(workspace_id)::uuid AND r.id IN (
    SELECT o.id FROM automation_runs o
    WHERE o.workspace_id = sqlc.arg(workspace_id)::uuid AND o.created_at < sqlc.arg(before)::timestamptz
    LIMIT sqlc.arg(lim)::int);

-- name: CountRuleReplies :one
SELECT count(*) FILTER (WHERE created_at > @hour_since)::int AS hour, count(*)::int AS day
FROM messages
WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND rule IS NOT NULL
  AND kind = 'message' AND direction = 'out' AND created_at > @day_since;

-- name: AutomationRunExists :one
SELECT EXISTS (SELECT 1 FROM automation_runs WHERE workspace_id = @workspace_id AND rule_id = @rule_id AND trigger_key = @trigger_key) AS found;

-- name: SetAutomationRunOutcome :exec
UPDATE automation_runs SET outcome = @outcome WHERE workspace_id = @workspace_id AND id = @id;

-- name: SetConversationSnoozeRule :exec
UPDATE conversations SET snooze_rule_id = sqlc.narg(rule_id) WHERE workspace_id = @workspace_id AND id = @id;

-- name: LatestContactMessage :one
SELECT m.id, m.body, coalesce(e.auto, false)::bool AS auto, coalesce(e.dmarc, '')::text AS dmarc
FROM messages m
LEFT JOIN message_emails e ON e.workspace_id = m.workspace_id AND e.message_id = m.id
WHERE m.workspace_id = sqlc.arg(workspace_id)::uuid AND m.conversation_id = sqlc.arg(conversation_id)::uuid
  AND m.kind = 'message' AND m.direction = 'in' AND NOT m.draft
ORDER BY m.created_at DESC, m.id DESC
LIMIT 1;

-- name: ContactMessageOf :one
SELECT m.id, m.body, coalesce(e.auto, false)::bool AS auto, coalesce(e.dmarc, '')::text AS dmarc
FROM messages m
LEFT JOIN message_emails e ON e.workspace_id = m.workspace_id AND e.message_id = m.id
WHERE m.workspace_id = sqlc.arg(workspace_id)::uuid AND m.conversation_id = sqlc.arg(conversation_id)::uuid AND m.id = sqlc.arg(id)::uuid
  AND m.kind = 'message' AND m.direction = 'in';

-- name: ShareLockWebhookEndpoint :one
SELECT * FROM webhook_endpoints WHERE workspace_id = @workspace_id AND id = @id FOR SHARE;

-- name: TouchConversationByRule :exec
UPDATE conversations SET
    last_message_at = CASE WHEN @is_message::bool THEN greatest(coalesce(last_message_at, @now::timestamptz), @now::timestamptz) ELSE last_message_at END,
    updated_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListCustomFieldKeys :many
SELECT key FROM custom_fields WHERE workspace_id = @workspace_id AND object = 'conversation' AND key = ANY(@keys::text[]);

-- name: ListWebhookEndpointsByID :many
SELECT * FROM webhook_endpoints WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);

-- name: ListCannedRepliesByID :many
SELECT id FROM canned_replies WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);
