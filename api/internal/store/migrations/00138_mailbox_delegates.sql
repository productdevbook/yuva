-- +goose Up
CREATE TABLE mailbox_delegates (
    workspace_id uuid        NOT NULL,
    inbox_id     uuid        NOT NULL,
    member_id    uuid        NOT NULL,
    access       text        NOT NULL CHECK (access IN ('read', 'send')),
    granted_by   uuid,
    created_at   timestamptz NOT NULL,
    PRIMARY KEY (inbox_id, member_id),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, granted_by) REFERENCES members (workspace_id, id) ON DELETE SET NULL (granted_by)
);
CREATE INDEX mailbox_delegates_member_idx ON mailbox_delegates (workspace_id, member_id);

ALTER TABLE messages
    ADD COLUMN on_behalf_of_member_id uuid,
    ADD CONSTRAINT messages_on_behalf_of_fkey FOREIGN KEY (workspace_id, on_behalf_of_member_id)
        REFERENCES members (workspace_id, id) ON DELETE SET NULL (on_behalf_of_member_id) NOT VALID;

-- A delegate reads a personal mailbox like its owner, so a delegation is an access row here, and
-- only while the mailbox has an owner.
CREATE OR REPLACE VIEW member_inbox_access AS
SELECT workspace_id, inbox_id, member_id, NULL::uuid AS team_id FROM inbox_members
UNION ALL
SELECT ti.workspace_id, ti.inbox_id, tm.member_id, ti.team_id
FROM team_inboxes ti
JOIN team_members tm ON tm.workspace_id = ti.workspace_id AND tm.team_id = ti.team_id
UNION ALL
SELECT workspace_id, id, owner_member_id, NULL::uuid FROM inboxes WHERE owner_member_id IS NOT NULL
UNION ALL
SELECT d.workspace_id, d.inbox_id, d.member_id, NULL::uuid
FROM mailbox_delegates d
JOIN inboxes i ON i.workspace_id = d.workspace_id AND i.id = d.inbox_id AND i.owner_member_id IS NOT NULL;

-- +goose Down
CREATE OR REPLACE VIEW member_inbox_access AS
SELECT workspace_id, inbox_id, member_id, NULL::uuid AS team_id FROM inbox_members
UNION ALL
SELECT ti.workspace_id, ti.inbox_id, tm.member_id, ti.team_id
FROM team_inboxes ti
JOIN team_members tm ON tm.workspace_id = ti.workspace_id AND tm.team_id = ti.team_id
UNION ALL
SELECT workspace_id, id, owner_member_id, NULL::uuid FROM inboxes WHERE owner_member_id IS NOT NULL;
ALTER TABLE messages DROP CONSTRAINT messages_on_behalf_of_fkey, DROP COLUMN on_behalf_of_member_id;
DROP TABLE mailbox_delegates;
