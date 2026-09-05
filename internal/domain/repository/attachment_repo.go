package repository

import (
	"context"
	"time"

	"finarch/internal/domain/model"
)

// AttachmentDeletion is a durable request to remove an attachment object from
// external storage. It deliberately survives deletion of the owning user.
type AttachmentDeletion struct {
	StorageKey string
	UserID     string
	Attempts   int
	LastError  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// AttachmentUsage is the current per-user attachment footprint used to reject
// uploads before their bytes are written to external storage.
type AttachmentUsage struct {
	FileCount  int64
	TotalBytes int64
}

// AttachmentRepository stores attachment metadata and OCR state.
type AttachmentRepository interface {
	Create(ctx context.Context, attachment model.Attachment) error
	UsageByUser(ctx context.Context, userID string) (AttachmentUsage, error)
	CreateWithinQuota(ctx context.Context, attachment model.Attachment, maxFiles, maxTotalBytes int64) (bool, error)
	GetByID(ctx context.Context, id, userID string) (model.Attachment, error)
	ListByUser(ctx context.Context, userID string) ([]model.Attachment, error)
	ListByTransaction(ctx context.Context, transactionID, userID string) ([]model.Attachment, error)
	LinkToTransaction(ctx context.Context, id, userID, transactionID string) error
	Delete(ctx context.Context, id, userID string) error
	UpdateOCR(ctx context.Context, attachment model.Attachment) error
	EnqueueDeletion(ctx context.Context, storageKey, userID string) error
	EnqueueUserDeletions(ctx context.Context, userID string) error
	ListPendingDeletions(ctx context.Context, limit int) ([]AttachmentDeletion, error)
	MarkDeletionFailed(ctx context.Context, storageKey, lastError string) error
	CompleteDeletion(ctx context.Context, storageKey string) error
	CleanupExpiredOrphans(ctx context.Context, before time.Time, limit int) (int64, error)
}
