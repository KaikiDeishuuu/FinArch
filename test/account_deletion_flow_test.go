package test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	"finarch/internal/infrastructure/email"
	sqliterepo "finarch/internal/infrastructure/repository"

	"github.com/google/uuid"
)

type failingUserDataCleaner struct {
	called      bool
	drainCalled bool
	err         error
	drainErr    error
}

type accountDeletionStorage struct {
	mu        sync.Mutex
	deleteErr error
	deletes   int
}

func (*accountDeletionStorage) Save(context.Context, string, string, string, string, io.Reader, int64) (service.StoredAttachment, error) {
	return service.StoredAttachment{}, errors.New("not implemented")
}

func (*accountDeletionStorage) Restore(context.Context, string, io.Reader) error { return nil }

func (*accountDeletionStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (s *accountDeletionStorage) Delete(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes++
	return s.deleteErr
}

func (s *accountDeletionStorage) setDeleteError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteErr = err
}

func (s *accountDeletionStorage) deleteCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deletes
}

func (c *failingUserDataCleaner) EnqueueUserDataDeletion(context.Context, string) error {
	c.called = true
	return c.err
}

func (c *failingUserDataCleaner) DrainDeletionQueue(context.Context, int) error {
	c.drainCalled = true
	return c.drainErr
}

type captureSender struct {
	email.NoopSender
	token string
}

func (c *captureSender) SendAccountDeletion(_, _, token string) error {
	c.token = token
	return nil
}

func newAuthSvc(t *testing.T) (*service.AuthService, *sqliterepo.SQLiteUserRepository, *captureSender, *sql.DB) {
	db := setupDB(t)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqliterepo.NewSQLiteUserRepository(db)
	sender := &captureSender{}
	jwt := auth.NewJWTService("test-secret")
	actions := auth.NewActionTokenService("test-secret")
	tracker := auth.NewLoginAttemptTracker(5, time.Minute)
	svc := service.NewAuthService(repo, actions, tracker, sender, false, "http://localhost:5173", sqliterepo.NewSQLiteTransactionManager(db), newTestSessionService(t, db, jwt, "test-secret"))

	u := model.User{
		ID:            uuid.NewString(),
		Email:         "delete@test.com",
		Username:      "deleter",
		Name:          "deleter",
		Nickname:      "deleter",
		PasswordHash:  "x",
		Role:          "owner",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return svc, repo, sender, db
}

func addAccountDeletionAttachment(t *testing.T, authSvc *service.AuthService, database *sql.DB, userID string, deleteErr error) (*service.AttachmentService, *sqliterepo.SQLiteAttachmentRepository, *accountDeletionStorage) {
	t.Helper()
	now := time.Now().UTC()
	attachmentRepo := sqliterepo.NewSQLiteAttachmentRepository(database)
	if err := attachmentRepo.Create(context.Background(), model.Attachment{
		ID: "account-delete-attachment", UserID: userID, StorageKey: userID + "/account-delete.png",
		OriginalFilename: "receipt.png", ContentType: "image/png", SizeBytes: 1,
		SHA256: strings.Repeat("b", 64), Kind: model.AttachmentKindReceipt,
		OCRStatus: model.OCRStatusNotRequested, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	storage := &accountDeletionStorage{deleteErr: deleteErr}
	attachmentSvc := service.NewAttachmentService(
		attachmentRepo,
		sqliterepo.NewSQLiteTransactionRepository(database),
		storage,
		nil,
		service.DefaultAttachmentMaxBytes,
		sqliterepo.NewSQLiteTransactionManager(database),
	)
	authSvc.SetUserDataCleaner(attachmentSvc)
	return attachmentSvc, attachmentRepo, storage
}

func TestAccountDeletion_FullFlowAndReplay(t *testing.T) {
	svc, repo, sender, _ := newAuthSvc(t)
	ctx := context.Background()
	u, _ := repo.GetByEmail(ctx, "delete@test.com")

	if err := svc.RequestAccountDeletion(ctx, u.ID); err != nil {
		t.Fatalf("request deletion: %v", err)
	}
	if sender.token == "" {
		t.Fatal("expected token captured from email sender")
	}

	if err := svc.ConfirmAccountDeletion(ctx, sender.token); err != nil {
		t.Fatalf("confirm deletion: %v", err)
	}
	if _, err := repo.GetByID(ctx, u.ID); err == nil {
		t.Fatal("expected deleted user lookup to fail")
	}

	if err := svc.ConfirmAccountDeletion(ctx, sender.token); err != service.ErrInvalidToken {
		t.Fatalf("expected replay invalid token, got %v", err)
	}
}

func TestAccountDeletion_ExpiredToken(t *testing.T) {
	svc, repo, sender, db := newAuthSvc(t)
	ctx := context.Background()
	u, _ := repo.GetByEmail(ctx, "delete@test.com")

	if err := svc.RequestAccountDeletion(ctx, u.ID); err != nil {
		t.Fatalf("request deletion: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE action_requests SET expires_at = ? WHERE action = ?`, time.Now().Add(-time.Hour).Unix(), service.ActionAccountDelete); err != nil {
		t.Fatalf("expire token row: %v", err)
	}
	if err := svc.ConfirmAccountDeletion(ctx, sender.token); err != service.ErrExpiredToken {
		t.Fatalf("expected expired token error, got %v", err)
	}
}

func TestAccountDeletion_DeleteFailureDoesNotDeleteUser(t *testing.T) {
	svc, repo, sender, db := newAuthSvc(t)
	ctx := context.Background()
	u, _ := repo.GetByEmail(ctx, "delete@test.com")
	_, attachmentRepo, storage := addAccountDeletionAttachment(t, svc, db, u.ID, nil)

	if err := svc.RequestAccountDeletion(ctx, u.ID); err != nil {
		t.Fatalf("request deletion: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		CREATE TRIGGER deny_user_delete BEFORE DELETE ON users
		BEGIN
			SELECT RAISE(ABORT, 'deny delete');
		END;`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if err := svc.ConfirmAccountDeletion(ctx, sender.token); err == nil {
		t.Fatal("expected deletion failure")
	}
	if _, err := repo.GetByID(ctx, u.ID); err != nil {
		t.Fatalf("user should still exist after failed deletion: %v", err)
	}
	if _, err := attachmentRepo.GetByID(ctx, "account-delete-attachment", u.ID); err != nil {
		t.Fatalf("attachment metadata should roll back with user deletion: %v", err)
	}
	queued, err := attachmentRepo.ListPendingDeletions(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 0 {
		t.Fatalf("attachment outbox should roll back with user deletion: %#v", queued)
	}
	if storage.deleteCount() != 0 {
		t.Fatal("storage cleanup ran even though account deletion rolled back")
	}
}

func TestAccountDeletion_ExternalCleanupFailurePreservesMetadata(t *testing.T) {
	svc, repo, sender, db := newAuthSvc(t)
	ctx := context.Background()
	u, _ := repo.GetByEmail(ctx, "delete@test.com")
	cleaner := &failingUserDataCleaner{err: errors.New("storage unavailable")}
	svc.SetUserDataCleaner(cleaner)

	if err := svc.RequestAccountDeletion(ctx, u.ID); err != nil {
		t.Fatalf("request deletion: %v", err)
	}
	if err := svc.ConfirmAccountDeletion(ctx, sender.token); err == nil {
		t.Fatal("expected external cleanup failure")
	}
	if !cleaner.called {
		t.Fatal("external user-data cleaner was not called")
	}
	if _, err := repo.GetByID(ctx, u.ID); err != nil {
		t.Fatalf("user metadata must remain after cleanup failure: %v", err)
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE action = ? AND user_id = ?`, service.ActionAccountDelete, u.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("deletion token status = %q, want pending", status)
	}
}

func TestAccountDeletionPersistsAttachmentCleanupAfterUserRemoval(t *testing.T) {
	svc, repo, sender, database := newAuthSvc(t)
	ctx := context.Background()
	u, _ := repo.GetByEmail(ctx, "delete@test.com")
	attachmentSvc, attachmentRepo, storage := addAccountDeletionAttachment(
		t, svc, database, u.ID, errors.New("storage offline"),
	)

	if err := svc.RequestAccountDeletion(ctx, u.ID); err != nil {
		t.Fatalf("request deletion: %v", err)
	}
	if err := svc.ConfirmAccountDeletion(ctx, sender.token); err != nil {
		t.Fatalf("durably queued account deletion must succeed: %v", err)
	}
	if _, err := repo.GetByID(ctx, u.ID); err == nil {
		t.Fatal("user still exists after confirmed deletion")
	}
	if _, err := attachmentRepo.GetByID(ctx, "account-delete-attachment", u.ID); err == nil {
		t.Fatal("attachment metadata still exists after account deletion")
	}
	queued, err := attachmentRepo.ListPendingDeletions(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || queued[0].UserID != u.ID || queued[0].Attempts != 1 || !strings.Contains(queued[0].LastError, "storage offline") {
		t.Fatalf("unexpected durable cleanup state after user deletion: %#v", queued)
	}

	storage.setDeleteError(nil)
	if err := attachmentSvc.DrainDeletionQueue(ctx, 10); err != nil {
		t.Fatalf("retry attachment cleanup: %v", err)
	}
	queued, err = attachmentRepo.ListPendingDeletions(ctx, 10)
	if err != nil || len(queued) != 0 {
		t.Fatalf("cleanup queue after retry = %#v, %v", queued, err)
	}
}
