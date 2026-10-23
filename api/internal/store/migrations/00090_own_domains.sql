-- +goose Up
ALTER TABLE help_centers ADD COLUMN custom_domain text
    CHECK (custom_domain = lower(custom_domain) AND length(custom_domain) BETWEEN 4 AND 253);
ALTER TABLE help_centers ADD COLUMN domain_token text;
ALTER TABLE help_centers ADD COLUMN domain_state text CHECK (domain_state IN ('pending', 'verified', 'failed', 'off'));
ALTER TABLE help_centers ADD COLUMN domain_problem text;
ALTER TABLE help_centers ADD COLUMN domain_checked_at timestamptz;
ALTER TABLE help_centers ADD COLUMN domain_check_at timestamptz;
ALTER TABLE help_centers ADD COLUMN domain_verified_at timestamptz;
ALTER TABLE help_centers ADD COLUMN domain_missing_since timestamptz;
ALTER TABLE help_centers ADD COLUMN domain_claimed_at timestamptz;
ALTER TABLE help_centers ADD COLUMN cf_hostname_id text;
ALTER TABLE help_centers ADD COLUMN cf_status text;
ALTER TABLE help_centers ADD COLUMN cf_ssl_status text;
ALTER TABLE help_centers ADD COLUMN cf_errors text[];
ALTER TABLE help_centers ADD COLUMN cf_checked_at timestamptz;
ALTER TABLE help_centers ADD CONSTRAINT help_centers_domain_complete
    CHECK ((custom_domain IS NULL) = (domain_state IS NULL) AND (custom_domain IS NULL) = (domain_token IS NULL)
        AND (custom_domain IS NULL) = (domain_claimed_at IS NULL));
CREATE UNIQUE INDEX help_centers_custom_domain_verified_key ON help_centers (custom_domain) WHERE domain_state = 'verified';
CREATE INDEX help_centers_custom_domain_idx ON help_centers (custom_domain) WHERE custom_domain IS NOT NULL;
CREATE INDEX help_centers_domain_due_idx ON help_centers (domain_check_at) WHERE domain_check_at IS NOT NULL;

ALTER TABLE roadmap_boards ADD COLUMN custom_domain text
    CHECK (custom_domain = lower(custom_domain) AND length(custom_domain) BETWEEN 4 AND 253);
ALTER TABLE roadmap_boards ADD COLUMN domain_token text;
ALTER TABLE roadmap_boards ADD COLUMN domain_state text CHECK (domain_state IN ('pending', 'verified', 'failed', 'off'));
ALTER TABLE roadmap_boards ADD COLUMN domain_problem text;
ALTER TABLE roadmap_boards ADD COLUMN domain_checked_at timestamptz;
ALTER TABLE roadmap_boards ADD COLUMN domain_check_at timestamptz;
ALTER TABLE roadmap_boards ADD COLUMN domain_verified_at timestamptz;
ALTER TABLE roadmap_boards ADD COLUMN domain_missing_since timestamptz;
ALTER TABLE roadmap_boards ADD COLUMN domain_claimed_at timestamptz;
ALTER TABLE roadmap_boards ADD COLUMN cf_hostname_id text;
ALTER TABLE roadmap_boards ADD COLUMN cf_status text;
ALTER TABLE roadmap_boards ADD COLUMN cf_ssl_status text;
ALTER TABLE roadmap_boards ADD COLUMN cf_errors text[];
ALTER TABLE roadmap_boards ADD COLUMN cf_checked_at timestamptz;
ALTER TABLE roadmap_boards ADD CONSTRAINT roadmap_boards_domain_complete
    CHECK ((custom_domain IS NULL) = (domain_state IS NULL) AND (custom_domain IS NULL) = (domain_token IS NULL)
        AND (custom_domain IS NULL) = (domain_claimed_at IS NULL));
CREATE UNIQUE INDEX roadmap_boards_custom_domain_verified_key ON roadmap_boards (custom_domain) WHERE domain_state = 'verified';
CREATE INDEX roadmap_boards_custom_domain_idx ON roadmap_boards (custom_domain) WHERE custom_domain IS NOT NULL;
CREATE INDEX roadmap_boards_domain_due_idx ON roadmap_boards (domain_check_at) WHERE domain_check_at IS NOT NULL;

ALTER TABLE changelogs ADD COLUMN custom_domain text
    CHECK (custom_domain = lower(custom_domain) AND length(custom_domain) BETWEEN 4 AND 253);
ALTER TABLE changelogs ADD COLUMN domain_token text;
ALTER TABLE changelogs ADD COLUMN domain_state text CHECK (domain_state IN ('pending', 'verified', 'failed', 'off'));
ALTER TABLE changelogs ADD COLUMN domain_problem text;
ALTER TABLE changelogs ADD COLUMN domain_checked_at timestamptz;
ALTER TABLE changelogs ADD COLUMN domain_check_at timestamptz;
ALTER TABLE changelogs ADD COLUMN domain_verified_at timestamptz;
ALTER TABLE changelogs ADD COLUMN domain_missing_since timestamptz;
ALTER TABLE changelogs ADD COLUMN domain_claimed_at timestamptz;
ALTER TABLE changelogs ADD COLUMN cf_hostname_id text;
ALTER TABLE changelogs ADD COLUMN cf_status text;
ALTER TABLE changelogs ADD COLUMN cf_ssl_status text;
ALTER TABLE changelogs ADD COLUMN cf_errors text[];
ALTER TABLE changelogs ADD COLUMN cf_checked_at timestamptz;
ALTER TABLE changelogs ADD CONSTRAINT changelogs_domain_complete
    CHECK ((custom_domain IS NULL) = (domain_state IS NULL) AND (custom_domain IS NULL) = (domain_token IS NULL)
        AND (custom_domain IS NULL) = (domain_claimed_at IS NULL));
CREATE UNIQUE INDEX changelogs_custom_domain_verified_key ON changelogs (custom_domain) WHERE domain_state = 'verified';
CREATE INDEX changelogs_custom_domain_idx ON changelogs (custom_domain) WHERE custom_domain IS NOT NULL;
CREATE INDEX changelogs_domain_due_idx ON changelogs (domain_check_at) WHERE domain_check_at IS NOT NULL;

-- +goose Down
DROP INDEX changelogs_domain_due_idx;
DROP INDEX changelogs_custom_domain_idx;
DROP INDEX changelogs_custom_domain_verified_key;
ALTER TABLE changelogs DROP CONSTRAINT changelogs_domain_complete;
ALTER TABLE changelogs DROP COLUMN cf_checked_at, DROP COLUMN cf_errors, DROP COLUMN cf_ssl_status, DROP COLUMN cf_status,
    DROP COLUMN cf_hostname_id, DROP COLUMN domain_claimed_at, DROP COLUMN domain_missing_since, DROP COLUMN domain_verified_at,
    DROP COLUMN domain_check_at, DROP COLUMN domain_checked_at, DROP COLUMN domain_problem, DROP COLUMN domain_state,
    DROP COLUMN domain_token, DROP COLUMN custom_domain;

DROP INDEX roadmap_boards_domain_due_idx;
DROP INDEX roadmap_boards_custom_domain_idx;
DROP INDEX roadmap_boards_custom_domain_verified_key;
ALTER TABLE roadmap_boards DROP CONSTRAINT roadmap_boards_domain_complete;
ALTER TABLE roadmap_boards DROP COLUMN cf_checked_at, DROP COLUMN cf_errors, DROP COLUMN cf_ssl_status, DROP COLUMN cf_status,
    DROP COLUMN cf_hostname_id, DROP COLUMN domain_claimed_at, DROP COLUMN domain_missing_since, DROP COLUMN domain_verified_at,
    DROP COLUMN domain_check_at, DROP COLUMN domain_checked_at, DROP COLUMN domain_problem, DROP COLUMN domain_state,
    DROP COLUMN domain_token, DROP COLUMN custom_domain;

DROP INDEX help_centers_domain_due_idx;
DROP INDEX help_centers_custom_domain_idx;
DROP INDEX help_centers_custom_domain_verified_key;
ALTER TABLE help_centers DROP CONSTRAINT help_centers_domain_complete;
ALTER TABLE help_centers DROP COLUMN cf_checked_at, DROP COLUMN cf_errors, DROP COLUMN cf_ssl_status, DROP COLUMN cf_status,
    DROP COLUMN cf_hostname_id, DROP COLUMN domain_claimed_at, DROP COLUMN domain_missing_since, DROP COLUMN domain_verified_at,
    DROP COLUMN domain_check_at, DROP COLUMN domain_checked_at, DROP COLUMN domain_problem, DROP COLUMN domain_state,
    DROP COLUMN domain_token, DROP COLUMN custom_domain;
