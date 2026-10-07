-- +goose Up
CREATE TABLE email_channels (
    workspace_id              uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    channel_id                uuid NOT NULL,
    address                   text NOT NULL CHECK (address = lower(address) AND length(address) BETWEEN 3 AND 320),
    display_name              text NOT NULL DEFAULT '' CHECK (length(display_name) <= 200),
    from_address              text CHECK (from_address = lower(from_address) AND length(from_address) BETWEEN 3 AND 320),
    smtp_host                 text NOT NULL DEFAULT '' CHECK (length(smtp_host) <= 253),
    smtp_port                 integer NOT NULL DEFAULT 587 CHECK (smtp_port BETWEEN 1 AND 65535),
    smtp_username             text NOT NULL DEFAULT '' CHECK (length(smtp_username) <= 320),
    smtp_password             bytea,
    smtp_tls                  text NOT NULL DEFAULT 'starttls' CHECK (smtp_tls IN ('starttls', 'tls', 'none')),
    auto_reply_enabled        boolean NOT NULL DEFAULT false,
    auto_reply_text           text NOT NULL DEFAULT '' CHECK (length(auto_reply_text) <= 5000),
    auto_reply_interval_hours integer NOT NULL DEFAULT 24 CHECK (auto_reply_interval_hours BETWEEN 1 AND 8760),
    PRIMARY KEY (workspace_id, channel_id),
    FOREIGN KEY (workspace_id, channel_id) REFERENCES channels (workspace_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX email_channels_address_key ON email_channels (address);

ALTER TABLE conversations
    ADD COLUMN spam boolean NOT NULL DEFAULT false,
    ADD COLUMN email_token text CHECK (email_token ~ '^[a-z0-9]{16,64}$'),
    ADD CONSTRAINT conversations_email_token_key UNIQUE (workspace_id, email_token);
CREATE INDEX conversations_channel_contact_idx ON conversations (workspace_id, channel_id, contact_id, created_at);

ALTER TABLE messages
    ADD COLUMN delivery_state      text CHECK (delivery_state IN ('queued', 'sent', 'failed')),
    ADD COLUMN delivery_error      text CHECK (length(delivery_error) <= 2000),
    ADD COLUMN delivery_updated_at timestamptz,
    ADD CONSTRAINT messages_delivery_check CHECK ((delivery_state IS NULL) = (delivery_updated_at IS NULL));

CREATE TABLE message_emails (
    workspace_id           uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    message_id             uuid NOT NULL,
    conversation_id        uuid NOT NULL,
    channel_id             uuid,
    direction              text NOT NULL CHECK (direction IN ('in', 'out')),
    header_message_id      text NOT NULL CHECK (length(header_message_id) BETWEEN 1 AND 998),
    in_reply_to            text,
    references_ids         text[] NOT NULL DEFAULT '{}',
    from_address           text NOT NULL,
    to_addresses           text[] NOT NULL DEFAULT '{}',
    cc_addresses           text[] NOT NULL DEFAULT '{}',
    subject                text NOT NULL DEFAULT '',
    full_text              text NOT NULL DEFAULT '',
    full_html              text,
    quoted                 boolean NOT NULL DEFAULT false,
    headers                jsonb NOT NULL DEFAULT '{}',
    raw_key                text UNIQUE,
    raw_size               bigint,
    authentication_results text,
    dmarc                  text NOT NULL DEFAULT 'unknown' CHECK (dmarc IN ('pass', 'fail', 'none', 'unknown')),
    auto                   boolean NOT NULL DEFAULT false,
    created_at             timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, message_id),
    FOREIGN KEY (workspace_id, message_id) REFERENCES messages (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, channel_id) REFERENCES channels (workspace_id, id) ON DELETE SET NULL (channel_id)
);
CREATE INDEX message_emails_header_idx ON message_emails (workspace_id, header_message_id);
CREATE UNIQUE INDEX message_emails_inbound_key ON message_emails (workspace_id, channel_id, header_message_id) WHERE direction = 'in';
CREATE INDEX message_emails_outbound_idx ON message_emails (header_message_id) WHERE direction = 'out';
CREATE INDEX message_emails_conversation_idx ON message_emails (workspace_id, conversation_id, created_at);

CREATE TABLE email_suppressions (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    email        text NOT NULL CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 320),
    reason       text NOT NULL CHECK (reason IN ('bounce', 'complaint')),
    detail       text NOT NULL DEFAULT '' CHECK (length(detail) <= 2000),
    created_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, email)
);

CREATE TABLE email_auto_replies (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    channel_id   uuid NOT NULL,
    contact_id   uuid NOT NULL,
    sent_at      timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, channel_id, contact_id),
    FOREIGN KEY (workspace_id, channel_id) REFERENCES channels (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE email_auto_replies;
DROP TABLE email_suppressions;
DROP TABLE message_emails;
ALTER TABLE messages DROP CONSTRAINT messages_delivery_check,
    DROP COLUMN delivery_updated_at, DROP COLUMN delivery_error, DROP COLUMN delivery_state;
DROP INDEX conversations_channel_contact_idx;
ALTER TABLE conversations DROP CONSTRAINT conversations_email_token_key, DROP COLUMN email_token, DROP COLUMN spam;
DROP TABLE email_channels;
