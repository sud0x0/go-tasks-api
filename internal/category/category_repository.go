package category

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Repository size limits (defence-in-depth validation).
const (
	repoMaxUserIDLength      = 64
	repoMaxNameLength        = 100
	repoMaxDescriptionLength = 500
)

// Prepared statement queries.
const (
	queryGetCategory     = `SELECT id, user_id, name, description, created_at, updated_at FROM categories WHERE id = $1 AND user_id = $2`
	queryGetCategories   = `SELECT id, user_id, name, description, created_at, updated_at FROM categories WHERE user_id = $1 ORDER BY name ASC LIMIT $2 OFFSET $3`
	queryCreateCategory  = `INSERT INTO categories (user_id, name, description) VALUES ($1, $2, $3) RETURNING id, user_id, name, description, created_at, updated_at`
	queryUpdateCategory  = `UPDATE categories SET name = $1, description = $2, updated_at = NOW() WHERE id = $3 AND user_id = $4 RETURNING id, user_id, name, description, created_at, updated_at`
	queryDeleteCategory  = `DELETE FROM categories WHERE id = $1 AND user_id = $2`
	queryCheckTasksExist = `SELECT EXISTS(SELECT 1 FROM tasks WHERE category_id = $1 AND user_id = $2 AND is_active = true)`
)

// categoryRepository defines the interface for category data access.
type categoryRepository interface {
	getCategory(ctx context.Context, id, userID string) (Category, error)
	getCategories(ctx context.Context, userID string, limit, offset int) ([]Category, error)
	createCategory(ctx context.Context, userID, name string, description *string) (Category, error)
	updateCategory(ctx context.Context, id, userID, name string, description *string) (Category, error)
	deleteCategory(ctx context.Context, id, userID string) error
	hasActiveTasks(ctx context.Context, categoryID, userID string) (bool, error)
	Close() error
}

// sqlCategoryRepository implements categoryRepository using a SQL database.
type sqlCategoryRepository struct {
	stmtGetCategory     *sql.Stmt
	stmtGetCategories   *sql.Stmt
	stmtCreateCategory  *sql.Stmt
	stmtUpdateCategory  *sql.Stmt
	stmtDeleteCategory  *sql.Stmt
	stmtCheckTasksExist *sql.Stmt
}

// NewCategoryRepository creates a new categoryRepository with prepared statements.
// Panics if any statement cannot be prepared — this is a fatal startup error.
func NewCategoryRepository(db *sql.DB, _ categoryLogger) categoryRepository {
	repo := &sqlCategoryRepository{}

	var err error

	repo.stmtGetCategory, err = db.Prepare(queryGetCategory)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare getCategory: %v", err))
	}

	repo.stmtGetCategories, err = db.Prepare(queryGetCategories)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare getCategories: %v", err))
	}

	repo.stmtCreateCategory, err = db.Prepare(queryCreateCategory)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare createCategory: %v", err))
	}

	repo.stmtUpdateCategory, err = db.Prepare(queryUpdateCategory)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare updateCategory: %v", err))
	}

	repo.stmtDeleteCategory, err = db.Prepare(queryDeleteCategory)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare deleteCategory: %v", err))
	}

	repo.stmtCheckTasksExist, err = db.Prepare(queryCheckTasksExist)
	if err != nil {
		panic(fmt.Sprintf("category_repository: failed to prepare checkTasksExist: %v", err))
	}

	return repo
}

// Close closes all prepared statements.
func (r *sqlCategoryRepository) Close() error {
	var errs []error
	for _, stmt := range []*sql.Stmt{
		r.stmtGetCategory,
		r.stmtGetCategories,
		r.stmtCreateCategory,
		r.stmtUpdateCategory,
		r.stmtDeleteCategory,
		r.stmtCheckTasksExist,
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
		&cat.ID, &cat.UserID, &cat.Name, &cat.Description,
		&cat.CreatedAt, &cat.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("getCategory %s: %w", id, ErrDatabase)
	}

	if err := cat.Validate(); err != nil {
		return Category{}, fmt.Errorf("getCategory validate %s: %w", id, ErrDatabase)
	}
	return cat, nil
}

func (r *sqlCategoryRepository) getCategories(ctx context.Context, userID string, limit, offset int) ([]Category, error) {
	if len(userID) > repoMaxUserIDLength {
		return []Category{}, nil
	}

	rows, err := r.stmtGetCategories.QueryContext(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getCategories: %w", ErrDatabase)
	}
	defer func() { _ = rows.Close() }()

	var categories []Category
	for rows.Next() {
		var cat Category
		if err := rows.Scan(
			&cat.ID, &cat.UserID, &cat.Name, &cat.Description,
			&cat.CreatedAt, &cat.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("getCategories scan: %w", ErrDatabase)
		}
		if err := cat.Validate(); err != nil {
			return nil, fmt.Errorf("getCategories validate: %w", ErrDatabase)
		}
		categories = append(categories, cat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getCategories rows: %w", ErrDatabase)
	}

	if len(categories) == 0 {
		return []Category{}, nil
	}
	return categories, nil
}

func (r *sqlCategoryRepository) createCategory(ctx context.Context, userID, name string, description *string) (Category, error) {
	if len(userID) > repoMaxUserIDLength {
		return Category{}, fmt.Errorf("createCategory: %w", ErrDatabase)
	}
	if len(name) > repoMaxNameLength {
		return Category{}, ErrNameTooLong
	}
	if description != nil && len(*description) > repoMaxDescriptionLength {
		return Category{}, ErrDescriptionTooLong
	}

	var cat Category
	err := r.stmtCreateCategory.QueryRowContext(ctx, userID, name, description).Scan(
		&cat.ID, &cat.UserID, &cat.Name, &cat.Description,
		&cat.CreatedAt, &cat.UpdatedAt,
	)
	if err != nil {
		return Category{}, fmt.Errorf("createCategory: %w", ErrDatabase)
	}

	if err := cat.Validate(); err != nil {
		return Category{}, fmt.Errorf("createCategory validate: %w", ErrDatabase)
	}
	return cat, nil
}

func (r *sqlCategoryRepository) updateCategory(ctx context.Context, id, userID, name string, description *string) (Category, error) {
	if len(userID) > repoMaxUserIDLength {
		return Category{}, ErrCategoryNotFound
	}
	if len(name) > repoMaxNameLength {
		return Category{}, ErrNameTooLong
	}
	if description != nil && len(*description) > repoMaxDescriptionLength {
		return Category{}, ErrDescriptionTooLong
	}

	var cat Category
	err := r.stmtUpdateCategory.QueryRowContext(ctx, name, description, id, userID).Scan(
		&cat.ID, &cat.UserID, &cat.Name, &cat.Description,
		&cat.CreatedAt, &cat.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("updateCategory %s: %w", id, ErrDatabase)
	}

	if err := cat.Validate(); err != nil {
		return Category{}, fmt.Errorf("updateCategory validate %s: %w", id, ErrDatabase)
	}
	return cat, nil
}

func (r *sqlCategoryRepository) deleteCategory(ctx context.Context, id, userID string) error {
	if len(userID) > repoMaxUserIDLength {
		return ErrCategoryNotFound
	}

	result, err := r.stmtDeleteCategory.ExecContext(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("deleteCategory %s: %w", id, ErrDatabase)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("deleteCategory rowsAffected %s: %w", id, ErrDatabase)
	}

	if rowsAffected == 0 {
		return ErrCategoryNotFound
	}
	return nil
}

func (r *sqlCategoryRepository) hasActiveTasks(ctx context.Context, categoryID, userID string) (bool, error) {
	var exists bool
	err := r.stmtCheckTasksExist.QueryRowContext(ctx, categoryID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("hasActiveTasks %s: %w", categoryID, ErrDatabase)
	}
	return exists, nil
}
