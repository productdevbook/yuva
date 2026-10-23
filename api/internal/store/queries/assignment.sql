-- name: LockAssignment :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_assign:' || sqlc.arg(workspace_id)::uuid::text, 0));

-- name: AssignmentEnabled :one
SELECT (EXISTS (SELECT 1 FROM inboxes i WHERE i.workspace_id = @workspace_id AND i.assignment_mode <> 'off')
     OR EXISTS (SELECT 1 FROM teams t WHERE t.workspace_id = @workspace_id AND t.assignment_mode <> 'off'))::bool AS enabled;

-- name: ListAssignCandidates :many
SELECT m.id, t.last_at,
    (SELECT count(*) FROM conversations c
     WHERE c.workspace_id = m.workspace_id AND c.assignee_id = m.id AND c.status = 'open' AND NOT c.spam AND c.trashed_at IS NULL)::int AS open_count
FROM members m
JOIN people p ON p.id = m.person_id
LEFT JOIN assignment_turns t ON t.workspace_id = m.workspace_id AND t.pool_id = @pool_id AND t.member_id = m.id
WHERE m.workspace_id = @workspace_id AND p.availability = 'auto'
  AND EXISTS (SELECT 1 FROM member_inbox_access a WHERE a.workspace_id = m.workspace_id AND a.inbox_id = @inbox_id AND a.member_id = m.id)
  AND EXISTS (SELECT 1 FROM realtime_connections rc WHERE rc.workspace_id = m.workspace_id AND rc.member_id = m.id AND rc.seen_at > @fresh_after)
  AND (sqlc.narg(team_id)::uuid IS NULL OR EXISTS (
      SELECT 1 FROM team_members tm WHERE tm.workspace_id = m.workspace_id AND tm.team_id = sqlc.narg(team_id)::uuid AND tm.member_id = m.id))
ORDER BY m.id;

-- name: TakeAssignmentTurn :exec
-- Turns of a pool never share an instant, so round robin never ties on a clock that did not move.
INSERT INTO assignment_turns (workspace_id, pool_id, member_id, last_at)
SELECT @workspace_id, @pool_id, @member_id, greatest(@now::timestamptz, max(t.last_at) + interval '1 microsecond')
FROM assignment_turns t WHERE t.workspace_id = @workspace_id AND t.pool_id = @pool_id
ON CONFLICT (workspace_id, pool_id, member_id) DO UPDATE SET last_at = excluded.last_at;

-- name: MemberAway :one
SELECT (p.availability = 'away')::bool AS away FROM members m JOIN people p ON p.id = m.person_id
WHERE m.workspace_id = @workspace_id AND m.id = @id;

-- name: StartAssignWaiting :one
UPDATE conversations SET assign_waiting_since = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id AND assign_waiting_since IS NULL
RETURNING *;

-- name: StopAssignWaiting :exec
UPDATE conversations SET assign_waiting_since = NULL
WHERE workspace_id = @workspace_id AND id = @id AND assign_waiting_since IS NOT NULL;

-- name: ListAssignWaiting :many
SELECT id, inbox_id, team_id, assign_waiting_since FROM conversations
WHERE workspace_id = @workspace_id AND assign_waiting_since IS NOT NULL
  AND (assign_waiting_since, id) > (@after_since::timestamptz, @after_id::uuid)
ORDER BY assign_waiting_since, id
LIMIT @lim;

-- name: HasAssignWaiting :one
SELECT EXISTS (SELECT 1 FROM conversations WHERE workspace_id = @workspace_id AND assign_waiting_since IS NOT NULL) AS waiting;

-- name: AssignWaitingReady :one
SELECT (EXISTS (SELECT 1 FROM conversations c WHERE c.workspace_id = @workspace_id AND c.assign_waiting_since IS NOT NULL)
    AND EXISTS (
        SELECT 1 FROM realtime_connections rc
        JOIN members m ON m.workspace_id = rc.workspace_id AND m.id = rc.member_id
        JOIN people p ON p.id = m.person_id
        WHERE rc.workspace_id = @workspace_id AND rc.seen_at > @fresh_after AND p.availability = 'auto'))::bool AS ready;

-- name: ListAppliedRunActions :many
SELECT actions FROM automation_runs
WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND trigger_key = @trigger_key AND outcome = 'applied';

-- name: DeleteAssignmentTurns :exec
DELETE FROM assignment_turns WHERE workspace_id = @workspace_id AND pool_id = @pool_id;
