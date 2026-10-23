-- The conversation is locked by the caller, so the count holds until the insert.
-- name: AddConversationParticipant :execrows
INSERT INTO conversation_participants (workspace_id, conversation_id, email, role, status, created_at)
SELECT @workspace_id, @conversation_id, @email, 'cc', @status, @now
WHERE (SELECT count(*) FROM conversation_participants
       WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id) < @max_participants::int
ON CONFLICT DO NOTHING;

-- name: ActivateConversationParticipant :execrows
UPDATE conversation_participants SET status = 'active'
WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND email = @email AND status = 'suggested';

-- name: DeleteConversationParticipant :execrows
DELETE FROM conversation_participants WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND email = @email;

-- name: ListConversationParticipants :many
SELECT p.email, p.role, p.status, p.created_at, ce.contact_id, c.name
FROM conversation_participants p
LEFT JOIN contact_emails ce ON ce.workspace_id = p.workspace_id AND ce.email = p.email
LEFT JOIN contacts c ON c.workspace_id = ce.workspace_id AND c.id = ce.contact_id
WHERE p.workspace_id = @workspace_id AND p.conversation_id = @conversation_id
ORDER BY p.created_at, p.email;

-- name: IsConversationParticipant :one
SELECT EXISTS (
  SELECT 1 FROM conversation_participants
  WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND email = @email AND status = 'active'
)::bool AS participant;

-- name: DeleteParticipantsByEmails :exec
DELETE FROM conversation_participants WHERE workspace_id = @workspace_id AND email = ANY(@emails::text[]);

-- name: ScrubMessageEmailAddresses :exec
UPDATE message_emails SET
  shared = shared OR cardinality(to_addresses) + cardinality(cc_addresses) + cardinality(bcc_addresses) > 1,
  to_addresses = ARRAY(SELECT a FROM unnest(to_addresses) WITH ORDINALITY u (a, i) WHERE a <> ALL(@emails::text[]) ORDER BY i),
  cc_addresses = ARRAY(SELECT a FROM unnest(cc_addresses) WITH ORDINALITY u (a, i) WHERE a <> ALL(@emails::text[]) ORDER BY i),
  bcc_addresses = ARRAY(SELECT a FROM unnest(bcc_addresses) WITH ORDINALITY u (a, i) WHERE a <> ALL(@emails::text[]) ORDER BY i)
WHERE workspace_id = @workspace_id
  AND (to_addresses && @emails::text[] OR cc_addresses && @emails::text[] OR bcc_addresses && @emails::text[]);

-- name: SetMessageEmailBcc :exec
UPDATE message_emails SET bcc_addresses = @bcc_addresses::text[] WHERE workspace_id = @workspace_id AND message_id = @message_id;

-- name: ListContactParticipations :many
WITH addrs AS (
  SELECT ce.email FROM contact_emails ce WHERE ce.workspace_id = @workspace_id AND ce.contact_id = @contact_id
), hits AS (
  SELECT p.conversation_id, p.email, true AS participant
  FROM conversation_participants p JOIN addrs a ON a.email = p.email
  WHERE p.workspace_id = @workspace_id
  UNION ALL
  SELECT e.conversation_id, a.email, false AS participant
  FROM message_emails e
  JOIN addrs a ON a.email = ANY(e.to_addresses) OR a.email = ANY(e.cc_addresses) OR a.email = ANY(e.bcc_addresses)
  WHERE e.workspace_id = @workspace_id
)
SELECT cv.id, cv.inbox_id, cv.subject, cv.created_at,
       array_agg(DISTINCT h.email ORDER BY h.email)::text[] AS emails, bool_or(h.participant)::bool AS participant
FROM hits h
JOIN conversations cv ON cv.workspace_id = @workspace_id AND cv.id = h.conversation_id
WHERE cv.contact_id <> @contact_id AND cv.trashed_at IS NULL
  AND (@all_inboxes::bool OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = cv.workspace_id AND iv.inbox_id = cv.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (cv.created_at, cv.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
GROUP BY cv.id
ORDER BY cv.created_at DESC, cv.id DESC
LIMIT @lim;
