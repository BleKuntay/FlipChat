-- Replace the plain index from 000009 with a UNIQUE PARTIAL index.
-- UNIQUE: enforces one attachment per message at DB level.
-- PARTIAL (WHERE ...IS NOT NULL): excludes text-only messages from the index.
DROP INDEX IF EXISTS idx_messages_attachment_id;

CREATE UNIQUE INDEX idx_messages_attachment_id
    ON messages ((metadata->>'attachment_id'))
    WHERE metadata->>'attachment_id' IS NOT NULL;