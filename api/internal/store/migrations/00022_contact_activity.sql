-- +goose Up
ALTER TABLE contacts ADD COLUMN last_active_at timestamptz;
UPDATE contacts c SET last_active_at = a.at
FROM (
    SELECT contact_id, max(at) AS at FROM (
        SELECT workspace_id, author_contact_id AS contact_id, max(created_at) AS at FROM messages
        WHERE author_contact_id IS NOT NULL GROUP BY 1, 2
        UNION ALL
        SELECT workspace_id, contact_id, max(last_seen_at) FROM contact_sessions GROUP BY 1, 2
    ) x GROUP BY workspace_id, contact_id
) a
WHERE a.contact_id = c.id;
CREATE INDEX contacts_activity_idx ON contacts (workspace_id, (coalesce(last_active_at, created_at)) DESC, id DESC);

-- +goose Down
DROP INDEX contacts_activity_idx;
ALTER TABLE contacts DROP COLUMN last_active_at;
