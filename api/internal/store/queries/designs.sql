-- name: CreateEmailDesign :one
INSERT INTO email_designs (id, workspace_id, inbox_id, name, document, template_slug, created_by, updated_by, created_at, updated_at)
VALUES (@id, @workspace_id, sqlc.narg(inbox_id), @name, @document, sqlc.narg(template_slug), sqlc.narg(member_id), sqlc.narg(member_id), @now, @now)
RETURNING *;

-- name: GetEmailDesign :one
SELECT * FROM email_designs WHERE workspace_id = $1 AND id = $2;

-- name: LockEmailDesign :one
SELECT * FROM email_designs WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: SaveEmailDesign :one
UPDATE email_designs SET name = @name, inbox_id = sqlc.narg(inbox_id), document = @document, version = version + 1,
    updated_by = sqlc.narg(member_id), archived_at = NULL, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: SetEmailDesignOffered :one
UPDATE email_designs SET for_conversations = @for_conversations
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: ArchiveEmailDesign :execrows
UPDATE email_designs SET archived_at = coalesce(archived_at, @now::timestamptz)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListEmailDesigns :many
SELECT id, workspace_id, inbox_id, name, version, template_slug, created_by, updated_by, archived_at, created_at, updated_at, for_conversations
FROM email_designs d
WHERE d.workspace_id = @workspace_id
  AND @archived::bool = (d.archived_at IS NOT NULL)
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR d.inbox_id = sqlc.narg(inbox_id)::uuid OR (@with_shared::bool AND d.inbox_id IS NULL))
  AND (sqlc.narg(for_conversations)::bool IS NULL OR d.for_conversations = sqlc.narg(for_conversations)::bool)
  AND (sqlc.narg(q)::text IS NULL OR d.name ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (d.inbox_id IS NULL OR (@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = d.workspace_id AND pmb.id = d.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = d.workspace_id AND iv.inbox_id = d.inbox_id AND iv.viewer_id = @viewer_id))
  AND NOT EXISTS (SELECT 1 FROM inboxes own WHERE own.workspace_id = d.workspace_id AND own.id = d.inbox_id AND own.personal AND own.owner_member_id IS DISTINCT FROM @viewer_id::uuid)

  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (d.updated_at, d.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY d.updated_at DESC, d.id DESC
LIMIT @lim;

-- name: ListDesignsUsingImage :many
SELECT id, name FROM email_designs
WHERE workspace_id = @workspace_id AND archived_at IS NULL
  AND jsonb_path_exists(document, '$.**.image_id ? (@ == $id)', jsonb_build_object('id', @image_id::text))
ORDER BY updated_at DESC, id DESC
LIMIT 20;

-- name: InsertEmailDesignVersion :exec
INSERT INTO email_design_versions (workspace_id, design_id, version, document, reason, member_id, created_at)
VALUES (@workspace_id, @design_id, @version, @document, @reason, sqlc.narg(member_id), @now)
ON CONFLICT (workspace_id, design_id, version) DO UPDATE SET reason = EXCLUDED.reason;

-- name: LatestEmailDesignVersionAt :one
SELECT coalesce(max(created_at), 'epoch'::timestamptz)::timestamptz FROM email_design_versions WHERE workspace_id = @workspace_id AND design_id = @design_id;

-- name: PruneEmailDesignVersions :exec
DELETE FROM email_design_versions ev
WHERE ev.workspace_id = @workspace_id AND ev.design_id = @design_id
  AND ev.version NOT IN (
      SELECT k.version FROM email_design_versions k
      WHERE k.workspace_id = @workspace_id AND k.design_id = @design_id
      ORDER BY k.version = (
          SELECT max(c.version) FROM email_design_versions c
          WHERE c.workspace_id = @workspace_id AND c.design_id = @design_id AND c.reason = 'conflict'
      ) IS TRUE DESC, k.version DESC LIMIT @keep);

-- name: InsertEmailDesignConflictVersion :exec
INSERT INTO email_design_versions (workspace_id, design_id, version, document, reason, member_id, created_at)
VALUES (@workspace_id, @design_id, @version, @document, 'conflict', sqlc.narg(member_id), @now)
ON CONFLICT (workspace_id, design_id, version) DO NOTHING;

-- name: ListEmailDesignVersions :many
SELECT version, reason, member_id, created_at FROM email_design_versions
WHERE workspace_id = @workspace_id AND design_id = @design_id
ORDER BY version DESC;

-- name: GetEmailDesignVersion :one
SELECT * FROM email_design_versions WHERE workspace_id = @workspace_id AND design_id = @design_id AND version = @version;

-- name: CreateEmailImage :one
INSERT INTO email_images (id, workspace_id, token, storage_key, content_type, width, height, size_bytes, sha256, filename, member_id, created_at)
VALUES (@id, @workspace_id, @token, @storage_key, @content_type, @width, @height, @size_bytes, @sha256, @filename, sqlc.narg(member_id), @now)
ON CONFLICT (workspace_id, sha256) DO NOTHING
RETURNING *;

-- name: GetEmailImage :one
SELECT * FROM email_images WHERE workspace_id = $1 AND id = $2;

-- name: LockEmailImage :one
SELECT * FROM email_images WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: GetEmailImageBySHA256 :one
SELECT * FROM email_images WHERE workspace_id = @workspace_id AND sha256 = @sha256;

-- name: GetPublicEmailImage :one
SELECT i.* FROM email_images i JOIN workspaces w ON w.id = i.workspace_id
WHERE i.token = @token AND w.deleted_at IS NULL;

-- name: ListEmailImages :many
SELECT * FROM email_images
WHERE workspace_id = @workspace_id
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (created_at, id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: ListEmailImagesByIDs :many
SELECT * FROM email_images WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);

-- name: LockEmailImageQuota :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_email_images:' || sqlc.arg(workspace_id)::uuid::text, 0));

-- name: SumEmailImageBytes :one
SELECT coalesce(sum(size_bytes), 0)::bigint FROM email_images WHERE workspace_id = @workspace_id;

-- name: DeleteEmailImage :execrows
DELETE FROM email_images WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteUnusedEmailImages :many
DELETE FROM email_images i
WHERE i.id IN (
    SELECT c.id FROM email_images c
    WHERE c.workspace_id = @workspace_id AND c.created_at < @before AND c.sent_at IS NULL
      AND NOT EXISTS (
          SELECT 1 FROM email_designs d
          WHERE d.workspace_id = c.workspace_id
            AND jsonb_path_exists(d.document, '$.**.image_id ? (@ == $id)', jsonb_build_object('id', c.id::text)))
      AND NOT EXISTS (
          SELECT 1 FROM help_article_translations t
          WHERE t.workspace_id = c.workspace_id AND t.image_ids @> ARRAY[c.id])
      AND NOT EXISTS (
          SELECT 1 FROM help_article_revisions r
          WHERE r.workspace_id = c.workspace_id AND r.image_ids @> ARRAY[c.id])
      AND NOT EXISTS (
          SELECT 1 FROM changelog_entries e
          WHERE e.workspace_id = c.workspace_id AND e.image_id = c.id)
      AND NOT EXISTS (
          SELECT 1 FROM changelog_entry_translations t
          WHERE t.workspace_id = c.workspace_id AND t.image_ids @> ARRAY[c.id])
      AND NOT EXISTS (
          SELECT 1 FROM member_signatures s
          WHERE s.workspace_id = c.workspace_id AND s.image_ids @> ARRAY[c.id])
      AND NOT EXISTS (
          SELECT 1 FROM inboxes b
          WHERE b.workspace_id = c.workspace_id AND b.signature_image_ids @> ARRAY[c.id])
    LIMIT @lim)
RETURNING i.storage_key;

-- name: MarkEmailImagesSent :exec
UPDATE email_images SET sent_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]) AND sent_at IS NULL;

-- name: DeleteWorkspaceEmailImages :many
DELETE FROM email_images i WHERE i.id IN (
    SELECT c.id FROM email_images c WHERE c.workspace_id = @workspace_id LIMIT @lim)
RETURNING i.storage_key;
