-- +goose Up
ALTER TABLE contact_sessions ADD COLUMN IF NOT EXISTS first_event_id bigint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE contact_sessions DROP COLUMN IF EXISTS first_event_id;
