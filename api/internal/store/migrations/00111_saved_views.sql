-- +goose Up
CREATE TABLE saved_views (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    visibility      text NOT NULL CHECK (visibility IN ('personal', 'workspace', 'team')),
    owner_member_id uuid,
    team_id         uuid,
    name            text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    filters         jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(filters) = 'object'),
    position        integer NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    CHECK ((visibility = 'personal') = (owner_member_id IS NOT NULL)),
    CHECK ((visibility = 'team') = (team_id IS NOT NULL)),
    FOREIGN KEY (workspace_id, owner_member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, team_id) REFERENCES teams (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX saved_views_owner_idx ON saved_views (workspace_id, owner_member_id) WHERE owner_member_id IS NOT NULL;
CREATE INDEX saved_views_team_idx ON saved_views (workspace_id, team_id) WHERE team_id IS NOT NULL;

-- +goose Down
DROP TABLE saved_views;
