package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"finarch/internal/domain/model"
)

// SQLiteCategoryRepository stores categories in SQLite.
type SQLiteCategoryRepository struct {
	db *sql.DB
}

// NewSQLiteCategoryRepository creates a new category repository.
func NewSQLiteCategoryRepository(db *sql.DB) *SQLiteCategoryRepository {
	return &SQLiteCategoryRepository{db: db}
}

// Create inserts one category.
func (r *SQLiteCategoryRepository) Create(ctx context.Context, c model.Category) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO categories (id, user_id, name, type, parent_id, sort_order, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.UserID, c.Name, c.Type, c.ParentID, c.SortOrder, boolToInt(c.IsActive),
	)
	if err != nil {
		return fmt.Errorf("create category: %w", err)
	}
	return nil
}

// ListByUser returns all active categories for a user.
func (r *SQLiteCategoryRepository) ListByUser(ctx context.Context, userID string) ([]model.Category, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, name, type, parent_id, sort_order, is_active
		 FROM categories WHERE user_id = ? AND is_active = 1
		 ORDER BY type, sort_order, name`, userID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()
	var out []model.Category
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetByID loads one category.
func (r *SQLiteCategoryRepository) GetByID(ctx context.Context, id string) (model.Category, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, type, parent_id, sort_order, is_active FROM categories WHERE id = ?`, id)
	return scanCategory(row)
}

// Update saves mutable fields.
func (r *SQLiteCategoryRepository) Update(ctx context.Context, c model.Category) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE categories SET name = ?, sort_order = ?, is_active = ? WHERE id = ? AND user_id = ?`,
		c.Name, c.SortOrder, boolToInt(c.IsActive), c.ID, c.UserID,
	)
	return err
}

// Delete removes a category.
func (r *SQLiteCategoryRepository) Delete(ctx context.Context, id string, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM categories WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

type catScanner interface {
	Scan(dest ...any) error
}

func scanCategory(s catScanner) (model.Category, error) {
	var c model.Category
	var parentID sql.NullString
	var isActive int
	if err := s.Scan(
		&c.ID, &c.UserID, &c.Name, &c.Type, &parentID, &c.SortOrder, &isActive,
	); err != nil {
		if err == sql.ErrNoRows {
			return model.Category{}, fmt.Errorf("category not found")
		}
		return model.Category{}, fmt.Errorf("scan category: %w", err)
	}
	c.IsActive = isActive == 1
	if parentID.Valid {
		v := parentID.String
		c.ParentID = &v
	}
	c.CreatedAt = time.Time{} // not stored in current schema; zero value
	return c, nil
}
