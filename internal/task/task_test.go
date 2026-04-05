package task

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

// Run from project root: go test ./internal/task/... -v

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

	// Prepare all statements
	mock.ExpectPrepare("SELECT id, user_id, category_id, name, description, answer_type, is_active, created_at, updated_at")
	mock.ExpectPrepare("SELECT id, user_id, category_id, name, description, answer_type, is_active, created_at, updated_at")
	mock.ExpectPrepare("SELECT id, user_id, category_id, name, description, answer_type, is_active, created_at, updated_at")
	mock.ExpectPrepare("INSERT INTO tasks")
	mock.ExpectPrepare("UPDATE tasks SET name =")
	mock.ExpectPrepare("UPDATE tasks SET is_active =")
	mock.ExpectPrepare("SELECT EXISTS")
	mock.ExpectPrepare("INSERT INTO task_schedules")
	mock.ExpectPrepare("SELECT ts.id, ts.task_id, ts.recurrence_type")
	mock.ExpectPrepare("INSERT INTO task_select_options")
	mock.ExpectPrepare("SELECT tso.id, tso.task_id, tso.value")

	repo := NewTaskRepository(db, logger)
	service := NewTaskService(repo, logger)
	handler := NewTaskHandler(service, logger)

	return handler, mock, func() { _ = db.Close() }
}

const testUserID = "550e8400-e29b-41d4-a716-446655440000"
const testTaskID = "660e8400-e29b-41d4-a716-446655440001"
const testCategoryID = "770e8400-e29b-41d4-a716-446655440002"
const testScheduleID = "880e8400-e29b-41d4-a716-446655440003"

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

// ============================================================================
// TESTS
// ============================================================================

func TestTaskHandler(t *testing.T) {
	now := time.Now()
	description := "Test Description"
	startDate, _ := time.Parse("2006-01-02", "2025-12-28")

	// TEST 1: GET A TASK BY ID
	t.Run("Get Task By ID", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Get Task by ID")

		mock.ExpectQuery(`SELECT .+ FROM tasks WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testTaskID, testUserID).
			WillReturnRows(mockTaskRows().AddRow(
				testTaskID, testUserID, testCategoryID, "Morning Workout", &description, "boolean", true, now, now,
			))

		mock.ExpectQuery(`SELECT ts\.id, ts\.task_id`).
			WithArgs(testTaskID, testUserID).
			WillReturnRows(mockScheduleRows().AddRow(
				testScheduleID, testTaskID, "daily", nil, "{}",
				nil, nil, nil, nil, "{}",
				startDate, "never", nil, nil, now,
			))

		urlParams := map[string]string{"id": testTaskID}
		req := createRequest(http.MethodGet, fmt.Sprintf("/api/v1/tasks/%s", testTaskID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.GetTask(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response WithDetails
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Task.ID != testTaskID {
			t.Errorf("FAIL: Expected ID %s, got %s", testTaskID, response.Task.ID)
		} else {
			fmt.Println("PASS: Task ID matches")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 2: LIST TASKS
	t.Run("List Tasks", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: List Tasks")

		mock.ExpectQuery(`SELECT .+ FROM tasks WHERE user_id = \$1 AND is_active = \$2 ORDER BY name ASC LIMIT \$3 OFFSET \$4`).
			WithArgs(testUserID, true, 20, 0).
			WillReturnRows(mockTaskRows().
				AddRow(testTaskID, testUserID, testCategoryID, "Morning Workout", nil, "boolean", true, now, now).
				AddRow("uuid-2", testUserID, testCategoryID, "Read Book", &description, "integer", true, now, now))

		req := createRequest(http.MethodGet, "/api/v1/tasks", nil, nil, nil)
		rr := httptest.NewRecorder()

		handler.ListTasks(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response []Task
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if len(response) != 2 {
			t.Errorf("FAIL: Expected 2 tasks, got %d", len(response))
		} else {
			fmt.Printf("PASS: Got %d tasks\n", len(response))
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 3: UPDATE TASK
	t.Run("Update Task", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Update Task")

		updatedDesc := "Updated Description"
		mock.ExpectQuery(`UPDATE tasks SET name = \$1, description = \$2, updated_at = NOW\(\) WHERE id = \$3 AND user_id = \$4 RETURNING .+`).
			WithArgs("Evening Walk", &updatedDesc, testTaskID, testUserID).
			WillReturnRows(mockTaskRows().AddRow(
				testTaskID, testUserID, testCategoryID, "Evening Walk", &updatedDesc, "boolean", true, now, now,
			))

		taskReq := UpdateTaskRequest{Name: "Evening Walk", Description: &updatedDesc}
		reqBody, _ := json.Marshal(taskReq)

		urlParams := map[string]string{"id": testTaskID}
		req := createRequest(http.MethodPut, fmt.Sprintf("/api/v1/tasks/%s", testTaskID), reqBody, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.UpdateTask(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response Task
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Name != "Evening Walk" {
			t.Errorf("FAIL: Expected name to be 'Evening Walk', got %s", response.Name)
		} else {
			fmt.Println("PASS: Task updated successfully")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 4: DELETE TASK (DEACTIVATE)
	t.Run("Delete Task", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Delete Task")

		mock.ExpectExec(`UPDATE tasks SET is_active = false, updated_at = NOW\(\) WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testTaskID, testUserID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		urlParams := map[string]string{"id": testTaskID}
		req := createRequest(http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%s", testTaskID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.DeleteTask(rr, req)

		if status := rr.Code; status != http.StatusNoContent {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusNoContent, rr.Body.String())
		} else {
			fmt.Println("PASS: Task deleted successfully")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 5: TASK NOT FOUND
	t.Run("Get Non-Existent Task", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Get Non-Existent Task")

		nonExistentID := "00000000-0000-0000-0000-000000000000"
		mock.ExpectQuery(`SELECT .+ FROM tasks WHERE id = \$1 AND user_id = \$2`).
			WithArgs(nonExistentID, testUserID).
			WillReturnRows(mockTaskRows())

		urlParams := map[string]string{"id": nonExistentID}
		req := createRequest(http.MethodGet, "/api/v1/tasks/"+nonExistentID, nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.GetTask(rr, req)

		if status := rr.Code; status != http.StatusNotFound {
			t.Errorf("FAIL: Expected NotFound, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned NotFound for non-existent task")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 6: EMPTY TASK NAME
	t.Run("Empty Task Name", func(t *testing.T) {
		fmt.Println("Running Test: Empty Task Name")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		taskReq := UpdateTaskRequest{Name: ""}
		reqBody, _ := json.Marshal(taskReq)

		urlParams := map[string]string{"id": testTaskID}
		req := createRequest(http.MethodPut, "/api/v1/tasks/"+testTaskID, reqBody, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.UpdateTask(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for empty name, got %v. Body: %s", rr.Code, rr.Body.String())
		} else {
			fmt.Println("PASS: Rejected empty task name")
		}
	})

	// TEST 7: NAME TOO LONG
	t.Run("Name Too Long", func(t *testing.T) {
		fmt.Println("Running Test: Name Too Long")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		longName := make([]byte, 201)
		for i := range longName {
			longName[i] = 'a'
		}

		taskReq := UpdateTaskRequest{Name: string(longName)}
		reqBody, _ := json.Marshal(taskReq)

		urlParams := map[string]string{"id": testTaskID}
		req := createRequest(http.MethodPut, "/api/v1/tasks/"+testTaskID, reqBody, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.UpdateTask(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for too long name, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected too long name")
		}
	})

	// TEST 8: INVALID UUID
	t.Run("Invalid UUID", func(t *testing.T) {
		fmt.Println("Running Test: Invalid UUID")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		urlParams := map[string]string{"id": "not-a-uuid"}
		req := createRequest(http.MethodGet, "/api/v1/tasks/not-a-uuid", nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.GetTask(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for invalid UUID, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected invalid UUID")
		}
	})

	// TEST 9: INVALID CATEGORY ID UUID
	t.Run("Invalid Category ID UUID", func(t *testing.T) {
		fmt.Println("Running Test: Invalid Category ID UUID")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		queryParams := map[string]string{"category_id": "not-a-uuid"}
		req := createRequest(http.MethodGet, "/api/v1/tasks", nil, nil, queryParams)
		rr := httptest.NewRecorder()

		handler.ListTasks(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for invalid category_id UUID, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected invalid category_id UUID")
		}
	})

	// TEST 10: LIST INACTIVE TASKS
	t.Run("List Inactive Tasks", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: List Inactive Tasks")

		mock.ExpectQuery(`SELECT .+ FROM tasks WHERE user_id = \$1 AND is_active = \$2 ORDER BY name ASC LIMIT \$3 OFFSET \$4`).
			WithArgs(testUserID, false, 20, 0).
			WillReturnRows(mockTaskRows().
				AddRow(testTaskID, testUserID, testCategoryID, "Archived Task", nil, "string", false, now, now))

		queryParams := map[string]string{"active": "false"}
		req := createRequest(http.MethodGet, "/api/v1/tasks", nil, nil, queryParams)
		rr := httptest.NewRecorder()

		handler.ListTasks(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response []Task
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if len(response) != 1 {
			t.Errorf("FAIL: Expected 1 task, got %d", len(response))
		} else {
			fmt.Println("PASS: Got inactive tasks")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 11: LIST TASKS BY CATEGORY
	t.Run("List Tasks By Category", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: List Tasks By Category")

		mock.ExpectQuery(`SELECT .+ FROM tasks WHERE user_id = \$1 AND category_id = \$2 AND is_active = \$3 ORDER BY name ASC LIMIT \$4 OFFSET \$5`).
			WithArgs(testUserID, testCategoryID, true, 20, 0).
			WillReturnRows(mockTaskRows().
				AddRow(testTaskID, testUserID, testCategoryID, "Health Task", nil, "boolean", true, now, now))

		queryParams := map[string]string{"category_id": testCategoryID}
		req := createRequest(http.MethodGet, "/api/v1/tasks", nil, nil, queryParams)
		rr := httptest.NewRecorder()

		handler.ListTasks(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response []Task
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if len(response) != 1 {
			t.Errorf("FAIL: Expected 1 task, got %d", len(response))
		} else {
			fmt.Println("PASS: Got tasks by category")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})
}
