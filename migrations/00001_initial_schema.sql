-- +goose Up
-- +goose StatementBegin

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Logs table
-- Allows multiple log entries per user with a timestamp.
-- Use id as the primary lookup key.
CREATE TABLE logs (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL,
    date_and_time TIMESTAMPTZ NOT NULL,
    log          TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_logs_user_id        ON logs(user_id);
CREATE INDEX idx_logs_user_datetime  ON logs(user_id, date_and_time DESC);
CREATE INDEX idx_logs_datetime       ON logs(date_and_time DESC);

-- updated_at trigger
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_logs_updated_at
    BEFORE UPDATE ON logs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS update_logs_updated_at ON logs;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP TABLE IF EXISTS logs CASCADE;
DROP EXTENSION IF EXISTS "pgcrypto";

-- +goose StatementEnd
