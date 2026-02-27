package repository

import (
	"context"
	"database/sql"
	"fmt"

	"finarch/internal/domain/model"
)

// SQLiteProjectRepository stores projects in SQLite.
type SQLiteProjectRepository struct {
	db *sql.DB
}

// NewSQLiteProjectRepository creates a new project repository.
func NewSQLiteProjectRepository(db *sql.DB) *SQLiteProjectRepository {
	return &SQLiteProjectRepository{db: db}
}

// Create inserts one project.
func (r *SQLiteProjectRepository) Create(ctx context.Context, project model.Project) error {
	exec := getExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, `
		INSERT INTO projects (id, name, code, created_at)
		VALUES (?, ?, ?, ?)
	`, project.ID, project.Name, project.Code, project.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("insert project: %w", err)
	}
	return nil
}
