-- +goose NO TRANSACTION
-- +goose Up
ALTER TABLE messages ADD COLUMN IF NOT EXISTS forward jsonb;

-- +goose StatementBegin
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'messages_kind_forward_check') THEN
        ALTER TABLE messages
            DROP CONSTRAINT messages_kind_check,
            DROP CONSTRAINT messages_check,
            DROP CONSTRAINT messages_draft_check,
            ADD CONSTRAINT messages_kind_forward_check CHECK (kind IN ('message', 'note', 'event', 'forward')) NOT VALID,
            ADD CONSTRAINT messages_direction_kind_check CHECK ((kind IN ('message', 'forward')) = (direction IS NOT NULL)) NOT VALID,
            ADD CONSTRAINT messages_draft_kind_check CHECK (NOT draft OR (kind IN ('message', 'forward') AND direction = 'out')) NOT VALID,
            ADD CONSTRAINT messages_forward_check CHECK (
                (kind = 'forward' AND direction = 'out') = (forward IS NOT NULL) AND (forward IS NULL OR jsonb_typeof(forward) = 'object')
            ) NOT VALID;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE messages VALIDATE CONSTRAINT messages_kind_forward_check;
ALTER TABLE messages VALIDATE CONSTRAINT messages_direction_kind_check;
ALTER TABLE messages VALIDATE CONSTRAINT messages_draft_kind_check;
ALTER TABLE messages VALIDATE CONSTRAINT messages_forward_check;

ALTER TABLE conversation_participants DROP CONSTRAINT IF EXISTS conversation_participants_role_check;
ALTER TABLE conversation_participants ADD CONSTRAINT conversation_participants_role_check CHECK (role IN ('cc', 'forwarded'));

-- +goose Down
DELETE FROM conversation_participants WHERE role = 'forwarded';
ALTER TABLE conversation_participants DROP CONSTRAINT conversation_participants_role_check;
ALTER TABLE conversation_participants ADD CONSTRAINT conversation_participants_role_check CHECK (role IN ('cc'));
DELETE FROM messages WHERE kind = 'forward';
ALTER TABLE messages
    DROP CONSTRAINT messages_forward_check,
    DROP CONSTRAINT messages_draft_kind_check,
    DROP CONSTRAINT messages_direction_kind_check,
    DROP CONSTRAINT messages_kind_forward_check,
    ADD CONSTRAINT messages_kind_check CHECK (kind IN ('message', 'note', 'event')),
    ADD CONSTRAINT messages_check CHECK ((kind = 'message') = (direction IS NOT NULL)),
    ADD CONSTRAINT messages_draft_check CHECK (NOT draft OR (kind = 'message' AND direction = 'out')),
    DROP COLUMN forward;
