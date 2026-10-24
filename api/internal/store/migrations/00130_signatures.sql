-- +goose Up
ALTER TABLE members ADD COLUMN title text NOT NULL DEFAULT '' CHECK (char_length(title) <= 100);

ALTER TABLE inboxes
    ADD COLUMN signature           text NOT NULL DEFAULT '' CHECK (char_length(signature) <= 4000),
    ADD COLUMN signature_image_ids uuid[] NOT NULL DEFAULT '{}';

CREATE TABLE member_signatures (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    member_id    uuid NOT NULL,
    inbox_id     uuid NOT NULL,
    body         text NOT NULL CHECK (char_length(body) <= 4000),
    image_ids    uuid[] NOT NULL DEFAULT '{}',
    updated_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, member_id, inbox_id),
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE
);

ALTER TABLE messages
    ADD COLUMN signature_html text,
    ADD COLUMN signature_text text;

-- +goose Down
ALTER TABLE messages DROP COLUMN signature_text, DROP COLUMN signature_html;
DROP TABLE member_signatures;
ALTER TABLE inboxes DROP COLUMN signature_image_ids, DROP COLUMN signature;
ALTER TABLE members DROP COLUMN title;
