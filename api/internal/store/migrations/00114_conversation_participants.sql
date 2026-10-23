-- +goose Up
CREATE TABLE conversation_participants (
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    conversation_id uuid NOT NULL,
    email           text NOT NULL CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 320),
    role            text NOT NULL DEFAULT 'cc' CHECK (role IN ('cc')),
    status          text NOT NULL CHECK (status IN ('active', 'suggested')),
    created_at      timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, conversation_id, email),
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX conversation_participants_email_idx ON conversation_participants (workspace_id, email);

ALTER TABLE message_emails
    ADD COLUMN bcc_addresses text[] NOT NULL DEFAULT '{}',
    ADD COLUMN shared boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE message_emails DROP COLUMN bcc_addresses, DROP COLUMN shared;
DROP TABLE conversation_participants;
