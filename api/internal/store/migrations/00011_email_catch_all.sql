-- +goose Up
ALTER TABLE conversations
    ADD COLUMN email_address text CHECK (email_address = lower(email_address) AND length(email_address) BETWEEN 3 AND 320);

-- +goose Down
ALTER TABLE conversations DROP COLUMN email_address;
