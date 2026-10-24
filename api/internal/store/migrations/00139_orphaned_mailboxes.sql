-- +goose Up
-- A personal mailbox outlives its owner's account: the owner link goes, the mailbox stays personal
-- and is read by nobody until it is handed over or deleted, so it never turns into a shared inbox.
ALTER TABLE inboxes ADD COLUMN personal boolean NOT NULL DEFAULT false;
UPDATE inboxes SET personal = true WHERE owner_member_id IS NOT NULL;
ALTER TABLE inboxes
    DROP CONSTRAINT inboxes_owner_mail_check,
    DROP CONSTRAINT inboxes_owner_member_fkey,
    ADD CONSTRAINT inboxes_owner_member_fkey FOREIGN KEY (workspace_id, owner_member_id)
        REFERENCES members (workspace_id, id) ON DELETE SET NULL (owner_member_id),
    ADD CONSTRAINT inboxes_owner_personal_check CHECK (owner_member_id IS NULL OR personal),
    ADD CONSTRAINT inboxes_personal_mail_check CHECK (NOT personal OR purpose = 'mail');

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM inboxes WHERE personal AND owner_member_id IS NULL) THEN
        RAISE EXCEPTION 'orphaned personal mailboxes exist: hand them over or delete them first';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE inboxes
    DROP CONSTRAINT inboxes_personal_mail_check,
    DROP CONSTRAINT inboxes_owner_personal_check,
    DROP CONSTRAINT inboxes_owner_member_fkey,
    ADD CONSTRAINT inboxes_owner_member_fkey FOREIGN KEY (workspace_id, owner_member_id)
        REFERENCES members (workspace_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT inboxes_owner_mail_check CHECK (owner_member_id IS NULL OR purpose = 'mail'),
    DROP COLUMN personal;
