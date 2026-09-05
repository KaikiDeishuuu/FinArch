package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"
)

type ocrTestAttachmentRepo struct {
	mu          sync.Mutex
	attachments map[string]model.Attachment
	deletions   map[string]repository.AttachmentDeletion
	listCalls   int
}

func (r *ocrTestAttachmentRepo) Create(_ context.Context, attachment model.Attachment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attachments[attachment.ID] = attachment
	return nil
}

func (r *ocrTestAttachmentRepo) UsageByUser(_ context.Context, userID string) (repository.AttachmentUsage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var usage repository.AttachmentUsage
	for _, attachment := range r.attachments {
		if attachment.UserID == userID {
			usage.FileCount++
			usage.TotalBytes += attachment.SizeBytes
		}
	}
	return usage, nil
}

func (r *ocrTestAttachmentRepo) CreateWithinQuota(_ context.Context, attachment model.Attachment, maxFiles, maxTotalBytes int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var usage repository.AttachmentUsage
	for _, existing := range r.attachments {
		if existing.UserID == attachment.UserID {
			usage.FileCount++
			usage.TotalBytes += existing.SizeBytes
		}
	}
	if usage.FileCount >= maxFiles || attachment.SizeBytes > maxTotalBytes-usage.TotalBytes {
		return false, nil
	}
	r.attachments[attachment.ID] = attachment
	return true, nil
}

func (r *ocrTestAttachmentRepo) GetByID(_ context.Context, id, userID string) (model.Attachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	attachment, ok := r.attachments[id]
	if !ok || attachment.UserID != userID {
		return model.Attachment{}, errors.New("not found")
	}
	return attachment, nil
}

func (r *ocrTestAttachmentRepo) ListByUser(context.Context, string) ([]model.Attachment, error) {
	return nil, nil
}

func (r *ocrTestAttachmentRepo) ListByTransaction(context.Context, string, string) ([]model.Attachment, error) {
	return nil, nil
}

func (r *ocrTestAttachmentRepo) LinkToTransaction(context.Context, string, string, string) error {
	return nil
}

func (r *ocrTestAttachmentRepo) Delete(_ context.Context, id, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if attachment, ok := r.attachments[id]; !ok || attachment.UserID != userID {
		return errors.New("not found")
	}
	delete(r.attachments, id)
	return nil
}

func (r *ocrTestAttachmentRepo) UpdateOCR(_ context.Context, attachment model.Attachment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attachments[attachment.ID] = attachment
	return nil
}

func (r *ocrTestAttachmentRepo) EnqueueDeletion(_ context.Context, storageKey, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deletions == nil {
		r.deletions = make(map[string]repository.AttachmentDeletion)
	}
	if _, exists := r.deletions[storageKey]; !exists {
		now := time.Now()
		r.deletions[storageKey] = repository.AttachmentDeletion{StorageKey: storageKey, UserID: userID, CreatedAt: now, UpdatedAt: now}
	}
	return nil
}

func (r *ocrTestAttachmentRepo) EnqueueUserDeletions(ctx context.Context, userID string) error {
	r.mu.Lock()
	keys := make([]string, 0)
	for _, attachment := range r.attachments {
		if attachment.UserID == userID {
			keys = append(keys, attachment.StorageKey)
		}
	}
	r.mu.Unlock()
	for _, storageKey := range keys {
		if err := r.EnqueueDeletion(ctx, storageKey, userID); err != nil {
			return err
		}
	}
	return nil
}

func (r *ocrTestAttachmentRepo) ListPendingDeletions(context.Context, int) ([]repository.AttachmentDeletion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listCalls++
	out := make([]repository.AttachmentDeletion, 0, len(r.deletions))
	for _, deletion := range r.deletions {
		out = append(out, deletion)
	}
	return out, nil
}

func (r *ocrTestAttachmentRepo) MarkDeletionFailed(_ context.Context, storageKey, lastError string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	deletion, exists := r.deletions[storageKey]
	if !exists {
		return nil
	}
	deletion.Attempts++
	deletion.LastError = lastError
	deletion.UpdatedAt = time.Now()
	r.deletions[storageKey] = deletion
	return nil
}

func (r *ocrTestAttachmentRepo) CompleteDeletion(_ context.Context, storageKey string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.deletions, storageKey)
	return nil
}

func (r *ocrTestAttachmentRepo) CleanupExpiredOrphans(ctx context.Context, before time.Time, limit int) (int64, error) {
	r.mu.Lock()
	ids := make([]string, 0)
	for id, attachment := range r.attachments {
		if attachment.TransactionID == nil && attachment.CreatedAt.Before(before) && len(ids) < limit {
			ids = append(ids, id)
		}
	}
	deletions := make([]repository.AttachmentDeletion, 0, len(ids))
	for _, id := range ids {
		attachment := r.attachments[id]
		deletions = append(deletions, repository.AttachmentDeletion{StorageKey: attachment.StorageKey, UserID: attachment.UserID})
		delete(r.attachments, id)
	}
	r.mu.Unlock()
	for _, deletion := range deletions {
		if err := r.EnqueueDeletion(ctx, deletion.StorageKey, deletion.UserID); err != nil {
			return 0, err
		}
	}
	return int64(len(ids)), nil
}

type ocrTestStorage struct {
	mu        sync.Mutex
	deleteErr error
	deletes   int
}

func (s *ocrTestStorage) Save(context.Context, string, string, string, string, io.Reader, int64) (StoredAttachment, error) {
	return StoredAttachment{}, errors.New("not implemented")
}
func (s *ocrTestStorage) Restore(context.Context, string, io.Reader) error { return nil }
func (s *ocrTestStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("receipt")), nil
}
func (s *ocrTestStorage) Delete(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes++
	return s.deleteErr
}

type blockingOCRProvider struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
	result  model.OCRResult
}

func (p *blockingOCRProvider) Name() string                   { return "test" }
func (p *blockingOCRProvider) Available(context.Context) bool { return true }
func (p *blockingOCRProvider) Extract(ctx context.Context, _ model.Attachment, _ io.Reader) (model.OCRResult, error) {
	p.calls.Add(1)
	if p.started != nil {
		select {
		case p.started <- struct{}{}:
		default:
		}
	}
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return model.OCRResult{}, ctx.Err()
		}
	}
	if p.result.Provider != "" || p.result.Text != "" || p.result.Raw != nil {
		return p.result, nil
	}
	return model.OCRResult{Provider: "test", Text: "ok"}, nil
}

func testAttachment(id string) model.Attachment {
	return model.Attachment{ID: id, UserID: "user-1", StorageKey: "user-1/" + id + ".png", OCRStatus: model.OCRStatusNotRequested}
}

func TestAttachmentServiceRunOCRCoalescesAndIsIdempotent(t *testing.T) {
	repo := &ocrTestAttachmentRepo{attachments: map[string]model.Attachment{"a1": testAttachment("a1")}}
	provider := &blockingOCRProvider{started: make(chan struct{}, 1), release: make(chan struct{})}
	svc := NewAttachmentService(repo, nil, &ocrTestStorage{}, provider, DefaultAttachmentMaxBytes)

	type result struct {
		attachment model.Attachment
		err        error
	}
	first := make(chan result, 1)
	second := make(chan result, 1)
	go func() {
		a, err := svc.RunOCR(context.Background(), "user-1", "a1")
		first <- result{a, err}
	}()
	<-provider.started
	go func() {
		a, err := svc.RunOCR(context.Background(), "user-1", "a1")
		second <- result{a, err}
	}()
	select {
	case <-second:
		t.Fatal("duplicate OCR call returned before the in-flight job completed")
	case <-time.After(10 * time.Millisecond):
	}
	close(provider.release)
	for _, ch := range []chan result{first, second} {
		got := <-ch
		if got.err != nil || got.attachment.OCRStatus != model.OCRStatusDone {
			t.Fatalf("RunOCR = (%#v, %v)", got.attachment, got.err)
		}
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls.Load())
	}
	if _, err := svc.RunOCR(context.Background(), "user-1", "a1"); err != nil {
		t.Fatalf("idempotent RunOCR: %v", err)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("completed attachment triggered another provider call: %d", provider.calls.Load())
	}
}

func TestAttachmentServiceRunOCRLimitsConcurrencyAndRate(t *testing.T) {
	repo := &ocrTestAttachmentRepo{attachments: map[string]model.Attachment{
		"a1": testAttachment("a1"),
		"a2": testAttachment("a2"),
		"a3": testAttachment("a3"),
	}}
	provider := &blockingOCRProvider{started: make(chan struct{}, 1), release: make(chan struct{})}
	svc := NewAttachmentService(repo, nil, &ocrTestStorage{}, provider, DefaultAttachmentMaxBytes)
	svc.ConfigureOCRLimits(1, 4, 1)
	done := make(chan error, 1)
	go func() {
		_, err := svc.RunOCR(context.Background(), "user-1", "a1")
		done <- err
	}()
	<-provider.started
	if _, err := svc.RunOCR(context.Background(), "user-1", "a2"); !errors.Is(err, ErrOCRBusy) {
		t.Fatalf("concurrent RunOCR error = %v, want ErrOCRBusy", err)
	}
	close(provider.release)
	if err := <-done; err != nil {
		t.Fatalf("first RunOCR: %v", err)
	}
	if _, err := svc.RunOCR(context.Background(), "user-1", "a3"); !errors.Is(err, ErrOCRRateLimited) {
		t.Fatalf("second charged RunOCR error = %v, want ErrOCRRateLimited", err)
	}
}

func TestAttachmentServiceRunOCRRejectsOversizedPersistedResult(t *testing.T) {
	repo := &ocrTestAttachmentRepo{attachments: map[string]model.Attachment{"a1": testAttachment("a1")}}
	provider := &blockingOCRProvider{result: model.OCRResult{
		Provider: "test",
		Text:     strings.Repeat("x", DefaultOCRMaxPersistedTextBytes+1),
	}}
	svc := NewAttachmentService(repo, nil, &ocrTestStorage{}, provider, DefaultAttachmentMaxBytes)

	attachment, err := svc.RunOCR(context.Background(), "user-1", "a1")
	if err != nil {
		t.Fatalf("RunOCR returned infrastructure error: %v", err)
	}
	if attachment.OCRStatus != model.OCRStatusFailed || attachment.OCRText != nil || attachment.OCRJSON != nil {
		t.Fatalf("oversized OCR result was persisted: %#v", attachment)
	}
	if attachment.OCRError == nil || !strings.Contains(*attachment.OCRError, "persisted result limit") {
		t.Fatalf("oversized OCR failure reason = %v", attachment.OCRError)
	}
}

func TestAttachmentServiceDeleteQueuesStorageFailureForRetry(t *testing.T) {
	repo := &ocrTestAttachmentRepo{attachments: map[string]model.Attachment{"a1": testAttachment("a1")}}
	storage := &ocrTestStorage{deleteErr: errors.New("disk failure")}
	svc := NewAttachmentService(repo, nil, storage, nil, DefaultAttachmentMaxBytes)
	if err := svc.Delete(context.Background(), "user-1", "a1"); err != nil {
		t.Fatalf("logical delete must succeed after durable enqueue: %v", err)
	}
	if _, err := repo.GetByID(context.Background(), "a1", "user-1"); err == nil {
		t.Fatal("attachment metadata was not deleted")
	}
	queued, err := repo.ListPendingDeletions(context.Background(), 10)
	if err != nil || len(queued) != 1 {
		t.Fatalf("queued deletions = %#v, %v", queued, err)
	}
	if queued[0].Attempts != 1 || !strings.Contains(queued[0].LastError, "disk failure") {
		t.Fatalf("failure state = %#v", queued[0])
	}

	storage.mu.Lock()
	storage.deleteErr = nil
	storage.mu.Unlock()
	if err := svc.DrainDeletionQueue(context.Background(), 10); err != nil {
		t.Fatalf("retry drain: %v", err)
	}
	queued, err = repo.ListPendingDeletions(context.Background(), 10)
	if err != nil || len(queued) != 0 {
		t.Fatalf("queue after retry = %#v, %v", queued, err)
	}
}
