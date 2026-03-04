package test

import (
	"context"
	"database/sql"
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

type flowSender struct {
	email.NoopSender
	verifyToken   string
	resetToken    string
	emailOldToken string
	emailNewToken string
	deleteToken   string
}

func (f *flowSender) SendVerification(_, _, token string) error  { f.verifyToken = token; return nil }
func (f *flowSender) SendPasswordReset(_, _, token string) error { f.resetToken = token; return nil }
func (f *flowSender) SendEmailChangeOldVerify(_, _, _, token string) error {
	f.emailOldToken = token
	return nil
}
func (f *flowSender) SendEmailChange(_, _, token string) error     { f.emailNewToken = token; return nil }
func (f *flowSender) SendAccountDeletion(_, _, token string) error { f.deleteToken = token; return nil }

func setupFlowAuth(t *testing.T) (*service.AuthService, *sqliterepo.SQLiteUserRepository, *flowSender, *sql.DB, model.User) {
	db := setupDB(t)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqliterepo.NewSQLiteUserRepository(db)
	sender := &flowSender{}
	svc := service.NewAuthService(repo, auth.NewJWTService("test-secret"), auth.NewActionTokenService("test-secret"), auth.NewLoginAttemptTracker(5, time.Minute), sender, true, "http://localhost")
	pwd, _ := auth.HashPassword("Password123")
	u := model.User{ID: uuid.NewString(), Email: "flow@test.com", Username: "flow", Name: "flow", Nickname: "flow", PasswordHash: pwd, Role: "owner", EmailVerified: false, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return svc, repo, sender, db, u
}

func TestVerifyEmailReplay(t *testing.T) {
	svc, _, sender, _, u := setupFlowAuth(t)
	if err := svc.ResendVerification(context.Background(), u.Email); err != nil {
		t.Fatal(err)
	}
	if err := svc.VerifyEmail(context.Background(), sender.verifyToken); err != nil {
		t.Fatal(err)
	}
	if err := svc.VerifyEmail(context.Background(), sender.verifyToken); err != service.ErrAlreadyUsed && err != service.ErrInvalidToken {
		t.Fatalf("expected replay protection, got %v", err)
	}
}

func TestPasswordResetExpiredAndReplay(t *testing.T) {
	svc, _, sender, db, u := setupFlowAuth(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ForgotPassword(ctx, u.Email); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE action_requests SET expires_at = ? WHERE action = ?`, time.Now().Add(-time.Hour).Unix(), service.ActionPasswordReset); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(ctx, sender.resetToken, "NewPassword123"); err != service.ErrExpiredToken {
		t.Fatalf("want expired, got %v", err)
	}
	if err := svc.ForgotPassword(ctx, u.Email); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(ctx, sender.resetToken, "NewPassword123"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(ctx, sender.resetToken, "OtherPassword123"); err != service.ErrAlreadyUsed && err != service.ErrInvalidToken {
		t.Fatalf("replay must fail: %v", err)
	}
}

func TestEmailChangeRequiresReauthAndConcurrentConsume(t *testing.T) {
	svc, repo, sender, _, u := setupFlowAuth(t)
	ctx := context.Background()
	if err := svc.RequestEmailChange(ctx, u.ID, "bad-pass", "new@test.com"); err != service.ErrNotAuthorized {
		t.Fatalf("want not_authorized, got %v", err)
	}
	if err := svc.RequestEmailChange(ctx, u.ID, "Password123", "new@test.com"); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- svc.ConfirmOldEmailForChange(ctx, sender.emailOldToken) }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("expected exactly one successful consume, got %d", success)
	}
	if err := svc.ConfirmEmailChange(ctx, sender.emailNewToken); err != nil {
		t.Fatal(err)
	}
	u2, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if u2.Email != "new@test.com" {
		t.Fatalf("email not updated: %s", u2.Email)
	}
}
