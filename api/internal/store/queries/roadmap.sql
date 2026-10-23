-- name: CreateRoadmapBoard :one
INSERT INTO roadmap_boards (id, workspace_id, inbox_id, channel_id, name, description, slug, public, suggestions, analytics_property_id,
    changed_at, created_at, updated_at)
VALUES (@id, @workspace_id, @inbox_id, @channel_id, @name, @description, @slug, @public, @suggestions, sqlc.narg(analytics_property_id), @now, @now, @now)
RETURNING *;

-- name: GetRoadmapBoard :one
SELECT * FROM roadmap_boards WHERE workspace_id = $1 AND id = $2;

-- name: LockRoadmapBoard :one
SELECT * FROM roadmap_boards WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: GetInboxRoadmapBoard :one
SELECT * FROM roadmap_boards WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id;

-- name: ListRoadmapBoards :many
SELECT * FROM roadmap_boards b
WHERE b.workspace_id = @workspace_id
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR b.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = b.workspace_id AND iv.inbox_id = b.inbox_id AND iv.viewer_id = @viewer_id))
ORDER BY b.created_at, b.id;

-- name: UpdateRoadmapBoard :one
UPDATE roadmap_boards SET channel_id = @channel_id, name = @name, description = @description, slug = @slug, public = @public,
    suggestions = @suggestions, analytics_property_id = sqlc.narg(analytics_property_id),
    changed_at = greatest(changed_at + interval '1 microsecond', @now::timestamptz), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: TouchRoadmapBoardNow :exec
UPDATE roadmap_boards SET changed_at = greatest(changed_at + interval '1 microsecond', now())
WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteRoadmapBoard :execrows
DELETE FROM roadmap_boards WHERE workspace_id = $1 AND id = $2;

-- name: RoadmapSlugTaken :one
-- The documented exception: slugs, current and former, are unique across the server, so availability is checked across workspaces.
SELECT (EXISTS (SELECT 1 FROM roadmap_boards b WHERE b.slug = @slug AND b.id <> @id)
    OR EXISTS (SELECT 1 FROM roadmap_board_slugs f WHERE f.slug = @slug AND f.board_id <> @id))::bool;

-- name: AddRoadmapFormerSlug :exec
INSERT INTO roadmap_board_slugs (slug, workspace_id, board_id, created_at) VALUES (@slug, @workspace_id, @board_id, @now);

-- name: ReleaseRoadmapFormerSlug :exec
DELETE FROM roadmap_board_slugs WHERE workspace_id = @workspace_id AND board_id = @board_id AND slug = @slug;

-- name: PruneRoadmapFormerSlugs :exec
DELETE FROM roadmap_board_slugs f
WHERE f.workspace_id = @workspace_id AND f.board_id = @board_id
  AND f.slug NOT IN (
      SELECT k.slug FROM roadmap_board_slugs k
      WHERE k.workspace_id = @workspace_id AND k.board_id = @board_id
      ORDER BY k.created_at DESC, k.slug LIMIT @keep);

-- name: DeleteOldRoadmapFormerSlugs :execrows
DELETE FROM roadmap_board_slugs WHERE workspace_id = @workspace_id AND created_at < @before;

-- name: CountRoadmapBoardIdeas :many
SELECT stage::text AS stage, state::text AS state, count(*)::int AS n FROM discussions
WHERE workspace_id = @workspace_id AND board_id = @board_id AND kind = 'idea' AND state IN ('published', 'pending')
GROUP BY stage, state;

-- name: CountWorkspaceIdeas :one
SELECT count(*)::int FROM discussions WHERE workspace_id = @workspace_id AND kind = 'idea';

-- name: CountPendingBoardIdeas :one
SELECT count(*)::int FROM discussions WHERE workspace_id = @workspace_id AND board_id = @board_id AND kind = 'idea' AND state = 'pending';

-- name: ListBoardIdeas :many
SELECT d.* FROM discussions d
JOIN discussion_posts o ON o.workspace_id = d.workspace_id AND o.id = d.opening_post_id
WHERE d.workspace_id = @workspace_id AND d.board_id = @board_id AND d.kind = 'idea' AND d.stage <> 'closed'
  AND (d.state = 'published'
       OR (d.state = 'pending' AND (o.author_contact_id = sqlc.narg(viewer_contact)::uuid
                                    OR o.author_member_id = sqlc.narg(viewer_member)::uuid)))
  AND (sqlc.narg(stage)::text IS NULL OR d.stage = sqlc.narg(stage)::text)
  AND (sqlc.narg(tsq)::text IS NULL OR d.search @@ to_tsquery('simple', sqlc.narg(tsq)::text))
ORDER BY
    CASE WHEN @sort::text = 'top' THEN d.vote_count END DESC NULLS LAST,
    d.created_at DESC, d.id DESC
LIMIT @lim OFFSET @off;

-- name: SetIdeaStage :one
UPDATE discussions SET stage = @stage, stage_changed_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id AND kind = 'idea'
RETURNING *;

-- name: CreateRoadmapStageChange :one
INSERT INTO roadmap_stage_changes (id, workspace_id, idea_id, from_stage, to_stage, member_id, api_key_id, grant_id, via, comment_post_id,
    linked, progress, created_at)
VALUES (@id, @workspace_id, @idea_id, @from_stage, @to_stage, sqlc.narg(member_id), sqlc.narg(api_key_id), sqlc.narg(grant_id), @via,
    sqlc.narg(comment_post_id), @linked, sqlc.narg(progress), @now)
RETURNING *;

-- name: GetRoadmapStageChange :one
SELECT * FROM roadmap_stage_changes WHERE workspace_id = @workspace_id AND id = @id;

-- name: LatestRoadmapStageChange :one
SELECT id FROM roadmap_stage_changes
WHERE workspace_id = @workspace_id AND idea_id = @idea_id
ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: ListRoadmapStageChanges :many
SELECT * FROM roadmap_stage_changes
WHERE workspace_id = @workspace_id AND idea_id = @idea_id
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (created_at, id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: BumpRoadmapStageSet :exec
UPDATE roadmap_stage_changes SET progress_set = progress_set + 1 WHERE workspace_id = @workspace_id AND id = @id;

-- name: FinishRoadmapStageChange :exec
UPDATE roadmap_stage_changes SET progress_skipped = @skipped, finished_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id AND finished_at IS NULL;

-- name: LinkIdeaConversation :exec
INSERT INTO roadmap_idea_conversations (workspace_id, conversation_id, idea_id, linked_by_member_id, linked_by_api_key_id, linked_at)
VALUES (@workspace_id, @conversation_id, @idea_id, sqlc.narg(linked_by_member_id), sqlc.narg(linked_by_api_key_id), @now)
ON CONFLICT (workspace_id, conversation_id) DO UPDATE SET idea_id = excluded.idea_id,
    linked_by_member_id = excluded.linked_by_member_id, linked_by_api_key_id = excluded.linked_by_api_key_id, linked_at = excluded.linked_at;

-- name: UnlinkIdeaConversation :execrows
DELETE FROM roadmap_idea_conversations
WHERE workspace_id = @workspace_id AND idea_id = @idea_id AND conversation_id = @conversation_id;

-- name: GetConversationIdeaLink :one
SELECT idea_id FROM roadmap_idea_conversations WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id;

-- name: CountIdeaLinks :one
SELECT count(*)::int FROM roadmap_idea_conversations WHERE workspace_id = @workspace_id AND idea_id = @idea_id;

-- name: CountIdeaLinksByIdeas :many
SELECT idea_id, count(*)::int AS n FROM roadmap_idea_conversations
WHERE workspace_id = @workspace_id AND idea_id = ANY(@idea_ids::uuid[])
GROUP BY idea_id;

-- name: ListConversationIdeas :many
SELECT l.conversation_id, d.id, d.title, d.stage::text AS stage FROM roadmap_idea_conversations l
JOIN discussions d ON d.workspace_id = l.workspace_id AND d.id = l.idea_id
WHERE l.workspace_id = @workspace_id AND l.conversation_id = ANY(@conversation_ids::uuid[]);

-- name: IdeaLinkBatch :many
SELECT l.conversation_id, c.feedback_progress FROM roadmap_idea_conversations l
JOIN conversations c ON c.workspace_id = l.workspace_id AND c.id = l.conversation_id
WHERE l.workspace_id = @workspace_id AND l.idea_id = @idea_id AND l.conversation_id > @after::uuid
ORDER BY l.conversation_id
LIMIT @lim;

-- name: ListIdeaReplyTargets :many
SELECT c.id FROM roadmap_idea_conversations l
JOIN conversations c ON c.workspace_id = l.workspace_id AND c.id = l.conversation_id
WHERE l.workspace_id = @workspace_id AND l.idea_id = @idea_id AND c.status <> 'closed' AND NOT c.spam
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = c.workspace_id AND iv.inbox_id = c.inbox_id AND iv.viewer_id = @viewer_id))
ORDER BY l.linked_at, c.id;

-- name: IdeaConversationLinked :one
SELECT EXISTS (SELECT 1 FROM roadmap_idea_conversations
    WHERE workspace_id = @workspace_id AND idea_id = @idea_id AND conversation_id = @conversation_id)::bool;

-- name: ListStageDigestChanges :many
SELECT to_stage::text AS to_stage, created_at FROM roadmap_stage_changes
WHERE workspace_id = @workspace_id AND idea_id = @idea_id AND created_at > @after AND to_stage <> 'ideas'
ORDER BY created_at, id
LIMIT 10;

-- name: ChannelRoadmapBoard :one
SELECT id FROM roadmap_boards WHERE workspace_id = @workspace_id AND channel_id = @channel_id;

-- name: MoveBoardIdeasToChannel :exec
UPDATE discussions SET channel_id = @channel_id WHERE workspace_id = @workspace_id AND board_id = @board_id AND channel_id <> @channel_id;

-- name: UnlinkConversationIdea :exec
DELETE FROM roadmap_idea_conversations WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id;

-- name: FindRoadmapBoardBySlug :one
-- The documented exception: a public page is found by its slug before its workspace is known.
SELECT b.* FROM roadmap_boards b JOIN workspaces w ON w.id = b.workspace_id AND w.deleted_at IS NULL WHERE b.slug = @slug;

-- name: FindRoadmapBoardByFormerSlug :one
-- The documented exception: a public page is found by a former slug before its workspace is known.
SELECT b.* FROM roadmap_board_slugs s
JOIN roadmap_boards b ON b.workspace_id = s.workspace_id AND b.id = s.board_id
JOIN workspaces w ON w.id = b.workspace_id AND w.deleted_at IS NULL
WHERE s.slug = @slug;

-- name: ListPublicIdeas :many
SELECT d.id, d.title, d.stage::text AS stage, d.vote_count, d.reply_count, d.created_at, d.stage_changed_at, o.body
FROM discussions d
JOIN discussion_posts o ON o.workspace_id = d.workspace_id AND o.id = d.opening_post_id
WHERE d.workspace_id = @workspace_id AND d.board_id = @board_id AND d.kind = 'idea' AND d.state = 'published'
  AND d.stage = @stage::text AND o.deleted_at IS NULL
  AND (sqlc.narg(since)::timestamptz IS NULL OR coalesce(d.stage_changed_at, d.created_at) >= sqlc.narg(since)::timestamptz)
ORDER BY d.vote_count DESC, d.created_at DESC, d.id DESC
LIMIT @lim;

-- name: CountIdeaLinksToChange :one
SELECT count(*)::int FROM roadmap_idea_conversations l
JOIN conversations c ON c.workspace_id = l.workspace_id AND c.id = l.conversation_id
WHERE l.workspace_id = @workspace_id AND l.idea_id = @idea_id AND c.feedback_progress IS DISTINCT FROM @progress::text;
