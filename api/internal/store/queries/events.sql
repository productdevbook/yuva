-- name: LockEventStream :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_events:' || sqlc.arg(workspace_id)::uuid::text, 0));

-- name: InsertEvent :one
INSERT INTO events (workspace_id, type, inbox_id, conversation_id, payload)
VALUES (@workspace_id, @type, sqlc.narg(inbox_id), sqlc.narg(conversation_id), @payload)
RETURNING id;

-- name: NotifyEvent :exec
SELECT pg_notify(sqlc.arg(channel)::text, sqlc.arg(workspace_id)::uuid::text || ':' || sqlc.arg(id)::bigint::text);

-- name: GetEvent :one
SELECT * FROM events WHERE workspace_id = $1 AND id = $2;

-- name: EventExists :one
SELECT EXISTS (SELECT 1 FROM events WHERE workspace_id = $1 AND id = $2) AS found;

-- name: LastEventID :one
SELECT coalesce(max(id), 0)::bigint AS id FROM events WHERE workspace_id = $1;

-- name: ListEventsAfter :many
SELECT * FROM events WHERE workspace_id = @workspace_id AND id > @after ORDER BY id LIMIT @lim;

-- name: DeleteEventsBefore :execrows
DELETE FROM events WHERE workspace_id = @workspace_id AND created_at < @before;

-- name: NotifySignal :exec
SELECT pg_notify(sqlc.arg(channel)::text, sqlc.arg(payload)::text);

-- name: FirstEventID :one
SELECT coalesce(min(id), 0)::bigint AS id FROM events WHERE workspace_id = $1;

-- name: DeleteConversationEvents :exec
DELETE FROM events WHERE workspace_id = @workspace_id AND conversation_id = ANY(@conversation_ids::uuid[])
  AND (NOT @keep_trash::bool OR type NOT IN ('conversation.trashed', 'conversation.restored'));

-- name: DeleteEventsMentioning :exec
DELETE FROM events WHERE workspace_id = @workspace_id AND strpos(payload::text, @needle::text) > 0;

-- name: LockConversationEvents :many
SELECT id, payload FROM events WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id
ORDER BY id
FOR UPDATE;

-- name: SetEventPayload :exec
UPDATE events SET payload = @payload WHERE workspace_id = @workspace_id AND id = @id;

-- Webhook fan-out reads its event under this lock, so a redaction that rewrites the event waits
-- for the deliveries it writes and then rewrites them too.
-- name: LockEventShared :one
SELECT * FROM events WHERE workspace_id = $1 AND id = $2 FOR SHARE;
