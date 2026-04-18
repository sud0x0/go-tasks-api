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

	// Prepared statements in order from NewCategoryRepository
	mock.ExpectPrepare("SELECT id, user_id, name, description, colour, is_active, created_at, updated_at FROM categories WHERE id =")                         // getCategory
	mock.ExpectPrepare("SELECT id, user_id, name, description, colour, is_active, created_at, updated_at FROM categories WHERE user_id =.*is_active = true")  // getCategories
	mock.ExpectPrepare("SELECT id, user_id, name, description, colour, is_active, created_at, updated_at FROM categories WHERE user_id =.*is_active = false") // getInactiveCategories
	mock.ExpectPrepare("INSERT INTO categories")                                                                                                              // createCategory
	mock.ExpectPrepare("UPDATE categories SET name =.*is_active = true RETURNING")                                                                            // updateCategory
	mock.ExpectPrepare("UPDATE categories SET is_active = false")                                                                                             // deactivateCategory
	mock.ExpectPrepare("DELETE FROM categories WHERE id =.*is_active = false")                                                                                // hardDeleteCategory
	mock.ExpectPrepare("UPDATE categories SET is_active = true")                                                                                              // reactivateCategory
	mock.ExpectPrepare("SELECT EXISTS.*is_active = true AND id !=")                                                                                           // checkActiveNameExists
	mock.ExpectPrepare("SELECT name FROM categories WHERE id =")                                                                                              // getCategoryForReactivate
	mock.ExpectPrepare("SELECT EXISTS.*FROM categories WHERE id =")                                                                                           // checkOwnership
	mock.ExpectPrepare("SELECT is_active FROM categories WHERE id =")                                                                                         // getCategoryIsActive
	mock.ExpectPrepare("SELECT EXISTS.*FROM tasks WHERE category_id =")                                                                                       // hasActiveTasks

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
		"id", "user_id", "name", "description", "colour", "is_active", "created_at", "updated_at",
	})
}

// ============================================================================
// TESTS
// ============================================================================

func TestCategoryHandler(t *testing.T) {
	now := time.Now()
	description := "Test Description"

	// TEST 1: CREATE A CATEGORY WITH DEFAULT COLOUR
	t.Run("Create a Category with Default Colour", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running test: Create Category with Default Colour")

		mock.ExpectQuery(`INSERT INTO categories .+ RETURNING .+`).
			WithArgs(testUserID, "Work", &description, "#808080").
			WillReturnRows(mockCategoryRows().AddRow(
				testCategoryID, testUserID, "Work", &description, "#808080", true, now, now,
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

		if response.Colour != "#808080" {
			t.Errorf("FAIL: Expected colour to be '#808080', got %s", response.Colour)
		} else {
			fmt.Println("PASS: Default colour applied")
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
				testCategoryID, testUserID, "Work", &description, "#ff0000", true, now, now,
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

		if response.Colour != "#ff0000" {
			t.Errorf("FAIL: Expected colour '#ff0000', got %s", response.Colour)
		} else {
			fmt.Println("PASS: Category colour matches")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 3: UPDATE CATEGORY WITHOUT COLOUR (KEEPS EXISTING)
	t.Run("Update Category Without Colour", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Update Category Without Colour")

		// Check is_active status first - returns true (active)
		mock.ExpectQuery(`SELECT is_active FROM categories WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testCategoryID, testUserID).
			WillReturnRows(sqlmock.NewRows([]string{"is_active"}).AddRow(true))

		updatedDesc := "Updated Description"
		// When colour is nil, COALESCE keeps the existing colour
		mock.ExpectQuery(`UPDATE categories SET name = \$1, description = \$2, colour = COALESCE\(\$3, colour\), updated_at = NOW\(\) WHERE id = \$4 AND user_id = \$5.*RETURNING .+`).
			WithArgs("Personal", sqlmock.AnyArg(), nil, testCategoryID, testUserID).
			WillReturnRows(mockCategoryRows().AddRow(
				testCategoryID, testUserID, "Personal", &updatedDesc, "#ff0000", true, now, now,
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

		if response.Colour != "#ff0000" {
			t.Errorf("FAIL: Expected colour to remain '#ff0000', got %s", response.Colour)
		} else {
			fmt.Println("PASS: Colour preserved when not provided")
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

		mock.ExpectQuery(`SELECT .+ FROM categories WHERE user_id = \$1 AND is_active = true ORDER BY name ASC LIMIT \$2 OFFSET \$3`).
			WithArgs(testUserID, 20, 0).
			WillReturnRows(mockCategoryRows().
				AddRow(testCategoryID, testUserID, "Personal", nil, "#808080", true, now, now).
				AddRow("uuid-2", testUserID, "Work", &description, "#00ff00", true, now, now))

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

	// TEST 5: SOFT DELETE CATEGORY (first delete of active category)
	t.Run("Soft Delete Category", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Soft Delete Category")

		// Check is_active status - returns true (active)
		mock.ExpectQuery(`SELECT is_active FROM categories WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testCategoryID, testUserID).
			WillReturnRows(sqlmock.NewRows([]string{"is_active"}).AddRow(true))

		// Check for active tasks - returns false (no active tasks)
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM tasks WHERE category_id = \$1 AND user_id = \$2 AND is_active = true\)`).
			WithArgs(testCategoryID, testUserID).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		// Deactivate the category
		mock.ExpectExec(`UPDATE categories SET is_active = false, updated_at = NOW\(\) WHERE id = \$1 AND user_id = \$2 AND is_active = true`).
			WithArgs(testCategoryID, testUserID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		urlParams := map[string]string{"id": testCategoryID}
		req := createRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%s", testCategoryID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.DeleteCategory(rr, req)

		if status := rr.Code; status != http.StatusNoContent {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusNoContent, rr.Body.String())
		} else {
			fmt.Println("PASS: Category soft deleted successfully")
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
			WithArgs(testUserID, sanitisedName, (*string)(nil), "#808080").
			WillReturnRows(mockCategoryRows().AddRow(
				testCategoryID, testUserID, sanitisedName, nil, "#808080", true, now, now,
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

	// TEST 11: HARD DELETE CATEGORY (permanent delete of inactive category)
	t.Run("Hard Delete Category", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Hard Delete Category")

		// Check is_active status - returns false (inactive, can be hard deleted)
		mock.ExpectQuery(`SELECT is_active FROM categories WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testCategoryID, testUserID).
			WillReturnRows(sqlmock.NewRows([]string{"is_active"}).AddRow(false))

		// Hard delete the category
		mock.ExpectExec(`DELETE FROM categories WHERE id = \$1 AND user_id = \$2 AND is_active = false`).
			WithArgs(testCategoryID, testUserID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		urlParams := map[string]string{"id": testCategoryID}
		req := createRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%s/permanent", testCategoryID), nil, urlParams, nil)
		rr := httptest.NewRecorder()

		handler.PermanentDeleteCategory(rr, req)

		if status := rr.Code; status != http.StatusNoContent {
			t.Errorf("FAIL: Expected NoContent, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Category hard deleted successfully")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 12: CREATE CATEGORY WITH CUSTOM COLOUR (LOWER-CASE CONVERSION)
	t.Run("Create Category with Custom Colour", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Create Category with Custom Colour")

		// Colour should be lower-cased before storage
		mock.ExpectQuery(`INSERT INTO categories .+ RETURNING .+`).
			WithArgs(testUserID, "Work", (*string)(nil), "#aabbcc").
			WillReturnRows(mockCategoryRows().AddRow(
				testCategoryID, testUserID, "Work", nil, "#aabbcc", true, now, now,
			))

		colour := "#AABBCC" // Upper-case input
		catReq := CreateRequest{Name: "Work", Colour: &colour}
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

		if response.Colour != "#aabbcc" {
			t.Errorf("FAIL: Expected colour to be lower-cased '#aabbcc', got %s", response.Colour)
		} else {
			fmt.Println("PASS: Colour lower-cased correctly")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 13: INVALID COLOUR FORMAT
	t.Run("Invalid Colour Format", func(t *testing.T) {
		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Invalid Colour Format")

		colour := "red" // Invalid format
		catReq := CreateRequest{Name: "Work", Colour: &colour}
		reqBody, _ := json.Marshal(catReq)

		req := createRequest(http.MethodPost, "/api/v1/categories", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateCategory(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("FAIL: Expected BadRequest for invalid colour, got %v. Body: %s", rr.Code, rr.Body.String())
		} else {
			fmt.Println("PASS: Rejected invalid colour format")
		}
	})

	// TEST 14: UPDATE CATEGORY WITH COLOUR
	t.Run("Update Category with Colour", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Update Category with Colour")

		// Check is_active status first - returns true (active)
		mock.ExpectQuery(`SELECT is_active FROM categories WHERE id = \$1 AND user_id = \$2`).
			WithArgs(testCategoryID, testUserID).
			WillReturnRows(sqlmock.NewRows([]string{"is_active"}).AddRow(true))

		colour := "#00ff00"
		mock.ExpectQuery(`UPDATE categories SET name = \$1, description = \$2, colour = COALESCE\(\$3, colour\), updated_at = NOW\(\) WHERE id = \$4 AND user_id = \$5.*RETURNING .+`).
			WithArgs("Personal", (*string)(nil), sqlmock.AnyArg(), testCategoryID, testUserID).
			WillReturnRows(mockCategoryRows().AddRow(
				testCategoryID, testUserID, "Personal", nil, colour, true, now, now,
			))

		catReq := UpdateRequest{Name: "Personal", Colour: &colour}
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

		if response.Colour != colour {
			t.Errorf("FAIL: Expected colour '%s', got '%s'", colour, response.Colour)
		} else {
			fmt.Println("PASS: Colour updated successfully")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})

	// TEST 15: WHITESPACE-ONLY NAME
	t.Run("Whitespace-Only Name", func(t *testing.T) {
		handler, _, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Whitespace-Only Name")

		catReq := CreateRequest{Name: "   "}
		reqBody, _ := json.Marshal(catReq)

		req := createRequest(http.MethodPost, "/api/v1/categories", reqBody, nil, nil)
		rr := httptest.NewRecorder()

		handler.CreateCategory(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("FAIL: Expected BadRequest for whitespace-only name, got %v. Body: %s", rr.Code, rr.Body.String())
		} else {
			fmt.Println("PASS: Rejected whitespace-only name")
		}
	})

	// TEST 16: NAME TRIMMED BEFORE STORAGE
	t.Run("Name Trimmed Before Storage", func(t *testing.T) {
		handler, mock, cleanup := setupTestStack(t)
		defer cleanup()

		fmt.Println("Running Test: Name Trimmed Before Storage")

		// Service should trim the name before passing to repo
		mock.ExpectQuery(`INSERT INTO categories .+ RETURNING .+`).
			WithArgs(testUserID, "Work", (*string)(nil), "#808080").
			WillReturnRows(mockCategoryRows().AddRow(
				testCategoryID, testUserID, "Work", nil, "#808080", true, now, now,
			))

		catReq := CreateRequest{Name: "  Work  "} // Spaces around name
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
			t.Errorf("FAIL: Expected trimmed name 'Work', got '%s'", response.Name)
		} else {
			fmt.Println("PASS: Name trimmed correctly")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Unfulfilled expectations: %s", err)
		}
	})
}
