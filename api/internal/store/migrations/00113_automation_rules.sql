-- +goose Up
CREATE TABLE automation_rules (
    id                  uuid PRIMARY KEY,
    workspace_id        uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    inbox_id            uuid,
    name                text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    enabled             boolean NOT NULL DEFAULT true,
    position            integer NOT NULL,
    trigger             text NOT NULL CHECK (trigger IN ('conversation_created', 'message_received', 'status_changed', 'idle')),
    idle_minutes        integer CHECK (idle_minutes BETWEEN 5 AND 43200),
    conditions          jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(conditions) = 'array'),
    actions             jsonb NOT NULL CHECK (jsonb_typeof(actions) = 'array'),
    saved_by_kind       text NOT NULL CHECK (saved_by_kind IN ('member', 'api_key', 'oauth_grant')),
    saved_by_member_id  uuid,
    saved_by_api_key_id uuid,
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE,
    CHECK ((trigger = 'idle') = (idle_minutes IS NOT NULL))
);
CREATE INDEX automation_rules_trigger_idx ON automation_rules (workspace_id, trigger, position) WHERE enabled;

CREATE TABLE automation_runs (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    rule_id         uuid NOT NULL,
    conversation_id uuid NOT NULL,
    trigger         text NOT NULL,
    trigger_key     text NOT NULL,
    outcome         text NOT NULL CHECK (outcome IN ('applied', 'failed', 'not_matched')),
    error           text,
    detail          text,
    actions         jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(actions) = 'array'),
    created_at      timestamptz NOT NULL,
    UNIQUE (workspace_id, rule_id, trigger_key),
    FOREIGN KEY (workspace_id, rule_id) REFERENCES automation_rules (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX automation_runs_log_idx ON automation_runs (workspace_id, rule_id, created_at DESC, id DESC);
CREATE INDEX automation_runs_conversation_idx ON automation_runs (workspace_id, conversation_id);
CREATE INDEX automation_runs_created_idx ON automation_runs (workspace_id, created_at);

ALTER TABLE messages ADD COLUMN rule jsonb;

-- The rule whose action set the current snooze; its wake-up triggers no rule.
ALTER TABLE conversations ADD COLUMN snooze_rule_id uuid;

ALTER TABLE canned_replies
    ADD COLUMN saved_by_kind text CHECK (saved_by_kind IN ('member', 'api_key', 'oauth_grant')),
    ADD COLUMN saved_by_member_id uuid,
    ADD COLUMN saved_by_api_key_id uuid;

-- +goose Down
ALTER TABLE canned_replies DROP COLUMN saved_by_kind, DROP COLUMN saved_by_member_id, DROP COLUMN saved_by_api_key_id;
ALTER TABLE conversations DROP COLUMN snooze_rule_id;
ALTER TABLE messages DROP COLUMN rule;
DROP TABLE automation_runs;
DROP TABLE automation_rules;
