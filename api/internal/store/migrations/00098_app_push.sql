-- +goose Up
CREATE TABLE app_push_settings (
    workspace_id     uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    channel_id       uuid NOT NULL,
    enabled          boolean NOT NULL DEFAULT false,
    show_content     boolean NOT NULL DEFAULT false,
    apns_key_id      text CHECK (apns_key_id ~ '^[A-Z0-9]{10}$'),
    apns_team_id     text CHECK (apns_team_id ~ '^[A-Z0-9]{10}$'),
    apns_bundle_id   text CHECK (length(apns_bundle_id) BETWEEN 1 AND 155),
    apns_production  boolean NOT NULL DEFAULT true,
    apns_key         bytea,
    apns_verified    boolean NOT NULL DEFAULT false,
    fcm_project_id   text CHECK (length(fcm_project_id) BETWEEN 1 AND 100),
    fcm_client_email text CHECK (length(fcm_client_email) BETWEEN 1 AND 320),
    fcm_credentials  bytea,
    last_error       text CHECK (length(last_error) <= 1000),
    last_error_at    timestamptz,
    updated_at       timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, channel_id),
    FOREIGN KEY (workspace_id, channel_id) REFERENCES channels (workspace_id, id) ON DELETE CASCADE,
    CHECK ((apns_key IS NULL) = (apns_key_id IS NULL) AND (apns_key IS NULL) = (apns_team_id IS NULL) AND (apns_key IS NULL) = (apns_bundle_id IS NULL)),
    CHECK ((fcm_credentials IS NULL) = (fcm_project_id IS NULL)),
    CHECK (NOT enabled OR apns_key IS NOT NULL OR fcm_credentials IS NOT NULL)
);

CREATE TABLE push_devices (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    channel_id   uuid NOT NULL,
    contact_id   uuid NOT NULL,
    platform     text NOT NULL CHECK (platform IN ('ios', 'android')),
    token        text NOT NULL CHECK (length(token) BETWEEN 1 AND 1024),
    app_id       text NOT NULL CHECK (length(app_id) BETWEEN 1 AND 155),
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL,
    FOREIGN KEY (workspace_id, channel_id) REFERENCES channels (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX push_devices_token_key ON push_devices (workspace_id, channel_id, token);
CREATE INDEX push_devices_contact_idx ON push_devices (workspace_id, contact_id);

ALTER TABLE conversations
    ADD COLUMN push_through timestamptz,
    ADD COLUMN push_sent_at timestamptz;

-- +goose Down
ALTER TABLE conversations DROP COLUMN push_sent_at, DROP COLUMN push_through;
DROP TABLE push_devices;
DROP TABLE app_push_settings;
