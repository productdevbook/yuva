-- +goose Up
-- A conversation answers mail the team sent: a broadcast or triggered e-mail reply, or designed outreach.
-- +goose StatementBegin
CREATE FUNCTION conversation_outbound(ws uuid, conversation uuid, broadcast uuid) RETURNS boolean
    LANGUAGE sql STABLE PARALLEL SAFE AS $$
    SELECT broadcast IS NOT NULL OR EXISTS (
        SELECT 1 FROM message_emails e WHERE e.workspace_id = ws AND e.conversation_id = conversation AND e.outreach)
$$;
-- +goose StatementEnd

CREATE INDEX message_emails_outreach_idx ON message_emails (workspace_id, conversation_id) WHERE outreach;

ALTER TABLE report_rollups ADD COLUMN outbound boolean NOT NULL DEFAULT false;
-- Rows from before count as support; the next rollup run rebuilds the last 31 days with origins.
UPDATE report_rollup_state SET recomputed_on = NULL, recompute_at = NULL;

-- +goose Down
ALTER TABLE report_rollups DROP COLUMN outbound;
DROP INDEX message_emails_outreach_idx;
DROP FUNCTION conversation_outbound(uuid, uuid, uuid);
