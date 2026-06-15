package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"finarch/internal/domain/model"
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
