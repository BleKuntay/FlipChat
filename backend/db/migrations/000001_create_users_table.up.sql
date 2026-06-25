CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS users (
    id          UUID                     PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        VARCHAR(100)             NOT NULL,
    username    VARCHAR(50)              NOT NULL UNIQUE,
    bio         TEXT,
    email       VARCHAR(255)             NOT NULL UNIQUE,
    password    VARCHAR(255)             NOT NULL,
    language    VARCHAR(10)              NOT NULL DEFAULT 'en',
    avatar_url  TEXT,
    last_seen_at TIMESTAMP WITH TIME ZONE,
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
    );

CREATE INDEX idx_users_username ON users (username);
CREATE INDEX idx_users_email ON users (email);

CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();