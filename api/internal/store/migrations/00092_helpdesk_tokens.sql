-- +goose Up
-- A help desk that connects with an API token stores it as kind 'token'; the help desks a server
-- reads are its registry's, so a new one needs no change here.
ALTER TABLE migration_credentials DROP CONSTRAINT migration_credentials_kind_check;
ALTER TABLE migration_credentials ADD CONSTRAINT migration_credentials_kind_check CHECK (kind IN ('password', 'oauth', 'token'));
ALTER TABLE migrations DROP CONSTRAINT migrations_source_check;
ALTER TABLE migrations ADD CONSTRAINT migrations_source_check CHECK (source ~ '^[a-z0-9_]{1,40}$');

-- +goose Down
DELETE FROM migration_credentials WHERE kind = 'token';
ALTER TABLE migration_credentials DROP CONSTRAINT migration_credentials_kind_check;
ALTER TABLE migration_credentials ADD CONSTRAINT migration_credentials_kind_check CHECK (kind IN ('password', 'oauth'));
ALTER TABLE migrations DROP CONSTRAINT migrations_source_check;
ALTER TABLE migrations ADD CONSTRAINT migrations_source_check CHECK (source IN ('imap', 'file', 'zendesk', 'helpscout'));
