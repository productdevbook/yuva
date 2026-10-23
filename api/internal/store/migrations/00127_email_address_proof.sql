-- +goose Up
ALTER TABLE email_channels
    ADD COLUMN verified_at             timestamptz,
    ADD COLUMN verified_by             text CHECK (verified_by IN ('code', 'dns')),
    ADD COLUMN verify_problem          text CHECK (length(verify_problem) <= 500),
    ADD COLUMN verify_checked_at       timestamptz,
    ADD COLUMN verify_code_hash        bytea,
    ADD COLUMN verify_code_expires_at  timestamptz,
    ADD COLUMN verify_attempts         integer NOT NULL DEFAULT 0,
    ADD COLUMN verify_message_id       text,
    ADD COLUMN verify_relay_message_id text,
    ADD COLUMN verify_sent_at          timestamptz,
    ADD CONSTRAINT email_channels_verified_check CHECK ((verified_at IS NULL) = (verified_by IS NULL));

DROP INDEX email_channels_address_key;
CREATE UNIQUE INDEX email_channels_workspace_address_key ON email_channels (workspace_id, address);
CREATE UNIQUE INDEX email_channels_verified_address_key ON email_channels (address) WHERE verified_at IS NOT NULL;

ALTER TABLE operator_notices
    DROP CONSTRAINT operator_notices_kind_check,
    ADD CONSTRAINT operator_notices_kind_check
        CHECK (kind IN ('signup', 'workspace', 'broadcast_approval', 'broadcast_suspended', 'broadcast_paused', 'analytics_limit', 'analytics_dropped',
                        'ses_tenant_paused', 'workspace_deleted', 'email_unproven'));

-- Stored mail does not tell delivery by MX from delivery through a forwarding address, which the
-- channel's own workspace can send itself, so no address keeps routing without a proof: the
-- operator learns once which channels now need one.
INSERT INTO operator_notices (id, kind, email, workspace_name, detail, created_at)
SELECT gen_random_uuid(), 'email_unproven', '', w.name,
       'channel "' || c.name || '" (' || e.channel_id || ') at ' || e.address ||
       CASE WHEN e.address LIKE '*@%' THEN ' is a catch-all and needs the TXT record _yuva-mail.' || split_part(e.address, '@', 2)
            ELSE ' needs the code mail or the TXT record _yuva-mail.' || split_part(e.address, '@', 2) END ||
       '; until then it receives mail only at its forwarding address',
       now()
FROM email_channels e
JOIN channels c ON c.workspace_id = e.workspace_id AND c.id = e.channel_id
JOIN workspaces w ON w.id = e.workspace_id
WHERE w.deleted_at IS NULL;

-- +goose Down
DELETE FROM operator_notices WHERE kind = 'email_unproven';
ALTER TABLE operator_notices
    DROP CONSTRAINT operator_notices_kind_check,
    ADD CONSTRAINT operator_notices_kind_check
        CHECK (kind IN ('signup', 'workspace', 'broadcast_approval', 'broadcast_suspended', 'broadcast_paused', 'analytics_limit', 'analytics_dropped',
                        'ses_tenant_paused', 'workspace_deleted'));
DROP INDEX email_channels_verified_address_key;
DROP INDEX email_channels_workspace_address_key;
CREATE UNIQUE INDEX email_channels_address_key ON email_channels (address);
ALTER TABLE email_channels
    DROP CONSTRAINT email_channels_verified_check,
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
