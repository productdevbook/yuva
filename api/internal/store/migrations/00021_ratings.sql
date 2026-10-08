-- +goose Up
ALTER TABLE inboxes ADD COLUMN ask_for_rating boolean NOT NULL DEFAULT false;

ALTER TABLE conversations
    ADD COLUMN closed_at           timestamptz,
    ADD COLUMN rating              text CHECK (rating IN ('good', 'bad')),
    ADD COLUMN rating_comment      text,
    ADD COLUMN rated_at            timestamptz,
    ADD COLUMN rating_requested_at timestamptz,
    ADD CHECK ((rating IS NULL) = (rated_at IS NULL));
UPDATE conversations SET closed_at = updated_at WHERE status = 'closed';
CREATE INDEX conversations_rated_idx ON conversations (workspace_id, rated_at) WHERE rated_at IS NOT NULL;

-- +goose Down
DROP INDEX conversations_rated_idx;
ALTER TABLE conversations
    DROP COLUMN rating_requested_at,
    DROP COLUMN rated_at,
    DROP COLUMN rating_comment,
    DROP COLUMN rating,
    DROP COLUMN closed_at;
ALTER TABLE inboxes DROP COLUMN ask_for_rating;
