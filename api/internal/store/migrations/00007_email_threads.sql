-- +goose Up
ALTER TABLE conversations
    ADD COLUMN related_conversation_id uuid,
    ADD CONSTRAINT conversations_related_fkey FOREIGN KEY (workspace_id, related_conversation_id)
        REFERENCES conversations (workspace_id, id) ON DELETE SET NULL (related_conversation_id);

ALTER TABLE attachments
    ADD COLUMN content_id text CHECK (length(content_id) BETWEEN 1 AND 998),
    ADD COLUMN inline     boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE attachments DROP COLUMN inline, DROP COLUMN content_id;
ALTER TABLE conversations DROP CONSTRAINT conversations_related_fkey, DROP COLUMN related_conversation_id;
