-- name: CreatePerson :one
INSERT INTO people (id, email, name, locale, webauthn_handle) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPerson :one
SELECT * FROM people WHERE id = $1;

-- name: GetPersonByEmail :one
SELECT * FROM people WHERE email = $1;

-- name: GetPersonByWebauthnHandle :one
SELECT * FROM people WHERE webauthn_handle = $1;

-- name: UpdatePerson :one
UPDATE people SET
    name = coalesce(sqlc.narg('name'), name),
    locale = coalesce(sqlc.narg('locale'), locale)
WHERE id = $1
RETURNING *;

-- name: DeletePerson :execrows
DELETE FROM people WHERE id = $1;

-- name: SignInTarget :one
SELECT
    EXISTS (SELECT 1 FROM people p WHERE p.email = @email)
    OR EXISTS (
        SELECT 1 FROM invites i JOIN workspaces w ON w.id = i.workspace_id
        WHERE i.email = @email AND i.expires_at > @now AND w.deleted_at IS NULL
    ) AS known;
