-- name: AddAuditEntry :exec
INSERT INTO audit_log (id, workspace_id, actor_kind, actor_id, actor_member_id, actor_name, via, action, target_type, target_id, details, created_at)
VALUES (@id, @workspace_id, @actor_kind, sqlc.narg(actor_id), sqlc.narg(actor_member_id),
    left(coalesce(CASE @actor_kind::text
        WHEN 'api_key' THEN (SELECT coalesce(k.bot_name, k.name) FROM api_keys k WHERE k.workspace_id = @workspace_id AND k.id = sqlc.narg(actor_id))
        WHEN 'operator' THEN ''
        ELSE (SELECT coalesce(nullif(pe.name, ''), pe.email) FROM members m JOIN people pe ON pe.id = m.person_id
              WHERE m.workspace_id = @workspace_id AND m.id = sqlc.narg(actor_member_id))
    END, ''), 400),
    left(sqlc.narg(via)::text, 200), @action, @target_type, @target_id, @details, @now);

-- name: ListAuditEntries :many
SELECT * FROM audit_log
WHERE workspace_id = @workspace_id
  AND (sqlc.narg(actor_id)::uuid IS NULL OR actor_id = sqlc.narg(actor_id)::uuid OR actor_member_id = sqlc.narg(actor_id)::uuid)
  AND (sqlc.narg(actor_kind)::text IS NULL OR actor_kind = sqlc.narg(actor_kind)::text)
  AND (cardinality(@actions::text[]) = 0 OR action = ANY(@actions::text[]))
  AND (sqlc.narg(since)::timestamptz IS NULL OR created_at >= sqlc.narg(since)::timestamptz)
  AND (sqlc.narg(until)::timestamptz IS NULL OR created_at < sqlc.narg(until)::timestamptz)
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (created_at, id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: DeleteOldAuditEntries :execrows
DELETE FROM audit_log WHERE workspace_id = @workspace_id AND created_at < @before;
