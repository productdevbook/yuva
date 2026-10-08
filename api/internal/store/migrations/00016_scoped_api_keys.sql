-- +goose Up
ALTER TABLE api_keys ADD CONSTRAINT api_keys_workspace_id_id_key UNIQUE (workspace_id, id);
ALTER TABLE api_keys
    ADD COLUMN scopes         text[] NOT NULL DEFAULT ARRAY[
        'conversations:read', 'conversations:write', 'messages:write', 'drafts:send', 'notes:write',
        'contacts:read', 'contacts:write', 'inboxes:read', 'inboxes:manage', 'labels:write',
        'canned_replies:write', 'webhooks:manage', 'workspace:manage', 'feedback:write'],
    ADD COLUMN inbox_limited  boolean NOT NULL DEFAULT false,
    ADD COLUMN expires_at     timestamptz,
    ADD COLUMN bot_name       text CHECK (length(bot_name) BETWEEN 1 AND 200),
    ADD COLUMN bot_avatar_url text CHECK (length(bot_avatar_url) BETWEEN 1 AND 2000);
ALTER TABLE api_keys ALTER COLUMN scopes DROP DEFAULT;

CREATE TABLE api_key_inboxes (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    api_key_id   uuid NOT NULL,
    inbox_id     uuid NOT NULL,
    PRIMARY KEY (workspace_id, api_key_id, inbox_id),
    FOREIGN KEY (workspace_id, api_key_id) REFERENCES api_keys (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX api_key_inboxes_inbox_idx ON api_key_inboxes (workspace_id, inbox_id);

-- An agent and an API key limited to inboxes see the same things; ids are random UUIDs, so a
-- member id and a key id never meet.
CREATE VIEW inbox_viewers AS
SELECT workspace_id, inbox_id, member_id AS viewer_id FROM inbox_members
UNION ALL
SELECT workspace_id, inbox_id, api_key_id AS viewer_id FROM api_key_inboxes;

ALTER TABLE workspaces ADD COLUMN bots_may_send boolean NOT NULL DEFAULT false;
UPDATE workspaces w SET bots_may_send = true
WHERE EXISTS (SELECT 1 FROM api_keys k WHERE k.workspace_id = w.id AND k.revoked_at IS NULL);

ALTER TABLE messages DROP CONSTRAINT messages_author_type_check;
ALTER TABLE messages ADD CONSTRAINT messages_author_type_check
    CHECK (author_type IN ('contact', 'member', 'system', 'bot'));
ALTER TABLE messages
    ADD COLUMN author_api_key_id  uuid,
    ADD COLUMN draft              boolean NOT NULL DEFAULT false,
    ADD COLUMN sent_by_member_id  uuid,
    ADD COLUMN sent_by_api_key_id uuid,
    ADD CONSTRAINT messages_draft_check CHECK (NOT draft OR (kind = 'message' AND direction = 'out')),
    ADD CONSTRAINT messages_author_api_key_fkey FOREIGN KEY (workspace_id, author_api_key_id)
        REFERENCES api_keys (workspace_id, id) ON DELETE SET NULL (author_api_key_id),
    ADD CONSTRAINT messages_sent_by_member_fkey FOREIGN KEY (workspace_id, sent_by_member_id)
        REFERENCES members (workspace_id, id) ON DELETE SET NULL (sent_by_member_id),
    ADD CONSTRAINT messages_sent_by_api_key_fkey FOREIGN KEY (workspace_id, sent_by_api_key_id)
        REFERENCES api_keys (workspace_id, id) ON DELETE SET NULL (sent_by_api_key_id);
CREATE INDEX messages_drafts_idx ON messages (workspace_id, conversation_id) WHERE draft;

-- +goose Down
DROP INDEX messages_drafts_idx;
DELETE FROM messages WHERE draft;
ALTER TABLE messages
    DROP CONSTRAINT messages_sent_by_api_key_fkey,
    DROP CONSTRAINT messages_sent_by_member_fkey,
    DROP CONSTRAINT messages_author_api_key_fkey,
    DROP CONSTRAINT messages_draft_check,
    DROP COLUMN sent_by_api_key_id,
    DROP COLUMN sent_by_member_id,
    DROP COLUMN draft,
    DROP COLUMN author_api_key_id;
UPDATE messages SET author_type = 'system' WHERE author_type = 'bot';
ALTER TABLE messages DROP CONSTRAINT messages_author_type_check;
ALTER TABLE messages ADD CONSTRAINT messages_author_type_check
    CHECK (author_type IN ('contact', 'member', 'system'));
ALTER TABLE workspaces DROP COLUMN bots_may_send;
DROP VIEW inbox_viewers;
DROP TABLE api_key_inboxes;
ALTER TABLE api_keys
    DROP COLUMN bot_avatar_url,
    DROP COLUMN bot_name,
    DROP COLUMN expires_at,
    DROP COLUMN inbox_limited,
    DROP COLUMN scopes;
ALTER TABLE api_keys DROP CONSTRAINT api_keys_workspace_id_id_key;
