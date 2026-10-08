-- +goose Up
ALTER TABLE workspaces ADD COLUMN deleted_at timestamptz;

-- +goose Down
ALTER TABLE workspaces DROP COLUMN deleted_at;
