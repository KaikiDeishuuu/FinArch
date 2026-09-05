package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"

	"github.com/google/uuid"
)

const (
	DefaultAttachmentMaxBytes               int64 = 20 << 20 // 20 MiB
	DefaultAttachmentMaxFilesPerUser        int64 = 500
	DefaultAttachmentMaxTotalBytesPerUser   int64 = 1 << 30 // 1 GiB
	DefaultAttachmentUploadsPerMinute             = 30
	DefaultAttachmentUploadWindow                 = time.Minute
	DefaultAttachmentDeletionBatchSize            = 100
	DefaultAttachmentDeletionInterval             = time.Minute
	DefaultAttachmentOrphanTTL                    = 24 * time.Hour
	DefaultAttachmentOrphanCleanupInterval        = time.Hour
	DefaultAttachmentOrphanCleanupBatchSize       = 100
)

const (
	DefaultOCRMaxConcurrentPerUser  = 1
	DefaultOCRMaxConcurrentGlobal   = 4
	DefaultOCRMaxRunsPerHour        = 20
	DefaultOCRMaxPersistedTextBytes = 512 << 10 // 512 KiB
	DefaultOCRMaxPersistedJSONBytes = 1 << 20   // 1 MiB
)

var (
	ErrAttachmentQuotaExceeded = errors.New("附件存储配额已用尽")
	ErrOCRBusy                 = errors.New("已有 OCR 任务正在处理，请稍后再试")
	ErrOCRRateLimited          = errors.New("OCR 请求过于频繁，请稍后再试")
)

type ocrFlight struct {
	done       chan struct{}
	attachment model.Attachment
	err        error
}

type ocrQuota struct {
	windowStarted time.Time
	runs          int
}

type attachmentUploadWindow struct {
	started time.Time
	count   int
}

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
	deletionMu   sync.Mutex

	quotaMu              sync.RWMutex
	maxFilesPerUser      int64
	maxTotalBytesPerUser int64

	uploadMu           sync.Mutex
	uploadWindows      map[string]attachmentUploadWindow
	uploadMaxPerWindow int
	uploadWindow       time.Duration
	uploadLastSweep    time.Time

	ocrMu                   sync.Mutex
	ocrFlights              map[string]*ocrFlight
	ocrActiveByUser         map[string]int
	ocrActiveGlobal         int
	ocrQuotaByUser          map[string]ocrQuota
	ocrMaxConcurrentPerUser int
	ocrMaxConcurrentGlobal  int
	ocrMaxRunsPerHour       int
}

func NewAttachmentService(attachments repository.AttachmentRepository, transactions repository.TransactionRepository, storage AttachmentStorage, ocr OCRProvider, maxBytes int64, txManagers ...repository.TransactionManager) *AttachmentService {
	if maxBytes <= 0 {
		maxBytes = DefaultAttachmentMaxBytes
	}
	var txManager repository.TransactionManager
	if len(txManagers) > 0 {
		txManager = txManagers[0]
	}
	return &AttachmentService{
		attachments:             attachments,
		transactions:            transactions,
		txManager:               txManager,
		storage:                 storage,
		ocr:                     ocr,
		maxBytes:                maxBytes,
		maxFilesPerUser:         DefaultAttachmentMaxFilesPerUser,
		maxTotalBytesPerUser:    DefaultAttachmentMaxTotalBytesPerUser,
		uploadWindows:           make(map[string]attachmentUploadWindow),
		uploadMaxPerWindow:      DefaultAttachmentUploadsPerMinute,
		uploadWindow:            DefaultAttachmentUploadWindow,
		ocrFlights:              make(map[string]*ocrFlight),
		ocrActiveByUser:         make(map[string]int),
		ocrQuotaByUser:          make(map[string]ocrQuota),
		ocrMaxConcurrentPerUser: DefaultOCRMaxConcurrentPerUser,
		ocrMaxConcurrentGlobal:  DefaultOCRMaxConcurrentGlobal,
		ocrMaxRunsPerHour:       DefaultOCRMaxRunsPerHour,
	}
}

// ConfigureQuota applies positive per-user attachment limits. Non-positive
// values leave the existing safe limits in place.
func (s *AttachmentService) ConfigureQuota(maxFiles, maxTotalBytes int64) {
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	if maxFiles > 0 {
		s.maxFilesPerUser = maxFiles
	}
	if maxTotalBytes > 0 {
		s.maxTotalBytesPerUser = maxTotalBytes
	}
}

// ConfigureUploadRateLimit applies a process-local per-user fixed-window
// upload limit. Resetting state makes startup configuration deterministic.
func (s *AttachmentService) ConfigureUploadRateLimit(maxUploads int, window time.Duration) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	if maxUploads > 0 {
		s.uploadMaxPerWindow = maxUploads
	}
	if window > 0 {
		s.uploadWindow = window
	}
	s.uploadWindows = make(map[string]attachmentUploadWindow)
	s.uploadLastSweep = time.Time{}
}

// AllowUpload consumes one process-local upload attempt for the user.
func (s *AttachmentService) AllowUpload(userID string) bool {
	return s.allowUploadAt(strings.TrimSpace(userID), time.Now())
}

func (s *AttachmentService) allowUploadAt(userID string, now time.Time) bool {
	if userID == "" {
		return false
	}
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()

	window := s.uploadWindows[userID]
	elapsed := now.Sub(window.started)
	if window.started.IsZero() || elapsed < 0 || elapsed >= s.uploadWindow {
		window = attachmentUploadWindow{started: now}
	}
	if window.count >= s.uploadMaxPerWindow {
		return false
	}
	window.count++
	s.uploadWindows[userID] = window

	if len(s.uploadWindows) > 1024 &&
		(s.uploadLastSweep.IsZero() || now.Sub(s.uploadLastSweep) >= s.uploadWindow) {
		for id, candidate := range s.uploadWindows {
			age := now.Sub(candidate.started)
			if age < 0 || age >= 2*s.uploadWindow {
				delete(s.uploadWindows, id)
			}
		}
		s.uploadLastSweep = now
	}
	return true
}

// ConfigureOCRLimits applies process-local cost and concurrency guards to cloud
// OCR. Values less than one retain the safe defaults.
func (s *AttachmentService) ConfigureOCRLimits(maxConcurrentPerUser, maxConcurrentGlobal, maxRunsPerHour int) {
	s.ocrMu.Lock()
	defer s.ocrMu.Unlock()
	if maxConcurrentPerUser > 0 {
		s.ocrMaxConcurrentPerUser = maxConcurrentPerUser
	}
	if maxConcurrentGlobal > 0 {
		s.ocrMaxConcurrentGlobal = maxConcurrentGlobal
	}
	if maxRunsPerHour > 0 {
		s.ocrMaxRunsPerHour = maxRunsPerHour
	}
}

type UploadAttachmentRequest struct {
	UserID           string
	TransactionID    *string
	OriginalFilename string
	ContentType      string
	SizeBytesHint    int64
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
	maxFiles, maxTotalBytes := s.quotaLimits()
	usage, err := s.attachments.UsageByUser(ctx, req.UserID)
	if err != nil {
		return model.Attachment{}, fmt.Errorf("检查附件存储配额: %w", err)
	}
	remainingBytes := maxTotalBytes - usage.TotalBytes
	if usage.FileCount >= maxFiles || usage.TotalBytes >= maxTotalBytes ||
		(req.SizeBytesHint > 0 && req.SizeBytesHint > remainingBytes) {
		return model.Attachment{}, ErrAttachmentQuotaExceeded
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
		created, err := s.attachments.CreateWithinQuota(txCtx, attachment, maxFiles, maxTotalBytes)
		if err != nil {
			return fmt.Errorf("附件保存失败: %w", err)
		}
		if !created {
			return ErrAttachmentQuotaExceeded
		}
		if req.TransactionID != nil {
			if err := s.transactions.SetAttachmentKey(txCtx, *req.TransactionID, req.UserID, stored.StorageKey); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		if cleanupErr := s.compensateFailedUpload(ctx, req.UserID, stored.StorageKey); cleanupErr != nil {
			return model.Attachment{}, errors.Join(err, cleanupErr)
		}
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

func (s *AttachmentService) compensateFailedUpload(ctx context.Context, userID, storageKey string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.storage.Delete(cleanupCtx, storageKey); err == nil {
		return nil
	} else {
		deleteErr := fmt.Errorf("删除未提交的附件对象失败: %w", err)
		if queueErr := s.attachments.EnqueueDeletion(cleanupCtx, storageKey, userID); queueErr != nil {
			return errors.Join(deleteErr, fmt.Errorf("持久化附件清理任务失败: %w", queueErr))
		}
		if markErr := s.attachments.MarkDeletionFailed(cleanupCtx, storageKey, truncateAttachmentError(err.Error())); markErr != nil {
			return errors.Join(deleteErr, fmt.Errorf("记录附件清理失败原因: %w", markErr))
		}
		log.Printf("[attachments] queued failed upload cleanup storage_key=%q: %v", storageKey, err)
		return nil
	}
}

func (s *AttachmentService) quotaLimits() (int64, int64) {
	s.quotaMu.RLock()
	defer s.quotaMu.RUnlock()
	return s.maxFilesPerUser, s.maxTotalBytesPerUser
}

func (s *AttachmentService) Get(ctx context.Context, userID, id string) (model.Attachment, error) {
	return s.attachments.GetByID(ctx, id, userID)
}

func (s *AttachmentService) ListByUser(ctx context.Context, userID string) ([]model.Attachment, error) {
	return s.attachments.ListByUser(ctx, userID)
}

// EnqueueUserDataDeletion snapshots all of a user's attachment keys into the
// durable deletion outbox. Callers deleting the user must invoke this inside
// the same SQL transaction as the metadata deletion.
func (s *AttachmentService) EnqueueUserDataDeletion(ctx context.Context, userID string) error {
	return s.attachments.EnqueueUserDeletions(ctx, userID)
}

// DeleteUserData is retained for compatibility. Physical deletion is now
// asynchronous, so this method only durably enqueues the user's objects.
func (s *AttachmentService) DeleteUserData(ctx context.Context, userID string) error {
	return s.EnqueueUserDataDeletion(ctx, userID)
}

// DrainDeletionQueue makes one best-effort pass over the durable deletion
// outbox. Storage failures remain queued with their failure history; successful
// deletes remove their queue rows. A mutex prevents overlapping passes within
// one process, while idempotent storage deletion keeps cross-process races safe.
func (s *AttachmentService) DrainDeletionQueue(ctx context.Context, limit int) error {
	s.deletionMu.Lock()
	defer s.deletionMu.Unlock()

	deletions, err := s.attachments.ListPendingDeletions(ctx, limit)
	if err != nil {
		return err
	}
	var drainErrors []error
	for _, deletion := range deletions {
		if err := ctx.Err(); err != nil {
			drainErrors = append(drainErrors, err)
			break
		}
		if err := s.storage.Delete(ctx, deletion.StorageKey); err != nil {
			deleteErr := fmt.Errorf("delete attachment object %q: %w", deletion.StorageKey, err)
			if markErr := s.attachments.MarkDeletionFailed(ctx, deletion.StorageKey, truncateAttachmentError(err.Error())); markErr != nil {
				deleteErr = errors.Join(deleteErr, markErr)
			}
			drainErrors = append(drainErrors, deleteErr)
			continue
		}
		if err := s.attachments.CompleteDeletion(ctx, deletion.StorageKey); err != nil {
			drainErrors = append(drainErrors, err)
		}
	}
	return errors.Join(drainErrors...)
}

// CleanupExpiredOrphans durably queues and removes unlinked attachment
// metadata older than before. Physical object deletion remains retryable via
// DrainDeletionQueue.
func (s *AttachmentService) CleanupExpiredOrphans(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = DefaultAttachmentOrphanCleanupBatchSize
	}
	var cleaned int64
	err := s.withinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		cleaned, err = s.attachments.CleanupExpiredOrphans(txCtx, before, limit)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("cleanup expired orphan attachments: %w", err)
	}
	return cleaned, nil
}

// RunDeletionQueueWorker drains once at startup and then periodically until
// ctx is cancelled. The ticker is always stopped before the worker returns.
func (s *AttachmentService) RunDeletionQueueWorker(ctx context.Context, interval time.Duration, limit int) {
	if interval <= 0 {
		interval = DefaultAttachmentDeletionInterval
	}
	drain := func() {
		if err := s.DrainDeletionQueue(ctx, limit); err != nil && ctx.Err() == nil {
			log.Printf("[attachments] deletion queue drain failed: %v", err)
		}
	}
	if ctx.Err() != nil {
		return
	}
	drain()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			drain()
		}
	}
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

// DeleteStorage removes a storage object without changing attachment metadata.
// It is used by disaster-recovery compensation after the database transaction
// boundary has already been restored separately.
func (s *AttachmentService) DeleteStorage(ctx context.Context, storageKey string) error {
	return s.storage.Delete(ctx, storageKey)
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
	if err := s.withinTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.attachments.GetByID(txCtx, id, userID)
		if err != nil {
			return err
		}
		if err := s.attachments.EnqueueDeletion(txCtx, current.StorageKey, userID); err != nil {
			return err
		}
		if err := s.attachments.Delete(txCtx, id, userID); err != nil {
			return err
		}
		if current.TransactionID != nil {
			if err := s.transactions.ClearAttachmentKey(txCtx, *current.TransactionID, userID, current.StorageKey); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	// The durable queue is already committed. Storage failure must not turn a
	// successful logical deletion into an API failure; the worker will retry.
	_ = s.DrainDeletionQueue(ctx, DefaultAttachmentDeletionBatchSize)
	return nil
}

func (s *AttachmentService) RunOCR(ctx context.Context, userID, id string) (model.Attachment, error) {
	key := userID + "\x00" + id
	s.ocrMu.Lock()
	if existing := s.ocrFlights[key]; existing != nil {
		s.ocrMu.Unlock()
		select {
		case <-existing.done:
			return existing.attachment, existing.err
		case <-ctx.Done():
			return model.Attachment{}, ctx.Err()
		}
	}
	if s.ocrActiveByUser[userID] >= s.ocrMaxConcurrentPerUser || s.ocrActiveGlobal >= s.ocrMaxConcurrentGlobal {
		s.ocrMu.Unlock()
		return model.Attachment{}, ErrOCRBusy
	}
	flight := &ocrFlight{done: make(chan struct{})}
	s.ocrFlights[key] = flight
	s.ocrActiveByUser[userID]++
	s.ocrActiveGlobal++
	s.ocrMu.Unlock()

	attachment, err := s.runOCR(ctx, userID, id)

	s.ocrMu.Lock()
	flight.attachment = attachment
	flight.err = err
	delete(s.ocrFlights, key)
	s.ocrActiveByUser[userID]--
	s.ocrActiveGlobal--
	if s.ocrActiveByUser[userID] == 0 {
		delete(s.ocrActiveByUser, userID)
	}
	close(flight.done)
	s.ocrMu.Unlock()
	return attachment, err
}

func (s *AttachmentService) runOCR(ctx context.Context, userID, id string) (model.Attachment, error) {
	attachment, err := s.attachments.GetByID(ctx, id, userID)
	if err != nil {
		return model.Attachment{}, err
	}
	// A completed extraction is immutable for this attachment. Returning it
	// makes POST /ocr idempotent and prevents duplicate paid cloud jobs.
	if attachment.OCRStatus == model.OCRStatusDone {
		return attachment, nil
	}
	providerName := "none"
	if s.ocr != nil {
		providerName = s.ocr.Name()
	}
	attachment.OCRProvider = &providerName
	if s.ocr == nil || !s.ocr.Available(ctx) {
		msg := "OCR provider unavailable"
		attachment.OCRStatus = model.OCRStatusUnavailable
		attachment.OCRError = &msg
		attachment.UpdatedAt = time.Now()
		_ = s.attachments.UpdateOCR(ctx, attachment)
		return attachment, nil
	}
	if !s.takeOCRQuota(userID, time.Now()) {
		return attachment, ErrOCRRateLimited
	}
	attachment.OCRStatus = model.OCRStatusProcessing
	attachment.UpdatedAt = time.Now()
	_ = s.attachments.UpdateOCR(ctx, attachment)
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
	if len(text) > DefaultOCRMaxPersistedTextBytes {
		return s.failOversizedOCRResult(ctx, attachment, "OCR text exceeds the persisted result limit")
	}
	structured, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return s.failOversizedOCRResult(ctx, attachment, "OCR structured result is invalid")
	}
	if len(structured) > DefaultOCRMaxPersistedJSONBytes {
		return s.failOversizedOCRResult(ctx, attachment, "OCR structured result exceeds the persisted result limit")
	}
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

func (s *AttachmentService) failOversizedOCRResult(ctx context.Context, attachment model.Attachment, message string) (model.Attachment, error) {
	message = truncateAttachmentError(message)
	attachment.OCRStatus = model.OCRStatusFailed
	attachment.OCRText = nil
	attachment.OCRJSON = nil
	attachment.OCRError = &message
	attachment.UpdatedAt = time.Now()
	if err := s.attachments.UpdateOCR(ctx, attachment); err != nil {
		return model.Attachment{}, err
	}
	return attachment, nil
}

func (s *AttachmentService) takeOCRQuota(userID string, now time.Time) bool {
	s.ocrMu.Lock()
	defer s.ocrMu.Unlock()
	quota := s.ocrQuotaByUser[userID]
	if quota.windowStarted.IsZero() || now.Sub(quota.windowStarted) >= time.Hour || now.Before(quota.windowStarted) {
		quota = ocrQuota{windowStarted: now}
	}
	if quota.runs >= s.ocrMaxRunsPerHour {
		return false
	}
	quota.runs++
	s.ocrQuotaByUser[userID] = quota
	if len(s.ocrQuotaByUser) > 1024 {
		for id, candidate := range s.ocrQuotaByUser {
			if now.Sub(candidate.windowStarted) >= 2*time.Hour {
				delete(s.ocrQuotaByUser, id)
			}
		}
	}
	return true
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
