-- name: LockCustomFields :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_custom_fields:' || sqlc.arg(workspace_id)::uuid::text, 0));

-- name: ShareLockCustomFields :exec
SELECT pg_advisory_xact_lock_shared(hashtextextended('yuva_custom_fields:' || sqlc.arg(workspace_id)::uuid::text, 0));

-- name: ListCustomFields :many
SELECT * FROM custom_fields
WHERE workspace_id = @workspace_id AND (sqlc.narg(object)::text IS NULL OR object = sqlc.narg(object)::text)
ORDER BY object, position, created_at, id;

-- name: GetCustomField :one
SELECT * FROM custom_fields WHERE workspace_id = $1 AND id = $2;

-- name: CountCustomFields :one
SELECT count(*) FROM custom_fields WHERE workspace_id = $1 AND object = $2;

-- name: CreateCustomField :one
INSERT INTO custom_fields (id, workspace_id, object, key, name, type, options, required_on_close, position, created_at, updated_at)
VALUES (@id, @workspace_id, @object, @key, @name, @type, @options, @required_on_close, @position, @now, @now)
RETURNING *;

-- name: UpdateCustomField :one
UPDATE custom_fields SET name = @name, options = @options, required_on_close = @required_on_close, position = @position, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteCustomField :one
DELETE FROM custom_fields WHERE workspace_id = $1 AND id = $2
RETURNING *;

-- name: ConversationsHoldField :one
SELECT EXISTS (SELECT 1 FROM conversations WHERE workspace_id = @workspace_id AND fields ? @key::text)::bool;

-- name: SetConversationFields :one
UPDATE conversations SET fields = @fields, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- ClearConversationField removes the values of a deleted field (option null) or of a select
-- option no longer offered, a batch at a time in id order after @after (the (workspace_id, id)
-- unique index walks it), while that is still so.
-- name: ClearConversationField :many
WITH batch AS (
    SELECT x.id FROM conversations x
    WHERE x.workspace_id = @workspace_id AND x.id > @after AND x.fields ? @key::text
      AND (sqlc.narg(option)::text IS NULL OR (x.fields ->> sqlc.arg(key)::text) = sqlc.narg(option)::text)
    ORDER BY x.id
    LIMIT @lim
    FOR UPDATE)
UPDATE conversations c SET fields = c.fields - @key::text, updated_at = @now
FROM batch
WHERE c.workspace_id = @workspace_id AND c.id = batch.id
  AND CASE WHEN sqlc.narg(option)::text IS NULL THEN NOT EXISTS (
          SELECT 1 FROM custom_fields cf
          WHERE cf.workspace_id = c.workspace_id AND cf.object = 'conversation' AND cf.key = @key::text)
      ELSE EXISTS (
          SELECT 1 FROM custom_fields cf
          WHERE cf.workspace_id = c.workspace_id AND cf.object = 'conversation' AND cf.key = @key::text
            AND cf.type = 'select' AND NOT cf.options ? sqlc.narg(option)::text) END
RETURNING c.*;
