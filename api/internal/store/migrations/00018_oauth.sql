-- +goose Up
-- Clients and pending authorization requests belong to no workspace: a client is used by many,
-- and a request exists before the person picks a workspace (docs/architecture.md, OAuth and MCP).
CREATE TABLE oauth_clients (
    id            uuid PRIMARY KEY,
    client_id     text NOT NULL UNIQUE CHECK (length(client_id) BETWEEN 1 AND 2000),
    kind          text NOT NULL CHECK (kind IN ('registered', 'metadata')),
    name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    client_uri    text CHECK (length(client_uri) BETWEEN 1 AND 2000),
    redirect_uris text[] NOT NULL,
    created_at    timestamptz NOT NULL,
    fetched_at    timestamptz
);

CREATE TABLE oauth_requests (
    id             uuid PRIMARY KEY,
    client_id      uuid NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
    redirect_uri   text NOT NULL,
    state          text,
    code_challenge text NOT NULL,
    scopes         text[],
    resource       text NOT NULL,
    created_at     timestamptz NOT NULL,
    expires_at     timestamptz NOT NULL
);
CREATE INDEX oauth_requests_expires_idx ON oauth_requests (expires_at);

CREATE TABLE oauth_grants (
    workspace_id  uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    id            uuid NOT NULL,
    member_id     uuid NOT NULL,
    client_id     uuid NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
    scopes        text[] NOT NULL,
    resource      text NOT NULL,
    created_at    timestamptz NOT NULL,
    last_used_at  timestamptz,
    request_month date,
    request_count bigint NOT NULL DEFAULT 0,
    revoked_at    timestamptz,
    PRIMARY KEY (workspace_id, id),
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX oauth_grants_active_idx ON oauth_grants (workspace_id, member_id, client_id) WHERE revoked_at IS NULL;
CREATE INDEX oauth_grants_client_idx ON oauth_grants (client_id);

CREATE TABLE oauth_codes (
    workspace_id   uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    id             uuid NOT NULL,
    grant_id       uuid NOT NULL,
    code_hash      bytea NOT NULL UNIQUE,
    redirect_uri   text NOT NULL,
    code_challenge text NOT NULL,
    created_at     timestamptz NOT NULL,
    expires_at     timestamptz NOT NULL,
    used_at        timestamptz,
    PRIMARY KEY (workspace_id, id),
    FOREIGN KEY (workspace_id, grant_id) REFERENCES oauth_grants (workspace_id, id) ON DELETE CASCADE
);

CREATE TABLE oauth_tokens (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    id           uuid NOT NULL,
    grant_id     uuid NOT NULL,
    kind         text NOT NULL CHECK (kind IN ('access', 'refresh')),
    token_hash   bytea NOT NULL UNIQUE,
    created_at   timestamptz NOT NULL,
    expires_at   timestamptz NOT NULL,
    used_at      timestamptz,
    PRIMARY KEY (workspace_id, id),
    FOREIGN KEY (workspace_id, grant_id) REFERENCES oauth_grants (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX oauth_tokens_grant_idx ON oauth_tokens (workspace_id, grant_id);
CREATE INDEX oauth_tokens_expires_idx ON oauth_tokens (workspace_id, expires_at);

ALTER TABLE messages
    ADD COLUMN via      text CHECK (length(via) BETWEEN 1 AND 200),
    ADD COLUMN sent_via text CHECK (length(sent_via) BETWEEN 1 AND 200);

ALTER TABLE idempotency_keys DROP CONSTRAINT idempotency_keys_caller_type_check;
ALTER TABLE idempotency_keys ADD CONSTRAINT idempotency_keys_caller_type_check
    CHECK (caller_type IN ('api_key', 'member', 'contact', 'oauth_grant'));

-- +goose Down
DELETE FROM idempotency_keys WHERE caller_type = 'oauth_grant';
ALTER TABLE idempotency_keys DROP CONSTRAINT idempotency_keys_caller_type_check;
ALTER TABLE idempotency_keys ADD CONSTRAINT idempotency_keys_caller_type_check
    CHECK (caller_type IN ('api_key', 'member', 'contact'));
ALTER TABLE messages DROP COLUMN sent_via, DROP COLUMN via;
DROP TABLE oauth_tokens;
DROP TABLE oauth_codes;
DROP TABLE oauth_grants;
DROP TABLE oauth_requests;
DROP TABLE oauth_clients;
