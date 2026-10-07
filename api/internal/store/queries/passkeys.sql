-- name: CreatePasskey :one
INSERT INTO passkeys (id, person_id, rp_id, credential_id, name, credential)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListPasskeys :many
SELECT * FROM passkeys WHERE person_id = $1 AND rp_id = $2 ORDER BY created_at, id;

-- name: DeletePasskey :execrows
DELETE FROM passkeys WHERE person_id = $1 AND id = $2;

-- name: UpdatePasskeyUse :exec
UPDATE passkeys SET credential = $3, last_used_at = $4 WHERE person_id = $1 AND id = $2;

-- name: CreateWebauthnCeremony :exec
INSERT INTO webauthn_ceremonies (id_hash, kind, person_id, session_data, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: TakeWebauthnCeremony :one
DELETE FROM webauthn_ceremonies WHERE id_hash = $1 AND kind = $2
RETURNING *;

-- name: DeleteExpiredWebauthnCeremonies :exec
DELETE FROM webauthn_ceremonies WHERE expires_at < $1;
