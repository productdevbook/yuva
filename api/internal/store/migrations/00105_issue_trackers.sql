-- +goose Up
CREATE TABLE issue_integrations (
    id                   uuid PRIMARY KEY,
    workspace_id         uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    provider             text NOT NULL CHECK (provider IN ('github', 'linear')),
    name                 text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    account              text NOT NULL CHECK (length(account) <= 200),
    api_url              text CHECK (length(api_url) BETWEEN 1 AND 500),
    default_target       text CHECK (length(default_target) BETWEEN 1 AND 200),
    token                bytea NOT NULL,
    webhook_secret       bytea,
    last_error           text CHECK (length(last_error) <= 1000),
    last_error_at        timestamptz,
    created_by_member_id uuid,
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    CHECK (api_url IS NULL OR provider = 'github')
);
CREATE INDEX issue_integrations_workspace_idx ON issue_integrations (workspace_id, created_at, id);

CREATE TABLE tracked_issues (
    id               uuid PRIMARY KEY,
    workspace_id     uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    integration_id   uuid NOT NULL,
    external_id      text NOT NULL CHECK (length(external_id) BETWEEN 1 AND 300),
    key              text NOT NULL CHECK (length(key) BETWEEN 1 AND 300),
    url              text NOT NULL CHECK (length(url) BETWEEN 1 AND 2000),
    title            text NOT NULL CHECK (length(title) <= 1000),
    state            text NOT NULL CHECK (length(state) <= 100),
    progress         text NOT NULL CHECK (progress IN ('planned', 'in_progress', 'done', 'wont_do')),
    state_changed_at timestamptz,
    synced_at        timestamptz NOT NULL,
    next_sync_at     timestamptz NOT NULL,
    sync_failures    integer NOT NULL DEFAULT 0,
    created_at       timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, integration_id, external_id),
    FOREIGN KEY (workspace_id, integration_id) REFERENCES issue_integrations (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX tracked_issues_next_sync_idx ON tracked_issues (next_sync_at);

CREATE TABLE issue_links (
    id                   uuid PRIMARY KEY,
    workspace_id         uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    issue_id             uuid NOT NULL,
    conversation_id      uuid,
    idea_id              uuid,
    linked_by_member_id  uuid,
    linked_by_api_key_id uuid,
    linked_by_grant_id   uuid,
    via                  text NOT NULL DEFAULT '',
    applied_progress     text CHECK (applied_progress IN ('planned', 'in_progress', 'done', 'wont_do')),
    created_at           timestamptz NOT NULL,
    FOREIGN KEY (workspace_id, issue_id) REFERENCES tracked_issues (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, idea_id) REFERENCES discussions (workspace_id, id) ON DELETE CASCADE,
    CHECK (num_nonnulls(conversation_id, idea_id) = 1)
);
CREATE UNIQUE INDEX issue_links_conversation_key ON issue_links (workspace_id, conversation_id, issue_id) WHERE conversation_id IS NOT NULL;
CREATE UNIQUE INDEX issue_links_idea_key ON issue_links (workspace_id, idea_id, issue_id) WHERE idea_id IS NOT NULL;
CREATE INDEX issue_links_issue_idx ON issue_links (workspace_id, issue_id, created_at, id);

-- +goose Down
DROP TABLE issue_links;
DROP TABLE tracked_issues;
DROP TABLE issue_integrations;
