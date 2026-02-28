package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"finarch/internal/domain/model"

	"github.com/google/uuid"
)

// SQLiteUserRepository stores users in SQLite.
type SQLiteUserRepository struct{ db *sql.DB }

func NewSQLiteUserRepository(db *sql.DB) *SQLiteUserRepository {
	return &SQLiteUserRepository{db: db}
}

func (r *SQLiteUserRepository) Create(ctx context.Context, u model.User) error {
	verified := 0
	if u.EmailVerified {
		verified = 1
	}
	_, err := r.db.ExecContext(ctx, `
			INSERT INTO users (id, email, name, password_hash, role, email_verified, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.Name, u.PasswordHash, u.Role, verified,
		u.CreatedAt.Unix(), u.UpdatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (r *SQLiteUserRepository) GetByEmail(ctx context.Context, email string) (model.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, email, name, password_hash, role, email_verified, created_at, updated_at
			 FROM users WHERE email = ? AND deleted_at IS NULL`, email)
	return scanUser(row)
}

func (r *SQLiteUserRepository) GetByID(ctx context.Context, id string) (model.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, email, name, password_hash, role, email_verified, created_at, updated_at
			 FROM users WHERE id = ? AND deleted_at IS NULL`, id)
	return scanUser(row)
}

func (r *SQLiteUserRepository) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash, time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

func (r *SQLiteUserRepository) SetEmailVerified(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET email_verified = 1, updated_at = ? WHERE id = ?`,
		time.Now().Unix(), id,
	)
	return err
}

func (r *SQLiteUserRepository) CreateEmailToken(ctx context.Context, t model.EmailToken) error {
	if t.Token == "" {
		t.Token = uuid.NewString()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO email_tokens (token, user_id, kind, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`,
		t.Token, t.UserID, t.Kind, t.ExpiresAt.Unix(), t.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("create email token: %w", err)
	}
	return nil
}

func (r *SQLiteUserRepository) GetEmailToken(ctx context.Context, token string) (model.EmailToken, error) {
	var t model.EmailToken
	var expiresAt, createdAt int64
	err := r.db.QueryRowContext(ctx,
		`SELECT token, user_id, kind, expires_at, created_at FROM email_tokens WHERE token = ?`, token,
	).Scan(&t.Token, &t.UserID, &t.Kind, &expiresAt, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.EmailToken{}, fmt.Errorf("token not found")
		}
		return model.EmailToken{}, fmt.Errorf("get email token: %w", err)
	}
	t.ExpiresAt = time.Unix(expiresAt, 0)
	t.CreatedAt = time.Unix(createdAt, 0)
	return t, nil
}

func (r *SQLiteUserRepository) DeleteEmailToken(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM email_tokens WHERE token = ?`, token)
	return err
}

func (r *SQLiteUserRepository) DeleteEmailTokensByUser(ctx context.Context, userID, kind string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM email_tokens WHERE user_id = ? AND kind = ?`, userID, kind,
	)
	return err
}

// DeleteUser permanently deletes the user and (via CASCADE) all related tokens.
// Transactions, tags, and fund_pools that reference the user are also cleaned up.
func (r *SQLiteUserRepository) DeleteUser(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete owned data first (no FK CASCADE on all tables)
	for _, q := range []string{
		`DELETE FROM email_tokens WHERE user_id = ?`,
		`DELETE FROM transaction_tags WHERE transaction_id IN (SELECT id FROM transactions WHERE owner_id = ?)`,
		`DELETE FROM transactions WHERE owner_id = ?`,
		`DELETE FROM tags WHERE owner_id = ?`,
		`DELETE FROM fund_pools WHERE owner_id = ?`,
		`DELETE FROM users WHERE id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return fmt.Errorf("delete user data: %w", err)
		}
	}
	return tx.Commit()
}

func scanUser(row *sql.Row) (model.User, error) {
	var u model.User
	var createdAt, updatedAt int64
	var verified int
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &verified, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return model.User{}, fmt.Errorf("user not found")
		}
		return model.User{}, fmt.Errorf("scan user: %w", err)
	}
	u.EmailVerified = verified == 1
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
