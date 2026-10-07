-- +goose Up
ALTER TABLE workspaces ADD COLUMN retention_days integer CHECK (retention_days BETWEEN 1 AND 36500);
CREATE INDEX conversations_closed_idx ON conversations (workspace_id, updated_at) WHERE status = 'closed';
CREATE INDEX message_emails_raw_idx ON message_emails (workspace_id, created_at) WHERE raw_key IS NOT NULL;

-- +goose Down
DROP INDEX message_emails_raw_idx;
DROP INDEX conversations_closed_idx;
ALTER TABLE workspaces DROP COLUMN retention_days;
