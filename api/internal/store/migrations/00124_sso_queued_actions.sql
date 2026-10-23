-- +goose Up
-- The single sign-on settings row of the session that queued a member's action, so the job applies
-- sso_required as the request would have. No foreign keys, like sessions.sso_settings_id.
ALTER TABLE linked_replies ADD COLUMN sso_settings_id uuid;
ALTER TABLE issue_links ADD COLUMN sso_settings_id uuid;
ALTER TABLE roadmap_stage_changes ADD COLUMN sso_settings_id uuid;

-- +goose Down
ALTER TABLE roadmap_stage_changes DROP COLUMN sso_settings_id;
ALTER TABLE issue_links DROP COLUMN sso_settings_id;
ALTER TABLE linked_replies DROP COLUMN sso_settings_id;
