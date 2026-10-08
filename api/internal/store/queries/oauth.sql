-- name: CreateOAuthClient :one
INSERT INTO oauth_clients (id, client_id, kind, name, client_uri, redirect_uris, created_at, fetched_at)
VALUES (@id, @client_id, @kind, @name, sqlc.narg(client_uri), @redirect_uris, @created_at, sqlc.narg(fetched_at))
RETURNING *;

-- name: SaveMetadataClient :one
INSERT INTO oauth_clients (id, client_id, kind, name, client_uri, redirect_uris, created_at, fetched_at)
VALUES (@id, @client_id, 'metadata', @name, sqlc.narg(client_uri), @redirect_uris, @now::timestamptz, @now::timestamptz)
ON CONFLICT (client_id) DO UPDATE SET name = EXCLUDED.name, client_uri = EXCLUDED.client_uri,
    redirect_uris = EXCLUDED.redirect_uris, fetched_at = EXCLUDED.fetched_at
RETURNING *;

-- name: GetOAuthClientByClientID :one
SELECT * FROM oauth_clients WHERE client_id = $1;

-- name: CreateOAuthRequest :exec
INSERT INTO oauth_requests (id, client_id, redirect_uri, state, code_challenge, scopes, resource, created_at, expires_at)
VALUES (@id, @client_id, @redirect_uri, sqlc.narg(state), @code_challenge, sqlc.narg(scopes), @resource, @created_at, @expires_at);

-- name: GetOAuthRequest :one
SELECT r.id, r.redirect_uri, r.state, r.code_challenge, r.scopes, r.resource, r.created_at, r.expires_at,
       c.id AS client_ref, c.client_id AS client_key, c.name AS client_name, c.client_uri
FROM oauth_requests r JOIN oauth_clients c ON c.id = r.client_id
WHERE r.id = @id AND r.expires_at > @now::timestamptz;

-- name: DeleteOAuthRequest :execrows
DELETE FROM oauth_requests WHERE id = $1;

-- name: UpsertOAuthGrant :one
INSERT INTO oauth_grants (workspace_id, id, member_id, client_id, scopes, resource, created_at)
VALUES (@workspace_id, @id, @member_id, @client_id, @scopes, @resource, @created_at)
ON CONFLICT (workspace_id, member_id, client_id) WHERE revoked_at IS NULL
DO UPDATE SET scopes = EXCLUDED.scopes, resource = EXCLUDED.resource
RETURNING *;

-- name: CreateOAuthCode :exec
INSERT INTO oauth_codes (workspace_id, id, grant_id, code_hash, redirect_uri, code_challenge, created_at, expires_at)
VALUES (@workspace_id, @id, @grant_id, @code_hash, @redirect_uri, @code_challenge, @created_at, @expires_at);

-- name: LockOAuthCode :one
SELECT oc.workspace_id, oc.id, oc.grant_id, oc.redirect_uri, oc.code_challenge, oc.expires_at, oc.used_at,
       g.scopes, g.resource, cl.client_id AS client_key
FROM oauth_codes oc
JOIN oauth_grants g ON g.workspace_id = oc.workspace_id AND g.id = oc.grant_id
JOIN oauth_clients cl ON cl.id = g.client_id
WHERE oc.code_hash = $1 AND g.revoked_at IS NULL
FOR UPDATE OF oc;

-- name: MarkOAuthCodeUsed :exec
UPDATE oauth_codes SET used_at = @now::timestamptz WHERE workspace_id = @workspace_id AND id = @id;

-- name: CreateOAuthToken :exec
INSERT INTO oauth_tokens (workspace_id, id, grant_id, kind, token_hash, created_at, expires_at)
VALUES (@workspace_id, @id, @grant_id, @kind, @token_hash, @created_at, @expires_at);

-- name: LockOAuthRefreshToken :one
SELECT t.workspace_id, t.id, t.grant_id, t.expires_at, t.used_at, g.scopes, g.resource, cl.client_id AS client_key
FROM oauth_tokens t
JOIN oauth_grants g ON g.workspace_id = t.workspace_id AND g.id = t.grant_id
JOIN oauth_clients cl ON cl.id = g.client_id
WHERE t.token_hash = $1 AND t.kind = 'refresh' AND g.revoked_at IS NULL
FOR UPDATE OF t;

-- name: MarkOAuthTokenUsed :exec
UPDATE oauth_tokens SET used_at = @now::timestamptz WHERE workspace_id = @workspace_id AND id = @id;

-- name: GetOAuthTokenByHash :one
SELECT t.workspace_id, t.id, t.grant_id, t.kind, cl.client_id AS client_key
FROM oauth_tokens t
JOIN oauth_grants g ON g.workspace_id = t.workspace_id AND g.id = t.grant_id
JOIN oauth_clients cl ON cl.id = g.client_id
WHERE t.token_hash = $1;

-- name: DeleteOAuthToken :exec
DELETE FROM oauth_tokens WHERE workspace_id = $1 AND id = $2;

-- name: RevokeOAuthGrant :execrows
UPDATE oauth_grants SET revoked_at = coalesce(revoked_at, @now::timestamptz)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteOAuthGrantTokens :exec
DELETE FROM oauth_tokens WHERE workspace_id = $1 AND grant_id = $2;

-- name: DeleteOAuthGrantCodes :exec
DELETE FROM oauth_codes WHERE workspace_id = $1 AND grant_id = $2;

-- name: GetOAuthAccess :one
SELECT t.expires_at, g.workspace_id, g.id AS grant_id, g.scopes, g.resource, m.id AS member_id, m.role,
       cl.name AS client_name, w.bots_may_send
FROM oauth_tokens t
JOIN oauth_grants g ON g.workspace_id = t.workspace_id AND g.id = t.grant_id
JOIN members m ON m.workspace_id = g.workspace_id AND m.id = g.member_id
JOIN oauth_clients cl ON cl.id = g.client_id
JOIN workspaces w ON w.id = g.workspace_id
WHERE t.token_hash = $1 AND t.kind = 'access' AND g.revoked_at IS NULL AND w.deleted_at IS NULL;

-- name: CountOAuthGrantRequest :exec
UPDATE oauth_grants SET last_used_at = @now::timestamptz,
    request_count = CASE WHEN request_month = @month::date THEN request_count + 1 ELSE 1 END,
    request_month = @month::date
WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListOAuthGrants :many
SELECT g.id, g.member_id, g.scopes, g.resource, g.created_at, g.last_used_at, g.request_month, g.request_count,
       cl.client_id AS client_key, cl.name AS client_name, cl.client_uri
FROM oauth_grants g JOIN oauth_clients cl ON cl.id = g.client_id
WHERE g.workspace_id = @workspace_id AND g.revoked_at IS NULL
  AND (sqlc.narg(member_id)::uuid IS NULL OR g.member_id = sqlc.narg(member_id)::uuid)
ORDER BY g.created_at, g.id;

-- name: GetOAuthGrant :one
SELECT * FROM oauth_grants WHERE workspace_id = $1 AND id = $2 AND revoked_at IS NULL;

-- name: DeleteExpiredOAuthTokens :execrows
DELETE FROM oauth_tokens WHERE workspace_id = $1 AND expires_at < @before::timestamptz;

-- name: DeleteExpiredOAuthCodes :execrows
DELETE FROM oauth_codes WHERE workspace_id = $1 AND expires_at < @before::timestamptz;

-- name: DeleteExpiredOAuthRequests :execrows
DELETE FROM oauth_requests WHERE expires_at < @before::timestamptz;

-- name: DeleteUnusedOAuthClients :execrows
DELETE FROM oauth_clients c
WHERE c.created_at < @before::timestamptz
  AND NOT EXISTS (SELECT 1 FROM oauth_grants g WHERE g.client_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM oauth_requests r WHERE r.client_id = c.id);
