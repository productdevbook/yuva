-- name: CreateHelpCenter :one
INSERT INTO help_centers (id, workspace_id, inbox_id, name, description, slug, default_locale, locales, logo_url, accent_color, site_url,
    chat_channel_id, analytics_property_id, agents_may_publish, published, changed_at, created_at, updated_at)
VALUES (@id, @workspace_id, @inbox_id, @name, @description, @slug, @default_locale, @locales::text[], sqlc.narg(logo_url), sqlc.narg(accent_color),
    sqlc.narg(site_url), sqlc.narg(chat_channel_id), sqlc.narg(analytics_property_id), @agents_may_publish, @published, @now, @now, @now)
RETURNING *;

-- name: GetHelpCenter :one
SELECT * FROM help_centers WHERE workspace_id = $1 AND id = $2;

-- name: LockHelpCenter :one
SELECT * FROM help_centers WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: GetInboxHelpCenter :one
SELECT * FROM help_centers WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id;

-- name: ListHelpCenters :many
SELECT * FROM help_centers c
WHERE c.workspace_id = @workspace_id
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR c.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = c.workspace_id AND pmb.id = c.inbox_id AND pmb.personal)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id))
ORDER BY c.created_at, c.id;

-- name: UpdateHelpCenter :one
UPDATE help_centers SET name = @name, description = @description, slug = @slug, default_locale = @default_locale, locales = @locales::text[],
    logo_url = sqlc.narg(logo_url), accent_color = sqlc.narg(accent_color), site_url = sqlc.narg(site_url),
    chat_channel_id = sqlc.narg(chat_channel_id), analytics_property_id = sqlc.narg(analytics_property_id),
    agents_may_publish = @agents_may_publish, published = @published,
    changed_at = greatest(changed_at + interval '1 microsecond', @now::timestamptz), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: TouchHelpCenter :exec
UPDATE help_centers SET changed_at = greatest(changed_at + interval '1 microsecond', @now::timestamptz)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteHelpCenter :execrows
DELETE FROM help_centers WHERE workspace_id = $1 AND id = $2;

-- name: HelpSlugTaken :one
-- The documented exception: slugs, current and former, are unique across the server, so availability is checked across workspaces.
SELECT (EXISTS (SELECT 1 FROM help_centers c WHERE c.slug = @slug AND c.id <> @id)
    OR EXISTS (SELECT 1 FROM help_center_slugs f WHERE f.slug = @slug AND f.center_id <> @id))::bool;

-- name: AddHelpFormerSlug :exec
INSERT INTO help_center_slugs (slug, workspace_id, center_id, created_at) VALUES (@slug, @workspace_id, @center_id, @now);

-- name: PruneHelpFormerSlugs :exec
DELETE FROM help_center_slugs f
WHERE f.workspace_id = @workspace_id AND f.center_id = @center_id
  AND f.slug NOT IN (
      SELECT k.slug FROM help_center_slugs k
      WHERE k.workspace_id = @workspace_id AND k.center_id = @center_id
      ORDER BY k.created_at DESC, k.slug LIMIT @keep);

-- name: DeleteOldHelpFormerSlugs :execrows
DELETE FROM help_center_slugs WHERE workspace_id = @workspace_id AND created_at < @before;

-- name: ReleaseHelpFormerSlug :exec
DELETE FROM help_center_slugs WHERE workspace_id = @workspace_id AND center_id = @center_id AND slug = @slug;

-- name: CountHelpCenter :one
SELECT
    (SELECT count(*) FROM help_collections c WHERE c.workspace_id = @workspace_id AND c.center_id = @center_id)::int AS collections,
    (SELECT count(*) FROM help_articles a WHERE a.workspace_id = @workspace_id AND a.center_id = @center_id)::int AS articles,
    (SELECT count(*) FROM help_articles a WHERE a.workspace_id = @workspace_id AND a.center_id = @center_id
        AND EXISTS (SELECT 1 FROM help_article_translations t
            WHERE t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.published_at IS NOT NULL))::int AS published_articles;

-- name: TakeHelpArticleNumber :one
UPDATE help_centers SET next_article_number = next_article_number + 1
WHERE workspace_id = @workspace_id AND id = @id
RETURNING (next_article_number - 1)::int AS number;

-- name: TakeHelpCollectionNumber :one
UPDATE help_centers SET next_collection_number = next_collection_number + 1
WHERE workspace_id = @workspace_id AND id = @id
RETURNING (next_collection_number - 1)::int AS number;

-- name: HelpLocalesInUse :many
SELECT DISTINCT t.locale FROM help_article_translations t
JOIN help_articles a ON a.workspace_id = t.workspace_id AND a.id = t.article_id
WHERE t.workspace_id = @workspace_id AND a.center_id = @center_id AND NOT (t.locale = ANY (@locales::text[]))
UNION
SELECT DISTINCT ct.locale FROM help_collection_translations ct
JOIN help_collections c ON c.workspace_id = ct.workspace_id AND c.id = ct.collection_id
WHERE ct.workspace_id = @workspace_id AND c.center_id = @center_id AND NOT (ct.locale = ANY (@locales::text[]));

-- name: CountWorkspaceHelpArticles :one
SELECT count(*)::int FROM help_articles WHERE workspace_id = @workspace_id;

-- name: LockHelpArticleQuota :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_help_articles:' || sqlc.arg(workspace_id)::uuid::text, 0));

-- name: CreateHelpCollection :one
INSERT INTO help_collections (id, workspace_id, center_id, parent_id, number, icon, position, created_at, updated_at)
VALUES (@id, @workspace_id, @center_id, sqlc.narg(parent_id), @number, sqlc.narg(icon), @position, @now, @now)
RETURNING *;

-- name: GetHelpCollection :one
SELECT * FROM help_collections WHERE workspace_id = @workspace_id AND center_id = @center_id AND id = @id;

-- name: UpdateHelpCollection :one
UPDATE help_collections SET parent_id = sqlc.narg(parent_id), icon = sqlc.narg(icon), position = @position, updated_at = @now
WHERE workspace_id = @workspace_id AND center_id = @center_id AND id = @id
RETURNING *;

-- name: DeleteHelpCollection :execrows
DELETE FROM help_collections WHERE workspace_id = @workspace_id AND center_id = @center_id AND id = @id;

-- name: CountHelpCollections :one
SELECT count(*)::int FROM help_collections WHERE workspace_id = @workspace_id AND center_id = @center_id;

-- name: HelpCollectionHasChildren :one
SELECT EXISTS (SELECT 1 FROM help_collections WHERE workspace_id = @workspace_id AND parent_id = @id)::bool;

-- name: ListHelpCollections :many
SELECT c.*, (SELECT count(*) FROM help_articles a WHERE a.workspace_id = c.workspace_id AND a.collection_id = c.id)::int AS article_count
FROM help_collections c
LEFT JOIN help_collections p ON p.workspace_id = c.workspace_id AND p.id = c.parent_id
WHERE c.workspace_id = @workspace_id AND c.center_id = @center_id
ORDER BY coalesce(p.position, c.position), coalesce(p.number, c.number), c.parent_id IS NOT NULL, c.position, c.number;

-- name: CountHelpCollectionArticles :one
SELECT count(*)::int FROM help_articles WHERE workspace_id = @workspace_id AND collection_id = @collection_id;

-- name: ListHelpCollectionTranslations :many
SELECT * FROM help_collection_translations
WHERE workspace_id = @workspace_id AND collection_id = ANY (@collection_ids::uuid[])
ORDER BY collection_id, locale;

-- name: UpsertHelpCollectionTranslation :exec
INSERT INTO help_collection_translations (workspace_id, collection_id, locale, name, description, slug, updated_at)
VALUES (@workspace_id, @collection_id, @locale, @name, @description, @slug, @now)
ON CONFLICT (workspace_id, collection_id, locale) DO UPDATE SET
    name = EXCLUDED.name, description = EXCLUDED.description, slug = EXCLUDED.slug, updated_at = EXCLUDED.updated_at;

-- name: DeleteHelpCollectionTranslations :exec
DELETE FROM help_collection_translations
WHERE workspace_id = @workspace_id AND collection_id = @collection_id AND locale = ANY (@locales::text[]);

-- name: CountHelpCollectionTranslations :one
SELECT count(*)::int FROM help_collection_translations WHERE workspace_id = @workspace_id AND collection_id = @collection_id;

-- name: CreateHelpArticle :one
INSERT INTO help_articles (id, workspace_id, center_id, collection_id, number, position, paths, created_by_member_id, created_by_api_key_id, created_at, updated_at)
VALUES (@id, @workspace_id, @center_id, sqlc.narg(collection_id), @number, @position, @paths::text[], sqlc.narg(member_id), sqlc.narg(api_key_id), @now, @now)
RETURNING *;

-- name: GetHelpArticle :one
SELECT * FROM help_articles WHERE workspace_id = @workspace_id AND center_id = @center_id AND id = @id;

-- name: LockHelpArticle :one
SELECT * FROM help_articles WHERE workspace_id = @workspace_id AND center_id = @center_id AND id = @id FOR UPDATE;

-- name: UpdateHelpArticle :one
UPDATE help_articles SET collection_id = sqlc.narg(collection_id), position = @position, paths = @paths::text[], updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: TouchHelpArticle :exec
UPDATE help_articles SET updated_at = @now WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteHelpArticle :execrows
DELETE FROM help_articles WHERE workspace_id = @workspace_id AND center_id = @center_id AND id = @id;

-- name: ListHelpArticles :many
SELECT a.* FROM help_articles a
WHERE a.workspace_id = @workspace_id AND a.center_id = @center_id
  AND (sqlc.narg(collection_id)::uuid IS NULL OR a.collection_id = sqlc.narg(collection_id)::uuid)
  AND (sqlc.narg(state)::text IS NULL OR CASE sqlc.narg(state)::text
      WHEN 'missing' THEN NOT EXISTS (
          SELECT 1 FROM help_article_translations t
          WHERE t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.locale = @locale)
      ELSE EXISTS (
          SELECT 1 FROM help_article_translations t
          WHERE t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.locale = @locale
            AND CASE sqlc.narg(state)::text
                WHEN 'draft' THEN t.published_at IS NULL
                WHEN 'published' THEN t.published_at IS NOT NULL AND NOT t.has_unpublished_changes
                ELSE t.has_unpublished_changes END)
      END)
  AND (sqlc.narg(q)::text IS NULL OR EXISTS (
      SELECT 1 FROM help_article_translations t
      WHERE t.workspace_id = a.workspace_id AND t.article_id = a.id
        AND to_tsvector('simple', translate(t.title, 'İı', 'ii')) @@ websearch_to_tsquery('simple', translate(sqlc.narg(q)::text, 'İı', 'ii'))))
  AND (sqlc.narg(cursor_position)::int IS NULL
       OR (a.position, a.number) > (sqlc.narg(cursor_position)::int, sqlc.narg(cursor_number)::int))
ORDER BY a.position, a.number
LIMIT @lim;

-- name: CreateHelpTranslation :one
INSERT INTO help_article_translations (workspace_id, article_id, locale, title, body, keywords, slug, image_ids, search_config,
    updated_by_member_id, updated_by_api_key_id, created_at, updated_at)
VALUES (@workspace_id, @article_id, @locale, @title, @body, @keywords, @slug, @image_ids::uuid[], @search_config,
    sqlc.narg(member_id), sqlc.narg(api_key_id), @now, @now)
RETURNING *;

-- name: GetHelpTranslation :one
SELECT * FROM help_article_translations WHERE workspace_id = @workspace_id AND article_id = @article_id AND locale = @locale;

-- name: LockHelpTranslation :one
SELECT * FROM help_article_translations WHERE workspace_id = @workspace_id AND article_id = @article_id AND locale = @locale FOR UPDATE;

-- name: ListHelpTranslationSummaries :many
SELECT article_id, locale, title, slug, version, published_title, published_slug, published_at, has_unpublished_changes, updated_at
FROM help_article_translations
WHERE workspace_id = @workspace_id AND article_id = ANY (@article_ids::uuid[])
ORDER BY article_id, locale;

-- name: ListHelpArticlesUsingImage :many
SELECT a.id, a.center_id, a.number,
    coalesce((SELECT t.title FROM help_article_translations t
        WHERE t.workspace_id = a.workspace_id AND t.article_id = a.id ORDER BY t.created_at, t.locale LIMIT 1), '')::text AS title
FROM help_articles a
WHERE a.workspace_id = @workspace_id AND (
    EXISTS (SELECT 1 FROM help_article_translations t
        WHERE t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.image_ids @> ARRAY[@image_id::uuid])
    OR EXISTS (SELECT 1 FROM help_article_revisions r
        WHERE r.workspace_id = a.workspace_id AND r.article_id = a.id AND r.image_ids @> ARRAY[@image_id::uuid]))
ORDER BY a.number
LIMIT 20;

-- name: ListHelpTranslations :many
SELECT * FROM help_article_translations
WHERE workspace_id = @workspace_id AND article_id = ANY (@article_ids::uuid[])
ORDER BY article_id, locale;

-- name: SaveHelpTranslation :one
UPDATE help_article_translations SET title = @title, body = @body, keywords = @keywords, slug = @slug, image_ids = @image_ids::uuid[],
    version = version + 1, updated_by_member_id = sqlc.narg(member_id), updated_by_api_key_id = sqlc.narg(api_key_id), updated_at = @now
WHERE workspace_id = @workspace_id AND article_id = @article_id AND locale = @locale
RETURNING *;

-- name: PublishHelpTranslation :one
UPDATE help_article_translations SET published_title = title, published_body = body, published_keywords = keywords, published_slug = slug,
    published_html = @html, published_excerpt = @excerpt, published_at = @now,
    published_by_member_id = sqlc.narg(member_id), published_by_api_key_id = sqlc.narg(api_key_id)
WHERE workspace_id = @workspace_id AND article_id = @article_id AND locale = @locale
RETURNING *;

-- name: UnpublishHelpTranslation :one
UPDATE help_article_translations SET published_title = NULL, published_body = NULL, published_keywords = NULL, published_slug = NULL,
    published_html = NULL, published_excerpt = NULL, published_at = NULL, published_by_member_id = NULL, published_by_api_key_id = NULL
WHERE workspace_id = @workspace_id AND article_id = @article_id AND locale = @locale
RETURNING *;

-- name: DeleteHelpTranslation :execrows
DELETE FROM help_article_translations WHERE workspace_id = @workspace_id AND article_id = @article_id AND locale = @locale;

-- name: CountHelpTranslations :one
SELECT count(*)::int FROM help_article_translations WHERE workspace_id = @workspace_id AND article_id = @article_id;

-- name: InsertHelpRevision :exec
INSERT INTO help_article_revisions (id, workspace_id, article_id, locale, kind, title, body, keywords, image_ids, author_member_id, author_api_key_id, via, created_at)
VALUES (@id, @workspace_id, @article_id, @locale, @kind, @title, @body, @keywords, @image_ids::uuid[], sqlc.narg(member_id), sqlc.narg(api_key_id), sqlc.narg(via), @now);

-- name: LatestHelpSaveRevisionAt :one
SELECT coalesce(max(created_at), 'epoch'::timestamptz)::timestamptz FROM help_article_revisions
WHERE workspace_id = @workspace_id AND article_id = @article_id AND locale = @locale AND kind = 'save'
  AND author_member_id IS NOT DISTINCT FROM sqlc.narg(member_id)::uuid
  AND author_api_key_id IS NOT DISTINCT FROM sqlc.narg(api_key_id)::uuid;

-- name: PruneHelpRevisions :exec
DELETE FROM help_article_revisions r
WHERE r.workspace_id = @workspace_id AND r.article_id = @article_id AND r.locale = @locale
  AND r.id NOT IN (
      SELECT k.id FROM help_article_revisions k
      WHERE k.workspace_id = @workspace_id AND k.article_id = @article_id AND k.locale = @locale
      ORDER BY k.created_at DESC, k.id DESC LIMIT @keep)
  AND r.id IS DISTINCT FROM (
      SELECT k.id FROM help_article_revisions k
      WHERE k.workspace_id = @workspace_id AND k.article_id = @article_id AND k.locale = @locale AND k.kind = 'publish'
      ORDER BY k.created_at DESC, k.id DESC LIMIT 1);

-- name: ListHelpRevisions :many
SELECT * FROM help_article_revisions
WHERE workspace_id = @workspace_id AND article_id = @article_id AND locale = @locale
ORDER BY created_at DESC, id DESC;

-- name: GetHelpRevision :one
SELECT * FROM help_article_revisions
WHERE workspace_id = @workspace_id AND article_id = @article_id AND locale = @locale AND id = @id;

-- name: ListEmailImagesByTokens :many
SELECT id, token FROM email_images WHERE workspace_id = @workspace_id AND token = ANY (@tokens::text[]);

-- name: CountHelpPublishedInLocale :one
SELECT count(*)::int FROM help_article_translations t
JOIN help_articles a ON a.workspace_id = t.workspace_id AND a.id = t.article_id
WHERE t.workspace_id = @workspace_id AND a.center_id = @center_id AND t.locale = @locale AND t.published_at IS NOT NULL;

-- name: SearchHelpPublished :many
WITH q AS (
    SELECT CASE WHEN @any_word::bool
        THEN to_tsquery(CAST(@config::text AS regconfig), array_to_string(@heads::text[], ' | ')) || l.q
        ELSE to_tsquery(CAST(@config::text AS regconfig), array_to_string(@heads::text[], ' & ')) && l.q END AS query
    FROM (SELECT to_tsquery(CAST(@config::text AS regconfig), @last::text || ':*') || to_tsquery('simple', @last::text || ':*') AS q) l
)
SELECT t.article_id, a.number, a.collection_id, t.locale, t.published_title::text AS title, t.published_excerpt::text AS excerpt,
    t.published_slug::text AS slug, t.has_unpublished_changes,
    (ts_rank_cd(t.search, q.query, 32)
        + CASE WHEN starts_with(lower(translate(t.published_title, 'İı', 'ii')), @prefix::text) THEN 1 ELSE 0 END)::float8 AS rank,
    coalesce((SELECT sum(d.views) FROM help_article_daily d
        WHERE d.workspace_id = t.workspace_id AND d.article_id = t.article_id AND d.day >= @since::date), 0)::bigint AS views
FROM help_article_translations t
JOIN help_articles a ON a.workspace_id = t.workspace_id AND a.id = t.article_id
CROSS JOIN q
WHERE t.workspace_id = @workspace_id AND a.center_id = @center_id AND t.locale = @locale
  AND t.published_at IS NOT NULL AND t.search @@ q.query
ORDER BY rank DESC, views DESC, a.number
LIMIT @lim;

-- name: SearchHelpDrafts :many
WITH q AS (
    SELECT CASE WHEN @any_word::bool
        THEN to_tsquery(CAST(@config::text AS regconfig), array_to_string(@heads::text[], ' | ')) || l.q
        ELSE to_tsquery(CAST(@config::text AS regconfig), array_to_string(@heads::text[], ' & ')) && l.q END AS query
    FROM (SELECT to_tsquery(CAST(@config::text AS regconfig), @last::text || ':*') || to_tsquery('simple', @last::text || ':*') AS q) l
)
SELECT t.article_id, a.number, a.collection_id, t.locale, t.title, left(t.body, 1000)::text AS body_start, t.slug, t.published_slug, t.published_at, t.has_unpublished_changes,
    (ts_rank_cd(v.doc, q.query, 32)
        + CASE WHEN starts_with(lower(translate(t.title, 'İı', 'ii')), @prefix::text) THEN 1 ELSE 0 END)::float8 AS rank
FROM help_article_translations t
JOIN help_articles a ON a.workspace_id = t.workspace_id AND a.id = t.article_id
CROSS JOIN q
CROSS JOIN LATERAL (SELECT
    setweight(to_tsvector(t.search_config::regconfig, translate(t.title, 'İı', 'ii')), 'A') ||
    setweight(to_tsvector('simple', translate(t.title, 'İı', 'ii')), 'A') ||
    setweight(to_tsvector(t.search_config::regconfig, translate(t.keywords, 'İı', 'ii')), 'B') ||
    setweight(to_tsvector('simple', translate(t.keywords, 'İı', 'ii')), 'B') ||
    setweight(to_tsvector(t.search_config::regconfig, translate(t.body, 'İı', 'ii')), 'C') ||
    setweight(to_tsvector('simple', translate(t.body, 'İı', 'ii')), 'C') AS doc) v
WHERE t.workspace_id = @workspace_id AND a.center_id = @center_id AND t.locale = @locale AND v.doc @@ q.query
ORDER BY rank DESC, a.number
LIMIT @lim;

-- name: AddHelpArticleDaily :exec
INSERT INTO help_article_daily (workspace_id, center_id, article_id, locale, day, views, helpful_up, helpful_down, conversations, sent)
SELECT a.workspace_id, a.center_id, a.id, @locale, @day, @views, greatest(0, @helpful_up::int), greatest(0, @helpful_down::int), @conversations, @sent
FROM help_articles a WHERE a.workspace_id = @workspace_id AND a.id = @article_id
ON CONFLICT (workspace_id, article_id, locale, day) DO UPDATE SET
    views = help_article_daily.views + EXCLUDED.views,
    helpful_up = greatest(0, help_article_daily.helpful_up + EXCLUDED.helpful_up),
    helpful_down = greatest(0, help_article_daily.helpful_down + EXCLUDED.helpful_down),
    conversations = help_article_daily.conversations + EXCLUDED.conversations,
    sent = help_article_daily.sent + EXCLUDED.sent;

-- name: AddHelpSearchDaily :exec
-- A center keeps at most @max_queries distinct queries a day; the rest are counted under (other).
INSERT INTO help_search_daily (workspace_id, center_id, locale, day, query, searches, no_results, clicks)
SELECT c.workspace_id, c.id, @locale, @day,
    CASE WHEN EXISTS (SELECT 1 FROM help_search_daily d
            WHERE d.workspace_id = c.workspace_id AND d.center_id = c.id AND d.locale = @locale AND d.day = @day AND d.query = @query)
        OR (SELECT count(*) FROM help_search_daily d WHERE d.workspace_id = c.workspace_id AND d.center_id = c.id AND d.day = @day) < @max_queries::int
    THEN @query::text ELSE @overflow::text END,
    @searches, @no_results, @clicks
FROM help_centers c WHERE c.workspace_id = @workspace_id AND c.id = @center_id
ON CONFLICT (workspace_id, center_id, locale, day, query) DO UPDATE SET
    searches = help_search_daily.searches + EXCLUDED.searches,
    no_results = help_search_daily.no_results + EXCLUDED.no_results,
    clicks = help_search_daily.clicks + EXCLUDED.clicks;

-- name: AddHelpSearchUsage :exec
INSERT INTO usage_counters (workspace_id, month, help_searches)
SELECT w.id, @month, @searches FROM workspaces w WHERE w.id = @workspace_id
ON CONFLICT (workspace_id, month) DO UPDATE SET help_searches = usage_counters.help_searches + EXCLUDED.help_searches;

-- name: DeleteOldHelpArticleDaily :execrows
DELETE FROM help_article_daily WHERE workspace_id = @workspace_id AND day < @before::date;

-- name: DeleteOldHelpSearchDaily :execrows
DELETE FROM help_search_daily WHERE workspace_id = @workspace_id AND day < @before::date;

-- name: HelpReportArticles :many
SELECT d.article_id, a.number, sum(d.views)::bigint AS views, sum(d.helpful_up)::bigint AS helpful_up,
    sum(d.helpful_down)::bigint AS helpful_down, sum(d.conversations)::bigint AS conversations, sum(d.sent)::bigint AS sent
FROM help_article_daily d
JOIN help_articles a ON a.workspace_id = d.workspace_id AND a.id = d.article_id
WHERE d.workspace_id = @workspace_id AND d.center_id = @center_id AND d.day >= @since::date
GROUP BY d.article_id, a.number;

-- name: HelpReportSearchTotals :one
SELECT coalesce(sum(searches), 0)::bigint AS searches, coalesce(sum(no_results), 0)::bigint AS no_results, coalesce(sum(clicks), 0)::bigint AS clicks
FROM help_search_daily
WHERE workspace_id = @workspace_id AND center_id = @center_id AND day >= @since::date;

-- name: HelpReportNoResultQueries :many
SELECT query, locale, sum(no_results)::bigint AS searches
FROM help_search_daily
WHERE workspace_id = @workspace_id AND center_id = @center_id AND day >= @since::date AND no_results > 0
GROUP BY query, locale
ORDER BY searches DESC, query COLLATE "C", locale
LIMIT 50;

-- name: FindHelpCenterBySlug :one
-- The documented exception: a public URL names only the slug, which is unique across the server.
SELECT * FROM help_centers WHERE slug = @slug;

-- name: FindHelpCenterByFormerSlug :one
-- The documented exception, as FindHelpCenterBySlug, for a slug the center no longer uses.
SELECT c.* FROM help_center_slugs f
JOIN help_centers c ON c.workspace_id = f.workspace_id AND c.id = f.center_id
WHERE f.slug = @slug;

-- name: ListPublicHelpCollections :many
SELECT c.id, c.parent_id, c.number, c.icon, c.position, t.locale, t.name, t.description, t.slug,
    (SELECT count(*) FROM help_articles a
        JOIN help_article_translations pt ON pt.workspace_id = a.workspace_id AND pt.article_id = a.id
        WHERE a.workspace_id = c.workspace_id AND a.collection_id = c.id AND pt.locale = t.locale AND pt.published_at IS NOT NULL)::int AS article_count
FROM help_collections c
JOIN help_collection_translations t ON t.workspace_id = c.workspace_id AND t.collection_id = c.id AND t.locale = @locale
WHERE c.workspace_id = @workspace_id AND c.center_id = @center_id
ORDER BY c.position, c.number;

-- name: ListPublicHelpArticles :many
SELECT a.id, a.number, a.collection_id, a.position, t.locale, t.published_title::text AS title, t.published_slug::text AS slug,
    t.published_excerpt::text AS excerpt, t.published_at::timestamptz AS published_at
FROM help_articles a
JOIN help_article_translations t ON t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.locale = @locale AND t.published_at IS NOT NULL
WHERE a.workspace_id = @workspace_id AND a.center_id = @center_id
  AND (sqlc.narg(collection_id)::uuid IS NULL OR a.collection_id = sqlc.narg(collection_id)::uuid)
ORDER BY a.position, a.number;

-- name: GetPublicHelpArticle :one
SELECT a.id, a.number, a.collection_id, a.position, t.locale, t.published_title::text AS title, t.published_slug::text AS slug,
    t.published_excerpt::text AS excerpt, t.published_html::text AS html, t.published_at::timestamptz AS published_at
FROM help_articles a
JOIN help_article_translations t ON t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.locale = @locale AND t.published_at IS NOT NULL
WHERE a.workspace_id = @workspace_id AND a.center_id = @center_id AND a.id = @id;

-- name: GetPublicHelpArticleByNumber :one
SELECT a.id, a.number, a.collection_id, a.position, t.locale, t.published_title::text AS title, t.published_slug::text AS slug,
    t.published_excerpt::text AS excerpt, t.published_html::text AS html, t.published_at::timestamptz AS published_at
FROM help_articles a
JOIN help_article_translations t ON t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.locale = @locale AND t.published_at IS NOT NULL
WHERE a.workspace_id = @workspace_id AND a.center_id = @center_id AND a.number = @number;

-- name: ListHelpArticleLocales :many
SELECT t.locale, t.published_slug::text AS slug FROM help_article_translations t
WHERE t.workspace_id = @workspace_id AND t.article_id = @article_id AND t.published_at IS NOT NULL
ORDER BY t.locale;

-- name: ListHelpSitemapArticles :many
SELECT a.number, t.locale, t.published_slug::text AS slug, t.published_at::timestamptz AS published_at
FROM help_articles a
JOIN help_article_translations t ON t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.published_at IS NOT NULL
WHERE a.workspace_id = @workspace_id AND a.center_id = @center_id
ORDER BY a.number, t.locale;

-- name: ListPopularHelpArticles :many
SELECT a.id, a.number, a.collection_id, t.locale, t.published_title::text AS title, t.published_slug::text AS slug,
    t.published_excerpt::text AS excerpt
FROM help_articles a
JOIN help_article_translations t ON t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.locale = @locale AND t.published_at IS NOT NULL
JOIN LATERAL (SELECT sum(d.views) AS views FROM help_article_daily d
    WHERE d.workspace_id = a.workspace_id AND d.article_id = a.id AND d.day >= @since::date) v ON v.views > 0
WHERE a.workspace_id = @workspace_id AND a.center_id = @center_id
ORDER BY v.views DESC, a.number
LIMIT @lim;

-- name: ListHelpArticlesWithPaths :many
SELECT a.id, a.number, a.collection_id, a.paths, t.locale, t.published_title::text AS title, t.published_slug::text AS slug,
    t.published_excerpt::text AS excerpt
FROM help_articles a
JOIN help_article_translations t ON t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.locale = @locale AND t.published_at IS NOT NULL
WHERE a.workspace_id = @workspace_id AND a.center_id = @center_id AND cardinality(a.paths) > 0
ORDER BY a.position, a.number;

-- name: ListRelatedHelpArticles :many
SELECT a.id, a.number, a.collection_id, t.locale, t.published_title::text AS title, t.published_slug::text AS slug,
    t.published_excerpt::text AS excerpt
FROM help_articles a
JOIN help_article_translations t ON t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.locale = @locale AND t.published_at IS NOT NULL
WHERE a.workspace_id = @workspace_id AND a.center_id = @center_id AND a.id <> @id AND a.collection_id = @collection_id
ORDER BY a.position, a.number
LIMIT @lim;

-- name: ListHelpCardArticles :many
SELECT a.id, a.number, t.locale, t.published_title::text AS title, t.published_slug::text AS slug, t.published_excerpt::text AS excerpt
FROM help_articles a
JOIN help_article_translations t ON t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.published_at IS NOT NULL
WHERE a.workspace_id = @workspace_id AND a.center_id = @center_id AND a.id = ANY (@article_ids::uuid[]);

-- name: GetHelpArticleLocalesForConversation :many
SELECT c.id AS center_id, c.default_locale, t.locale FROM help_articles a
JOIN help_centers c ON c.workspace_id = a.workspace_id AND c.id = a.center_id
JOIN help_article_translations t ON t.workspace_id = a.workspace_id AND t.article_id = a.id AND t.published_at IS NOT NULL
WHERE a.workspace_id = @workspace_id AND a.id = @id AND c.inbox_id = @inbox_id AND c.published
ORDER BY t.locale;

-- name: ListHelpPublishedLocales :many
SELECT DISTINCT t.locale FROM help_article_translations t
JOIN help_articles a ON a.workspace_id = t.workspace_id AND a.id = t.article_id
WHERE t.workspace_id = @workspace_id AND a.center_id = @center_id AND t.published_at IS NOT NULL
ORDER BY t.locale;
