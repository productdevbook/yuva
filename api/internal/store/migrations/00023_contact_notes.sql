-- +goose Up
CREATE TABLE contact_notes (
    id                uuid PRIMARY KEY,
    workspace_id      uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    contact_id        uuid NOT NULL,
    author_member_id  uuid,
    author_api_key_id uuid,
    body              text NOT NULL CHECK (length(body) BETWEEN 1 AND 10000),
    created_at        timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, author_member_id) REFERENCES members (workspace_id, id) ON DELETE SET NULL (author_member_id),
    FOREIGN KEY (workspace_id, author_api_key_id) REFERENCES api_keys (workspace_id, id) ON DELETE SET NULL (author_api_key_id)
);
CREATE INDEX contact_notes_contact_idx ON contact_notes (workspace_id, contact_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE contact_notes;
