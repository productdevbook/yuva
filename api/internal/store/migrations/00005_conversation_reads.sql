-- +goose Up
CREATE TABLE conversation_reads (
    workspace_id         uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    member_id            uuid NOT NULL,
    conversation_id      uuid NOT NULL,
    last_read_message_id uuid NOT NULL,
    last_read_at         timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, member_id, conversation_id),
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, last_read_message_id) REFERENCES messages (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX conversation_reads_conversation_idx ON conversation_reads (workspace_id, conversation_id);
CREATE INDEX conversation_reads_message_idx ON conversation_reads (workspace_id, last_read_message_id);

-- +goose Down
DROP TABLE conversation_reads;
