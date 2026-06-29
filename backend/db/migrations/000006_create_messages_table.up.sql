CREATE TABLE messages (
    id              UUID        NOT NULL,
    conversation_id UUID        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id       UUID        NOT NULL REFERENCES users(id),
    content         TEXT,
    reply_to_id     UUID        REFERENCES messages(id),
    metadata        JSONB,
    is_edited       BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id)
);

CREATE INDEX idx_messages_conversation_id ON messages (conversation_id, id DESC);