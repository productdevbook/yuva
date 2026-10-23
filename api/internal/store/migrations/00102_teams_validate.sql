-- +goose NO TRANSACTION
-- +goose Up
ALTER TABLE conversations VALIDATE CONSTRAINT conversations_team_fkey;
CREATE INDEX CONCURRENTLY IF NOT EXISTS conversations_team_idx ON conversations (workspace_id, team_id) WHERE team_id IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS conversations_team_idx;
