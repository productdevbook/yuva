-- +goose Up
-- The from_address a draft goes out from when it is sent, scheduled or not.
ALTER TABLE messages ADD COLUMN draft_from text;
UPDATE messages SET draft_from = scheduled_from, scheduled_from = NULL WHERE scheduled_from IS NOT NULL;

-- +goose Down
UPDATE messages SET scheduled_from = draft_from WHERE draft AND scheduled_at IS NOT NULL;
ALTER TABLE messages DROP COLUMN draft_from;
