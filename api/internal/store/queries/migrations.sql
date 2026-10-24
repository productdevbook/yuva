-- name: CreateMigration :one
INSERT INTO migrations (id, workspace_id, source, preset, source_address, inbox_id, channel_id, options,
    created_by_member_id, created_by_api_key_id, created_via, created_at, updated_at)
VALUES (@id, @workspace_id, @source, @preset, @source_address, sqlc.narg(inbox_id), sqlc.narg(channel_id), @options,
    sqlc.narg(member_id), sqlc.narg(api_key_id), sqlc.narg(via), @now, @now)
RETURNING *;

-- name: GetMigration :one
SELECT * FROM migrations WHERE workspace_id = $1 AND id = $2;

-- name: LockMigration :one
SELECT * FROM migrations WHERE workspace_id = $1 AND id = $2 FOR UPDATE;

-- name: ListMigrations :many
SELECT m.* FROM migrations m
LEFT JOIN inboxes i ON i.workspace_id = m.workspace_id AND i.id = m.inbox_id
WHERE m.workspace_id = @workspace_id
  AND (cardinality(@states::text[]) = 0 OR m.state = ANY(@states::text[]))
  AND (i.owner_member_id IS NULL OR i.owner_member_id = @viewer_id::uuid)
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (m.created_at, m.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY m.created_at DESC, m.id DESC
LIMIT @lim;

-- name: LockInboxMigrations :many
SELECT id, state FROM migrations WHERE workspace_id = @workspace_id AND inbox_id = @inbox_id ORDER BY id FOR UPDATE;

-- name: UpdateMigration :one
UPDATE migrations SET preset = @preset, source_address = @source_address, inbox_id = sqlc.narg(inbox_id),
    channel_id = sqlc.narg(channel_id), options = @options, state = @state, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: SetMigrationState :one
UPDATE migrations SET state = @state, state_reason = @state_reason, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteMigration :execrows
DELETE FROM migrations WHERE workspace_id = $1 AND id = $2;

-- name: ListMigrationSources :many
SELECT id, source, preset FROM migrations WHERE workspace_id = @workspace_id AND id = ANY(@ids::uuid[]);

-- name: SetMigrationCredentials :exec
INSERT INTO migration_credentials (workspace_id, migration_id, kind, sealed, host, port, tls, certificate_sha256,
    scopes, expires_at, created_at, updated_at)
VALUES (@workspace_id, @migration_id, @kind, @sealed, @host, @port, @tls, sqlc.narg(certificate_sha256),
    @scopes, sqlc.narg(expires_at), @now, @now)
ON CONFLICT (workspace_id, migration_id) DO UPDATE SET
    kind = excluded.kind, sealed = excluded.sealed, host = excluded.host, port = excluded.port, tls = excluded.tls,
    certificate_sha256 = excluded.certificate_sha256, scopes = excluded.scopes, expires_at = excluded.expires_at,
    updated_at = excluded.updated_at;

-- name: GetMigrationCredentials :one
SELECT * FROM migration_credentials WHERE workspace_id = $1 AND migration_id = $2;

-- name: ListMigrationCredentialInfos :many
SELECT migration_id, kind, host, port, tls, certificate_sha256, scopes, expires_at, updated_at
FROM migration_credentials
WHERE workspace_id = @workspace_id AND migration_id = ANY(@migration_ids::uuid[]);

-- name: AddMigrationAudit :one
INSERT INTO migration_audit (workspace_id, migration_id, action, actor_member_id, actor_api_key_id, via, detail, created_at)
VALUES (@workspace_id, @migration_id, @action, sqlc.narg(member_id), sqlc.narg(api_key_id), sqlc.narg(via), @detail, @now)
RETURNING id;

-- name: SetMigrationAuditDetail :exec
UPDATE migration_audit SET detail = @detail WHERE workspace_id = @workspace_id AND id = @id;

-- name: CountMigrationTests :one
SELECT count(*) FILTER (WHERE migration_id = @migration_id)::int AS migration,
       count(*)::int AS workspace,
       coalesce(min(created_at) FILTER (WHERE migration_id = @migration_id), @since)::timestamptz AS migration_oldest,
       coalesce(min(created_at), @since)::timestamptz AS workspace_oldest
FROM migration_audit
WHERE workspace_id = @workspace_id AND action = 'tested' AND created_at > @since;

-- name: ListRecentMigrationTests :many
SELECT coalesce(detail->>'code', '')::text AS code, created_at FROM migration_audit
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND action = 'tested'
ORDER BY id DESC
LIMIT @lim;

-- name: SetMigrationRunState :one
UPDATE migrations SET state = @state, state_reason = @state_reason, waiting_until = sqlc.narg(waiting_until), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: StartMigrationCopy :one
UPDATE migrations SET state = 'copying', state_reason = '', waiting_until = NULL, started_at = coalesce(started_at, @now),
    updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: FinishMigration :one
UPDATE migrations SET state = @state, state_reason = @state_reason, waiting_until = NULL, lease_until = NULL, lease_job_id = NULL,
    finished_at = @now, credentials_deleted_at = coalesce(credentials_deleted_at, @now), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: DeleteMigrationCredentials :execrows
DELETE FROM migration_credentials WHERE workspace_id = $1 AND migration_id = $2;

-- name: SetMigrationCheckpoint :exec
UPDATE migrations SET checkpoint = @checkpoint WHERE workspace_id = @workspace_id AND id = @id;

-- name: SetMigrationEstimate :exec
UPDATE migrations SET estimate = @estimate WHERE workspace_id = @workspace_id AND id = @id;

-- name: SetMigrationThrottle :exec
UPDATE migrations SET throttle = @throttle WHERE workspace_id = @workspace_id AND id = @id;

-- name: SetMigrationCounts :exec
UPDATE migrations SET counts = @counts WHERE workspace_id = @workspace_id AND id = @id;

-- name: TakeMigrationLease :one
UPDATE migrations SET lease_until = @until, lease_job_id = @job_id
WHERE workspace_id = @workspace_id AND id = @id AND state = ANY(@states::text[])
  AND (lease_until IS NULL OR lease_until < @now OR lease_job_id = @job_id)
RETURNING *;

-- name: ReleaseMigrationLease :exec
UPDATE migrations SET lease_until = NULL, lease_job_id = NULL
WHERE workspace_id = @workspace_id AND id = @id AND lease_job_id = @job_id;

-- The watchdog looks across workspaces (see "Hosting for others later" in docs/architecture.md).
-- name: ListStaleMigrations :many
SELECT workspace_id, id, state, lease_job_id FROM migrations
WHERE state IN ('scanning', 'copying', 'syncing', 'removing') AND (lease_until IS NULL OR lease_until < @now)
  AND (waiting_until IS NULL OR waiting_until <= @now)
ORDER BY lease_until NULLS FIRST
LIMIT 500;

-- name: CountRunningMigrations :one
SELECT count(*)::int FROM migrations
WHERE workspace_id = @workspace_id AND state IN ('scanning', 'copying', 'syncing') AND id <> @id;

-- name: ImportedBytesThisMonth :one
SELECT coalesce((SELECT imported_bytes FROM usage_counters WHERE workspace_id = @workspace_id AND month = @month), 0)::bigint;

-- name: ListMigrationFolders :many
SELECT * FROM migration_folders WHERE workspace_id = $1 AND migration_id = $2 ORDER BY id;

-- name: UpsertMigrationFolder :exec
INSERT INTO migration_folders (workspace_id, migration_id, id, name, kind, selected, label_action, label_id, uidvalidity, uidnext, total, delim)
VALUES (@workspace_id, @migration_id, @id, @name, @kind, @selected, @label_action, sqlc.narg(label_id), sqlc.narg(uidvalidity), @uidnext, @total, @delim)
ON CONFLICT (workspace_id, migration_id, id) DO UPDATE SET
    name = excluded.name, kind = excluded.kind, selected = excluded.selected, label_action = excluded.label_action,
    label_id = excluded.label_id, uidvalidity = coalesce(excluded.uidvalidity, migration_folders.uidvalidity),
    uidnext = excluded.uidnext, total = excluded.total, delim = excluded.delim;

-- name: DeleteMigrationFolder :exec
DELETE FROM migration_folders WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND id = @id;

-- name: ResetMigrationFolder :exec
UPDATE migration_folders SET uidvalidity = @uidvalidity, scanned_uid = 0, size_bytes = 0, oldest_at = NULL, newest_at = NULL
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND id = @id;

-- name: DeleteMigrationFolderItems :exec
DELETE FROM migration_items WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND folder_id = @folder_id;

-- name: SetMigrationFolderScanned :exec
UPDATE migration_folders SET scanned_uid = greatest(scanned_uid, @scanned_uid), size_bytes = size_bytes + @size_bytes,
    oldest_at = least(oldest_at, sqlc.narg(oldest_at)::timestamptz), newest_at = greatest(newest_at, sqlc.narg(newest_at)::timestamptz)
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND id = @id;

-- name: SetMigrationFolderUIDValidity :exec
UPDATE migration_folders SET uidvalidity = @uidvalidity, scanned_uid = greatest(scanned_uid, @scanned_uid)
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND id = @id;

-- name: SetMigrationFolderCounts :exec
UPDATE migration_folders f SET imported = c.imported, duplicate = c.duplicate, skipped = c.skipped, failed = c.failed, bytes = c.bytes
FROM (SELECT unnest(@ids::int[]) AS id, unnest(@imported::int[]) AS imported, unnest(@duplicate::int[]) AS duplicate,
             unnest(@skipped::int[]) AS skipped, unnest(@failed::int[]) AS failed, unnest(@bytes::bigint[]) AS bytes) c
WHERE f.workspace_id = @workspace_id AND f.migration_id = @migration_id AND f.id = c.id;

-- name: InsertMigrationItems :execrows
INSERT INTO migration_items (workspace_id, migration_id, source_id, folder_id, thread_key, dedupe_key, received_at, size_bytes,
    meta, state, reason, in_copy)
SELECT @workspace_id, @migration_id, unnest(@source_ids::text[]), unnest(@folder_ids::int[]), unnest(@dedupe_keys::text[]),
    unnest(@dedupe_keys::text[]), unnest(@received_ats::timestamptz[]), unnest(@sizes::bigint[]), unnest(@metas::jsonb[]),
    @state, @reason, @in_copy
ON CONFLICT (workspace_id, migration_id, source_id) DO NOTHING;

-- name: ApplyMigrationItemsPlan :exec
UPDATE migration_items i SET state = p.state, reason = p.reason, thread_key = p.thread_key
FROM (SELECT unnest(@source_ids::text[]) AS source_id, unnest(@states::text[]) AS state, unnest(@reasons::text[]) AS reason,
             unnest(@thread_keys::text[]) AS thread_key) p
WHERE i.workspace_id = @workspace_id AND i.migration_id = @migration_id AND i.source_id = p.source_id;

-- name: DeleteMigrationThreads :exec
DELETE FROM migration_threads WHERE workspace_id = @workspace_id AND migration_id = @migration_id;

-- name: InsertMigrationThreads :exec
INSERT INTO migration_threads (workspace_id, migration_id, thread_key, last_at, items, plan)
SELECT @workspace_id, @migration_id, unnest(@thread_keys::text[]), unnest(@last_ats::timestamptz[]), unnest(@items::int[]),
    unnest(@plans::jsonb[]);

-- name: MarkMigrationItemsInCopy :execrows
UPDATE migration_items SET in_copy = true
WHERE workspace_id = @workspace_id AND migration_id = @migration_id
  AND (state IN ('planned', 'duplicate') OR (state = 'skipped' AND reason NOT IN ('unselected', 'label_only', 'unplanned')));

-- name: ListContactEmailsIn :many
SELECT ce.email, c.blocked FROM contact_emails ce
JOIN contacts c ON c.workspace_id = ce.workspace_id AND c.id = ce.contact_id
WHERE ce.workspace_id = @workspace_id AND ce.email = ANY(@emails::text[]);

-- name: ListChannelHeaderIDsIn :many
SELECT DISTINCT header_message_id FROM message_emails
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND header_message_id = ANY(@ids::text[]);

-- name: NextMigrationItems :many
WITH t AS (
    SELECT thread_key, last_at FROM migration_threads
    WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND done < items
    ORDER BY last_at DESC, thread_key
    LIMIT @threads)
SELECT i.source_id, i.folder_id, i.thread_key, i.dedupe_key, i.received_at, i.size_bytes, i.meta, i.attempts
FROM migration_items i JOIN t ON t.thread_key = i.thread_key
WHERE i.workspace_id = @workspace_id AND i.migration_id = @migration_id AND i.state = 'planned' AND i.in_copy
ORDER BY t.last_at DESC, t.thread_key, i.received_at, i.source_id
LIMIT @lim;

-- name: CountPlannedMigrationItems :one
SELECT count(*)::bigint FROM migration_items
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND state = 'planned' AND in_copy;

-- name: GetMigrationItemState :one
SELECT state FROM migration_items WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND source_id = @source_id;

-- name: LockMigrationItem :one
SELECT state, attempts, message_id FROM migration_items
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND source_id = @source_id
FOR UPDATE;

-- name: SetMigrationItemDone :exec
UPDATE migration_items SET state = @state, reason = @reason, message_id = sqlc.narg(message_id)
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND source_id = @source_id;

-- name: CountMigrationItemAttempt :one
UPDATE migration_items SET attempts = attempts + 1,
    state = CASE WHEN attempts + 1 >= @max_attempts::int THEN 'failed' ELSE state END,
    reason = CASE WHEN attempts + 1 >= @max_attempts::int THEN 'failed' ELSE reason END
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND source_id = @source_id AND state = 'planned'
RETURNING state;

-- name: AddMigrationThreadDone :exec
UPDATE migration_threads SET done = done + 1
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND thread_key = @thread_key;

-- name: LockMigrationThread :one
SELECT * FROM migration_threads
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND thread_key = @thread_key
FOR UPDATE;

-- name: SetMigrationThreadConversation :exec
UPDATE migration_threads SET conversation_id = @conversation_id
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND thread_key = @thread_key;

-- name: AddMigrationThreadItem :exec
UPDATE migration_threads SET items = items + 1, last_at = greatest(last_at, @at)
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND thread_key = @thread_key;

-- name: ListFolderPlannedItems :many
SELECT source_id, dedupe_key, thread_key FROM migration_items
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND folder_id = @folder_id AND state = 'planned';

-- name: RenameMigrationItem :exec
UPDATE migration_items SET source_id = @new_source_id, received_at = @received_at
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND source_id = @source_id AND state = 'planned';

-- name: ListMigrationItemsByDedupe :many
SELECT dedupe_key, thread_key, state FROM migration_items
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND dedupe_key = ANY(@keys::text[]);

-- name: MigrationCopyCounts :many
SELECT folder_id, state, count(*)::bigint AS n, coalesce(sum(size_bytes), 0)::bigint AS bytes FROM migration_items
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND in_copy
GROUP BY folder_id, state;

-- name: RetryFailedMigrationItems :execrows
WITH r AS (
    UPDATE migration_items SET state = 'planned', reason = '', attempts = 0
    WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND state = 'failed'
    RETURNING thread_key),
c AS (SELECT thread_key, count(*)::int AS n FROM r GROUP BY thread_key)
UPDATE migration_threads t SET done = t.done - c.n FROM c
WHERE t.workspace_id = @workspace_id AND t.migration_id = @migration_id AND t.thread_key = c.thread_key;

-- name: AddMigrationError :exec
INSERT INTO migration_errors (workspace_id, migration_id, code, detail, folder, item_date, item_subject, created_at)
SELECT @workspace_id, @migration_id, @code, @detail, @folder, sqlc.narg(item_date), @item_subject, @now
WHERE (SELECT count(*) FROM migration_errors WHERE workspace_id = @workspace_id AND migration_id = @migration_id) < 1000;

-- name: ListMigrationErrors :many
SELECT * FROM migration_errors
WHERE workspace_id = @workspace_id AND migration_id = @migration_id
  AND (sqlc.narg(before_id)::bigint IS NULL OR id < sqlc.narg(before_id)::bigint)
ORDER BY id DESC
LIMIT @lim;

-- name: LockMigrationChannel :exec
SELECT pg_advisory_xact_lock(hashtextextended('yuva_migration_channel:' || sqlc.arg(channel_id)::uuid::text, 0));

-- name: FindChannelEmailByHeader :one
SELECT message_id, conversation_id FROM message_emails
WHERE workspace_id = @workspace_id AND channel_id = @channel_id AND header_message_id = @header_message_id
LIMIT 1;

-- name: ImportConversation :one
INSERT INTO conversations (id, workspace_id, inbox_id, contact_id, channel_id, subject, status, priority, spam, channel_kind,
    last_message_at, last_activity_at, closed_at, created_at, updated_at, migration_id, imported_at)
VALUES (@id, @workspace_id, @inbox_id, @contact_id, @channel_id, @subject, @status, 'normal', false,
    (SELECT ch.kind FROM channels ch WHERE ch.workspace_id = @workspace_id AND ch.id = @channel_id),
    @at, @at, sqlc.narg(closed_at), @at, @now, @migration_id, @now)
RETURNING *;

-- name: ImportTouchConversation :exec
UPDATE conversations SET
    created_at = least(created_at, @at::timestamptz),
    last_message_at = greatest(coalesce(last_message_at, @at::timestamptz), @at::timestamptz),
    last_activity_at = greatest(last_activity_at, @at::timestamptz)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: CreateImportedContact :one
INSERT INTO contacts (id, workspace_id, name, attributes, blocked, created_at, updated_at, migration_id)
VALUES (@id, @workspace_id, @name, '{}', false, @now, @now, @migration_id)
RETURNING id;

-- name: FillContactName :exec
UPDATE contacts SET name = @name WHERE workspace_id = @workspace_id AND id = @id AND name = '';

-- name: ImportMessage :exec
WITH m AS (
    INSERT INTO messages (id, workspace_id, conversation_id, kind, direction, author_type, author_member_id, author_contact_id,
        body, html, created_at, delivery_state, delivery_updated_at, imported_at, imported_author)
    VALUES (@id, @workspace_id, @conversation_id, 'message', @direction, @author_type, sqlc.narg(author_member_id),
        sqlc.narg(author_contact_id), @body, sqlc.narg(html), @created_at, sqlc.narg(delivery_state),
        CASE WHEN sqlc.narg(delivery_state)::text IS NULL THEN NULL ELSE @created_at::timestamptz END, @imported_at,
        sqlc.narg(imported_author))
    RETURNING workspace_id, author_contact_id, created_at)
UPDATE contacts SET last_active_at = greatest(coalesce(contacts.last_active_at, m.created_at), m.created_at)
FROM m WHERE contacts.workspace_id = m.workspace_id AND contacts.id = m.author_contact_id;

-- name: ImportMessageEmail :exec
INSERT INTO message_emails (workspace_id, message_id, conversation_id, channel_id, direction, header_message_id,
    in_reply_to, references_ids, from_address, to_addresses, cc_addresses, subject, full_text, full_html, quoted, headers,
    raw_key, raw_size, authentication_results, dmarc, auto, created_at, imported_at)
VALUES (@workspace_id, @message_id, @conversation_id, @channel_id, @direction, @header_message_id,
    sqlc.narg(in_reply_to), @references_ids, @from_address, @to_addresses, @cc_addresses, @subject, @full_text, sqlc.narg(full_html),
    @quoted, @headers, sqlc.narg(raw_key), sqlc.narg(raw_size), sqlc.narg(authentication_results), 'unknown', @auto, @created_at, @imported_at);

-- name: GetLabelByName :one
SELECT * FROM labels WHERE workspace_id = $1 AND name = $2;

-- name: CreateLabelIfMissing :exec
INSERT INTO labels (id, workspace_id, name, color, created_at) VALUES (@id, @workspace_id, @name, @color, @now)
ON CONFLICT (workspace_id, name) DO NOTHING;

-- The cleanup looks across workspaces (see "Hosting for others later" in docs/architecture.md).
-- name: ListFinishedMigrations :many
SELECT m.workspace_id, m.id FROM migrations m
WHERE m.state IN ('completed', 'cancelled', 'removing', 'removed') AND m.finished_at < @before
  AND EXISTS (SELECT 1 FROM migration_items i WHERE i.workspace_id = m.workspace_id AND i.migration_id = m.id)
LIMIT 100;

-- name: DeleteMigrationItems :exec
DELETE FROM migration_items WHERE workspace_id = @workspace_id AND migration_id = @migration_id;

-- name: DeleteMigrationErrors :exec
DELETE FROM migration_errors WHERE workspace_id = @workspace_id AND migration_id = @migration_id;

-- name: ListNewestMigrationThreads :many
SELECT thread_key, plan FROM migration_threads
WHERE workspace_id = @workspace_id AND migration_id = @migration_id
ORDER BY last_at DESC, thread_key
LIMIT @lim;

-- name: ListMigrationThreadItems :many
SELECT source_id, folder_id, thread_key, meta FROM migration_items
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND thread_key = ANY(@thread_keys::text[]) AND state = 'planned'
ORDER BY thread_key, received_at, source_id;

-- name: ClearMigrationLease :exec
UPDATE migrations SET lease_until = NULL, lease_job_id = NULL WHERE workspace_id = @workspace_id AND id = @id;

-- name: EnterMigrationSync :one
UPDATE migrations SET state = 'syncing', state_reason = '', waiting_until = NULL, live_from = coalesce(live_from, @now), updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: SetMigrationSynced :exec
UPDATE migrations SET synced_at = @now, quiet_since = coalesce(sqlc.narg(quiet_since)::timestamptz, quiet_since)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: SetMigrationProbeSent :one
UPDATE migrations SET probe_sent_at = @now, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: SetMigrationProbeSynced :exec
UPDATE migrations SET probe_synced_at = @now WHERE workspace_id = @workspace_id AND id = @id;

-- name: GetChannelMigration :one
SELECT * FROM migrations WHERE workspace_id = @workspace_id AND id = @id AND channel_id = @channel_id;

-- name: VerifyMigrationDelivery :one
UPDATE migrations SET verified_at = @now, verified_route = @route, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id AND verified_at IS NULL
RETURNING *;

-- name: FindForwardingMigration :one
SELECT * FROM migrations
WHERE workspace_id = @workspace_id AND channel_id = @channel_id
  AND state IN ('connected', 'scanning', 'ready', 'copying', 'paused', 'needs_attention', 'syncing')
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: SetMigrationForwardingConfirmation :one
UPDATE migrations SET forwarding_confirmation = @confirmation, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: CountMigrationHistory :many
SELECT c.migration_id::uuid AS migration_id, count(*)::bigint AS conversations,
       count(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM messages m
           WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND m.imported_at IS NULL))::bigint AS removable
FROM conversations c
WHERE c.workspace_id = @workspace_id AND c.migration_id = ANY(@migration_ids::uuid[])
GROUP BY c.migration_id;

-- A conversation someone wrote in after the import (any message, note, draft or event not imported) stays.
-- name: ListRemovableMigrationConversations :many
SELECT c.id FROM conversations c
WHERE c.workspace_id = @workspace_id AND c.migration_id = @migration_id
  AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.workspace_id = c.workspace_id AND m.conversation_id = c.id AND m.imported_at IS NULL)
ORDER BY c.id
LIMIT @max_rows;

-- Messages cascade with their author contact, so a contact that wrote anywhere stays, and so does one
-- that analytics visits are linked to.
-- name: DeleteLeftoverMigrationContacts :execrows
DELETE FROM contacts ct
WHERE ct.workspace_id = @workspace_id AND ct.migration_id = @migration_id
  AND NOT EXISTS (SELECT 1 FROM conversations c WHERE c.workspace_id = ct.workspace_id AND c.contact_id = ct.id)
  AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.workspace_id = ct.workspace_id AND m.author_contact_id = ct.id)
  AND NOT EXISTS (SELECT 1 FROM analytics_persistent_visitors v WHERE v.workspace_id = ct.workspace_id AND v.contact_id = ct.id);

-- An audit entry is kept while its migration runs and for the audit period after it ended or was deleted.
-- name: DeleteOldMigrationAudit :execrows
DELETE FROM migration_audit a
WHERE a.workspace_id = @workspace_id AND a.created_at < @before
  AND NOT EXISTS (SELECT 1 FROM migrations m WHERE m.workspace_id = a.workspace_id AND m.id = a.migration_id
                  AND (m.finished_at IS NULL OR m.finished_at >= @before));

-- name: SetMigrationFolderSeen :exec
UPDATE migration_folders SET seen_uid = greatest(seen_uid, @seen_uid)
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND id = @id;

-- name: ListMigrationDedupeKeysIn :many
SELECT dedupe_key FROM migration_items
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND dedupe_key = ANY(@keys::text[]);

-- name: ListInboxImports :many
SELECT DISTINCT ON (inbox_id) inbox_id::uuid AS inbox_id, source_address, state, counts FROM migrations
WHERE workspace_id = @workspace_id AND inbox_id = ANY(@inbox_ids::uuid[])
  AND (state IN ('scanning', 'copying', 'syncing') OR (state IN ('paused', 'needs_attention') AND started_at IS NOT NULL))
ORDER BY inbox_id, created_at DESC, id DESC;

-- name: ResetMigrationPlan :exec
UPDATE migration_items SET state = 'skipped', thread_key = dedupe_key, in_copy = false,
    reason = CASE WHEN folder_id = ANY(@label_folder_ids::int[]) THEN 'label_only'
                  WHEN folder_id = ANY(@import_folder_ids::int[]) THEN 'unplanned'
                  ELSE 'unselected' END
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND state <> 'imported';

-- name: DeleteMigrationPlanNodes :exec
DELETE FROM migration_plan_nodes WHERE workspace_id = @workspace_id AND migration_id = @migration_id;

-- name: DeleteMigrationPlanNames :exec
DELETE FROM migration_plan_names WHERE workspace_id = @workspace_id AND migration_id = @migration_id;

-- name: InsertMigrationPlanNames :exec
INSERT INTO migration_plan_names (workspace_id, migration_id, email, name)
SELECT DISTINCT ON (c.email) @workspace_id, @migration_id, c.email, c.name FROM (
    SELECT x.a->>'email' AS email, x.a->>'name' AS name, count(*) AS n
    FROM migration_items i
    CROSS JOIN LATERAL (
        SELECT i.meta->'from' AS a
        UNION ALL SELECT jsonb_array_elements(coalesce(i.meta->'to', '[]'::jsonb))
        UNION ALL SELECT jsonb_array_elements(coalesce(i.meta->'cc', '[]'::jsonb))
    ) x
    WHERE i.workspace_id = @workspace_id AND i.migration_id = @migration_id AND i.folder_id = ANY(@folder_ids::int[])
      AND length(coalesce(x.a->>'email', '')) BETWEEN 1 AND 998 AND coalesce(x.a->>'name', '') <> ''
    GROUP BY 1, 2
) c
ORDER BY c.email, c.n DESC, c.name COLLATE "C";

-- name: ListMigrationPlanNames :many
SELECT email, name FROM migration_plan_names
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND email = ANY(@emails::text[]);

-- name: ListMigrationPlanPrimaries :many
SELECT DISTINCT ON (dedupe_key) dedupe_key, coalesce(meta->'refs', '[]'::jsonb)::jsonb AS refs, coalesce(meta->>'thread', '')::text AS thread
FROM migration_items
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND folder_id = ANY(@folder_ids::int[]) AND dedupe_key > @after
ORDER BY dedupe_key, array_position(@folder_ids::int[], folder_id), source_id COLLATE "C"
LIMIT @lim;

-- name: ListMigrationPlanNodes :many
SELECT node, root, size FROM migration_plan_nodes
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND node = ANY(@nodes::text[]);

-- name: MoveMigrationPlanRoots :exec
UPDATE migration_plan_nodes n SET root = m.new_root
FROM (SELECT unnest(@old_roots::text[]) AS old_root, unnest(@new_roots::text[]) AS new_root) m
WHERE n.workspace_id = @workspace_id AND n.migration_id = @migration_id AND n.root = m.old_root;

-- name: InsertMigrationPlanNodes :exec
INSERT INTO migration_plan_nodes (workspace_id, migration_id, node, root, size)
SELECT @workspace_id, @migration_id, unnest(@nodes::text[]), unnest(@roots::text[]), unnest(@sizes::int[]);

-- name: SetMigrationPlanSizes :exec
UPDATE migration_plan_nodes n SET size = s.size
FROM (SELECT unnest(@nodes::text[]) AS node, unnest(@sizes::int[]) AS size) s
WHERE n.workspace_id = @workspace_id AND n.migration_id = @migration_id AND n.node = s.node;

-- name: ListMigrationPlanRoots :many
SELECT node, size FROM migration_plan_nodes
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND node = root AND node > @after
ORDER BY node
LIMIT @lim;

-- name: ListMigrationPlanItems :many
SELECT i.source_id, i.folder_id, i.dedupe_key, i.received_at, i.size_bytes, i.meta
FROM migration_plan_nodes n
JOIN migration_items i ON i.workspace_id = n.workspace_id AND i.migration_id = n.migration_id AND i.dedupe_key = n.node
WHERE n.workspace_id = @workspace_id AND n.migration_id = @migration_id AND n.root = ANY(@roots::text[]);

-- name: CountMigrationPlanContacts :one
WITH c AS (
    SELECT t.plan->'contact'->>'email' AS email FROM migration_threads t
    WHERE t.workspace_id = @workspace_id AND t.migration_id = @migration_id
    UNION
    SELECT coalesce(i.meta->'from'->>'email', '') FROM migration_items i
    WHERE i.workspace_id = @workspace_id AND i.migration_id = @migration_id AND i.state = 'planned'
      AND NOT coalesce(i.meta->'from'->>'email', '') = ANY(@team::text[]))
SELECT count(*)::bigint AS total,
    count(*) FILTER (WHERE EXISTS (
        SELECT 1 FROM contact_emails ce JOIN contacts ct ON ct.workspace_id = ce.workspace_id AND ct.id = ce.contact_id
        WHERE ce.workspace_id = @workspace_id AND ce.email = c.email))::bigint AS existing
FROM c;

-- name: ListMigrationSentFrom :many
WITH s AS (
    SELECT DISTINCT ON (dedupe_key) meta->'from'->>'email' AS email, coalesce(meta->'from'->>'name', '') AS name
    FROM migration_items
    WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND folder_id = ANY(@folder_ids::int[])
      AND coalesce(meta->'from'->>'email', '') NOT IN ('', @mailbox::text)
    ORDER BY dedupe_key, source_id),
n AS (
    SELECT DISTINCT ON (email) email, name FROM (SELECT email, name, count(*) AS c FROM s WHERE name <> '' GROUP BY email, name) x
    ORDER BY email, c DESC, name COLLATE "C")
SELECT s.email::text AS email, coalesce(n.name, '')::text AS name, count(*)::bigint AS messages
FROM s LEFT JOIN n ON n.email = s.email
GROUP BY s.email, n.name
ORDER BY messages DESC, s.email COLLATE "C"
LIMIT @lim;

-- name: SetMigrationUpload :one
UPDATE migrations SET upload = @upload, state = @state, updated_at = @now
WHERE workspace_id = @workspace_id AND id = @id
RETURNING *;

-- name: SetMigrationUploadScan :exec
UPDATE migrations SET upload = jsonb_set(coalesce(upload, '{}'), '{scan}', @scan::jsonb)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: SetMigrationFollows :exec
UPDATE migrations SET follows_migration_id = sqlc.narg(follows_migration_id)
WHERE workspace_id = @workspace_id AND id = @id;

-- name: GetMigrationUploadPart :one
SELECT * FROM migration_upload_parts WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND part = @part FOR UPDATE;

-- name: UpsertMigrationUploadPart :exec
INSERT INTO migration_upload_parts (workspace_id, migration_id, part, size_bytes, sha256, storage_key, created_at)
VALUES (@workspace_id, @migration_id, @part, @size_bytes, @sha256, @storage_key, @now)
ON CONFLICT (workspace_id, migration_id, part) DO UPDATE SET
    size_bytes = excluded.size_bytes, sha256 = excluded.sha256, storage_key = excluded.storage_key, created_at = excluded.created_at;

-- name: ListMigrationUploadParts :many
SELECT part, size_bytes, sha256, storage_key FROM migration_upload_parts
WHERE workspace_id = @workspace_id AND migration_id = @migration_id
ORDER BY part;

-- name: ListMigrationUploadTotals :many
SELECT migration_id, count(*)::int AS parts, coalesce(sum(size_bytes), 0)::bigint AS bytes FROM migration_upload_parts
WHERE workspace_id = @workspace_id AND migration_id = ANY(@migration_ids::uuid[])
GROUP BY migration_id;

-- name: DeleteMigrationUploadParts :exec
DELETE FROM migration_upload_parts WHERE workspace_id = @workspace_id AND migration_id = @migration_id;

-- name: ListWorkspaceMigrationUploadKeys :many
SELECT storage_key FROM migration_upload_parts WHERE workspace_id = @workspace_id;

-- name: ListEndedMigrationUploads :many
SELECT DISTINCT m.workspace_id, m.id FROM migrations m
JOIN migration_upload_parts p ON p.workspace_id = m.workspace_id AND p.migration_id = m.id
WHERE m.state IN ('completed', 'cancelled', 'removing', 'removed')
LIMIT 100;

-- name: AddMigrationFolderTotals :exec
UPDATE migration_folders f SET total = f.total + a.n
FROM (SELECT unnest(@ids::int[]) AS id, unnest(@adds::int[]) AS n) a
WHERE f.workspace_id = @workspace_id AND f.migration_id = @migration_id AND f.id = a.id;

-- name: FindFollowedFileMigration :one
SELECT m.id, max(f.newest_at)::timestamptz AS newest_at
FROM migrations m
JOIN migration_folders f ON f.workspace_id = m.workspace_id AND f.migration_id = m.id AND f.selected
WHERE m.workspace_id = @workspace_id AND m.channel_id = @channel_id AND m.source = 'file' AND m.source_address = @source_address
  AND m.started_at IS NOT NULL AND m.state NOT IN ('removing', 'removed') AND f.newest_at IS NOT NULL
GROUP BY m.id, m.created_at
ORDER BY m.created_at DESC
LIMIT 1;

-- name: SetMigrationUploadDeleted :exec
UPDATE migrations SET upload = jsonb_set(upload, '{deleted_at}', to_jsonb(@now::timestamptz))
WHERE workspace_id = @workspace_id AND id = @id AND upload IS NOT NULL AND NOT upload ? 'deleted_at';

-- name: FindThreadConversationInChannel :one
SELECT e.conversation_id FROM migration_items i
JOIN message_emails e ON e.workspace_id = i.workspace_id AND e.channel_id = @channel_id AND e.header_message_id = i.dedupe_key
WHERE i.workspace_id = @workspace_id AND i.migration_id = @migration_id AND i.thread_key = @thread_key
  AND i.state = 'duplicate' AND i.reason = 'in_yuva'
ORDER BY i.received_at DESC
LIMIT 1;

-- name: ClearMigrationItemMeta :exec
UPDATE migration_items SET meta = '{}'
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND meta <> '{}';

-- name: ClearMigrationThreadPlans :exec
UPDATE migration_threads SET plan = '{}'
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND plan <> '{}';

-- name: ClearMigrationErrorSubjects :exec
UPDATE migration_errors SET item_subject = ''
WHERE workspace_id = @workspace_id AND migration_id = @migration_id AND item_subject <> '';

-- name: ForgetMigrationAddresses :exec
WITH hit AS (
    SELECT t.migration_id, t.thread_key FROM migration_threads t
    WHERE t.workspace_id = @workspace_id AND lower(t.plan #>> '{contact,email}') = ANY(@emails::text[])
    UNION
    SELECT i.migration_id, i.thread_key FROM migration_items i
    WHERE i.workspace_id = @workspace_id AND (
        lower(i.meta #>> '{from,email}') = ANY(@emails::text[])
        OR lower(i.meta #>> '{requester,email}') = ANY(@emails::text[])
        OR EXISTS (SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(i.meta -> 'to') = 'array' THEN i.meta -> 'to' ELSE '[]'::jsonb END) e
                   WHERE lower(e ->> 'email') = ANY(@emails::text[]))
        OR EXISTS (SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(i.meta -> 'cc') = 'array' THEN i.meta -> 'cc' ELSE '[]'::jsonb END) e
                   WHERE lower(e ->> 'email') = ANY(@emails::text[])))
), pending AS (
    SELECT i.migration_id, i.thread_key, count(*)::int AS n FROM migration_items i
    JOIN hit h ON h.migration_id = i.migration_id AND h.thread_key = i.thread_key
    WHERE i.workspace_id = @workspace_id AND i.state = 'planned'
    GROUP BY i.migration_id, i.thread_key
), skipped AS (
    UPDATE migration_items i SET state = 'skipped', reason = 'contact_deleted', in_copy = true, meta = '{}'
    FROM hit h
    WHERE i.workspace_id = @workspace_id AND i.migration_id = h.migration_id AND i.thread_key = h.thread_key AND i.state IN ('planned', 'failed')
    RETURNING i.migration_id
), cleared AS (
    UPDATE migration_items i SET meta = '{}'
    FROM hit h
    WHERE i.workspace_id = @workspace_id AND i.migration_id = h.migration_id AND i.thread_key = h.thread_key AND i.state NOT IN ('planned', 'failed')
    RETURNING i.migration_id
), names AS (
    DELETE FROM migration_plan_names WHERE workspace_id = @workspace_id AND email = ANY(@emails::text[]) RETURNING migration_id
)
UPDATE migration_threads t SET done = t.done + coalesce(p.n, 0), plan = '{}'
FROM hit h LEFT JOIN pending p ON p.migration_id = h.migration_id AND p.thread_key = h.thread_key
WHERE t.workspace_id = @workspace_id AND t.migration_id = h.migration_id AND t.thread_key = h.thread_key;

-- name: LockMigrationCredentials :one
SELECT * FROM migration_credentials WHERE workspace_id = $1 AND migration_id = $2 FOR UPDATE;
