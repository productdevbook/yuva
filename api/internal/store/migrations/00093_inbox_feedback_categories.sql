-- +goose Up
-- Entries are {key, names: {locale: name}, icon}; the default is the four categories every inbox had before.
ALTER TABLE inboxes ADD COLUMN feedback_categories jsonb NOT NULL DEFAULT '[
  {"key": "bug", "names": {"en": "Bug", "tr": "Hata", "de": "Fehler"}, "icon": "bug"},
  {"key": "idea", "names": {"en": "Idea", "tr": "Fikir", "de": "Idee"}, "icon": "lightbulb"},
  {"key": "praise", "names": {"en": "Praise", "tr": "Övgü", "de": "Lob"}, "icon": "heart"},
  {"key": "other", "names": {"en": "Other", "tr": "Diğer", "de": "Sonstiges"}, "icon": "message-circle"}
]'::jsonb CHECK (jsonb_typeof(feedback_categories) = 'array');

-- +goose Down
ALTER TABLE inboxes DROP COLUMN feedback_categories;
