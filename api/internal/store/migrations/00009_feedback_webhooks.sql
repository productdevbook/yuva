-- +goose Up
-- chat_channels holds the client settings of both `chat` and `app` channels; app channels have
-- no origins.
ALTER TABLE chat_channels
    DROP CONSTRAINT chat_channels_allowed_origins_check,
    ADD CONSTRAINT chat_channels_allowed_origins_check CHECK (cardinality(allowed_origins) <= 20),
    ADD COLUMN platforms text[] NOT NULL DEFAULT '{}'
        CHECK (platforms <@ ARRAY['ios', 'android']::text[]);

ALTER TABLE conversations
    ADD COLUMN kind     text NOT NULL DEFAULT 'conversation' CHECK (kind IN ('conversation', 'feedback')),
    ADD COLUMN feedback jsonb,
    ADD CONSTRAINT conversations_feedback_check CHECK ((kind = 'feedback') = (feedback IS NOT NULL));
CREATE INDEX conversations_feedback_idx ON conversations (workspace_id, last_activity_at DESC, id DESC) WHERE kind = 'feedback';

CREATE TABLE webhook_endpoints (
    id                    uuid PRIMARY KEY,
    workspace_id          uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    inbox_id              uuid,
    url                   text NOT NULL CHECK (length(url) BETWEEN 8 AND 2000),
    description           text NOT NULL DEFAULT '' CHECK (length(description) <= 200),
    events                text[] NOT NULL CHECK (cardinality(events) BETWEEN 1 AND 20),
    include_notes         boolean NOT NULL DEFAULT false,
    enabled               boolean NOT NULL DEFAULT true,
    secret                bytea NOT NULL,
    previous_secret       bytea,
    previous_secret_until timestamptz,
    failing_since         timestamptz,
    disabled_at           timestamptz,
    disabled_reason       text CHECK (length(disabled_reason) <= 1000),
    created_at            timestamptz NOT NULL,
    updated_at            timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX webhook_endpoints_workspace_idx ON webhook_endpoints (workspace_id, created_at, id);

CREATE TABLE webhook_deliveries (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    endpoint_id     uuid NOT NULL,
    message_id      text NOT NULL CHECK (length(message_id) BETWEEN 1 AND 100),
    event_type      text NOT NULL,
    payload         text NOT NULL,
    state           text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'succeeded', 'failed')),
    attempts        integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz,
    last_attempt_at timestamptz,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, endpoint_id) REFERENCES webhook_endpoints (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX webhook_deliveries_endpoint_idx ON webhook_deliveries (workspace_id, endpoint_id, created_at DESC, id DESC);
CREATE INDEX webhook_deliveries_created_idx ON webhook_deliveries (created_at);

CREATE TABLE webhook_attempts (
    id            uuid PRIMARY KEY,
    workspace_id  uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    endpoint_id   uuid NOT NULL,
    delivery_id   uuid NOT NULL,
    attempted_at  timestamptz NOT NULL,
    manual        boolean NOT NULL DEFAULT false,
    success       boolean NOT NULL,
    status_code   integer,
    latency_ms    integer NOT NULL,
    response_body text NOT NULL DEFAULT '',
    error         text,
    FOREIGN KEY (workspace_id, endpoint_id) REFERENCES webhook_endpoints (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, delivery_id) REFERENCES webhook_deliveries (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX webhook_attempts_endpoint_idx ON webhook_attempts (workspace_id, endpoint_id, attempted_at DESC, id DESC);
CREATE INDEX webhook_attempts_delivery_idx ON webhook_attempts (workspace_id, delivery_id, attempted_at DESC, id DESC);

-- +goose Down
DROP TABLE webhook_attempts;
DROP TABLE webhook_deliveries;
DROP TABLE webhook_endpoints;
DROP INDEX conversations_feedback_idx;
ALTER TABLE conversations DROP CONSTRAINT conversations_feedback_check, DROP COLUMN feedback, DROP COLUMN kind;
ALTER TABLE chat_channels
    DROP COLUMN platforms,
    DROP CONSTRAINT chat_channels_allowed_origins_check,
    ADD CONSTRAINT chat_channels_allowed_origins_check CHECK (cardinality(allowed_origins) BETWEEN 1 AND 20);
