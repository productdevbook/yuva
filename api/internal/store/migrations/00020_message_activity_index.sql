-- +goose Up
CREATE INDEX messages_workspace_created_idx ON messages (workspace_id, created_at);

-- +goose Down
DROP INDEX messages_workspace_created_idx;
