-- +goose Up
CREATE TABLE idempotency_keys (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    caller_type  text NOT NULL CHECK (caller_type IN ('api_key', 'member', 'contact')),
    caller_id    uuid NOT NULL,
    key          text NOT NULL CHECK (length(key) BETWEEN 1 AND 255),
    method       text NOT NULL,
    path         text NOT NULL,
    body_sha256  bytea NOT NULL,
    status       integer,
    headers      jsonb,
    body         bytea,
    created_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, caller_type, caller_id, key)
);
CREATE INDEX idempotency_keys_created_idx ON idempotency_keys (workspace_id, created_at);

-- +goose Down
DROP TABLE idempotency_keys;
