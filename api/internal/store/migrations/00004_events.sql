-- +goose Up
CREATE TABLE events (
    id              bigserial PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    type            text NOT NULL,
    inbox_id        uuid,
    conversation_id uuid,
    payload         jsonb NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX events_workspace_id_idx ON events (workspace_id, id);
CREATE INDEX events_created_at_idx ON events (created_at);

-- +goose Down
DROP TABLE events;
