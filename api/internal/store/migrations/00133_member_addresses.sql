-- +goose Up
CREATE TABLE member_addresses (
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    id              uuid NOT NULL,
    member_id       uuid NOT NULL,
    address         text NOT NULL CHECK (length(address) BETWEEN 3 AND 320 AND address = lower(address)),
    verification    text NOT NULL DEFAULT 'code' CHECK (verification IN ('code', 'approval')),
    verified_at     timestamptz,
    code_hash       bytea,
    code_expires_at timestamptz,
    code_attempts   integer NOT NULL DEFAULT 0,
    code_sent_at    timestamptz,
    created_at      timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, id),
    UNIQUE (workspace_id, address),
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX member_addresses_member_idx ON member_addresses (workspace_id, member_id);
CREATE INDEX member_addresses_verified_idx ON member_addresses (address) WHERE verified_at IS NOT NULL;

ALTER TABLE message_emails ADD COLUMN member_address_id uuid;
ALTER TABLE email_channels ADD COLUMN sending_changed_at timestamptz;

-- +goose Down
ALTER TABLE email_channels DROP COLUMN sending_changed_at;
ALTER TABLE message_emails DROP COLUMN member_address_id;
DROP TABLE member_addresses;
