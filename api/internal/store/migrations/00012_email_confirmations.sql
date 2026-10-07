-- +goose Up
CREATE TABLE email_confirmations (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    id uuid NOT NULL,
    contact_id uuid NOT NULL,
    inbox_id uuid NOT NULL,
    email text NOT NULL CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 320),
    token_hash bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, id),
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX email_confirmations_contact_idx ON email_confirmations (workspace_id, contact_id);

-- +goose Down
DROP TABLE email_confirmations;
