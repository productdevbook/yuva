-- +goose Up
CREATE TABLE people (
    id              uuid PRIMARY KEY,
    email           text NOT NULL UNIQUE CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 320),
    name            text NOT NULL DEFAULT '' CHECK (length(name) <= 200),
    locale          text NOT NULL DEFAULT 'en' CHECK (locale IN ('en', 'tr')),
    webauthn_handle bytea NOT NULL UNIQUE,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE members (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    person_id    uuid NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    role         text NOT NULL CHECK (role IN ('owner', 'admin', 'agent')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, person_id)
);
CREATE INDEX members_person_id_idx ON members (person_id);

CREATE TABLE invites (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    email        text NOT NULL CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 320),
    role         text NOT NULL CHECK (role IN ('owner', 'admin', 'agent')),
    locale       text NOT NULL DEFAULT 'en' CHECK (locale IN ('en', 'tr')),
    invited_by   uuid REFERENCES members (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL,
    expires_at   timestamptz NOT NULL,
    UNIQUE (workspace_id, email)
);
CREATE INDEX invites_email_idx ON invites (email);

CREATE TABLE api_keys (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    prefix       text NOT NULL,
    secret_hash  bytea NOT NULL UNIQUE,
    created_by   uuid REFERENCES members (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at   timestamptz
);
CREATE INDEX api_keys_workspace_id_idx ON api_keys (workspace_id);

CREATE TABLE login_codes (
    id          uuid PRIMARY KEY,
    email       text NOT NULL,
    code_hash   bytea NOT NULL,
    ip          text NOT NULL,
    attempts    integer NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL,
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz
);
CREATE INDEX login_codes_email_idx ON login_codes (email, created_at DESC);
CREATE INDEX login_codes_ip_idx ON login_codes (ip, created_at DESC);

CREATE TABLE sessions (
    id           uuid PRIMARY KEY,
    person_id    uuid NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    token_hash   bytea NOT NULL UNIQUE,
    method       text NOT NULL CHECK (method IN ('code', 'passkey')),
    created_at   timestamptz NOT NULL,
    expires_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL
);
CREATE INDEX sessions_person_id_idx ON sessions (person_id);

CREATE TABLE passkeys (
    id            uuid PRIMARY KEY,
    person_id     uuid NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    rp_id         text NOT NULL,
    credential_id bytea NOT NULL,
    name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    credential    jsonb NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_used_at  timestamptz,
    UNIQUE (rp_id, credential_id)
);
CREATE INDEX passkeys_person_id_idx ON passkeys (person_id);

CREATE TABLE webauthn_ceremonies (
    id_hash      bytea PRIMARY KEY,
    kind         text NOT NULL CHECK (kind IN ('registration', 'login')),
    person_id    uuid REFERENCES people (id) ON DELETE CASCADE,
    session_data jsonb NOT NULL,
    expires_at   timestamptz NOT NULL
);

-- +goose Down
DROP TABLE webauthn_ceremonies;
DROP TABLE passkeys;
DROP TABLE sessions;
DROP TABLE login_codes;
DROP TABLE api_keys;
DROP TABLE invites;
DROP TABLE members;
DROP TABLE people;
