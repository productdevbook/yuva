-- +goose Up
CREATE TABLE trash_empties (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    id           uuid NOT NULL,
    deleted      bigint NOT NULL DEFAULT 0 CHECK (deleted >= 0),
    created_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, id)
);

CREATE TABLE trash_empty_items (
    workspace_id    uuid NOT NULL,
    empty_id        uuid NOT NULL,
    conversation_id uuid NOT NULL,
    trashed_at      timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, empty_id, conversation_id),
    FOREIGN KEY (workspace_id, empty_id) REFERENCES trash_empties (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX trash_empty_items_conversation_idx ON trash_empty_items (workspace_id, conversation_id);

-- +goose Down
DROP TABLE trash_empty_items;
DROP TABLE trash_empties;
