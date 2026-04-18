package auth

import "time"

// User represents a user account.
type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"` // Never expose password hash
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Validate checks that data retrieved from the database meets expected constraints.
func (u *User) Validate() error {
	if u.ID == "" {
		return ErrInvalidInput
	}
	if u.Username == "" {
		return ErrInvalidInput
	}
	if u.Password == "" {
		return ErrInvalidInput
	}
	if u.CreatedAt.IsZero() {
		return ErrInvalidInput
	}
	if u.UpdatedAt.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

// UserResponse is the public representation of a user (no password).
type UserResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToResponse converts a User to a UserResponse.
func (u *User) ToResponse() UserResponse {
	return UserResponse{
		ID:        u.ID,
		Username:  u.Username,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

// RefreshToken represents a refresh token stored in the database.
type RefreshToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TokenHash string    `json:"-"` // Never expose token hash
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// Validate checks that data retrieved from the database meets expected constraints.
func (rt *RefreshToken) Validate() error {
	if rt.ID == "" {
		return ErrInvalidInput
	}
	if rt.UserID == "" {
		return ErrInvalidInput
	}
	if rt.TokenHash == "" {
		return ErrInvalidInput
	}
	if rt.ExpiresAt.IsZero() {
		return ErrInvalidInput
	}
	if rt.CreatedAt.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

// RegisterRequest is used for user registration.
// Password length validation (8-128 code points) is performed in the service layer
// after NFKC normalization, not by the validator.
type RegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
	Password string `json:"password" validate:"required"`
}

// LoginRequest is used for user login.
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// TokenResponse is used internally for token generation.
// Handlers convert this to LoginResponse or RefreshResponse for the API response.
type TokenResponse struct {
	AccessToken  string `json:"-"` // Internal use only
	RefreshToken string `json:"-"` // Internal use only
	ExpiresIn    int    `json:"-"` // Internal use only
	TokenType    string `json:"-"` // Internal use only
}

// LoginResponse is returned after successful login.
// Contains user info and tokens in the response body.
type LoginResponse struct {
	User         UserResponse `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresAt    time.Time    `json:"expires_at"`
	TokenType    string       `json:"token_type"`
}

// RefreshResponse is returned after successful token refresh.
// Contains new tokens in the response body.
type RefreshResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// AccessTokenClaims represents the JWT claims for access tokens.
type AccessTokenClaims struct {
	Subject   string `json:"sub"`
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	ExpiresAt int64  `json:"exp"`
	NotBefore int64  `json:"nbf"`
	IssuedAt  int64  `json:"iat"`
	JWTID     string `json:"jti"`
}
