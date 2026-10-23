-- name: OwnDomainTaken :one
-- The documented exception: a verified own domain is unique across the server and across status pages, help centers,
-- roadmap boards and changelogs, so availability is checked across workspaces.
SELECT (EXISTS (SELECT 1 FROM status_pages WHERE custom_domain = @domain::text AND id <> sqlc.arg(id)::uuid AND domain_state = 'verified')
    OR EXISTS (SELECT 1 FROM help_centers WHERE custom_domain = @domain::text AND id <> sqlc.arg(id)::uuid AND domain_state = 'verified')
    OR EXISTS (SELECT 1 FROM roadmap_boards WHERE custom_domain = @domain::text AND id <> sqlc.arg(id)::uuid AND domain_state = 'verified')
    OR EXISTS (SELECT 1 FROM changelogs WHERE custom_domain = @domain::text AND id <> sqlc.arg(id)::uuid AND domain_state = 'verified'))::bool;

-- name: LockStatusDomainName :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_status_domain:' || @domain::text, 0));

-- name: LockStatusDomainCount :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_status_domains:' || sqlc.arg(workspace_id)::uuid::text, 0));

-- name: FailOwnDomainClaims :many
-- The documented exception: once a page of any kind verifies a domain, the other claims of every workspace lose it.
WITH s AS (
    UPDATE status_pages SET domain_state = 'failed', domain_problem = 'taken', domain_check_at = NULL, updated_at = sqlc.arg(now)::timestamptz
    WHERE custom_domain = @domain::text AND id <> sqlc.arg(id)::uuid AND domain_state IN ('pending', 'failed')
    RETURNING 'status'::text AS kind, id, workspace_id, inbox_id
), h AS (
    UPDATE help_centers SET domain_state = 'failed', domain_problem = 'taken', domain_check_at = NULL, updated_at = sqlc.arg(now)::timestamptz
    WHERE custom_domain = @domain::text AND id <> sqlc.arg(id)::uuid AND domain_state IN ('pending', 'failed')
    RETURNING 'help'::text AS kind, id, workspace_id, inbox_id
), r AS (
    UPDATE roadmap_boards SET domain_state = 'failed', domain_problem = 'taken', domain_check_at = NULL, updated_at = sqlc.arg(now)::timestamptz
    WHERE custom_domain = @domain::text AND id <> sqlc.arg(id)::uuid AND domain_state IN ('pending', 'failed')
    RETURNING 'roadmap'::text AS kind, id, workspace_id, inbox_id
), c AS (
    UPDATE changelogs SET domain_state = 'failed', domain_problem = 'taken', domain_check_at = NULL, updated_at = sqlc.arg(now)::timestamptz
    WHERE custom_domain = @domain::text AND id <> sqlc.arg(id)::uuid AND domain_state IN ('pending', 'failed')
    RETURNING 'changelog'::text AS kind, id, workspace_id, inbox_id
)
SELECT kind, id, workspace_id, inbox_id FROM s
UNION ALL SELECT kind, id, workspace_id, inbox_id FROM h
UNION ALL SELECT kind, id, workspace_id, inbox_id FROM r
UNION ALL SELECT kind, id, workspace_id, inbox_id FROM c;

-- name: CountOwnDomains :one
SELECT ((SELECT count(*) FROM status_pages WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND id <> sqlc.arg(id)::uuid AND custom_domain IS NOT NULL AND domain_state <> 'off')
    + (SELECT count(*) FROM help_centers WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND id <> sqlc.arg(id)::uuid AND custom_domain IS NOT NULL AND domain_state <> 'off')
    + (SELECT count(*) FROM roadmap_boards WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND id <> sqlc.arg(id)::uuid AND custom_domain IS NOT NULL AND domain_state <> 'off')
    + (SELECT count(*) FROM changelogs WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND id <> sqlc.arg(id)::uuid AND custom_domain IS NOT NULL AND domain_state <> 'off'))::bigint;

-- name: ListDueOwnDomains :many
-- The documented exception: the sweep lists the own domains due for a check of every workspace.
SELECT d.kind::text AS kind, d.workspace_id::uuid AS workspace_id, d.id::uuid AS id FROM (
    SELECT 'status' AS kind, workspace_id, id, domain_check_at FROM status_pages
    WHERE domain_check_at <= @now::timestamptz AND domain_state IN ('pending', 'failed', 'verified')
    UNION ALL
    SELECT 'help', workspace_id, id, domain_check_at FROM help_centers
    WHERE domain_check_at <= @now::timestamptz AND domain_state IN ('pending', 'failed', 'verified')
    UNION ALL
    SELECT 'roadmap', workspace_id, id, domain_check_at FROM roadmap_boards
    WHERE domain_check_at <= @now::timestamptz AND domain_state IN ('pending', 'failed', 'verified')
    UNION ALL
    SELECT 'changelog', workspace_id, id, domain_check_at FROM changelogs
    WHERE domain_check_at <= @now::timestamptz AND domain_state IN ('pending', 'failed', 'verified')
) d
JOIN workspaces w ON w.id = d.workspace_id AND w.deleted_at IS NULL
ORDER BY d.domain_check_at
LIMIT @lim;

-- name: FindOwnDomain :one
-- The documented exception: a request on an own domain finds its page before its workspace is known.
SELECT d.kind::text AS kind, d.workspace_id::uuid AS workspace_id, d.id::uuid AS id, d.slug::text AS slug, d.live::bool AS live FROM (
    SELECT 'status' AS kind, workspace_id, id, slug, published AS live FROM status_pages WHERE custom_domain = @domain::text AND domain_state = 'verified'
    UNION ALL
    SELECT 'help', workspace_id, id, slug, published FROM help_centers WHERE custom_domain = @domain::text AND domain_state = 'verified'
    UNION ALL
    SELECT 'roadmap', workspace_id, id, slug, public FROM roadmap_boards WHERE custom_domain = @domain::text AND domain_state = 'verified'
    UNION ALL
    SELECT 'changelog', workspace_id, id, slug, published FROM changelogs WHERE custom_domain = @domain::text AND domain_state = 'verified'
) d
JOIN workspaces w ON w.id = d.workspace_id AND w.deleted_at IS NULL
ORDER BY d.kind, d.id
LIMIT 1;

-- name: ListOwnCloudflareHostnames :many
SELECT cf_hostname_id::text AS hostname_id, custom_domain::text AS domain FROM status_pages
WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND cf_hostname_id IS NOT NULL
UNION ALL
SELECT cf_hostname_id::text, custom_domain::text FROM help_centers WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND cf_hostname_id IS NOT NULL
UNION ALL
SELECT cf_hostname_id::text, custom_domain::text FROM roadmap_boards WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND cf_hostname_id IS NOT NULL
UNION ALL
SELECT cf_hostname_id::text, custom_domain::text FROM changelogs WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND cf_hostname_id IS NOT NULL;

-- name: ListInboxOwnDomains :many
SELECT id, cf_hostname_id, custom_domain FROM status_pages WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND inbox_id = sqlc.arg(inbox_id)::uuid
UNION ALL
SELECT id, cf_hostname_id, custom_domain FROM help_centers WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND inbox_id = sqlc.arg(inbox_id)::uuid
UNION ALL
SELECT id, cf_hostname_id, custom_domain FROM roadmap_boards WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND inbox_id = sqlc.arg(inbox_id)::uuid
UNION ALL
SELECT id, cf_hostname_id, custom_domain FROM changelogs WHERE workspace_id = sqlc.arg(workspace_id)::uuid AND inbox_id = sqlc.arg(inbox_id)::uuid;

-- name: WriteStatusPageDomain :one
UPDATE status_pages SET custom_domain = sqlc.narg(domain), domain_token = sqlc.narg(token), domain_state = sqlc.narg(state),
    domain_problem = sqlc.narg(problem), domain_checked_at = sqlc.narg(checked_at), domain_check_at = sqlc.narg(check_at),
    domain_verified_at = sqlc.narg(verified_at), domain_missing_since = sqlc.narg(missing_since), domain_claimed_at = sqlc.narg(claimed_at),
    cf_hostname_id = sqlc.narg(cf_hostname_id), cf_status = sqlc.narg(cf_status), cf_ssl_status = sqlc.narg(cf_ssl_status),
    cf_errors = sqlc.narg(cf_errors)::text[], cf_checked_at = sqlc.narg(cf_checked_at),
    changed_at = CASE WHEN @moved::bool THEN greatest(changed_at + interval '1 microsecond', @now::timestamptz) ELSE changed_at END,
    updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: WriteHelpCenterDomain :one
UPDATE help_centers SET custom_domain = sqlc.narg(domain), domain_token = sqlc.narg(token), domain_state = sqlc.narg(state),
    domain_problem = sqlc.narg(problem), domain_checked_at = sqlc.narg(checked_at), domain_check_at = sqlc.narg(check_at),
    domain_verified_at = sqlc.narg(verified_at), domain_missing_since = sqlc.narg(missing_since), domain_claimed_at = sqlc.narg(claimed_at),
    cf_hostname_id = sqlc.narg(cf_hostname_id), cf_status = sqlc.narg(cf_status), cf_ssl_status = sqlc.narg(cf_ssl_status),
    cf_errors = sqlc.narg(cf_errors)::text[], cf_checked_at = sqlc.narg(cf_checked_at),
    changed_at = CASE WHEN @moved::bool THEN greatest(changed_at + interval '1 microsecond', @now::timestamptz) ELSE changed_at END,
    updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: WriteRoadmapBoardDomain :one
UPDATE roadmap_boards SET custom_domain = sqlc.narg(domain), domain_token = sqlc.narg(token), domain_state = sqlc.narg(state),
    domain_problem = sqlc.narg(problem), domain_checked_at = sqlc.narg(checked_at), domain_check_at = sqlc.narg(check_at),
    domain_verified_at = sqlc.narg(verified_at), domain_missing_since = sqlc.narg(missing_since), domain_claimed_at = sqlc.narg(claimed_at),
    cf_hostname_id = sqlc.narg(cf_hostname_id), cf_status = sqlc.narg(cf_status), cf_ssl_status = sqlc.narg(cf_ssl_status),
    cf_errors = sqlc.narg(cf_errors)::text[], cf_checked_at = sqlc.narg(cf_checked_at),
    changed_at = CASE WHEN @moved::bool THEN greatest(changed_at + interval '1 microsecond', @now::timestamptz) ELSE changed_at END,
    updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: WriteChangelogDomain :one
UPDATE changelogs SET custom_domain = sqlc.narg(domain), domain_token = sqlc.narg(token), domain_state = sqlc.narg(state),
    domain_problem = sqlc.narg(problem), domain_checked_at = sqlc.narg(checked_at), domain_check_at = sqlc.narg(check_at),
    domain_verified_at = sqlc.narg(verified_at), domain_missing_since = sqlc.narg(missing_since), domain_claimed_at = sqlc.narg(claimed_at),
    cf_hostname_id = sqlc.narg(cf_hostname_id), cf_status = sqlc.narg(cf_status), cf_ssl_status = sqlc.narg(cf_ssl_status),
    cf_errors = sqlc.narg(cf_errors)::text[], cf_checked_at = sqlc.narg(cf_checked_at),
    changed_at = CASE WHEN @moved::bool THEN greatest(changed_at + interval '1 microsecond', @now::timestamptz) ELSE changed_at END,
    updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;
