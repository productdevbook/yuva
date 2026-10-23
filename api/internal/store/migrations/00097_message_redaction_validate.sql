-- +goose NO TRANSACTION
-- +goose Up
ALTER TABLE messages VALIDATE CONSTRAINT messages_redacted_by_check;
ALTER TABLE messages VALIDATE CONSTRAINT messages_redacted_by_member_fkey;
ALTER TABLE messages VALIDATE CONSTRAINT messages_redacted_by_api_key_fkey;

-- +goose Down
SELECT 1;
