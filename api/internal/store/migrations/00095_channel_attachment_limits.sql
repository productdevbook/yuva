-- +goose Up
ALTER TABLE channels ADD COLUMN attachment_max_bytes bigint CHECK (attachment_max_bytes > 0);
ALTER TABLE channels ADD COLUMN attachment_types text[]
    CHECK (cardinality(attachment_types) BETWEEN 1 AND 100);

-- +goose Down
ALTER TABLE channels DROP COLUMN attachment_types;
ALTER TABLE channels DROP COLUMN attachment_max_bytes;
