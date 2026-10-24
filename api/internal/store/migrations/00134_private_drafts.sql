-- +goose Up
ALTER TABLE messages
    ADD COLUMN scheduled_at timestamptz,
    ADD COLUMN scheduled_by uuid,
    ADD COLUMN scheduled_cc text[],
    ADD COLUMN scheduled_bcc text[],
    ADD COLUMN scheduled_from text,
    ADD COLUMN send_error jsonb,
    ADD CONSTRAINT messages_scheduled_by_fkey FOREIGN KEY (workspace_id, scheduled_by)
        REFERENCES members (workspace_id, id) ON DELETE SET NULL (scheduled_by) NOT VALID,
    ADD CONSTRAINT messages_scheduled_draft CHECK (scheduled_at IS NULL OR draft) NOT VALID;

CREATE INDEX messages_scheduled_idx ON messages (workspace_id, scheduled_by, scheduled_at) WHERE scheduled_at IS NOT NULL;

ALTER TABLE conversations
    ADD COLUMN draft_of uuid,
    ADD CONSTRAINT conversations_draft_of_fkey FOREIGN KEY (workspace_id, draft_of)
        REFERENCES members (workspace_id, id) ON DELETE CASCADE NOT VALID;

CREATE INDEX conversations_draft_of_idx ON conversations (workspace_id, draft_of) WHERE draft_of IS NOT NULL;

ALTER TABLE events ADD COLUMN only_member uuid;

-- +goose StatementBegin
CREATE FUNCTION draft_visible_to(draft boolean, author_type text, author_member_id uuid, viewer uuid) RETURNS boolean
    LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT NOT draft OR author_type <> 'member' OR coalesce(author_member_id = viewer, false)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION conversation_visible_to(draft_of uuid, viewer uuid) RETURNS boolean
    LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT draft_of IS NULL OR coalesce(draft_of = viewer, false)
$$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE events DROP COLUMN only_member;
DROP FUNCTION conversation_visible_to(uuid, uuid);
DROP FUNCTION draft_visible_to(boolean, text, uuid, uuid);
DROP INDEX conversations_draft_of_idx;
ALTER TABLE conversations DROP COLUMN draft_of;
DROP INDEX messages_scheduled_idx;
ALTER TABLE messages
    DROP COLUMN scheduled_at,
    DROP COLUMN scheduled_by,
    DROP COLUMN scheduled_cc,
    DROP COLUMN scheduled_bcc,
    DROP COLUMN scheduled_from,
    DROP COLUMN send_error;
