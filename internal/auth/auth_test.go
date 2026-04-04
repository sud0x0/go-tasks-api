package auth

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
)

// Run from project root: go test ./internal/auth/... -v

// ============================================================================
// TEST HELPERS
// ============================================================================

type mockLogger struct{}

func (m *mockLogger) LogError(simplifiedError, actualError error)  {}
func (m *mockLogger) LogInfo(message string, args ...any)          {}
func (m *mockLogger) LogDebug(message string)                      {}
func (m *mockLogger) WithRequestID(requestID string) logger.Logger { return m }

// mockAuthService implements authService for testing.
type mockAuthService struct {
	registerFunc            func(ctx context.Context, req RegisterRequest) (User, error)
	loginFunc               func(ctx context.Context, req LoginRequest) (TokenResponse, error)
	refreshFunc             func(ctx context.Context, refreshToken string) (TokenResponse, error)
	logoutFunc              func(ctx context.Context, refreshToken string, jti string, tokenExp time.Time) error
	validateAccessTokenFunc func(ctx context.Context, tokenString string) (string, string, time.Time, error)
}

func (m *mockAuthService) register(ctx context.Context, req RegisterRequest) (User, error) {
	if m.registerFunc != nil {
		return m.registerFunc(ctx, req)
	}
	return User{}, nil
}

func (m *mockAuthService) login(ctx context.Context, req LoginRequest) (TokenResponse, error) {
	if m.loginFunc != nil {
		return m.loginFunc(ctx, req)
	}
	return TokenResponse{}, nil
}

func (m *mockAuthService) refresh(ctx context.Context, refreshToken string) (TokenResponse, error) {
	if m.refreshFunc != nil {
		return m.refreshFunc(ctx, refreshToken)
	}
	return TokenResponse{}, nil
}

func (m *mockAuthService) logout(ctx context.Context, refreshToken string, jti string, tokenExp time.Time) error {
	if m.logoutFunc != nil {
		return m.logoutFunc(ctx, refreshToken, jti, tokenExp)
	}
	return nil
}

func (m *mockAuthService) validateAccessToken(ctx context.Context, tokenString string) (string, string, time.Time, error) {
	if m.validateAccessTokenFunc != nil {
		return m.validateAccessTokenFunc(ctx, tokenString)
	}
	return "", "", time.Time{}, nil
}

func setupTestHandler(service authService) *Handler {
	logger := &mockLogger{}
	return NewAuthHandler(service, logger)
}

func createRequest(method, path string, body []byte) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// ============================================================================
// TESTS
// ============================================================================

func TestAuthHandler(t *testing.T) {
	now := time.Now()

	// TEST 1: REGISTER SUCCESS
	t.Run("Register Success", func(t *testing.T) {
		fmt.Println("Running test: Register Success")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{
					ID:        "test-user-id",
					Username:  req.Username,
					Password:  "hashed-password",
					CreatedAt: now,
					UpdatedAt: now,
				}, nil
			},
		}

		handler := setupTestHandler(mockService)

		registerReq := RegisterRequest{Username: "testuser", Password: "Password123!"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusCreated {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusCreated, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response UserResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.Username != "testuser" {
			t.Errorf("FAIL: Expected username 'testuser', got %s", response.Username)
		} else {
			fmt.Println("PASS: Username matches")
		}
	})

	// TEST 2: REGISTER USER EXISTS
	t.Run("Register User Exists", func(t *testing.T) {
		fmt.Println("Running test: Register User Exists")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{}, ErrUserExists
			},
		}

		handler := setupTestHandler(mockService)

		registerReq := RegisterRequest{Username: "existinguser", Password: "Password123!"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusConflict {
			t.Errorf("FAIL: Expected Conflict, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned Conflict for existing user")
		}
	})

	// TEST 3: LOGIN SUCCESS
	t.Run("Login Success", func(t *testing.T) {
		fmt.Println("Running test: Login Success")

		mockService := &mockAuthService{
			loginFunc: func(ctx context.Context, req LoginRequest) (TokenResponse, error) {
				return TokenResponse{
					AccessToken:  "mock-access-token",
					RefreshToken: "mock-refresh-token",
					ExpiresIn:    900,
					TokenType:    "Bearer",
				}, nil
			},
		}

		handler := setupTestHandler(mockService)

		loginReq := LoginRequest{Username: "testuser", Password: "Password123!"}
		reqBody, _ := json.Marshal(loginReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/login", reqBody)
		rr := httptest.NewRecorder()

		handler.Login(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Status code check")
		}

		var response TokenResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.AccessToken == "" {
			t.Errorf("FAIL: Expected access_token to be non-empty")
		} else {
			fmt.Println("PASS: Access token present")
		}

		if response.TokenType != "Bearer" {
			t.Errorf("FAIL: Expected token_type 'Bearer', got %s", response.TokenType)
		} else {
			fmt.Println("PASS: Token type is Bearer")
		}
	})

	// TEST 4: LOGIN INVALID CREDENTIALS
	t.Run("Login Invalid Credentials", func(t *testing.T) {
		fmt.Println("Running test: Login Invalid Credentials")

		mockService := &mockAuthService{
			loginFunc: func(ctx context.Context, req LoginRequest) (TokenResponse, error) {
				return TokenResponse{}, ErrInvalidCredentials
			},
		}

		handler := setupTestHandler(mockService)

		loginReq := LoginRequest{Username: "testuser", Password: "wrongpassword"}
		reqBody, _ := json.Marshal(loginReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/login", reqBody)
		rr := httptest.NewRecorder()

		handler.Login(rr, req)

		if status := rr.Code; status != http.StatusUnauthorized {
			t.Errorf("FAIL: Expected Unauthorized, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned Unauthorized for invalid credentials")
		}
	})

	// TEST 5: REFRESH TOKEN SUCCESS
	t.Run("Refresh Token Success", func(t *testing.T) {
		fmt.Println("Running test: Refresh Token Success")

		mockService := &mockAuthService{
			refreshFunc: func(ctx context.Context, refreshToken string) (TokenResponse, error) {
				return TokenResponse{
					AccessToken:  "new-access-token",
					RefreshToken: "new-refresh-token",
					ExpiresIn:    900,
					TokenType:    "Bearer",
				}, nil
			},
		}

		handler := setupTestHandler(mockService)

		refreshReq := RefreshRequest{RefreshToken: "valid-refresh-token"}
		reqBody, _ := json.Marshal(refreshReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/refresh", reqBody)
		rr := httptest.NewRecorder()

		handler.Refresh(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Token refreshed successfully")
		}
	})

	// TEST 6: REFRESH TOKEN INVALID
	t.Run("Refresh Token Invalid", func(t *testing.T) {
		fmt.Println("Running test: Refresh Token Invalid")

		mockService := &mockAuthService{
			refreshFunc: func(ctx context.Context, refreshToken string) (TokenResponse, error) {
				return TokenResponse{}, ErrInvalidToken
			},
		}

		handler := setupTestHandler(mockService)

		refreshReq := RefreshRequest{RefreshToken: "invalid-refresh-token"}
		reqBody, _ := json.Marshal(refreshReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/refresh", reqBody)
		rr := httptest.NewRecorder()

		handler.Refresh(rr, req)

		if status := rr.Code; status != http.StatusUnauthorized {
			t.Errorf("FAIL: Expected Unauthorized, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned Unauthorized for invalid token")
		}
	})

	// TEST 7: LOGOUT SUCCESS
	t.Run("Logout Success", func(t *testing.T) {
		fmt.Println("Running test: Logout Success")

		mockService := &mockAuthService{
			logoutFunc: func(ctx context.Context, refreshToken string, jti string, tokenExp time.Time) error {
				return nil
			},
			validateAccessTokenFunc: func(ctx context.Context, tokenString string) (string, string, time.Time, error) {
				return "user-id", "jti-123", time.Now().Add(15 * time.Minute), nil
			},
		}

		handler := setupTestHandler(mockService)

		logoutReq := LogoutRequest{RefreshToken: "valid-refresh-token"}
		reqBody, _ := json.Marshal(logoutReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/logout", reqBody)
		req.Header.Set("Authorization", "Bearer mock-access-token")
		rr := httptest.NewRecorder()

		handler.Logout(rr, req)

		if status := rr.Code; status != http.StatusNoContent {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusNoContent, rr.Body.String())
		} else {
			fmt.Println("PASS: Logout successful")
		}
	})

	// TEST 8: INVALID REQUEST BODY
	t.Run("Invalid Request Body", func(t *testing.T) {
		fmt.Println("Running test: Invalid Request Body")

		mockService := &mockAuthService{}
		handler := setupTestHandler(mockService)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", []byte("not-json"))
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("FAIL: Expected BadRequest, got %v", status)
		} else {
			fmt.Println("PASS: Correctly rejected invalid JSON body")
		}
	})

	// TEST 9: VALIDATION ERRORS - SHORT PASSWORD
	t.Run("Validation Error - Short Password", func(t *testing.T) {
		fmt.Println("Running test: Validation Error - Short Password")

		mockService := &mockAuthService{}
		handler := setupTestHandler(mockService)

		registerReq := RegisterRequest{Username: "testuser", Password: "short"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("FAIL: Expected BadRequest, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly rejected short password")
		}
	})

	// TEST 10: VALIDATION ERRORS - SHORT USERNAME
	t.Run("Validation Error - Short Username", func(t *testing.T) {
		fmt.Println("Running test: Validation Error - Short Username")

		mockService := &mockAuthService{}
		handler := setupTestHandler(mockService)

		registerReq := RegisterRequest{Username: "ab", Password: "Password123!"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("FAIL: Expected BadRequest, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly rejected short username")
		}
	})

	// TEST 11: VALIDATION ERRORS - MISSING USERNAME
	t.Run("Validation Error - Missing Username", func(t *testing.T) {
		fmt.Println("Running test: Validation Error - Missing Username")

		mockService := &mockAuthService{}
		handler := setupTestHandler(mockService)

		registerReq := RegisterRequest{Password: "Password123!"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("FAIL: Expected BadRequest, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly rejected missing username")
		}
	})

	// TEST 12: XSS SANITISATION
	t.Run("XSS Sanitisation", func(t *testing.T) {
		fmt.Println("Running test: XSS Sanitisation")

		var capturedUsername string
		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				capturedUsername = req.Username
				return User{
					ID:        "test-user-id",
					Username:  req.Username,
					Password:  "hashed-password",
					CreatedAt: now,
					UpdatedAt: now,
				}, nil
			},
		}

		handler := setupTestHandler(mockService)

		registerReq := RegisterRequest{Username: "<script>alert('XSS')</script>testuser", Password: "Password123!"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if capturedUsername != "testuser" {
			t.Errorf("FAIL: XSS not sanitised. Got: %s, expected: testuser", capturedUsername)
		} else {
			fmt.Println("PASS: XSS sanitised in username")
		}
	})

	// TEST 13: WHITESPACE-ONLY USERNAME
	t.Run("Whitespace-Only Username", func(t *testing.T) {
		fmt.Println("Running test: Whitespace-Only Username")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{}, ErrInvalidUsername
			},
		}

		handler := setupTestHandler(mockService)

		registerReq := RegisterRequest{Username: "   ", Password: "Password123!"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		// Note: The handler will sanitise whitespace to empty string, which fails validation
		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("FAIL: Expected BadRequest for whitespace-only username, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly rejected whitespace-only username")
		}
	})

	// TEST 14: TOKEN REVOKED
	t.Run("Token Revoked", func(t *testing.T) {
		fmt.Println("Running test: Token Revoked")

		mockService := &mockAuthService{
			refreshFunc: func(ctx context.Context, refreshToken string) (TokenResponse, error) {
				return TokenResponse{}, ErrTokenRevoked
			},
		}

		handler := setupTestHandler(mockService)

		refreshReq := RefreshRequest{RefreshToken: "revoked-refresh-token"}
		reqBody, _ := json.Marshal(refreshReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/refresh", reqBody)
		rr := httptest.NewRecorder()

		handler.Refresh(rr, req)

		if status := rr.Code; status != http.StatusUnauthorized {
			t.Errorf("FAIL: Expected Unauthorized for revoked token, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned Unauthorized for revoked token")
		}
	})

	// TEST 15: USERNAME TOO LONG
	t.Run("Username Too Long", func(t *testing.T) {
		fmt.Println("Running test: Username Too Long")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{}, ErrUsernameTooLong
			},
		}

		handler := setupTestHandler(mockService)

		longUsername := make([]byte, 51)
		for i := range longUsername {
			longUsername[i] = 'a'
		}

		registerReq := RegisterRequest{Username: string(longUsername), Password: "Password123!"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		// Note: Validator will catch max=50 before service is called
		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("FAIL: Expected BadRequest for too long username, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly rejected too long username")
		}
	})
}
