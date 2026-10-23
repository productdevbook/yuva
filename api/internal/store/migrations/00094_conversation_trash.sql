-- +goose Up
ALTER TABLE conversations
    ADD COLUMN trashed_at timestamptz,
    ADD COLUMN trashed_by_member_id uuid,
    ADD COLUMN trashed_by_api_key_id uuid,
    ADD CONSTRAINT conversations_trashed_by_check
        CHECK (trashed_at IS NOT NULL OR (trashed_by_member_id IS NULL AND trashed_by_api_key_id IS NULL)),
    ADD CONSTRAINT conversations_trashed_by_member_fkey FOREIGN KEY (workspace_id, trashed_by_member_id)
        REFERENCES members (workspace_id, id) ON DELETE SET NULL (trashed_by_member_id) NOT VALID,
    ADD CONSTRAINT conversations_trashed_by_api_key_fkey FOREIGN KEY (workspace_id, trashed_by_api_key_id)
        REFERENCES api_keys (workspace_id, id) ON DELETE SET NULL (trashed_by_api_key_id) NOT VALID;
ALTER TABLE conversations VALIDATE CONSTRAINT conversations_trashed_by_member_fkey;
ALTER TABLE conversations VALIDATE CONSTRAINT conversations_trashed_by_api_key_fkey;
CREATE INDEX conversations_trashed_idx ON conversations (workspace_id, trashed_at) WHERE trashed_at IS NOT NULL;

CREATE OR REPLACE VIEW member_mentions AS
SELECT m.workspace_id, mm.member_id, m.id AS message_id, m.conversation_id, c.inbox_id, m.created_at, m.mentions,
       (ms.message_id IS NOT NULL
        OR (r.last_read_at IS NOT NULL AND (r.last_read_at, r.last_read_message_id) >= (m.created_at, m.id)))::bool AS seen
FROM messages m
CROSS JOIN LATERAL unnest(m.mentions) AS mm(member_id)
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
LEFT JOIN conversation_reads r
       ON r.workspace_id = m.workspace_id AND r.conversation_id = m.conversation_id AND r.member_id = mm.member_id
LEFT JOIN mention_seen ms
       ON ms.workspace_id = m.workspace_id AND ms.message_id = m.id AND ms.member_id = mm.member_id
WHERE m.kind = 'note' AND NOT m.draft AND m.author_member_id IS DISTINCT FROM mm.member_id AND c.trashed_at IS NULL;

ALTER TABLE audit_log
    DROP CONSTRAINT audit_log_actor_kind_check,
    ADD CONSTRAINT audit_log_actor_kind_check CHECK (actor_kind IN ('member', 'api_key', 'oauth_grant', 'operator', 'system')),
    DROP CONSTRAINT audit_log_check,
    ADD CONSTRAINT audit_log_check CHECK ((actor_kind IN ('operator', 'system')) = (actor_id IS NULL));

-- +goose Down
DELETE FROM audit_log WHERE actor_kind = 'system';
ALTER TABLE audit_log
    DROP CONSTRAINT audit_log_check,
    ADD CONSTRAINT audit_log_check CHECK ((actor_kind = 'operator') = (actor_id IS NULL)),
    DROP CONSTRAINT audit_log_actor_kind_check,
    ADD CONSTRAINT audit_log_actor_kind_check CHECK (actor_kind IN ('member', 'api_key', 'oauth_grant', 'operator'));

CREATE OR REPLACE VIEW member_mentions AS
SELECT m.workspace_id, mm.member_id, m.id AS message_id, m.conversation_id, c.inbox_id, m.created_at, m.mentions,
       (ms.message_id IS NOT NULL
        OR (r.last_read_at IS NOT NULL AND (r.last_read_at, r.last_read_message_id) >= (m.created_at, m.id)))::bool AS seen
FROM messages m
CROSS JOIN LATERAL unnest(m.mentions) AS mm(member_id)
JOIN conversations c ON c.workspace_id = m.workspace_id AND c.id = m.conversation_id
LEFT JOIN conversation_reads r
       ON r.workspace_id = m.workspace_id AND r.conversation_id = m.conversation_id AND r.member_id = mm.member_id
LEFT JOIN mention_seen ms
       ON ms.workspace_id = m.workspace_id AND ms.message_id = m.id AND ms.member_id = mm.member_id
WHERE m.kind = 'note' AND NOT m.draft AND m.author_member_id IS DISTINCT FROM mm.member_id;

DROP INDEX conversations_trashed_idx;
ALTER TABLE conversations
    DROP CONSTRAINT conversations_trashed_by_api_key_fkey,
    DROP CONSTRAINT conversations_trashed_by_member_fkey,
    DROP CONSTRAINT conversations_trashed_by_check,
    DROP COLUMN trashed_by_api_key_id,
    DROP COLUMN trashed_by_member_id,
    DROP COLUMN trashed_at;
