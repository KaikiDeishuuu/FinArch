package repository

import (
	"context"

	"finarch/internal/domain/model"
)

// AttachmentRepository stores attachment metadata and OCR state.
type AttachmentRepository interface {
	Create(ctx context.Context, attachment model.Attachment) error
	GetByID(ctx context.Context, id, userID string) (model.Attachment, error)
	ListByUser(ctx context.Context, userID string) ([]model.Attachment, error)
	ListByTransaction(ctx context.Context, transactionID, userID string) ([]model.Attachment, error)
	LinkToTransaction(ctx context.Context, id, userID, transactionID string) error
	Delete(ctx context.Context, id, userID string) error
	UpdateOCR(ctx context.Context, attachment model.Attachment) error
}
