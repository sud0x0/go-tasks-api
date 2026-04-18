-- +goose Up
-- +goose StatementBegin

-- Consolidated initial schema (combines migrations 00001-00010)
-- Last consolidated: April 2026

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- AUTHENTICATION TABLES
-- ============================================================================

-- Users table
CREATE TABLE users (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username   VARCHAR(50) NOT NULL UNIQUE,
    password   VARCHAR(255) NOT NULL,  -- Argon2id hash
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_username ON users(username);

CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Refresh tokens table
CREATE TABLE refresh_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL UNIQUE,  -- SHA-256 hash of the token
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens(expires_at);

-- ============================================================================
-- DOMAIN TABLES
-- ============================================================================

-- Categories table
CREATE TABLE categories (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    colour      CHAR(7)      NOT NULL DEFAULT '#808080',
    is_active   BOOLEAN      NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT categories_colour_format_chk CHECK (colour ~ '^#[0-9a-f]{6}$')
);

CREATE INDEX idx_categories_user_id ON categories(user_id);
CREATE UNIQUE INDEX idx_categories_user_lower_name_unique ON categories(user_id, lower(name));

CREATE TRIGGER update_categories_updated_at
    BEFORE UPDATE ON categories
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Tasks table
CREATE TABLE tasks (
    id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID          NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID          NOT NULL,
    name        VARCHAR(200)  NOT NULL,
    description VARCHAR(1000),
    answer_type VARCHAR(20)   NOT NULL,
    is_active   BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_answer_type CHECK (answer_type IN ('string', 'integer', 'boolean', 'select')),
    CONSTRAINT fk_tasks_category FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE
);

CREATE INDEX idx_tasks_user_id ON tasks(user_id);
CREATE INDEX idx_tasks_category_id ON tasks(category_id);
CREATE INDEX idx_tasks_user_active ON tasks(user_id, is_active);

CREATE TRIGGER update_tasks_updated_at
    BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Task select options table
-- Only valid for tasks where answer_type = 'select'
-- Options are fixed at task creation - to change options, deactivate the task and create a new one
CREATE TABLE task_select_options (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id    UUID         NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    value      VARCHAR(100) NOT NULL,
    position   SMALLINT     NOT NULL DEFAULT 0,  -- display order
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_select_options_task_id ON task_select_options(task_id);

-- Task schedules table
CREATE TABLE task_schedules (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id             UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    recurrence_type     VARCHAR(20) NOT NULL,
    recurrence_interval SMALLINT,           -- N for 'every_n_days', 'every_n_weeks'
    days_of_week        SMALLINT[],         -- 0=Sun,1=Mon...6=Sat for 'weekly', 'every_n_weeks'
    month_day           SMALLINT,           -- 1-31 for 'monthly_date', 'yearly'
    month_week          SMALLINT,           -- 1-5 for 'monthly_weekday' (which week)
    month_weekday       SMALLINT,           -- 0-6 for 'monthly_weekday' (which day of week)
    month_of_year       SMALLINT,           -- 1-12 for 'yearly'
    scheduled_times     TIME[],             -- specific times of day (e.g. {09:00, 13:00, 17:00})
                                            -- empty array means unscheduled (whole day)
    start_date          DATE        NOT NULL,
    end_type            VARCHAR(10) NOT NULL DEFAULT 'never',  -- 'never', 'on_date', 'after_n'
    end_date            DATE,               -- for end_type = 'on_date'
    end_after_n         SMALLINT,           -- for end_type = 'after_n'
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_recurrence_type CHECK (recurrence_type IN (
        'once', 'daily', 'every_n_days', 'weekly', 'every_n_weeks',
        'monthly_date', 'monthly_weekday', 'yearly'
    )),
    CONSTRAINT chk_end_type CHECK (end_type IN ('never', 'on_date', 'after_n'))
);

CREATE INDEX idx_task_schedules_task_id ON task_schedules(task_id);

-- Task occurrences table
-- Generated records representing each instance of a task on a specific day and time.
-- These are generated on demand (e.g. when a user loads their day view) and cached here.
-- This is the iCalendar/CalDAV materialised occurrence pattern.
CREATE TABLE task_occurrences (
    id              UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id         UUID    NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    schedule_id     UUID    NOT NULL REFERENCES task_schedules(id) ON DELETE CASCADE,
    user_id         UUID    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    occurrence_date DATE    NOT NULL,
    scheduled_time  TIME,   -- NULL if no specific time
    is_suppressed   BOOLEAN NOT NULL DEFAULT FALSE,  -- deleted for this day only
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_occurrences_task_id ON task_occurrences(task_id);
CREATE INDEX idx_task_occurrences_schedule_id ON task_occurrences(schedule_id);
CREATE INDEX idx_task_occurrences_user_id ON task_occurrences(user_id);
CREATE INDEX idx_task_occurrences_user_date ON task_occurrences(user_id, occurrence_date);

-- Per-task unique indexes for occurrence uniqueness
-- Timed occurrences: unique on (task_id, occurrence_date, scheduled_time) where scheduled_time IS NOT NULL
CREATE UNIQUE INDEX idx_task_occurrences_task_timed
    ON task_occurrences (task_id, occurrence_date, scheduled_time)
    WHERE scheduled_time IS NOT NULL;

-- Untimed occurrences: unique on (task_id, occurrence_date) where scheduled_time IS NULL
CREATE UNIQUE INDEX idx_task_occurrences_task_untimed
    ON task_occurrences (task_id, occurrence_date)
    WHERE scheduled_time IS NULL;

-- Task answers table
-- One answer per occurrence (enforced by unique constraint for upsert)
CREATE TABLE task_answers (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    occurrence_id  UUID        NOT NULL UNIQUE REFERENCES task_occurrences(id) ON DELETE CASCADE,
    user_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    answer_string  VARCHAR(500),   -- for answer_type = 'string'
    answer_integer INTEGER,        -- for answer_type = 'integer'
    answer_boolean BOOLEAN,        -- for answer_type = 'boolean'
    answer_select  UUID,           -- FK to task_select_options.id for answer_type = 'select'
    answered_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_single_answer CHECK (
        (CASE WHEN answer_string  IS NOT NULL THEN 1 ELSE 0 END +
         CASE WHEN answer_integer IS NOT NULL THEN 1 ELSE 0 END +
         CASE WHEN answer_boolean IS NOT NULL THEN 1 ELSE 0 END +
         CASE WHEN answer_select  IS NOT NULL THEN 1 ELSE 0 END) = 1
    ),
    CONSTRAINT fk_task_answers_select_option FOREIGN KEY (answer_select) REFERENCES task_select_options(id) ON DELETE SET NULL
);

CREATE INDEX idx_task_answers_occurrence_id ON task_answers(occurrence_id);
CREATE INDEX idx_task_answers_user_id ON task_answers(user_id);
CREATE INDEX idx_task_answers_answer_select ON task_answers(answer_select);

CREATE TRIGGER update_task_answers_updated_at
    BEFORE UPDATE ON task_answers
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Daily logs table
-- One journal entry per user per day
CREATE TABLE daily_logs (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    log_date   DATE        NOT NULL,
    entry      TEXT        NOT NULL,  -- max 10000 characters enforced at application level
    is_active  BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, log_date)
);

CREATE INDEX idx_daily_logs_user_id ON daily_logs(user_id);
CREATE INDEX idx_daily_logs_user_date ON daily_logs(user_id, log_date);
CREATE INDEX idx_daily_logs_user_active ON daily_logs(user_id, is_active);

CREATE TRIGGER update_daily_logs_updated_at
    BEFORE UPDATE ON daily_logs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop in reverse order of creation to respect foreign key constraints
DROP TRIGGER IF EXISTS update_daily_logs_updated_at ON daily_logs;
DROP TABLE IF EXISTS daily_logs CASCADE;

DROP TRIGGER IF EXISTS update_task_answers_updated_at ON task_answers;
DROP TABLE IF EXISTS task_answers CASCADE;

DROP TABLE IF EXISTS task_occurrences CASCADE;
DROP TABLE IF EXISTS task_schedules CASCADE;
DROP TABLE IF EXISTS task_select_options CASCADE;

DROP TRIGGER IF EXISTS update_tasks_updated_at ON tasks;
DROP TABLE IF EXISTS tasks CASCADE;

DROP TRIGGER IF EXISTS update_categories_updated_at ON categories;
DROP TABLE IF EXISTS categories CASCADE;

DROP TABLE IF EXISTS refresh_tokens CASCADE;

DROP TRIGGER IF EXISTS update_users_updated_at ON users;
DROP TABLE IF EXISTS users CASCADE;

DROP FUNCTION IF EXISTS update_updated_at_column();
DROP EXTENSION IF EXISTS "pgcrypto";

-- +goose StatementEnd
