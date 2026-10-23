-- name: AddPageRating :exec
INSERT INTO page_ratings (workspace_id, inbox_id, channel_id, page, title, day, up, down)
VALUES (@workspace_id, @inbox_id, @channel_id, @page, @title, @day, @up, @down)
ON CONFLICT (workspace_id, channel_id, page, day) DO UPDATE SET
    up = page_ratings.up + excluded.up,
    down = page_ratings.down + excluded.down,
    title = CASE WHEN excluded.title <> '' THEN excluded.title ELSE page_ratings.title END;

-- name: TakeBackPageRating :exec
UPDATE page_ratings pr SET
    up = pr.up - CASE WHEN @up::bool THEN 1 ELSE 0 END,
    down = pr.down - CASE WHEN @up::bool THEN 0 ELSE 1 END
WHERE pr.workspace_id = @workspace_id AND pr.channel_id = @channel_id AND pr.page = @page
  AND pr.day = (
      SELECT max(r.day) FROM page_ratings r
      WHERE r.workspace_id = @workspace_id AND r.channel_id = @channel_id AND r.page = @page
        AND r.day >= @since::date AND CASE WHEN @up::bool THEN r.up > 0 ELSE r.down > 0 END);

-- name: ListDocsPages :many
WITH visible AS (
    SELECT i.id FROM inboxes i
    WHERE i.workspace_id = @workspace_id
      AND (sqlc.narg(inbox_id)::uuid IS NULL OR i.id = sqlc.narg(inbox_id)::uuid)
      AND (@all_inboxes::bool OR EXISTS (
          SELECT 1 FROM inbox_viewers iv
          WHERE iv.workspace_id = i.workspace_id AND iv.inbox_id = i.id AND iv.viewer_id = @viewer_id::uuid))
), rated AS (
    SELECT r.inbox_id, r.page, sum(r.up)::bigint AS up, sum(r.down)::bigint AS down
    FROM page_ratings r
    WHERE r.workspace_id = @workspace_id AND r.inbox_id IN (SELECT id FROM visible) AND r.day >= @since::date
      AND (sqlc.narg(page)::text IS NULL OR r.page = sqlc.narg(page)::text)
      AND (sqlc.narg(site)::text IS NULL OR left(r.page, length(sqlc.narg(site)::text) + 1) = sqlc.narg(site)::text || '/')
    GROUP BY r.inbox_id, r.page
), opened AS (
    SELECT c.inbox_id, c.page_url AS page,
           count(*)::bigint AS open_feedback
    FROM conversations c
    WHERE c.workspace_id = @workspace_id AND c.inbox_id IN (SELECT id FROM visible)
      AND c.page_url IS NOT NULL AND c.kind = 'feedback' AND c.status = 'open' AND NOT c.spam AND c.trashed_at IS NULL
      AND (sqlc.narg(page)::text IS NULL OR c.page_url = sqlc.narg(page)::text)
      AND (sqlc.narg(site)::text IS NULL OR left(c.page_url, length(sqlc.narg(site)::text) + 1) = sqlc.narg(site)::text || '/')
    GROUP BY c.inbox_id, c.page_url
), threads AS (
    SELECT d.inbox_id, d.page,
           count(*) FILTER (WHERE d.state = 'published' AND d.answer_post_id IS NULL AND d.locked_at IS NULL)::bigint AS open_threads,
           count(*) FILTER (WHERE d.state = 'published' AND d.answer_post_id IS NULL AND d.locked_at IS NULL AND d.reply_count = 0)::bigint AS unanswered_threads,
           sum(d.pending_posts)::bigint AS pending_posts
    FROM discussions d
    WHERE d.workspace_id = @workspace_id AND d.inbox_id IN (SELECT id FROM visible) AND d.kind = 'page'
      AND (sqlc.narg(page)::text IS NULL OR d.page = sqlc.narg(page)::text)
      AND (sqlc.narg(site)::text IS NULL OR left(d.page, length(sqlc.narg(site)::text) + 1) = sqlc.narg(site)::text || '/')
    GROUP BY d.inbox_id, d.page
), pages AS (
    SELECT rated.inbox_id, rated.page FROM rated
    UNION SELECT opened.inbox_id, opened.page FROM opened
    UNION SELECT threads.inbox_id, threads.page FROM threads
)
SELECT d.inbox_id, d.page, d.title, d.up, d.down, d.open_feedback, d.open_threads, d.unanswered_threads, d.pending_posts FROM (
SELECT p.inbox_id::uuid AS inbox_id, p.page::text AS page,
    coalesce(
        (SELECT t.title FROM page_ratings t
         WHERE t.workspace_id = @workspace_id AND t.inbox_id = p.inbox_id AND t.page = p.page AND t.title <> ''
         ORDER BY t.day DESC LIMIT 1),
        (SELECT ct.page_title FROM conversations ct
         WHERE ct.workspace_id = @workspace_id AND ct.inbox_id = p.inbox_id AND ct.page_url = p.page AND ct.page_title <> ''
         ORDER BY ct.created_at DESC LIMIT 1),
        (SELECT dt.page_title FROM discussions dt
         WHERE dt.workspace_id = @workspace_id AND dt.inbox_id = p.inbox_id AND dt.kind = 'page' AND dt.page = p.page AND dt.page_title <> ''
         ORDER BY dt.created_at DESC LIMIT 1),
        '')::text AS title,
    coalesce(rated.up, 0)::bigint AS up,
    coalesce(rated.down, 0)::bigint AS down,
    coalesce(opened.open_feedback, 0)::bigint AS open_feedback,
    coalesce(threads.open_threads, 0)::bigint AS open_threads,
    coalesce(threads.unanswered_threads, 0)::bigint AS unanswered_threads,
    coalesce(threads.pending_posts, 0)::bigint AS pending_posts
FROM pages p
LEFT JOIN rated ON rated.inbox_id = p.inbox_id AND rated.page = p.page
LEFT JOIN opened ON opened.inbox_id = p.inbox_id AND opened.page = p.page
LEFT JOIN threads ON threads.inbox_id = p.inbox_id AND threads.page = p.page
) d
ORDER BY
    CASE @sort::text
        WHEN 'up' THEN d.up
        WHEN 'activity' THEN d.up + d.down + d.open_feedback + d.open_threads + d.pending_posts
        WHEN 'helpful' THEN CASE WHEN d.up + d.down >= @helpful_min::bigint THEN 1 ELSE 0 END
        ELSE d.down
    END DESC,
    CASE WHEN @sort::text = 'helpful' THEN d.up::float8 / nullif(d.up + d.down, 0) END DESC NULLS LAST,
    d.up + d.down DESC, d.page, d.inbox_id
LIMIT @lim OFFSET @off;

-- name: ListPageRatingDays :many
SELECT day, sum(up)::bigint AS up, sum(down)::bigint AS down
FROM page_ratings
WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id AND page = @page AND day >= @since::date
GROUP BY day
ORDER BY day;

-- name: ListDocsSummaryDays :many
WITH visible AS (
    SELECT i.id FROM inboxes i
    WHERE i.workspace_id = @workspace_id
      AND (sqlc.narg(inbox_id)::uuid IS NULL OR i.id = sqlc.narg(inbox_id)::uuid)
      AND (@all_inboxes::bool OR EXISTS (
          SELECT 1 FROM inbox_viewers iv
          WHERE iv.workspace_id = i.workspace_id AND iv.inbox_id = i.id AND iv.viewer_id = @viewer_id::uuid))
), days AS (
    SELECT generate_series(@since::date, @until::date, interval '1 day')::date AS day
), rated AS (
    SELECT r.day, sum(r.up)::bigint AS up, sum(r.down)::bigint AS down
    FROM page_ratings r
    WHERE r.workspace_id = @workspace_id AND r.inbox_id IN (SELECT id FROM visible) AND r.day >= @since::date
      AND (sqlc.narg(site)::text IS NULL OR left(r.page, length(sqlc.narg(site)::text) + 1) = sqlc.narg(site)::text || '/')
    GROUP BY r.day
), sent AS (
    SELECT (c.created_at AT TIME ZONE 'UTC')::date AS day,
           count(*)::bigint AS feedback
    FROM conversations c
    WHERE c.workspace_id = @workspace_id AND c.inbox_id IN (SELECT id FROM visible)
      AND c.page_url IS NOT NULL AND c.kind = 'feedback' AND NOT c.spam AND c.created_at >= @since_at::timestamptz AND c.trashed_at IS NULL
      AND (sqlc.narg(site)::text IS NULL OR left(c.page_url, length(sqlc.narg(site)::text) + 1) = sqlc.narg(site)::text || '/')
    GROUP BY 1
), started AS (
    SELECT (d.created_at AT TIME ZONE 'UTC')::date AS day, count(*)::bigint AS threads
    FROM discussions d
    WHERE d.workspace_id = @workspace_id AND d.inbox_id IN (SELECT id FROM visible) AND d.kind = 'page'
      AND d.created_at >= @since_at::timestamptz AND (sqlc.narg(site)::text IS NULL OR left(d.page, length(sqlc.narg(site)::text) + 1) = sqlc.narg(site)::text || '/')
    GROUP BY 1
)
SELECT days.day::date AS day,
    coalesce(rated.up, 0)::bigint AS up,
    coalesce(rated.down, 0)::bigint AS down,
    coalesce(sent.feedback, 0)::bigint AS feedback,
    coalesce(started.threads, 0)::bigint AS threads
FROM days
LEFT JOIN rated ON rated.day = days.day
LEFT JOIN sent ON sent.day = days.day
LEFT JOIN started ON started.day = days.day
ORDER BY days.day;

-- name: GetDocsSummaryCounts :one
WITH visible AS (
    SELECT i.id FROM inboxes i
    WHERE i.workspace_id = @workspace_id
      AND (sqlc.narg(inbox_id)::uuid IS NULL OR i.id = sqlc.narg(inbox_id)::uuid)
      AND (@all_inboxes::bool OR EXISTS (
          SELECT 1 FROM inbox_viewers iv
          WHERE iv.workspace_id = i.workspace_id AND iv.inbox_id = i.id AND iv.viewer_id = @viewer_id::uuid))
)
SELECT
    (SELECT count(*) FROM (
        SELECT DISTINCT r.inbox_id, r.page FROM page_ratings r
        WHERE r.workspace_id = @workspace_id AND r.inbox_id IN (SELECT id FROM visible) AND r.day >= @since::date
          AND r.up + r.down > 0 AND (sqlc.narg(site)::text IS NULL OR left(r.page, length(sqlc.narg(site)::text) + 1) = sqlc.narg(site)::text || '/')) rp)::bigint AS rated_pages,
    (SELECT coalesce(sum(d.pending_posts), 0) FROM discussions d
     WHERE d.workspace_id = @workspace_id AND d.inbox_id IN (SELECT id FROM visible) AND d.kind = 'page'
       AND (sqlc.narg(site)::text IS NULL OR left(d.page, length(sqlc.narg(site)::text) + 1) = sqlc.narg(site)::text || '/'))::bigint AS pending_posts;

-- name: ListDocsSites :many
WITH visible AS (
    SELECT i.id FROM inboxes i
    WHERE i.workspace_id = @workspace_id
      AND (sqlc.narg(inbox_id)::uuid IS NULL OR i.id = sqlc.narg(inbox_id)::uuid)
      AND (@all_inboxes::bool OR EXISTS (
          SELECT 1 FROM inbox_viewers iv
          WHERE iv.workspace_id = i.workspace_id AND iv.inbox_id = i.id AND iv.viewer_id = @viewer_id::uuid))
), rated AS (
    SELECT r.inbox_id, r.page, sum(r.up)::bigint AS up, sum(r.down)::bigint AS down
    FROM page_ratings r
    WHERE r.workspace_id = @workspace_id AND r.inbox_id IN (SELECT id FROM visible) AND r.day >= @since::date
    GROUP BY r.inbox_id, r.page
), opened AS (
    SELECT c.inbox_id, c.page_url AS page,
           count(*)::bigint AS open_feedback
    FROM conversations c
    WHERE c.workspace_id = @workspace_id AND c.inbox_id IN (SELECT id FROM visible)
      AND c.page_url IS NOT NULL AND c.kind = 'feedback' AND c.status = 'open' AND NOT c.spam AND c.trashed_at IS NULL
    GROUP BY c.inbox_id, c.page_url
), threads AS (
    SELECT d.inbox_id, d.page,
           count(*) FILTER (WHERE d.state = 'published' AND d.answer_post_id IS NULL AND d.locked_at IS NULL)::bigint AS open_threads,
           count(*) FILTER (WHERE d.state = 'published' AND d.answer_post_id IS NULL AND d.locked_at IS NULL AND d.reply_count = 0)::bigint AS unanswered_threads,
           sum(d.pending_posts)::bigint AS pending_posts
    FROM discussions d
    WHERE d.workspace_id = @workspace_id AND d.inbox_id IN (SELECT id FROM visible) AND d.kind = 'page'
    GROUP BY d.inbox_id, d.page
), pages AS (
    SELECT rated.inbox_id, rated.page FROM rated
    UNION SELECT opened.inbox_id, opened.page FROM opened
    UNION SELECT threads.inbox_id, threads.page FROM threads
), data AS (
    SELECT p.inbox_id, substring(p.page from '^[a-z]+://[^/]+') AS origin, count(*)::bigint AS pages,
           coalesce(sum(rated.up), 0)::bigint AS up, coalesce(sum(rated.down), 0)::bigint AS down,
           coalesce(sum(opened.open_feedback), 0)::bigint AS open_feedback,
           coalesce(sum(threads.open_threads), 0)::bigint AS open_threads,
           coalesce(sum(threads.unanswered_threads), 0)::bigint AS unanswered_threads,
           coalesce(sum(threads.pending_posts), 0)::bigint AS pending_posts
    FROM pages p
    LEFT JOIN rated ON rated.inbox_id = p.inbox_id AND rated.page = p.page
    LEFT JOIN opened ON opened.inbox_id = p.inbox_id AND opened.page = p.page
    LEFT JOIN threads ON threads.inbox_id = p.inbox_id AND threads.page = p.page
    GROUP BY 1, 2
), allowed AS (
    SELECT ch.inbox_id, o.origin, ch.id AS channel_id, ch.name AS channel_name,
           row_number() OVER (PARTITION BY ch.inbox_id, o.origin ORDER BY ch.created_at, ch.id) AS n
    FROM channels ch
    JOIN chat_channels cc ON cc.workspace_id = ch.workspace_id AND cc.channel_id = ch.id
    CROSS JOIN LATERAL unnest(cc.allowed_origins) AS o(origin)
    WHERE ch.workspace_id = @workspace_id AND ch.kind = 'chat' AND ch.inbox_id IN (SELECT id FROM visible)
), sites AS (
    SELECT data.inbox_id, data.origin FROM data
    UNION SELECT allowed.inbox_id, allowed.origin FROM allowed
)
SELECT s.origin::text AS origin, s.inbox_id::uuid AS inbox_id,
    al.channel_id, al.channel_name,
    coalesce(d.pages, 0)::bigint AS pages,
    coalesce(d.up, 0)::bigint AS up,
    coalesce(d.down, 0)::bigint AS down,
    coalesce(d.open_feedback, 0)::bigint AS open_feedback,
    coalesce(d.open_threads, 0)::bigint AS open_threads,
    coalesce(d.unanswered_threads, 0)::bigint AS unanswered_threads,
    coalesce(d.pending_posts, 0)::bigint AS pending_posts
FROM sites s
LEFT JOIN data d ON d.inbox_id = s.inbox_id AND d.origin = s.origin
LEFT JOIN allowed al ON al.inbox_id = s.inbox_id AND al.origin = s.origin AND al.n = 1
ORDER BY s.origin, s.inbox_id;
