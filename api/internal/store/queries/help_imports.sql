-- name: CreateHelpImport :one
INSERT INTO help_imports (id, workspace_id, center_id, source, migration_id, origin, sealed, options, state, member_id, api_key_id, created_at, updated_at)
VALUES (@id, @workspace_id, @center_id, @source, sqlc.narg(migration_id), @origin, sqlc.narg(sealed), @options, 'queued', sqlc.narg(member_id), sqlc.narg(api_key_id), @now, @now)
RETURNING *;

-- name: GetHelpImport :one
SELECT * FROM help_imports WHERE workspace_id = @workspace_id AND center_id = @center_id AND id = @id;

-- name: GetHelpImportByID :one
SELECT * FROM help_imports WHERE workspace_id = @workspace_id AND id = @id;

-- name: LockHelpImport :one
SELECT * FROM help_imports WHERE workspace_id = @workspace_id AND id = @id FOR UPDATE;

-- name: ListHelpImports :many
SELECT * FROM help_imports
WHERE workspace_id = @workspace_id AND center_id = @center_id
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: StartHelpImportRun :one
UPDATE help_imports SET state = 'running', started_at = coalesce(started_at, @now), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id AND state IN ('queued', 'running')
RETURNING *;

-- name: SaveHelpImportProgress :one
UPDATE help_imports SET phase = @phase, checkpoint = @checkpoint, counts = @counts, origin = @origin, skipped_locales = @skipped_locales::text[], updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id AND state = 'running'
RETURNING *;

-- name: FinishHelpImport :one
UPDATE help_imports SET state = @state, error = sqlc.narg(error), sealed = NULL, checkpoint = '{}', finished_at = @now, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id AND state IN ('queued', 'running')
RETURNING *;

-- name: CountHelpImportRedirects :one
SELECT count(*)::int FROM help_import_items i
WHERE i.workspace_id = @workspace_id AND i.import_id = @import_id AND i.old_url <> '';

-- name: GetHelpImportItem :one
SELECT * FROM help_import_items
WHERE workspace_id = @workspace_id AND center_id = @center_id AND origin = @origin AND kind = @kind AND source_id = @source_id AND locale = @locale;

-- name: FindHelpImportTarget :one
SELECT target_id FROM help_import_items
WHERE workspace_id = @workspace_id AND center_id = @center_id AND origin = @origin AND kind = @kind AND source_id = @source_id
ORDER BY locale
LIMIT 1;

-- name: UpsertHelpImportItem :exec
INSERT INTO help_import_items (workspace_id, center_id, origin, kind, source_id, locale, link_key, target_id, old_url, version, content_hash, import_id, links_pending, updated_at)
VALUES (@workspace_id, @center_id, @origin, @kind, @source_id, @locale, @link_key, @target_id, @old_url, @version, @content_hash, @import_id, @links_pending, @now)
ON CONFLICT (workspace_id, center_id, origin, kind, source_id, locale) DO UPDATE SET
    link_key = EXCLUDED.link_key, target_id = EXCLUDED.target_id, old_url = EXCLUDED.old_url, version = EXCLUDED.version,
    content_hash = EXCLUDED.content_hash, import_id = EXCLUDED.import_id, links_pending = EXCLUDED.links_pending, updated_at = EXCLUDED.updated_at;

-- name: SeeHelpImportItem :exec
UPDATE help_import_items SET import_id = @import_id, old_url = @old_url, link_key = @link_key, updated_at = @now
WHERE workspace_id = @workspace_id AND center_id = @center_id AND origin = @origin AND kind = @kind AND source_id = @source_id AND locale = @locale;

-- name: ListHelpImportPendingLinks :many
SELECT * FROM help_import_items
WHERE workspace_id = @workspace_id AND import_id = @import_id AND kind = 'article' AND links_pending
  AND (source_id, locale) > (@after_source::text, @after_locale::text)
ORDER BY source_id, locale
LIMIT @lim;

-- name: SetHelpImportItemLinked :exec
UPDATE help_import_items SET links_pending = false, version = @version, updated_at = @now
WHERE workspace_id = @workspace_id AND center_id = @center_id AND origin = @origin AND kind = 'article' AND source_id = @source_id AND locale = @locale;

-- name: FindHelpImportArticlePages :many
SELECT i.locale, i.target_id AS article_id, a.number, t.slug
FROM help_import_items i
JOIN help_articles a ON a.workspace_id = i.workspace_id AND a.id = i.target_id
JOIN help_article_translations t ON t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.locale = i.locale
WHERE i.workspace_id = @workspace_id AND i.center_id = @center_id AND i.origin = @origin AND i.kind = 'article'
  AND (i.link_key = @link_key::text OR i.old_url = @old_url::text)
ORDER BY i.locale;

-- name: ListHelpImportRedirects :many
SELECT i.kind, i.locale, i.old_url, i.target_id, i.source_id,
    coalesce(a.number, c.number)::int AS number,
    coalesce(t.slug, ct.slug, '')::text AS slug,
    CASE WHEN i.kind = 'article' THEN coalesce(t.published_at IS NOT NULL, false)
         ELSE EXISTS (
            SELECT 1 FROM help_articles aa
            JOIN help_article_translations tt ON tt.workspace_id = aa.workspace_id AND tt.article_id = aa.id
            WHERE aa.workspace_id = i.workspace_id AND tt.locale = i.locale AND tt.published_at IS NOT NULL
              AND (aa.collection_id = i.target_id OR aa.collection_id IN (
                    SELECT cc.id FROM help_collections cc WHERE cc.workspace_id = i.workspace_id AND cc.parent_id = i.target_id)))
    END::bool AS published
FROM help_import_items i
LEFT JOIN help_articles a ON i.kind = 'article' AND a.workspace_id = i.workspace_id AND a.id = i.target_id
LEFT JOIN help_article_translations t ON t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.locale = i.locale
LEFT JOIN help_collections c ON i.kind = 'collection' AND c.workspace_id = i.workspace_id AND c.id = i.target_id
LEFT JOIN help_collection_translations ct ON ct.workspace_id = c.workspace_id AND ct.collection_id = c.id AND ct.locale = i.locale
WHERE i.workspace_id = @workspace_id AND i.import_id = @import_id AND i.old_url <> '' AND (a.id IS NOT NULL OR c.id IS NOT NULL)
  AND (i.kind, i.source_id, i.locale) > (@after_kind::text, @after_source::text, @after_locale::text)
ORDER BY i.kind, i.source_id, i.locale
LIMIT @lim;
