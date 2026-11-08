-- +goose Up
-- App and device details a widget or SDK sends: kept on the session, the latest value on the
-- conversation and on the contact.
ALTER TABLE contact_sessions ADD COLUMN client_info jsonb;
ALTER TABLE conversations ADD COLUMN client_info jsonb, ADD COLUMN client_info_at timestamptz;
ALTER TABLE contacts ADD COLUMN client_info jsonb, ADD COLUMN client_info_at timestamptz;

-- +goose Down
ALTER TABLE contacts DROP COLUMN client_info, DROP COLUMN client_info_at;
ALTER TABLE conversations DROP COLUMN client_info, DROP COLUMN client_info_at;
ALTER TABLE contact_sessions DROP COLUMN client_info;
