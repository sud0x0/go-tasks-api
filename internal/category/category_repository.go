package category

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5/pgconn"
)

// Repository size limits (defence-in-depth validation).
// Length validation for name and description is handled at the service layer
// using rune count to match PostgreSQL VARCHAR(n) character semantics.
const (
	repoMaxUserIDLength = 64
	repoColourLength    = 7
)

// colourValidationRegex validates hex colour format (accepts both cases for input validation).
var colourValidationRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// uniqueNameConstraint is the name of the unique index for per-user category names.
const uniqueNameConstraint = "idx_categories_user_lower_name_unique"

// Prepared statement queries.
const (
	queryGetCategory              = `SELECT id, user_id, name, description, colour, is_active, created_at, updated_at FROM categories WHERE id = $1 AND user_id = $2 AND is_active = true`
	queryGetCategories            = `SELECT id, user_id, name, description, colour, is_active, created_at, updated_at FROM categories WHERE user_id = $1 AND is_active = true ORDER BY name ASC LIMIT $2 OFFSET $3`
	queryGetInactiveCategories    = `SELECT id, user_id, name, description, colour, is_active, created_at, updated_at FROM categories WHERE user_id = $1 AND is_active = false ORDER BY name ASC LIMIT $2 OFFSET $3`
	queryCreateCategory           = `INSERT INTO categories (user_id, name, description, colour) VALUES ($1, $2, $3, $4) RETURNING id, user_id, name, description, colour, is_active, created_at, updated_at`
	queryUpdateCategory           = `UPDATE categories SET name = $1, description = $2, colour = COALESCE($3, colour), updated_at = NOW() WHERE id = $4 AND user_id = $5 AND is_active = true RETURNING id, user_id, name, description, colour, is_active, created_at, updated_at`
	queryDeactivateCategory       = `UPDATE categories SET is_active = false, updated_at = NOW() WHERE id = $1 AND user_id = $2 AND is_active = true`
	queryHardDeleteCategory       = `DELETE FROM categories WHERE id = $1 AND user_id = $2 AND is_active = false`
	queryReactivateCategory       = `UPDATE categories SET is_active = true, updated_at = NOW() WHERE id = $1 AND user_id = $2 AND is_active = false RETURNING id, user_id, name, description, colour, is_active, created_at, updated_at`
	queryCheckActiveNameExists    = `SELECT EXISTS(SELECT 1 FROM categories WHERE user_id = $1 AND LOWER(name) = LOWER($2) AND is_active = true AND id != $3)`
	queryGetCategoryForReactivate = `SELECT name FROM categories WHERE id = $1 AND user_id = $2 AND is_active = false`
	queryCheckOwnership           = `SELECT EXISTS(SELECT 1 FROM categories WHERE id = $1 AND user_id = $2)`
	queryGetCategoryIsActive      = `SELECT is_active FROM categories WHERE id = $1 AND user_id = $2`
	queryHasActiveTasks           = `SELECT EXISTS(SELECT 1 FROM tasks WHERE category_id = $1 AND user_id = $2 AND is_active = true)`
)

// categoryRepository defines the interface for category data access.
type categoryRepository interface {
	getCategory(ctx context.Context, id, userID string) (Category, error)
	getCategories(ctx context.Context, userID string, limit, offset int) ([]Category, error)
	getInactiveCategories(ctx context.Context, userID string, limit, offset int) ([]Category, error)
	createCategory(ctx context.Context, userID, name string, description *string, colour string) (Category, error)
	updateCategory(ctx context.Context, id, userID, name string, description *string, colour *string) (Category, error)
	deactivateCategory(ctx context.Context, id, userID string) error
	hardDeleteCategory(ctx context.Context, id, userID string) error
	bulkDeactivateCategories(ctx context.Context, userID string, ids []string) (int, error)
	bulkHardDeleteCategories(ctx context.Context, userID string, ids []string) (int, error)
	reactivateCategory(ctx context.Context, id, userID string) (Category, error)
	checkCategoryOwnership(ctx context.Context, id, userID string) error
	getCategoryIsActive(ctx context.Context, id, userID string) (bool, error)
	hasActiveTasks(ctx context.Context, id, userID string) (bool, error)
	Close() error
}

// sqlCategoryRepository implements categoryRepository using a SQL database.
type sqlCategoryRepository struct {
	db                           *sql.DB
	logger                       categoryLogger
	stmtGetCategory              *sql.Stmt
	stmtGetCategories            *sql.Stmt
	stmtGetInactiveCategories    *sql.Stmt
	stmtCreateCategory           *sql.Stmt
	stmtUpdateCategory           *sql.Stmt
	stmtDeactivateCategory       *sql.Stmt
	stmtHardDeleteCategory       *sql.Stmt
	stmtReactivateCategory       *sql.Stmt
	stmtCheckActiveNameExists    *sql.Stmt
	stmtGetCategoryForReactivate *sql.Stmt
	stmtCheckOwnership           *sql.Stmt
	stmtGetCategoryIsActive      *sql.Stmt
	stmtHasActiveTasks           *sql.Stmt
}

// NewCategoryRepository creates a new categoryRepository with prepared statements.
// Panics if any statement cannot be prepared — this is a fatal startup error.
func NewCategoryRepository(db *sql.DB, log categoryLogger) categoryRepository {
	repo := &sqlCategoryRepository{db: db, logger: log}

	var err error

	repo.stmtGetCategory, err = db.Prepare(queryGetCategory)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare getCategory: %v", err))
	}

	repo.stmtGetCategories, err = db.Prepare(queryGetCategories)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare getCategories: %v", err))
	}

	repo.stmtGetInactiveCategories, err = db.Prepare(queryGetInactiveCategories)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare getInactiveCategories: %v", err))
	}

	repo.stmtCreateCategory, err = db.Prepare(queryCreateCategory)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare createCategory: %v", err))
	}

	repo.stmtUpdateCategory, err = db.Prepare(queryUpdateCategory)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare updateCategory: %v", err))
	}

	repo.stmtDeactivateCategory, err = db.Prepare(queryDeactivateCategory)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare deactivateCategory: %v", err))
	}

	repo.stmtHardDeleteCategory, err = db.Prepare(queryHardDeleteCategory)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare hardDeleteCategory: %v", err))
	}

	repo.stmtReactivateCategory, err = db.Prepare(queryReactivateCategory)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare reactivateCategory: %v", err))
	}

	repo.stmtCheckActiveNameExists, err = db.Prepare(queryCheckActiveNameExists)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare checkActiveNameExists: %v", err))
	}

	repo.stmtGetCategoryForReactivate, err = db.Prepare(queryGetCategoryForReactivate)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare getCategoryForReactivate: %v", err))
	}

	repo.stmtCheckOwnership, err = db.Prepare(queryCheckOwnership)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare checkOwnership: %v", err))
	}

	repo.stmtGetCategoryIsActive, err = db.Prepare(queryGetCategoryIsActive)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare getCategoryIsActive: %v", err))
	}

	repo.stmtHasActiveTasks, err = db.Prepare(queryHasActiveTasks)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare hasActiveTasks: %v", err))
	}

	return repo
}

// Close closes all prepared statements.
func (r *sqlCategoryRepository) Close() error {
	var errs []error
	for _, stmt := range []*sql.Stmt{
		r.stmtGetCategory,
		r.stmtGetCategories,
		r.stmtGetInactiveCategories,
		r.stmtCreateCategory,
		r.stmtUpdateCategory,
		r.stmtDeactivateCategory,
		r.stmtHardDeleteCategory,
		r.stmtReactivateCategory,
		r.stmtCheckActiveNameExists,
		r.stmtGetCategoryForReactivate,
		r.stmtCheckOwnership,
		r.stmtGetCategoryIsActive,
		r.stmtHasActiveTasks,
	} {
		if err := stmt.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *sqlCategoryRepository) getCategory(ctx context.Context, id, userID string) (Category, error) {
	if len(userID) > repoMaxUserIDLength {
		return Category{}, ErrInvalidInput
	}

	var cat Category
	err := r.stmtGetCategory.QueryRowContext(ctx, id, userID).Scan(
		&cat.ID, &cat.UserID, &cat.Name, &cat.Description, &cat.Colour,
		&cat.IsActive, &cat.CreatedAt, &cat.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("getCategory %s: %w: %w", id, ErrDatabase, err)
	}

	if err := cat.Validate(); err != nil {
		return Category{}, fmt.Errorf("getCategory validate %s: %w: %w", id, ErrDatabase, err)
	}
	return cat, nil
}

func (r *sqlCategoryRepository) getCategories(ctx context.Context, userID string, limit, offset int) ([]Category, error) {
	if len(userID) > repoMaxUserIDLength {
		return []Category{}, nil
	}

	rows, err := r.stmtGetCategories.QueryContext(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getCategories: %w: %w", ErrDatabase, err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanCategories(rows, "getCategories")
}

func (r *sqlCategoryRepository) getInactiveCategories(ctx context.Context, userID string, limit, offset int) ([]Category, error) {
	if len(userID) > repoMaxUserIDLength {
		return []Category{}, nil
	}

	rows, err := r.stmtGetInactiveCategories.QueryContext(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getInactiveCategories: %w: %w", ErrDatabase, err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanCategories(rows, "getInactiveCategories")
}

func (r *sqlCategoryRepository) scanCategories(rows *sql.Rows, methodName string) ([]Category, error) {
	var categories []Category
	for rows.Next() {
		var cat Category
		if err := rows.Scan(
			&cat.ID, &cat.UserID, &cat.Name, &cat.Description, &cat.Colour,
			&cat.IsActive, &cat.CreatedAt, &cat.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("%s scan: %w: %w", methodName, ErrDatabase, err)
		}
		if err := cat.Validate(); err != nil {
			return nil, fmt.Errorf("%s validate: %w: %w", methodName, ErrDatabase, err)
		}
		categories = append(categories, cat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w: %w", methodName, ErrDatabase, err)
	}

	if len(categories) == 0 {
		return []Category{}, nil
	}
	return categories, nil
}

func (r *sqlCategoryRepository) createCategory(ctx context.Context, userID, name string, description *string, colour string) (Category, error) {
	// Defence-in-depth: validate userID length and colour format.
	// Name/description length validation is handled at the service layer.
	if len(userID) > repoMaxUserIDLength {
		return Category{}, fmt.Errorf("createCategory: %w", ErrDatabase)
	}
	if len(colour) != repoColourLength || !colourValidationRegex.MatchString(colour) {
		return Category{}, ErrInvalidColour
	}

	var cat Category
	err := r.stmtCreateCategory.QueryRowContext(ctx, userID, name, description, colour).Scan(
		&cat.ID, &cat.UserID, &cat.Name, &cat.Description, &cat.Colour,
		&cat.IsActive, &cat.CreatedAt, &cat.UpdatedAt,
	)
	if err != nil {
		// Check for unique constraint violation
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == uniqueNameConstraint {
				return Category{}, ErrDuplicateName
			}
			// Unknown unique constraint - log and return database error
			r.logger.LogError(ErrDatabase, fmt.Errorf("createCategory unknown unique violation: %s", pgErr.ConstraintName))
			return Category{}, fmt.Errorf("createCategory: %w: %w", ErrDatabase, err)
		}
		return Category{}, fmt.Errorf("createCategory: %w: %w", ErrDatabase, err)
	}

	if err := cat.Validate(); err != nil {
		return Category{}, fmt.Errorf("createCategory validate: %w: %w", ErrDatabase, err)
	}
	return cat, nil
}

func (r *sqlCategoryRepository) updateCategory(ctx context.Context, id, userID, name string, description *string, colour *string) (Category, error) {
	// Defence-in-depth: validate userID length and colour format.
	// Name/description length validation is handled at the service layer.
	if len(userID) > repoMaxUserIDLength {
		return Category{}, ErrCategoryNotFound
	}
	if colour != nil {
		if len(*colour) != repoColourLength || !colourValidationRegex.MatchString(*colour) {
			return Category{}, ErrInvalidColour
		}
	}

	// Convert *string to sql.NullString for COALESCE handling
	var colourParam sql.NullString
	if colour != nil {
		colourParam = sql.NullString{String: *colour, Valid: true}
	}

	var cat Category
	err := r.stmtUpdateCategory.QueryRowContext(ctx, name, description, colourParam, id, userID).Scan(
		&cat.ID, &cat.UserID, &cat.Name, &cat.Description, &cat.Colour,
		&cat.IsActive, &cat.CreatedAt, &cat.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		// Check for unique constraint violation
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == uniqueNameConstraint {
				return Category{}, ErrDuplicateName
			}
			// Unknown unique constraint - log and return database error
			r.logger.LogError(ErrDatabase, fmt.Errorf("updateCategory unknown unique violation: %s", pgErr.ConstraintName))
			return Category{}, fmt.Errorf("updateCategory %s: %w: %w", id, ErrDatabase, err)
		}
		return Category{}, fmt.Errorf("updateCategory %s: %w: %w", id, ErrDatabase, err)
	}

	if err := cat.Validate(); err != nil {
		return Category{}, fmt.Errorf("updateCategory validate %s: %w: %w", id, ErrDatabase, err)
	}
	return cat, nil
}

func (r *sqlCategoryRepository) deactivateCategory(ctx context.Context, id, userID string) error {
	if len(userID) > repoMaxUserIDLength {
		return ErrCategoryNotFound
	}

	result, err := r.stmtDeactivateCategory.ExecContext(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("deactivateCategory %s: %w: %w", id, ErrDatabase, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("deactivateCategory rowsAffected %s: %w: %w", id, ErrDatabase, err)
	}

	if rowsAffected == 0 {
		return ErrCategoryNotFound
	}
	return nil
}

func (r *sqlCategoryRepository) hardDeleteCategory(ctx context.Context, id, userID string) error {
	if len(userID) > repoMaxUserIDLength {
		return ErrCategoryNotFound
	}

	result, err := r.stmtHardDeleteCategory.ExecContext(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("hardDeleteCategory %s: %w: %w", id, ErrDatabase, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("hardDeleteCategory rowsAffected %s: %w: %w", id, ErrDatabase, err)
	}

	if rowsAffected == 0 {
		return ErrCategoryNotFound
	}
	return nil
}

func (r *sqlCategoryRepository) bulkDeactivateCategories(ctx context.Context, userID string, ids []string) (int, error) {
	if len(userID) > repoMaxUserIDLength {
		return 0, nil
	}

	query := `UPDATE categories SET is_active = false, updated_at = NOW() WHERE user_id = $1 AND id = ANY($2::uuid[]) AND is_active = true`
	result, err := r.db.ExecContext(ctx, query, userID, ids)
	if err != nil {
		return 0, fmt.Errorf("bulkDeactivateCategories: %w: %w", ErrDatabase, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("bulkDeactivateCategories rowsAffected: %w: %w", ErrDatabase, err)
	}

	return int(rowsAffected), nil
}

func (r *sqlCategoryRepository) bulkHardDeleteCategories(ctx context.Context, userID string, ids []string) (int, error) {
	if len(userID) > repoMaxUserIDLength {
		return 0, nil
	}

	query := `DELETE FROM categories WHERE user_id = $1 AND id = ANY($2::uuid[]) AND is_active = false`
	result, err := r.db.ExecContext(ctx, query, userID, ids)
	if err != nil {
		return 0, fmt.Errorf("bulkHardDeleteCategories: %w: %w", ErrDatabase, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("bulkHardDeleteCategories rowsAffected: %w: %w", ErrDatabase, err)
	}

	return int(rowsAffected), nil
}

func (r *sqlCategoryRepository) reactivateCategory(ctx context.Context, id, userID string) (Category, error) {
	if len(userID) > repoMaxUserIDLength {
		return Category{}, ErrCategoryNotFound
	}

	// First get the category name to check for collision
	var name string
	err := r.stmtGetCategoryForReactivate.QueryRowContext(ctx, id, userID).Scan(&name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("reactivateCategory getName %s: %w: %w", id, ErrDatabase, err)
	}

	// Check if another active category has this name
	var exists bool
	err = r.stmtCheckActiveNameExists.QueryRowContext(ctx, userID, name, id).Scan(&exists)
	if err != nil {
		return Category{}, fmt.Errorf("reactivateCategory checkName %s: %w: %w", id, ErrDatabase, err)
	}
	if exists {
		return Category{}, ErrReactivateNameCollision
	}

	// Perform the reactivation
	var cat Category
	err = r.stmtReactivateCategory.QueryRowContext(ctx, id, userID).Scan(
		&cat.ID, &cat.UserID, &cat.Name, &cat.Description, &cat.Colour,
		&cat.IsActive, &cat.CreatedAt, &cat.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("reactivateCategory %s: %w: %w", id, ErrDatabase, err)
	}

	if err := cat.Validate(); err != nil {
		return Category{}, fmt.Errorf("reactivateCategory validate %s: %w: %w", id, ErrDatabase, err)
	}
	return cat, nil
}

// checkCategoryOwnership verifies that a category exists and belongs to the user.
// Unlike getCategory, this method does not validate the stored data, allowing
// operations like delete to proceed even if pre-existing data fails current validation rules.
func (r *sqlCategoryRepository) checkCategoryOwnership(ctx context.Context, id, userID string) error {
	if len(userID) > repoMaxUserIDLength {
		return ErrInvalidInput
	}

	var exists bool
	err := r.stmtCheckOwnership.QueryRowContext(ctx, id, userID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("checkCategoryOwnership %s: %w: %w", id, ErrDatabase, err)
	}

	if !exists {
		return ErrCategoryNotFound
	}
	return nil
}

// getCategoryIsActive returns whether a category is active.
// Returns ErrCategoryNotFound if the category doesn't exist or doesn't belong to the user.
func (r *sqlCategoryRepository) getCategoryIsActive(ctx context.Context, id, userID string) (bool, error) {
	if len(userID) > repoMaxUserIDLength {
		return false, ErrCategoryNotFound
	}

	var isActive bool
	err := r.stmtGetCategoryIsActive.QueryRowContext(ctx, id, userID).Scan(&isActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrCategoryNotFound
		}
		return false, fmt.Errorf("getCategoryIsActive %s: %w: %w", id, ErrDatabase, err)
	}

	return isActive, nil
}

// hasActiveTasks returns whether a category has any active tasks.
func (r *sqlCategoryRepository) hasActiveTasks(ctx context.Context, id, userID string) (bool, error) {
	if len(userID) > repoMaxUserIDLength {
		return false, ErrInvalidInput
	}

	var exists bool
	err := r.stmtHasActiveTasks.QueryRowContext(ctx, id, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("hasActiveTasks %s: %w: %w", id, ErrDatabase, err)
	}

	return exists, nil
}
