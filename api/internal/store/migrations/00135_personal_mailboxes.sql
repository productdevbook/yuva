-- +goose Up
ALTER TABLE inboxes
    ADD COLUMN owner_member_id uuid,
    ADD CONSTRAINT inboxes_owner_member_fkey FOREIGN KEY (workspace_id, owner_member_id)
        REFERENCES members (workspace_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT inboxes_owner_mail_check CHECK (owner_member_id IS NULL OR purpose = 'mail');
CREATE INDEX inboxes_owner_member_idx ON inboxes (workspace_id, owner_member_id) WHERE owner_member_id IS NOT NULL;

-- A personal mailbox is read by its owner only, so it is the owner's access row here and never
-- a grant: inbox_viewers and every access check built on this view follow it.
CREATE OR REPLACE VIEW member_inbox_access AS
SELECT workspace_id, inbox_id, member_id, NULL::uuid AS team_id FROM inbox_members
UNION ALL
SELECT ti.workspace_id, ti.inbox_id, tm.member_id, ti.team_id
FROM team_inboxes ti
JOIN team_members tm ON tm.workspace_id = ti.workspace_id AND tm.team_id = ti.team_id
UNION ALL
SELECT workspace_id, id, owner_member_id, NULL::uuid FROM inboxes WHERE owner_member_id IS NOT NULL;

-- +goose Down
CREATE OR REPLACE VIEW member_inbox_access AS
SELECT workspace_id, inbox_id, member_id, NULL::uuid AS team_id FROM inbox_members
UNION ALL
SELECT ti.workspace_id, ti.inbox_id, tm.member_id, ti.team_id
FROM team_inboxes ti
JOIN team_members tm ON tm.workspace_id = ti.workspace_id AND tm.team_id = ti.team_id;
DROP INDEX inboxes_owner_member_idx;
ALTER TABLE inboxes DROP CONSTRAINT inboxes_owner_mail_check, DROP CONSTRAINT inboxes_owner_member_fkey, DROP COLUMN owner_member_id;
