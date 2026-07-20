CREATE INDEX idx_messages_attachment_id ON messages ((metadata->>'attachment_id'));
