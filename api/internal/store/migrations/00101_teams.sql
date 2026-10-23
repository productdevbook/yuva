-- +goose Up
CREATE TABLE teams (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name            text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
    description     text NOT NULL DEFAULT '' CHECK (length(description) <= 500),
    assignment_mode text NOT NULL DEFAULT 'off' CHECK (assignment_mode IN ('off', 'round_robin', 'least_open')),
    assignment_cap  integer CHECK (assignment_cap BETWEEN 1 AND 500),
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, name)
);

CREATE TABLE team_members (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    team_id      uuid NOT NULL,
    member_id    uuid NOT NULL,
    created_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, team_id, member_id),
    FOREIGN KEY (workspace_id, team_id) REFERENCES teams (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX team_members_member_idx ON team_members (workspace_id, member_id);

CREATE TABLE team_inboxes (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    team_id      uuid NOT NULL,
    inbox_id     uuid NOT NULL,
    created_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, team_id, inbox_id),
    FOREIGN KEY (workspace_id, team_id) REFERENCES teams (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX team_inboxes_inbox_idx ON team_inboxes (workspace_id, inbox_id);

-- Validated and indexed by 00102 outside a transaction, so adding it does not scan conversations.
ALTER TABLE conversations
    ADD COLUMN team_id uuid,
    ADD CONSTRAINT conversations_team_fkey FOREIGN KEY (workspace_id, team_id)
        REFERENCES teams (workspace_id, id) ON DELETE SET NULL (team_id) NOT VALID;

-- A member's access is the union of their own grants (team_id NULL) and their teams' grants; a
-- member may be listed more than once, so readers that return rows use DISTINCT.
CREATE VIEW member_inbox_access AS
SELECT workspace_id, inbox_id, member_id, NULL::uuid AS team_id FROM inbox_members
UNION ALL
SELECT ti.workspace_id, ti.inbox_id, tm.member_id, ti.team_id
FROM team_inboxes ti
JOIN team_members tm ON tm.workspace_id = ti.workspace_id AND tm.team_id = ti.team_id;

CREATE OR REPLACE VIEW inbox_viewers AS
SELECT workspace_id, inbox_id, member_id AS viewer_id FROM member_inbox_access
UNION ALL
SELECT workspace_id, inbox_id, api_key_id AS viewer_id FROM api_key_inboxes;

-- +goose Down
CREATE OR REPLACE VIEW inbox_viewers AS
SELECT workspace_id, inbox_id, member_id AS viewer_id FROM inbox_members
UNION ALL
SELECT workspace_id, inbox_id, api_key_id AS viewer_id FROM api_key_inboxes;
DROP VIEW member_inbox_access;
ALTER TABLE conversations DROP CONSTRAINT conversations_team_fkey, DROP COLUMN team_id;
DROP TABLE team_inboxes;
DROP TABLE team_members;
DROP TABLE teams;
