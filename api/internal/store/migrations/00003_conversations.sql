-- +goose Up
ALTER TABLE members ADD CONSTRAINT members_workspace_id_id_key UNIQUE (workspace_id, id);

CREATE TABLE inboxes (
    id                     uuid PRIMARY KEY,
    workspace_id           uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name                   text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    slug                   text NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug) <= 64),
    branding               jsonb NOT NULL DEFAULT '{}',
    default_locale         text NOT NULL DEFAULT 'en',
    timezone               text NOT NULL DEFAULT 'UTC',
    mode                   text NOT NULL DEFAULT 'async' CHECK (mode IN ('live', 'async')),
    expected_reply_minutes integer CHECK (expected_reply_minutes BETWEEN 1 AND 43200),
    business_hours         jsonb NOT NULL DEFAULT '{"enabled": false, "intervals": []}',
    identity_secret        bytea NOT NULL,
    created_at             timestamptz NOT NULL,
    updated_at             timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, slug)
);

CREATE TABLE inbox_members (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    inbox_id     uuid NOT NULL,
    member_id    uuid NOT NULL,
    created_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, inbox_id, member_id),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX inbox_members_member_idx ON inbox_members (workspace_id, member_id);

CREATE TABLE channels (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    inbox_id     uuid NOT NULL,
    kind         text NOT NULL CHECK (kind IN ('email', 'chat', 'app', 'api')),
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    settings     jsonb NOT NULL DEFAULT '{}',
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX channels_inbox_idx ON channels (workspace_id, inbox_id);

CREATE TABLE contacts (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name         text NOT NULL DEFAULT '' CHECK (length(name) <= 200),
    attributes   jsonb NOT NULL DEFAULT '{}',
    blocked      boolean NOT NULL DEFAULT false,
    search       tsvector NOT NULL DEFAULT ''::tsvector,
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL,
    UNIQUE (workspace_id, id)
);
CREATE INDEX contacts_list_idx ON contacts (workspace_id, created_at DESC, id DESC);
CREATE INDEX contacts_search_idx ON contacts USING gin (search);

CREATE TABLE contact_emails (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    contact_id   uuid NOT NULL,
    email        text NOT NULL CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 320),
    position     integer NOT NULL,
    PRIMARY KEY (workspace_id, email),
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX contact_emails_contact_idx ON contact_emails (workspace_id, contact_id);

CREATE TABLE contact_external_ids (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    inbox_id     uuid NOT NULL,
    external_id  text NOT NULL CHECK (length(external_id) BETWEEN 1 AND 200),
    contact_id   uuid NOT NULL,
    PRIMARY KEY (workspace_id, inbox_id, external_id),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX contact_external_ids_contact_idx ON contact_external_ids (workspace_id, contact_id);

CREATE TABLE labels (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
    color        text NOT NULL CHECK (color ~ '^#[0-9a-f]{6}$'),
    created_at   timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, name)
);

CREATE TABLE canned_replies (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    shortcut     text NOT NULL CHECK (shortcut ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(shortcut) <= 64),
    title        text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    body         text NOT NULL CHECK (length(body) BETWEEN 1 AND 65536),
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL,
    UNIQUE (workspace_id, shortcut)
);

CREATE TABLE conversations (
    id               uuid PRIMARY KEY,
    workspace_id     uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    inbox_id         uuid NOT NULL,
    contact_id       uuid NOT NULL,
    channel_id       uuid,
    subject          text NOT NULL DEFAULT '' CHECK (length(subject) <= 500),
    status           text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'pending', 'snoozed', 'closed')),
    snooze_until     timestamptz,
    priority         text NOT NULL DEFAULT 'normal' CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    assignee_id      uuid,
    last_message_at  timestamptz,
    last_activity_at timestamptz NOT NULL,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    CHECK ((status = 'snoozed') = (snooze_until IS NOT NULL)),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, channel_id) REFERENCES channels (workspace_id, id) ON DELETE SET NULL (channel_id),
    FOREIGN KEY (workspace_id, assignee_id) REFERENCES members (workspace_id, id) ON DELETE SET NULL (assignee_id)
);
CREATE INDEX conversations_activity_idx ON conversations (workspace_id, last_activity_at DESC, id DESC);
CREATE INDEX conversations_inbox_idx ON conversations (workspace_id, inbox_id, last_activity_at DESC, id DESC);
CREATE INDEX conversations_assignee_idx ON conversations (workspace_id, assignee_id);
CREATE INDEX conversations_contact_idx ON conversations (workspace_id, contact_id);

CREATE TABLE conversation_labels (
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    conversation_id uuid NOT NULL,
    label_id        uuid NOT NULL,
    PRIMARY KEY (workspace_id, conversation_id, label_id),
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, label_id) REFERENCES labels (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX conversation_labels_label_idx ON conversation_labels (workspace_id, label_id);

CREATE TABLE messages (
    id                uuid PRIMARY KEY,
    workspace_id      uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    conversation_id   uuid NOT NULL,
    kind              text NOT NULL CHECK (kind IN ('message', 'note', 'event')),
    direction         text CHECK (direction IN ('in', 'out')),
    author_type       text NOT NULL CHECK (author_type IN ('contact', 'member', 'system')),
    author_member_id  uuid,
    author_contact_id uuid,
    body              text NOT NULL DEFAULT '',
    html              text,
    client_id         text CHECK (length(client_id) BETWEEN 1 AND 200),
    event             jsonb,
    search            tsvector NOT NULL GENERATED ALWAYS AS (to_tsvector('simple', translate(body, 'İı', 'ii'))) STORED,
    created_at        timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, conversation_id, client_id),
    CHECK ((kind = 'message') = (direction IS NOT NULL)),
    CHECK ((kind = 'event') = (event IS NOT NULL)),
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, author_member_id) REFERENCES members (workspace_id, id) ON DELETE SET NULL (author_member_id),
    FOREIGN KEY (workspace_id, author_contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX messages_conversation_idx ON messages (workspace_id, conversation_id, created_at, id);
CREATE INDEX messages_search_idx ON messages USING gin (search);

CREATE TABLE attachments (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    conversation_id uuid NOT NULL,
    message_id      uuid NOT NULL,
    storage_key     text NOT NULL UNIQUE,
    filename        text NOT NULL CHECK (length(filename) BETWEEN 1 AND 255),
    content_type    text NOT NULL,
    size_bytes      bigint NOT NULL CHECK (size_bytes >= 0),
    created_at      timestamptz NOT NULL,
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, message_id) REFERENCES messages (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX attachments_message_idx ON attachments (workspace_id, message_id);
CREATE INDEX attachments_conversation_idx ON attachments (workspace_id, conversation_id);

CREATE TABLE usage_counters (
    workspace_id     uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    month            date NOT NULL CHECK (extract(day FROM month) = 1),
    conversations    bigint NOT NULL DEFAULT 0,
    messages         bigint NOT NULL DEFAULT 0,
    attachment_bytes bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (workspace_id, month)
);

-- +goose Down
DROP TABLE usage_counters;
DROP TABLE attachments;
DROP TABLE messages;
DROP TABLE conversation_labels;
DROP TABLE conversations;
DROP TABLE canned_replies;
DROP TABLE labels;
DROP TABLE contact_external_ids;
DROP TABLE contact_emails;
DROP TABLE contacts;
DROP TABLE channels;
DROP TABLE inbox_members;
DROP TABLE inboxes;
ALTER TABLE members DROP CONSTRAINT members_workspace_id_id_key;
