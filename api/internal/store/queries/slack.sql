-- name: GetSlackInstallation :one
SELECT * FROM slack_installations WHERE workspace_id = @workspace_id;

-- name: LockSlackInstallation :one
SELECT * FROM slack_installations WHERE workspace_id = @workspace_id FOR UPDATE;

-- name: FindSlackInstallationByTeam :one
SELECT workspace_id, app_id FROM slack_installations WHERE team_id = @team_id;

-- name: SaveSlackInstallation :one
INSERT INTO slack_installations (workspace_id, team_id, team_name, app_id, bot_user_id, bot_token, scopes, connected_by, created_at, updated_at)
VALUES (@workspace_id, @team_id, @team_name, @app_id, @bot_user_id, @bot_token, @scopes, sqlc.narg(connected_by), @now, @now)
ON CONFLICT (workspace_id) DO UPDATE SET
    team_id = excluded.team_id, team_name = excluded.team_name, app_id = excluded.app_id, bot_user_id = excluded.bot_user_id,
    bot_token = excluded.bot_token, scopes = excluded.scopes, connected_by = excluded.connected_by,
    created_at = excluded.created_at, updated_at = excluded.updated_at, last_error = NULL, last_error_at = NULL
RETURNING *;

-- name: SetSlackReplyCommand :one
UPDATE slack_installations SET reply_command = @reply_command, updated_at = @now
WHERE workspace_id = @workspace_id
RETURNING *;

-- name: SetSlackError :exec
UPDATE slack_installations SET last_error = sqlc.narg(error)::text,
    last_error_at = CASE WHEN sqlc.narg(error)::text IS NULL THEN NULL ELSE @now::timestamptz END
WHERE workspace_id = @workspace_id AND last_error IS DISTINCT FROM sqlc.narg(error)::text;

-- name: DeleteSlackInstallation :one
DELETE FROM slack_installations WHERE workspace_id = @workspace_id RETURNING *;

-- name: DeleteSlackInboxSettings :exec
DELETE FROM slack_inbox_settings WHERE workspace_id = @workspace_id;

-- name: DeleteSlackThreads :exec
DELETE FROM slack_threads WHERE workspace_id = @workspace_id;

-- name: DeleteSlackMemberLinks :exec
DELETE FROM slack_member_links WHERE workspace_id = @workspace_id;

-- name: DeleteSlackLinkCodes :exec
DELETE FROM slack_link_codes WHERE workspace_id = @workspace_id;

-- name: SetSlackConnectState :exec
INSERT INTO slack_connect_states (workspace_id, member_id, state_hash, expires_at)
VALUES (@workspace_id, @member_id, @state_hash, @expires_at)
ON CONFLICT (workspace_id) DO UPDATE SET member_id = excluded.member_id, state_hash = excluded.state_hash, expires_at = excluded.expires_at;

-- name: TakeSlackConnectState :one
DELETE FROM slack_connect_states
WHERE workspace_id = @workspace_id AND state_hash = @state_hash AND expires_at > @now
RETURNING member_id;

-- name: GetSlackInboxSettings :one
SELECT * FROM slack_inbox_settings WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id;

-- name: SaveSlackInboxSettings :one
INSERT INTO slack_inbox_settings (workspace_id, inbox_id, new_conversations_channel, mentions_channel, sla_channel, sla_warnings, include_content, updated_at)
VALUES (@workspace_id, @inbox_id, sqlc.narg(new_conversations_channel), sqlc.narg(mentions_channel), sqlc.narg(sla_channel), @sla_warnings, @include_content, @now)
ON CONFLICT (workspace_id, inbox_id) DO UPDATE SET
    new_conversations_channel = excluded.new_conversations_channel, mentions_channel = excluded.mentions_channel,
    sla_channel = excluded.sla_channel, sla_warnings = excluded.sla_warnings,
    include_content = excluded.include_content, updated_at = excluded.updated_at
RETURNING *;

-- name: GetSlackThread :one
SELECT * FROM slack_threads WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND thread_ts = @thread_ts;

-- name: GetSlackThreadOfConversation :one
SELECT * FROM slack_threads WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND channel_id = @channel_id
ORDER BY created_at LIMIT 1;

-- name: AddSlackThread :exec
INSERT INTO slack_threads (workspace_id, channel_id, thread_ts, conversation_id, created_at)
VALUES (@workspace_id, @channel_id, @thread_ts, @conversation_id, @now)
ON CONFLICT DO NOTHING;

-- name: GetSlackMemberLink :one
SELECT * FROM slack_member_links WHERE workspace_id = @workspace_id AND member_id = @member_id;

-- name: GetSlackMemberLinkByUser :one
SELECT * FROM slack_member_links WHERE workspace_id = @workspace_id AND slack_user_id = @slack_user_id;

-- name: ListSlackMemberLinks :many
SELECT * FROM slack_member_links WHERE workspace_id = @workspace_id AND member_id = ANY(@member_ids::uuid[]);

-- name: AddSlackMemberLink :one
INSERT INTO slack_member_links (workspace_id, member_id, slack_user_id, slack_user_name, linked_at)
VALUES (@workspace_id, @member_id, @slack_user_id, @slack_user_name, @now)
ON CONFLICT (workspace_id, member_id) DO UPDATE SET
    slack_user_id = excluded.slack_user_id, slack_user_name = excluded.slack_user_name, linked_at = excluded.linked_at
RETURNING *;

-- name: DeleteSlackMemberLink :one
DELETE FROM slack_member_links WHERE workspace_id = @workspace_id AND member_id = @member_id RETURNING *;

-- name: GetSlackLinkCode :one
SELECT * FROM slack_link_codes WHERE workspace_id = @workspace_id AND member_id = @member_id;

-- name: LockSlackLinkCode :one
SELECT * FROM slack_link_codes WHERE workspace_id = @workspace_id AND member_id = @member_id FOR UPDATE;

-- name: SetSlackLinkCode :one
INSERT INTO slack_link_codes (workspace_id, member_id, slack_user_id, slack_user_name, code_hash, attempts, expires_at)
VALUES (@workspace_id, @member_id, @slack_user_id, @slack_user_name, @code_hash, 0, @expires_at)
ON CONFLICT (workspace_id, member_id) DO UPDATE SET
    slack_user_id = excluded.slack_user_id, slack_user_name = excluded.slack_user_name, code_hash = excluded.code_hash,
    attempts = 0, expires_at = excluded.expires_at
RETURNING *;

-- name: CountSlackLinkCodeAttempt :exec
UPDATE slack_link_codes SET attempts = attempts + 1 WHERE workspace_id = @workspace_id AND member_id = @member_id;

-- name: DeleteSlackLinkCode :exec
DELETE FROM slack_link_codes WHERE workspace_id = @workspace_id AND member_id = @member_id;

-- name: ClaimSlackEvent :execrows
INSERT INTO slack_events (workspace_id, event_id, received_at) VALUES (@workspace_id, @event_id, @now)
ON CONFLICT DO NOTHING;

-- name: PruneSlackEvents :exec
DELETE FROM slack_events WHERE workspace_id = @workspace_id AND received_at < @before;
