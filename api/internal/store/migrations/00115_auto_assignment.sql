-- +goose Up
ALTER TABLE inboxes
    ADD COLUMN assignment_mode text NOT NULL DEFAULT 'off' CHECK (assignment_mode IN ('off', 'round_robin', 'least_open')),
    ADD COLUMN assignment_cap  integer CHECK (assignment_cap BETWEEN 1 AND 500);

-- pool_id is the inbox's or the team's id.
CREATE TABLE assignment_turns (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    pool_id      uuid NOT NULL,
    member_id    uuid NOT NULL,
    last_at      timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, pool_id, member_id),
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE CASCADE
);

-- Indexed by 00116 outside a transaction. assign_after_rules holds a waiting conversation back
-- from the waiting runs while its rules job waits for the rate limit.
ALTER TABLE conversations
    ADD COLUMN assign_waiting_since timestamptz,
    ADD COLUMN assign_after_rules   boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE conversations DROP COLUMN assign_after_rules, DROP COLUMN assign_waiting_since;
DROP TABLE assignment_turns;
ALTER TABLE inboxes DROP COLUMN assignment_cap, DROP COLUMN assignment_mode;
