-- name: CreateDiscussion :one
INSERT INTO discussions (id, workspace_id, inbox_id, channel_id, kind, page, page_title, board_id, stage, title, opening_post_id, state,
                         conversation_id, created_at, last_activity_at)
VALUES (@id, @workspace_id, @inbox_id, @channel_id, @kind, sqlc.narg(page), @page_title, sqlc.narg(board_id), sqlc.narg(stage), @title, @opening_post_id, @state,
        sqlc.narg(conversation_id), @now, @now)
RETURNING *;

-- name: CreateDiscussionPost :one
INSERT INTO discussion_posts (id, workspace_id, discussion_id, parent_post_id, opening, author_type, author_member_id,
                              author_contact_id, body, state, created_at, via)
VALUES (@id, @workspace_id, @discussion_id, sqlc.narg(parent_post_id), @opening, @author_type, sqlc.narg(author_member_id),
        sqlc.narg(author_contact_id), @body, @state, @now, sqlc.narg(via))
RETURNING id;

-- name: GetDiscussion :one
SELECT * FROM discussions WHERE workspace_id = @workspace_id AND id = @id;

-- name: LockDiscussion :one
SELECT * FROM discussions WHERE workspace_id = @workspace_id AND id = @id FOR UPDATE;

-- name: GetDiscussionByConversation :one
SELECT * FROM discussions WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id;

-- name: GetDiscussionPost :one
SELECT * FROM discussion_post_view WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListDiscussionPosts :many
SELECT * FROM discussion_post_view
WHERE workspace_id = @workspace_id AND discussion_id = @discussion_id
ORDER BY created_at, id
LIMIT 1000;

-- name: ListOpeningPosts :many
SELECT * FROM discussion_post_view
WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);

-- name: ListDiscussionsByIDs :many
SELECT * FROM discussions WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);

-- name: RecountDiscussion :one
UPDATE discussions d SET
    state = (SELECT op.state FROM discussion_posts op WHERE op.workspace_id = d.workspace_id AND op.id = d.opening_post_id),
    reply_count = (SELECT count(*) FROM discussion_posts p
                   WHERE p.workspace_id = d.workspace_id AND p.discussion_id = d.id AND NOT p.opening
                     AND p.state = 'published' AND p.deleted_at IS NULL),
    pending_posts = (SELECT count(*) FROM discussion_posts p
                     WHERE p.workspace_id = d.workspace_id AND p.discussion_id = d.id
                       AND p.state = 'pending' AND p.deleted_at IS NULL),
    hidden_posts = (SELECT count(*) FROM discussion_posts p
                    WHERE p.workspace_id = d.workspace_id AND p.discussion_id = d.id
                      AND p.state = 'hidden' AND p.deleted_at IS NULL),
    last_activity_at = greatest(d.created_at, coalesce((SELECT max(p.created_at) FROM discussion_posts p
                        WHERE p.workspace_id = d.workspace_id AND p.discussion_id = d.id
                          AND p.state = 'published' AND p.deleted_at IS NULL), d.created_at))
WHERE d.workspace_id = @workspace_id AND d.id = @id
RETURNING *;

-- name: SetDiscussionTitle :exec
UPDATE discussions SET title = @title WHERE workspace_id = @workspace_id AND id = @id;

-- name: SetDiscussionLocked :exec
UPDATE discussions SET locked_at = sqlc.narg(locked_at) WHERE workspace_id = @workspace_id AND id = @id;

-- name: SetDiscussionAnswer :exec
UPDATE discussions SET answer_post_id = sqlc.narg(answer_post_id), answered_at = sqlc.narg(answered_at)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: SetDiscussionPostState :exec
UPDATE discussion_posts SET state = @state WHERE workspace_id = @workspace_id AND id = @id;

-- name: EditDiscussionPost :exec
UPDATE discussion_posts SET body = @body, edited_at = @now::timestamptz WHERE workspace_id = @workspace_id AND id = @id;

-- name: SoftDeleteDiscussionPost :exec
UPDATE discussion_posts SET body = '', deleted_at = @now::timestamptz, author_member_id = NULL, author_contact_id = NULL
WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteDiscussionPost :exec
DELETE FROM discussion_posts WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteDiscussion :exec
DELETE FROM discussions WHERE workspace_id = @workspace_id AND id = @id;

-- name: DiscussionPostHasReplies :one
SELECT EXISTS (SELECT 1 FROM discussion_posts
               WHERE workspace_id = @workspace_id AND parent_post_id = @id::uuid)::bool;

-- name: DiscussionHasOtherPosts :one
SELECT EXISTS (SELECT 1 FROM discussion_posts
               WHERE workspace_id = @workspace_id AND discussion_id = @discussion_id AND NOT opening)::bool;

-- name: AddDiscussionVote :execrows
INSERT INTO discussion_votes (workspace_id, post_id, contact_id, member_id, created_at)
VALUES (@workspace_id, @post_id, sqlc.narg(contact_id), sqlc.narg(member_id), @now)
ON CONFLICT DO NOTHING;

-- name: RemoveDiscussionVote :execrows
DELETE FROM discussion_votes
WHERE workspace_id = @workspace_id AND post_id = @post_id
  AND (contact_id = sqlc.narg(contact_id)::uuid OR member_id = sqlc.narg(member_id)::uuid);

-- name: ListVoterPosts :many
SELECT v.post_id FROM discussion_votes v
JOIN discussion_posts p ON p.workspace_id = v.workspace_id AND p.id = v.post_id
WHERE v.workspace_id = @workspace_id AND p.discussion_id = ANY(@discussion_ids::uuid[])
  AND (v.contact_id = sqlc.narg(contact_id)::uuid OR v.member_id = sqlc.narg(member_id)::uuid);

-- name: FollowDiscussion :exec
INSERT INTO discussion_follows (workspace_id, discussion_id, contact_id, created_at, seen_through)
VALUES (@workspace_id, @discussion_id, @contact_id, @now, @now)
ON CONFLICT DO NOTHING;

-- name: UnfollowDiscussion :execrows
DELETE FROM discussion_follows WHERE workspace_id = @workspace_id AND discussion_id = @discussion_id AND contact_id = @contact_id;

-- name: ListFollowedDiscussions :many
SELECT discussion_id FROM discussion_follows
WHERE workspace_id = @workspace_id AND contact_id = @contact_id AND discussion_id = ANY(@discussion_ids::uuid[]);

-- name: ListDiscussionFollowers :many
SELECT f.contact_id, c.person_id FROM discussion_follows f
JOIN contacts c ON c.workspace_id = f.workspace_id AND c.id = f.contact_id
WHERE f.workspace_id = @workspace_id AND f.discussion_id = @discussion_id;

-- name: LockDiscussionFollow :one
SELECT * FROM discussion_follows
WHERE workspace_id = @workspace_id AND discussion_id = @discussion_id AND contact_id = @contact_id FOR UPDATE;

-- name: MarkDiscussionFollowEmailed :exec
UPDATE discussion_follows SET emailed_at = @now::timestamptz, seen_through = @seen_through
WHERE workspace_id = @workspace_id AND discussion_id = @discussion_id AND contact_id = @contact_id;

-- name: ListDigestPosts :many
SELECT * FROM discussion_post_view
WHERE workspace_id = @workspace_id AND discussion_id = @discussion_id AND NOT opening
  AND state = 'published' AND deleted_at IS NULL AND created_at > @after
  AND author_contact_id IS DISTINCT FROM sqlc.narg(contact_id)::uuid
ORDER BY created_at, id
LIMIT 20;

-- name: FindContactByPerson :one
SELECT id, blocked, discussions_blocked_at FROM contacts WHERE workspace_id = @workspace_id AND person_id = @person_id;

-- name: LinkContactByEmail :one
UPDATE contacts c SET person_id = @person_id, updated_at = @now
FROM contact_emails e
WHERE e.workspace_id = @workspace_id AND e.email = @email AND c.workspace_id = e.workspace_id AND c.id = e.contact_id
  AND c.person_id IS NULL
RETURNING c.id, c.blocked, c.discussions_blocked_at;

-- name: SetContactPerson :exec
UPDATE contacts SET person_id = sqlc.narg(person_id) WHERE workspace_id = @workspace_id AND id = @id;

-- name: GetContactDiscussionState :one
SELECT person_id, discussions_blocked_at,
       EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id AND x.inbox_id = @inbox_id)::bool AS identified,
       EXISTS (SELECT 1 FROM discussion_posts p WHERE p.workspace_id = c.workspace_id AND p.author_contact_id = c.id AND p.state = 'published')::bool AS trusted
FROM contacts c WHERE c.workspace_id = @workspace_id AND c.id = @id;

-- name: BlockContactDiscussions :exec
UPDATE contacts SET discussions_blocked_at = @now::timestamptz WHERE workspace_id = @workspace_id AND id = @id;

-- name: HidePendingPostsOfContact :many
UPDATE discussion_posts SET state = 'hidden'
WHERE workspace_id = @workspace_id AND author_contact_id = @contact_id::uuid AND state = 'pending'
RETURNING discussion_id;

-- name: MoveDiscussionAuthorship :exec
UPDATE discussion_posts SET author_contact_id = @to_contact
WHERE workspace_id = @workspace_id AND author_contact_id = @from_contact;

-- name: MoveDiscussionVotes :exec
UPDATE discussion_votes v SET contact_id = @to_contact
WHERE v.workspace_id = @workspace_id AND v.contact_id = @from_contact
  AND NOT EXISTS (SELECT 1 FROM discussion_votes o WHERE o.workspace_id = v.workspace_id AND o.post_id = v.post_id AND o.contact_id = @to_contact);

-- name: MoveDiscussionFollows :exec
UPDATE discussion_follows f SET contact_id = @to_contact
WHERE f.workspace_id = @workspace_id AND f.contact_id = @from_contact
  AND NOT EXISTS (SELECT 1 FROM discussion_follows o WHERE o.workspace_id = f.workspace_id AND o.discussion_id = f.discussion_id AND o.contact_id = @to_contact);

-- name: SetContactPersonIfUnset :exec
UPDATE contacts t SET person_id = @person_id
WHERE t.workspace_id = @workspace_id AND t.id = @id AND t.person_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM contacts o WHERE o.workspace_id = t.workspace_id AND o.person_id = @person_id);

-- name: ClearContactPerson :exec
UPDATE contacts SET person_id = NULL WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListPageDiscussions :many
SELECT d.* FROM discussions d
JOIN discussion_posts o ON o.workspace_id = d.workspace_id AND o.id = d.opening_post_id
WHERE d.workspace_id = @workspace_id AND d.inbox_id = @inbox_id AND d.kind = 'page' AND d.page = @page
  AND (d.state = 'published'
       OR (d.state = 'pending' AND (o.author_contact_id = sqlc.narg(viewer_contact)::uuid
                                    OR o.author_member_id = sqlc.narg(viewer_member)::uuid)))
  AND (@sort::text <> 'unanswered' OR d.answer_post_id IS NULL)
ORDER BY
    CASE WHEN @sort::text = 'top' THEN (SELECT count(*) FROM discussion_votes v WHERE v.workspace_id = d.workspace_id AND v.post_id = d.opening_post_id) END DESC NULLS LAST,
    CASE WHEN @sort::text = 'unanswered' THEN d.reply_count END ASC NULLS LAST,
    d.created_at DESC, d.id DESC
LIMIT @lim OFFSET @off;

-- name: ListTeamDiscussions :many
SELECT d.* FROM discussions d
WHERE d.workspace_id = @workspace_id AND d.kind = @kind::text
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = d.workspace_id AND iv.inbox_id = d.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR d.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (sqlc.narg(board_id)::uuid IS NULL OR d.board_id = sqlc.narg(board_id)::uuid)
  AND (sqlc.narg(stage)::text IS NULL OR d.stage = sqlc.narg(stage)::text)
  AND (sqlc.narg(tsq)::text IS NULL OR d.search @@ to_tsquery('simple', sqlc.narg(tsq)::text))
  AND (sqlc.narg(page)::text IS NULL OR d.page = sqlc.narg(page)::text)
  AND (sqlc.narg(site)::text IS NULL OR left(d.page, length(sqlc.narg(site)::text) + 1) = sqlc.narg(site)::text || '/')
  AND (sqlc.narg(state)::text IS NULL OR d.state = sqlc.narg(state)::text)
  AND (sqlc.narg(answered)::bool IS NULL OR (d.answer_post_id IS NOT NULL) = sqlc.narg(answered)::bool)
  AND (sqlc.narg(locked)::bool IS NULL OR (d.locked_at IS NOT NULL) = sqlc.narg(locked)::bool)
  AND (sqlc.narg(pending)::bool IS NULL OR (d.pending_posts > 0) = sqlc.narg(pending)::bool)
ORDER BY
    CASE WHEN @sort::text = 'top' THEN (SELECT count(*) FROM discussion_votes v WHERE v.workspace_id = d.workspace_id AND v.post_id = d.opening_post_id) END DESC NULLS LAST,
    CASE WHEN @sort::text = 'votes' THEN d.vote_count END DESC NULLS LAST,
    CASE WHEN @sort::text = 'new' THEN d.created_at END DESC NULLS LAST,
    d.last_activity_at DESC, d.id DESC
LIMIT @lim OFFSET @off;

-- name: ListTeamDiscussionPosts :many
SELECT p.* FROM discussion_post_view p
JOIN discussions d ON d.workspace_id = p.workspace_id AND d.id = p.discussion_id
WHERE p.workspace_id = @workspace_id AND p.state = @state AND p.deleted_at IS NULL
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = d.workspace_id AND iv.inbox_id = d.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND (sqlc.narg(inbox_id)::uuid IS NULL OR d.inbox_id = sqlc.narg(inbox_id)::uuid)
  AND (sqlc.narg(page)::text IS NULL OR d.page = sqlc.narg(page)::text)
  AND (sqlc.narg(site)::text IS NULL OR left(d.page, length(sqlc.narg(site)::text) + 1) = sqlc.narg(site)::text || '/')
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (p.created_at, p.id) > (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY p.created_at, p.id
LIMIT @lim;

-- name: ContactHasPersonOrIdentity :one
SELECT (c.person_id IS NOT NULL OR EXISTS (
    SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = c.workspace_id AND x.contact_id = c.id AND x.inbox_id = @inbox_id))::bool
FROM contacts c WHERE c.workspace_id = @workspace_id AND c.id = @id;

-- name: FindDiscussionChannel :one
SELECT ch.id FROM channels ch
JOIN chat_channels cc ON cc.workspace_id = ch.workspace_id AND cc.channel_id = ch.id
WHERE ch.workspace_id = @workspace_id AND ch.inbox_id = @inbox_id AND ch.kind = 'chat'
  AND cc.discussions <> 'off' AND @origin::text = ANY(cc.allowed_origins)
ORDER BY (ch.id = sqlc.narg(prefer)::uuid) DESC NULLS LAST, ch.created_at, ch.id
LIMIT 1;

-- name: CreateReaderToken :exec
INSERT INTO reader_tokens (id, workspace_id, channel_id, person_id, session_id, token_hash, origin, created_at, expires_at)
VALUES (@id, @workspace_id, @channel_id, @person_id, @session_id, @token_hash, @origin, @now, @expires_at);

-- name: GetReaderTokenByHash :one
SELECT t.id, t.workspace_id, t.channel_id, t.person_id, t.origin, t.expires_at FROM reader_tokens t
JOIN sessions s ON s.id = t.session_id
JOIN workspaces w ON w.id = t.workspace_id
WHERE t.token_hash = @token_hash AND t.expires_at > @now AND s.expires_at > @now AND w.deleted_at IS NULL;

-- name: DeleteReaderToken :exec
DELETE FROM reader_tokens WHERE workspace_id = @workspace_id AND id = @id;

-- name: DeleteExpiredReaderTokens :exec
DELETE FROM reader_tokens WHERE expires_at < @now;

-- name: CreateReaderNotification :exec
INSERT INTO reader_notifications (id, workspace_id, person_id, discussion_id, post_id, kind, created_at)
VALUES (@id, @workspace_id, @person_id, @discussion_id, sqlc.narg(post_id), @kind, @now);

-- name: DeleteOldReaderNotifications :exec
DELETE FROM reader_notifications WHERE created_at < @before;

-- name: ListReaderNotifications :many
SELECT n.id, n.workspace_id, n.kind, n.discussion_id, n.post_id, n.created_at, n.read_at,
       d.title, coalesce(d.page, '')::text AS page, i.name AS inbox_name
FROM reader_notifications n
JOIN discussions d ON d.workspace_id = n.workspace_id AND d.id = n.discussion_id
JOIN inboxes i ON i.workspace_id = d.workspace_id AND i.id = d.inbox_id
JOIN workspaces w ON w.id = n.workspace_id
WHERE n.person_id = @person_id AND w.deleted_at IS NULL
  AND (NOT @unread_only::bool OR n.read_at IS NULL)
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (n.created_at, n.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY n.created_at DESC, n.id DESC
LIMIT @lim;

-- name: CountUnreadReaderNotifications :one
SELECT count(*) FROM reader_notifications n
JOIN workspaces w ON w.id = n.workspace_id
WHERE n.person_id = @person_id AND n.read_at IS NULL AND w.deleted_at IS NULL;

-- name: MarkReaderNotificationsRead :exec
UPDATE reader_notifications SET read_at = @now::timestamptz
WHERE person_id = @person_id AND read_at IS NULL
  AND (NOT @only_ids::bool OR id = ANY(@ids::uuid[]));

-- name: ListPersonDiscussions :many
SELECT d.id, d.workspace_id, coalesce(d.page, '')::text AS page, d.page_title, d.title, d.state, d.locked_at, d.answer_post_id, d.reply_count,
       d.created_at, d.last_activity_at, d.opening_post_id, w.name AS workspace_name, i.name AS inbox_name,
       (o.author_contact_id = c.id)::bool AS started,
       EXISTS (SELECT 1 FROM discussion_follows f WHERE f.workspace_id = d.workspace_id AND f.discussion_id = d.id AND f.contact_id = c.id)::bool AS following,
       (SELECT count(*) FROM discussion_votes v WHERE v.workspace_id = d.workspace_id AND v.post_id = d.opening_post_id)::integer AS votes
FROM contacts c
JOIN workspaces w ON w.id = c.workspace_id AND w.deleted_at IS NULL
JOIN discussions d ON d.workspace_id = c.workspace_id
JOIN discussion_posts o ON o.workspace_id = d.workspace_id AND o.id = d.opening_post_id
JOIN inboxes i ON i.workspace_id = d.workspace_id AND i.id = d.inbox_id
WHERE c.person_id = @person_id AND d.kind = 'page'
  AND CASE WHEN @following_only::bool
      THEN EXISTS (SELECT 1 FROM discussion_follows f WHERE f.workspace_id = d.workspace_id AND f.discussion_id = d.id AND f.contact_id = c.id)
      ELSE EXISTS (SELECT 1 FROM discussion_posts p WHERE p.workspace_id = d.workspace_id AND p.discussion_id = d.id AND p.author_contact_id = c.id)
  END
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (d.last_activity_at, d.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY d.last_activity_at DESC, d.id DESC
LIMIT @lim;

-- name: UnfollowByPerson :execrows
DELETE FROM discussion_follows f
USING contacts c
WHERE c.person_id = @person_id AND f.workspace_id = c.workspace_id AND f.contact_id = c.id AND f.discussion_id = @discussion_id;

-- name: ListPersonContacts :many
SELECT workspace_id, id FROM contacts WHERE person_id = @person_id;

-- name: ReaderContactRemovable :one
SELECT (NOT EXISTS (SELECT 1 FROM conversations cv WHERE cv.workspace_id = @workspace_id AND cv.contact_id = @id)
    AND NOT EXISTS (SELECT 1 FROM contact_external_ids x WHERE x.workspace_id = @workspace_id AND x.contact_id = @id))::bool;

-- name: ForgetContactDiscussions :many
WITH voted AS (
    DELETE FROM discussion_votes v WHERE v.workspace_id = @workspace_id AND v.contact_id = @id
    RETURNING v.post_id
), followed AS (
    DELETE FROM discussion_follows f WHERE f.workspace_id = @workspace_id AND f.contact_id = @id
), authored AS (
    UPDATE discussion_posts p SET author_contact_id = NULL
    WHERE p.workspace_id = @workspace_id AND p.author_contact_id = @id
    RETURNING p.discussion_id
)
SELECT discussion_id FROM authored
UNION SELECT p.discussion_id FROM discussion_posts p WHERE p.workspace_id = @workspace_id AND p.id IN (SELECT post_id FROM voted);

-- name: SetPersonReaderSettings :exec
UPDATE people SET reader_email_notifications = @reader_email_notifications WHERE id = @id;

-- name: PersonIsMember :one
SELECT EXISTS (SELECT 1 FROM members m JOIN workspaces w ON w.id = m.workspace_id AND w.deleted_at IS NULL
               WHERE m.person_id = @person_id)::bool;
