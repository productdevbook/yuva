-- name: GetAppPushSettings :one
SELECT * FROM app_push_settings WHERE workspace_id = $1 AND channel_id = $2;

-- name: ListAppPushSettings :many
SELECT * FROM app_push_settings WHERE workspace_id = @workspace_id AND channel_id = ANY(@channel_ids::uuid[]);

-- name: SaveAppPushSettings :one
INSERT INTO app_push_settings (workspace_id, channel_id, enabled, show_content, apns_key_id, apns_team_id, apns_bundle_id,
                               apns_production, apns_key, apns_verified, fcm_project_id, fcm_client_email, fcm_credentials, updated_at)
VALUES (@workspace_id, @channel_id, @enabled, @show_content, sqlc.narg(apns_key_id), sqlc.narg(apns_team_id), sqlc.narg(apns_bundle_id),
        @apns_production, sqlc.narg(apns_key), @apns_verified, sqlc.narg(fcm_project_id), sqlc.narg(fcm_client_email), sqlc.narg(fcm_credentials), @now)
ON CONFLICT (workspace_id, channel_id) DO UPDATE SET
    enabled = excluded.enabled, show_content = excluded.show_content, apns_key_id = excluded.apns_key_id,
    apns_team_id = excluded.apns_team_id, apns_bundle_id = excluded.apns_bundle_id, apns_production = excluded.apns_production,
    apns_key = excluded.apns_key, apns_verified = excluded.apns_verified, fcm_project_id = excluded.fcm_project_id, fcm_client_email = excluded.fcm_client_email,
    fcm_credentials = excluded.fcm_credentials, last_error = NULL, last_error_at = NULL, updated_at = excluded.updated_at
RETURNING *;

-- name: SetAppPushError :exec
UPDATE app_push_settings SET last_error = sqlc.narg(error)::text,
    last_error_at = CASE WHEN sqlc.narg(error)::text IS NULL THEN NULL ELSE @now::timestamptz END
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND last_error IS DISTINCT FROM sqlc.narg(error)::text;

-- name: MarkAPNsVerified :exec
UPDATE app_push_settings SET apns_verified = true
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND NOT apns_verified AND apns_key IS NOT NULL;

-- name: UpsertPushDevice :one
INSERT INTO push_devices (id, workspace_id, channel_id, contact_id, platform, token, app_id, created_at, updated_at)
VALUES (@id, @workspace_id, @channel_id, @contact_id, @platform, @token, @app_id, @now, @now)
ON CONFLICT (workspace_id, channel_id, token) DO UPDATE SET
    contact_id = excluded.contact_id, platform = excluded.platform, app_id = excluded.app_id, updated_at = excluded.updated_at,
    created_at = CASE WHEN push_devices.contact_id = excluded.contact_id THEN push_devices.created_at ELSE excluded.created_at END
RETURNING *;

-- name: TrimPushDevices :exec
DELETE FROM push_devices p WHERE p.workspace_id = @workspace_id AND p.contact_id = @contact_id AND p.id NOT IN (
    SELECT d.id FROM push_devices d WHERE d.workspace_id = @workspace_id AND d.contact_id = @contact_id
    ORDER BY d.updated_at DESC, d.id DESC LIMIT @keep
);

-- name: DeletePushDevice :execrows
DELETE FROM push_devices WHERE workspace_id = @workspace_id AND contact_id = @contact_id AND id = @id;

-- name: DropPushDevice :execrows
DELETE FROM push_devices WHERE workspace_id = @workspace_id AND id = @id AND token = @token;

-- name: ListPushTargets :many
SELECT d.id, s.show_content FROM push_devices d
JOIN channels ch ON ch.workspace_id = d.workspace_id AND ch.id = d.channel_id
JOIN app_push_settings s ON s.workspace_id = d.workspace_id AND s.channel_id = d.channel_id
WHERE d.workspace_id = @workspace_id AND d.contact_id = @contact_id AND ch.inbox_id = @inbox_id AND s.enabled
ORDER BY d.updated_at DESC, d.id;

-- name: GetPushDeviceTarget :one
SELECT d.id, d.channel_id, d.contact_id, d.platform, d.token, d.app_id,
       s.apns_key_id, s.apns_team_id, s.apns_bundle_id, s.apns_production, s.apns_key, s.apns_verified,
       s.fcm_project_id, s.fcm_credentials, s.show_content
FROM push_devices d
JOIN app_push_settings s ON s.workspace_id = d.workspace_id AND s.channel_id = d.channel_id
WHERE d.workspace_id = @workspace_id AND d.id = @id AND d.contact_id = @contact_id AND s.enabled;

-- name: LatestPushPendingReply :one
SELECT m.id, m.created_at, m.body FROM messages m
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
LEFT JOIN contact_reads r ON r.workspace_id = m.workspace_id AND r.conversation_id = m.conversation_id
WHERE m.workspace_id = @workspace_id AND m.conversation_id = @conversation_id
  AND m.kind = 'message' AND m.direction = 'out' AND m.author_type IN ('member', 'bot') AND m.delivery_state IS NULL
  AND NOT m.draft AND m.imported_at IS NULL AND m.redacted_at IS NULL
  AND (r.last_read_at IS NULL OR (m.created_at, m.id) > (r.last_read_at, r.last_read_message_id))
  AND (c.push_through IS NULL OR m.created_at > c.push_through)
  AND m.created_at >= (SELECT min(d.created_at) FROM push_devices d WHERE d.workspace_id = c.workspace_id AND d.contact_id = c.contact_id)
ORDER BY m.created_at DESC, m.id DESC
LIMIT 1;

-- name: GetPushPreviewMessage :one
SELECT m.body, c.inbox_id, ct.locale AS contact_locale FROM messages m
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
JOIN contacts ct ON ct.workspace_id = c.workspace_id AND ct.id = c.contact_id
WHERE m.workspace_id = @workspace_id AND m.conversation_id = @conversation_id AND c.contact_id = @contact_id
  AND c.trashed_at IS NULL AND NOT c.spam
  AND m.kind = 'message' AND m.direction = 'out' AND m.author_type IN ('member', 'bot') AND m.delivery_state IS NULL
  AND NOT m.draft AND m.imported_at IS NULL AND m.redacted_at IS NULL
  AND (sqlc.narg(message_id)::uuid IS NULL OR m.id = sqlc.narg(message_id)::uuid)
ORDER BY m.created_at DESC, m.id DESC
LIMIT 1;

-- name: ListConversationsWithPushPending :many
SELECT DISTINCT c.id FROM conversations c
JOIN messages m ON m.workspace_id = c.workspace_id AND m.conversation_id = c.id
LEFT JOIN contact_reads r ON r.workspace_id = c.workspace_id AND r.conversation_id = c.id
WHERE c.workspace_id = @workspace_id AND c.contact_id = @contact_id AND c.trashed_at IS NULL
  AND m.kind = 'message' AND m.direction = 'out' AND m.author_type IN ('member', 'bot') AND m.delivery_state IS NULL
  AND NOT m.draft AND m.imported_at IS NULL
  AND (r.last_read_at IS NULL OR (m.created_at, m.id) > (r.last_read_at, r.last_read_message_id))
  AND (c.push_through IS NULL OR m.created_at > c.push_through)
  AND m.created_at >= (SELECT min(d.created_at) FROM push_devices d WHERE d.workspace_id = c.workspace_id AND d.contact_id = c.contact_id);

-- name: ClaimAppPush :exec
UPDATE conversations SET push_through = @through::timestamptz, push_sent_at = @now::timestamptz
WHERE workspace_id = @workspace_id AND id = @id;

-- name: CountUnreadForContact :one
SELECT count(*) FROM conversations c
LEFT JOIN contact_reads r ON r.workspace_id = c.workspace_id AND r.conversation_id = c.id
WHERE c.workspace_id = @workspace_id AND c.inbox_id = @inbox_id AND c.contact_id = @contact_id AND c.trashed_at IS NULL
  AND EXISTS (
      SELECT 1 FROM messages m
      WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id
        AND m.kind = 'message' AND m.direction = 'out' AND NOT m.draft AND m.imported_at IS NULL
        AND (r.last_read_at IS NULL OR (m.created_at, m.id) > (r.last_read_at, r.last_read_message_id)));

-- name: MoveContactPushDevices :exec
UPDATE push_devices SET contact_id = @to_contact
WHERE workspace_id = @workspace_id AND contact_id = @from_contact;
