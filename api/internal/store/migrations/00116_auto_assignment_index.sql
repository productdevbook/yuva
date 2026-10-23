-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS conversations_assign_waiting_idx ON conversations (workspace_id, assign_waiting_since) WHERE assign_waiting_since IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS conversations_open_assignee_idx ON conversations (workspace_id, assignee_id) WHERE status = 'open' AND NOT spam AND trashed_at IS NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS conversations_open_assignee_idx;
DROP INDEX CONCURRENTLY IF EXISTS conversations_assign_waiting_idx;
