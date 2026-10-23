-- +goose Up
-- SAML 2.0 providers share the settings row, the domains and the rules with OpenID Connect; a
-- workspace has one protocol. Certificates are DER.
ALTER TABLE sso_settings
    DROP CONSTRAINT sso_settings_protocol_check,
    ADD CONSTRAINT sso_settings_protocol_check CHECK (protocol IN ('oidc', 'saml')),
    ADD COLUMN saml_entity_id       text CHECK (length(saml_entity_id) BETWEEN 1 AND 1000),
    ADD COLUMN saml_sso_url         text CHECK (length(saml_sso_url) BETWEEN 1 AND 2000),
    ADD COLUMN saml_certificates    bytea[] NOT NULL DEFAULT '{}' CHECK (cardinality(saml_certificates) <= 10),
    ADD COLUMN saml_metadata_url    text CHECK (length(saml_metadata_url) BETWEEN 1 AND 2000),
    ADD COLUMN saml_metadata_at     timestamptz,
    ADD COLUMN saml_metadata_tried_at timestamptz,
    ADD COLUMN saml_email_attribute text CHECK (length(saml_email_attribute) BETWEEN 1 AND 200),
    ADD CONSTRAINT sso_settings_saml_check CHECK ((protocol = 'saml') =
        (saml_entity_id IS NOT NULL AND saml_sso_url IS NOT NULL AND cardinality(saml_certificates) > 0));

-- Assertion ids seen, kept until the assertion could no longer be accepted, so none is used twice.
CREATE TABLE saml_assertions (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    id_hash      bytea NOT NULL,
    expires_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, id_hash)
);
CREATE INDEX saml_assertions_expires_idx ON saml_assertions (workspace_id, expires_at);

-- +goose Down
DROP TABLE saml_assertions;
DELETE FROM sso_logins s USING sso_settings t WHERE t.workspace_id = s.workspace_id AND t.protocol = 'saml';
UPDATE sso_settings SET protocol = NULL, sso_required = false WHERE protocol = 'saml';
ALTER TABLE sso_settings
    DROP CONSTRAINT sso_settings_saml_check,
    DROP COLUMN saml_entity_id,
    DROP COLUMN saml_sso_url,
    DROP COLUMN saml_certificates,
    DROP COLUMN saml_metadata_url,
    DROP COLUMN saml_metadata_at,
    DROP COLUMN saml_metadata_tried_at,
    DROP COLUMN saml_email_attribute,
    DROP CONSTRAINT sso_settings_protocol_check,
    ADD CONSTRAINT sso_settings_protocol_check CHECK (protocol IN ('oidc'));
