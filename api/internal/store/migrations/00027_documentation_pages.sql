-- +goose Up
ALTER TABLE conversations
    DROP CONSTRAINT conversations_kind_check,
    ADD CONSTRAINT conversations_kind_check CHECK (kind IN ('conversation', 'feedback', 'question')),
    ADD COLUMN page_url   text CHECK (length(page_url) BETWEEN 1 AND 2000),
    ADD COLUMN page_title text CHECK (length(page_title) <= 300),
    ADD CONSTRAINT conversations_question_check CHECK (kind <> 'question' OR page_url IS NOT NULL);
CREATE INDEX conversations_page_idx ON conversations (workspace_id, inbox_id, page_url) WHERE page_url IS NOT NULL;

CREATE TABLE page_ratings (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    inbox_id     uuid NOT NULL,
    channel_id   uuid NOT NULL,
    page         text NOT NULL CHECK (length(page) BETWEEN 1 AND 2000),
    title        text NOT NULL DEFAULT '' CHECK (length(title) <= 300),
    day          date NOT NULL,
    up           integer NOT NULL DEFAULT 0 CHECK (up >= 0),
    down         integer NOT NULL DEFAULT 0 CHECK (down >= 0),
    PRIMARY KEY (workspace_id, channel_id, page, day),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, channel_id) REFERENCES channels (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX page_ratings_inbox_idx ON page_ratings (workspace_id, inbox_id, page, day);

CREATE TABLE page_answers (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    inbox_id        uuid NOT NULL,
    channel_id      uuid NOT NULL,
    page            text NOT NULL CHECK (length(page) BETWEEN 1 AND 2000),
    title           text NOT NULL DEFAULT '' CHECK (length(title) <= 300),
    question        text NOT NULL CHECK (length(question) BETWEEN 1 AND 2000),
    answer          text NOT NULL CHECK (length(answer) BETWEEN 1 AND 20000),
    member_id       uuid,
    conversation_id uuid,
    published_at    timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, channel_id) REFERENCES channels (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, member_id) REFERENCES members (workspace_id, id) ON DELETE SET NULL (member_id),
    FOREIGN KEY (workspace_id, conversation_id) REFERENCES conversations (workspace_id, id) ON DELETE SET NULL (conversation_id)
);
CREATE INDEX page_answers_page_idx ON page_answers (workspace_id, inbox_id, page, published_at DESC, id DESC);
CREATE INDEX page_answers_list_idx ON page_answers (workspace_id, published_at DESC, id DESC);
CREATE UNIQUE INDEX page_answers_conversation_idx ON page_answers (workspace_id, conversation_id) WHERE conversation_id IS NOT NULL;

-- +goose Down
DROP TABLE page_answers;
DROP TABLE page_ratings;
DROP INDEX conversations_page_idx;
UPDATE conversations SET kind = 'conversation' WHERE kind = 'question';
ALTER TABLE conversations
    DROP CONSTRAINT conversations_question_check,
    DROP COLUMN page_title,
    DROP COLUMN page_url,
    DROP CONSTRAINT conversations_kind_check,
    ADD CONSTRAINT conversations_kind_check CHECK (kind IN ('conversation', 'feedback'));
