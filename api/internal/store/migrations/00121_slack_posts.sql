-- +goose Up
CREATE TABLE slack_posts (
    workspace_id    uuid NOT NULL,
    channel_id      text NOT NULL,
    ts              text NOT NULL CHECK (length(ts) <= 40),
    conversation_id uuid NOT NULL,
    message_id      uuid,
    with_content    boolean NOT NULL,
    created_at      timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, channel_id, ts),
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX slack_posts_conversation_idx ON slack_posts (workspace_id, conversation_id);

-- Posts made before this table: the top-level ones are in slack_threads. Whether they carried text
-- is unknown, so they count as having it.
INSERT INTO slack_posts (workspace_id, channel_id, ts, conversation_id, message_id, with_content, created_at)
SELECT workspace_id, channel_id, thread_ts, conversation_id, NULL, true, created_at FROM slack_threads
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE slack_posts;
