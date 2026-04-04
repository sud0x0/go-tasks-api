package log

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-tasks-api/internal/shared/logger"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

// Run from project root: go test ./internal/log/... -v

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

	mock.ExpectPrepare("SELECT id, user_id, date_and_time, log, created_at, updated_at FROM logs WHERE id =")
	mock.ExpectPrepare("SELECT id, user_id, date_and_time, log, created_at, updated_at FROM logs WHERE user_id =")
	mock.ExpectPrepare("INSERT INTO logs")
	mock.ExpectPrepare("UPDATE logs SET log =")
	mock.ExpectPrepare("DELETE FROM logs")

	repo := NewLogRepository(db, logger)
	service := NewLogService(repo, logger)
	handler := NewLogHandler(service, logger)

	return handler, mock, func() { _ = db.Close() }
}

const testUserID = "550e8400-e29b-41d4-a716-446655440000"
const testLogID = "660e8400-e29b-41d4-a716-446655440001"

func createRequest(method, path string, body []byte, urlParams map[string]string, queryParams map[string]string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	ctx := context.WithValue(req.Context(), UserContextKey, testUserID)

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

func mockLogRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "user_id", "date_and_time", "log", "created_at", "updated_at",
	})
}

// ============================================================================
// TESTS
// ============================================================================

func TestLogHandler(t *testing.T) {
	testDateTime := "2025-12-28T10:30:00Z"
	now := time.Now()

	// NOTE: TEST 1: CREATE A LOG
	t.Run("Create a Log", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running test: Create Log")

		parsedTime, _ := time.Parse(time.RFC3339, testDateTime)

		mock.ExpectQuery(`INSERT INTO logs .+ RETURNING .+`).
			WithArgs(testUserID, testDateTime, "Test Log Entry").
			WillReturnRows(mockLogRows().AddRow(
				testLogID, testUserID, parsedTime, "Test Log Entry", now, now,
			))

		logReq := Request{DateAndTime: testDateTime, Log: "Test Log Entry"}
		reqBody, _ := json.Marshal(logReq)

		req := createRequest(http.MethodPost, "/api/v1/logs", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateLog(rr, req)

		if status := rr.Code; status != http.StatusCreated {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusCreated, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response Log
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Log != "Test Log Entry" {
			t.Errorf("FAIL: Expected log to be 'Test Log Entry', got %s", response.Log)
		} else {
			fmt.Println("PASS: Log content matches")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// NOTE: TEST 2: GET A LOG BY ID
	t.Run("Get Log By ID", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Get Log by ID")

		parsedTime, _ := time.Parse(time.RFC3339, testDateTime)

		mock.ExpectQuery(`SELECT .+ FROM logs WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testLogID, testUserID).
			WillReturnRows(mockLogRows().AddRow(
				testLogID, testUserID, parsedTime, "Test Log Entry", now, now,
			))

		urlParams := map[string]string{"id": testLogID}
		req := createRequest(http.MethodGet, fmt.Sprintf("/api/v1/logs/%s", testLogID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.GetLog(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response Log
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.ID != testLogID {
			t.Errorf("FAIL: Expected ID %s, got %s", testLogID, response.ID)
		} else {
			fmt.Println("PASS: Log ID matches")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// NOTE: TEST 3: UPDATE LOG
	t.Run("Update Log", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Update Log")

		parsedTime, _ := time.Parse(time.RFC3339, testDateTime)

		mock.ExpectQuery(`UPDATE logs SET log = \$1, updated_at = NOW\(\) WHERE id = \$2 AND user_id = \$3 RETURNING .+`).
			WithArgs("Updated Log Entry", testLogID, testUserID).
			WillReturnRows(mockLogRows().AddRow(
				testLogID, testUserID, parsedTime, "Updated Log Entry", now, now,
			))

		logReq := Request{DateAndTime: testDateTime, Log: "Updated Log Entry"}
		reqBody, _ := json.Marshal(logReq)

		urlParams := map[string]string{"id": testLogID}
		req := createRequest(http.MethodPut, fmt.Sprintf("/api/v1/logs/%s", testLogID), reqBody, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.UpdateLog(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response Log
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Log != "Updated Log Entry" {
			t.Errorf("FAIL: Expected log to be 'Updated Log Entry', got %s", response.Log)
		} else {
			fmt.Println("PASS: Log updated successfully")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// NOTE: TEST 4: LIST LOGS
	t.Run("List Logs", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: List Logs")

		parsedTime, _ := time.Parse(time.RFC3339, testDateTime)

		mock.ExpectQuery(`SELECT .+ FROM logs WHERE user_id = \$1 AND date_and_time >= \$2 AND date_and_time <= \$3 ORDER BY date_and_time DESC LIMIT \$4 OFFSET \$5`).
			WithArgs(testUserID, "2025-12-01T00:00:00Z", "2025-12-31T23:59:59Z", 20, 0).
			WillReturnRows(mockLogRows().
				AddRow(testLogID, testUserID, parsedTime, "Log 1", now, now).
				AddRow("uuid-2", testUserID, parsedTime, "Log 2", now, now))

		queryParams := map[string]string{
			"start_date": "2025-12-01T00:00:00Z",
			"end_date":   "2025-12-31T23:59:59Z",
		}

		req := createRequest(http.MethodGet, "/api/v1/logs", nil, nil, queryParams)
		rr := httptest.NewRecorder()

		handler.ListLogs(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response []Log
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if len(response) != 2 {
			t.Errorf("FAIL: Expected 2 logs, got %d", len(response))
		} else {
			fmt.Printf("PASS: Got %d logs\n", len(response))
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// NOTE: TEST 5: DELETE LOG
	t.Run("Delete Log", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Delete Log")

		mock.ExpectExec(`DELETE FROM logs WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testLogID, testUserID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		urlParams := map[string]string{"id": testLogID}
		req := createRequest(http.MethodDelete, fmt.Sprintf("/api/v1/logs/%s", testLogID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.DeleteLog(rr, req)

		if status := rr.Code; status != http.StatusNoContent {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusNoContent, rr.Body.String())
		} else {
			fmt.Println("PASS: Log deleted successfully")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// NOTE: TEST 6: XSS SANITISATION
	t.Run("XSS Sanitisation", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: XSS Sanitisation")

		parsedTime, _ := time.Parse(time.RFC3339, testDateTime)

		mock.ExpectQuery(`INSERT INTO logs .+ RETURNING .+`).
			WithArgs(testUserID, testDateTime, "Normal text").
			WillReturnRows(mockLogRows().AddRow(
				testLogID, testUserID, parsedTime, "Normal text", now, now,
			))

		logReq := Request{DateAndTime: testDateTime, Log: "<script>alert('XSS')</script>Normal text"}
		reqBody, _ := json.Marshal(logReq)

		req := createRequest(http.MethodPost, "/api/v1/logs", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateLog(rr, req)

		if status := rr.Code; status != http.StatusCreated {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusCreated, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response Log
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Log != "Normal text" {
			t.Errorf("FAIL: XSS not sanitised. Got: %s", response.Log)
		} else {
			fmt.Printf("PASS: XSS sanitised. Result: %s\n", response.Log)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// NOTE: TEST 7: INVALID DATE_AND_TIME FORMAT
	t.Run("Invalid DateAndTime Format", func(t *testing.T) {
		fmt.Println("Running Test: Invalid DateAndTime Format")

		invalidDateTimes := []string{
			"2025-12-28",           // date only, no time
			"28-12-2025T10:00:00Z", // wrong date order
			"not-a-date",
		}

		for _, badDT := range invalidDateTimes {
			t.Run(fmt.Sprintf("POST with date_and_time=%s", badDT), func(t *testing.T) {
				handler, _, cleanup := setupTestStack(t)
				defer cleanup()

				logReq := Request{DateAndTime: badDT, Log: "Test log"}
				reqBody, _ := json.Marshal(logReq)

				req := createRequest(http.MethodPost, "/api/v1/logs", reqBody, nil, nil)
				rr := httptest.NewRecorder()

				handler.CreateLog(rr, req)

				if rr.Code != http.StatusBadRequest {
					t.Errorf("Expected BadRequest for date_and_time '%s', got %v", badDT, rr.Code)
				}
			})
		}

		fmt.Println("PASS: All invalid date_and_time tests completed")
	})

	// NOTE: TEST 8: LOG NOT FOUND
	t.Run("Get Non-Existent Log", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Get Non-Existent Log")

		nonExistentID := "00000000-0000-0000-0000-000000000000"
		mock.ExpectQuery(`SELECT .+ FROM logs WHERE id = \$1 AND user_id = \$2`).
			WithArgs(nonExistentID, testUserID).
			WillReturnRows(mockLogRows())

		urlParams := map[string]string{"id": nonExistentID}
		req := createRequest(http.MethodGet, "/api/v1/logs/"+nonExistentID, nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.GetLog(rr, req)

		if status := rr.Code; status != http.StatusNotFound {
			t.Errorf("FAIL: Expected NotFound, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned NotFound for non-existent log")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// NOTE: TEST 9: EMPTY LOG CONTENT
	t.Run("Empty Log Content", func(t *testing.T) {
		fmt.Println("Running Test: Empty Log Content")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		logReq := Request{DateAndTime: testDateTime, Log: ""}
		reqBody, _ := json.Marshal(logReq)

		req := createRequest(http.MethodPost, "/api/v1/logs", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateLog(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for empty log, got %v. Body: %s", rr.Code, rr.Body.String())
		} else {
			fmt.Println("PASS: Rejected empty log content")
		}
	})

	// NOTE: TEST 10: LOG TOO LONG
	t.Run("Log Too Long", func(t *testing.T) {
		fmt.Println("Running Test: Log Too Long")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		longLog := make([]byte, 10001)
		for i := range longLog {
			longLog[i] = 'a'
		}

		logReq := Request{DateAndTime: testDateTime, Log: string(longLog)}
		reqBody, _ := json.Marshal(logReq)

		req := createRequest(http.MethodPost, "/api/v1/logs", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateLog(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for too long log, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected too long log")
		}

		var errResp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
			t.Errorf("Failed to parse error response as JSON: %v", err)
		}
		if errResp["error"] != "limit_exceeded" {
			t.Errorf("Expected error='limit_exceeded', got %v", errResp["error"])
		}
		fmt.Println("PASS: JSON error response format verified")
	})
}
