-- +goose Up
CREATE INDEX messages_mentions_idx ON messages USING gin (mentions) WHERE kind = 'note';

-- +goose Down
DROP INDEX messages_mentions_idx;
