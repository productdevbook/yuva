-- +goose Up
CREATE TABLE slack_installations (
    workspace_id  uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE CASCADE,
    team_id       text NOT NULL CHECK (team_id ~ '^[A-Z0-9]{1,40}$'),
    team_name     text NOT NULL CHECK (length(team_name) <= 200),
    app_id        text NOT NULL CHECK (length(app_id) BETWEEN 1 AND 40),
    bot_user_id   text NOT NULL CHECK (bot_user_id ~ '^[UW][A-Z0-9]{1,40}$'),
    bot_token     bytea NOT NULL,
    scopes        text[] NOT NULL,
    reply_command text NOT NULL DEFAULT '!reply' CHECK (reply_command ~ '^![a-z]{1,20}$'),
    connected_by  uuid,
    last_error    text CHECK (length(last_error) <= 500),
    last_error_at timestamptz,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    FOREIGN KEY (workspace_id, connected_by) REFERENCES members (workspace_id, id) ON DELETE SET NULL (connected_by)
);
CREATE UNIQUE INDEX slack_installations_team_key ON slack_installations (team_id);

CREATE TABLE slack_connect_states (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE CASCADE,
    member_id    uuid NOT NULL,
    state_hash   text NOT NULL,
    expires_at   timestamptz NOT NULL,
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE
);

CREATE TABLE slack_inbox_settings (
    workspace_id              uuid NOT NULL,
    inbox_id                  uuid NOT NULL,
    new_conversations_channel text CHECK (new_conversations_channel ~ '^[CG][A-Z0-9]{1,40}$'),
    mentions_channel          text CHECK (mentions_channel ~ '^[CG][A-Z0-9]{1,40}$'),
    include_content           boolean NOT NULL DEFAULT false,
    updated_at                timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, inbox_id),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE
);

CREATE TABLE slack_threads (
    workspace_id    uuid NOT NULL,
    channel_id      text NOT NULL,
    thread_ts       text NOT NULL CHECK (length(thread_ts) <= 40),
    conversation_id uuid NOT NULL,
    created_at      timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, channel_id, thread_ts),
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX slack_threads_conversation_idx ON slack_threads (workspace_id, conversation_id, channel_id);

CREATE TABLE slack_member_links (
    workspace_id    uuid NOT NULL,
    member_id       uuid NOT NULL,
    slack_user_id   text NOT NULL CHECK (slack_user_id ~ '^[UW][A-Z0-9]{1,40}$'),
    slack_user_name text NOT NULL CHECK (length(slack_user_name) <= 200),
    linked_at       timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, member_id),
    UNIQUE (workspace_id, slack_user_id),
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE
);

CREATE TABLE slack_link_codes (
    workspace_id    uuid NOT NULL,
    member_id       uuid NOT NULL,
    slack_user_id   text NOT NULL CHECK (slack_user_id ~ '^[UW][A-Z0-9]{1,40}$'),
    slack_user_name text NOT NULL CHECK (length(slack_user_name) <= 200),
    code_hash       bytea NOT NULL,
    attempts        integer NOT NULL DEFAULT 0,
    expires_at      timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, member_id),
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE
);

CREATE TABLE slack_events (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    event_id     text NOT NULL CHECK (length(event_id) BETWEEN 1 AND 100),
    received_at  timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, event_id)
);
CREATE INDEX slack_events_received_idx ON slack_events (workspace_id, received_at);

-- +goose Down
DROP TABLE slack_events;
DROP TABLE slack_link_codes;
DROP TABLE slack_member_links;
DROP TABLE slack_threads;
DROP TABLE slack_inbox_settings;
DROP TABLE slack_connect_states;
DROP TABLE slack_installations;
