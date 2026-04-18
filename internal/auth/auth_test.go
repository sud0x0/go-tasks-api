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
	registerFunc                 func(ctx context.Context, req RegisterRequest) (User, error)
	loginFunc                    func(ctx context.Context, req LoginRequest) (TokenResponse, error)
	loginWithUserFunc            func(ctx context.Context, req LoginRequest) (TokenResponse, User, error)
	refreshFunc                  func(ctx context.Context, refreshToken string, oldAccessToken string) (TokenResponse, error)
	logoutFunc                   func(ctx context.Context, refreshToken string, jti string, tokenExp time.Time) error
	logoutWithOwnershipCheckFunc func(ctx context.Context, tokenHash, userID, jti string, tokenExp time.Time) error
	blocklistJTIFunc             func(ctx context.Context, jti string, tokenExp time.Time) error
	validateAccessTokenFunc      func(ctx context.Context, tokenString string) (string, string, time.Time, error)
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

func (m *mockAuthService) loginWithUser(ctx context.Context, req LoginRequest) (TokenResponse, User, error) {
	if m.loginWithUserFunc != nil {
		return m.loginWithUserFunc(ctx, req)
	}
	return TokenResponse{}, User{}, nil
}

func (m *mockAuthService) refresh(ctx context.Context, refreshToken string, oldAccessToken string) (TokenResponse, error) {
	if m.refreshFunc != nil {
		return m.refreshFunc(ctx, refreshToken, oldAccessToken)
	}
	return TokenResponse{}, nil
}

func (m *mockAuthService) logout(ctx context.Context, refreshToken string, jti string, tokenExp time.Time) error {
	if m.logoutFunc != nil {
		return m.logoutFunc(ctx, refreshToken, jti, tokenExp)
	}
	return nil
}

func (m *mockAuthService) logoutWithOwnershipCheck(ctx context.Context, tokenHash, userID, jti string, tokenExp time.Time) error {
	if m.logoutWithOwnershipCheckFunc != nil {
		return m.logoutWithOwnershipCheckFunc(ctx, tokenHash, userID, jti, tokenExp)
	}
	return nil
}

func (m *mockAuthService) validateAccessToken(ctx context.Context, tokenString string) (string, string, time.Time, error) {
	if m.validateAccessTokenFunc != nil {
		return m.validateAccessTokenFunc(ctx, tokenString)
	}
	return "", "", time.Time{}, nil
}

func (m *mockAuthService) blocklistJTI(ctx context.Context, jti string, tokenExp time.Time) error {
	if m.blocklistJTIFunc != nil {
		return m.blocklistJTIFunc(ctx, jti, tokenExp)
	}
	return nil
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

	// TEST 3: LOGIN SUCCESS - CHECK RESPONSE BODY
	t.Run("Login Success with Tokens in Body", func(t *testing.T) {
		fmt.Println("Running test: Login Success with Tokens in Body")

		mockService := &mockAuthService{
			loginWithUserFunc: func(ctx context.Context, req LoginRequest) (TokenResponse, User, error) {
				return TokenResponse{
						AccessToken:  "mock-access-token",
						RefreshToken: "mock-refresh-token",
						ExpiresIn:    900,
						TokenType:    "Bearer",
					}, User{
						ID:        "test-user-id",
						Username:  req.Username,
						Password:  "hashed",
						CreatedAt: now,
						UpdatedAt: now,
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

		// Check response body contains user and tokens
		var response LoginResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.User.Username != "testuser" {
			t.Errorf("FAIL: Expected username 'testuser' in response, got %s", response.User.Username)
		} else {
			fmt.Println("PASS: User info in response body")
		}

		// Verify tokens ARE in response body
		if response.AccessToken != "mock-access-token" {
			t.Errorf("FAIL: Expected access_token in response, got %s", response.AccessToken)
		} else {
			fmt.Println("PASS: access_token in response body")
		}

		if response.RefreshToken != "mock-refresh-token" {
			t.Errorf("FAIL: Expected refresh_token in response, got %s", response.RefreshToken)
		} else {
			fmt.Println("PASS: refresh_token in response body")
		}

		if response.TokenType != "Bearer" {
			t.Errorf("FAIL: Expected token_type 'Bearer', got %s", response.TokenType)
		} else {
			fmt.Println("PASS: token_type is Bearer")
		}

		if response.ExpiresAt.IsZero() {
			t.Error("FAIL: Expected expires_at to be set")
		} else {
			fmt.Println("PASS: expires_at is set")
		}

		// Verify NO cookies are set
		cookies := rr.Result().Cookies()
		if len(cookies) > 0 {
			t.Errorf("FAIL: No cookies should be set, but found %d cookies", len(cookies))
		} else {
			fmt.Println("PASS: No cookies set")
		}
	})

	// TEST 4: LOGIN INVALID CREDENTIALS
	t.Run("Login Invalid Credentials", func(t *testing.T) {
		fmt.Println("Running test: Login Invalid Credentials")

		mockService := &mockAuthService{
			loginWithUserFunc: func(ctx context.Context, req LoginRequest) (TokenResponse, User, error) {
				return TokenResponse{}, User{}, ErrInvalidCredentials
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

	// TEST 5: REFRESH TOKEN SUCCESS (via header)
	t.Run("Refresh Token Success via Header", func(t *testing.T) {
		fmt.Println("Running test: Refresh Token Success via Header")

		mockService := &mockAuthService{
			refreshFunc: func(ctx context.Context, refreshToken string, oldAccessToken string) (TokenResponse, error) {
				return TokenResponse{
					AccessToken:  "new-access-token",
					RefreshToken: "new-refresh-token",
					ExpiresIn:    900,
					TokenType:    "Bearer",
				}, nil
			},
		}

		handler := setupTestHandler(mockService)

		req := createRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		// Set refresh token in header
		req.Header.Set("X-Refresh-Token", "valid-refresh-token")
		rr := httptest.NewRecorder()

		handler.Refresh(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
		} else {
			fmt.Println("PASS: Token refreshed with 200 OK")
		}

		// Verify response body contains new tokens
		var response RefreshResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("FAIL: Failed to unmarshal response: %v", err)
		}

		if response.AccessToken != "new-access-token" {
			t.Errorf("FAIL: Expected new access_token, got %s", response.AccessToken)
		} else {
			fmt.Println("PASS: New access_token in response body")
		}

		if response.RefreshToken != "new-refresh-token" {
			t.Errorf("FAIL: Expected new refresh_token, got %s", response.RefreshToken)
		} else {
			fmt.Println("PASS: New refresh_token in response body")
		}

		// Verify NO cookies are set
		cookies := rr.Result().Cookies()
		if len(cookies) > 0 {
			t.Errorf("FAIL: No cookies should be set, but found %d cookies", len(cookies))
		} else {
			fmt.Println("PASS: No cookies set")
		}
	})

	// TEST 6: REFRESH TOKEN MISSING HEADER
	t.Run("Refresh Token Missing Header", func(t *testing.T) {
		fmt.Println("Running test: Refresh Token Missing Header")

		mockService := &mockAuthService{}

		handler := setupTestHandler(mockService)

		req := createRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		// No X-Refresh-Token header
		rr := httptest.NewRecorder()

		handler.Refresh(rr, req)

		if status := rr.Code; status != http.StatusUnauthorized {
			t.Errorf("FAIL: Expected Unauthorized, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned Unauthorized for missing refresh header")
		}
	})

	// TEST 7: REFRESH TOKEN INVALID
	t.Run("Refresh Token Invalid", func(t *testing.T) {
		fmt.Println("Running test: Refresh Token Invalid")

		mockService := &mockAuthService{
			refreshFunc: func(ctx context.Context, refreshToken string, oldAccessToken string) (TokenResponse, error) {
				return TokenResponse{}, ErrInvalidToken
			},
		}

		handler := setupTestHandler(mockService)

		req := createRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		req.Header.Set("X-Refresh-Token", "invalid-refresh-token")
		rr := httptest.NewRecorder()

		handler.Refresh(rr, req)

		if status := rr.Code; status != http.StatusUnauthorized {
			t.Errorf("FAIL: Expected Unauthorized, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned Unauthorized for invalid token")
		}
	})

	// TEST 8: LOGOUT SUCCESS (via headers)
	t.Run("Logout Success via Headers", func(t *testing.T) {
		fmt.Println("Running test: Logout Success via Headers")

		mockService := &mockAuthService{
			logoutWithOwnershipCheckFunc: func(ctx context.Context, tokenHash, userID, jti string, tokenExp time.Time) error {
				return nil
			},
			validateAccessTokenFunc: func(ctx context.Context, tokenString string) (string, string, time.Time, error) {
				return "user-id", "jti-123", time.Now().Add(15 * time.Minute), nil
			},
		}

		handler := setupTestHandler(mockService)

		req := createRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.Header.Set("Authorization", "Bearer mock-access-token")
		req.Header.Set("X-Refresh-Token", "valid-refresh-token")
		rr := httptest.NewRecorder()

		handler.Logout(rr, req)

		if status := rr.Code; status != http.StatusNoContent {
			t.Errorf("FAIL: Handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusNoContent, rr.Body.String())
		} else {
			fmt.Println("PASS: Logout successful with 204 No Content")
		}
	})

	// TEST 9: LOGOUT OWNERSHIP MISMATCH
	t.Run("Logout Ownership Mismatch", func(t *testing.T) {
		fmt.Println("Running test: Logout Ownership Mismatch")

		mockService := &mockAuthService{
			logoutWithOwnershipCheckFunc: func(ctx context.Context, tokenHash, userID, jti string, tokenExp time.Time) error {
				return ErrTokenOwnershipMismatch
			},
			validateAccessTokenFunc: func(ctx context.Context, tokenString string) (string, string, time.Time, error) {
				return "user-id", "jti-123", time.Now().Add(15 * time.Minute), nil
			},
		}

		handler := setupTestHandler(mockService)

		req := createRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.Header.Set("Authorization", "Bearer mock-access-token")
		req.Header.Set("X-Refresh-Token", "other-users-refresh-token")
		rr := httptest.NewRecorder()

		handler.Logout(rr, req)

		if status := rr.Code; status != http.StatusUnauthorized {
			t.Errorf("FAIL: Expected Unauthorized for ownership mismatch, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned Unauthorized for ownership mismatch")
		}
	})

	// TEST 10: INVALID REQUEST BODY
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

	// TEST 11: VALIDATION ERRORS - SHORT PASSWORD
	// Note: Password length is now validated in the service layer after NFKC normalization,
	// not by the handler validator. The service returns ErrPasswordTooShort.
	t.Run("Validation Error - Short Password", func(t *testing.T) {
		fmt.Println("Running test: Validation Error - Short Password")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{}, ErrPasswordTooShort
			},
		}
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

	// TEST 12: VALIDATION ERRORS - SHORT USERNAME
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

	// TEST 13: VALIDATION ERRORS - MISSING USERNAME
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

	// TEST 14: XSS SANITISATION
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

	// TEST 15: WHITESPACE-ONLY USERNAME
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

	// TEST 16: TOKEN REVOKED
	t.Run("Token Revoked", func(t *testing.T) {
		fmt.Println("Running test: Token Revoked")

		mockService := &mockAuthService{
			refreshFunc: func(ctx context.Context, refreshToken string, oldAccessToken string) (TokenResponse, error) {
				return TokenResponse{}, ErrTokenRevoked
			},
		}

		handler := setupTestHandler(mockService)

		req := createRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		req.Header.Set("X-Refresh-Token", "revoked-refresh-token")
		rr := httptest.NewRecorder()

		handler.Refresh(rr, req)

		if status := rr.Code; status != http.StatusUnauthorized {
			t.Errorf("FAIL: Expected Unauthorized for revoked token, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Correctly returned Unauthorized for revoked token")
		}
	})

	// TEST 17: USERNAME TOO LONG
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

	// TEST 18: PASSWORD 8 RUNES (MINIMUM BOUNDARY)
	t.Run("Password 8 Runes - Minimum Boundary", func(t *testing.T) {
		fmt.Println("Running test: Password 8 Runes - Minimum Boundary")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{
					ID:        "test-user-id",
					Username:  req.Username,
					Password:  "hashed",
					CreatedAt: now,
					UpdatedAt: now,
				}, nil
			},
		}

		handler := setupTestHandler(mockService)

		// Exactly 8 characters
		registerReq := RegisterRequest{Username: "testuser", Password: "12345678"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusCreated {
			t.Errorf("FAIL: Expected Created for 8-char password, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: 8-character password accepted")
		}
	})

	// TEST 19: PASSWORD 7 RUNES (BELOW MINIMUM)
	t.Run("Password 7 Runes - Below Minimum", func(t *testing.T) {
		fmt.Println("Running test: Password 7 Runes - Below Minimum")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{}, ErrPasswordTooShort
			},
		}

		handler := setupTestHandler(mockService)

		// 7 characters - should fail
		registerReq := RegisterRequest{Username: "testuser", Password: "1234567"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("FAIL: Expected BadRequest for 7-char password, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: 7-character password rejected")
		}
	})

	// TEST 20: PASSWORD 128 RUNES (MAXIMUM BOUNDARY)
	t.Run("Password 128 Runes - Maximum Boundary", func(t *testing.T) {
		fmt.Println("Running test: Password 128 Runes - Maximum Boundary")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{
					ID:        "test-user-id",
					Username:  req.Username,
					Password:  "hashed",
					CreatedAt: now,
					UpdatedAt: now,
				}, nil
			},
		}

		handler := setupTestHandler(mockService)

		// Exactly 128 characters
		password128 := make([]byte, 128)
		for i := range password128 {
			password128[i] = 'a'
		}

		registerReq := RegisterRequest{Username: "testuser", Password: string(password128)}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusCreated {
			t.Errorf("FAIL: Expected Created for 128-char password, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: 128-character password accepted")
		}
	})

	// TEST 21: PASSWORD 129 RUNES (ABOVE MAXIMUM)
	t.Run("Password 129 Runes - Above Maximum", func(t *testing.T) {
		fmt.Println("Running test: Password 129 Runes - Above Maximum")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{}, ErrPasswordTooLong
			},
		}

		handler := setupTestHandler(mockService)

		// 129 characters - should fail
		password129 := make([]byte, 129)
		for i := range password129 {
			password129[i] = 'a'
		}

		registerReq := RegisterRequest{Username: "testuser", Password: string(password129)}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("FAIL: Expected BadRequest for 129-char password, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: 129-character password rejected")
		}
	})

	// TEST 22: PASSWORD WITH MULTI-BYTE CHARACTERS (RUNE COUNT)
	t.Run("Password Multi-Byte Characters Rune Count", func(t *testing.T) {
		fmt.Println("Running test: Password Multi-Byte Characters Rune Count")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{
					ID:        "test-user-id",
					Username:  req.Username,
					Password:  "hashed",
					CreatedAt: now,
					UpdatedAt: now,
				}, nil
			},
		}

		handler := setupTestHandler(mockService)

		// 8 emoji = 8 runes (each emoji is typically 4 bytes but 1 rune)
		// Using simple emoji that are single code points
		registerReq := RegisterRequest{Username: "testuser", Password: "🔐🔑🔒🔓🛡️🗝️💻🖥️"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusCreated {
			t.Errorf("FAIL: Expected Created for 8-rune emoji password, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Multi-byte character password counted by runes")
		}
	})

	// TEST 23: PASSWORD WITH NULL BYTE (CONTROL CHARACTER)
	t.Run("Password With Null Byte", func(t *testing.T) {
		fmt.Println("Running test: Password With Null Byte")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{}, ErrPasswordInvalidChars
			},
		}

		handler := setupTestHandler(mockService)

		// Password with null byte
		registerReq := RegisterRequest{Username: "testuser", Password: "pass\x00word123"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("FAIL: Expected BadRequest for password with null byte, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Password with null byte rejected")
		}
	})

	// TEST 24: PASSWORD WITH CONTROL CHARACTER (SOH = 0x01)
	t.Run("Password With Control Character", func(t *testing.T) {
		fmt.Println("Running test: Password With Control Character")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{}, ErrPasswordInvalidChars
			},
		}

		handler := setupTestHandler(mockService)

		// Password with SOH control character
		registerReq := RegisterRequest{Username: "testuser", Password: "pass\x01word123"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("FAIL: Expected BadRequest for password with control char, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Password with control character rejected")
		}
	})

	// TEST 25: PASSPHRASE (LONG WITH SPACES)
	t.Run("Passphrase With Spaces", func(t *testing.T) {
		fmt.Println("Running test: Passphrase With Spaces")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{
					ID:        "test-user-id",
					Username:  req.Username,
					Password:  "hashed",
					CreatedAt: now,
					UpdatedAt: now,
				}, nil
			},
		}

		handler := setupTestHandler(mockService)

		// Long passphrase with spaces
		registerReq := RegisterRequest{Username: "testuser", Password: "correct horse battery staple"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusCreated {
			t.Errorf("FAIL: Expected Created for passphrase, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Passphrase with spaces accepted")
		}
	})

	// TEST 26: PASSWORD NOT SANITIZED (HTML-LIKE CHARS PRESERVED)
	t.Run("Password Not Sanitized", func(t *testing.T) {
		fmt.Println("Running test: Password Not Sanitized")

		var capturedPassword string
		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				capturedPassword = req.Password
				return User{
					ID:        "test-user-id",
					Username:  req.Username,
					Password:  "hashed",
					CreatedAt: now,
					UpdatedAt: now,
				}, nil
			},
		}

		handler := setupTestHandler(mockService)

		// Password with HTML-like characters that would be mangled by sanitizer
		originalPassword := "<script>&amp;test</script>"
		registerReq := RegisterRequest{Username: "testuser", Password: originalPassword}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if capturedPassword != originalPassword {
			t.Errorf("FAIL: Password was sanitized! Got: %q, expected: %q", capturedPassword, originalPassword)
		} else {
			fmt.Println("PASS: Password not sanitized, special chars preserved")
		}
	})

	// TEST 27: PASSWORD WITH ALLOWED WHITESPACE (TAB, LF, CR)
	t.Run("Password With Allowed Whitespace", func(t *testing.T) {
		fmt.Println("Running test: Password With Allowed Whitespace")

		mockService := &mockAuthService{
			registerFunc: func(ctx context.Context, req RegisterRequest) (User, error) {
				return User{
					ID:        "test-user-id",
					Username:  req.Username,
					Password:  "hashed",
					CreatedAt: now,
					UpdatedAt: now,
				}, nil
			},
		}

		handler := setupTestHandler(mockService)

		// Password with tab, which is allowed
		registerReq := RegisterRequest{Username: "testuser", Password: "pass\tword\twith\ttabs"}
		reqBody, _ := json.Marshal(registerReq)

		req := createRequest(http.MethodPost, "/api/v1/auth/register", reqBody)
		rr := httptest.NewRecorder()

		handler.Register(rr, req)

		if status := rr.Code; status != http.StatusCreated {
			t.Errorf("FAIL: Expected Created for password with tabs, got %v. Body: %s", status, rr.Body.String())
		} else {
			fmt.Println("PASS: Password with allowed whitespace (tab) accepted")
		}
	})
}

// ============================================================================
// AUTH MIDDLEWARE TESTS
// ============================================================================

// ============================================================================
// PASSWORD VALIDATION UNIT TESTS
// ============================================================================

func TestContainsControlChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"empty string", "", false},
		{"normal ASCII", "password123", false},
		{"with space", "pass word", false},
		{"with tab (allowed)", "pass\tword", false},
		{"with LF (allowed)", "pass\nword", false},
		{"with CR (allowed)", "pass\rword", false},
		{"with null byte", "pass\x00word", true},
		{"with SOH (0x01)", "pass\x01word", true},
		{"with STX (0x02)", "pass\x02word", true},
		{"with ETX (0x03)", "pass\x03word", true},
		{"with EOT (0x04)", "pass\x04word", true},
		{"with ENQ (0x05)", "pass\x05word", true},
		{"with ACK (0x06)", "pass\x06word", true},
		{"with BEL (0x07)", "pass\x07word", true},
		{"with BS (0x08)", "pass\x08word", true},
		{"with VT (0x0B)", "pass\x0Bword", true},
		{"with FF (0x0C)", "pass\x0Cword", true},
		{"with SO (0x0E)", "pass\x0Eword", true},
		{"with SI (0x0F)", "pass\x0Fword", true},
		{"with DLE (0x10)", "pass\x10word", true},
		{"with DC1-DC4", "pass\x11word", true},
		{"with NAK (0x15)", "pass\x15word", true},
		{"with SYN (0x16)", "pass\x16word", true},
		{"with ETB (0x17)", "pass\x17word", true},
		{"with CAN (0x18)", "pass\x18word", true},
		{"with EM (0x19)", "pass\x19word", true},
		{"with SUB (0x1A)", "pass\x1Aword", true},
		{"with ESC (0x1B)", "pass\x1Bword", true},
		{"with FS (0x1C)", "pass\x1Cword", true},
		{"with GS (0x1D)", "pass\x1Dword", true},
		{"with RS (0x1E)", "pass\x1Eword", true},
		{"with US (0x1F)", "pass\x1Fword", true},
		{"with DEL (0x7F)", "pass\x7Fword", true},
		{"with emoji", "🔐password", false},
		{"with CJK", "密码password", false},
		{"with accented", "pässwörd", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := containsControlChars(tc.input)
			if result != tc.expected {
				t.Errorf("containsControlChars(%q) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}

func TestNormalisePassword(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"ASCII unchanged", "password123", "password123"},
		{"full-width to ASCII", "ｐａｓｓｗｏｒｄ", "password"},
		{"composed to decomposed", "café", "café"}, // Both should normalize the same
		{"already NFKC", "password", "password"},
		{"space preserved", "pass word", "pass word"},
		{"special chars preserved", "p@ss!w0rd#", "p@ss!w0rd#"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := normalisePassword(tc.input)
			if result != tc.expected {
				t.Errorf("normalisePassword(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// ============================================================================
// AUTH MIDDLEWARE TESTS
// ============================================================================

func TestAuthMiddleware(t *testing.T) {
	// TEST: Middleware accepts valid Authorization header
	t.Run("Valid Authorization Header", func(t *testing.T) {
		fmt.Println("Running test: Valid Authorization Header")

		mockService := &mockAuthService{
			validateAccessTokenFunc: func(ctx context.Context, tokenString string) (string, string, time.Time, error) {
				if tokenString != "valid-access-token" {
					t.Errorf("FAIL: Expected token 'valid-access-token', got '%s'", tokenString)
				}
				return "user-123", "jti-abc", time.Now().Add(15 * time.Minute), nil
			},
		}

		middleware := NewAuthMiddleware(mockService, &mockLogger{})

		handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r.Context())
			if userID != "user-123" {
				t.Errorf("FAIL: Expected userID 'user-123', got '%s'", userID)
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer valid-access-token")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("FAIL: Expected OK, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Valid Authorization header accepted")
		}
	})

	// TEST: Middleware rejects missing Authorization header
	t.Run("Missing Authorization Header", func(t *testing.T) {
		fmt.Println("Running test: Missing Authorization Header")

		mockService := &mockAuthService{}
		middleware := NewAuthMiddleware(mockService, &mockLogger{})

		handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("FAIL: Handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
		// No Authorization header
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("FAIL: Expected Unauthorized, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Missing Authorization header rejected")
		}
	})

	// TEST: Middleware rejects revoked token
	t.Run("Revoked Token", func(t *testing.T) {
		fmt.Println("Running test: Revoked Token")

		mockService := &mockAuthService{
			validateAccessTokenFunc: func(ctx context.Context, tokenString string) (string, string, time.Time, error) {
				return "", "", time.Time{}, ErrTokenRevoked
			},
		}

		middleware := NewAuthMiddleware(mockService, &mockLogger{})

		handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("FAIL: Handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer revoked-token")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("FAIL: Expected Unauthorized, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Revoked token rejected")
		}
	})

	// TEST: Middleware rejects invalid Bearer format
	t.Run("Invalid Bearer Format", func(t *testing.T) {
		fmt.Println("Running test: Invalid Bearer Format")

		mockService := &mockAuthService{}
		middleware := NewAuthMiddleware(mockService, &mockLogger{})

		handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("FAIL: Handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
		req.Header.Set("Authorization", "Basic sometoken") // Wrong scheme
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("FAIL: Expected Unauthorized, got %v", rr.Code)
		} else {
			fmt.Println("PASS: Invalid Bearer format rejected")
		}
	})

	// TEST: Middleware rejects lowercase "bearer" (case-sensitive)
	t.Run("Lowercase Bearer Rejected", func(t *testing.T) {
		fmt.Println("Running test: Lowercase Bearer Rejected")

		mockService := &mockAuthService{}
		middleware := NewAuthMiddleware(mockService, &mockLogger{})

		handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("FAIL: Handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
		req.Header.Set("Authorization", "bearer valid-token") // lowercase
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("FAIL: Expected Unauthorized for lowercase 'bearer', got %v", rr.Code)
		} else {
			fmt.Println("PASS: Lowercase 'bearer' correctly rejected (case-sensitive)")
		}
	})
}
