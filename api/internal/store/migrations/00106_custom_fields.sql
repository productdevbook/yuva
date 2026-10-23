-- +goose Up
CREATE TABLE custom_fields (
    id                uuid PRIMARY KEY,
    workspace_id      uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    object            text NOT NULL CHECK (object IN ('conversation', 'contact')),
    key               text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,39}$'),
    name              text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    type              text NOT NULL CHECK (type IN ('text', 'number', 'date', 'select', 'checkbox')),
    options           jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(options) = 'array'),
    required_on_close boolean NOT NULL DEFAULT false,
    position          integer NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, object, key)
);

ALTER TABLE conversations ADD COLUMN fields jsonb NOT NULL DEFAULT '{}';
ALTER TABLE conversations ADD CONSTRAINT conversations_fields_object CHECK (jsonb_typeof(fields) = 'object') NOT VALID;

-- +goose Down
ALTER TABLE conversations DROP CONSTRAINT conversations_fields_object;
ALTER TABLE conversations DROP COLUMN fields;
DROP TABLE custom_fields;
