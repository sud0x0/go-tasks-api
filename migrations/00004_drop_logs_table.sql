-- +goose Up
-- +goose StatementBegin

-- Drop legacy logs table (no longer used)
DROP TRIGGER IF EXISTS update_logs_updated_at ON logs;
DROP TABLE IF EXISTS logs CASCADE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Recreate logs table if rolling back
CREATE TABLE logs (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL,
    date_and_time TIMESTAMPTZ NOT NULL,
    log          TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_logs_user_id        ON logs(user_id);
CREATE INDEX idx_logs_user_datetime  ON logs(user_id, date_and_time DESC);
CREATE INDEX idx_logs_datetime       ON logs(date_and_time DESC);

CREATE TRIGGER update_logs_updated_at
    BEFORE UPDATE ON logs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- +goose StatementEnd
