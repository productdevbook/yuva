-- +goose Up
ALTER TABLE automation_rules DROP CONSTRAINT automation_rules_trigger_check,
    ADD CONSTRAINT automation_rules_trigger_check CHECK (trigger IN ('conversation_created', 'message_received', 'status_changed', 'idle', 'mail_received'));

-- +goose Down
DELETE FROM automation_rules WHERE trigger = 'mail_received';
ALTER TABLE automation_rules DROP CONSTRAINT automation_rules_trigger_check,
    ADD CONSTRAINT automation_rules_trigger_check CHECK (trigger IN ('conversation_created', 'message_received', 'status_changed', 'idle'));
