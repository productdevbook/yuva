-- +goose Up
CREATE TABLE composed_mails (
    workspace_id  uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    message_id    uuid NOT NULL,
    inbox_id      uuid NOT NULL,
    client_id     text CHECK (length(client_id) BETWEEN 1 AND 200),
    to_addresses  text[] NOT NULL,
    cc_addresses  text[] NOT NULL DEFAULT '{}',
    bcc_addresses text[] NOT NULL DEFAULT '{}',
    subject       text NOT NULL,
    created_at    timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, message_id),
    UNIQUE (workspace_id, inbox_id, client_id),
    FOREIGN KEY (workspace_id, message_id) REFERENCES messages (workspace_id, id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE composed_mails;
