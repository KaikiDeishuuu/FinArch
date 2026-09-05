package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"finarch/internal/domain/model"
	domainrepo "finarch/internal/domain/repository"
)

// SQLiteAttachmentRepository stores attachment metadata.
type SQLiteAttachmentRepository struct {
	db *sql.DB
}

// NewSQLiteAttachmentRepository creates an attachment repository.
func NewSQLiteAttachmentRepository(db *sql.DB) *SQLiteAttachmentRepository {
	return &SQLiteAttachmentRepository{db: db}
}

const attachmentSelectCols = `
	id, user_id, transaction_id, storage_key, original_filename, content_type,
	size_bytes, sha256, kind, ocr_status, ocr_provider, ocr_text, ocr_json,
	ocr_error, created_at, updated_at`

func (r *SQLiteAttachmentRepository) Create(ctx context.Context, a model.Attachment) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO attachments (
			id, user_id, transaction_id, storage_key, original_filename, content_type,
			size_bytes, sha256, kind, ocr_status, ocr_provider, ocr_text, ocr_json,
			ocr_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.UserID, a.TransactionID, a.StorageKey, a.OriginalFilename, a.ContentType,
		a.SizeBytes, a.SHA256, string(a.Kind), string(a.OCRStatus), a.OCRProvider, a.OCRText, a.OCRJSON,
		a.OCRError, formatRepoTime(a.CreatedAt), formatRepoTime(a.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create attachment: %w", err)
	}
	return nil
}

func (r *SQLiteAttachmentRepository) UsageByUser(ctx context.Context, userID string) (domainrepo.AttachmentUsage, error) {
	var usage domainrepo.AttachmentUsage
	err := getExecutor(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(size_bytes), 0)
		FROM attachments
		WHERE user_id = ?`, userID).Scan(&usage.FileCount, &usage.TotalBytes)
	if err != nil {
		return domainrepo.AttachmentUsage{}, fmt.Errorf("read attachment usage: %w", err)
	}
	return usage, nil
}

// CreateWithinQuota is the final quota arbiter. The count, byte total, and
// insert live in one SQLite statement, so concurrent uploads cannot both
// observe stale capacity and push the user over either limit.
func (r *SQLiteAttachmentRepository) CreateWithinQuota(ctx context.Context, a model.Attachment, maxFiles, maxTotalBytes int64) (bool, error) {
	if maxFiles <= 0 || maxTotalBytes <= 0 || a.SizeBytes > maxTotalBytes {
		return false, nil
	}
	res, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO attachments (
			id, user_id, transaction_id, storage_key, original_filename, content_type,
			size_bytes, sha256, kind, ocr_status, ocr_provider, ocr_text, ocr_json,
			ocr_error, created_at, updated_at
		)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE
			(SELECT COUNT(*) FROM attachments WHERE user_id = ?) < ?
			AND
			(SELECT COALESCE(SUM(size_bytes), 0) FROM attachments WHERE user_id = ?) <= ?`,
		a.ID, a.UserID, a.TransactionID, a.StorageKey, a.OriginalFilename, a.ContentType,
		a.SizeBytes, a.SHA256, string(a.Kind), string(a.OCRStatus), a.OCRProvider, a.OCRText, a.OCRJSON,
		a.OCRError, formatRepoTime(a.CreatedAt), formatRepoTime(a.UpdatedAt),
		a.UserID, maxFiles, a.UserID, maxTotalBytes-a.SizeBytes)
	if err != nil {
		return false, fmt.Errorf("create attachment within quota: %w", err)
	}
	created, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read attachment insert count: %w", err)
	}
	return created == 1, nil
}

func (r *SQLiteAttachmentRepository) GetByID(ctx context.Context, id, userID string) (model.Attachment, error) {
	row := getExecutor(ctx, r.db).QueryRowContext(ctx,
		`SELECT`+attachmentSelectCols+` FROM attachments WHERE id = ? AND user_id = ?`, id, userID)
	return scanAttachment(row)
}

func (r *SQLiteAttachmentRepository) ListByUser(ctx context.Context, userID string) ([]model.Attachment, error) {
	rows, err := getExecutor(ctx, r.db).QueryContext(ctx,
		`SELECT`+attachmentSelectCols+` FROM attachments WHERE user_id = ? ORDER BY created_at ASC, id ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user attachments: %w", err)
	}
	defer rows.Close()
	out := []model.Attachment{}
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *SQLiteAttachmentRepository) ListByTransaction(ctx context.Context, transactionID, userID string) ([]model.Attachment, error) {
	rows, err := getExecutor(ctx, r.db).QueryContext(ctx,
		`SELECT`+attachmentSelectCols+` FROM attachments WHERE transaction_id = ? AND user_id = ? ORDER BY created_at DESC`, transactionID, userID)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()
	out := []model.Attachment{}
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *SQLiteAttachmentRepository) LinkToTransaction(ctx context.Context, id, userID, transactionID string) error {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE attachments SET transaction_id = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		transactionID, time.Now().UTC().Format(time.RFC3339), id, userID)
	if err != nil {
		return fmt.Errorf("link attachment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("attachment not found")
	}
	return nil
}

func (r *SQLiteAttachmentRepository) Delete(ctx context.Context, id, userID string) error {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`DELETE FROM attachments WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete attachment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("attachment not found")
	}
	return nil
}

func (r *SQLiteAttachmentRepository) UpdateOCR(ctx context.Context, a model.Attachment) error {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		UPDATE attachments
		SET ocr_status = ?, ocr_provider = ?, ocr_text = ?, ocr_json = ?, ocr_error = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		string(a.OCRStatus), a.OCRProvider, a.OCRText, a.OCRJSON, a.OCRError, time.Now().UTC().Format(time.RFC3339), a.ID, a.UserID)
	if err != nil {
		return fmt.Errorf("update attachment OCR: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("attachment not found")
	}
	return nil
}

// EnqueueDeletion durably records an object for eventual deletion. Repeated
// enqueue attempts preserve the original failure history.
func (r *SQLiteAttachmentRepository) EnqueueDeletion(ctx context.Context, storageKey, userID string) error {
	now := formatRepoTime(time.Now())
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO attachment_deletion_queue (
			storage_key, user_id, attempts, last_error, created_at, updated_at
		) VALUES (?, ?, 0, NULL, ?, ?)
		ON CONFLICT(storage_key) DO NOTHING`, storageKey, userID, now, now)
	if err != nil {
		return fmt.Errorf("enqueue attachment deletion: %w", err)
	}
	return nil
}

// EnqueueUserDeletions snapshots every current attachment key for a user. It
// uses the context executor so account deletion can enqueue and remove SQL
// metadata atomically in one transaction.
func (r *SQLiteAttachmentRepository) EnqueueUserDeletions(ctx context.Context, userID string) error {
	now := formatRepoTime(time.Now())
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO attachment_deletion_queue (
			storage_key, user_id, attempts, last_error, created_at, updated_at
		)
		SELECT storage_key, user_id, 0, NULL, ?, ?
		FROM attachments
		WHERE user_id = ?
		ON CONFLICT(storage_key) DO NOTHING`, now, now, userID)
	if err != nil {
		return fmt.Errorf("enqueue user attachment deletions: %w", err)
	}
	return nil
}

func (r *SQLiteAttachmentRepository) ListPendingDeletions(ctx context.Context, limit int) ([]domainrepo.AttachmentDeletion, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := getExecutor(ctx, r.db).QueryContext(ctx, `
		SELECT storage_key, user_id, attempts, COALESCE(last_error, ''), created_at, updated_at
		FROM attachment_deletion_queue
		ORDER BY updated_at ASC, created_at ASC, storage_key ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending attachment deletions: %w", err)
	}
	defer rows.Close()

	deletions := make([]domainrepo.AttachmentDeletion, 0)
	for rows.Next() {
		var deletion domainrepo.AttachmentDeletion
		var createdAt, updatedAt string
		if err := rows.Scan(&deletion.StorageKey, &deletion.UserID, &deletion.Attempts, &deletion.LastError, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan pending attachment deletion: %w", err)
		}
		deletion.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		deletion.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		deletions = append(deletions, deletion)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending attachment deletions: %w", err)
	}
	return deletions, nil
}

func (r *SQLiteAttachmentRepository) MarkDeletionFailed(ctx context.Context, storageKey, lastError string) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		UPDATE attachment_deletion_queue
		SET attempts = attempts + 1, last_error = ?, updated_at = ?
		WHERE storage_key = ?`, lastError, formatRepoTime(time.Now()), storageKey)
	if err != nil {
		return fmt.Errorf("mark attachment deletion failed: %w", err)
	}
	return nil
}

func (r *SQLiteAttachmentRepository) CompleteDeletion(ctx context.Context, storageKey string) error {
	if _, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`DELETE FROM attachment_deletion_queue WHERE storage_key = ?`, storageKey); err != nil {
		return fmt.Errorf("complete attachment deletion: %w", err)
	}
	return nil
}

// CleanupExpiredOrphans atomically enqueues each selected object for durable
// deletion before removing its metadata. It owns a transaction when the caller
// did not provide one through SQLiteTransactionManager.
func (r *SQLiteAttachmentRepository) CleanupExpiredOrphans(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	if tx, ok := ctx.Value(txContextKey).(*sql.Tx); ok {
		return cleanupExpiredOrphans(ctx, tx, before, limit)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin orphan attachment cleanup: %w", err)
	}
	deleted, err := cleanupExpiredOrphans(ctx, tx, before, limit)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("commit orphan attachment cleanup: %w", err)
	}
	return deleted, nil
}

func cleanupExpiredOrphans(ctx context.Context, exec sqlExecutor, before time.Time, limit int) (int64, error) {
	cutoff := formatRepoTime(before)
	now := formatRepoTime(time.Now())
	_, err := exec.ExecContext(ctx, `
		INSERT INTO attachment_deletion_queue (
			storage_key, user_id, attempts, last_error, created_at, updated_at
		)
		SELECT storage_key, user_id, 0, NULL, ?, ?
		FROM attachments
		WHERE id IN (
			SELECT id
			FROM attachments
			WHERE transaction_id IS NULL
				AND julianday(created_at) < julianday(?)
			ORDER BY julianday(created_at) ASC, id ASC
			LIMIT ?
		)
		ON CONFLICT(storage_key) DO NOTHING`, now, now, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("enqueue expired orphan attachments: %w", err)
	}
	res, err := exec.ExecContext(ctx, `
		DELETE FROM attachments
		WHERE id IN (
			SELECT id
			FROM attachments
			WHERE transaction_id IS NULL
				AND julianday(created_at) < julianday(?)
			ORDER BY julianday(created_at) ASC, id ASC
			LIMIT ?
		)`, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("delete expired orphan attachment metadata: %w", err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read expired orphan deletion count: %w", err)
	}
	return deleted, nil
}

type attachmentScanner interface{ Scan(dest ...any) error }

func scanAttachment(s attachmentScanner) (model.Attachment, error) {
	var a model.Attachment
	var transactionID, ocrProvider, ocrText, ocrJSON, ocrError sql.NullString
	var kind, ocrStatus, createdAt, updatedAt string
	if err := s.Scan(&a.ID, &a.UserID, &transactionID, &a.StorageKey, &a.OriginalFilename, &a.ContentType,
		&a.SizeBytes, &a.SHA256, &kind, &ocrStatus, &ocrProvider, &ocrText, &ocrJSON, &ocrError,
		&createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return model.Attachment{}, fmt.Errorf("attachment not found")
		}
		return model.Attachment{}, fmt.Errorf("scan attachment: %w", err)
	}
	a.Kind = model.AttachmentKind(kind)
	a.OCRStatus = model.OCRStatus(ocrStatus)
	if transactionID.Valid {
		v := transactionID.String
		a.TransactionID = &v
	}
	if ocrProvider.Valid {
		v := ocrProvider.String
		a.OCRProvider = &v
	}
	if ocrText.Valid {
		v := ocrText.String
		a.OCRText = &v
	}
	if ocrJSON.Valid {
		v := ocrJSON.String
		a.OCRJSON = &v
	}
	if ocrError.Valid {
		v := ocrError.String
		a.OCRError = &v
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		a.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		a.UpdatedAt = t
	}
	return a, nil
}
