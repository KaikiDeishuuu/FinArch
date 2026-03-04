package test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	"finarch/internal/infrastructure/email"
	sqliterepo "finarch/internal/infrastructure/repository"

	"github.com/google/uuid"
)

type captureSender struct {
	email.NoopSender
	token string
}

func (c *captureSender) SendAccountDeletion(_, _, token string) error {
	c.token = token
	return nil
}

func newAuthSvc(t *testing.T, ttl time.Duration) (*service.AuthService, *sqliterepo.SQLiteUserRepository, *captureSender, *sql.DB) {
	db := setupDB(t)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqliterepo.NewSQLiteUserRepository(db)
	sender := &captureSender{}
	jwt := auth.NewJWTService("test-secret")
	del := auth.NewDeletionTokenService("test-secret", ttl)
	tracker := auth.NewLoginAttemptTracker(5, time.Minute)
	svc := service.NewAuthService(repo, jwt, del, tracker, sender, false, "http://localhost:5173")

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

func TestAccountDeletion_FullFlowAndReplay(t *testing.T) {
	svc, repo, sender, _ := newAuthSvc(t, 30*time.Minute)
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

	if err := svc.ConfirmAccountDeletion(ctx, sender.token); err != service.ErrDeletionAlreadyCompleted {
		t.Fatalf("expected replay to be already completed, got %v", err)
	}
}

func TestAccountDeletion_ExpiredToken(t *testing.T) {
	svc, repo, sender, _ := newAuthSvc(t, -time.Minute)
	ctx := context.Background()
	u, _ := repo.GetByEmail(ctx, "delete@test.com")

	if err := svc.RequestAccountDeletion(ctx, u.ID); err != nil {
		t.Fatalf("request deletion: %v", err)
	}
	if err := svc.ConfirmAccountDeletion(ctx, sender.token); err != service.ErrDeletionTokenExpired {
		t.Fatalf("expected expired token error, got %v", err)
	}
}

func TestAccountDeletion_DeleteFailureDoesNotDeleteUser(t *testing.T) {
	svc, repo, sender, db := newAuthSvc(t, 30*time.Minute)
	ctx := context.Background()
	u, _ := repo.GetByEmail(ctx, "delete@test.com")

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
}
