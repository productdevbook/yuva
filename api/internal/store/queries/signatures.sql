-- name: GetMemberSignature :one
SELECT * FROM member_signatures WHERE workspace_id = @workspace_id AND member_id = @member_id AND inbox_id = @inbox_id;

-- name: ListMemberSignatures :many
SELECT * FROM member_signatures WHERE workspace_id = @workspace_id AND member_id = @member_id;

-- name: UpsertMemberSignature :one
INSERT INTO member_signatures (workspace_id, member_id, inbox_id, body, image_ids, updated_at)
VALUES (@workspace_id, @member_id, @inbox_id, @body, @image_ids::uuid[], @now)
ON CONFLICT (workspace_id, member_id, inbox_id) DO UPDATE SET body = EXCLUDED.body, image_ids = EXCLUDED.image_ids, updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: DeleteMemberSignature :execrows
DELETE FROM member_signatures WHERE workspace_id = @workspace_id AND member_id = @member_id AND inbox_id = @inbox_id;

-- name: ListInboxesUsingSignatureImage :many
SELECT id, name FROM inboxes
WHERE workspace_id = @workspace_id AND signature_image_ids @> ARRAY[@image_id::uuid]
ORDER BY name, id
LIMIT 20;

-- name: ListMemberSignaturesUsingImage :many
SELECT s.member_id, p.name, s.inbox_id
FROM member_signatures s
JOIN members m ON m.workspace_id = s.workspace_id AND m.id = s.member_id
JOIN people p ON p.id = m.person_id
WHERE s.workspace_id = @workspace_id AND s.image_ids @> ARRAY[@image_id::uuid]
ORDER BY p.name, s.member_id, s.inbox_id
LIMIT 20;
