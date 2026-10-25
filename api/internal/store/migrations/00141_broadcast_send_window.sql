-- +goose Up
ALTER TABLE broadcasts
    ADD COLUMN timezone text NOT NULL DEFAULT 'UTC' CHECK (length(timezone) BETWEEN 1 AND 64),
    ADD COLUMN send_window_start smallint CHECK (send_window_start BETWEEN 0 AND 1439),
    ADD COLUMN send_window_end   smallint CHECK (send_window_end BETWEEN 1 AND 1440),
    ADD CONSTRAINT broadcasts_send_window_check CHECK (
        (send_window_start IS NULL) = (send_window_end IS NULL)
        AND (send_window_start IS NULL OR (
            send_window_start <> send_window_end % 1440
            AND (send_window_end - send_window_start + 1440) % 1440 >= 30)));

UPDATE broadcasts b SET timezone = i.timezone
FROM inboxes i
WHERE i.workspace_id = b.workspace_id AND i.id = b.inbox_id AND length(i.timezone) BETWEEN 1 AND 64;

-- +goose Down
ALTER TABLE broadcasts
    DROP CONSTRAINT broadcasts_send_window_check,
    DROP COLUMN send_window_end,
    DROP COLUMN send_window_start,
    DROP COLUMN timezone;
