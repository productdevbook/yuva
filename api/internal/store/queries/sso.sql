-- name: GetSsoSettings :one
SELECT * FROM sso_settings WHERE workspace_id = $1;

-- name: LockSsoSettings :one
INSERT INTO sso_settings (workspace_id, id, created_at, updated_at)
VALUES (@workspace_id, @id, @now, @now)
ON CONFLICT (workspace_id) DO UPDATE SET workspace_id = excluded.workspace_id
RETURNING *;

-- name: SaveSsoOidc :one
UPDATE sso_settings SET protocol = 'oidc', oidc_issuer = @issuer, oidc_client_id = @client_id, oidc_client_secret = @client_secret,
    oidc_scopes = @scopes, groups_claim = @groups_claim, allowed_groups = @allowed_groups,
    saml_entity_id = NULL, saml_sso_url = NULL, saml_certificates = '{}', saml_metadata_url = NULL, saml_metadata_at = NULL, saml_metadata_tried_at = NULL,
    saml_email_attribute = NULL, updated_at = @now
WHERE workspace_id = @workspace_id
RETURNING *;

-- name: SaveSsoSaml :one
UPDATE sso_settings SET protocol = 'saml', saml_entity_id = @entity_id, saml_sso_url = @sso_url, saml_certificates = @certificates,
    saml_metadata_url = sqlc.narg(metadata_url), saml_metadata_at = sqlc.narg(metadata_at), saml_metadata_tried_at = sqlc.narg(metadata_at), saml_email_attribute = sqlc.narg(email_attribute),
    groups_claim = @groups_claim, allowed_groups = @allowed_groups,
    oidc_issuer = NULL, oidc_client_id = NULL, oidc_client_secret = NULL, oidc_scopes = '{}', updated_at = @now
WHERE workspace_id = @workspace_id
RETURNING *;

-- A refresh applies only to the metadata it was fetched for: a change made meanwhile wins.
-- name: RefreshSsoSamlMetadata :one
UPDATE sso_settings SET saml_entity_id = @entity_id, saml_sso_url = @sso_url, saml_certificates = @certificates,
    saml_metadata_at = @now, saml_metadata_tried_at = @now, updated_at = CASE WHEN @changed::boolean THEN @now ELSE updated_at END
WHERE workspace_id = @workspace_id AND id = @id AND protocol = 'saml' AND saml_metadata_url = @metadata_url
RETURNING *;

-- name: MarkSamlMetadataTried :exec
UPDATE sso_settings SET saml_metadata_tried_at = @now
WHERE workspace_id = @workspace_id AND id = @id AND protocol = 'saml' AND saml_metadata_url = @metadata_url;

-- name: ClearSsoProvider :one
UPDATE sso_settings SET protocol = NULL, oidc_issuer = NULL, oidc_client_id = NULL, oidc_client_secret = NULL,
    oidc_scopes = '{}', groups_claim = 'groups', allowed_groups = '{}', sso_required = false,
    saml_entity_id = NULL, saml_sso_url = NULL, saml_certificates = '{}', saml_metadata_url = NULL, saml_metadata_at = NULL, saml_metadata_tried_at = NULL,
    saml_email_attribute = NULL, updated_at = @now
WHERE workspace_id = @workspace_id AND protocol = @protocol
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

-- name: RelaxSsoWithoutDomains :execrows
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

-- name: DeleteExpiredSamlAssertions :exec
DELETE FROM saml_assertions WHERE workspace_id = @workspace_id AND expires_at <= @now;

-- name: RecordSamlAssertion :execrows
INSERT INTO saml_assertions (workspace_id, id_hash, expires_at)
VALUES (@workspace_id, @id_hash, @expires_at)
ON CONFLICT DO NOTHING;

-- name: RecordSsoSignIn :execrows
INSERT INTO sso_sign_ins (workspace_id, person_id, created_at) VALUES (@workspace_id, @person_id, @now)
ON CONFLICT DO NOTHING;

-- The documented exception: the daily sweep lists the verified domains due for a check of every workspace.
-- name: ListSsoDomainsToRecheck :many
SELECT d.workspace_id, d.id FROM sso_domains d JOIN workspaces w ON w.id = d.workspace_id
WHERE d.verified_at IS NOT NULL AND (d.checked_at IS NULL OR d.checked_at <= @due::timestamptz) AND w.deleted_at IS NULL
ORDER BY d.checked_at NULLS FIRST, d.id
LIMIT @lim;

-- name: MarkSsoDomainRechecked :one
UPDATE sso_domains SET checked_at = @now::timestamptz, problem = sqlc.narg(problem),
    missing_since = CASE WHEN @missing::boolean THEN coalesce(missing_since, @now::timestamptz) WHEN @found::boolean THEN NULL ELSE missing_since END
WHERE workspace_id = @workspace_id AND id = @id AND verified_at IS NOT NULL
RETURNING *;

-- name: LapseSsoDomain :one
UPDATE sso_domains SET verified_at = NULL, missing_since = NULL
WHERE workspace_id = @workspace_id AND id = @id AND verified_at IS NOT NULL AND missing_since <= @before::timestamptz
RETURNING *;

-- A replaced or removed identity provider: what it made ends (the caller's own session stays).
-- name: DeleteSsoProviderSessions :execrows
DELETE FROM sessions WHERE sso_workspace_id = @workspace_id AND sso_settings_id = @settings_id AND id <> @keep;

-- name: RevokeSsoProviderGrants :execrows
UPDATE oauth_grants SET revoked_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND sso_settings_id = @settings_id AND revoked_at IS NULL;

-- name: ForgetSsoOnLinkedReplies :exec
UPDATE linked_replies SET sso_settings_id = NULL WHERE workspace_id = @workspace_id AND sso_settings_id = @settings_id;

-- name: ForgetSsoOnIssueLinks :exec
UPDATE issue_links SET sso_settings_id = NULL WHERE workspace_id = @workspace_id AND sso_settings_id = @settings_id;

-- name: ForgetSsoOnStageChanges :exec
UPDATE roadmap_stage_changes SET sso_settings_id = NULL WHERE workspace_id = @workspace_id AND sso_settings_id = @settings_id;

-- A deleted workspace gives its domain claims up at once, so another workspace may claim them.
-- name: DeleteWorkspaceSsoDomains :execrows
DELETE FROM sso_domains WHERE workspace_id = $1;
