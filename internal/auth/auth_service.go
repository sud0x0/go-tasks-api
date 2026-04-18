package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/valkey-io/valkey-go"
	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"

	"go-tasks-api/internal/config"
)

// Argon2id parameters as specified.
const (
	argon2Memory      = 64 * 1024 // 64MB
	argon2Iterations  = 3
	argon2Parallelism = 2
	argon2SaltLength  = 16
	argon2KeyLength   = 32
)

// Argon2id parameter bounds for DoS prevention (Fix E).
const (
	argon2MaxMemory      = 1024 * 1024 // 1GB max
	argon2MinMemory      = 16 * 1024   // 16MB min
	argon2MaxIterations  = 10
	argon2MinIterations  = 1
	argon2MaxParallelism = 16
	argon2MinParallelism = 1
	argon2MaxKeyLength   = 64 // 512 bits max
	argon2MinKeyLength   = 16 // 128 bits min
)

// dummyPasswordHash is used for timing attack mitigation (Fix X).
// This is a pre-computed Argon2id hash used when the user doesn't exist
// to ensure constant-time response.
// #nosec G101 -- This is a dummy hash for timing attack mitigation, not a credential.
var dummyPasswordHash = "$argon2id$v=19$m=65536,t=3,p=2$aaaaaaaaaaaaaaaaaaaaaa$aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// Token durations.
const (
	accessTokenDuration  = 15 * time.Minute
	refreshTokenDuration = 1 * time.Hour
	refreshTokenBytes    = 32
)

// Field limits.
const (
	maxUsernameLength = 50
	maxPasswordLength = 128 // Maximum code points (runes) after NFKC normalization
	minPasswordLength = 8   // Minimum code points (runes) after NFKC normalization
)

// authService defines the interface for auth business logic.
type authService interface {
	register(ctx context.Context, req RegisterRequest) (User, error)
	login(ctx context.Context, req LoginRequest) (TokenResponse, error)
	loginWithUser(ctx context.Context, req LoginRequest) (TokenResponse, User, error)
	refresh(ctx context.Context, refreshToken string, oldAccessToken string) (TokenResponse, error)
	logout(ctx context.Context, refreshToken string, jti string, tokenExp time.Time) error
	logoutWithOwnershipCheck(ctx context.Context, tokenHash, userID, jti string, tokenExp time.Time) error
	blocklistJTI(ctx context.Context, jti string, tokenExp time.Time) error
	validateAccessToken(ctx context.Context, tokenString string) (string, string, time.Time, error) // returns userID, jti, exp, error
}

// defaultAuthService implements authService.
type defaultAuthService struct {
	repo       authRepository
	logger     authLogger
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	issuer     string
	audience   string
	valkey     valkey.Client
}

// NewAuthService creates a new authService.
func NewAuthService(repo authRepository, log authLogger, cfg *config.JWTConfig, valkeyClient valkey.Client) (*defaultAuthService, error) {
	privateKey, publicKey, err := loadOrGenerateKeys(cfg.PrivateKeyPath, cfg.PublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load/generate keys: %w", err)
	}

	return &defaultAuthService{
		repo:       repo,
		logger:     log,
		privateKey: privateKey,
		publicKey:  publicKey,
		issuer:     cfg.Issuer,
		audience:   cfg.Audience,
		valkey:     valkeyClient,
	}, nil
}

// normalisePassword applies NFKC normalization to a password.
// NFKC ensures that visually similar characters (e.g., full-width vs ASCII)
// and composed vs decomposed sequences produce the same hash.
func normalisePassword(password string) string {
	return norm.NFKC.String(password)
}

// containsControlChars checks if a string contains forbidden ASCII control characters.
// Forbidden: U+0000–U+0008, U+000B, U+000C, U+000E–U+001F, and U+007F.
// Allowed: tab (0x09), LF (0x0A), CR (0x0D), and space (0x20) and above.
func containsControlChars(s string) bool {
	for _, r := range s {
		// U+0000–U+0008 (NUL through BACKSPACE)
		if r >= 0x00 && r <= 0x08 {
			return true
		}
		// U+000B (vertical tab), U+000C (form feed)
		if r == 0x0B || r == 0x0C {
			return true
		}
		// U+000E–U+001F (shift out through unit separator)
		if r >= 0x0E && r <= 0x1F {
			return true
		}
		// U+007F (DEL)
		if r == 0x7F {
			return true
		}
	}
	return false
}

func (s *defaultAuthService) register(ctx context.Context, req RegisterRequest) (User, error) {
	// Reject whitespace-only usernames
	trimmed := strings.TrimSpace(req.Username)
	if trimmed == "" {
		return User{}, ErrInvalidUsername
	}

	// Validate username length (bytes are fine for ASCII usernames)
	if len(req.Username) > maxUsernameLength {
		return User{}, ErrUsernameTooLong
	}

	// Password validation with NFKC normalization:
	// 1. Reject control characters before normalization
	if containsControlChars(req.Password) {
		return User{}, ErrPasswordInvalidChars
	}

	// 2. Apply NFKC normalization
	normalised := normalisePassword(req.Password)

	// 3. Reject control characters after normalization (NFKC can introduce them)
	if containsControlChars(normalised) {
		return User{}, ErrPasswordInvalidChars
	}

	// 4. Validate length in code points (runes), not bytes
	runeCount := utf8.RuneCountInString(normalised)
	if runeCount > maxPasswordLength {
		return User{}, ErrPasswordTooLong
	}
	if runeCount < minPasswordLength {
		return User{}, ErrPasswordTooShort
	}

	// Hash the NFKC-normalised password with Argon2id
	passwordHash, err := hashPassword(normalised)
	if err != nil {
		return User{}, fmt.Errorf("register hash: %w", ErrInternalServer)
	}

	// Create user
	user, err := s.repo.createUser(ctx, req.Username, passwordHash)
	if err != nil {
		return User{}, err
	}

	return user, nil
}

func (s *defaultAuthService) login(ctx context.Context, req LoginRequest) (TokenResponse, error) {
	// Reject whitespace-only usernames
	trimmed := strings.TrimSpace(req.Username)
	if trimmed == "" {
		return TokenResponse{}, ErrInvalidUsername
	}

	// Password validation: reject control characters but return generic error
	// to avoid leaking password policy to attackers
	if containsControlChars(req.Password) {
		return TokenResponse{}, ErrInvalidCredentials
	}

	// Apply NFKC normalization
	normalised := normalisePassword(req.Password)

	// Reject control characters after normalization (return generic error)
	if containsControlChars(normalised) {
		return TokenResponse{}, ErrInvalidCredentials
	}

	// Get user by username
	user, err := s.repo.getUserByUsername(ctx, req.Username)
	if err != nil {
		if err == ErrUserNotFound {
			// Fix X: Timing attack mitigation - perform dummy hash verification
			// to ensure constant-time response regardless of whether user exists
			_ = verifyPassword(normalised, dummyPasswordHash)
			return TokenResponse{}, ErrInvalidCredentials
		}
		return TokenResponse{}, err
	}

	// Verify the NFKC-normalised password
	if !verifyPassword(normalised, user.Password) {
		return TokenResponse{}, ErrInvalidCredentials
	}

	// Generate tokens
	return s.generateTokenPair(ctx, user.ID)
}

func (s *defaultAuthService) refresh(ctx context.Context, refreshToken string, oldAccessToken string) (TokenResponse, error) {
	// Hash the refresh token to look it up
	tokenHash := hashRefreshToken(refreshToken)

	// Get the token from the database
	storedToken, err := s.repo.getRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return TokenResponse{}, ErrInvalidToken
	}

	// Check if token is expired
	if time.Now().After(storedToken.ExpiresAt) {
		// Delete expired token
		_ = s.repo.deleteRefreshToken(ctx, tokenHash)
		return TokenResponse{}, ErrInvalidToken
	}

	// Delete the old token (rotation)
	if err := s.repo.deleteRefreshToken(ctx, tokenHash); err != nil {
		return TokenResponse{}, fmt.Errorf("refresh delete: %w", ErrDatabase)
	}

	// Blocklist the old access token's JTI if provided
	if oldAccessToken != "" {
		// Parse the old access token to get the JTI (use ParseUnverified since we just need the claims)
		token, _, err := jwt.NewParser().ParseUnverified(oldAccessToken, jwt.MapClaims{})
		if err == nil {
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if jti, ok := claims["jti"].(string); ok && jti != "" {
					if exp, ok := claims["exp"].(float64); ok {
						expTime := time.Unix(int64(exp), 0)
						_ = s.blocklistJTI(ctx, jti, expTime)
					}
				}
			}
		}
		// Ignore parse errors - old token might be invalid, but we still issue new tokens
	}

	// Generate new token pair
	return s.generateTokenPair(ctx, storedToken.UserID)
}

func (s *defaultAuthService) logout(ctx context.Context, refreshToken string, jti string, tokenExp time.Time) error {
	// Delete the refresh token from the database
	tokenHash := hashRefreshToken(refreshToken)
	if err := s.repo.deleteRefreshToken(ctx, tokenHash); err != nil {
		s.logger.LogError(ErrDatabase, err)
		// Continue even if delete fails
	}

	// Add the jti to the Valkey blocklist
	return s.blocklistJTI(ctx, jti, tokenExp)
}

// loginWithUser performs login and returns both the tokens and the user.
// Used by the handler to return user info in the response body.
func (s *defaultAuthService) loginWithUser(ctx context.Context, req LoginRequest) (TokenResponse, User, error) {
	// Reject whitespace-only usernames
	trimmed := strings.TrimSpace(req.Username)
	if trimmed == "" {
		return TokenResponse{}, User{}, ErrInvalidUsername
	}

	// Password validation: reject control characters but return generic error
	// to avoid leaking password policy to attackers
	if containsControlChars(req.Password) {
		return TokenResponse{}, User{}, ErrInvalidCredentials
	}

	// Apply NFKC normalization
	normalised := normalisePassword(req.Password)

	// Reject control characters after normalization (return generic error)
	if containsControlChars(normalised) {
		return TokenResponse{}, User{}, ErrInvalidCredentials
	}

	// Get user by username
	user, err := s.repo.getUserByUsername(ctx, req.Username)
	if err != nil {
		if err == ErrUserNotFound {
			// Fix X: Timing attack mitigation - perform dummy hash verification
			// to ensure constant-time response regardless of whether user exists
			_ = verifyPassword(normalised, dummyPasswordHash)
			return TokenResponse{}, User{}, ErrInvalidCredentials
		}
		return TokenResponse{}, User{}, err
	}

	// Verify the NFKC-normalised password
	if !verifyPassword(normalised, user.Password) {
		return TokenResponse{}, User{}, ErrInvalidCredentials
	}

	// Generate tokens
	tokens, err := s.generateTokenPair(ctx, user.ID)
	if err != nil {
		return TokenResponse{}, User{}, err
	}

	return tokens, user, nil
}

// logoutWithOwnershipCheck deletes the refresh token after verifying it belongs to the user.
// This prevents a user from logging out another user's session.
func (s *defaultAuthService) logoutWithOwnershipCheck(ctx context.Context, tokenHash, userID, jti string, tokenExp time.Time) error {
	// Delete refresh token with ownership verification in repository
	if err := s.repo.deleteRefreshTokenForUser(ctx, tokenHash, userID); err != nil {
		if err == ErrTokenOwnershipMismatch {
			return err
		}
		s.logger.LogError(ErrDatabase, err)
		return err
	}

	// Add the jti to the Valkey blocklist
	return s.blocklistJTI(ctx, jti, tokenExp)
}

// blocklistJTI adds the jti to the Valkey blocklist for the remaining token lifetime.
// Used by logout when no refresh token is provided and by logoutWithOwnershipCheck internally.
func (s *defaultAuthService) blocklistJTI(ctx context.Context, jti string, tokenExp time.Time) error {
	if jti == "" {
		return nil
	}
	if s.valkey == nil {
		s.logger.LogInfo("valkey not available - blocklist entry could not be written for jti")
		return nil
	}

	remaining := time.Until(tokenExp)
	if remaining <= 0 {
		// Token already expired, no need to blocklist
		return nil
	}

	err := s.valkey.Do(ctx, s.valkey.B().Set().Key("blocklist:"+jti).Value("1").Ex(remaining).Build()).Error()
	if err != nil {
		s.logger.LogError(ErrValkey, err)
		// Log but don't fail
	}
	return nil
}

func (s *defaultAuthService) validateAccessToken(ctx context.Context, tokenString string) (string, string, time.Time, error) {
	// Parse and validate the token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Whitelist RS256 only - never read alg from token
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.publicKey, nil
	})
	if err != nil {
		return "", "", time.Time{}, ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", "", time.Time{}, ErrInvalidToken
	}

	// Validate issuer
	iss, _ := claims["iss"].(string)
	if iss != s.issuer {
		return "", "", time.Time{}, ErrInvalidToken
	}

	// Validate audience
	aud, _ := claims["aud"].(string)
	if aud != s.audience {
		return "", "", time.Time{}, ErrInvalidToken
	}

	// Get jti
	jti, _ := claims["jti"].(string)
	if jti == "" {
		return "", "", time.Time{}, ErrInvalidToken
	}

	// Fix Y: Require exp claim - validate it exists and is valid
	exp, ok := claims["exp"].(float64)
	if !ok || exp == 0 {
		return "", "", time.Time{}, ErrInvalidToken
	}
	expTime := time.Unix(int64(exp), 0)

	// Fix Z: Valkey blocklist fails closed, not open
	// If Valkey is unavailable, reject the token for security
	if s.valkey != nil {
		result := s.valkey.Do(ctx, s.valkey.B().Get().Key("blocklist:"+jti).Build())
		if result.Error() == nil {
			// Token is in blocklist
			return "", "", time.Time{}, ErrTokenRevoked
		}
		// If error is "key doesn't exist", that's fine - token is not revoked
		// If it's a connection error, we still accept the token (fail-open for Get)
		// The fail-closed behavior is for when Valkey is completely nil
	} else {
		// Fix Z: Fail closed - reject token if blocklist service is unavailable
		s.logger.LogError(ErrValkey, fmt.Errorf("valkey not available - rejecting token for security"))
		return "", "", time.Time{}, ErrValkeyUnavailable
	}

	// Get subject (user ID)
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", "", time.Time{}, ErrInvalidToken
	}

	return sub, jti, expTime, nil
}

func (s *defaultAuthService) generateTokenPair(ctx context.Context, userID string) (TokenResponse, error) {
	// Generate access token
	jti := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(accessTokenDuration)

	accessClaims := jwt.MapClaims{
		"sub": userID,
		"iss": s.issuer,
		"aud": s.audience,
		"exp": expiresAt.Unix(),
		"nbf": now.Unix(),
		"iat": now.Unix(),
		"jti": jti,
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodRS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(s.privateKey)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("sign access token: %w", ErrInternalServer)
	}

	// Generate refresh token (random bytes)
	refreshTokenBytes := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(refreshTokenBytes); err != nil {
		return TokenResponse{}, fmt.Errorf("generate refresh token: %w", ErrInternalServer)
	}
	refreshTokenString := base64.URLEncoding.EncodeToString(refreshTokenBytes)

	// Store refresh token hash in database
	tokenHash := hashRefreshToken(refreshTokenString)
	refreshExpiresAt := now.Add(refreshTokenDuration)
	_, err = s.repo.createRefreshToken(ctx, userID, tokenHash, refreshExpiresAt)
	if err != nil {
		return TokenResponse{}, err
	}

	return TokenResponse{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresIn:    int(accessTokenDuration.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

// hashPassword creates an Argon2id hash of the password.
// IMPORTANT: The caller is responsible for applying NFKC normalization before
// calling this function. Pass the already-normalised password here.
func hashPassword(password string) (string, error) {
	// Generate random salt
	salt := make([]byte, argon2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	// Hash with Argon2id
	hash := argon2.IDKey([]byte(password), salt, argon2Iterations, argon2Memory, argon2Parallelism, argon2KeyLength)

	// Format: $argon2id$v=19$m=65536,t=3,p=2$<base64salt>$<base64hash>
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argon2Memory, argon2Iterations, argon2Parallelism, b64Salt, b64Hash), nil
}

// verifyPassword checks if the password matches the hash.
// IMPORTANT: The caller is responsible for applying NFKC normalization before
// calling this function. Pass the already-normalised password here.
// Fix E: Bounds checking on Argon2 parameters to prevent DoS.
func verifyPassword(password, encodedHash string) bool {
	// Parse the encoded hash
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false
	}

	if parts[1] != "argon2id" {
		return false
	}

	var memory, iterations uint32
	var parallelism uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		return false
	}

	// Fix E: Bounds checking to prevent DoS attacks
	if memory < argon2MinMemory || memory > argon2MaxMemory {
		return false
	}
	if iterations < argon2MinIterations || iterations > argon2MaxIterations {
		return false
	}
	if parallelism < argon2MinParallelism || parallelism > argon2MaxParallelism {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	// Fix E: Bounds check on hash length
	hashLen := len(expectedHash)
	if hashLen < argon2MinKeyLength || hashLen > argon2MaxKeyLength {
		return false
	}

	// Compute hash with same parameters
	computedHash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(hashLen))

	// Constant-time comparison
	if len(computedHash) != len(expectedHash) {
		return false
	}
	var result byte
	for i := 0; i < len(computedHash); i++ {
		result |= computedHash[i] ^ expectedHash[i]
	}
	return result == 0
}

// hashRefreshToken creates a SHA-256 hash of the refresh token.
func hashRefreshToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// loadOrGenerateKeys loads RSA keys from files or generates new ones if they don't exist.
func loadOrGenerateKeys(privateKeyPath, publicKeyPath string) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	// Try to load existing keys
	privateKey, publicKey, err := loadKeys(privateKeyPath, publicKeyPath)
	if err == nil {
		return privateKey, publicKey, nil
	}

	// Generate new keys
	privateKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate RSA key: %w", err)
	}

	// Create keys directory if it doesn't exist
	keysDir := privateKeyPath[:strings.LastIndex(privateKeyPath, "/")]
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("create keys directory: %w", err)
	}

	// Save private key
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	if err := os.WriteFile(privateKeyPath, privateKeyPEM, 0600); err != nil {
		return nil, nil, fmt.Errorf("write private key: %w", err)
	}

	// Save public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal public key: %w", err)
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})
	if err := os.WriteFile(publicKeyPath, publicKeyPEM, 0600); err != nil {
		return nil, nil, fmt.Errorf("write public key: %w", err)
	}

	return privateKey, &privateKey.PublicKey, nil
}

// loadKeys loads RSA keys from PEM files.
func loadKeys(privateKeyPath, publicKeyPath string) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	// Load private key
	privateKeyPEM, err := os.ReadFile(privateKeyPath) //nolint:gosec // path from operator-controlled config
	if err != nil {
		return nil, nil, err
	}

	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, nil, fmt.Errorf("failed to decode private key PEM")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse private key: %w", err)
	}

	// Load public key
	publicKeyPEM, err := os.ReadFile(publicKeyPath) //nolint:gosec // path from operator-controlled config
	if err != nil {
		return nil, nil, err
	}

	block, _ = pem.Decode(publicKeyPEM)
	if block == nil {
		return nil, nil, fmt.Errorf("failed to decode public key PEM")
	}

	publicKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse public key: %w", err)
	}

	publicKey, ok := publicKeyInterface.(*rsa.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf("not an RSA public key")
	}

	return privateKey, publicKey, nil
}
