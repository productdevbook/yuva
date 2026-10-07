-- name: CreateLoginCode :exec
INSERT INTO login_codes (id, email, code_hash, ip, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: CountLoginCodesByEmail :one
SELECT count(*) FROM login_codes WHERE email = $1 AND created_at > $2;

-- name: CountLoginCodesByIP :one
SELECT count(*) FROM login_codes WHERE ip = $1 AND created_at > $2;

-- name: LatestLoginCode :one
SELECT * FROM login_codes WHERE email = $1 AND consumed_at IS NULL
ORDER BY created_at DESC LIMIT 1
FOR UPDATE;

-- name: AddLoginCodeAttempt :one
UPDATE login_codes SET attempts = attempts + 1 WHERE id = $1 RETURNING attempts;

-- name: ConsumeLoginCode :exec
UPDATE login_codes SET consumed_at = $2 WHERE id = $1;
