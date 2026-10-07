-- +goose Up
CREATE TABLE identity_token_ids (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    inbox_id uuid NOT NULL,
    jti text NOT NULL CHECK (length(jti) BETWEEN 1 AND 200),
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, inbox_id, jti),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX identity_token_ids_expires_idx ON identity_token_ids (workspace_id, inbox_id, expires_at);

-- +goose Down
DROP TABLE identity_token_ids;
