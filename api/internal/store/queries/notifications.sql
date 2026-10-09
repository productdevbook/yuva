-- name: SetMemberNotifications :exec
UPDATE members SET notification_events = @events, notification_email_delay = sqlc.narg(email_delay)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListInboxNotifications :many
SELECT n.* FROM inbox_notifications n
WHERE n.workspace_id = @workspace_id AND n.member_id = @member_id
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_members im
      WHERE im.workspace_id = n.workspace_id AND im.inbox_id = n.inbox_id AND im.member_id = n.member_id))
ORDER BY n.updated_at, n.inbox_id;

-- name: UpsertInboxNotifications :one
INSERT INTO inbox_notifications (workspace_id, member_id, inbox_id, events, updated_at)
VALUES (@workspace_id, @member_id, @inbox_id, @events, @now)
ON CONFLICT (workspace_id, member_id, inbox_id) DO UPDATE SET events = excluded.events, updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteInboxNotifications :execrows
DELETE FROM inbox_notifications WHERE workspace_id = $1 AND member_id = $2 AND inbox_id = $3;

-- name: ListInboxNotificationsOfInbox :many
SELECT * FROM inbox_notifications WHERE workspace_id = $1 AND inbox_id = $2;

-- name: ListNotificationCandidates :many
SELECT m.id, m.role, m.person_id, m.notification_events, m.notification_email_delay,
       p.email, p.locale, p.availability
FROM members m JOIN people p ON p.id = m.person_id
WHERE m.workspace_id = @workspace_id
  AND (m.role IN ('owner', 'admin') OR EXISTS (
      SELECT 1 FROM inbox_members im
      WHERE im.workspace_id = m.workspace_id AND im.inbox_id = @inbox_id AND im.member_id = m.id))
ORDER BY m.id;

-- name: SetConnectionViewing :exec
UPDATE realtime_connections SET viewing_conversation_id = sqlc.narg(conversation_id), seen_at = @now
WHERE workspace_id = @workspace_id AND id = @id;

-- name: ListViewingMembers :many
SELECT DISTINCT member_id::uuid FROM realtime_connections
WHERE workspace_id = @workspace_id AND viewing_conversation_id = @conversation_id
  AND member_id IS NOT NULL AND seen_at > @fresh_after;

-- name: UpsertPushSubscription :one
INSERT INTO push_subscriptions (id, person_id, session_id, endpoint, p256dh, auth, user_agent, created_at, updated_at)
VALUES (@id, @person_id, @session_id, @endpoint, @p256dh, @auth, @user_agent, @now, @now)
ON CONFLICT (endpoint) DO UPDATE SET person_id = excluded.person_id, session_id = excluded.session_id,
    p256dh = excluded.p256dh, auth = excluded.auth, user_agent = excluded.user_agent, updated_at = excluded.updated_at,
    last_failure_at = NULL, last_error = NULL
RETURNING *;

-- name: CountOtherPushSubscriptions :one
SELECT count(*) FROM push_subscriptions WHERE person_id = @person_id AND endpoint <> @endpoint;

-- name: ListPushSubscriptions :many
SELECT * FROM push_subscriptions WHERE person_id = $1 ORDER BY created_at DESC, id DESC;

-- name: GetPushSubscription :one
SELECT * FROM push_subscriptions WHERE person_id = $1 AND id = $2;

-- name: DeletePushSubscription :execrows
DELETE FROM push_subscriptions WHERE person_id = $1 AND id = $2;

-- name: GetLivePushSubscription :one
SELECT ps.* FROM push_subscriptions ps JOIN sessions s ON s.id = ps.session_id
WHERE ps.id = @id AND s.expires_at > @now;

-- name: ListLivePushSubscriptionIDs :many
SELECT ps.id FROM push_subscriptions ps JOIN sessions s ON s.id = ps.session_id
WHERE ps.person_id = @person_id AND s.expires_at > @now
ORDER BY ps.created_at, ps.id;

-- name: DropPushSubscription :execrows
DELETE FROM push_subscriptions WHERE id = @id AND endpoint = @endpoint;

-- name: PushSucceeded :exec
UPDATE push_subscriptions SET last_success_at = @now::timestamptz, last_error = NULL WHERE id = @id;

-- name: PushFailed :exec
UPDATE push_subscriptions SET last_failure_at = @now::timestamptz, last_error = @error WHERE id = @id;

-- name: GetNotificationEmail :one
SELECT * FROM notification_emails WHERE workspace_id = $1 AND member_id = $2 AND conversation_id = $3;

-- name: RecordNotificationEmail :exec
INSERT INTO notification_emails (workspace_id, member_id, conversation_id, sent_at, through)
VALUES (@workspace_id, @member_id, @conversation_id, @now, @through)
ON CONFLICT (workspace_id, member_id, conversation_id) DO UPDATE SET sent_at = excluded.sent_at, through = excluded.through;

-- name: ListNotifiableMessages :many
SELECT id, kind, body, author_member_id, created_at FROM messages
WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND created_at > @after
  AND ((kind = 'message' AND author_type = 'contact')
       OR (kind = 'event' AND event->>'type' = 'assigned' AND (event->>'assignee_id')::uuid = @member_id::uuid
           AND author_member_id IS DISTINCT FROM @member_id::uuid)
       OR (kind = 'note' AND @member_id::uuid = ANY(mentions) AND author_member_id IS DISTINCT FROM @member_id::uuid))
ORDER BY created_at, id
LIMIT 50;
