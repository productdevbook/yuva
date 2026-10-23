-- +goose Up
-- Report counts per UTC quarter hour, so a local day of any time zone in use today is a whole
-- number of buckets. A row with label_id repeats a figure for one label of its conversation;
-- totals read the rows without. Durations (first_reply, resolution) count per histogram bin, the
-- others use bin 0. The ids are not foreign keys: reads drop deleted inboxes and labels and fold
-- deleted teams and members into "none", so deletions never rewrite the table.
CREATE TABLE report_rollups (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    bucket       timestamptz NOT NULL,
    inbox_id     uuid NOT NULL,
    channel_kind text,
    team_id      uuid,
    member_id    uuid,
    label_id     uuid,
    metric       text NOT NULL,
    bin          smallint NOT NULL DEFAULT 0,
    n            integer NOT NULL CHECK (n > 0)
);
CREATE INDEX report_rollups_bucket_idx ON report_rollups (workspace_id, bucket);

-- built_until: every bucket before it is rolled up; recomputed_on: the UTC day whose nightly
-- rebuild finished; recompute_at: where an unfinished nightly rebuild goes on.
CREATE TABLE report_rollup_state (
    workspace_id  uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE CASCADE,
    built_until   timestamptz NOT NULL,
    recomputed_on date,
    recompute_at  timestamptz,
    updated_at    timestamptz NOT NULL
);

-- +goose Down
DROP TABLE report_rollup_state;
DROP TABLE report_rollups;
