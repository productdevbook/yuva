-- +goose Up
ALTER TABLE broadcasts
    ADD COLUMN translations jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(translations) = 'object');

ALTER TABLE broadcast_deliveries
    ADD COLUMN locale text CHECK (length(locale) BETWEEN 2 AND 35);

-- '' is the only version of a design without translations, so links of broadcasts scheduled
-- before this migration keep resolving their clicks.
ALTER TABLE broadcast_links
    ADD COLUMN locale text NOT NULL DEFAULT '' CHECK (locale = '' OR length(locale) BETWEEN 2 AND 35),
    DROP CONSTRAINT broadcast_links_pkey,
    ADD PRIMARY KEY (workspace_id, broadcast_id, locale, position);

-- +goose Down
DELETE FROM broadcast_links WHERE locale <> '';
ALTER TABLE broadcast_links
    DROP CONSTRAINT broadcast_links_pkey,
    ADD PRIMARY KEY (workspace_id, broadcast_id, position),
    DROP COLUMN locale;
ALTER TABLE broadcast_deliveries DROP COLUMN locale;
ALTER TABLE broadcasts DROP COLUMN translations;
