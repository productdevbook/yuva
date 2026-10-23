-- +goose Up
-- app_push_send jobs queued before the preview moved to send time carry the reply text. River's own
-- tables may not exist yet on a new database.
-- +goose StatementBegin
DO $$ BEGIN
    IF to_regclass('river_job') IS NOT NULL THEN
        UPDATE river_job SET args = args - 'title' - 'body' WHERE kind = 'app_push_send' AND (args ? 'title' OR args ? 'body');
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
SELECT 1;
