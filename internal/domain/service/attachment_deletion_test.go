package service_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/db"
	sqliterepo "finarch/internal/infrastructure/repository"
)

type retryAttachmentStorage struct {
	mu        sync.Mutex
	deleteErr error
	deletes   int
}

type quotaAttachmentStorage struct {
	mu              sync.Mutex
	storedSize      int64
	saves           int
	deletes         int
	deleteErr       error
	waitForSaves    int
	allSavesStarted chan struct{}
	releaseSaves    <-chan struct{}
}

func (s *quotaAttachmentStorage) Save(_ context.Context, userID, attachmentID, _ string, contentType string, r io.Reader, _ int64) (service.StoredAttachment, error) {
	if _, err := io.Copy(io.Discard, r); err != nil {
		return service.StoredAttachment{}, err
	}
	s.mu.Lock()
	s.saves++
	if s.waitForSaves > 0 && s.saves == s.waitForSaves && s.allSavesStarted != nil {
		close(s.allSavesStarted)
	}
	release := s.releaseSaves
	s.mu.Unlock()
	if release != nil {
		<-release
	}
	return service.StoredAttachment{
		StorageKey:  userID + "/" + attachmentID,
		ContentType: contentType,
		SizeBytes:   s.storedSize,
		SHA256:      strings.Repeat("b", 64),
	}, nil
}

func (*quotaAttachmentStorage) Restore(context.Context, string, io.Reader) error { return nil }

func (*quotaAttachmentStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (s *quotaAttachmentStorage) Delete(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes++
	return s.deleteErr
}

func (s *quotaAttachmentStorage) counts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saves, s.deletes
}

func (*retryAttachmentStorage) Save(context.Context, string, string, string, string, io.Reader, int64) (service.StoredAttachment, error) {
	return service.StoredAttachment{}, errors.New("not implemented")
}

func (*retryAttachmentStorage) Restore(context.Context, string, io.Reader) error { return nil }

func (*retryAttachmentStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (s *retryAttachmentStorage) Delete(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes++
	return s.deleteErr
}

func (s *retryAttachmentStorage) setDeleteError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteErr = err
}

func (s *retryAttachmentStorage) deleteCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deletes
}

type attachmentDeletionFixture struct {
	database   *sql.DB
	repository *sqliterepo.SQLiteAttachmentRepository
	storage    *retryAttachmentStorage
	service    *service.AttachmentService
	userID     string
}

func newAttachmentDeletionFixture(t *testing.T) attachmentDeletionFixture {
	t.Helper()
	ctx := context.Background()
	database, err := db.OpenSQLite(ctx, filepath.Join(t.TempDir(), "finarch.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatal(err)
	}

	userID := "attachment-deletion-user"
	now := time.Now().UTC()
	if err := sqliterepo.NewSQLiteUserRepository(database).Create(ctx, model.User{
		ID: userID, Email: "attachment-deletion@example.com", Username: "attachment-deletion",
		Name: "Attachment Deletion", Nickname: "Attachment Deletion", PasswordHash: "x",
		Role: "owner", EmailVerified: true, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	attachmentRepository := sqliterepo.NewSQLiteAttachmentRepository(database)
	if err := attachmentRepository.Create(ctx, model.Attachment{
		ID: "attachment-1", UserID: userID, StorageKey: userID + "/attachment-1.png",
		OriginalFilename: "receipt.png", ContentType: "image/png", SizeBytes: 1,
		SHA256: strings.Repeat("a", 64), Kind: model.AttachmentKindReceipt,
		OCRStatus: model.OCRStatusNotRequested, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	storage := &retryAttachmentStorage{}
	attachmentService := service.NewAttachmentService(
		attachmentRepository,
		sqliterepo.NewSQLiteTransactionRepository(database),
		storage,
		nil,
		service.DefaultAttachmentMaxBytes,
		sqliterepo.NewSQLiteTransactionManager(database),
	)
	return attachmentDeletionFixture{database, attachmentRepository, storage, attachmentService, userID}
}

func TestAttachmentDeleteRollbackAlsoRollsBackDeletionQueue(t *testing.T) {
	fixture := newAttachmentDeletionFixture(t)
	ctx := context.Background()
	if _, err := fixture.database.ExecContext(ctx, `
		CREATE TRIGGER reject_attachment_delete BEFORE DELETE ON attachments
		BEGIN
			SELECT RAISE(ABORT, 'forced attachment delete failure');
		END;`); err != nil {
		t.Fatal(err)
	}

	if err := fixture.service.Delete(ctx, fixture.userID, "attachment-1"); err == nil {
		t.Fatal("expected metadata delete failure")
	}
	if _, err := fixture.repository.GetByID(ctx, "attachment-1", fixture.userID); err != nil {
		t.Fatalf("metadata did not roll back: %v", err)
	}
	queued, err := fixture.repository.ListPendingDeletions(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 0 {
		t.Fatalf("outbox insert did not roll back: %#v", queued)
	}
	if fixture.storage.deleteCount() != 0 {
		t.Fatal("storage was touched before the SQL transaction committed")
	}
}

func TestAttachmentDeletionQueuePersistsAndRetries(t *testing.T) {
	fixture := newAttachmentDeletionFixture(t)
	ctx := context.Background()
	fixture.storage.setDeleteError(errors.New("storage temporarily unavailable"))

	if err := fixture.service.Delete(ctx, fixture.userID, "attachment-1"); err != nil {
		t.Fatalf("logical delete: %v", err)
	}
	if _, err := fixture.repository.GetByID(ctx, "attachment-1", fixture.userID); err == nil {
		t.Fatal("metadata still exists after logical deletion")
	}
	queued, err := fixture.repository.ListPendingDeletions(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || queued[0].Attempts != 1 || !strings.Contains(queued[0].LastError, "temporarily unavailable") {
		t.Fatalf("unexpected queued failure: %#v", queued)
	}

	fixture.storage.setDeleteError(nil)
	restartedService := service.NewAttachmentService(
		fixture.repository,
		sqliterepo.NewSQLiteTransactionRepository(fixture.database),
		fixture.storage,
		nil,
		service.DefaultAttachmentMaxBytes,
		sqliterepo.NewSQLiteTransactionManager(fixture.database),
	)
	if err := restartedService.DrainDeletionQueue(ctx, 10); err != nil {
		t.Fatalf("retry after service restart: %v", err)
	}
	queued, err = fixture.repository.ListPendingDeletions(ctx, 10)
	if err != nil || len(queued) != 0 {
		t.Fatalf("queue after successful retry = %#v, %v", queued, err)
	}
	if fixture.storage.deleteCount() != 2 {
		t.Fatalf("storage delete calls = %d, want 2", fixture.storage.deleteCount())
	}
}

func TestAttachmentDeletionWorkerStopsAfterCancellation(t *testing.T) {
	fixture := newAttachmentDeletionFixture(t)
	ctx := context.Background()
	fixture.storage.setDeleteError(errors.New("keep queued"))
	if err := fixture.repository.EnqueueDeletion(ctx, "queued/object.png", fixture.userID); err != nil {
		t.Fatal(err)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		fixture.service.RunDeletionQueueWorker(workerCtx, 5*time.Millisecond, 10)
	}()

	deadline := time.Now().Add(time.Second)
	for fixture.storage.deleteCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if fixture.storage.deleteCount() < 2 {
		t.Fatal("worker did not perform startup and periodic drains")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not return after cancellation")
	}
	countAfterStop := fixture.storage.deleteCount()
	time.Sleep(20 * time.Millisecond)
	if fixture.storage.deleteCount() != countAfterStop {
		t.Fatal("worker continued draining after cancellation")
	}
}

func TestAttachmentUploadEnforcesPerUserQuotas(t *testing.T) {
	tests := []struct {
		name        string
		maxFiles    int64
		maxTotal    int64
		sizeHint    int64
		storedSize  int64
		wantSaves   int
		wantDeletes int
		deleteErr   error
		wantQueued  int
	}{
		{
			name:       "file count rejected before storage",
			maxFiles:   1,
			maxTotal:   100,
			sizeHint:   1,
			storedSize: 1,
		},
		{
			name:       "declared bytes rejected before storage",
			maxFiles:   10,
			maxTotal:   5,
			sizeHint:   5,
			storedSize: 5,
		},
		{
			name:        "actual bytes rejected after storage",
			maxFiles:    10,
			maxTotal:    5,
			sizeHint:    1,
			storedSize:  5,
			wantSaves:   1,
			wantDeletes: 1,
		},
		{
			name:        "failed storage compensation is durably queued",
			maxFiles:    10,
			maxTotal:    5,
			sizeHint:    1,
			storedSize:  5,
			wantSaves:   1,
			wantDeletes: 1,
			deleteErr:   errors.New("storage delete unavailable"),
			wantQueued:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newAttachmentDeletionFixture(t)
			storage := &quotaAttachmentStorage{storedSize: tt.storedSize, deleteErr: tt.deleteErr}
			attachmentService := service.NewAttachmentService(
				fixture.repository,
				sqliterepo.NewSQLiteTransactionRepository(fixture.database),
				storage,
				nil,
				service.DefaultAttachmentMaxBytes,
				sqliterepo.NewSQLiteTransactionManager(fixture.database),
			)
			attachmentService.ConfigureQuota(tt.maxFiles, tt.maxTotal)

			_, err := attachmentService.Upload(context.Background(), service.UploadAttachmentRequest{
				UserID:           fixture.userID,
				OriginalFilename: "quota.png",
				ContentType:      "image/png",
				SizeBytesHint:    tt.sizeHint,
				Reader:           strings.NewReader("x"),
			})
			if !errors.Is(err, service.ErrAttachmentQuotaExceeded) {
				t.Fatalf("Upload() error = %v, want quota exceeded", err)
			}
			saves, deletes := storage.counts()
			if saves != tt.wantSaves || deletes != tt.wantDeletes {
				t.Fatalf("storage saves=%d deletes=%d, want %d/%d", saves, deletes, tt.wantSaves, tt.wantDeletes)
			}
			usage, err := fixture.repository.UsageByUser(context.Background(), fixture.userID)
			if err != nil {
				t.Fatal(err)
			}
			if usage.FileCount != 1 || usage.TotalBytes != 1 {
				t.Fatalf("usage after rejected upload = %#v", usage)
			}
			queued, err := fixture.repository.ListPendingDeletions(context.Background(), 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(queued) != tt.wantQueued {
				t.Fatalf("queued cleanup count=%d, want %d", len(queued), tt.wantQueued)
			}
			if tt.wantQueued == 1 && (queued[0].Attempts != 1 || !strings.Contains(queued[0].LastError, "storage delete unavailable")) {
				t.Fatalf("queued cleanup did not retain failure details: %#v", queued[0])
			}
		})
	}
}

func TestAttachmentConcurrentUploadsCannotExceedQuota(t *testing.T) {
	fixture := newAttachmentDeletionFixture(t)
	allSavesStarted := make(chan struct{})
	releaseSaves := make(chan struct{})
	storage := &quotaAttachmentStorage{
		storedSize:      1,
		waitForSaves:    2,
		allSavesStarted: allSavesStarted,
		releaseSaves:    releaseSaves,
	}
	attachmentService := service.NewAttachmentService(
		fixture.repository,
		sqliterepo.NewSQLiteTransactionRepository(fixture.database),
		storage,
		nil,
		service.DefaultAttachmentMaxBytes,
		sqliterepo.NewSQLiteTransactionManager(fixture.database),
	)
	// The fixture already owns one attachment. Both uploads must pass the
	// read-only preflight, while the final SQL arbiter admits only one.
	attachmentService.ConfigureQuota(2, 100)

	results := make(chan error, 2)
	for index := 0; index < 2; index++ {
		go func(index int) {
			_, err := attachmentService.Upload(context.Background(), service.UploadAttachmentRequest{
				UserID:           fixture.userID,
				OriginalFilename: fmt.Sprintf("concurrent-%d.png", index),
				ContentType:      "image/png",
				SizeBytesHint:    1,
				Reader:           strings.NewReader("x"),
			})
			results <- err
		}(index)
	}
	select {
	case <-allSavesStarted:
	case <-time.After(time.Second):
		t.Fatal("concurrent uploads did not both pass quota preflight")
	}
	close(releaseSaves)

	succeeded := 0
	rejected := 0
	for index := 0; index < 2; index++ {
		select {
		case err := <-results:
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, service.ErrAttachmentQuotaExceeded):
				rejected++
			default:
				t.Fatalf("concurrent upload returned unexpected error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent upload did not finish")
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent results succeeded=%d rejected=%d, want 1/1", succeeded, rejected)
	}
	usage, err := fixture.repository.UsageByUser(context.Background(), fixture.userID)
	if err != nil {
		t.Fatal(err)
	}
	if usage.FileCount != 2 || usage.TotalBytes != 2 {
		t.Fatalf("quota was exceeded under concurrency: %#v", usage)
	}
	if saves, deletes := storage.counts(); saves != 2 || deletes != 1 {
		t.Fatalf("storage saves=%d deletes=%d, want 2/1", saves, deletes)
	}
}

func TestCleanupExpiredOrphanAttachmentsQueuesBeforeMetadataRemoval(t *testing.T) {
	fixture := newAttachmentDeletionFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()

	accountID := "attachment-account"
	if err := sqliterepo.NewSQLiteAccountRepository(fixture.database).Create(ctx, model.Account{
		ID: accountID, UserID: fixture.userID, Name: "Attachment Account",
		Type: model.AccountTypePersonal, Currency: "CNY", Version: 1, IsActive: true,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	transactionID := "linked-transaction"
	if err := sqliterepo.NewSQLiteTransactionRepository(fixture.database).Create(ctx, model.Transaction{
		ID: transactionID, UserID: fixture.userID, AccountID: accountID,
		LedgerDir: model.LedgerDebit, TxType: model.TxTypeExpense,
		AmountCents: 100, Currency: "CNY", ExchangeRate: 1,
		BaseCurrency: "CNY", BaseAmountCents: 100,
		Category: "attachment", Mode: model.ModeWork, OccurredAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	createAttachment := func(id string, createdAt time.Time, linked bool) {
		t.Helper()
		var linkedTransactionID *string
		if linked {
			linkedTransactionID = &transactionID
		}
		if err := fixture.repository.Create(ctx, model.Attachment{
			ID: id, UserID: fixture.userID, TransactionID: linkedTransactionID,
			StorageKey:       fixture.userID + "/" + id + ".png",
			OriginalFilename: id + ".png", ContentType: "image/png", SizeBytes: 1,
			SHA256: strings.Repeat("c", 64), Kind: model.AttachmentKindReceipt,
			OCRStatus: model.OCRStatusNotRequested, CreatedAt: createdAt, UpdatedAt: createdAt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	createAttachment("old-orphan-1", now.Add(-48*time.Hour), false)
	createAttachment("old-orphan-2", now.Add(-47*time.Hour), false)
	createAttachment("old-linked", now.Add(-49*time.Hour), true)
	createAttachment("recent-orphan", now.Add(-time.Hour), false)

	cleaned, err := fixture.service.CleanupExpiredOrphans(ctx, now.Add(-24*time.Hour), 1)
	if err != nil {
		t.Fatal(err)
	}
	if cleaned != 1 {
		t.Fatalf("first cleanup count = %d, want 1", cleaned)
	}
	if _, err := fixture.repository.GetByID(ctx, "old-orphan-1", fixture.userID); err == nil {
		t.Fatal("oldest orphan metadata still exists")
	}
	if _, err := fixture.repository.GetByID(ctx, "old-orphan-2", fixture.userID); err != nil {
		t.Fatalf("batch limit removed second orphan: %v", err)
	}

	cleaned, err = fixture.service.CleanupExpiredOrphans(ctx, now.Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if cleaned != 1 {
		t.Fatalf("second cleanup count = %d, want 1", cleaned)
	}
	for _, id := range []string{"old-linked", "recent-orphan", "attachment-1"} {
		if _, err := fixture.repository.GetByID(ctx, id, fixture.userID); err != nil {
			t.Fatalf("protected attachment %q was removed: %v", id, err)
		}
	}
	queued, err := fixture.repository.ListPendingDeletions(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 2 {
		t.Fatalf("queued deletions = %#v, want two expired orphans", queued)
	}
}

func TestCleanupExpiredOrphanAttachmentsRollsBackQueueOnDeleteFailure(t *testing.T) {
	fixture := newAttachmentDeletionFixture(t)
	ctx := context.Background()
	old := time.Now().UTC().Add(-48 * time.Hour)
	if err := fixture.repository.Create(ctx, model.Attachment{
		ID: "old-rollback", UserID: fixture.userID, StorageKey: fixture.userID + "/old-rollback.png",
		OriginalFilename: "old-rollback.png", ContentType: "image/png", SizeBytes: 1,
		SHA256: strings.Repeat("d", 64), Kind: model.AttachmentKindReceipt,
		OCRStatus: model.OCRStatusNotRequested, CreatedAt: old, UpdatedAt: old,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.database.ExecContext(ctx, `
		CREATE TRIGGER reject_orphan_cleanup BEFORE DELETE ON attachments
		WHEN OLD.id = 'old-rollback'
		BEGIN
			SELECT RAISE(ABORT, 'forced orphan cleanup failure');
		END;`); err != nil {
		t.Fatal(err)
	}

	if _, err := fixture.service.CleanupExpiredOrphans(ctx, time.Now().UTC().Add(-24*time.Hour), 10); err == nil {
		t.Fatal("expected orphan metadata delete failure")
	}
	if _, err := fixture.repository.GetByID(ctx, "old-rollback", fixture.userID); err != nil {
		t.Fatalf("orphan metadata did not roll back: %v", err)
	}
	queued, err := fixture.repository.ListPendingDeletions(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 0 {
		t.Fatalf("orphan deletion queue did not roll back: %#v", queued)
	}
}
