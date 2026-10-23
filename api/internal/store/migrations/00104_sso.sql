-- +goose Up
-- One identity provider per workspace; SAML (#161) adds its own columns and protocol 'saml'.
CREATE TABLE sso_settings (
    workspace_id       uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE CASCADE,
    id                 uuid NOT NULL UNIQUE,
    protocol           text CHECK (protocol IN ('oidc')),
    oidc_issuer        text CHECK (length(oidc_issuer) BETWEEN 1 AND 500),
    oidc_client_id     text CHECK (length(oidc_client_id) BETWEEN 1 AND 500),
    oidc_client_secret bytea,
    oidc_scopes        text[] NOT NULL DEFAULT '{}',
    groups_claim       text NOT NULL DEFAULT 'groups' CHECK (length(groups_claim) BETWEEN 1 AND 100),
    allowed_groups     text[] NOT NULL DEFAULT '{}',
    sso_required       boolean NOT NULL DEFAULT false,
    jit                boolean NOT NULL DEFAULT false,
    jit_role           text NOT NULL DEFAULT 'agent' CHECK (jit_role IN ('admin', 'agent')),
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL,
    CHECK ((protocol = 'oidc') = (oidc_issuer IS NOT NULL AND oidc_client_id IS NOT NULL AND oidc_client_secret IS NOT NULL)),
    CHECK (NOT sso_required OR protocol IS NOT NULL)
);

-- A verified domain belongs to one workspace on the server; pending claims may overlap.
CREATE TABLE sso_domains (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    domain       text NOT NULL CHECK (length(domain) BETWEEN 4 AND 253 AND domain = lower(domain)),
    token        text NOT NULL CHECK (length(token) BETWEEN 1 AND 100),
    verified_at  timestamptz,
    checked_at   timestamptz,
    problem      text CHECK (length(problem) <= 1000),
    created_at   timestamptz NOT NULL,
    UNIQUE (workspace_id, domain)
);
CREATE UNIQUE INDEX sso_domains_verified_key ON sso_domains (domain) WHERE verified_at IS NOT NULL;

CREATE TABLE sso_logins (
    state_hash    bytea PRIMARY KEY,
    workspace_id  uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    settings_id   uuid NOT NULL,
    nonce         text NOT NULL,
    verifier      text NOT NULL,
    return_to     text NOT NULL DEFAULT '/' CHECK (length(return_to) <= 2000),
    created_at    timestamptz NOT NULL,
    expires_at    timestamptz NOT NULL
);
CREATE INDEX sso_logins_expires_idx ON sso_logins (expires_at);

-- A session made through a workspace's identity provider names it; sso_required lets only such
-- sessions act there for people of its domains (owners excepted). No foreign keys: sessions are the
-- person's and outlive a workspace's settings.
ALTER TABLE sessions
    DROP CONSTRAINT sessions_method_check,
    ADD CONSTRAINT sessions_method_check CHECK (method IN ('code', 'passkey', 'sso')),
    ADD COLUMN sso_workspace_id uuid,
    ADD COLUMN sso_settings_id uuid,
    ADD CONSTRAINT sessions_sso_check CHECK ((method = 'sso') = (sso_settings_id IS NOT NULL AND sso_workspace_id IS NOT NULL));

ALTER TABLE oauth_grants ADD COLUMN sso_settings_id uuid;

-- +goose Down
ALTER TABLE oauth_grants DROP COLUMN sso_settings_id;
DELETE FROM sessions WHERE method = 'sso';
ALTER TABLE sessions
    DROP CONSTRAINT sessions_sso_check,
    DROP COLUMN sso_workspace_id,
    DROP COLUMN sso_settings_id,
    DROP CONSTRAINT sessions_method_check,
    ADD CONSTRAINT sessions_method_check CHECK (method IN ('code', 'passkey'));
DROP TABLE sso_logins;
DROP TABLE sso_domains;
DROP TABLE sso_settings;
