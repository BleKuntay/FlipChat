CREATE TABLE conversations (
    id           UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_low_id  UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_high_id UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    CONSTRAINT conversations_canonical_order CHECK (user_low_id < user_high_id),
    CONSTRAINT conversations_unique_pair     UNIQUE (user_low_id, user_high_id)
);

CREATE INDEX idx_conversations_user_low  ON conversations (user_low_id);
CREATE INDEX idx_conversations_user_high ON conversations (user_high_id);