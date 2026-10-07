-- name: CreateWebhookEndpoint :one
INSERT INTO webhook_endpoints (id, workspace_id, inbox_id, url, description, events, include_notes, enabled, secret, created_at, updated_at)
VALUES (@id, @workspace_id, sqlc.narg(inbox_id), @url, @description, @events, @include_notes, @enabled, @secret, @now, @now)
RETURNING *;

-- name: GetWebhookEndpoint :one
SELECT * FROM webhook_endpoints WHERE workspace_id = $1 AND id = $2;

-- name: LockWebhookEndpoint :one
SELECT * FROM webhook_endpoints WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: ListWebhookEndpoints :many
SELECT * FROM webhook_endpoints
WHERE workspace_id = @workspace_id AND (sqlc.narg(inbox_id)::uuid IS NULL OR inbox_id = sqlc.narg(inbox_id)::uuid)
ORDER BY created_at, id;

-- name: ListEnabledWebhookEndpoints :many
SELECT * FROM webhook_endpoints WHERE workspace_id = $1 AND enabled ORDER BY created_at, id;

-- name: WorkspaceHasWebhooks :one
SELECT EXISTS (SELECT 1 FROM webhook_endpoints WHERE workspace_id = $1 AND enabled) AS found;

-- name: UpdateWebhookEndpoint :one
UPDATE webhook_endpoints SET url = @url, description = @description, events = @events, include_notes = @include_notes,
    enabled = @enabled,
    disabled_at = CASE WHEN @enabled::bool THEN NULL ELSE disabled_at END,
    disabled_reason = CASE WHEN @enabled::bool THEN NULL ELSE disabled_reason END,
    failing_since = CASE WHEN @enabled::bool AND NOT enabled THEN NULL ELSE failing_since END,
    updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: RotateWebhookSecret :one
UPDATE webhook_endpoints SET previous_secret = secret, previous_secret_until = @previous_until, secret = @secret, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteWebhookEndpoint :execrows
DELETE FROM webhook_endpoints WHERE workspace_id = $1 AND id = $2;

-- name: WebhookSucceeded :exec
UPDATE webhook_endpoints SET failing_since = NULL WHERE workspace_id = $1 AND id = $2;

-- name: WebhookFailed :one
UPDATE webhook_endpoints SET failing_since = coalesce(failing_since, @now::timestamptz)
WHERE workspace_id = @workspace_id AND id = @id
RETURNING failing_since::timestamptz AS failing_since, enabled;

-- name: DisableWebhookEndpoint :execrows
UPDATE webhook_endpoints SET enabled = false, disabled_at = @now::timestamptz, disabled_reason = @reason, updated_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id AND enabled;

-- name: CreateWebhookDelivery :one
INSERT INTO webhook_deliveries (id, workspace_id, endpoint_id, message_id, event_type, payload, next_attempt_at, created_at, updated_at)
VALUES (@id, @workspace_id, @endpoint_id, @message_id, @event_type, @payload, @now::timestamptz, @now::timestamptz, @now::timestamptz)
RETURNING *;

-- name: GetWebhookDelivery :one
SELECT * FROM webhook_deliveries WHERE workspace_id = $1 AND id = $2;

-- name: GetEndpointDelivery :one
SELECT * FROM webhook_deliveries WHERE workspace_id = $1 AND endpoint_id = $2 AND id = $3;

-- name: ListWebhookDeliveries :many
SELECT * FROM webhook_deliveries d
WHERE d.workspace_id = @workspace_id AND d.endpoint_id = @endpoint_id
  AND (sqlc.narg(state)::text IS NULL OR d.state = sqlc.narg(state)::text)
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (d.created_at, d.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY d.created_at DESC, d.id DESC
LIMIT @lim;

-- name: RecordWebhookAttempt :exec
INSERT INTO webhook_attempts (id, workspace_id, endpoint_id, delivery_id, attempted_at, manual, success, status_code,
                              latency_ms, response_body, error)
VALUES (@id, @workspace_id, @endpoint_id, @delivery_id, @attempted_at, @manual, @success, sqlc.narg(status_code),
        @latency_ms, @response_body, sqlc.narg(error));

-- name: FinishWebhookAttempt :one
UPDATE webhook_deliveries SET attempts = attempts + 1, last_attempt_at = @now::timestamptz, state = @state,
    next_attempt_at = sqlc.narg(next_attempt_at), updated_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: FailPendingWebhookDelivery :exec
UPDATE webhook_deliveries SET state = 'failed', next_attempt_at = NULL, updated_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id AND state = 'pending';

-- name: PruneWebhookAttempts :exec
DELETE FROM webhook_attempts w WHERE w.workspace_id = $1 AND w.endpoint_id = $2 AND w.id IN (
    SELECT a.id FROM webhook_attempts a
    WHERE a.workspace_id = $1 AND a.endpoint_id = $2
    ORDER BY a.attempted_at DESC, a.id DESC
    OFFSET $3);

-- name: ListWebhookAttempts :many
SELECT * FROM webhook_attempts
WHERE workspace_id = @workspace_id AND endpoint_id = @endpoint_id
ORDER BY attempted_at DESC, id DESC
LIMIT @lim;

-- name: ListDeliveryAttempts :many
SELECT * FROM webhook_attempts
WHERE workspace_id = @workspace_id AND delivery_id = ANY(@delivery_ids::uuid[])
ORDER BY delivery_id, attempted_at DESC, id DESC;

-- name: DeleteFinishedWebhookDeliveries :execrows
DELETE FROM webhook_deliveries WHERE workspace_id = @workspace_id AND state <> 'pending' AND created_at < @before;

-- name: ContactInboxIDs :many
SELECT x.inbox_id FROM contact_external_ids x WHERE x.workspace_id = $1 AND x.contact_id = $2
UNION
SELECT c.inbox_id FROM conversations c WHERE c.workspace_id = $1 AND c.contact_id = $2;

-- name: ContactLastActivity :one
SELECT coalesce(greatest(
    (SELECT max(last_seen_at) FROM contact_sessions s WHERE s.workspace_id = @workspace_id AND s.contact_id = @contact_id),
    (SELECT max(seen_at) FROM realtime_connections rc WHERE rc.workspace_id = @workspace_id AND rc.contact_id = @contact_id)
), 'epoch')::timestamptz AS last_seen;
