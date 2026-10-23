-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS conversations_sla_due_idx ON conversations (workspace_id, sla_due_at) WHERE sla_due_at IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS conversations_snoozed_idx ON conversations (workspace_id, snooze_until) WHERE status = 'snoozed';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS conversations_snoozed_idx;
DROP INDEX CONCURRENTLY IF EXISTS conversations_sla_due_idx;
