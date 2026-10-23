-- name: CountIssueIntegrations :one
SELECT count(*)::int FROM issue_integrations WHERE workspace_id = $1;

-- name: CreateIssueIntegration :one
INSERT INTO issue_integrations (id, workspace_id, provider, name, account, api_url, default_target, token, webhook_secret,
                                created_by_member_id, created_at, updated_at)
VALUES (@id, @workspace_id, @provider, @name, @account, sqlc.narg(api_url), sqlc.narg(default_target), @token, @webhook_secret,
        sqlc.narg(created_by_member_id), @now, @now)
RETURNING *;

-- name: ListIssueIntegrations :many
SELECT * FROM issue_integrations WHERE workspace_id = $1 ORDER BY created_at, id;

-- name: GetIssueIntegration :one
SELECT * FROM issue_integrations WHERE workspace_id = $1 AND id = $2;

-- name: LockIssueIntegration :one
SELECT * FROM issue_integrations WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: FindIssueIntegration :one
-- The webhook ingress names only the integration: this lookup comes before the workspace is known.
SELECT * FROM issue_integrations WHERE id = $1;

-- name: UpdateIssueIntegration :one
UPDATE issue_integrations SET name = @name, account = @account, default_target = sqlc.narg(default_target), token = @token,
    webhook_secret = @webhook_secret, last_error = CASE WHEN @token_changed::bool THEN NULL ELSE last_error END,
    last_error_at = CASE WHEN @token_changed::bool THEN NULL ELSE last_error_at END, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteIssueIntegration :execrows
DELETE FROM issue_integrations WHERE workspace_id = $1 AND id = $2;

-- name: SetIssueIntegrationError :exec
UPDATE issue_integrations SET last_error = sqlc.narg(error)::text,
    last_error_at = CASE WHEN sqlc.narg(error)::text IS NULL THEN NULL ELSE @now::timestamptz END
WHERE workspace_id = @workspace_id AND id = @id AND last_error IS DISTINCT FROM sqlc.narg(error)::text;

-- name: GetTrackedIssueByExternal :one
SELECT * FROM tracked_issues WHERE workspace_id = @workspace_id AND integration_id = @integration_id AND external_id = @external_id
FOR UPDATE;

-- name: LockTrackedIssue :one
SELECT * FROM tracked_issues WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: GetTrackedIssue :one
SELECT * FROM tracked_issues WHERE workspace_id = $1 AND id = $2;

-- name: InsertTrackedIssue :one
INSERT INTO tracked_issues (id, workspace_id, integration_id, external_id, key, url, title, state, progress, synced_at, next_sync_at, created_at)
VALUES (@id, @workspace_id, @integration_id, @external_id, @key, @url, @title, @state, @progress, @now, @next_sync_at, @now)
RETURNING *;

-- name: UpdateTrackedIssue :one
UPDATE tracked_issues SET key = @key, url = @url, title = @title, state = @state, progress = @progress,
    state_changed_at = CASE WHEN progress <> @progress THEN @now::timestamptz ELSE state_changed_at END, synced_at = @synced_at,
    next_sync_at = @next_sync_at, sync_failures = 0
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: TrackedIssueFailed :exec
UPDATE tracked_issues SET sync_failures = sync_failures + 1, next_sync_at = @next_sync_at
WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteOrphanTrackedIssue :exec
DELETE FROM tracked_issues t WHERE t.workspace_id = @workspace_id AND t.id = @id
    AND NOT EXISTS (SELECT 1 FROM issue_links l WHERE l.workspace_id = t.workspace_id AND l.issue_id = t.id);

-- name: FindTrackedIssues :many
SELECT id FROM tracked_issues
WHERE workspace_id = @workspace_id AND integration_id = @integration_id AND external_id = ANY(@external_ids::text[]);

-- name: DueTrackedIssues :many
-- The reconciliation sweep takes linked issues of every workspace that are due, at most
-- per_integration of one integration, and holds each for 15 minutes while its read is queued.
UPDATE tracked_issues t SET next_sync_at = @hold_until
FROM (
    SELECT d.workspace_id, d.id FROM (
        SELECT x.workspace_id, x.id, x.next_sync_at,
               row_number() OVER (PARTITION BY x.integration_id ORDER BY x.next_sync_at, x.id) AS rank
        FROM tracked_issues x
        WHERE x.next_sync_at <= @now
          AND EXISTS (SELECT 1 FROM issue_links l WHERE l.workspace_id = x.workspace_id AND l.issue_id = x.id)
    ) d
    WHERE d.rank <= @per_integration::bigint
    ORDER BY d.next_sync_at, d.id
    LIMIT @lim
) p
WHERE t.workspace_id = p.workspace_id AND t.id = p.id
RETURNING t.workspace_id, t.id;

-- name: InsertIssueLink :one
INSERT INTO issue_links (id, workspace_id, issue_id, conversation_id, idea_id, linked_by_member_id, linked_by_api_key_id,
                         linked_by_grant_id, via, created_at, sso_settings_id)
VALUES (@id, @workspace_id, @issue_id, sqlc.narg(conversation_id), sqlc.narg(idea_id), sqlc.narg(linked_by_member_id),
        sqlc.narg(linked_by_api_key_id), sqlc.narg(linked_by_grant_id), @via, @now, sqlc.narg(sso_settings_id))
ON CONFLICT DO NOTHING
RETURNING *;

-- name: FindIssueLink :one
SELECT * FROM issue_links
WHERE workspace_id = @workspace_id AND issue_id = @issue_id
  AND conversation_id IS NOT DISTINCT FROM sqlc.narg(conversation_id)::uuid AND idea_id IS NOT DISTINCT FROM sqlc.narg(idea_id)::uuid;

-- name: CountIssueLinksOfIssue :one
SELECT count(*)::int FROM issue_links WHERE workspace_id = $1 AND issue_id = $2;

-- name: CountIssueLinksOfTarget :one
SELECT count(*)::int FROM issue_links
WHERE workspace_id = @workspace_id AND conversation_id IS NOT DISTINCT FROM sqlc.narg(conversation_id)::uuid
  AND idea_id IS NOT DISTINCT FROM sqlc.narg(idea_id)::uuid;

-- name: LockIssueLink :one
SELECT * FROM issue_links WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: DeleteIssueLink :one
DELETE FROM issue_links
WHERE workspace_id = @workspace_id AND id = @id
  AND conversation_id IS NOT DISTINCT FROM sqlc.narg(conversation_id)::uuid AND idea_id IS NOT DISTINCT FROM sqlc.narg(idea_id)::uuid
RETURNING *;

-- name: SetIssueLinkApplied :exec
UPDATE issue_links SET applied_progress = @progress WHERE workspace_id = @workspace_id AND id = @id;

-- name: IssueLinksToApply :many
SELECT l.* FROM issue_links l
JOIN tracked_issues t ON t.workspace_id = l.workspace_id AND t.id = l.issue_id
WHERE l.workspace_id = @workspace_id AND l.issue_id = @issue_id AND l.applied_progress IS DISTINCT FROM t.progress
ORDER BY l.created_at, l.id;

-- name: ListIssueLinkViews :many
-- Every link with its issue, provider and current inbox, chosen by link ids, conversations, ideas,
-- an issue or an integration; empty lists and a null issue or integration choose nothing.
SELECT sqlc.embed(l), sqlc.embed(t), i.provider, coalesce(c.inbox_id, d.inbox_id)::uuid AS inbox_id
FROM issue_links l
JOIN tracked_issues t ON t.workspace_id = l.workspace_id AND t.id = l.issue_id
JOIN issue_integrations i ON i.workspace_id = t.workspace_id AND i.id = t.integration_id
LEFT JOIN conversations c ON c.workspace_id = l.workspace_id AND c.id = l.conversation_id
LEFT JOIN discussions d ON d.workspace_id = l.workspace_id AND d.id = l.idea_id
WHERE l.workspace_id = @workspace_id
  AND (l.id = ANY(@ids::uuid[]) OR l.conversation_id = ANY(@conversation_ids::uuid[]) OR l.idea_id = ANY(@idea_ids::uuid[])
       OR l.issue_id = sqlc.narg(issue_id)::uuid OR t.integration_id = sqlc.narg(integration_id)::uuid)
ORDER BY l.created_at, l.id;
