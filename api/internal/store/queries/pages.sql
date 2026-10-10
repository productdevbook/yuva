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

-- name: ListClientPageAnswers :many
SELECT * FROM page_answers
WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id AND page = @page
ORDER BY published_at DESC, id DESC
LIMIT 100;

-- name: CreatePageAnswer :one
INSERT INTO page_answers (id, workspace_id, inbox_id, channel_id, page, title, question, answer, member_id,
                          conversation_id, published_at, updated_at)
VALUES (@id, @workspace_id, @inbox_id, @channel_id, @page, @title, @question, @answer, @member_id,
        @conversation_id, @now, @now)
RETURNING *;

-- name: GetPageAnswer :one
SELECT * FROM page_answers WHERE workspace_id = @workspace_id AND id = @id;

-- name: UpdatePageAnswer :one
UPDATE page_answers SET question = @question, answer = @answer, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeletePageAnswer :execrows
DELETE FROM page_answers WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListPageAnswers :many
SELECT a.* FROM page_answers a
WHERE a.workspace_id = @workspace_id
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = a.workspace_id AND iv.inbox_id = a.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR a.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (sqlc.narg(page)::text IS NULL OR a.page = sqlc.narg(page)::text)
  AND (sqlc.narg(conversation_id)::uuid IS NULL OR a.conversation_id = sqlc.narg(conversation_id)::uuid)
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (a.published_at, a.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY a.published_at DESC, a.id DESC
LIMIT @lim;

-- name: ConversationHasMemberReply :one
SELECT EXISTS (
    SELECT 1 FROM messages
    WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id
      AND kind = 'message' AND direction = 'out' AND author_type = 'member' AND NOT draft)::bool;

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
    GROUP BY r.inbox_id, r.page
), opened AS (
    SELECT c.inbox_id, c.page_url AS page,
           count(*) FILTER (WHERE c.kind = 'feedback')::bigint AS open_feedback,
           count(*) FILTER (WHERE c.kind = 'question')::bigint AS open_questions
    FROM conversations c
    WHERE c.workspace_id = @workspace_id AND c.inbox_id IN (SELECT id FROM visible)
      AND c.page_url IS NOT NULL AND c.status = 'open' AND NOT c.spam
      AND (sqlc.narg(page)::text IS NULL OR c.page_url = sqlc.narg(page)::text)
    GROUP BY c.inbox_id, c.page_url
), answered AS (
    SELECT a.inbox_id, a.page, count(*)::bigint AS published_answers
    FROM page_answers a
    WHERE a.workspace_id = @workspace_id AND a.inbox_id IN (SELECT id FROM visible)
      AND (sqlc.narg(page)::text IS NULL OR a.page = sqlc.narg(page)::text)
    GROUP BY a.inbox_id, a.page
), pages AS (
    SELECT rated.inbox_id, rated.page FROM rated
    UNION SELECT opened.inbox_id, opened.page FROM opened
    UNION SELECT answered.inbox_id, answered.page FROM answered
)
SELECT d.inbox_id, d.page, d.title, d.up, d.down, d.open_feedback, d.open_questions, d.published_answers FROM (
SELECT p.inbox_id::uuid AS inbox_id, p.page::text AS page,
    coalesce(
        (SELECT t.title FROM page_ratings t
         WHERE t.workspace_id = @workspace_id AND t.inbox_id = p.inbox_id AND t.page = p.page AND t.title <> ''
         ORDER BY t.day DESC LIMIT 1),
        (SELECT ct.page_title FROM conversations ct
         WHERE ct.workspace_id = @workspace_id AND ct.inbox_id = p.inbox_id AND ct.page_url = p.page AND ct.page_title <> ''
         ORDER BY ct.created_at DESC LIMIT 1),
        (SELECT at.title FROM page_answers at
         WHERE at.workspace_id = @workspace_id AND at.inbox_id = p.inbox_id AND at.page = p.page AND at.title <> ''
         ORDER BY at.published_at DESC LIMIT 1),
        '')::text AS title,
    coalesce(rated.up, 0)::bigint AS up,
    coalesce(rated.down, 0)::bigint AS down,
    coalesce(opened.open_feedback, 0)::bigint AS open_feedback,
    coalesce(opened.open_questions, 0)::bigint AS open_questions,
    coalesce(answered.published_answers, 0)::bigint AS published_answers
FROM pages p
LEFT JOIN rated ON rated.inbox_id = p.inbox_id AND rated.page = p.page
LEFT JOIN opened ON opened.inbox_id = p.inbox_id AND opened.page = p.page
LEFT JOIN answered ON answered.inbox_id = p.inbox_id AND answered.page = p.page
) d
ORDER BY
    CASE @sort::text
        WHEN 'up' THEN d.up
        WHEN 'activity' THEN d.up + d.down + d.open_feedback + d.open_questions
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
    GROUP BY r.day
), sent AS (
    SELECT (c.created_at AT TIME ZONE 'UTC')::date AS day,
           count(*) FILTER (WHERE c.kind = 'feedback')::bigint AS feedback,
           count(*) FILTER (WHERE c.kind = 'question')::bigint AS questions
    FROM conversations c
    WHERE c.workspace_id = @workspace_id AND c.inbox_id IN (SELECT id FROM visible)
      AND c.page_url IS NOT NULL AND NOT c.spam AND c.created_at >= @since_at::timestamptz
    GROUP BY 1
)
SELECT days.day::date AS day,
    coalesce(rated.up, 0)::bigint AS up,
    coalesce(rated.down, 0)::bigint AS down,
    coalesce(sent.feedback, 0)::bigint AS feedback,
    coalesce(sent.questions, 0)::bigint AS questions
FROM days
LEFT JOIN rated ON rated.day = days.day
LEFT JOIN sent ON sent.day = days.day
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
          AND r.up + r.down > 0) rp)::bigint AS rated_pages,
    (SELECT count(*) FROM page_answers a
     WHERE a.workspace_id = @workspace_id AND a.inbox_id IN (SELECT id FROM visible)
       AND a.published_at >= @since_at::timestamptz)::bigint AS published_answers;
