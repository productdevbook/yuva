-- name: CreateChangelog :one
INSERT INTO changelogs (id, workspace_id, inbox_id, name, description, slug, default_locale, locales, analytics_property_id,
    agents_may_publish, published, changed_at, created_at, updated_at)
VALUES (@id, @workspace_id, @inbox_id, @name, @description, @slug, @default_locale, @locales::text[], sqlc.narg(analytics_property_id),
    @agents_may_publish, @published, @now, @now, @now)
RETURNING *;

-- name: GetChangelog :one
SELECT * FROM changelogs WHERE workspace_id = $1 AND id = $2;

-- name: LockChangelog :one
SELECT * FROM changelogs WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: GetInboxChangelog :one
SELECT * FROM changelogs WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id;

-- name: ListChangelogs :many
SELECT * FROM changelogs c
WHERE c.workspace_id = @workspace_id
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR c.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id))
ORDER BY c.created_at, c.id;

-- name: UpdateChangelog :one
UPDATE changelogs SET name = @name, description = @description, slug = @slug, default_locale = @default_locale, locales = @locales::text[],
    analytics_property_id = sqlc.narg(analytics_property_id), agents_may_publish = @agents_may_publish, published = @published,
    changed_at = greatest(changed_at + interval '1 microsecond', @now::timestamptz), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: TouchChangelog :exec
UPDATE changelogs SET changed_at = greatest(changed_at + interval '1 microsecond', @now::timestamptz)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteChangelog :execrows
DELETE FROM changelogs WHERE workspace_id = $1 AND id = $2;

-- name: ChangelogSlugTaken :one
-- The documented exception: slugs, current and former, are unique across the server, so availability is checked across workspaces.
SELECT (EXISTS (SELECT 1 FROM changelogs c WHERE c.slug = @slug AND c.id <> @id)
    OR EXISTS (SELECT 1 FROM changelog_slugs f WHERE f.slug = @slug AND f.changelog_id <> @id))::bool;

-- name: AddChangelogFormerSlug :exec
INSERT INTO changelog_slugs (slug, workspace_id, changelog_id, created_at) VALUES (@slug, @workspace_id, @changelog_id, @now);

-- name: PruneChangelogFormerSlugs :exec
DELETE FROM changelog_slugs f
WHERE f.workspace_id = @workspace_id AND f.changelog_id = @changelog_id
  AND f.slug NOT IN (
      SELECT k.slug FROM changelog_slugs k
      WHERE k.workspace_id = @workspace_id AND k.changelog_id = @changelog_id
      ORDER BY k.created_at DESC, k.slug LIMIT @keep);

-- name: DeleteOldChangelogFormerSlugs :execrows
DELETE FROM changelog_slugs WHERE workspace_id = @workspace_id AND created_at < @before;

-- name: ReleaseChangelogFormerSlug :exec
DELETE FROM changelog_slugs WHERE workspace_id = @workspace_id AND changelog_id = @changelog_id AND slug = @slug;

-- name: CountChangelog :one
SELECT
    count(*)::int AS entries,
    (count(*) FILTER (WHERE e.state = 'published'))::int AS published,
    (count(*) FILTER (WHERE e.state = 'scheduled'))::int AS scheduled
FROM changelog_entries e WHERE e.workspace_id = @workspace_id AND e.changelog_id = @changelog_id;

-- name: TakeChangelogEntryNumber :one
UPDATE changelogs SET next_entry_number = next_entry_number + 1
WHERE workspace_id = @workspace_id AND id = @id
RETURNING (next_entry_number - 1)::int AS number;

-- name: ChangelogLocalesInUse :many
SELECT DISTINCT t.locale FROM changelog_entry_translations t
JOIN changelog_entries e ON e.workspace_id = t.workspace_id AND e.id = t.entry_id
WHERE t.workspace_id = @workspace_id AND e.changelog_id = @changelog_id AND NOT (t.locale = ANY (@locales::text[]));

-- name: CountWorkspaceChangelogEntries :one
SELECT count(*)::int FROM changelog_entries WHERE workspace_id = @workspace_id;

-- name: LockChangelogEntryQuota :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_changelog_entries:' || sqlc.arg(workspace_id)::uuid::text, 0));

-- name: CountScheduledChangelogEntries :one
SELECT count(*)::int FROM changelog_entries
WHERE workspace_id = @workspace_id AND changelog_id = @changelog_id AND state = 'scheduled' AND id <> @except_id;

-- name: CreateChangelogEntry :one
INSERT INTO changelog_entries (id, workspace_id, changelog_id, number, labels, image_id, created_by_member_id, created_by_api_key_id, created_at, updated_at)
VALUES (@id, @workspace_id, @changelog_id, @number, @labels::text[], sqlc.narg(image_id), sqlc.narg(member_id), sqlc.narg(api_key_id), @now, @now)
RETURNING *;

-- name: GetChangelogEntry :one
SELECT * FROM changelog_entries WHERE workspace_id = @workspace_id AND changelog_id = @changelog_id AND id = @id;

-- name: LockChangelogEntry :one
SELECT * FROM changelog_entries WHERE workspace_id = @workspace_id AND changelog_id = @changelog_id AND id = @id FOR UPDATE;

-- name: UpdateChangelogEntry :one
UPDATE changelog_entries SET labels = @labels::text[], image_id = sqlc.narg(image_id), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: TouchChangelogEntry :exec
UPDATE changelog_entries SET updated_at = @now WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteChangelogEntry :execrows
DELETE FROM changelog_entries WHERE workspace_id = @workspace_id AND changelog_id = @changelog_id AND id = @id;

-- name: ListChangelogEntries :many
SELECT e.* FROM changelog_entries e
WHERE e.workspace_id = @workspace_id AND e.changelog_id = @changelog_id
  AND (sqlc.narg(state)::text IS NULL OR CASE sqlc.narg(state)::text
      WHEN 'changed' THEN e.state = 'published' AND EXISTS (
          SELECT 1 FROM changelog_entry_translations t
          WHERE t.workspace_id = e.workspace_id AND t.entry_id = e.id AND t.has_unpublished_changes
            AND (sqlc.narg(locale)::text IS NULL OR t.locale = sqlc.narg(locale)::text))
      ELSE e.state = sqlc.narg(state)::text END)
  AND (sqlc.narg(label)::text IS NULL OR sqlc.narg(label)::text = ANY (e.labels))
  AND (sqlc.narg(locale)::text IS NULL OR EXISTS (
      SELECT 1 FROM changelog_entry_translations t
      WHERE t.workspace_id = e.workspace_id AND t.entry_id = e.id AND t.locale = sqlc.narg(locale)::text))
  AND (sqlc.narg(cursor_number)::int IS NULL OR e.number < sqlc.narg(cursor_number)::int)
ORDER BY e.number DESC
LIMIT @lim;

-- name: ScheduleChangelogEntry :one
UPDATE changelog_entries SET state = 'scheduled', publish_at = @publish_at, scheduled_by_member_id = sqlc.narg(member_id),
    scheduled_by_api_key_id = sqlc.narg(api_key_id), schedule_failed = NULL, schedule_failed_at = NULL, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: UnscheduleChangelogEntry :one
UPDATE changelog_entries SET state = 'draft', publish_at = NULL, scheduled_by_member_id = NULL, scheduled_by_api_key_id = NULL, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id AND state = 'scheduled'
RETURNING *;

-- name: FailChangelogSchedule :one
UPDATE changelog_entries SET state = 'draft', publish_at = NULL, schedule_failed = @reason, schedule_failed_at = @now::timestamptz, updated_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id AND state = 'scheduled'
RETURNING *;

-- name: PublishChangelogEntry :one
UPDATE changelog_entries SET state = 'published', published_at = coalesce(published_at, @now::timestamptz), publish_at = NULL,
    schedule_failed = NULL, schedule_failed_at = NULL, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: PublishDueChangelogEntry :one
UPDATE changelog_entries SET state = 'published', published_at = coalesce(published_at, @now::timestamptz), publish_at = NULL, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id AND state = 'scheduled' AND publish_at <= @now::timestamptz
RETURNING *;

-- name: UnpublishChangelogEntry :one
UPDATE changelog_entries SET state = 'draft', updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id AND state = 'published'
RETURNING *;

-- name: DueChangelogEntries :many
-- The documented exception: the sweep reads across workspaces; each publish then works in the entry's own workspace.
SELECT id, workspace_id FROM changelog_entries WHERE state = 'scheduled' AND publish_at <= @now::timestamptz ORDER BY publish_at LIMIT @lim;

-- name: CreateChangelogTranslation :one
INSERT INTO changelog_entry_translations (workspace_id, entry_id, locale, title, body, slug, image_ids,
    updated_by_member_id, updated_by_api_key_id, created_at, updated_at)
VALUES (@workspace_id, @entry_id, @locale, @title, @body, @slug, @image_ids::uuid[], sqlc.narg(member_id), sqlc.narg(api_key_id), @now, @now)
RETURNING *;

-- name: GetChangelogTranslation :one
SELECT * FROM changelog_entry_translations WHERE workspace_id = @workspace_id AND entry_id = @entry_id AND locale = @locale;

-- name: LockChangelogTranslation :one
SELECT * FROM changelog_entry_translations WHERE workspace_id = @workspace_id AND entry_id = @entry_id AND locale = @locale FOR UPDATE;

-- name: ListChangelogTranslations :many
SELECT * FROM changelog_entry_translations
WHERE workspace_id = @workspace_id AND entry_id = ANY (@entry_ids::uuid[])
ORDER BY entry_id, locale;

-- name: ListChangelogTranslationSummaries :many
SELECT entry_id, locale, title, slug, version, published_title, published_slug, published_at, has_unpublished_changes, updated_at
FROM changelog_entry_translations
WHERE workspace_id = @workspace_id AND entry_id = ANY (@entry_ids::uuid[])
ORDER BY entry_id, locale;

-- name: SaveChangelogTranslation :one
UPDATE changelog_entry_translations SET title = @title, body = @body, slug = @slug, image_ids = @image_ids::uuid[],
    version = version + 1, updated_by_member_id = sqlc.narg(member_id), updated_by_api_key_id = sqlc.narg(api_key_id), updated_at = @now
WHERE workspace_id = @workspace_id AND entry_id = @entry_id AND locale = @locale
RETURNING *;

-- name: PublishChangelogTranslation :one
UPDATE changelog_entry_translations SET published_title = title, published_body = body, published_slug = slug,
    published_html = @html, published_excerpt = @excerpt, published_at = @now,
    published_by_member_id = sqlc.narg(member_id), published_by_api_key_id = sqlc.narg(api_key_id)
WHERE workspace_id = @workspace_id AND entry_id = @entry_id AND locale = @locale
RETURNING *;

-- name: UnpublishChangelogTranslations :exec
UPDATE changelog_entry_translations SET published_title = NULL, published_body = NULL, published_slug = NULL,
    published_html = NULL, published_excerpt = NULL, published_at = NULL, published_by_member_id = NULL, published_by_api_key_id = NULL
WHERE workspace_id = @workspace_id AND entry_id = @entry_id;

-- name: DeleteChangelogTranslation :execrows
DELETE FROM changelog_entry_translations WHERE workspace_id = @workspace_id AND entry_id = @entry_id AND locale = @locale;

-- name: CountChangelogTranslations :one
SELECT count(*)::int FROM changelog_entry_translations WHERE workspace_id = @workspace_id AND entry_id = @entry_id;

-- name: ListPublicChangelogEntries :many
SELECT e.id, e.number, e.labels, e.published_at::timestamptz AS published_at,
    t.locale, t.published_title::text AS title, t.published_slug::text AS slug, t.published_excerpt::text AS excerpt, t.published_html::text AS html,
    i.token AS image_token, i.width AS image_width, i.height AS image_height
FROM changelog_entries e
JOIN LATERAL (
    SELECT x.* FROM changelog_entry_translations x
    WHERE x.workspace_id = e.workspace_id AND x.entry_id = e.id AND x.published_at IS NOT NULL AND x.locale IN (@locale::text, @default_locale::text)
    ORDER BY (x.locale = @locale::text) DESC
    LIMIT 1) t ON true
LEFT JOIN email_images i ON i.workspace_id = e.workspace_id AND i.id = e.image_id
WHERE e.workspace_id = @workspace_id AND e.changelog_id = @changelog_id AND e.state = 'published'
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (e.published_at, e.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY e.published_at DESC, e.id DESC
LIMIT @lim;

-- name: GetPublicChangelogEntry :one
SELECT e.id, e.number, e.labels, e.published_at::timestamptz AS published_at,
    t.locale, t.published_title::text AS title, t.published_slug::text AS slug, t.published_excerpt::text AS excerpt, t.published_html::text AS html,
    i.token AS image_token, i.width AS image_width, i.height AS image_height
FROM changelog_entries e
JOIN LATERAL (
    SELECT x.* FROM changelog_entry_translations x
    WHERE x.workspace_id = e.workspace_id AND x.entry_id = e.id AND x.published_at IS NOT NULL AND x.locale IN (@locale::text, @default_locale::text)
    ORDER BY (x.locale = @locale::text) DESC
    LIMIT 1) t ON true
LEFT JOIN email_images i ON i.workspace_id = e.workspace_id AND i.id = e.image_id
WHERE e.workspace_id = @workspace_id AND e.changelog_id = @changelog_id AND e.id = @id AND e.state = 'published';

-- name: LatestChangelogPublishedAt :one
SELECT e.published_at::timestamptz AS latest FROM changelog_entries e
WHERE e.workspace_id = @workspace_id AND e.changelog_id = @changelog_id AND e.state = 'published'
ORDER BY e.published_at DESC
LIMIT 1;

-- name: GetChangelogEntryByID :one
SELECT * FROM changelog_entries WHERE workspace_id = @workspace_id AND id = @id;

-- name: CountChangelogEntriesMissingLocale :one
SELECT count(*)::int FROM changelog_entries e
WHERE e.workspace_id = @workspace_id AND e.changelog_id = @changelog_id AND e.state <> 'draft'
  AND NOT EXISTS (
      SELECT 1 FROM changelog_entry_translations t
      WHERE t.workspace_id = e.workspace_id AND t.entry_id = e.id AND t.locale = @locale);

-- name: ListChangelogEntriesUsingImage :many
SELECT e.id, e.changelog_id, e.number,
    coalesce((SELECT t.title FROM changelog_entry_translations t
        WHERE t.workspace_id = e.workspace_id AND t.entry_id = e.id ORDER BY t.created_at, t.locale LIMIT 1), '')::text AS title
FROM changelog_entries e
WHERE e.workspace_id = @workspace_id AND (
    e.image_id = @image_id
    OR EXISTS (SELECT 1 FROM changelog_entry_translations t
        WHERE t.workspace_id = e.workspace_id AND t.entry_id = e.id AND t.image_ids @> ARRAY[@image_id::uuid]))
ORDER BY e.number
LIMIT 20;

-- name: FindChangelogBySlug :one
-- The documented exception: a public URL names only the slug, which is unique across the server.
SELECT * FROM changelogs WHERE slug = @slug;

-- name: FindChangelogByFormerSlug :one
-- The documented exception, as FindChangelogBySlug, for a slug the changelog no longer uses.
SELECT c.* FROM changelog_slugs f
JOIN changelogs c ON c.workspace_id = f.workspace_id AND c.id = f.changelog_id
WHERE f.slug = @slug;

-- name: ListPublicChangelogPage :many
SELECT e.id, e.number, e.labels, e.published_at::timestamptz AS published_at,
    t.published_title::text AS title, t.published_slug::text AS slug, t.published_excerpt::text AS excerpt, t.published_html::text AS html,
    t.published_at::timestamptz AS translation_published_at,
    i.token AS image_token, i.width AS image_width, i.height AS image_height
FROM changelog_entries e
JOIN changelog_entry_translations t ON t.workspace_id = e.workspace_id AND t.entry_id = e.id AND t.locale = @locale AND t.published_at IS NOT NULL
LEFT JOIN email_images i ON i.workspace_id = e.workspace_id AND i.id = e.image_id
WHERE e.workspace_id = @workspace_id AND e.changelog_id = @changelog_id AND e.state = 'published'
  AND (sqlc.narg(label)::text IS NULL OR sqlc.narg(label)::text = ANY (e.labels))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (e.published_at, e.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY e.published_at DESC, e.id DESC
LIMIT @lim;

-- name: GetPublicChangelogEntryByNumber :one
SELECT e.id, e.number, e.labels, e.published_at::timestamptz AS published_at,
    t.published_title::text AS title, t.published_slug::text AS slug, t.published_excerpt::text AS excerpt, t.published_html::text AS html,
    t.published_at::timestamptz AS translation_published_at,
    i.token AS image_token, i.width AS image_width, i.height AS image_height
FROM changelog_entries e
JOIN changelog_entry_translations t ON t.workspace_id = e.workspace_id AND t.entry_id = e.id AND t.locale = @locale AND t.published_at IS NOT NULL
LEFT JOIN email_images i ON i.workspace_id = e.workspace_id AND i.id = e.image_id
WHERE e.workspace_id = @workspace_id AND e.changelog_id = @changelog_id AND e.number = @number AND e.state = 'published';

-- name: ListPublicChangelogEntryLocales :many
SELECT t.locale, t.published_slug::text AS slug FROM changelog_entry_translations t
JOIN changelog_entries e ON e.workspace_id = t.workspace_id AND e.id = t.entry_id
WHERE t.workspace_id = @workspace_id AND e.id = @entry_id AND e.state = 'published' AND t.published_at IS NOT NULL
ORDER BY t.locale;

-- name: ListPublicChangelogLocales :many
SELECT DISTINCT t.locale FROM changelog_entry_translations t
JOIN changelog_entries e ON e.workspace_id = t.workspace_id AND e.id = t.entry_id
WHERE t.workspace_id = @workspace_id AND e.changelog_id = @changelog_id AND e.state = 'published' AND t.published_at IS NOT NULL
ORDER BY t.locale;

-- name: ListChangelogSitemapEntries :many
SELECT e.number, t.locale, t.published_slug::text AS slug, t.published_at::timestamptz AS published_at
FROM changelog_entries e
JOIN changelog_entry_translations t ON t.workspace_id = e.workspace_id AND t.entry_id = e.id AND t.published_at IS NOT NULL
WHERE e.workspace_id = @workspace_id AND e.changelog_id = @changelog_id AND e.state = 'published'
ORDER BY e.number, t.locale;
