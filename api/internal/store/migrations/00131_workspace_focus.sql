-- +goose Up
ALTER TABLE workspaces ADD COLUMN focus text CHECK (focus IN ('support', 'mail', 'both'));

UPDATE workspaces w SET focus = CASE
    WHEN EXISTS (SELECT 1 FROM inboxes i WHERE i.workspace_id = w.id AND i.purpose = 'mail') THEN 'both'
    ELSE 'support'
END;

-- +goose Down
ALTER TABLE workspaces DROP COLUMN focus;
