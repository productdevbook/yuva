-- +goose Up
-- Actors and targets have no foreign keys: an entry outlives the member, key or inbox it names.
CREATE TABLE audit_log (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    actor_kind      text NOT NULL CHECK (actor_kind IN ('member', 'api_key', 'oauth_grant', 'operator')),
    actor_id        uuid,
    actor_member_id uuid,
    actor_name      text NOT NULL DEFAULT '' CHECK (length(actor_name) <= 400),
    via             text CHECK (length(via) <= 200),
    action          text NOT NULL CHECK (action ~ '^[a-z_]+\.[a-z_]+$' AND length(action) <= 100),
    target_type     text NOT NULL CHECK (target_type ~ '^[a-z_]+$' AND length(target_type) <= 50),
    target_id       uuid NOT NULL,
    details         jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(details) = 'object' AND length(details::text) <= 4000),
    created_at      timestamptz NOT NULL,
    CHECK ((actor_kind = 'operator') = (actor_id IS NULL))
);
CREATE INDEX audit_log_time_idx ON audit_log (workspace_id, created_at DESC, id DESC);
CREATE INDEX audit_log_actor_idx ON audit_log (workspace_id, actor_id, created_at DESC) WHERE actor_id IS NOT NULL;
CREATE INDEX audit_log_actor_member_idx ON audit_log (workspace_id, actor_member_id, created_at DESC) WHERE actor_member_id IS NOT NULL;
CREATE INDEX audit_log_action_idx ON audit_log (workspace_id, action, created_at DESC);

-- +goose Down
DROP TABLE audit_log;
