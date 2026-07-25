ALTER TABLE messages
    ADD COLUMN updated_at  TIMESTAMPTZ,
    ADD COLUMN read_at     TIMESTAMPTZ,
    ADD COLUMN deleted_at  TIMESTAMPTZ,
    ADD COLUMN deleted_by  UUID REFERENCES users(id);