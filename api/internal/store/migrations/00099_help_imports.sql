-- +goose Up
-- An import of the articles of a Zendesk Guide or Help Scout Docs into a help center. A Zendesk import
-- reads the sign-in of a migration (migration_id); a Help Scout import holds its Docs API key sealed
-- under the master key (context "help-import:" + workspace + import) until it ends.
CREATE TABLE help_imports (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    center_id       uuid NOT NULL,
    source          text NOT NULL CHECK (source IN ('zendesk', 'helpscout')),
    migration_id    uuid,
    origin          text NOT NULL DEFAULT '' CHECK (length(origin) <= 253),
    sealed          bytea,
    options         jsonb NOT NULL DEFAULT '{}',
    state           text NOT NULL CHECK (state IN ('queued', 'running', 'done', 'failed', 'cancelled')),
    phase           text NOT NULL DEFAULT 'source' CHECK (phase IN ('source', 'collections', 'articles', 'links')),
    checkpoint      jsonb NOT NULL DEFAULT '{}',
    counts          jsonb NOT NULL DEFAULT '{}',
    skipped_locales text[] NOT NULL DEFAULT '{}',
    error           text CHECK (length(error) <= 2000),
    member_id       uuid,
    api_key_id      uuid,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    started_at      timestamptz,
    finished_at     timestamptz,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, center_id) REFERENCES help_centers (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, migration_id) REFERENCES migrations (workspace_id, id) ON DELETE SET NULL (migration_id),
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE SET NULL (member_id)
);
CREATE INDEX help_imports_center_idx ON help_imports (workspace_id, center_id, created_at DESC, id DESC);
CREATE UNIQUE INDEX help_imports_running_idx ON help_imports (workspace_id, center_id) WHERE state IN ('queued', 'running');

-- What an import wrote and where it came from, one row per article translation and per collection
-- translation. The key is what makes a second run update instead of duplicate: origin names the
-- account (the Zendesk host), source_id the item there. target_id is the article or the collection.
-- version is the draft's version right after the import wrote it and content_hash what it wrote, so
-- a draft edited since is told from an unchanged one. import_id is the import that last saw the row.
CREATE TABLE help_import_items (
    workspace_id  uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    center_id     uuid NOT NULL,
    origin        text NOT NULL CHECK (length(origin) BETWEEN 1 AND 253),
    kind          text NOT NULL CHECK (kind IN ('article', 'collection')),
    source_id     text NOT NULL CHECK (length(source_id) BETWEEN 1 AND 200),
    locale        text NOT NULL,
    link_key      text NOT NULL DEFAULT '' CHECK (length(link_key) <= 200),
    target_id     uuid NOT NULL,
    old_url       text NOT NULL DEFAULT '' CHECK (length(old_url) <= 2000),
    version       integer NOT NULL DEFAULT 0,
    content_hash  bytea,
    import_id     uuid NOT NULL,
    links_pending boolean NOT NULL DEFAULT false,
    updated_at    timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, center_id, origin, kind, source_id, locale),
    FOREIGN KEY (workspace_id, center_id) REFERENCES help_centers (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, import_id) REFERENCES help_imports (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX help_import_items_import_idx ON help_import_items (workspace_id, import_id, kind, source_id, locale);
CREATE INDEX help_import_items_link_idx ON help_import_items (workspace_id, center_id, origin, link_key) WHERE kind = 'article' AND link_key <> '';
CREATE INDEX help_import_items_url_idx ON help_import_items (workspace_id, center_id, old_url) WHERE old_url <> '';

-- +goose Down
DROP TABLE help_import_items;
DROP TABLE help_imports;
