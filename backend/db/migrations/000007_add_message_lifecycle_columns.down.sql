ALTER TABLE messages
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS read_at,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS deleted_by;