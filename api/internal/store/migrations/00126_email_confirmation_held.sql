-- +goose Up
ALTER TABLE email_confirmations ADD COLUMN IF NOT EXISTS held_at timestamptz;

-- +goose Down
ALTER TABLE email_confirmations DROP COLUMN IF EXISTS held_at;
