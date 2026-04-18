package dailylog

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

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

// Run from project root: go test ./internal/dailylog/... -v

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

	mock.ExpectPrepare("SELECT id, user_id, log_date, entry, is_active, created_at, updated_at")
	mock.ExpectPrepare("SELECT id, user_id, log_date, entry, is_active, created_at, updated_at")
	mock.ExpectPrepare("SELECT id, user_id, log_date, entry, is_active, created_at, updated_at")
	mock.ExpectPrepare("SELECT id, user_id, log_date, entry, is_active, created_at, updated_at")
	mock.ExpectPrepare("INSERT INTO daily_logs")
	mock.ExpectPrepare("UPDATE daily_logs SET")
	mock.ExpectPrepare("UPDATE daily_logs SET is_active = false")
	mock.ExpectPrepare("DELETE FROM daily_logs")
	mock.ExpectPrepare("UPDATE daily_logs SET is_active = true")
	mock.ExpectPrepare("SELECT is_active FROM daily_logs")

	repo := NewDailyLogRepository(db, logger)
	service := NewDailyLogService(repo, logger)
	handler := NewDailyLogHandler(service, logger)

	return handler, mock, func() { _ = db.Close() }
}

const testUserID = "550e8400-e29b-41d4-a716-446655440000"
const testDailyLogID = "660e8400-e29b-41d4-a716-446655440001"

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

func mockDailyLogRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "user_id", "log_date", "entry", "is_active", "created_at", "updated_at",
	})
}

// ============================================================================
// TESTS
// ============================================================================

func TestDailyLogHandler(t *testing.T) {
	testDate := "2025-12-28"
	now := time.Now()
	parsedDate, _ := time.Parse("2006-01-02", testDate)

	// TEST 1: CREATE A DAILY LOG
	t.Run("Create a Daily Log", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running test: Create Daily Log")

		mock.ExpectQuery(`INSERT INTO daily_logs .+ RETURNING .+`).
			WithArgs(testUserID, parsedDate, "Today was productive").
			WillReturnRows(mockDailyLogRows().AddRow(
				testDailyLogID, testUserID, parsedDate, "Today was productive", true, now, now,
			))

		logReq := CreateRequest{LogDate: testDate, Entry: "Today was productive"}
		reqBody, _ := json.Marshal(logReq)

		req := createRequest(http.MethodPost, "/api/v1/daily-logs", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateDailyLog(rr, req)

		if status := rr.Code; status != http.StatusCreated {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusCreated, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response DailyLog
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Entry != "Today was productive" {
			t.Errorf("FAIL: Expected entry to be 'Today was productive', got %s", response.Entry)
		} else {
			fmt.Println("PASS: Daily log entry matches")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 2: GET DAILY LOG BY DATE
	t.Run("Get Daily Log By Date", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Get Daily Log by Date")

		mock.ExpectQuery(`SELECT .+ FROM daily_logs WHERE user_id = \$1 AND log_date = \$2`).
			WithArgs(testUserID, parsedDate).
			WillReturnRows(mockDailyLogRows().AddRow(
				testDailyLogID, testUserID, parsedDate, "Today was productive", true, now, now,
			))

		queryParams := map[string]string{"date": testDate}
		req := createRequest(http.MethodGet, "/api/v1/daily-logs", nil, nil, queryParams)
		rr := httptest.NewRecorder()

		handler.ListDailyLogs(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response []DailyLog
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if len(response) != 1 {
			t.Errorf("FAIL: Expected 1 log, got %d", len(response))
		} else {
			fmt.Println("PASS: Got 1 daily log")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 3: UPDATE DAILY LOG
	t.Run("Update Daily Log", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Update Daily Log")

		mock.ExpectQuery(`UPDATE daily_logs SET entry = \$1, updated_at = NOW\(\) WHERE id = \$2 AND user_id = \$3 .+ RETURNING .+`).
			WithArgs("Updated entry", testDailyLogID, testUserID).
			WillReturnRows(mockDailyLogRows().AddRow(
				testDailyLogID, testUserID, parsedDate, "Updated entry", true, now, now,
			))

		logReq := UpdateRequest{Entry: "Updated entry"}
		reqBody, _ := json.Marshal(logReq)

		urlParams := map[string]string{"id": testDailyLogID}
		req := createRequest(http.MethodPut, fmt.Sprintf("/api/v1/daily-logs/%s", testDailyLogID), reqBody, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.UpdateDailyLog(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response DailyLog
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Entry != "Updated entry" {
			t.Errorf("FAIL: Expected entry to be 'Updated entry', got %s", response.Entry)
		} else {
			fmt.Println("PASS: Daily log updated successfully")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 4: LIST DAILY LOGS BY DATE RANGE
	t.Run("List Daily Logs By Date Range", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: List Daily Logs By Date Range")

		startDate := "2025-12-01"
		endDate := "2025-12-31"
		parsedStartDate, _ := time.Parse("2006-01-02", startDate)
		parsedEndDate, _ := time.Parse("2006-01-02", endDate)

		mock.ExpectQuery(`SELECT .+ FROM daily_logs WHERE user_id = \$1 AND log_date >= \$2 AND log_date <= \$3`).
			WithArgs(testUserID, parsedStartDate, parsedEndDate).
			WillReturnRows(mockDailyLogRows().
				AddRow(testDailyLogID, testUserID, parsedDate, "Log 1", true, now, now).
				AddRow("uuid-2", testUserID, parsedDate, "Log 2", true, now, now))

		queryParams := map[string]string{
			"start_date": startDate,
			"end_date":   endDate,
		}

		req := createRequest(http.MethodGet, "/api/v1/daily-logs", nil, nil, queryParams)
		rr := httptest.NewRecorder()

		handler.ListDailyLogs(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response []DailyLog
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if len(response) != 2 {
			t.Errorf("FAIL: Expected 2 logs, got %d", len(response))
		} else {
			fmt.Printf("PASS: Got %d daily logs\n", len(response))
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 5: XSS SANITISATION
	t.Run("XSS Sanitisation", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: XSS Sanitisation")

		mock.ExpectQuery(`INSERT INTO daily_logs .+ RETURNING .+`).
			WithArgs(testUserID, parsedDate, "Normal entry").
			WillReturnRows(mockDailyLogRows().AddRow(
				testDailyLogID, testUserID, parsedDate, "Normal entry", true, now, now,
			))

		logReq := CreateRequest{LogDate: testDate, Entry: "<script>alert('XSS')</script>Normal entry"}
		reqBody, _ := json.Marshal(logReq)

		req := createRequest(http.MethodPost, "/api/v1/daily-logs", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateDailyLog(rr, req)

		if status := rr.Code; status != http.StatusCreated {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusCreated, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response DailyLog
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Entry != "Normal entry" {
			t.Errorf("FAIL: XSS not sanitised. Got: %s", response.Entry)
		} else {
			fmt.Printf("PASS: XSS sanitised. Result: %s\n", response.Entry)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 6: INVALID DATE FORMAT
	t.Run("Invalid Date Format", func(t *testing.T) {
		fmt.Println("Running Test: Invalid Date Format")

		invalidDates := []string{
			"28-12-2025", // wrong format
			"2025/12/28", // wrong separator
			"not-a-date", // garbage
			"2025-13-01", // invalid month
			"2025-12-32", // invalid day
		}

		for _, badDate := range invalidDates {
			t.Run(fmt.Sprintf("POST with log_date=%s", badDate), func(t *testing.T) {
				handler, _, cleanup := setupTestStack(t)
				defer cleanup()

				logReq := CreateRequest{LogDate: badDate, Entry: "Test entry"}
				reqBody, _ := json.Marshal(logReq)

				req := createRequest(http.MethodPost, "/api/v1/daily-logs", reqBody, nil, nil)
				rr := httptest.NewRecorder()

				handler.CreateDailyLog(rr, req)

				if rr.Code != http.StatusBadRequest {
					t.Errorf("Expected BadRequest for log_date '%s', got %v", badDate, rr.Code)
				}
			})
		}

		fmt.Println("PASS: All invalid date tests completed")
	})

	// TEST 7: DAILY LOG NOT FOUND ON DATE
	t.Run("Get Non-Existent Daily Log By Date", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Get Non-Existent Daily Log By Date")

		mock.ExpectQuery(`SELECT .+ FROM daily_logs WHERE user_id = \$1 AND log_date = \$2`).
			WithArgs(testUserID, parsedDate).
			WillReturnRows(mockDailyLogRows())

		queryParams := map[string]string{"date": testDate}
		req := createRequest(http.MethodGet, "/api/v1/daily-logs", nil, nil, queryParams)
		rr := httptest.NewRecorder()

		handler.ListDailyLogs(rr, req)

		// Should return empty array, not 404
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Expected OK with empty array, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned OK with empty array")
		}

		var response []DailyLog
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if len(response) != 0 {
			t.Errorf("FAIL: Expected 0 logs, got %d", len(response))
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 8: EMPTY ENTRY
	t.Run("Empty Entry", func(t *testing.T) {
		fmt.Println("Running Test: Empty Entry")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		logReq := CreateRequest{LogDate: testDate, Entry: ""}
		reqBody, _ := json.Marshal(logReq)

		req := createRequest(http.MethodPost, "/api/v1/daily-logs", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateDailyLog(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for empty entry, got %v. Body: %s", rr.Code, rr.Body.String())
		} else {
			fmt.Println("PASS: Rejected empty entry")
		}
	})

	// TEST 9: ENTRY TOO LONG
	t.Run("Entry Too Long", func(t *testing.T) {
		fmt.Println("Running Test: Entry Too Long")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		longEntry := make([]byte, 10001)
		for i := range longEntry {
			longEntry[i] = 'a'
		}

		logReq := CreateRequest{LogDate: testDate, Entry: string(longEntry)}
		reqBody, _ := json.Marshal(logReq)

		req := createRequest(http.MethodPost, "/api/v1/daily-logs", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateDailyLog(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for too long entry, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected too long entry")
		}
	})

	// TEST 10: INVALID UUID
	t.Run("Invalid UUID", func(t *testing.T) {
		fmt.Println("Running Test: Invalid UUID")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		logReq := UpdateRequest{Entry: "Updated entry"}
		reqBody, _ := json.Marshal(logReq)

		urlParams := map[string]string{"id": "not-a-uuid"}
		req := createRequest(http.MethodPut, "/api/v1/daily-logs/not-a-uuid", reqBody, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.UpdateDailyLog(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for invalid UUID, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected invalid UUID")
		}
	})

	// =========================================================================
	// SINGLE DELETE TESTS
	// =========================================================================

	// TEST 11: DELETE AN EXISTING DAILY LOG
	t.Run("Delete Existing Daily Log", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Delete Existing Daily Log")

		// Check is_active status first - returns true (active)
		mock.ExpectQuery(`SELECT is_active FROM daily_logs WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testDailyLogID, testUserID).
			WillReturnRows(sqlmock.NewRows([]string{"is_active"}).AddRow(true))

		// Then deactivate the daily log
		mock.ExpectExec(`UPDATE daily_logs SET is_active = false`).
			WithArgs(testDailyLogID, testUserID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		urlParams := map[string]string{"id": testDailyLogID}
		req := createRequest(http.MethodDelete, fmt.Sprintf("/api/v1/daily-logs/%s", testDailyLogID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.DeleteDailyLog(rr, req)

		if status := rr.Code; status != http.StatusNoContent {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusNoContent, rr.Body.String())
		} else {
			fmt.Println("PASS: Daily log deleted successfully")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 12: DELETE NON-EXISTENT DAILY LOG
	t.Run("Delete Non-Existent Daily Log", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Delete Non-Existent Daily Log")

		nonExistentID := "00000000-0000-0000-0000-000000000000"

		// Check is_active returns no rows for non-existent log
		mock.ExpectQuery(`SELECT is_active FROM daily_logs WHERE id = \$1 AND user_id = \$2`).
			WithArgs(nonExistentID, testUserID).
			WillReturnRows(sqlmock.NewRows([]string{"is_active"})) // Empty result

		urlParams := map[string]string{"id": nonExistentID}
		req := createRequest(http.MethodDelete, fmt.Sprintf("/api/v1/daily-logs/%s", nonExistentID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.DeleteDailyLog(rr, req)

		if status := rr.Code; status != http.StatusNotFound {
			t.Errorf("FAIL: Expected NotFound, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned NotFound for non-existent log")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 13: DELETE WITH INVALID UUID
	t.Run("Delete with Invalid UUID", func(t *testing.T) {
		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Delete with Invalid UUID")

		urlParams := map[string]string{"id": "not-a-uuid"}
		req := createRequest(http.MethodDelete, "/api/v1/daily-logs/not-a-uuid", nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.DeleteDailyLog(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for invalid UUID, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected invalid UUID")
		}
	})

	// TEST 14: DELETE WITHOUT AUTH
	t.Run("Delete Without Auth", func(t *testing.T) {
		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Delete Without Auth")

		// Create request without auth context
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/daily-logs/"+testDailyLogID, nil)
		chiCtx := chi.NewRouteContext()
		chiCtx.URLParams.Add("id", testDailyLogID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
		rr := httptest.NewRecorder()

		handler.DeleteDailyLog(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected Unauthorized, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected unauthenticated request")
		}
	})

	// =========================================================================
	// BULK DELETE TESTS
	// =========================================================================

	// TEST 15: BULK DELETE DAILY LOGS
	// Note: Skipped because sqlmock doesn't properly handle pgx array arguments.
	// Bulk delete operations should be tested via integration tests.
	t.Run("Bulk Delete Daily Logs", func(t *testing.T) {
		t.Skip("Skipped: sqlmock doesn't support pgx array arguments - test via integration tests")
	})

	// TEST 16: BULK DELETE WITH DUPLICATES
	// Note: Skipped because sqlmock doesn't properly handle pgx array arguments.
	t.Run("Bulk Delete with Duplicates", func(t *testing.T) {
		t.Skip("Skipped: sqlmock doesn't support pgx array arguments - test via integration tests")
	})

	// TEST 17: BULK DELETE EMPTY LIST
	t.Run("Bulk Delete Empty List", func(t *testing.T) {
		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Bulk Delete Empty List")

		bulkReq := BulkDeleteRequest{IDs: []string{}}
		reqBody, _ := json.Marshal(bulkReq)

		req := createRequest(http.MethodDelete, "/api/v1/daily-logs", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.BulkDeleteDailyLogs(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for empty list, got %v. Body: %s", rr.Code, rr.Body.String())
		} else {
			fmt.Println("PASS: Rejected empty list")
		}
	})

	// TEST 18: BULK DELETE TOO MANY IDS
	t.Run("Bulk Delete Too Many IDs", func(t *testing.T) {
		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Bulk Delete Too Many IDs")

		ids := make([]string, 101)
		for i := range ids {
			ids[i] = fmt.Sprintf("660e8400-e29b-41d4-a716-4466554400%02d", i)
		}

		bulkReq := BulkDeleteRequest{IDs: ids}
		reqBody, _ := json.Marshal(bulkReq)

		req := createRequest(http.MethodDelete, "/api/v1/daily-logs", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.BulkDeleteDailyLogs(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for too many IDs, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected too many IDs")
		}
	})

	// TEST 19: BULK DELETE WITH INVALID UUID
	t.Run("Bulk Delete with Invalid UUID", func(t *testing.T) {
		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Bulk Delete with Invalid UUID")

		ids := []string{
			"660e8400-e29b-41d4-a716-446655440001",
			"not-a-uuid",
		}

		bulkReq := BulkDeleteRequest{IDs: ids}
		reqBody, _ := json.Marshal(bulkReq)

		req := createRequest(http.MethodDelete, "/api/v1/daily-logs", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.BulkDeleteDailyLogs(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for invalid UUID, got %v. Body: %s", rr.Code, rr.Body.String())
		} else {
			fmt.Println("PASS: Rejected invalid UUID in bulk delete")
		}
	})

	// TEST 20: BULK DELETE WITHOUT AUTH
	t.Run("Bulk Delete Without Auth", func(t *testing.T) {
		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Bulk Delete Without Auth")

		bulkReq := BulkDeleteRequest{IDs: []string{"660e8400-e29b-41d4-a716-446655440001"}}
		reqBody, _ := json.Marshal(bulkReq)

		// Create request without auth context
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/daily-logs", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.BulkDeleteDailyLogs(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected Unauthorized, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected unauthenticated bulk delete")
		}
	})

	// TEST 21: BULK DELETE INVALID JSON
	t.Run("Bulk Delete Invalid JSON", func(t *testing.T) {
		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Bulk Delete Invalid JSON")

		req := createRequest(http.MethodDelete, "/api/v1/daily-logs", []byte("not valid json"), nil, nil)
		rr := httptest.NewRecorder()

		handler.BulkDeleteDailyLogs(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for invalid JSON, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected invalid JSON")
		}
	})

	// TEST 22: BULK DELETE MISSING IDS FIELD
	t.Run("Bulk Delete Missing IDs Field", func(t *testing.T) {
		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Bulk Delete Missing IDs Field")

		req := createRequest(http.MethodDelete, "/api/v1/daily-logs", []byte(`{}`), nil, nil)
		rr := httptest.NewRecorder()

		handler.BulkDeleteDailyLogs(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for missing ids field, got %v. Body: %s", rr.Code, rr.Body.String())
		} else {
			fmt.Println("PASS: Rejected missing ids field")
		}
	})
}
