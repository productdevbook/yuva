-- +goose NO TRANSACTION
-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION address_text(addresses text[]) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT lower(array_to_string(addresses, ' '))
$$;
-- +goose StatementEnd

CREATE INDEX CONCURRENTLY IF NOT EXISTS message_emails_from_trgm_idx ON message_emails USING gin (from_address gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS message_emails_to_trgm_idx ON message_emails USING gin (address_text(to_addresses) gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS message_emails_cc_trgm_idx ON message_emails USING gin (address_text(cc_addresses) gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS contacts_name_trgm_idx ON contacts USING gin (name gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS contact_emails_trgm_idx ON contact_emails USING gin (email gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS messages_author_member_idx ON messages (workspace_id, author_member_id) WHERE author_member_id IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS messages_author_member_idx;
DROP INDEX CONCURRENTLY IF EXISTS contact_emails_trgm_idx;
DROP INDEX CONCURRENTLY IF EXISTS contacts_name_trgm_idx;
DROP INDEX CONCURRENTLY IF EXISTS message_emails_cc_trgm_idx;
DROP INDEX CONCURRENTLY IF EXISTS message_emails_to_trgm_idx;
DROP INDEX CONCURRENTLY IF EXISTS message_emails_from_trgm_idx;
DROP FUNCTION IF EXISTS address_text(text[]);
