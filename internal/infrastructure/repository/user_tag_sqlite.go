package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/infrastructure/auth"

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
	// Sync name = username for backward compat.
	if u.Name == "" {
		u.Name = u.Username
	}
	if u.Nickname == "" {
		u.Nickname = u.Username
	}
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, `
			INSERT INTO users (id, email, name, username, nickname, password_hash, role, email_verified, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.Name, u.Username, u.Nickname, u.PasswordHash, u.Role, verified,
		u.CreatedAt.Unix(), u.UpdatedAt.Unix(),
	)
	if err != nil {
		if strings.Contains(err.Error(), "users.username") {
			return fmt.Errorf("username_taken")
		}
		if strings.Contains(err.Error(), "users.email") {
			return fmt.Errorf("email_taken")
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (r *SQLiteUserRepository) GetByEmail(ctx context.Context, email string) (model.User, error) {
	row := getExecutor(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, email, name, COALESCE(username,name), COALESCE(nickname,''), password_hash, role, email_verified, created_at, updated_at, COALESCE(pending_email,''), COALESCE(pwd_version,0)
			 FROM users WHERE email = ? AND deleted_at IS NULL`, email)
	return scanUser(row)
}

func (r *SQLiteUserRepository) GetByID(ctx context.Context, id string) (model.User, error) {
	row := getExecutor(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, email, name, COALESCE(username,name), COALESCE(nickname,''), password_hash, role, email_verified, created_at, updated_at, COALESCE(pending_email,''), COALESCE(pwd_version,0)
			 FROM users WHERE id = ? AND deleted_at IS NULL`, id)
	return scanUser(row)
}

func (r *SQLiteUserRepository) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE users SET password_hash = ?, pwd_version = pwd_version + 1, updated_at = ? WHERE id = ?`,
		passwordHash, time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

func (r *SQLiteUserRepository) SetEmailVerified(ctx context.Context, id string) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE users SET email_verified = 1, updated_at = ? WHERE id = ?`,
		time.Now().Unix(), id,
	)
	return err
}

func (r *SQLiteUserRepository) CreateEmailToken(ctx context.Context, t model.EmailToken) error {
	if t.Token == "" {
		t.Token = uuid.NewString()
	}
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`INSERT INTO email_tokens (token, user_id, kind, expires_at, created_at, meta) VALUES (?, ?, ?, ?, ?, ?)`,
		t.Token, t.UserID, t.Kind, t.ExpiresAt.Unix(), t.CreatedAt.Unix(), t.Meta,
	)
	if err != nil {
		return fmt.Errorf("create email token: %w", err)
	}
	return nil
}

func (r *SQLiteUserRepository) GetEmailToken(ctx context.Context, token string) (model.EmailToken, error) {
	var t model.EmailToken
	var expiresAt, createdAt int64
	err := getExecutor(ctx, r.db).QueryRowContext(ctx,
		`SELECT token, user_id, kind, expires_at, created_at, meta FROM email_tokens WHERE token = ?`, token,
	).Scan(&t.Token, &t.UserID, &t.Kind, &expiresAt, &createdAt, &t.Meta)
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

func (r *SQLiteUserRepository) SetPendingEmail(ctx context.Context, id, pendingEmail string) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE users SET pending_email = ?, updated_at = ? WHERE id = ?`,
		pendingEmail, time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("set pending email: %w", err)
	}
	return nil
}

func (r *SQLiteUserRepository) UpdateEmail(ctx context.Context, id, newEmail string) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE users SET email = ?, pending_email = NULL, updated_at = ? WHERE id = ?`,
		newEmail, time.Now().Unix(), id,
	)
	if err != nil {
		if strings.Contains(err.Error(), "users.email") {
			return fmt.Errorf("email_taken")
		}
		return fmt.Errorf("update email: %w", err)
	}
	return nil
}

func (r *SQLiteUserRepository) DeleteEmailToken(ctx context.Context, token string) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, `DELETE FROM email_tokens WHERE token = ?`, token)
	return err
}

func (r *SQLiteUserRepository) DeleteEmailTokensByUser(ctx context.Context, userID, kind string) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`DELETE FROM email_tokens WHERE user_id = ? AND kind = ?`, userID, kind,
	)
	return err
}

func (r *SQLiteUserRepository) CreateActionRequest(ctx context.Context, req model.ActionRequest) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`INSERT INTO action_requests (jti, user_id, action, status, meta, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.JTI, req.UserID, req.Action, req.Status, req.Meta, req.ExpiresAt.Unix(), req.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("create action request: %w", err)
	}
	return nil
}

func (r *SQLiteUserRepository) GetActionRequestByJTI(ctx context.Context, jti string) (model.ActionRequest, error) {
	var req model.ActionRequest
	var expiresAt, createdAt int64
	var usedAt sql.NullInt64
	err := getExecutor(ctx, r.db).QueryRowContext(ctx,
		`SELECT jti, user_id, action, status, meta, expires_at, used_at, created_at FROM action_requests WHERE jti = ?`,
		jti,
	).Scan(&req.JTI, &req.UserID, &req.Action, &req.Status, &req.Meta, &expiresAt, &usedAt, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.ActionRequest{}, fmt.Errorf("token not found")
		}
		return model.ActionRequest{}, fmt.Errorf("get action request: %w", err)
	}
	req.ExpiresAt = time.Unix(expiresAt, 0)
	req.CreatedAt = time.Unix(createdAt, 0)
	if usedAt.Valid {
		t := time.Unix(usedAt.Int64, 0)
		req.UsedAt = &t
	}
	return req, nil
}

func (r *SQLiteUserRepository) ConsumeActionRequest(ctx context.Context, jti string, consumedAt time.Time) (model.ActionRequest, error) {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE action_requests SET status = 'completed', used_at = ? WHERE jti = ? AND status = 'pending'`,
		consumedAt.Unix(), jti,
	)
	if err != nil {
		return model.ActionRequest{}, fmt.Errorf("consume action request: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return model.ActionRequest{}, auth.ErrTokenAlreadyUsed
	}
	return r.GetActionRequestByJTI(ctx, jti)
}

func (r *SQLiteUserRepository) ExpireActionRequests(ctx context.Context, action string, now time.Time) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE action_requests SET status = 'expired' WHERE action = ? AND status = 'pending' AND expires_at < ?`,
		action, now.Unix(),
	)
	if err != nil {
		return fmt.Errorf("expire action requests: %w", err)
	}
	return nil
}

func (r *SQLiteUserRepository) CreateAuditEvent(ctx context.Context, userID, eventType, ipAddr, deviceMeta string) error {
	payload, _ := json.Marshal(map[string]string{"event_type": eventType, "device": deviceMeta})
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`INSERT INTO audit_log (user_id, table_name, row_id, action, old_data, new_data, ip_addr, created_at)
		 VALUES (?, 'security_events', ?, 'INSERT', NULL, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
		userID, userID, string(payload), ipAddr,
	)
	if err != nil {
		return fmt.Errorf("create audit event: %w", err)
	}
	return nil
}

func (r *SQLiteUserRepository) UpdateNickname(ctx context.Context, id, nickname string) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE users SET nickname = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		nickname, time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("update nickname: %w", err)
	}
	return nil
}

// DeleteUser permanently deletes the user and all their data in one transaction.
func (r *SQLiteUserRepository) DeleteUser(ctx context.Context, id string) error {
	executor := getExecutor(ctx, r.db)
	res, err := executor.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user data: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("user not found")
	}

	for _, q := range []string{
		`DELETE FROM monthly_summary_cache WHERE user_id = ?`,
		`DELETE FROM audit_log WHERE user_id = ?`,
		`DELETE FROM account_deletion_requests WHERE user_id = ?`,
		`DELETE FROM action_requests WHERE user_id = ?`,
		`DELETE FROM transaction_tags WHERE transaction_id IN (SELECT id FROM transactions WHERE user_id = ?)`,
		`DELETE FROM tags WHERE owner_id = ?`,
		`DELETE FROM fund_pools WHERE owner_id = ?`,
	} {
		if _, err := executor.ExecContext(ctx, q, id); err != nil {
			return fmt.Errorf("delete user data: %w", err)
		}
	}
	return nil
}

// DeleteExpiredUnverifiedUsers removes unverified users whose created_at is before olderThan,
// along with all their owned data and tokens. Returns the count of deleted users.
func (r *SQLiteUserRepository) DeleteExpiredUnverifiedUsers(ctx context.Context, olderThan time.Time) (int64, error) {
	// Collect IDs of expired unverified users first.
	rows, err := getExecutor(ctx, r.db).QueryContext(ctx,
		`SELECT id FROM users WHERE email_verified = 0 AND created_at < ? AND deleted_at IS NULL`,
		olderThan.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("query expired unverified users: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan user id: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	// Delete each user in a transaction (reusing the same cascade logic as DeleteUser).
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, uid := range ids {
		for _, q := range []string{
			`DELETE FROM email_tokens WHERE user_id = ?`,
			`DELETE FROM transaction_tags WHERE transaction_id IN (SELECT id FROM transactions WHERE user_id = ?)`,
			`DELETE FROM transactions WHERE user_id = ?`,
			`DELETE FROM tags WHERE owner_id = ?`,
			`DELETE FROM fund_pools WHERE owner_id = ?`,
			`DELETE FROM users WHERE id = ?`,
		} {
			if _, err := tx.ExecContext(ctx, q, uid); err != nil {
				return 0, fmt.Errorf("cleanup user %s: %w", uid, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit cleanup: %w", err)
	}
	return int64(len(ids)), nil
}

func scanUser(row *sql.Row) (model.User, error) {
	var u model.User
	var createdAt, updatedAt int64
	var verified int
	// SELECT: id, email, name, username, nickname, password_hash, role, email_verified, created_at, updated_at, pending_email, pwd_version
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Username, &u.Nickname, &u.PasswordHash, &u.Role, &verified, &createdAt, &updatedAt, &u.PendingEmail, &u.PwdVersion); err != nil {
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
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`INSERT INTO tags (id, owner_id, name, color, created_at) VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.OwnerID, t.Name, t.Color, t.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert tag: %w", err)
	}
	return nil
}

func (r *SQLiteTagRepository) ListByOwner(ctx context.Context, ownerID string) ([]model.Tag, error) {
	rows, err := getExecutor(ctx, r.db).QueryContext(ctx,
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
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, `DELETE FROM tags WHERE id = ? AND owner_id = ?`, id, ownerID)
	return err
}

func (r *SQLiteTagRepository) AddToTransaction(ctx context.Context, transactionID, tagID string) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`INSERT OR IGNORE INTO transaction_tags(transaction_id, tag_id) VALUES(?, ?)`,
		transactionID, tagID,
	)
	return err
}

func (r *SQLiteTagRepository) RemoveFromTransaction(ctx context.Context, transactionID, tagID string) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`DELETE FROM transaction_tags WHERE transaction_id = ? AND tag_id = ?`,
		transactionID, tagID,
	)
	return err
}

func (r *SQLiteTagRepository) ListByTransaction(ctx context.Context, transactionID string) ([]model.Tag, error) {
	rows, err := getExecutor(ctx, r.db).QueryContext(ctx, `
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
