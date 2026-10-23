-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS conversations_assign_waiting_idx ON conversations (workspace_id, assign_waiting_since) WHERE assign_waiting_since IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS conversations_assign_waiting_idx;
