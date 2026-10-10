-- +goose Up
ALTER TABLE conversations ADD COLUMN last_read_by_contact_at timestamptz;
UPDATE conversations c SET last_read_by_contact_at = r.last_read_at
FROM contact_reads r
WHERE r.workspace_id = c.workspace_id AND r.conversation_id = c.id;

-- +goose Down
ALTER TABLE conversations DROP COLUMN last_read_by_contact_at;
