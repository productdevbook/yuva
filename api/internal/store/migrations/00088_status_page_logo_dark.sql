-- +goose Up
ALTER TABLE status_pages ADD COLUMN logo_dark_url text;

-- +goose Down
ALTER TABLE status_pages DROP COLUMN logo_dark_url;
