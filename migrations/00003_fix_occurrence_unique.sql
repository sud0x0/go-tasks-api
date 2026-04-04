-- +goose Up
-- Fix unique constraint for task_occurrences to handle NULL scheduled_time correctly.
-- PostgreSQL treats NULL != NULL in unique constraints, so we need partial indexes.

-- Drop the existing unique constraint
ALTER TABLE task_occurrences DROP CONSTRAINT IF EXISTS task_occurrences_schedule_id_occurrence_date_scheduled_time_key;

-- Delete duplicate untimed occurrences (keep the oldest one by id)
DELETE FROM task_occurrences a
USING task_occurrences b
WHERE a.schedule_id = b.schedule_id
  AND a.occurrence_date = b.occurrence_date
  AND a.scheduled_time IS NULL
  AND b.scheduled_time IS NULL
  AND a.id > b.id;

-- Delete duplicate timed occurrences (keep the oldest one by id)
DELETE FROM task_occurrences a
USING task_occurrences b
WHERE a.schedule_id = b.schedule_id
  AND a.occurrence_date = b.occurrence_date
  AND a.scheduled_time IS NOT NULL
  AND b.scheduled_time IS NOT NULL
  AND a.scheduled_time = b.scheduled_time
  AND a.id > b.id;

-- Create partial unique index for timed occurrences (scheduled_time IS NOT NULL)
CREATE UNIQUE INDEX idx_occurrences_unique_timed
    ON task_occurrences(schedule_id, occurrence_date, scheduled_time)
    WHERE scheduled_time IS NOT NULL;

-- Create partial unique index for untimed occurrences (scheduled_time IS NULL)
CREATE UNIQUE INDEX idx_occurrences_unique_untimed
    ON task_occurrences(schedule_id, occurrence_date)
    WHERE scheduled_time IS NULL;

-- +goose Down
-- Drop the partial indexes
DROP INDEX IF EXISTS idx_occurrences_unique_timed;
DROP INDEX IF EXISTS idx_occurrences_unique_untimed;

-- Restore the original unique constraint
ALTER TABLE task_occurrences ADD CONSTRAINT task_occurrences_schedule_id_occurrence_date_scheduled_time_key
    UNIQUE(schedule_id, occurrence_date, scheduled_time);
