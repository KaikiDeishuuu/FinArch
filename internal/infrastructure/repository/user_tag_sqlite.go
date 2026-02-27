package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"finarch/internal/domain/model"
)

// SQLiteUserRepository stores users in SQLite.
type SQLiteUserRepository struct{ db *sql.DB }

func NewSQLiteUserRepository(db *sql.DB) *SQLiteUserRepository {
	return &SQLiteUserRepository{db: db}
}

func (r *SQLiteUserRepository) Create(ctx context.Context, u model.User) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO users (id, email, name, password_hash, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.Name, u.PasswordHash, u.Role,
		u.CreatedAt.Unix(), u.UpdatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (r *SQLiteUserRepository) GetByEmail(ctx context.Context, email string) (model.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, email, name, password_hash, role, created_at, updated_at
		 FROM users WHERE email = ? AND deleted_at IS NULL`, email)
	return scanUser(row)
}

func (r *SQLiteUserRepository) GetByID(ctx context.Context, id string) (model.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, email, name, password_hash, role, created_at, updated_at
		 FROM users WHERE id = ? AND deleted_at IS NULL`, id)
	return scanUser(row)
}

func scanUser(row *sql.Row) (model.User, error) {
	var u model.User
	var createdAt, updatedAt int64
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return model.User{}, fmt.Errorf("user not found")
		}
		return model.User{}, fmt.Errorf("scan user: %w", err)
	}
	u.CreatedAt = time.Unix(createdAt, 0)
	u.UpdatedAt = time.Unix(updatedAt, 0)
	return u, nil
}

// SQLiteTagRepository stores tags in SQLite.
type SQLiteTagRepository struct{ db *sql.DB }

func NewSQLiteTagRepository(db *sql.DB) *SQLiteTagRepository {
	return &SQLiteTagRepository{db: db}
}

func (r *SQLiteTagRepository) Create(ctx context.Context, t model.Tag) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO tags (id, owner_id, name, color, created_at) VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.OwnerID, t.Name, t.Color, t.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert tag: %w", err)
	}
	return nil
}

func (r *SQLiteTagRepository) ListByOwner(ctx context.Context, ownerID string) ([]model.Tag, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, owner_id, name, color, created_at FROM tags WHERE owner_id = ? ORDER BY name`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()
	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		var createdAt int64
		if err := rows.Scan(&t.ID, &t.OwnerID, &t.Name, &t.Color, &createdAt); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		t.CreatedAt = time.Unix(createdAt, 0)
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (r *SQLiteTagRepository) Delete(ctx context.Context, id, ownerID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ? AND owner_id = ?`, id, ownerID)
	return err
}

func (r *SQLiteTagRepository) AddToTransaction(ctx context.Context, transactionID, tagID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO transaction_tags(transaction_id, tag_id) VALUES(?, ?)`,
		transactionID, tagID,
	)
	return err
}

func (r *SQLiteTagRepository) RemoveFromTransaction(ctx context.Context, transactionID, tagID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM transaction_tags WHERE transaction_id = ? AND tag_id = ?`,
		transactionID, tagID,
	)
	return err
}

func (r *SQLiteTagRepository) ListByTransaction(ctx context.Context, transactionID string) ([]model.Tag, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.owner_id, t.name, t.color, t.created_at
		FROM tags t
		JOIN transaction_tags tt ON tt.tag_id = t.id
		WHERE tt.transaction_id = ?`, transactionID)
	if err != nil {
		return nil, fmt.Errorf("list transaction tags: %w", err)
	}
	defer rows.Close()
	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		var createdAt int64
		if err := rows.Scan(&t.ID, &t.OwnerID, &t.Name, &t.Color, &createdAt); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(createdAt, 0)
		tags = append(tags, t)
	}
	return tags, rows.Err()
}
