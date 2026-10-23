-- +goose Up
ALTER TABLE sessions ADD COLUMN user_agent text CHECK (length(user_agent) BETWEEN 1 AND 500);

-- Whether the authorization request named its redirect_uri: the token request must then repeat it.
ALTER TABLE oauth_requests ADD COLUMN redirect_uri_sent boolean NOT NULL DEFAULT false;
ALTER TABLE oauth_codes ADD COLUMN redirect_uri_sent boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE oauth_codes DROP COLUMN redirect_uri_sent;
ALTER TABLE oauth_requests DROP COLUMN redirect_uri_sent;
ALTER TABLE sessions DROP COLUMN user_agent;
