package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"

	"github.com/google/uuid"
)

const DefaultAttachmentMaxBytes int64 = 20 << 20 // 20 MB

// StoredAttachment describes bytes persisted by AttachmentStorage.
type StoredAttachment struct {
	StorageKey  string
	ContentType string
	SizeBytes   int64
	SHA256      string
}

// AttachmentStorage stores attachment bytes outside the metadata database.
type AttachmentStorage interface {
	Save(ctx context.Context, userID, attachmentID, filename, declaredContentType string, r io.Reader, maxBytes int64) (StoredAttachment, error)
	Restore(ctx context.Context, storageKey string, r io.Reader) error
	Open(ctx context.Context, storageKey string) (io.ReadCloser, error)
	Delete(ctx context.Context, storageKey string) error
}

// OCRProvider extracts text and suggestions from an attachment.
type OCRProvider interface {
	Name() string
	Available(ctx context.Context) bool
	Extract(ctx context.Context, attachment model.Attachment, r io.Reader) (model.OCRResult, error)
}

// AttachmentService manages receipt/invoice metadata, files, and OCR state.
type AttachmentService struct {
	attachments  repository.AttachmentRepository
	transactions repository.TransactionRepository
	txManager    repository.TransactionManager
	storage      AttachmentStorage
	ocr          OCRProvider
	maxBytes     int64
}

func NewAttachmentService(attachments repository.AttachmentRepository, transactions repository.TransactionRepository, storage AttachmentStorage, ocr OCRProvider, maxBytes int64, txManagers ...repository.TransactionManager) *AttachmentService {
	if maxBytes <= 0 {
		maxBytes = DefaultAttachmentMaxBytes
	}
	var txManager repository.TransactionManager
	if len(txManagers) > 0 {
		txManager = txManagers[0]
	}
	return &AttachmentService{attachments: attachments, transactions: transactions, txManager: txManager, storage: storage, ocr: ocr, maxBytes: maxBytes}
}

type UploadAttachmentRequest struct {
	UserID           string
	TransactionID    *string
	OriginalFilename string
	ContentType      string
	Kind             model.AttachmentKind
	Reader           io.Reader
	RunOCR           bool
}

func (s *AttachmentService) MaxBytes() int64 { return s.maxBytes }

func (s *AttachmentService) Upload(ctx context.Context, req UploadAttachmentRequest) (model.Attachment, error) {
	if strings.TrimSpace(req.UserID) == "" {
		return model.Attachment{}, fmt.Errorf("用户不存在")
	}
	if req.Reader == nil {
		return model.Attachment{}, fmt.Errorf("请上传文件")
	}
	if req.TransactionID != nil {
		id := strings.TrimSpace(*req.TransactionID)
		if id == "" {
			req.TransactionID = nil
		} else {
			if _, err := s.transactions.GetByIDForUser(ctx, id, req.UserID); err != nil {
				return model.Attachment{}, fmt.Errorf("交易记录不存在")
			}
			req.TransactionID = &id
		}
	}
	kind := req.Kind
	if kind == "" {
		kind = model.AttachmentKindReceipt
	}
	if kind != model.AttachmentKindReceipt && kind != model.AttachmentKindInvoice && kind != model.AttachmentKindOther {
		return model.Attachment{}, fmt.Errorf("无效的附件类型")
	}
	filename := sanitizeFilename(req.OriginalFilename)
	if filename == "" {
		filename = "attachment"
	}
	attachmentID := uuid.NewString()
	stored, err := s.storage.Save(ctx, req.UserID, attachmentID, filename, req.ContentType, req.Reader, s.maxBytes)
	if err != nil {
		return model.Attachment{}, err
	}
	now := time.Now()
	attachment := model.Attachment{
		ID:               attachmentID,
		UserID:           req.UserID,
		TransactionID:    req.TransactionID,
		StorageKey:       stored.StorageKey,
		OriginalFilename: filename,
		ContentType:      stored.ContentType,
		SizeBytes:        stored.SizeBytes,
		SHA256:           stored.SHA256,
		Kind:             kind,
		OCRStatus:        model.OCRStatusNotRequested,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.withinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.attachments.Create(txCtx, attachment); err != nil {
			return fmt.Errorf("附件保存失败: %w", err)
		}
		if req.TransactionID != nil {
			if err := s.transactions.SetAttachmentKey(txCtx, *req.TransactionID, req.UserID, stored.StorageKey); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		_ = s.storage.Delete(ctx, stored.StorageKey)
		return model.Attachment{}, err
	}
	if req.RunOCR {
		updated, err := s.RunOCR(ctx, req.UserID, attachment.ID)
		if err == nil {
			attachment = updated
		}
	}
	return attachment, nil
}

func (s *AttachmentService) Get(ctx context.Context, userID, id string) (model.Attachment, error) {
	return s.attachments.GetByID(ctx, id, userID)
}

func (s *AttachmentService) ListByUser(ctx context.Context, userID string) ([]model.Attachment, error) {
	return s.attachments.ListByUser(ctx, userID)
}

func (s *AttachmentService) ListByTransaction(ctx context.Context, userID, transactionID string) ([]model.Attachment, error) {
	if _, err := s.transactions.GetByIDForUser(ctx, transactionID, userID); err != nil {
		return nil, fmt.Errorf("交易记录不存在")
	}
	return s.attachments.ListByTransaction(ctx, transactionID, userID)
}

func (s *AttachmentService) OpenStorage(ctx context.Context, storageKey string) (io.ReadCloser, error) {
	return s.storage.Open(ctx, storageKey)
}

func (s *AttachmentService) RestoreStorage(ctx context.Context, storageKey string, r io.Reader) error {
	return s.storage.Restore(ctx, storageKey, r)
}

func (s *AttachmentService) Open(ctx context.Context, userID, id string) (model.Attachment, io.ReadCloser, error) {
	attachment, err := s.attachments.GetByID(ctx, id, userID)
	if err != nil {
		return model.Attachment{}, nil, err
	}
	r, err := s.storage.Open(ctx, attachment.StorageKey)
	if err != nil {
		return model.Attachment{}, nil, fmt.Errorf("附件文件不存在")
	}
	return attachment, r, nil
}

func (s *AttachmentService) Link(ctx context.Context, userID, id, transactionID string) (model.Attachment, error) {
	if _, err := s.transactions.GetByIDForUser(ctx, transactionID, userID); err != nil {
		return model.Attachment{}, fmt.Errorf("交易记录不存在")
	}
	var attachment model.Attachment
	if err := s.withinTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.attachments.GetByID(txCtx, id, userID)
		if err != nil {
			return err
		}
		if current.TransactionID != nil && *current.TransactionID != transactionID {
			if err := s.transactions.ClearAttachmentKey(txCtx, *current.TransactionID, userID, current.StorageKey); err != nil {
				return err
			}
		}
		if err := s.attachments.LinkToTransaction(txCtx, id, userID, transactionID); err != nil {
			return err
		}
		if err := s.transactions.SetAttachmentKey(txCtx, transactionID, userID, current.StorageKey); err != nil {
			return err
		}
		linked := current
		linked.TransactionID = &transactionID
		linked.UpdatedAt = time.Now()
		attachment = linked
		return nil
	}); err != nil {
		return model.Attachment{}, err
	}
	return attachment, nil
}

func (s *AttachmentService) Delete(ctx context.Context, userID, id string) error {
	attachment, err := s.attachments.GetByID(ctx, id, userID)
	if err != nil {
		return err
	}
	if err := s.withinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.attachments.Delete(txCtx, id, userID); err != nil {
			return err
		}
		if attachment.TransactionID != nil {
			if err := s.transactions.ClearAttachmentKey(txCtx, *attachment.TransactionID, userID, attachment.StorageKey); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return s.storage.Delete(ctx, attachment.StorageKey)
}

func (s *AttachmentService) RunOCR(ctx context.Context, userID, id string) (model.Attachment, error) {
	attachment, err := s.attachments.GetByID(ctx, id, userID)
	if err != nil {
		return model.Attachment{}, err
	}
	providerName := "none"
	if s.ocr != nil {
		providerName = s.ocr.Name()
	}
	attachment.OCRProvider = &providerName
	attachment.OCRStatus = model.OCRStatusProcessing
	attachment.UpdatedAt = time.Now()
	_ = s.attachments.UpdateOCR(ctx, attachment)
	if s.ocr == nil || !s.ocr.Available(ctx) {
		msg := "OCR provider unavailable"
		attachment.OCRStatus = model.OCRStatusUnavailable
		attachment.OCRError = &msg
		attachment.UpdatedAt = time.Now()
		_ = s.attachments.UpdateOCR(ctx, attachment)
		return attachment, nil
	}
	r, err := s.storage.Open(ctx, attachment.StorageKey)
	if err != nil {
		msg := "attachment file unavailable"
		attachment.OCRStatus = model.OCRStatusFailed
		attachment.OCRError = &msg
		attachment.UpdatedAt = time.Now()
		_ = s.attachments.UpdateOCR(ctx, attachment)
		return attachment, fmt.Errorf("附件文件不存在")
	}
	defer r.Close()
	result, err := s.ocr.Extract(ctx, attachment, r)
	if err != nil {
		msg := truncateAttachmentError(err.Error())
		attachment.OCRStatus = model.OCRStatusFailed
		attachment.OCRError = &msg
		attachment.UpdatedAt = time.Now()
		_ = s.attachments.UpdateOCR(ctx, attachment)
		return attachment, nil
	}
	providerName = result.Provider
	if providerName == "" {
		providerName = s.ocr.Name()
	}
	text := result.Text
	structured, _ := json.Marshal(result)
	jsonText := string(structured)
	attachment.OCRStatus = model.OCRStatusDone
	attachment.OCRProvider = &providerName
	attachment.OCRText = &text
	attachment.OCRJSON = &jsonText
	attachment.OCRError = nil
	attachment.UpdatedAt = time.Now()
	if err := s.attachments.UpdateOCR(ctx, attachment); err != nil {
		return model.Attachment{}, err
	}
	return attachment, nil
}

func (s *AttachmentService) withinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if s.txManager == nil {
		return fn(ctx)
	}
	return s.txManager.WithinTransaction(ctx, fn)
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "\x00", "")
	return name
}

func truncateAttachmentError(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > 240 {
		return msg[:240]
	}
	return msg
}
