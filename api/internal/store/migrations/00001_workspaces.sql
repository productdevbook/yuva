-- +goose Up
CREATE TABLE workspaces (
    id         uuid PRIMARY KEY,
    name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE workspaces;
