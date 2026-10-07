-- +goose Up
ALTER TABLE members
    ADD COLUMN notification_events      jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN notification_email_delay integer CHECK (notification_email_delay BETWEEN 5 AND 1440);

CREATE TABLE inbox_notifications (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    member_id    uuid NOT NULL,
    inbox_id     uuid NOT NULL,
    events       jsonb NOT NULL,
    updated_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, member_id, inbox_id),
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE
);

ALTER TABLE realtime_connections ADD COLUMN viewing_conversation_id uuid;
CREATE INDEX realtime_connections_viewing_idx ON realtime_connections (workspace_id, viewing_conversation_id)
    WHERE viewing_conversation_id IS NOT NULL;

CREATE TABLE push_subscriptions (
    id              uuid PRIMARY KEY,
    person_id       uuid NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    session_id      uuid NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    endpoint        text NOT NULL UNIQUE CHECK (length(endpoint) BETWEEN 8 AND 2000),
    p256dh          text NOT NULL CHECK (length(p256dh) <= 200),
    auth            text NOT NULL CHECK (length(auth) <= 100),
    user_agent      text NOT NULL DEFAULT '' CHECK (length(user_agent) <= 200),
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    last_success_at timestamptz,
    last_failure_at timestamptz,
    last_error      text CHECK (length(last_error) <= 1000)
);
CREATE INDEX push_subscriptions_person_idx ON push_subscriptions (person_id, created_at DESC);
CREATE INDEX push_subscriptions_session_idx ON push_subscriptions (session_id);

CREATE TABLE notification_emails (
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    member_id       uuid NOT NULL,
    conversation_id uuid NOT NULL,
    sent_at         timestamptz NOT NULL,
    through         timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, member_id, conversation_id),
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE notification_emails;
DROP TABLE push_subscriptions;
DROP INDEX realtime_connections_viewing_idx;
ALTER TABLE realtime_connections DROP COLUMN viewing_conversation_id;
DROP TABLE inbox_notifications;
ALTER TABLE members DROP COLUMN notification_email_delay, DROP COLUMN notification_events;
