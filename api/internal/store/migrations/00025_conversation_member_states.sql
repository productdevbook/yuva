-- +goose Up
CREATE TABLE conversation_member_states (
    workspace_id     uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    member_id        uuid NOT NULL,
    conversation_id  uuid NOT NULL,
    pinned_at        timestamptz,
    marked_unread_at timestamptz,
    PRIMARY KEY (workspace_id, member_id, conversation_id),
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX conversation_member_states_conversation_idx ON conversation_member_states (workspace_id, conversation_id);
CREATE INDEX conversation_member_states_pinned_idx ON conversation_member_states (workspace_id, member_id, conversation_id) WHERE pinned_at IS NOT NULL;

-- +goose Down
DROP TABLE conversation_member_states;
