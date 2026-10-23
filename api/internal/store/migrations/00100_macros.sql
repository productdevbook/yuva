-- +goose Up
CREATE TABLE macros (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    owner_member_id uuid,
    name            text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    shortcut        text NOT NULL CHECK (shortcut ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(shortcut) <= 64),
    body            text CHECK (length(body) BETWEEN 1 AND 65536),
    actions         jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(actions) = 'object'),
    position        integer NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, owner_member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX macros_shared_shortcut_key ON macros (workspace_id, shortcut) WHERE owner_member_id IS NULL;
CREATE UNIQUE INDEX macros_personal_shortcut_key ON macros (workspace_id, owner_member_id, shortcut) WHERE owner_member_id IS NOT NULL;

-- +goose Down
DROP TABLE macros;
