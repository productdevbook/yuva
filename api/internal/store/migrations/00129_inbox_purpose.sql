-- +goose Up
ALTER TABLE inboxes ADD COLUMN purpose text NOT NULL DEFAULT 'support' CHECK (purpose IN ('support', 'mail'));

CREATE TABLE conversation_stars (
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    member_id       uuid NOT NULL,
    conversation_id uuid NOT NULL,
    created_at      timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, member_id, conversation_id),
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX conversation_stars_conversation_idx ON conversation_stars (workspace_id, conversation_id);

-- +goose Down
DROP TABLE conversation_stars;
ALTER TABLE inboxes DROP COLUMN purpose;
