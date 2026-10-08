-- +goose Up
ALTER TABLE messages ADD COLUMN mentions uuid[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE messages DROP COLUMN mentions;
