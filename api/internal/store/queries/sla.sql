-- name: LockSLAPolicies :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_sla_policies:' || sqlc.arg(workspace_id)::uuid::text, 0));

-- name: ListSLAPolicies :many
SELECT * FROM sla_policies WHERE workspace_id = @workspace_id ORDER BY position, created_at, id;

-- name: GetSLAPolicy :one
SELECT * FROM sla_policies WHERE workspace_id = @workspace_id AND id = @id;

-- name: WorkspaceHasSLAPolicies :one
SELECT EXISTS (SELECT 1 FROM sla_policies WHERE workspace_id = @workspace_id);

-- name: CountSLAPolicies :one
SELECT count(*) FROM sla_policies WHERE workspace_id = @workspace_id;

-- name: CreateSLAPolicy :one
INSERT INTO sla_policies (id, workspace_id, name, position, enabled, inbox_ids, channel_kinds, label_ids, priorities,
    first_reply_minutes, next_reply_minutes, resolution_minutes, warn_minutes, created_at, updated_at)
VALUES (@id, @workspace_id, @name,
    (SELECT coalesce(max(position) + 1, 0) FROM sla_policies WHERE workspace_id = @workspace_id),
    @enabled, @inbox_ids::uuid[], @channel_kinds::text[], @label_ids::uuid[], @priorities::text[],
    sqlc.narg(first_reply_minutes), sqlc.narg(next_reply_minutes), sqlc.narg(resolution_minutes), @warn_minutes, @now, @now)
RETURNING *;

-- name: UpdateSLAPolicy :one
UPDATE sla_policies SET name = @name, enabled = @enabled, inbox_ids = @inbox_ids::uuid[], channel_kinds = @channel_kinds::text[],
    label_ids = @label_ids::uuid[], priorities = @priorities::text[], first_reply_minutes = sqlc.narg(first_reply_minutes),
    next_reply_minutes = sqlc.narg(next_reply_minutes), resolution_minutes = sqlc.narg(resolution_minutes),
    warn_minutes = @warn_minutes, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: LockSLAPolicy :one
SELECT * FROM sla_policies WHERE workspace_id = @workspace_id AND id = @id FOR UPDATE;

-- name: SetSLAPolicyPosition :exec
UPDATE sla_policies SET position = @position WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListSLAConversationsOfPolicy :many
SELECT conversation_id FROM conversation_sla
WHERE workspace_id = @workspace_id AND policy_id = @policy_id AND dropped_at IS NULL
ORDER BY conversation_id;

-- name: DeleteSLAPolicy :execrows
DELETE FROM sla_policies WHERE workspace_id = @workspace_id AND id = @id;

-- name: StripInboxFromSLAPolicies :exec
-- A policy whose only inbox is deleted would match every inbox, so it is turned off instead.
UPDATE sla_policies SET inbox_ids = array_remove(inbox_ids, @inbox_id::uuid),
    enabled = enabled AND cardinality(array_remove(inbox_ids, @inbox_id::uuid)) > 0, updated_at = @now
WHERE workspace_id = @workspace_id AND @inbox_id::uuid = ANY (inbox_ids);

-- name: StripLabelFromSLAPolicies :exec
-- A policy whose only label is deleted would match every conversation, so it is turned off instead.
UPDATE sla_policies SET label_ids = array_remove(label_ids, @label_id::uuid),
    enabled = enabled AND cardinality(array_remove(label_ids, @label_id::uuid)) > 0, updated_at = @now
WHERE workspace_id = @workspace_id AND @label_id::uuid = ANY (label_ids);

-- name: LockConversationSLA :one
SELECT * FROM conversation_sla WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id FOR UPDATE;

-- name: ListConversationSLAs :many
SELECT * FROM conversation_sla WHERE workspace_id = @workspace_id AND conversation_id = ANY (@conversation_ids::uuid[]);

-- name: SaveConversationSLA :exec
INSERT INTO conversation_sla (workspace_id, conversation_id, policy_id, match_key, dropped_at, warn_minutes, awaiting_reply, paused_at, check_at,
    first_reply_target_s, first_reply_started_at, first_reply_due_at, first_reply_remaining_s, first_reply_met_at, first_reply_breached_at, first_reply_warned_at,
    next_reply_target_s, next_reply_started_at, next_reply_due_at, next_reply_remaining_s, next_reply_met_at, next_reply_breached_at, next_reply_warned_at,
    resolution_target_s, resolution_started_at, resolution_due_at, resolution_remaining_s, resolution_met_at, resolution_breached_at, resolution_warned_at,
    created_at, updated_at)
VALUES (@workspace_id, @conversation_id, sqlc.narg(policy_id), @match_key, sqlc.narg(dropped_at), @warn_minutes, @awaiting_reply, sqlc.narg(paused_at), sqlc.narg(check_at),
    sqlc.narg(first_reply_target_s), sqlc.narg(first_reply_started_at), sqlc.narg(first_reply_due_at), sqlc.narg(first_reply_remaining_s),
    sqlc.narg(first_reply_met_at), sqlc.narg(first_reply_breached_at), sqlc.narg(first_reply_warned_at),
    sqlc.narg(next_reply_target_s), sqlc.narg(next_reply_started_at), sqlc.narg(next_reply_due_at), sqlc.narg(next_reply_remaining_s),
    sqlc.narg(next_reply_met_at), sqlc.narg(next_reply_breached_at), sqlc.narg(next_reply_warned_at),
    sqlc.narg(resolution_target_s), sqlc.narg(resolution_started_at), sqlc.narg(resolution_due_at), sqlc.narg(resolution_remaining_s),
    sqlc.narg(resolution_met_at), sqlc.narg(resolution_breached_at), sqlc.narg(resolution_warned_at),
    @now, @now)
ON CONFLICT (workspace_id, conversation_id) DO UPDATE SET
    policy_id = excluded.policy_id, match_key = excluded.match_key, dropped_at = excluded.dropped_at, warn_minutes = excluded.warn_minutes,
    awaiting_reply = excluded.awaiting_reply, paused_at = excluded.paused_at, check_at = excluded.check_at,
    first_reply_target_s = excluded.first_reply_target_s, first_reply_started_at = excluded.first_reply_started_at,
    first_reply_due_at = excluded.first_reply_due_at, first_reply_remaining_s = excluded.first_reply_remaining_s,
    first_reply_met_at = excluded.first_reply_met_at, first_reply_breached_at = excluded.first_reply_breached_at,
    first_reply_warned_at = excluded.first_reply_warned_at,
    next_reply_target_s = excluded.next_reply_target_s, next_reply_started_at = excluded.next_reply_started_at,
    next_reply_due_at = excluded.next_reply_due_at, next_reply_remaining_s = excluded.next_reply_remaining_s,
    next_reply_met_at = excluded.next_reply_met_at, next_reply_breached_at = excluded.next_reply_breached_at,
    next_reply_warned_at = excluded.next_reply_warned_at,
    resolution_target_s = excluded.resolution_target_s, resolution_started_at = excluded.resolution_started_at,
    resolution_due_at = excluded.resolution_due_at, resolution_remaining_s = excluded.resolution_remaining_s,
    resolution_met_at = excluded.resolution_met_at, resolution_breached_at = excluded.resolution_breached_at,
    resolution_warned_at = excluded.resolution_warned_at,
    updated_at = excluded.updated_at;

-- name: SetConversationSLADue :exec
UPDATE conversations SET sla_due_at = sqlc.narg(due_at)
WHERE workspace_id = @workspace_id AND id = @id AND sla_due_at IS DISTINCT FROM sqlc.narg(due_at);

-- name: MarkSLABreached :one
-- Acts only while the target still runs to the due time the check was queued for, so a check left
-- behind by a pause, a reply or a new policy does nothing, and each breach happens once.
UPDATE conversation_sla SET
    first_reply_breached_at = CASE WHEN @target::text = 'first_reply' THEN @now ELSE first_reply_breached_at END,
    next_reply_breached_at = CASE WHEN @target::text = 'next_reply' THEN @now ELSE next_reply_breached_at END,
    resolution_breached_at = CASE WHEN @target::text = 'resolution' THEN @now ELSE resolution_breached_at END,
    updated_at = @now
WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND dropped_at IS NULL AND CASE @target::text
    WHEN 'first_reply' THEN first_reply_due_at = @due_at::timestamptz AND first_reply_breached_at IS NULL AND first_reply_met_at IS NULL
    WHEN 'next_reply' THEN next_reply_due_at = @due_at::timestamptz AND next_reply_breached_at IS NULL AND next_reply_met_at IS NULL
    WHEN 'resolution' THEN resolution_due_at = @due_at::timestamptz AND resolution_breached_at IS NULL AND resolution_met_at IS NULL
    ELSE false END
RETURNING *;

-- name: MarkSLAWarned :one
UPDATE conversation_sla SET
    first_reply_warned_at = CASE WHEN @target::text = 'first_reply' THEN @now ELSE first_reply_warned_at END,
    next_reply_warned_at = CASE WHEN @target::text = 'next_reply' THEN @now ELSE next_reply_warned_at END,
    resolution_warned_at = CASE WHEN @target::text = 'resolution' THEN @now ELSE resolution_warned_at END,
    updated_at = @now
WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND dropped_at IS NULL AND CASE @target::text
    WHEN 'first_reply' THEN first_reply_due_at = @due_at::timestamptz AND first_reply_warned_at IS NULL AND first_reply_breached_at IS NULL AND first_reply_met_at IS NULL
    WHEN 'next_reply' THEN next_reply_due_at = @due_at::timestamptz AND next_reply_warned_at IS NULL AND next_reply_breached_at IS NULL AND next_reply_met_at IS NULL
    WHEN 'resolution' THEN resolution_due_at = @due_at::timestamptz AND resolution_warned_at IS NULL AND resolution_breached_at IS NULL AND resolution_met_at IS NULL
    ELSE false END
RETURNING *;

-- name: InsertSLAOutcome :execrows
-- A target started at one instant has one outcome, however often its row is rebuilt or checked.
INSERT INTO sla_outcomes (id, workspace_id, conversation_id, inbox_id, policy_id, target, started_at, outcome, at, assignee_id, team_id)
VALUES (@id, @workspace_id, @conversation_id, @inbox_id, sqlc.narg(policy_id), @target, @started_at, @outcome, @at, sqlc.narg(assignee_id), sqlc.narg(team_id))
ON CONFLICT (workspace_id, conversation_id, target, started_at) DO NOTHING;

-- name: ListSLAChecksDue :many
SELECT * FROM conversation_sla
WHERE workspace_id = @workspace_id AND check_at IS NOT NULL AND check_at <= @now::timestamptz AND dropped_at IS NULL
ORDER BY check_at
LIMIT @lim;

-- name: ListSLAMessages :many
-- The conversation's public messages, oldest first: from the contact (in) and replies (out) by
-- members and bots, without drafts, imports and automatic replies.
SELECT created_at, direction FROM messages
WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id
  AND kind = 'message' AND NOT draft AND imported_at IS NULL
  AND (direction = 'in' OR author_type <> 'system')
ORDER BY created_at, id;

-- name: ClaimDueSnoozed :many
SELECT id, snooze_rule_id FROM conversations
WHERE workspace_id = @workspace_id AND status = 'snoozed' AND snooze_until <= @now::timestamptz AND trashed_at IS NULL AND NOT spam
ORDER BY snooze_until, id
LIMIT @lim
FOR UPDATE SKIP LOCKED;

-- name: StatsSLA :many
SELECT o.inbox_id, o.assignee_id, o.outcome, count(*) AS n
FROM sla_outcomes o
JOIN conversations c ON c.workspace_id = o.workspace_id AND c.id = o.conversation_id
WHERE o.workspace_id = @workspace_id AND o.at >= @since AND o.at <= @until
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR o.inbox_id = sqlc.narg(inbox_id)::uuid) AND c.trashed_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM inboxes mi WHERE mi.workspace_id = o.workspace_id AND mi.id = o.inbox_id AND mi.purpose = 'mail')
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = o.workspace_id AND iv.inbox_id = o.inbox_id AND iv.viewer_id = @viewer_id::uuid))
GROUP BY 1, 2, 3
ORDER BY 1, 2, 3;

-- name: LockInboxesShared :many
SELECT id, purpose FROM inboxes WHERE workspace_id = @workspace_id AND id = ANY (@ids::uuid[]) FOR SHARE;
