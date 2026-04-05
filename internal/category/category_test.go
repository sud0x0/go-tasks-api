package category

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

// Run from project root: go test ./internal/category/... -v

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

	mock.ExpectPrepare("SELECT id, user_id, name, description, created_at, updated_at FROM categories WHERE id =")
	mock.ExpectPrepare("SELECT id, user_id, name, description, created_at, updated_at FROM categories WHERE user_id =")
	mock.ExpectPrepare("INSERT INTO categories")
	mock.ExpectPrepare("UPDATE categories SET")
	mock.ExpectPrepare("DELETE FROM categories")
	mock.ExpectPrepare("SELECT EXISTS")

	repo := NewCategoryRepository(db, logger)
	service := NewCategoryService(repo, logger)
	handler := NewCategoryHandler(service, logger)

	return handler, mock, func() { _ = db.Close() }
}

const testUserID = "550e8400-e29b-41d4-a716-446655440000"
const testCategoryID = "660e8400-e29b-41d4-a716-446655440001"

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

func mockCategoryRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "user_id", "name", "description", "created_at", "updated_at",
	})
}

// ============================================================================
// TESTS
// ============================================================================

func TestCategoryHandler(t *testing.T) {
	now := time.Now()
	description := "Test Description"

	// TEST 1: CREATE A CATEGORY
	t.Run("Create a Category", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running test: Create Category")

		mock.ExpectQuery(`INSERT INTO categories .+ RETURNING .+`).
			WithArgs(testUserID, "Work", &description).
			WillReturnRows(mockCategoryRows().AddRow(
				testCategoryID, testUserID, "Work", &description, now, now,
			))

		catReq := CreateRequest{Name: "Work", Description: &description}
		reqBody, _ := json.Marshal(catReq)

		req := createRequest(http.MethodPost, "/api/v1/categories", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateCategory(rr, req)

		if status := rr.Code; status != http.StatusCreated {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusCreated, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response Category
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Name != "Work" {
			t.Errorf("FAIL: Expected name to be 'Work', got %s", response.Name)
		} else {
			fmt.Println("PASS: Category name matches")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 2: GET A CATEGORY BY ID
	t.Run("Get Category By ID", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Get Category by ID")

		mock.ExpectQuery(`SELECT .+ FROM categories WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testCategoryID, testUserID).
			WillReturnRows(mockCategoryRows().AddRow(
				testCategoryID, testUserID, "Work", &description, now, now,
			))

		urlParams := map[string]string{"id": testCategoryID}
		req := createRequest(http.MethodGet, fmt.Sprintf("/api/v1/categories/%s", testCategoryID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.GetCategory(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response Category
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.ID != testCategoryID {
			t.Errorf("FAIL: Expected ID %s, got %s", testCategoryID, response.ID)
		} else {
			fmt.Println("PASS: Category ID matches")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 3: UPDATE CATEGORY
	t.Run("Update Category", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Update Category")

		updatedDesc := "Updated Description"
		mock.ExpectQuery(`UPDATE categories SET name = \$1, description = \$2, updated_at = NOW\(\) WHERE id = \$3 AND user_id = \$4 RETURNING .+`).
			WithArgs("Personal", &updatedDesc, testCategoryID, testUserID).
			WillReturnRows(mockCategoryRows().AddRow(
				testCategoryID, testUserID, "Personal", &updatedDesc, now, now,
			))

		catReq := UpdateRequest{Name: "Personal", Description: &updatedDesc}
		reqBody, _ := json.Marshal(catReq)

		urlParams := map[string]string{"id": testCategoryID}
		req := createRequest(http.MethodPut, fmt.Sprintf("/api/v1/categories/%s", testCategoryID), reqBody, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.UpdateCategory(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response Category
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Name != "Personal" {
			t.Errorf("FAIL: Expected name to be 'Personal', got %s", response.Name)
		} else {
			fmt.Println("PASS: Category updated successfully")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 4: LIST CATEGORIES
	t.Run("List Categories", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: List Categories")

		mock.ExpectQuery(`SELECT .+ FROM categories WHERE user_id = \$1 ORDER BY name ASC LIMIT \$2 OFFSET \$3`).
			WithArgs(testUserID, 20, 0).
			WillReturnRows(mockCategoryRows().
				AddRow(testCategoryID, testUserID, "Personal", nil, now, now).
				AddRow("uuid-2", testUserID, "Work", &description, now, now))

		req := createRequest(http.MethodGet, "/api/v1/categories", nil, nil, nil)
		rr := httptest.NewRecorder()

		handler.ListCategories(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response []Category
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if len(response) != 2 {
			t.Errorf("FAIL: Expected 2 categories, got %d", len(response))
		} else {
			fmt.Printf("PASS: Got %d categories\n", len(response))
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 5: DELETE CATEGORY
	t.Run("Delete Category", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Delete Category")

		// First verify category exists and ownership
		mock.ExpectQuery(`SELECT .+ FROM categories WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testCategoryID, testUserID).
			WillReturnRows(mockCategoryRows().AddRow(
				testCategoryID, testUserID, "Work", &description, now, now,
			))

		// Check for active tasks
		mock.ExpectQuery(`SELECT EXISTS.+`).
			WithArgs(testCategoryID, testUserID).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		mock.ExpectExec(`DELETE FROM categories WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testCategoryID, testUserID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		urlParams := map[string]string{"id": testCategoryID}
		req := createRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%s", testCategoryID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.DeleteCategory(rr, req)

		if status := rr.Code; status != http.StatusNoContent {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusNoContent, rr.Body.String())
		} else {
			fmt.Println("PASS: Category deleted successfully")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 6: XSS SANITISATION
	t.Run("XSS Sanitisation", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: XSS Sanitisation")

		sanitisedName := "Work"
		mock.ExpectQuery(`INSERT INTO categories .+ RETURNING .+`).
			WithArgs(testUserID, sanitisedName, (*string)(nil)).
			WillReturnRows(mockCategoryRows().AddRow(
				testCategoryID, testUserID, sanitisedName, nil, now, now,
			))

		catReq := CreateRequest{Name: "<script>alert('XSS')</script>Work"}
		reqBody, _ := json.Marshal(catReq)

		req := createRequest(http.MethodPost, "/api/v1/categories", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateCategory(rr, req)

		if status := rr.Code; status != http.StatusCreated {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusCreated, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response Category
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Name != sanitisedName {
			t.Errorf("FAIL: XSS not sanitised. Got: %s", response.Name)
		} else {
			fmt.Printf("PASS: XSS sanitised. Result: %s\n", response.Name)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 7: CATEGORY NOT FOUND
	t.Run("Get Non-Existent Category", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Get Non-Existent Category")

		nonExistentID := "00000000-0000-0000-0000-000000000000"
		mock.ExpectQuery(`SELECT .+ FROM categories WHERE id = \$1 AND user_id = \$2`).
			WithArgs(nonExistentID, testUserID).
			WillReturnRows(mockCategoryRows())

		urlParams := map[string]string{"id": nonExistentID}
		req := createRequest(http.MethodGet, "/api/v1/categories/"+nonExistentID, nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.GetCategory(rr, req)

		if status := rr.Code; status != http.StatusNotFound {
			t.Errorf("FAIL: Expected NotFound, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned NotFound for non-existent category")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 8: EMPTY CATEGORY NAME
	t.Run("Empty Category Name", func(t *testing.T) {
		fmt.Println("Running Test: Empty Category Name")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		catReq := CreateRequest{Name: ""}
		reqBody, _ := json.Marshal(catReq)

		req := createRequest(http.MethodPost, "/api/v1/categories", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateCategory(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for empty name, got %v. Body: %s", rr.Code, rr.Body.String())
		} else {
			fmt.Println("PASS: Rejected empty category name")
		}
	})

	// TEST 9: NAME TOO LONG
	t.Run("Name Too Long", func(t *testing.T) {
		fmt.Println("Running Test: Name Too Long")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		longName := make([]byte, 101)
		for i := range longName {
			longName[i] = 'a'
		}

		catReq := CreateRequest{Name: string(longName)}
		reqBody, _ := json.Marshal(catReq)

		req := createRequest(http.MethodPost, "/api/v1/categories", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateCategory(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for too long name, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected too long name")
		}
	})

	// TEST 10: INVALID UUID
	t.Run("Invalid UUID", func(t *testing.T) {
		fmt.Println("Running Test: Invalid UUID")

		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		urlParams := map[string]string{"id": "not-a-uuid"}
		req := createRequest(http.MethodGet, "/api/v1/categories/not-a-uuid", nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.GetCategory(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected BadRequest for invalid UUID, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Rejected invalid UUID")
		}
	})

	// TEST 11: DELETE CATEGORY IN USE
	t.Run("Delete Category In Use", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Delete Category In Use")

		// First verify category exists and ownership
		mock.ExpectQuery(`SELECT .+ FROM categories WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testCategoryID, testUserID).
			WillReturnRows(mockCategoryRows().AddRow(
				testCategoryID, testUserID, "Work", &description, now, now,
			))

		// Check for active tasks - returns true (has active tasks)
		mock.ExpectQuery(`SELECT EXISTS.+`).
			WithArgs(testCategoryID, testUserID).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		urlParams := map[string]string{"id": testCategoryID}
		req := createRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%s", testCategoryID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.DeleteCategory(rr, req)

		if status := rr.Code; status != http.StatusConflict {
			t.Errorf("FAIL: Expected Conflict, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned Conflict for category in use")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})
}
