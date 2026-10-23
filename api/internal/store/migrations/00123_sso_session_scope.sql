-- +goose Up
-- The daily re-check of verified domains: when it first missed the TXT record; seven days later
-- the claim lapses.
ALTER TABLE sso_domains ADD COLUMN missing_since timestamptz;
CREATE INDEX sso_domains_recheck_idx ON sso_domains (checked_at) WHERE verified_at IS NOT NULL;

-- The people who signed in to a workspace through its provider, so only the first sign-in mails.
CREATE TABLE sso_sign_ins (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    person_id    uuid NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, person_id)
);
INSERT INTO sso_sign_ins (workspace_id, person_id, created_at)
SELECT s.sso_workspace_id, s.person_id, min(s.created_at) FROM sessions s
JOIN workspaces w ON w.id = s.sso_workspace_id
WHERE s.method = 'sso'
GROUP BY s.sso_workspace_id, s.person_id;

-- +goose Down
DROP TABLE sso_sign_ins;
DROP INDEX sso_domains_recheck_idx;
ALTER TABLE sso_domains DROP COLUMN missing_since;
