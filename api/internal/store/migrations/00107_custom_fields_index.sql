-- +goose NO TRANSACTION
-- +goose Up
ALTER TABLE conversations VALIDATE CONSTRAINT conversations_fields_object;
CREATE INDEX CONCURRENTLY IF NOT EXISTS conversations_fields_idx ON conversations USING gin (fields jsonb_path_ops);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS conversations_fields_idx;
