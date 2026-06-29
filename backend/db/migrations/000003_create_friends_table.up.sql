CREATE TABLE friends (
    user_low_id  UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_high_id UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    requester_id UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status       TEXT        NOT NULL DEFAULT 'pending',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (user_low_id, user_high_id),

    CONSTRAINT friends_canonical_order CHECK (user_low_id < user_high_id),
    CONSTRAINT friends_status_valid    CHECK (status IN ('pending', 'accepted')),
    CONSTRAINT friends_requester_valid CHECK (requester_id = user_low_id OR requester_id = user_high_id)
);

CREATE INDEX idx_friends_user_high ON friends (user_high_id);

CREATE INDEX idx_friends_requester_pending ON friends (requester_id) WHERE status = 'pending';

CREATE TRIGGER set_friends_updated_at
    BEFORE UPDATE ON friends
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();