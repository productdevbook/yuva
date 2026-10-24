-- The conversation is locked by the caller, so the count holds until the insert.
-- name: AddConversationParticipant :execrows
INSERT INTO conversation_participants (workspace_id, conversation_id, email, role, status, created_at)
SELECT @workspace_id, @conversation_id, @email, @role, @status, @now
WHERE (SELECT count(*) FROM conversation_participants
       WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id) < @max_participants::int
ON CONFLICT DO NOTHING;

-- A member or the contact copying a forwarded participant makes them cc.
-- name: ActivateConversationParticipant :execrows
UPDATE conversation_participants SET status = 'active', role = CASE WHEN @role::text = 'cc' THEN 'cc' ELSE role END
WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND email = @email
  AND (status = 'suggested' OR (@role::text = 'cc' AND role = 'forwarded'));

-- name: GetConversationParticipant :one
SELECT role, status FROM conversation_participants
WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND email = @email;

-- name: IsForwardRecipient :one
SELECT EXISTS (
  SELECT 1 FROM message_emails e
  JOIN messages m ON m.workspace_id = e.workspace_id AND m.id = e.message_id
  WHERE e.workspace_id = @workspace_id AND e.conversation_id = @conversation_id AND e.direction = 'out'
    AND m.kind = 'forward' AND (e.header_message_id = ANY(@ids::text[]) OR e.relay_message_id = ANY(@ids::text[]))
    AND @email::text = ANY(e.to_addresses)
)::bool AS recipient;

-- name: ScrubForwardRecipients :exec
UPDATE messages SET forward = jsonb_set(forward, '{to}', coalesce((
    SELECT jsonb_agg(t ORDER BY i) FROM jsonb_array_elements_text(forward->'to') WITH ORDINALITY u (t, i)
    WHERE t <> ALL(@emails::text[])), '[]'::jsonb))
WHERE workspace_id = @workspace_id AND kind = 'forward' AND forward IS NOT NULL AND forward->'to' ?| @emails::text[];

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

-- name: ScrubComposedMailAddresses :exec
UPDATE composed_mails SET
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
  UNION ALL
  SELECT m.conversation_id, a.email, false AS participant
  FROM messages m
  JOIN addrs a ON m.forward->'to' ? a.email
  WHERE m.workspace_id = @workspace_id AND m.kind = 'forward' AND m.forward IS NOT NULL
)
SELECT cv.id, cv.inbox_id, cv.subject, cv.created_at,
       array_agg(DISTINCT h.email ORDER BY h.email)::text[] AS emails, bool_or(h.participant)::bool AS participant
FROM hits h
JOIN conversations cv ON cv.workspace_id = @workspace_id AND cv.id = h.conversation_id
WHERE cv.contact_id <> @contact_id AND cv.trashed_at IS NULL
  AND ((@all_inboxes::bool AND NOT EXISTS (SELECT 1 FROM inboxes pmb WHERE pmb.workspace_id = cv.workspace_id AND pmb.id = cv.inbox_id AND pmb.owner_member_id IS NOT NULL)) OR EXISTS (
      SELECT 1 FROM inbox_viewers iv
      WHERE iv.workspace_id = cv.workspace_id AND iv.inbox_id = cv.inbox_id AND iv.viewer_id = @viewer_id::uuid))
  AND conversation_visible_to(cv.draft_of, sqlc.narg(viewer_member_id)::uuid)
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL
       OR (cv.created_at, cv.id) < (sqlc.narg(cursor_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
GROUP BY cv.id
ORDER BY cv.created_at DESC, cv.id DESC
LIMIT @lim;

-- A suggested address that answers a forward it received becomes a forwarded participant.
-- name: PromoteSuggestedToForwarded :execrows
UPDATE conversation_participants SET status = 'active', role = 'forwarded'
WHERE workspace_id = @workspace_id AND conversation_id = @conversation_id AND email = @email AND status = 'suggested';

-- The members who sent the forwards an answer names in In-Reply-To or References.
-- name: ListForwardAnswerMembers :many
SELECT DISTINCT coalesce(m.sent_by_member_id, m.author_member_id)::uuid AS member_id
FROM message_emails a
JOIN message_emails f ON f.workspace_id = a.workspace_id AND f.conversation_id = a.conversation_id AND f.direction = 'out'
  AND (f.header_message_id = ANY(array_append(a.references_ids, a.in_reply_to))
       OR f.relay_message_id = ANY(array_append(a.references_ids, a.in_reply_to)))
JOIN messages m ON m.workspace_id = f.workspace_id AND m.id = f.message_id AND m.kind = 'forward'
WHERE a.workspace_id = @workspace_id AND a.message_id = @message_id
  AND coalesce(m.sent_by_member_id, m.author_member_id) IS NOT NULL;
