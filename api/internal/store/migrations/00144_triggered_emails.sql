-- +goose Up
ALTER TABLE broadcasts
    ADD COLUMN kind text NOT NULL DEFAULT 'once' CHECK (kind IN ('once', 'triggered')),
    ADD COLUMN trigger jsonb,
    ADD COLUMN delay_minutes integer CHECK (delay_minutes BETWEEN 0 AND 525600),
    ADD COLUMN basis text CHECK (basis IN ('service', 'marketing')),
    ADD COLUMN backfill_days integer NOT NULL DEFAULT 0 CHECK (backfill_days BETWEEN 0 AND 30),
    ADD COLUMN service_confirmed_by uuid,
    ADD COLUMN service_confirmed_by_key uuid,
    ADD COLUMN service_confirmed_at timestamptz,
    ADD CONSTRAINT broadcasts_triggered_check CHECK (
        (kind = 'once' AND trigger IS NULL AND delay_minutes IS NULL AND basis IS NULL)
        OR (kind = 'triggered' AND trigger IS NOT NULL AND delay_minutes IS NOT NULL AND basis IS NOT NULL)),
    DROP CONSTRAINT broadcasts_state_check,
    ADD CONSTRAINT broadcasts_state_check
        CHECK (state IN ('draft', 'scheduled', 'preparing', 'sending', 'active', 'paused', 'sent', 'cancelled', 'failed')),
    ADD CONSTRAINT broadcasts_service_confirmed_by_fkey FOREIGN KEY (workspace_id, service_confirmed_by)
        REFERENCES members (workspace_id, id) ON DELETE SET NULL (service_confirmed_by);
CREATE INDEX broadcasts_triggered_idx ON broadcasts (workspace_id, inbox_id)
    WHERE kind = 'triggered' AND state IN ('active', 'paused');
DROP INDEX broadcasts_active_idx;
CREATE INDEX broadcasts_active_idx ON broadcasts (workspace_id, state)
    WHERE state IN ('scheduled', 'preparing', 'sending', 'active', 'paused');
DROP INDEX broadcasts_channel_idx;
CREATE INDEX broadcasts_channel_idx ON broadcasts (workspace_id, channel_id, started_at)
    WHERE state IN ('sending', 'active');

-- One row per triggered e-mail and contact for as long as both exist: the once-per-contact rule.
CREATE TABLE triggered_enrollments (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    broadcast_id uuid NOT NULL,
    contact_id   uuid NOT NULL,
    triggered_at timestamptz NOT NULL,
    state        text NOT NULL DEFAULT 'waiting' CHECK (state IN ('waiting', 'queued', 'skipped', 'cancelled')),
    skip_reason  text CHECK (skip_reason IN ('no_address', 'blocked', 'suppressed', 'opted_out', 'stopped',
                                             'not_subscribed', 'not_matching', 'late', 'removed')),
    delivery_id  uuid,
    source       text NOT NULL CHECK (source IN ('trigger', 'backfill')),
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, broadcast_id, contact_id),
    FOREIGN KEY (workspace_id, broadcast_id) REFERENCES broadcasts (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, delivery_id) REFERENCES broadcast_deliveries (workspace_id, id) ON DELETE SET NULL (delivery_id)
);
CREATE INDEX triggered_enrollments_due_idx ON triggered_enrollments (workspace_id, broadcast_id, triggered_at, contact_id)
    WHERE state = 'waiting';
CREATE INDEX triggered_enrollments_contact_idx ON triggered_enrollments (workspace_id, contact_id);
CREATE INDEX triggered_enrollments_list_idx ON triggered_enrollments (workspace_id, broadcast_id, triggered_at DESC, contact_id DESC);

-- NULL for rows from before triggered e-mails; the back-fill reads the contact's created_at for them.
ALTER TABLE contact_external_ids ADD COLUMN created_at timestamptz;
CREATE INDEX contact_external_ids_inbox_created_idx ON contact_external_ids (workspace_id, inbox_id, created_at);

ALTER TABLE broadcast_deliveries
    DROP CONSTRAINT broadcast_deliveries_skip_reason_check,
    ADD CONSTRAINT broadcast_deliveries_skip_reason_check
        CHECK (skip_reason IN ('suppressed', 'opted_out', 'unsubscribed', 'cancelled', 'stopped'));

ALTER TABLE operator_notices
    DROP CONSTRAINT operator_notices_kind_check,
    ADD CONSTRAINT operator_notices_kind_check
        CHECK (kind IN ('signup', 'workspace', 'broadcast_approval', 'broadcast_suspended', 'broadcast_paused', 'analytics_limit', 'analytics_dropped',
                        'ses_tenant_paused', 'workspace_deleted', 'email_unproven', 'triggered_started'));

-- A broadcast audience's conditions and languages, shared by the audience snapshot and the
-- triggered e-mails' due-time check.
-- +goose StatementBegin
CREATE FUNCTION broadcast_audience_matches(attributes jsonb, locale text, conditions jsonb, locales text[]) RETURNS boolean
    LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT (cardinality(locales) = 0 OR EXISTS (
        SELECT 1 FROM unnest(locales) AS l (tag)
        WHERE lower(coalesce(locale, '')) = lower(l.tag) OR lower(coalesce(locale, '')) LIKE lower(l.tag) || '-%'))
    AND NOT EXISTS (
        SELECT 1 FROM jsonb_to_recordset(conditions) AS k (attribute text, op text, value text, "values" text[])
        WHERE NOT coalesce(CASE k.op
            WHEN 'exists' THEN attributes -> k.attribute IS NOT NULL
            WHEN 'not_exists' THEN attributes -> k.attribute IS NULL
            WHEN 'eq' THEN attributes ->> k.attribute = k.value
            WHEN 'neq' THEN (attributes ->> k.attribute) IS DISTINCT FROM k.value
            WHEN 'contains' THEN strpos(lower(attributes ->> k.attribute), lower(k.value)) > 0
            WHEN 'in' THEN attributes ->> k.attribute = ANY(k."values")
            WHEN 'gt' THEN CASE
                WHEN (attributes ->> k.attribute) ~ '^-?[0-9]{1,30}(\.[0-9]{1,30})?$' AND k.value ~ '^-?[0-9]{1,30}(\.[0-9]{1,30})?$'
                THEN (attributes ->> k.attribute)::numeric > k.value::numeric
                ELSE (attributes ->> k.attribute) > k.value END
            WHEN 'lt' THEN CASE
                WHEN (attributes ->> k.attribute) ~ '^-?[0-9]{1,30}(\.[0-9]{1,30})?$' AND k.value ~ '^-?[0-9]{1,30}(\.[0-9]{1,30})?$'
                THEN (attributes ->> k.attribute)::numeric < k.value::numeric
                ELSE (attributes ->> k.attribute) < k.value END
            ELSE false END, false))
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION broadcast_audience_matches(jsonb, text, jsonb, text[]);
DELETE FROM operator_notices WHERE kind = 'triggered_started';
ALTER TABLE operator_notices
    DROP CONSTRAINT operator_notices_kind_check,
    ADD CONSTRAINT operator_notices_kind_check
        CHECK (kind IN ('signup', 'workspace', 'broadcast_approval', 'broadcast_suspended', 'broadcast_paused', 'analytics_limit', 'analytics_dropped',
                        'ses_tenant_paused', 'workspace_deleted', 'email_unproven'));
DELETE FROM broadcast_deliveries d USING broadcasts b
WHERE b.workspace_id = d.workspace_id AND b.id = d.broadcast_id AND b.kind = 'triggered';
UPDATE broadcast_deliveries SET skip_reason = 'cancelled' WHERE skip_reason = 'stopped';
ALTER TABLE broadcast_deliveries
    DROP CONSTRAINT broadcast_deliveries_skip_reason_check,
    ADD CONSTRAINT broadcast_deliveries_skip_reason_check
        CHECK (skip_reason IN ('suppressed', 'opted_out', 'unsubscribed', 'cancelled'));
DROP INDEX contact_external_ids_inbox_created_idx;
ALTER TABLE contact_external_ids DROP COLUMN created_at;
DROP TABLE triggered_enrollments;
DELETE FROM broadcasts WHERE kind = 'triggered';
DROP INDEX broadcasts_channel_idx;
CREATE INDEX broadcasts_channel_idx ON broadcasts (workspace_id, channel_id, started_at) WHERE state = 'sending';
DROP INDEX broadcasts_active_idx;
CREATE INDEX broadcasts_active_idx ON broadcasts (workspace_id, state) WHERE state IN ('scheduled', 'preparing', 'sending', 'paused');
DROP INDEX broadcasts_triggered_idx;
ALTER TABLE broadcasts
    DROP CONSTRAINT broadcasts_service_confirmed_by_fkey,
    DROP CONSTRAINT broadcasts_state_check,
    ADD CONSTRAINT broadcasts_state_check
        CHECK (state IN ('draft', 'scheduled', 'preparing', 'sending', 'paused', 'sent', 'cancelled', 'failed')),
    DROP CONSTRAINT broadcasts_triggered_check,
    DROP COLUMN service_confirmed_at,
    DROP COLUMN service_confirmed_by_key,
    DROP COLUMN service_confirmed_by,
    DROP COLUMN backfill_days,
    DROP COLUMN basis,
    DROP COLUMN delay_minutes,
    DROP COLUMN trigger,
    DROP COLUMN kind;
