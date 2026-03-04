package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	"finarch/internal/infrastructure/email"
	sqliterepo "finarch/internal/infrastructure/repository"
)

func newAuthSvcForRegister(t *testing.T) *service.AuthService {
	t.Helper()
	db := setupDB(t)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqliterepo.NewSQLiteUserRepository(db)
	return service.NewAuthService(repo, auth.NewJWTService("test-secret"), auth.NewActionTokenService("test-secret"), auth.NewLoginAttemptTracker(5, time.Minute), &email.NoopSender{}, false, "http://localhost", sqliterepo.NewSQLiteTransactionManager(db))
}

func TestRegister_AllowsDuplicateNickname(t *testing.T) {
	svc := newAuthSvcForRegister(t)
	ctx := context.Background()

	_, err := svc.Register(ctx, service.RegisterRequest{Email: "a@example.com", Username: "user_a", Password: "Password123", Nickname: "SameNick"})
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	_, err = svc.Register(ctx, service.RegisterRequest{Email: "b@example.com", Username: "user_b", Password: "Password123", Nickname: "SameNick"})
	if err != nil {
		t.Fatalf("duplicate nickname should be allowed: %v", err)
	}
}

func TestRegister_RejectsDuplicateUsername(t *testing.T) {
	svc := newAuthSvcForRegister(t)
	ctx := context.Background()

	_, err := svc.Register(ctx, service.RegisterRequest{Email: "a2@example.com", Username: "dup_user", Password: "Password123", Nickname: "Nick1"})
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	_, err = svc.Register(ctx, service.RegisterRequest{Email: "b2@example.com", Username: "dup_user", Password: "Password123", Nickname: "Nick2"})
	if !errors.Is(err, service.ErrUsernameTaken) {
		t.Fatalf("expected username taken error, got %v", err)
	}
}

// TestConcurrentRegistrationCheckThenAct verifies that concurrent registration attempts
// for the same email or username correctly fail instead of creating orphaned state.
func TestConcurrentRegistrationCheckThenAct(t *testing.T) {
	svc := newAuthSvcForRegister(t)
	ctx := context.Background()

	const numGoroutines = 10
	errCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, err := svc.Register(ctx, service.RegisterRequest{
				Email:    "concurrent@example.com",
				Username: "concurrent_user",
				Password: "Password123",
				Nickname: "Concurrent",
			})
			errCh <- err
		}()
	}

	successCount := 0
	for i := 0; i < numGoroutines; i++ {
		if err := <-errCh; err == nil {
			successCount++
		}
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 success, got %d", successCount)
	}
}
