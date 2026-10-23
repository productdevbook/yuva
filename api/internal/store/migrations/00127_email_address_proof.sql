-- +goose Up
ALTER TABLE email_channels
    ADD COLUMN verified_at             timestamptz,
    ADD COLUMN verified_by             text CHECK (verified_by IN ('code', 'dns', 'received')),
    ADD COLUMN verify_problem          text CHECK (length(verify_problem) <= 500),
    ADD COLUMN verify_checked_at       timestamptz,
    ADD COLUMN verify_code_hash        bytea,
    ADD COLUMN verify_code_expires_at  timestamptz,
    ADD COLUMN verify_attempts         integer NOT NULL DEFAULT 0,
    ADD COLUMN verify_message_id       text,
    ADD COLUMN verify_relay_message_id text,
    ADD COLUMN verify_sent_at          timestamptz,
    ADD COLUMN verify_sends            integer NOT NULL DEFAULT 0,
    ADD COLUMN verify_window_at        timestamptz,
    ADD CONSTRAINT email_channels_verified_check CHECK ((verified_at IS NULL) = (verified_by IS NULL));

-- Addresses were unique on the server until now, so no two upgraded channels share one.
UPDATE email_channels e SET verified_at = now(), verified_by = 'received'
WHERE EXISTS (
    SELECT 1 FROM message_emails m
    WHERE m.workspace_id = e.workspace_id AND m.channel_id = e.channel_id AND m.direction = 'in' AND m.imported_at IS NULL
);

DROP INDEX email_channels_address_key;
CREATE UNIQUE INDEX email_channels_workspace_address_key ON email_channels (workspace_id, address);
CREATE UNIQUE INDEX email_channels_verified_address_key ON email_channels (address) WHERE verified_at IS NOT NULL;

-- +goose Down
DROP INDEX email_channels_verified_address_key;
DROP INDEX email_channels_workspace_address_key;
CREATE UNIQUE INDEX email_channels_address_key ON email_channels (address);
ALTER TABLE email_channels
    DROP CONSTRAINT email_channels_verified_check,
    DROP COLUMN verify_window_at,
    DROP COLUMN verify_sends,
    DROP COLUMN verify_sent_at,
    DROP COLUMN verify_relay_message_id,
    DROP COLUMN verify_message_id,
    DROP COLUMN verify_attempts,
    DROP COLUMN verify_code_expires_at,
    DROP COLUMN verify_code_hash,
    DROP COLUMN verify_checked_at,
    DROP COLUMN verify_problem,
    DROP COLUMN verified_by,
    DROP COLUMN verified_at;
