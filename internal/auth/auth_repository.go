package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL SQLSTATE code for unique_violation
// (https://www.postgresql.org/docs/current/errcodes-appendix.html).
const pgErrCodeUniqueViolation = "23505"

// Repository size limits (defence-in-depth validation).
const (
	repoMaxUsernameLength  = 50
	repoMaxPasswordLength  = 255 // Argon2id hash length
	repoMaxTokenHashLength = 255
)

// Prepared statement queries.
const (
	queryGetUserByID        = `SELECT id, username, password, created_at, updated_at FROM users WHERE id = $1`
	queryGetUserByUsername  = `SELECT id, username, password, created_at, updated_at FROM users WHERE username = $1`
	queryCreateUser         = `INSERT INTO users (username, password) VALUES ($1, $2) RETURNING id, username, password, created_at, updated_at`
	queryCreateToken        = `INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3) RETURNING id, user_id, token_hash, expires_at, created_at` //nolint:gosec // SQL query, not a credential
	queryGetTokenByHash     = `SELECT id, user_id, token_hash, expires_at, created_at FROM refresh_tokens WHERE token_hash = $1`                                           //nolint:gosec // SQL query, not a credential
	queryDeleteToken        = `DELETE FROM refresh_tokens WHERE token_hash = $1`                                                                                           //nolint:gosec // SQL query, not a credential
	queryDeleteTokenForUser = `DELETE FROM refresh_tokens WHERE token_hash = $1 AND user_id = $2`                                                                          //nolint:gosec // SQL query, not a credential
	queryDeleteUserTokens   = `DELETE FROM refresh_tokens WHERE user_id = $1`                                                                                              //nolint:gosec // SQL query, not a credential
	queryCleanExpiredTokens = `DELETE FROM refresh_tokens WHERE expires_at < NOW()`                                                                                        //nolint:gosec // SQL query, not a credential
)

// authRepository defines the interface for auth data access.
type authRepository interface {
	getUserByID(ctx context.Context, id string) (User, error)
	getUserByUsername(ctx context.Context, username string) (User, error)
	createUser(ctx context.Context, username, passwordHash string) (User, error)
	createRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (RefreshToken, error)
	getRefreshTokenByHash(ctx context.Context, tokenHash string) (RefreshToken, error)
	deleteRefreshToken(ctx context.Context, tokenHash string) error
	deleteRefreshTokenForUser(ctx context.Context, tokenHash, userID string) error
	deleteUserRefreshTokens(ctx context.Context, userID string) error
	CleanExpiredTokens(ctx context.Context) error
	Close() error
}

// sqlAuthRepository implements authRepository using a SQL database.
type sqlAuthRepository struct {
	stmtGetUserByID        *sql.Stmt
	stmtGetUserByUsername  *sql.Stmt
	stmtCreateUser         *sql.Stmt
	stmtCreateToken        *sql.Stmt
	stmtGetTokenByHash     *sql.Stmt
	stmtDeleteToken        *sql.Stmt
	stmtDeleteTokenForUser *sql.Stmt
	stmtDeleteUserTokens   *sql.Stmt
	stmtCleanExpiredTokens *sql.Stmt
}

// NewAuthRepository creates a new authRepository with prepared statements.
// Panics if any statement cannot be prepared — this is a fatal startup error.
func NewAuthRepository(db *sql.DB, _ authLogger) authRepository {
	repo := &sqlAuthRepository{}

	var err error

	repo.stmtGetUserByID, err = db.Prepare(queryGetUserByID)
	if err != nil {
		panic(fmt.Sprintf("auth_repository: failed to prepare getUserByID: %v", err))
	}

	repo.stmtGetUserByUsername, err = db.Prepare(queryGetUserByUsername)
	if err != nil {
		panic(fmt.Sprintf("auth_repository: failed to prepare getUserByUsername: %v", err))
	}

	repo.stmtCreateUser, err = db.Prepare(queryCreateUser)
	if err != nil {
		panic(fmt.Sprintf("auth_repository: failed to prepare createUser: %v", err))
	}

	repo.stmtCreateToken, err = db.Prepare(queryCreateToken)
	if err != nil {
		panic(fmt.Sprintf("auth_repository: failed to prepare createToken: %v", err))
	}

	repo.stmtGetTokenByHash, err = db.Prepare(queryGetTokenByHash)
	if err != nil {
		panic(fmt.Sprintf("auth_repository: failed to prepare getTokenByHash: %v", err))
	}

	repo.stmtDeleteToken, err = db.Prepare(queryDeleteToken)
	if err != nil {
		panic(fmt.Sprintf("auth_repository: failed to prepare deleteToken: %v", err))
	}

	repo.stmtDeleteTokenForUser, err = db.Prepare(queryDeleteTokenForUser)
	if err != nil {
		panic(fmt.Sprintf("auth_repository: failed to prepare deleteTokenForUser: %v", err))
	}

	repo.stmtDeleteUserTokens, err = db.Prepare(queryDeleteUserTokens)
	if err != nil {
		panic(fmt.Sprintf("auth_repository: failed to prepare deleteUserTokens: %v", err))
	}

	repo.stmtCleanExpiredTokens, err = db.Prepare(queryCleanExpiredTokens)
	if err != nil {
		panic(fmt.Sprintf("auth_repository: failed to prepare cleanExpiredTokens: %v", err))
	}

	return repo
}

// Close closes all prepared statements.
func (r *sqlAuthRepository) Close() error {
	var errs []error
	for _, stmt := range []*sql.Stmt{
		r.stmtGetUserByID,
		r.stmtGetUserByUsername,
		r.stmtCreateUser,
		r.stmtCreateToken,
		r.stmtGetTokenByHash,
		r.stmtDeleteToken,
		r.stmtDeleteTokenForUser,
		r.stmtDeleteUserTokens,
		r.stmtCleanExpiredTokens,
	} {
		if err := stmt.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *sqlAuthRepository) getUserByID(ctx context.Context, id string) (User, error) {
	var user User
	err := r.stmtGetUserByID.QueryRowContext(ctx, id).Scan(
		&user.ID, &user.Username, &user.Password,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("getUserByID %s: %w: %w", id, ErrDatabase, err)
	}

	if err := user.Validate(); err != nil {
		return User{}, fmt.Errorf("getUserByID validate %s: %w: %w", id, ErrDatabase, err)
	}
	return user, nil
}

func (r *sqlAuthRepository) getUserByUsername(ctx context.Context, username string) (User, error) {
	if len(username) > repoMaxUsernameLength {
		return User{}, ErrUserNotFound
	}

	var user User
	err := r.stmtGetUserByUsername.QueryRowContext(ctx, username).Scan(
		&user.ID, &user.Username, &user.Password,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("getUserByUsername: %w: %w", ErrDatabase, err)
	}

	if err := user.Validate(); err != nil {
		return User{}, fmt.Errorf("getUserByUsername validate: %w: %w", ErrDatabase, err)
	}
	return user, nil
}

func (r *sqlAuthRepository) createUser(ctx context.Context, username, passwordHash string) (User, error) {
	if len(username) > repoMaxUsernameLength {
		return User{}, ErrUsernameTooLong
	}
	if len(passwordHash) > repoMaxPasswordLength {
		return User{}, fmt.Errorf("createUser: %w", ErrDatabase)
	}

	var user User
	err := r.stmtCreateUser.QueryRowContext(ctx, username, passwordHash).Scan(
		&user.ID, &user.Username, &user.Password,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgErrCodeUniqueViolation {
			return User{}, ErrUserExists
		}
		return User{}, fmt.Errorf("createUser: %w: %w", ErrDatabase, err)
	}

	if err := user.Validate(); err != nil {
		return User{}, fmt.Errorf("createUser validate: %w: %w", ErrDatabase, err)
	}
	return user, nil
}

func (r *sqlAuthRepository) createRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (RefreshToken, error) {
	if len(tokenHash) > repoMaxTokenHashLength {
		return RefreshToken{}, fmt.Errorf("createRefreshToken: %w", ErrDatabase)
	}

	var token RefreshToken
	err := r.stmtCreateToken.QueryRowContext(ctx, userID, tokenHash, expiresAt).Scan(
		&token.ID, &token.UserID, &token.TokenHash,
		&token.ExpiresAt, &token.CreatedAt,
	)
	if err != nil {
		return RefreshToken{}, fmt.Errorf("createRefreshToken: %w: %w", ErrDatabase, err)
	}

	if err := token.Validate(); err != nil {
		return RefreshToken{}, fmt.Errorf("createRefreshToken validate: %w: %w", ErrDatabase, err)
	}
	return token, nil
}

func (r *sqlAuthRepository) getRefreshTokenByHash(ctx context.Context, tokenHash string) (RefreshToken, error) {
	if len(tokenHash) > repoMaxTokenHashLength {
		return RefreshToken{}, ErrInvalidToken
	}

	var token RefreshToken
	err := r.stmtGetTokenByHash.QueryRowContext(ctx, tokenHash).Scan(
		&token.ID, &token.UserID, &token.TokenHash,
		&token.ExpiresAt, &token.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RefreshToken{}, ErrInvalidToken
		}
		return RefreshToken{}, fmt.Errorf("getRefreshTokenByHash: %w: %w", ErrDatabase, err)
	}

	if err := token.Validate(); err != nil {
		return RefreshToken{}, fmt.Errorf("getRefreshTokenByHash validate: %w: %w", ErrDatabase, err)
	}
	return token, nil
}

func (r *sqlAuthRepository) deleteRefreshToken(ctx context.Context, tokenHash string) error {
	if len(tokenHash) > repoMaxTokenHashLength {
		return nil // No error, token simply doesn't exist
	}

	_, err := r.stmtDeleteToken.ExecContext(ctx, tokenHash)
	if err != nil {
		return fmt.Errorf("deleteRefreshToken: %w: %w", ErrDatabase, err)
	}
	return nil
}

// deleteRefreshTokenForUser deletes a refresh token only if it belongs to the specified user.
// Returns ErrTokenOwnershipMismatch if the token doesn't exist or doesn't belong to the user.
func (r *sqlAuthRepository) deleteRefreshTokenForUser(ctx context.Context, tokenHash, userID string) error {
	if len(tokenHash) > repoMaxTokenHashLength {
		return ErrTokenOwnershipMismatch
	}

	result, err := r.stmtDeleteTokenForUser.ExecContext(ctx, tokenHash, userID)
	if err != nil {
		return fmt.Errorf("deleteRefreshTokenForUser: %w: %w", ErrDatabase, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("deleteRefreshTokenForUser rows: %w: %w", ErrDatabase, err)
	}

	if rowsAffected == 0 {
		// Token doesn't exist or doesn't belong to this user
		return ErrTokenOwnershipMismatch
	}

	return nil
}

func (r *sqlAuthRepository) deleteUserRefreshTokens(ctx context.Context, userID string) error {
	_, err := r.stmtDeleteUserTokens.ExecContext(ctx, userID)
	if err != nil {
		return fmt.Errorf("deleteUserRefreshTokens: %w: %w", ErrDatabase, err)
	}
	return nil
}

func (r *sqlAuthRepository) CleanExpiredTokens(ctx context.Context) error {
	_, err := r.stmtCleanExpiredTokens.ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("CleanExpiredTokens: %w: %w", ErrDatabase, err)
	}
	return nil
}
