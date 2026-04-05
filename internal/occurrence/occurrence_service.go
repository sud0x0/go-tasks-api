package occurrence

import (
	"context"
	"errors"
	"time"

	"go-tasks-api/internal/shared"
	"go-tasks-api/internal/task"
)

// Field limits.
const (
	maxAnswerStringLength = 500
)

// occurrenceService defines the interface for occurrence business logic.
type occurrenceService interface {
	getOccurrencesByDate(ctx context.Context, userID string, date time.Time) ([]WithDetails, error)
	getOccurrencesByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) ([]WithDetails, error)
	suppressOccurrence(ctx context.Context, id, userID string) error
	submitAnswer(ctx context.Context, id, userID string, req AnswerRequest) (TaskAnswer, error)
}

// defaultOccurrenceService implements occurrenceService.
type defaultOccurrenceService struct {
	repo   occurrenceRepository
	logger occurrenceLogger
}

// NewOccurrenceService creates a new occurrenceService.
func NewOccurrenceService(repo occurrenceRepository, log occurrenceLogger) *defaultOccurrenceService {
	return &defaultOccurrenceService{repo: repo, logger: log}
}

func (s *defaultOccurrenceService) getOccurrencesByDate(ctx context.Context, userID string, date time.Time) ([]WithDetails, error) {
	if userID == "" {
		return nil, ErrMissingParameters
	}

	// Get active schedules for this date
	schedules, err := s.repo.getActiveSchedulesByDate(ctx, userID, date)
	if err != nil {
		return nil, err
	}

	// Generate occurrences for each matching schedule
	for _, sched := range schedules {
		if s.scheduleMatchesDate(sched, date) {
			// Fetch count once before the time slot loop
			remainingSlots := -1 // -1 means unlimited
			if sched.EndType == task.EndTypeAfterN && sched.EndAfterN != nil {
				count, err := s.repo.countOccurrences(ctx, sched.ID, userID)
				if err != nil {
					return nil, err
				}
				remaining := *sched.EndAfterN - count
				if remaining <= 0 {
					continue // skip this schedule entirely
				}
				remainingSlots = remaining
			}

			if len(sched.ScheduledTimes) > 0 {
				// Create one occurrence per scheduled time
				for _, timeStr := range sched.ScheduledTimes {
					if remainingSlots == 0 {
						break // reached the limit mid-day
					}
					scheduledTime, err := time.Parse("15:04:05", timeStr)
					if err != nil {
						// Try parsing without seconds
						scheduledTime, err = time.Parse("15:04", timeStr)
						if err != nil {
							continue
						}
					}
					// Combine date with time
					fullTime := time.Date(date.Year(), date.Month(), date.Day(),
						scheduledTime.Hour(), scheduledTime.Minute(), scheduledTime.Second(), 0, time.UTC)
					if _, err := s.repo.upsertOccurrence(ctx, sched.TaskID, sched.ID, userID, date, &fullTime); err != nil {
						s.logger.LogError(errors.New("upsertOccurrence failed"), err)
						// Continue — do not abort the whole request for a single occurrence failure
					}
					if remainingSlots > 0 {
						remainingSlots--
					}
				}
			} else {
				// No specific times - create one occurrence for the whole day
				if remainingSlots != 0 {
					if _, err := s.repo.upsertOccurrence(ctx, sched.TaskID, sched.ID, userID, date, nil); err != nil {
						s.logger.LogError(errors.New("upsertOccurrence failed"), err)
						// Continue — do not abort the whole request for a single occurrence failure
					}
				}
			}
		}
	}

	// Now fetch all occurrences for this date
	occurrences, err := s.repo.getOccurrencesByDate(ctx, userID, date)
	if err != nil {
		return nil, err
	}

	return s.enrichOccurrences(ctx, occurrences)
}

func (s *defaultOccurrenceService) getOccurrencesByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) ([]WithDetails, error) {
	if userID == "" {
		return nil, ErrMissingParameters
	}

	if startDate.After(endDate) {
		return nil, ErrInvalidDateRange
	}

	// Fetch all schedules active within the range once
	schedules, err := s.repo.getActiveSchedulesForRange(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Generate occurrences for each day in the range
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		for _, sched := range schedules {
			if s.scheduleMatchesDate(sched, d) {
				// Fetch count once before the time slot loop
				remainingSlots := -1 // -1 means unlimited
				if sched.EndType == task.EndTypeAfterN && sched.EndAfterN != nil {
					count, err := s.repo.countOccurrences(ctx, sched.ID, userID)
					if err != nil {
						return nil, err
					}
					remaining := *sched.EndAfterN - count
					if remaining <= 0 {
						continue // skip this schedule entirely
					}
					remainingSlots = remaining
				}

				if len(sched.ScheduledTimes) > 0 {
					for _, timeStr := range sched.ScheduledTimes {
						if remainingSlots == 0 {
							break // reached the limit mid-day
						}
						scheduledTime, err := time.Parse("15:04:05", timeStr)
						if err != nil {
							scheduledTime, err = time.Parse("15:04", timeStr)
							if err != nil {
								continue
							}
						}
						fullTime := time.Date(d.Year(), d.Month(), d.Day(),
							scheduledTime.Hour(), scheduledTime.Minute(), scheduledTime.Second(), 0, time.UTC)
						if _, err := s.repo.upsertOccurrence(ctx, sched.TaskID, sched.ID, userID, d, &fullTime); err != nil {
							s.logger.LogError(errors.New("upsertOccurrence failed"), err)
							// Continue — do not abort the whole request for a single occurrence failure
						}
						if remainingSlots > 0 {
							remainingSlots--
						}
					}
				} else {
					// No specific times - create one occurrence for the whole day
					if remainingSlots != 0 {
						if _, err := s.repo.upsertOccurrence(ctx, sched.TaskID, sched.ID, userID, d, nil); err != nil {
							s.logger.LogError(errors.New("upsertOccurrence failed"), err)
							// Continue — do not abort the whole request for a single occurrence failure
						}
					}
				}
			}
		}
	}

	occurrences, err := s.repo.getOccurrencesByDateRange(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	return s.enrichOccurrences(ctx, occurrences)
}

func (s *defaultOccurrenceService) suppressOccurrence(ctx context.Context, id, userID string) error {
	if id == "" || userID == "" {
		return ErrMissingParameters
	}

	// Verify occurrence exists and belongs to user
	occ, err := s.repo.getOccurrence(ctx, id, userID)
	if err != nil {
		return err
	}

	if occ.IsSuppressed {
		return ErrOccurrenceAlreadySuppressed
	}

	return s.repo.suppressOccurrence(ctx, id, userID)
}

func (s *defaultOccurrenceService) submitAnswer(ctx context.Context, id, userID string, req AnswerRequest) (TaskAnswer, error) {
	if id == "" || userID == "" {
		return TaskAnswer{}, ErrMissingParameters
	}

	// Validate answer string length
	if req.AnswerString != nil && len(*req.AnswerString) > maxAnswerStringLength {
		return TaskAnswer{}, ErrAnswerStringTooLong
	}

	// Get the occurrence to verify it exists and get the task ID
	occ, err := s.repo.getOccurrence(ctx, id, userID)
	if err != nil {
		return TaskAnswer{}, err
	}

	// Get the task to verify answer type
	t, err := s.repo.getTask(ctx, occ.TaskID, userID)
	if err != nil {
		return TaskAnswer{}, err
	}

	// Validate answer matches task's answer type
	if err := s.validateAnswer(ctx, t, req); err != nil {
		return TaskAnswer{}, err
	}

	return s.repo.upsertAnswer(ctx, id, userID, req)
}

func (s *defaultOccurrenceService) validateAnswer(ctx context.Context, t task.Task, req AnswerRequest) error {
	switch t.AnswerType {
	case task.AnswerTypeString:
		if req.AnswerString == nil {
			return ErrInvalidAnswerType
		}
	case task.AnswerTypeInteger:
		if req.AnswerInteger == nil {
			return ErrInvalidAnswerType
		}
	case task.AnswerTypeBoolean:
		if req.AnswerBoolean == nil {
			return ErrInvalidAnswerType
		}
	case task.AnswerTypeSelect:
		if req.AnswerSelect == nil {
			return ErrInvalidAnswerType
		}
		if !shared.IsValidUUID(*req.AnswerSelect) {
			return ErrInvalidInput
		}
		// Verify the select option belongs to this task
		exists, err := s.repo.selectOptionExists(ctx, *req.AnswerSelect, t.ID, t.UserID)
		if err != nil {
			return err
		}
		if !exists {
			return ErrInvalidSelectOption
		}
	}
	return nil
}

func (s *defaultOccurrenceService) enrichOccurrences(ctx context.Context, occurrences []TaskOccurrence) ([]WithDetails, error) {
	if len(occurrences) == 0 {
		return []WithDetails{}, nil
	}

	result := make([]WithDetails, 0, len(occurrences))

	// Cache tasks and options to avoid repeated queries
	taskCache := make(map[string]task.Task)
	optionsCache := make(map[string][]task.SelectOption)

	// Collect all occurrence IDs and fetch answers in one query
	ids := make([]string, len(occurrences))
	for i, o := range occurrences {
		ids[i] = o.ID
	}

	answers, err := s.repo.getAnswersByOccurrenceIDs(ctx, ids, occurrences[0].UserID)
	if err != nil {
		return nil, err
	}

	for _, occ := range occurrences {
		detail := WithDetails{Occurrence: occ}

		// Get task (from cache if available)
		t, ok := taskCache[occ.TaskID]
		if !ok {
			var err error
			t, err = s.repo.getTask(ctx, occ.TaskID, occ.UserID)
			if err != nil {
				return nil, err
			}
			taskCache[occ.TaskID] = t
		}
		detail.Task = t

		// Get select options if needed (from cache if available)
		if t.AnswerType == task.AnswerTypeSelect {
			opts, ok := optionsCache[occ.TaskID]
			if !ok {
				var err error
				opts, err = s.repo.getSelectOptions(ctx, occ.TaskID, occ.UserID)
				if err != nil {
					return nil, err
				}
				optionsCache[occ.TaskID] = opts
			}
			detail.SelectOptions = opts
		}

		// Get answer from the pre-fetched map
		detail.Answer = answers[occ.ID]

		result = append(result, detail)
	}

	return result, nil
}

// scheduleMatchesDate checks if a schedule should generate an occurrence for the given date.
func (s *defaultOccurrenceService) scheduleMatchesDate(sched task.Schedule, date time.Time) bool {
	// Check if date is before start date
	if date.Before(sched.StartDate) {
		return false
	}

	// Check end conditions (after_n is handled separately in the generation loop)
	switch sched.EndType {
	case task.EndTypeOnDate:
		if sched.EndDate != nil && date.After(*sched.EndDate) {
			return false
		}
	case task.EndTypeAfterN:
		// Handled in the generation loop after checking recurrence match
	}

	switch sched.RecurrenceType {
	case task.RecurrenceOnce:
		// Only matches on the start date
		return date.Year() == sched.StartDate.Year() &&
			date.YearDay() == sched.StartDate.YearDay()

	case task.RecurrenceDaily:
		return true

	case task.RecurrenceEveryNDays:
		if sched.RecurrenceInterval == nil || *sched.RecurrenceInterval <= 0 {
			return false
		}
		daysDiff := int(date.Sub(sched.StartDate).Hours() / 24)
		return daysDiff >= 0 && daysDiff%*sched.RecurrenceInterval == 0

	case task.RecurrenceWeekly:
		weekday := int(date.Weekday())
		for _, d := range sched.DaysOfWeek {
			if int(d) == weekday {
				return true
			}
		}
		return false

	case task.RecurrenceEveryNWeeks:
		if sched.RecurrenceInterval == nil || *sched.RecurrenceInterval <= 0 {
			return false
		}
		// Check weekday match first
		weekday := int(date.Weekday())
		weekdayMatches := false
		for _, d := range sched.DaysOfWeek {
			if int(d) == weekday {
				weekdayMatches = true
				break
			}
		}
		if !weekdayMatches {
			return false
		}
		// Calculate weeks elapsed using days, not ISO week numbers
		daysDiff := int(date.Sub(sched.StartDate).Hours() / 24)
		if daysDiff < 0 {
			return false
		}
		weeksDiff := daysDiff / 7
		return weeksDiff%*sched.RecurrenceInterval == 0

	case task.RecurrenceMonthlyDate:
		if sched.MonthDay == nil {
			return false
		}
		return date.Day() == *sched.MonthDay

	case task.RecurrenceMonthlyWeekday:
		if sched.MonthWeek == nil || sched.MonthWeekday == nil {
			return false
		}
		// Check if the weekday matches
		if int(date.Weekday()) != *sched.MonthWeekday {
			return false
		}
		// Check which week of the month this is
		weekOfMonth := (date.Day()-1)/7 + 1
		return weekOfMonth == *sched.MonthWeek

	case task.RecurrenceYearly:
		if sched.MonthDay == nil || sched.MonthOfYear == nil {
			return false
		}
		return int(date.Month()) == *sched.MonthOfYear && date.Day() == *sched.MonthDay
	}

	return false
}
