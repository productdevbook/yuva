-- +goose Up
-- optout_key is the key a workspace's email_optouts hashes are made with, sealed under the master
-- key: the hashes keep no address, so they cannot be made again under a new master key, and a
-- master key rotation re-seals this key instead.
CREATE TABLE workspace_keys (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE CASCADE,
    optout_key   bytea NOT NULL,
    created_at   timestamptz NOT NULL
);

-- +goose Down
DROP TABLE workspace_keys;
