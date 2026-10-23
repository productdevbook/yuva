-- +goose Up
ALTER TABLE messages
    ADD COLUMN redacted_at timestamptz,
    ADD COLUMN redacted_by_member_id uuid,
    ADD COLUMN redacted_by_api_key_id uuid,
    ADD CONSTRAINT messages_redacted_by_check
        CHECK (redacted_at IS NOT NULL OR (redacted_by_member_id IS NULL AND redacted_by_api_key_id IS NULL)),
    ADD CONSTRAINT messages_redacted_by_member_fkey FOREIGN KEY (workspace_id, redacted_by_member_id)
        REFERENCES members (workspace_id, id) ON DELETE SET NULL (redacted_by_member_id) NOT VALID,
    ADD CONSTRAINT messages_redacted_by_api_key_fkey FOREIGN KEY (workspace_id, redacted_by_api_key_id)
        REFERENCES api_keys (workspace_id, id) ON DELETE SET NULL (redacted_by_api_key_id) NOT VALID;
ALTER TABLE messages VALIDATE CONSTRAINT messages_redacted_by_member_fkey;
ALTER TABLE messages VALIDATE CONSTRAINT messages_redacted_by_api_key_fkey;

-- +goose Down
ALTER TABLE messages
    DROP CONSTRAINT messages_redacted_by_api_key_fkey,
    DROP CONSTRAINT messages_redacted_by_member_fkey,
    DROP CONSTRAINT messages_redacted_by_check,
    DROP COLUMN redacted_by_api_key_id,
    DROP COLUMN redacted_by_member_id,
    DROP COLUMN redacted_at;
