package repository

import (
	"context"

	"finarch/internal/domain/model"
)

// ProjectRepository defines project data access behavior.
type ProjectRepository interface {
	// Create inserts one project.
	Create(ctx context.Context, project model.Project) error
}
