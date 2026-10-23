-- +goose Up
CREATE TABLE sla_policies (
    id                  uuid PRIMARY KEY,
    workspace_id        uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name                text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    position            integer NOT NULL,
    enabled             boolean NOT NULL DEFAULT true,
    inbox_ids           uuid[] NOT NULL DEFAULT '{}',
    channel_kinds       text[] NOT NULL DEFAULT '{}',
    label_ids           uuid[] NOT NULL DEFAULT '{}',
    priorities          text[] NOT NULL DEFAULT '{}',
    first_reply_minutes integer CHECK (first_reply_minutes BETWEEN 1 AND 525600),
    next_reply_minutes  integer CHECK (next_reply_minutes BETWEEN 1 AND 525600),
    resolution_minutes  integer CHECK (resolution_minutes BETWEEN 1 AND 525600),
    warn_minutes        integer NOT NULL DEFAULT 15 CHECK (warn_minutes BETWEEN 0 AND 1440),
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    CHECK (first_reply_minutes IS NOT NULL OR next_reply_minutes IS NOT NULL OR resolution_minutes IS NOT NULL)
);
CREATE INDEX sla_policies_order_idx ON sla_policies (workspace_id, position);

-- One row per conversation a policy applies to. For each target: target_s is the business seconds in
-- force (NULL when the policy does not track it); started_at and met_at are kept even when it is
-- not tracked, so the reply and close history stays known; due_at is set while it runs and
-- remaining_s while it is paused.
CREATE TABLE conversation_sla (
    workspace_id             uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    conversation_id          uuid NOT NULL,
    policy_id                uuid NOT NULL,
    match_key                text NOT NULL,
    warn_minutes             integer NOT NULL,
    awaiting_reply           boolean NOT NULL DEFAULT false,
    paused_at                timestamptz,
    check_at                 timestamptz,
    first_reply_target_s     integer,
    first_reply_started_at   timestamptz,
    first_reply_due_at       timestamptz,
    first_reply_remaining_s  integer,
    first_reply_met_at       timestamptz,
    first_reply_breached_at  timestamptz,
    first_reply_warned_at    timestamptz,
    next_reply_target_s      integer,
    next_reply_started_at    timestamptz,
    next_reply_due_at        timestamptz,
    next_reply_remaining_s   integer,
    next_reply_met_at        timestamptz,
    next_reply_breached_at   timestamptz,
    next_reply_warned_at     timestamptz,
    resolution_target_s      integer,
    resolution_started_at    timestamptz,
    resolution_due_at        timestamptz,
    resolution_remaining_s   integer,
    resolution_met_at        timestamptz,
    resolution_breached_at   timestamptz,
    resolution_warned_at     timestamptz,
    created_at               timestamptz NOT NULL,
    updated_at               timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, conversation_id),
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, policy_id) REFERENCES sla_policies (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX conversation_sla_check_idx ON conversation_sla (workspace_id, check_at) WHERE check_at IS NOT NULL;
CREATE INDEX conversation_sla_policy_idx ON conversation_sla (workspace_id, policy_id);

-- Each met or breached target once, with the inbox, assignee and team it had then; /v1/stats and
-- reports count these, since a conversation's next-reply targets replace one another.
CREATE TABLE sla_outcomes (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    conversation_id uuid NOT NULL,
    inbox_id        uuid NOT NULL,
    policy_id       uuid,
    target          text NOT NULL CHECK (target IN ('first_reply', 'next_reply', 'resolution')),
    outcome         text NOT NULL CHECK (outcome IN ('met', 'breached')),
    at              timestamptz NOT NULL,
    assignee_id     uuid,
    team_id         uuid,
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, policy_id) REFERENCES sla_policies (workspace_id, id) ON DELETE SET NULL (policy_id),
    FOREIGN KEY (workspace_id, assignee_id) REFERENCES members (workspace_id, id) ON DELETE SET NULL (assignee_id),
    FOREIGN KEY (workspace_id, team_id) REFERENCES teams (workspace_id, id) ON DELETE SET NULL (team_id)
);
CREATE INDEX sla_outcomes_at_idx ON sla_outcomes (workspace_id, at);
CREATE INDEX sla_outcomes_conversation_idx ON sla_outcomes (workspace_id, conversation_id);

-- Indexed by 00110 outside a transaction.
ALTER TABLE conversations ADD COLUMN sla_due_at timestamptz;

-- +goose Down
ALTER TABLE conversations DROP COLUMN sla_due_at;
DROP TABLE sla_outcomes;
DROP TABLE conversation_sla;
DROP TABLE sla_policies;
