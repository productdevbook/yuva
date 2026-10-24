-- name: ListMemberAddresses :many
SELECT * FROM member_addresses WHERE workspace_id = @workspace_id AND member_id = @member_id
ORDER BY created_at, id;

-- name: LockMemberAddress :one
SELECT * FROM member_addresses WHERE workspace_id = @workspace_id AND member_id = @member_id AND id = @id FOR UPDATE;

-- name: LockMemberAddressByAddress :one
SELECT * FROM member_addresses WHERE workspace_id = @workspace_id AND address = @address FOR UPDATE;

-- name: CountMemberAddresses :one
SELECT count(*) FROM member_addresses WHERE workspace_id = @workspace_id AND member_id = @member_id;

-- name: DeleteMemberAddressRow :exec
DELETE FROM member_addresses WHERE workspace_id = @workspace_id AND id = @id;

-- name: CreateMemberAddress :one
INSERT INTO member_addresses (workspace_id, id, member_id, address, verification, code_hash, code_expires_at, code_sent_at, created_at)
VALUES (@workspace_id, @id, @member_id, @address, @verification, sqlc.narg(code_hash), sqlc.narg(code_expires_at), sqlc.narg(code_sent_at), @now)
RETURNING *;

-- name: StartMemberAddressCode :one
UPDATE member_addresses SET verification = 'code', code_hash = @code_hash, code_expires_at = @code_expires_at, code_attempts = 0, code_sent_at = @now
WHERE workspace_id = @workspace_id AND id = @id AND verified_at IS NULL
RETURNING *;

-- name: StartMemberAddressApproval :one
UPDATE member_addresses SET verification = 'approval', code_hash = NULL, code_expires_at = NULL, code_attempts = 0, code_sent_at = NULL
WHERE workspace_id = @workspace_id AND id = @id AND verified_at IS NULL
RETURNING *;

-- name: FailMemberAddressCode :exec
UPDATE member_addresses SET code_attempts = code_attempts + 1,
    code_hash = CASE WHEN code_attempts + 1 >= @max_attempts::int THEN NULL ELSE code_hash END
WHERE workspace_id = @workspace_id AND id = @id;

-- name: MarkMemberAddressVerified :one
UPDATE member_addresses SET verified_at = @now, code_hash = NULL, code_expires_at = NULL, code_attempts = 0
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteMemberAddress :one
DELETE FROM member_addresses WHERE workspace_id = @workspace_id AND member_id = @member_id AND id = @id
RETURNING *;

-- name: GetVerifiedMemberAddress :one
SELECT * FROM member_addresses
WHERE workspace_id = @workspace_id AND member_id = @member_id AND address = @address AND verified_at IS NOT NULL;

-- name: GetMemberAddressByID :one
SELECT * FROM member_addresses WHERE workspace_id = @workspace_id AND id = @id;

-- Inbound mail names only its recipient, so a member's address is found before the workspace is
-- known (see "Hosting for others later" in docs/architecture.md); only verified addresses route.
-- name: FindVerifiedMemberAddresses :many
SELECT a.workspace_id, a.id, a.member_id FROM member_addresses a
JOIN workspaces w ON w.id = a.workspace_id
WHERE a.address = @address AND a.verified_at IS NOT NULL AND w.deleted_at IS NULL
ORDER BY a.workspace_id;

-- name: FindConversationByEmailTokenAnyInbox :one
SELECT * FROM conversations WHERE workspace_id = @workspace_id AND email_token = ANY(@tokens::text[])
  AND trashed_at IS NULL AND channel_id IS NOT NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: ListInboxEmailChannels :many
SELECT e.* FROM email_channels e
JOIN channels c ON c.workspace_id = e.workspace_id AND c.id = e.channel_id
WHERE e.workspace_id = @workspace_id AND c.inbox_id = ANY(@inbox_ids::uuid[])
ORDER BY c.created_at, c.id;

-- name: SetMessageEmailMemberAddress :exec
UPDATE message_emails SET member_address_id = @member_address_id WHERE workspace_id = @workspace_id AND message_id = @message_id;
