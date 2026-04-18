package occurrence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-tasks-api/internal/auth"
	"go-tasks-api/internal/shared/logger"
	"go-tasks-api/internal/task"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

// Run from project root: go test ./internal/occurrence/... -v

// ============================================================================
// TEST HELPERS
// ============================================================================

type mockLogger struct{}

func (m *mockLogger) LogError(simplifiedError, actualError error)  {}
func (m *mockLogger) LogInfo(message string, args ...any)          {}
func (m *mockLogger) LogDebug(message string)                      {}
func (m *mockLogger) WithRequestID(requestID string) logger.Logger { return m }

func setupTestStack(t *testing.T) (*Handler, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}

	logger := &mockLogger{}

	// Prepare all statements in order from NewOccurrenceRepository
	mock.ExpectPrepare("SELECT o.id, o.task_id.*FROM task_occurrences o.*JOIN tasks t ON o.task_id = t.id.*JOIN categories c ON t.category_id = c.id.*WHERE o.id =")               // getOccurrence
	mock.ExpectPrepare("SELECT o.id, o.task_id.*FROM task_occurrences o.*JOIN tasks t ON o.task_id = t.id.*JOIN categories c ON t.category_id = c.id.*WHERE o.user_id =.*= \\$2")  // getOccurrencesByDate
	mock.ExpectPrepare("SELECT o.id, o.task_id.*FROM task_occurrences o.*JOIN tasks t ON o.task_id = t.id.*JOIN categories c ON t.category_id = c.id.*WHERE o.user_id =.*>= \\$2") // getOccurrencesByDateRange
	mock.ExpectPrepare("INSERT INTO task_occurrences.*ON CONFLICT \\(task_id, occurrence_date, scheduled_time\\)")                                                                 // upsertTimedOccurrence
	mock.ExpectPrepare("INSERT INTO task_occurrences.*ON CONFLICT \\(task_id, occurrence_date\\)")                                                                                 // upsertUntimedOccurrence
	mock.ExpectPrepare("UPDATE task_occurrences SET is_suppressed = true")                                                                                                         // suppressOccurrence
	mock.ExpectPrepare("UPDATE task_occurrences SET is_suppressed = false")                                                                                                        // unsuppressOccurrence
	mock.ExpectPrepare("SELECT COUNT")                                                                                                                                             // countOccurrences
	mock.ExpectPrepare("SELECT id, occurrence_id, user_id")                                                                                                                        // getAnswer
	mock.ExpectPrepare("INSERT INTO task_answers")                                                                                                                                 // upsertAnswer
	mock.ExpectPrepare("SELECT t.id, t.user_id.*FROM tasks t.*JOIN categories c ON t.category_id = c.id")                                                                          // getTask
	mock.ExpectPrepare("SELECT ts.id, ts.task_id.*FROM task_schedules ts.*JOIN tasks t ON ts.task_id = t.id.*JOIN categories c")                                                   // getActiveSchedulesByDate
	mock.ExpectPrepare("SELECT tso.id, tso.task_id, tso.value")                                                                                                                    // getSelectOptions
	mock.ExpectPrepare("SELECT EXISTS.*FROM task_select_options")                                                                                                                  // checkSelectOptionExists

	repo := NewOccurrenceRepository(db, logger)
	service := NewOccurrenceService(repo, logger)
	handler := NewOccurrenceHandler(service, logger)

	return handler, mock, func() { _ = db.Close() }
}

const testUserID = "550e8400-e29b-41d4-a716-446655440000"
const testOccurrenceID = "660e8400-e29b-41d4-a716-446655440001"
const testTaskID = "770e8400-e29b-41d4-a716-446655440002"
const testScheduleID = "880e8400-e29b-41d4-a716-446655440003"
const testCategoryID = "990e8400-e29b-41d4-a716-446655440004"

func createRequest(method, path string, body []byte, urlParams map[string]string, queryParams map[string]string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	ctx := context.WithValue(req.Context(), auth.UserContextKey, testUserID)

	if len(queryParams) > 0 {
		q := req.URL.Query()
		for key, value := range queryParams {
			q.Add(key, value)
		}
		req.URL.RawQuery = q.Encode()
	}

	if len(urlParams) > 0 {
		chiCtx := chi.NewRouteContext()
		for key, value := range urlParams {
			chiCtx.URLParams.Add(key, value)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, chiCtx)
	}

	return req.WithContext(ctx)
}

func mockOccurrenceRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "task_id", "schedule_id", "user_id", "occurrence_date", "scheduled_time", "is_suppressed", "created_at",
	})
}

func mockTaskRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "user_id", "category_id", "name", "description", "answer_type", "is_active", "created_at", "updated_at",
	})
}

func mockScheduleRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "task_id", "recurrence_type", "recurrence_interval", "days_of_week",
		"month_day", "month_week", "month_weekday", "month_of_year", "scheduled_times",
		"start_date", "end_type", "end_date", "end_after_n", "created_at",
	})
}

func mockAnswerRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "occurrence_id", "user_id", "answer_string", "answer_integer",
		"answer_boolean", "answer_select", "answered_at", "created_at", "updated_at",
	})
}

// ============================================================================
// TESTS
// ============================================================================

func TestOccurrenceHandler(t *testing.T) {
	now := time.Now()
	testDate := "2025-12-28"
	parsedDate, _ := time.Parse("2006-01-02", testDate)

	// TEST 1: LIST OCCURRENCES BY DATE (NO EXISTING OCCURRENCES)
	t.Run("List Occurrences By Date - Empty", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: List Occurrences By Date - Empty")

		// First, check for active schedules - return empty
		mock.ExpectQuery(`SELECT ts.id, ts.task_id.*FROM task_schedules ts.*JOIN tasks t ON ts.task_id = t.id.*JOIN categories c`).
			WithArgs(testUserID, parsedDate).
			WillReturnRows(mockScheduleRows())

		// Then get occurrences - return empty
		mock.ExpectQuery(`SELECT o.id, o.task_id.*FROM task_occurrences o.*JOIN tasks t.*JOIN categories c.*WHERE o.user_id = \$1 AND o.occurrence_date = \$2`).
			WithArgs(testUserID, parsedDate).
			WillReturnRows(mockOccurrenceRows())

		queryParams := map[string]string{"date": testDate}
		req := createRequest(http.MethodGet, "/api/v1/occurrences", nil, nil, queryParams)
		rr := httptest.NewRecorder()

		handler.ListOccurrences(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response []WithDetails
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if len(response) != 0 {
			t.Errorf("FAIL: Expected 0 occurrences, got %d", len(response))
		} else {
			fmt.Println("PASS: Got 0 occurrences as expected")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 2: SUPPRESS OCCURRENCE
	t.Run("Suppress Occurrence", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Suppress Occurrence")

		// First get the occurrence to verify it exists
		mock.ExpectQuery(`SELECT o.id, o.task_id.*FROM task_occurrences o.*JOIN tasks t.*JOIN categories c.*WHERE o.id = \$1 AND o.user_id = \$2`).
			WithArgs(testOccurrenceID, testUserID).
			WillReturnRows(mockOccurrenceRows().AddRow(
				testOccurrenceID, testTaskID, testScheduleID, testUserID, parsedDate, nil, false, now,
			))

		// Then suppress it
		mock.ExpectExec(`UPDATE task_occurrences SET is_suppressed = true WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testOccurrenceID, testUserID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		urlParams := map[string]string{"id": testOccurrenceID}
		req := createRequest(http.MethodPost, fmt.Sprintf("/api/v1/occurrences/%s/suppress", testOccurrenceID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.SuppressOccurrence(rr, req)

		if status := rr.Code; status != http.StatusNoContent {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusNoContent, rr.Body.String())
		} else {
			fmt.Println("PASS: Occurrence suppressed successfully")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 3: SUPPRESS ALREADY SUPPRESSED OCCURRENCE
	t.Run("Suppress Already Suppressed Occurrence", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Suppress Already Suppressed Occurrence")

		// Get the occurrence - it's already suppressed
		mock.ExpectQuery(`SELECT o.id, o.task_id.*FROM task_occurrences o.*JOIN tasks t.*JOIN categories c.*WHERE o.id = \$1 AND o.user_id = \$2`).
			WithArgs(testOccurrenceID, testUserID).
			WillReturnRows(mockOccurrenceRows().AddRow(
				testOccurrenceID, testTaskID, testScheduleID, testUserID, parsedDate, nil, true, now,
			))

		urlParams := map[string]string{"id": testOccurrenceID}
		req := createRequest(http.MethodPost, fmt.Sprintf("/api/v1/occurrences/%s/suppress", testOccurrenceID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.SuppressOccurrence(rr, req)

		if status := rr.Code; status != http.StatusConflict {
			t.Errorf("FAIL: Expected Conflict, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned Conflict for already suppressed occurrence")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 4: SUBMIT ANSWER - BOOLEAN
	t.Run("Submit Answer - Boolean", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Submit Answer - Boolean")

		trueVal := true

		// Get the occurrence
		mock.ExpectQuery(`SELECT o.id, o.task_id.*FROM task_occurrences o.*JOIN tasks t.*JOIN categories c.*WHERE o.id = \$1 AND o.user_id = \$2`).
			WithArgs(testOccurrenceID, testUserID).
			WillReturnRows(mockOccurrenceRows().AddRow(
				testOccurrenceID, testTaskID, testScheduleID, testUserID, parsedDate, nil, false, now,
			))

		// Get the task to verify answer type
		mock.ExpectQuery(`SELECT t.id, t.user_id.*FROM tasks t.*JOIN categories c ON t.category_id = c.id.*WHERE t.id = \$1 AND t.user_id = \$2`).
			WithArgs(testTaskID, testUserID).
			WillReturnRows(mockTaskRows().AddRow(
				testTaskID, testUserID, testCategoryID, "Morning Workout", nil, task.AnswerTypeBoolean, true, now, now,
			))

		// Upsert answer
		mock.ExpectQuery(`INSERT INTO task_answers .+ ON CONFLICT .+ RETURNING .+`).
			WithArgs(testOccurrenceID, testUserID, nil, nil, &trueVal, nil).
			WillReturnRows(mockAnswerRows().AddRow(
				"answer-id", testOccurrenceID, testUserID, nil, nil, &trueVal, nil, now, now, now,
			))

		answerReq := AnswerRequest{AnswerBoolean: &trueVal}
		reqBody, _ := json.Marshal(answerReq)

		urlParams := map[string]string{"id": testOccurrenceID}
		req := createRequest(http.MethodPost, fmt.Sprintf("/api/v1/occurrences/%s/answer", testOccurrenceID), reqBody, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.SubmitAnswer(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Answer submitted successfully")
		}

		var response TaskAnswer
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.AnswerBoolean == nil || *response.AnswerBoolean != true {
			t.Errorf("FAIL: Expected answer_boolean to be true")
		} else {
			fmt.Println("PASS: Answer value matches")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 5: INVALID DATE FORMAT
	t.Run("Invalid Date Format", func(t *testing.T) {
		fmt.Println("Running Test: Invalid Date Format")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		invalidDates := []string{
			"28-12-2025",
			"2025/12/28",
			"not-a-date",
		}

		for _, badDate := range invalidDates {
			t.Run(fmt.Sprintf("GET with date=%s", badDate), func(t *testing.T) {
				queryParams := map[string]string{"date": badDate}
				req := createRequest(http.MethodGet, "/api/v1/occurrences", nil, nil, queryParams)
				rr := httptest.NewRecorder()

				handler.ListOccurrences(rr, req)

				if rr.Code != http.StatusBadRequest {
					t.Errorf("Expected BadRequest for date '%s', got %v", badDate, rr.Code)
				}
			})
		}

		fmt.Println("PASS: All invalid date tests completed")
	})

	// TEST 6: MISSING DATE PARAMETER
	t.Run("Missing Date Parameter", func(t *testing.T) {
		fmt.Println("Running Test: Missing Date Parameter")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		// No date, start_date, or end_date provided
		req := createRequest(http.MethodGet, "/api/v1/occurrences", nil, nil, nil)
		rr := httptest.NewRecorder()

		handler.ListOccurrences(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for missing date params, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected request without date params")
		}
	})

	// TEST 7: INVALID UUID
	t.Run("Invalid UUID", func(t *testing.T) {
		fmt.Println("Running Test: Invalid UUID")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		urlParams := map[string]string{"id": "not-a-uuid"}
		req := createRequest(http.MethodPost, "/api/v1/occurrences/not-a-uuid/suppress", nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.SuppressOccurrence(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for invalid UUID, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected invalid UUID")
		}
	})

	// TEST 8: OCCURRENCE NOT FOUND
	t.Run("Occurrence Not Found", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Occurrence Not Found")

		nonExistentID := "00000000-0000-0000-0000-000000000000"
		mock.ExpectQuery(`SELECT o.id, o.task_id.*FROM task_occurrences o.*JOIN tasks t.*JOIN categories c.*WHERE o.id = \$1 AND o.user_id = \$2`).
			WithArgs(nonExistentID, testUserID).
			WillReturnRows(mockOccurrenceRows())

		urlParams := map[string]string{"id": nonExistentID}
		req := createRequest(http.MethodPost, fmt.Sprintf("/api/v1/occurrences/%s/suppress", nonExistentID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.SuppressOccurrence(rr, req)

		if status := rr.Code; status != http.StatusNotFound {
			t.Errorf("FAIL: Expected NotFound, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned NotFound")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 9: ANSWER STRING TOO LONG
	t.Run("Answer String Too Long", func(t *testing.T) {
		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Answer String Too Long")

		// Note: String length validation happens BEFORE any database queries
		// in the service, so no mock expectations are needed

		longString := make([]byte, 501)
		for i := range longString {
			longString[i] = 'a'
		}
		longStringVal := string(longString)

		answerReq := AnswerRequest{AnswerString: &longStringVal}
		reqBody, _ := json.Marshal(answerReq)

		urlParams := map[string]string{"id": testOccurrenceID}
		req := createRequest(http.MethodPost, fmt.Sprintf("/api/v1/occurrences/%s/answer", testOccurrenceID), reqBody, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.SubmitAnswer(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for too long answer_string, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected too long answer_string")
		}
	})

	// TEST 10: INVALID DATE RANGE
	t.Run("Invalid Date Range", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Invalid Date Range")

		// Get active schedules for the range - this happens before the date range check
		startDate := "2025-12-31"
		endDate := "2025-12-01"
		parsedStartDate, _ := time.Parse("2006-01-02", startDate)
		parsedEndDate, _ := time.Parse("2006-01-02", endDate)

		mock.ExpectQuery(`SELECT ts.id, ts.task_id.*FROM task_schedules ts.*JOIN tasks t ON ts.task_id = t.id.*JOIN categories c`).
			WithArgs(testUserID, parsedStartDate, parsedEndDate).
			WillReturnRows(mockScheduleRows())

		queryParams := map[string]string{
			"start_date": startDate,
			"end_date":   endDate,
		}
		req := createRequest(http.MethodGet, "/api/v1/occurrences", nil, nil, queryParams)
		rr := httptest.NewRecorder()

		handler.ListOccurrences(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for invalid date range, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected invalid date range")
		}
	})

	// TEST 11: BULK DELETE ANSWERS - SUCCESS
	t.Run("Bulk Delete Answers - Success", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Bulk Delete Answers - Success")

		occID1 := "11111111-1111-1111-1111-111111111111"
		occID2 := "22222222-2222-2222-2222-222222222222"

		// Use regexp matcher with AnyArg since sqlmock doesn't handle Go slice to Postgres array conversion
		mock.ExpectExec(`DELETE FROM task_answers`).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 2))

		reqBody, _ := json.Marshal(BulkDeleteAnswersRequest{OccurrenceIDs: []string{occID1, occID2}})
		req := createRequest(http.MethodPost, "/api/v1/occurrences/bulk-delete-answers", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.BulkDeleteAnswers(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response BulkDeleteAnswersResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Requested != 2 {
			t.Errorf("FAIL: Expected requested=2, got %d", response.Requested)
		} else {
			fmt.Println("PASS: Requested count matches")
		}

		if response.Deleted != 2 {
			t.Errorf("FAIL: Expected deleted=2, got %d", response.Deleted)
		} else {
			fmt.Println("PASS: Deleted count matches")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 12: BULK DELETE ANSWERS - EMPTY ID LIST
	t.Run("Bulk Delete Answers - Empty ID List", func(t *testing.T) {
		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Bulk Delete Answers - Empty ID List")

		reqBody, _ := json.Marshal(BulkDeleteAnswersRequest{OccurrenceIDs: []string{}})
		req := createRequest(http.MethodPost, "/api/v1/occurrences/bulk-delete-answers", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.BulkDeleteAnswers(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for empty ID list, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected empty ID list")
		}
	})

	// TEST 13: BULK DELETE ANSWERS - INVALID UUID
	t.Run("Bulk Delete Answers - Invalid UUID", func(t *testing.T) {
		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Bulk Delete Answers - Invalid UUID")

		reqBody, _ := json.Marshal(BulkDeleteAnswersRequest{OccurrenceIDs: []string{"not-a-uuid", "also-invalid"}})
		req := createRequest(http.MethodPost, "/api/v1/occurrences/bulk-delete-answers", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.BulkDeleteAnswers(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for invalid UUIDs, got %v. Body: %s", rr.Code, rr.Body.String())
		} else {
			fmt.Println("PASS: Rejected invalid UUIDs")
		}
	})

	// TEST 14: BULK DELETE ANSWERS - NO ANSWERS FOUND
	t.Run("Bulk Delete Answers - No Answers Found", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Bulk Delete Answers - No Answers Found")

		occID1 := "11111111-1111-1111-1111-111111111111"

		// Use regexp matcher with AnyArg since sqlmock doesn't handle Go slice to Postgres array conversion
		mock.ExpectExec(`DELETE FROM task_answers`).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 0))

		reqBody, _ := json.Marshal(BulkDeleteAnswersRequest{OccurrenceIDs: []string{occID1}})
		req := createRequest(http.MethodPost, "/api/v1/occurrences/bulk-delete-answers", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.BulkDeleteAnswers(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response BulkDeleteAnswersResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Requested != 1 {
			t.Errorf("FAIL: Expected requested=1, got %d", response.Requested)
		}

		if response.Deleted != 0 {
			t.Errorf("FAIL: Expected deleted=0, got %d", response.Deleted)
		} else {
			fmt.Println("PASS: Correctly reports 0 deleted")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})
}
