-- name: GetSsoSettings :one
SELECT * FROM sso_settings WHERE workspace_id = $1;

-- name: LockSsoSettings :one
INSERT INTO sso_settings (workspace_id, id, created_at, updated_at)
VALUES (@workspace_id, @id, @now, @now)
ON CONFLICT (workspace_id) DO UPDATE SET workspace_id = excluded.workspace_id
RETURNING *;

-- name: SaveSsoOidc :one
UPDATE sso_settings SET protocol = 'oidc', oidc_issuer = @issuer, oidc_client_id = @client_id, oidc_client_secret = @client_secret,
    oidc_scopes = @scopes, groups_claim = @groups_claim, allowed_groups = @allowed_groups, updated_at = @now
WHERE workspace_id = @workspace_id
RETURNING *;

-- name: ClearSsoProvider :one
UPDATE sso_settings SET protocol = NULL, oidc_issuer = NULL, oidc_client_id = NULL, oidc_client_secret = NULL,
    oidc_scopes = '{}', groups_claim = 'groups', allowed_groups = '{}', sso_required = false, updated_at = @now
WHERE workspace_id = @workspace_id AND protocol IS NOT NULL
RETURNING *;

-- name: SaveSsoRules :one
UPDATE sso_settings SET sso_required = @sso_required, jit = @jit, jit_role = @jit_role, updated_at = @now
WHERE workspace_id = @workspace_id
RETURNING *;

-- name: ListSsoDomains :many
SELECT * FROM sso_domains WHERE workspace_id = $1 ORDER BY domain, id;

-- name: CountSsoDomains :one
SELECT count(*) FROM sso_domains WHERE workspace_id = $1;

-- name: GetSsoDomain :one
SELECT * FROM sso_domains WHERE workspace_id = $1 AND id = $2;

-- name: AddSsoDomain :one
INSERT INTO sso_domains (id, workspace_id, domain, token, created_at)
VALUES (@id, @workspace_id, @domain, @token, @now)
RETURNING *;

-- name: SsoDomainVerifiedElsewhere :one
SELECT EXISTS (
    SELECT 1 FROM sso_domains d JOIN workspaces w ON w.id = d.workspace_id
    WHERE d.domain = @domain AND d.verified_at IS NOT NULL AND d.workspace_id <> @workspace_id AND w.deleted_at IS NULL
) AS taken;

-- name: MarkSsoDomainChecked :one
UPDATE sso_domains SET checked_at = @now::timestamptz, problem = sqlc.narg(problem),
    verified_at = CASE WHEN @verified::boolean THEN coalesce(verified_at, @now::timestamptz) ELSE verified_at END
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteSsoDomain :one
DELETE FROM sso_domains WHERE workspace_id = $1 AND id = $2
RETURNING *;

-- name: RelaxSsoWithoutDomains :exec
UPDATE sso_settings s SET sso_required = false, updated_at = @now
WHERE s.workspace_id = @workspace_id AND s.sso_required
  AND NOT EXISTS (SELECT 1 FROM sso_domains d WHERE d.workspace_id = @workspace_id AND d.verified_at IS NOT NULL);

-- Sign-in runs before any workspace is known: a verified domain is unique across the server.
-- name: FindSsoByDomain :one
SELECT s.* FROM sso_domains d
JOIN sso_settings s ON s.workspace_id = d.workspace_id
JOIN workspaces w ON w.id = d.workspace_id
WHERE d.domain = @domain AND d.verified_at IS NOT NULL AND w.deleted_at IS NULL;

-- name: SsoRequiredFor :one
SELECT s.id, p.email FROM sso_settings s
JOIN people p ON p.id = @person_id
JOIN sso_domains d ON d.workspace_id = s.workspace_id AND d.verified_at IS NOT NULL
    AND d.domain = substring(p.email from '@([^@]*)$')
WHERE s.workspace_id = @workspace_id AND s.sso_required AND s.protocol IS NOT NULL
LIMIT 1;

-- name: SsoDomainVerifiedIn :one
SELECT EXISTS (
    SELECT 1 FROM sso_domains WHERE workspace_id = @workspace_id AND domain = @domain AND verified_at IS NOT NULL
) AS verified;

-- name: HasVerifiedSsoDomain :one
SELECT EXISTS (SELECT 1 FROM sso_domains WHERE workspace_id = $1 AND verified_at IS NOT NULL) AS verified;

-- name: CreateSsoLogin :exec
INSERT INTO sso_logins (state_hash, workspace_id, settings_id, nonce, verifier, return_to, created_at, expires_at)
VALUES (@state_hash, @workspace_id, @settings_id, @nonce, @verifier, @return_to, @now, @expires_at);

-- Found by the state's hash alone: the callback knows no workspace yet.
-- name: TakeSsoLogin :one
DELETE FROM sso_logins WHERE state_hash = @state_hash
RETURNING *;

-- name: DeleteExpiredSsoLogins :exec
DELETE FROM sso_logins WHERE expires_at <= @now;
