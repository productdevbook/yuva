-- +goose Up
ALTER TABLE slack_inbox_settings
    ADD COLUMN sla_channel  text CHECK (sla_channel ~ '^[CG][A-Z0-9]{1,40}$'),
    ADD COLUMN sla_warnings boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE slack_inbox_settings DROP COLUMN sla_warnings, DROP COLUMN sla_channel;
