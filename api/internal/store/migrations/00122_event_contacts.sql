-- +goose Up
-- The contact an event is about (a contact event, or a message the contact wrote), and the
-- conversation and contact a webhook delivery came from, so deleting a contact or conversation
-- removes their copies by index instead of searching payloads.
ALTER TABLE events ADD COLUMN contact_id uuid;
CREATE INDEX events_contact_idx ON events (workspace_id, contact_id) WHERE contact_id IS NOT NULL;
UPDATE events SET contact_id = (payload->>'id')::uuid WHERE type = 'contact.updated' AND conversation_id IS NULL;

ALTER TABLE webhook_deliveries ADD COLUMN conversation_id uuid, ADD COLUMN contact_id uuid;
CREATE INDEX webhook_deliveries_conversation_idx ON webhook_deliveries (workspace_id, conversation_id) WHERE conversation_id IS NOT NULL;
CREATE INDEX webhook_deliveries_contact_idx ON webhook_deliveries (workspace_id, contact_id) WHERE contact_id IS NOT NULL;

-- +goose Down
DROP INDEX webhook_deliveries_contact_idx;
DROP INDEX webhook_deliveries_conversation_idx;
ALTER TABLE webhook_deliveries DROP COLUMN contact_id, DROP COLUMN conversation_id;
DROP INDEX events_contact_idx;
ALTER TABLE events DROP COLUMN contact_id;
