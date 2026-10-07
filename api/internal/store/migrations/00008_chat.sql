-- +goose Up
CREATE TABLE chat_channels (
    workspace_id      uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    channel_id        uuid NOT NULL,
    public_key        text NOT NULL CHECK (length(public_key) BETWEEN 16 AND 200),
    allowed_origins   text[] NOT NULL CHECK (cardinality(allowed_origins) BETWEEN 1 AND 20),
    allow_anonymous   boolean NOT NULL DEFAULT false,
    ask_email_offline boolean NOT NULL DEFAULT true,
    greeting          text NOT NULL DEFAULT '' CHECK (length(greeting) <= 500),
    launcher_position text CHECK (launcher_position IN ('right', 'left')),
    launcher_color    text CHECK (launcher_color ~ '^#[0-9a-fA-F]{6}$'),
    PRIMARY KEY (workspace_id, channel_id),
    FOREIGN KEY (workspace_id, channel_id) REFERENCES channels (workspace_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX chat_channels_public_key_key ON chat_channels (public_key);
CREATE INDEX chat_channels_origins_idx ON chat_channels USING gin (allowed_origins);

ALTER TABLE contacts
    ADD COLUMN locale      text CHECK (length(locale) BETWEEN 2 AND 35),
    ADD COLUMN typed_email text CHECK (typed_email = lower(typed_email) AND length(typed_email) BETWEEN 3 AND 320);

ALTER TABLE people ADD COLUMN availability text NOT NULL DEFAULT 'auto' CHECK (availability IN ('auto', 'away'));

CREATE TABLE chat_visitors (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    inbox_id     uuid NOT NULL,
    visitor_hash bytea NOT NULL,
    contact_id   uuid NOT NULL,
    created_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, inbox_id, visitor_hash),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX chat_visitors_contact_idx ON chat_visitors (workspace_id, contact_id);

CREATE TABLE contact_sessions (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    channel_id   uuid NOT NULL,
    inbox_id     uuid NOT NULL,
    contact_id   uuid NOT NULL,
    token_hash   bytea NOT NULL UNIQUE,
    identified   boolean NOT NULL,
    created_at   timestamptz NOT NULL,
    expires_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    FOREIGN KEY (workspace_id, channel_id) REFERENCES channels (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX contact_sessions_contact_idx ON contact_sessions (workspace_id, contact_id);
CREATE INDEX contact_sessions_expires_idx ON contact_sessions (expires_at);

CREATE TABLE contact_reads (
    workspace_id         uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    conversation_id      uuid NOT NULL,
    last_read_message_id uuid NOT NULL,
    last_read_at         timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, conversation_id),
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, last_read_message_id) REFERENCES messages (workspace_id, id) ON DELETE CASCADE
);

CREATE TABLE realtime_connections (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    member_id    uuid,
    contact_id   uuid,
    seen_at      timestamptz NOT NULL,
    CHECK ((member_id IS NULL) <> (contact_id IS NULL)),
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX realtime_connections_member_idx ON realtime_connections (workspace_id, member_id) WHERE member_id IS NOT NULL;
CREATE INDEX realtime_connections_contact_idx ON realtime_connections (workspace_id, contact_id) WHERE contact_id IS NOT NULL;

ALTER TABLE conversations
    ADD COLUMN continuity_through timestamptz,
    ADD COLUMN continuity_sent_at timestamptz;

-- +goose Down
ALTER TABLE conversations DROP COLUMN continuity_sent_at, DROP COLUMN continuity_through;
DROP TABLE realtime_connections;
DROP TABLE contact_reads;
DROP TABLE contact_sessions;
DROP TABLE chat_visitors;
ALTER TABLE people DROP COLUMN availability;
ALTER TABLE contacts DROP COLUMN typed_email, DROP COLUMN locale;
DROP TABLE chat_channels;
