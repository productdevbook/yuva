-- name: ClaimIdempotencyKey :execrows
INSERT INTO idempotency_keys (workspace_id, caller_type, caller_id, key, method, path, body_sha256, created_at)
VALUES (@workspace_id, @caller_type, @caller_id, @key, @method, @path, @body_sha256, @now)
ON CONFLICT (workspace_id, caller_type, caller_id, key) DO UPDATE
SET method = excluded.method, path = excluded.path, body_sha256 = excluded.body_sha256, status = NULL,
    headers = NULL, body = NULL, created_at = excluded.created_at
WHERE idempotency_keys.created_at < @stale_before::timestamptz;

-- name: GetIdempotencyKey :one
SELECT * FROM idempotency_keys
WHERE workspace_id = @workspace_id AND caller_type = @caller_type AND caller_id = @caller_id AND key = @key;

-- name: CompleteIdempotencyKey :exec
UPDATE idempotency_keys SET status = @status, headers = @headers, body = @body
WHERE workspace_id = @workspace_id AND caller_type = @caller_type AND caller_id = @caller_id AND key = @key;

-- name: ReleaseIdempotencyKey :exec
DELETE FROM idempotency_keys
WHERE workspace_id = @workspace_id AND caller_type = @caller_type AND caller_id = @caller_id AND key = @key
  AND status IS NULL;

-- name: DeleteIdempotencyKeysBefore :execrows
DELETE FROM idempotency_keys WHERE workspace_id = @workspace_id AND created_at < @before;
