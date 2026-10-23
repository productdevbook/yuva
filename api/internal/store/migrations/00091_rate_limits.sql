-- +goose Up
-- Unlogged: a counter lost in a crash only opens a fresh window, and the hottest write path skips the WAL.
CREATE UNLOGGED TABLE rate_limits (
    key bytea PRIMARY KEY,
    started_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    lim integer NOT NULL,
    want integer NOT NULL,
    count integer NOT NULL,
    granted integer NOT NULL
) WITH (fillfactor = 70);

CREATE UNLOGGED TABLE rate_slots (
    key bytea NOT NULL,
    slot integer NOT NULL,
    holder uuid NOT NULL,
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (key, slot)
);

-- +goose Down
DROP TABLE rate_slots;
DROP TABLE rate_limits;
