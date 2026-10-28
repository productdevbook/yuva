-- +goose Up
ALTER TABLE email_designs ADD COLUMN for_conversations boolean NOT NULL DEFAULT false;
CREATE INDEX email_designs_conversations_idx ON email_designs (workspace_id, updated_at DESC, id DESC)
    WHERE for_conversations AND archived_at IS NULL;

-- What the author asked for, drafts included; a NULL locale means the contact's.
ALTER TABLE messages
    ADD COLUMN email_design_id uuid,
    ADD COLUMN email_design_locale text CHECK (length(email_design_locale) BETWEEN 2 AND 35),
    ADD CONSTRAINT messages_email_design_fkey FOREIGN KEY (workspace_id, email_design_id)
        REFERENCES email_designs (workspace_id, id) ON DELETE SET NULL (email_design_id);

-- design_version set means full_text and full_html are the whole mail as sent; design_locale is ''
-- for a design without versions.
ALTER TABLE message_emails
    ADD COLUMN design_id uuid,
    ADD COLUMN design_version integer,
    ADD COLUMN design_locale text CHECK (length(design_locale) <= 35),
    ADD COLUMN outreach boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT message_emails_design_fkey FOREIGN KEY (workspace_id, design_id)
        REFERENCES email_designs (workspace_id, id) ON DELETE SET NULL (design_id);

CREATE TABLE inbox_mail_optouts (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    inbox_id     uuid NOT NULL,
    contact_id   uuid NOT NULL,
    source       text NOT NULL CHECK (source IN ('outreach', 'triggered')),
    created_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, inbox_id, contact_id),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX inbox_mail_optouts_contact_idx ON inbox_mail_optouts (workspace_id, contact_id);

-- +goose Down
DROP TABLE inbox_mail_optouts;
ALTER TABLE message_emails
    DROP CONSTRAINT message_emails_design_fkey,
    DROP COLUMN outreach,
    DROP COLUMN design_locale,
    DROP COLUMN design_version,
    DROP COLUMN design_id;
ALTER TABLE messages
    DROP CONSTRAINT messages_email_design_fkey,
    DROP COLUMN email_design_locale,
    DROP COLUMN email_design_id;
DROP INDEX email_designs_conversations_idx;
ALTER TABLE email_designs DROP COLUMN for_conversations;
