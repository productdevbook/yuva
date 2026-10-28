-- name: GetSESTenant :one
SELECT * FROM ses_tenants WHERE workspace_id = $1 AND inbox_id = $2;

-- name: ListSESTenantsOfWorkspace :many
SELECT * FROM ses_tenants WHERE workspace_id = $1 ORDER BY created_at, inbox_id;

-- name: CreateSESTenant :one
INSERT INTO ses_tenants (workspace_id, inbox_id, name, region, created_at)
VALUES (@workspace_id, @inbox_id, @name, @region, @now)
ON CONFLICT (workspace_id, inbox_id) DO NOTHING
RETURNING *;

-- name: SetSESTenantReady :one
UPDATE ses_tenants SET arn = @arn, state = 'ready', error = NULL
WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id
RETURNING *;

-- name: SetSESTenantError :one
UPDATE ses_tenants SET state = 'error', error = @error
WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id
RETURNING *;

-- name: SetSESTenantStatus :one
WITH prev AS (
    SELECT sending_status FROM ses_tenants
    WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id
    FOR UPDATE
)
UPDATE ses_tenants t SET sending_status = @sending_status, customer_status = sqlc.narg(customer_status),
    aws_status = sqlc.narg(aws_status), cause = sqlc.narg(cause), reputation_impact = @reputation_impact,
    status_at = sqlc.narg(status_at), checked_at = @now
FROM prev
WHERE t.workspace_id = @workspace_id AND t.inbox_id = @inbox_id
RETURNING prev.sending_status AS previous_status;

-- The status poll reads every tenant because SES names only the tenant; every write that follows is
-- scoped to the row's workspace and inbox (the written exception in docs/architecture.md).
-- name: ListSESTenantsAll :many
SELECT t.* FROM ses_tenants t
JOIN workspaces w ON w.id = t.workspace_id
WHERE w.deleted_at IS NULL AND t.arn IS NOT NULL
ORDER BY t.created_at, t.inbox_id;

-- An SES tenant event names only the tenant's ARN (the written exception in docs/architecture.md).
-- name: FindSESTenantByARN :one
SELECT t.* FROM ses_tenants t
JOIN workspaces w ON w.id = t.workspace_id
WHERE t.arn = $1 AND w.deleted_at IS NULL;

-- name: ListSESTenantResources :many
SELECT * FROM ses_tenant_resources WHERE workspace_id = $1 AND inbox_id = $2 ORDER BY kind, name;

-- name: AddSESTenantResource :exec
INSERT INTO ses_tenant_resources (workspace_id, inbox_id, arn, kind, name, created_at)
VALUES (@workspace_id, @inbox_id, @arn, @kind, @name, @now)
ON CONFLICT (workspace_id, inbox_id, arn) DO NOTHING;

-- name: DeleteSESTenantResource :exec
DELETE FROM ses_tenant_resources WHERE workspace_id = $1 AND inbox_id = $2 AND arn = $3;

-- name: ListInboxEmailChannelsForSES :many
SELECT e.* FROM email_channels e
JOIN channels c ON c.workspace_id = e.workspace_id AND c.id = e.channel_id
WHERE e.workspace_id = @workspace_id AND c.inbox_id = @inbox_id
ORDER BY e.channel_id;

-- name: SetEmailChannelSESAccounts :exec
UPDATE email_channels SET smtp_aws_account = sqlc.narg(smtp_aws_account),
    broadcast_smtp_aws_account = sqlc.narg(broadcast_smtp_aws_account)
WHERE workspace_id = @workspace_id AND channel_id = @channel_id
  AND smtp_host = @smtp_host AND smtp_username = @smtp_username
  AND coalesce(broadcast_smtp_host, '') = @broadcast_smtp_host::text
  AND coalesce(broadcast_smtp_username, '') = @broadcast_smtp_username::text;

-- A sync writes its verdict only when nothing it decided on changed since it read the channel.
-- name: SetEmailChannelSESState :execrows
UPDATE email_channels SET ses_state = @state, ses_detail = sqlc.narg(detail), ses_checked_at = @now
WHERE workspace_id = @workspace_id AND channel_id = @channel_id
  AND smtp_host = @smtp_host AND smtp_username = @smtp_username
  AND coalesce(broadcast_smtp_host, '') = @broadcast_smtp_host::text
  AND coalesce(broadcast_smtp_username, '') = @broadcast_smtp_username::text
  AND address = @address AND coalesce(from_address, '') = @from_address::text
  AND coalesce(ses_tenant, '') = @ses_tenant::text
  AND coalesce(ses_configuration_set, '') = @ses_configuration_set::text;

-- name: ResetEmailChannelSES :one
UPDATE email_channels SET
    smtp_aws_account = CASE WHEN @clear_smtp::bool THEN NULL ELSE smtp_aws_account END,
    broadcast_smtp_aws_account = CASE WHEN @clear_broadcast::bool THEN NULL ELSE broadcast_smtp_aws_account END,
    ses_state = @state, ses_detail = sqlc.narg(detail), ses_checked_at = NULL
WHERE workspace_id = @workspace_id AND channel_id = @channel_id
RETURNING *;

-- name: SetEmailChannelSESInput :one
UPDATE email_channels SET ses_tenant = sqlc.narg(tenant), ses_configuration_set = sqlc.narg(configuration_set)
WHERE workspace_id = @workspace_id AND channel_id = @channel_id
RETURNING *;

-- The operator's back-fill reconciles every workspace (the written exception in docs/architecture.md).
-- name: ListInboxesWithSESChannels :many
SELECT e.workspace_id, c.inbox_id FROM email_channels e
JOIN channels c ON c.workspace_id = e.workspace_id AND c.id = e.channel_id
JOIN workspaces w ON w.id = e.workspace_id
WHERE w.deleted_at IS NULL
  AND (e.smtp_host LIKE 'email-smtp%.amazonaws.com' OR e.broadcast_smtp_host LIKE 'email-smtp%.amazonaws.com' OR e.ses_state <> 'none')
UNION
SELECT t.workspace_id, t.inbox_id FROM ses_tenants t
JOIN workspaces w ON w.id = t.workspace_id
WHERE w.deleted_at IS NULL
ORDER BY 1, 2;

-- name: ListRunningBroadcastsOfInbox :many
SELECT * FROM broadcasts
WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id AND state IN ('scheduled', 'preparing', 'sending', 'active')
ORDER BY created_at, id;

-- name: SetSESTenantMissing :execrows
UPDATE ses_tenants SET state = 'missing', checked_at = @now
WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id AND arn = @arn AND state = 'ready';

-- name: DeleteSESTenantResources :exec
DELETE FROM ses_tenant_resources WHERE workspace_id = $1 AND inbox_id = $2;
