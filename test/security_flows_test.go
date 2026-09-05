package test

import (
	"context"
	"database/sql"
	"errors"
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
	jwt := auth.NewJWTService("test-secret")
	svc := service.NewAuthService(repo, auth.NewActionTokenService("test-secret"), auth.NewLoginAttemptTracker(5, time.Minute), sender, true, "http://localhost", sqliterepo.NewSQLiteTransactionManager(db), newTestSessionService(t, db, jwt, "test-secret"))
	pwd, _ := auth.HashPassword("Password123")
	u := model.User{ID: uuid.NewString(), Email: "flow@test.com", Username: "flow", Name: "flow", Nickname: "flow", PasswordHash: pwd, Role: "owner", EmailVerified: false, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return svc, repo, sender, db, u
}

func TestActionTokenTTLsMatchEmailContract(t *testing.T) {
	svc, _, sender, db, u := setupFlowAuth(t)
	ctx := context.Background()

	if err := svc.ResendVerification(ctx, u.Email); err != nil {
		t.Fatal(err)
	}
	verificationToken := sender.verifyToken

	if _, err := db.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ForgotPassword(ctx, u.Email); err != nil {
		t.Fatal(err)
	}
	if err := svc.RequestEmailChange(ctx, u.ID, "Password123", "ttl-change@test.com"); err != nil {
		t.Fatal(err)
	}
	oldEmailToken := sender.emailOldToken
	if err := svc.ConfirmOldEmailForChange(ctx, oldEmailToken); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		token  string
		action string
		want   time.Duration
	}{
		{name: "registration verification stays 24 hours", token: verificationToken, action: service.ActionRegisterVerify, want: 24 * time.Hour},
		{name: "password reset", token: sender.resetToken, action: service.ActionPasswordReset, want: time.Hour},
		{name: "old email authorization", token: oldEmailToken, action: service.ActionEmailChangeOld, want: time.Hour},
		{name: "new email confirmation", token: sender.emailNewToken, action: service.ActionEmailChangeNew, want: time.Hour},
	}
	verifier := auth.NewActionTokenService("test-secret")
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims, err := verifier.Verify(test.token, test.action)
			if err != nil {
				t.Fatalf("verify issued token: %v", err)
			}
			if claims.IssuedAt == nil || claims.ExpiresAt == nil {
				t.Fatal("issued token is missing its time boundary")
			}
			if got := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time); got != test.want {
				t.Fatalf("signed TTL = %s, want %s", got, test.want)
			}

			var storedExpiry int64
			if err := db.QueryRowContext(ctx, `SELECT expires_at FROM action_requests WHERE jti = ?`, claims.ID).Scan(&storedExpiry); err != nil {
				t.Fatalf("load persisted expiry: %v", err)
			}
			if want := claims.ExpiresAt.Time.Unix(); storedExpiry != want {
				t.Fatalf("persisted expiry = %d, want signed expiry %d", storedExpiry, want)
			}
		})
	}
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

func TestPasswordResetRejectsInvalidTokenBeforePasswordHash(t *testing.T) {
	svc, _, _, _, _ := setupFlowAuth(t)

	// bcrypt rejects passwords longer than 72 bytes. Receiving the token error
	// proves an unauthenticated caller cannot reach password hashing first.
	password := strings.Repeat("x", 73)
	if err := svc.ResetPassword(context.Background(), "not-a-token", password); !errors.Is(err, service.ErrInvalidToken) {
		t.Fatalf("ResetPassword error = %v, want invalid token before password hashing", err)
	}
}

func TestPasswordResetInvalidatesSiblingTokens(t *testing.T) {
	svc, repo, sender, db, u := setupFlowAuth(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ForgotPassword(ctx, u.Email); err != nil {
		t.Fatal(err)
	}
	firstToken := sender.resetToken
	firstClaims, err := auth.NewActionTokenService("test-secret").Verify(firstToken, service.ActionPasswordReset)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ForgotPassword(ctx, u.Email); err != nil {
		t.Fatal(err)
	}
	secondToken := sender.resetToken
	secondClaims, err := auth.NewActionTokenService("test-secret").Verify(secondToken, service.ActionPasswordReset)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.ResetPassword(ctx, firstToken, "FirstWinnerPassword123"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(ctx, secondToken, "SecondPasswordMustNotWin123"); !errors.Is(err, service.ErrExpiredToken) {
		t.Fatalf("sibling reset token error = %v, want expired", err)
	}
	updated, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.CheckPassword(updated.PasswordHash, "FirstWinnerPassword123"); err != nil {
		t.Fatal("winning reset password was not retained")
	}
	if err := auth.CheckPassword(updated.PasswordHash, "SecondPasswordMustNotWin123"); err == nil {
		t.Fatal("expired sibling reset token changed the password")
	}

	var firstStatus, secondStatus string
	if err := db.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE jti = ?`, firstClaims.ID).Scan(&firstStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE jti = ?`, secondClaims.ID).Scan(&secondStatus); err != nil {
		t.Fatal(err)
	}
	if firstStatus != "completed" || secondStatus != "expired" {
		t.Fatalf("reset statuses = (%s, %s), want (completed, expired)", firstStatus, secondStatus)
	}
}

func TestPasswordResetInvalidatesEmailChangeAndAccountDeletionAuthorizations(t *testing.T) {
	svc, repo, sender, db, u := setupFlowAuth(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.RequestEmailChange(ctx, u.ID, "Password123", "pending-after-reset@test.com"); err != nil {
		t.Fatal(err)
	}
	emailChangeToken := sender.emailOldToken
	if err := svc.RequestAccountDeletion(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	deleteToken := sender.deleteToken
	if err := svc.ForgotPassword(ctx, u.Email); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(ctx, sender.resetToken, "ResetWinsPassword123"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfirmOldEmailForChange(ctx, emailChangeToken); !errors.Is(err, service.ErrExpiredToken) {
		t.Fatalf("old email-change authorization survived reset: %v", err)
	}
	if err := svc.ConfirmAccountDeletion(ctx, deleteToken); !errors.Is(err, service.ErrExpiredToken) {
		t.Fatalf("account-delete authorization survived reset: %v", err)
	}
	updated, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PendingEmail != "" {
		t.Fatalf("password reset retained pending email %q", updated.PendingEmail)
	}
}

func TestEmailChangeInvalidatesPasswordResetAndAccountDeletionAuthorizations(t *testing.T) {
	svc, repo, sender, db, u := setupFlowAuth(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ForgotPassword(ctx, u.Email); err != nil {
		t.Fatal(err)
	}
	resetToken := sender.resetToken
	if err := svc.RequestAccountDeletion(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	deleteToken := sender.deleteToken
	if err := svc.RequestEmailChange(ctx, u.ID, "Password123", "credential-change@test.com"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfirmOldEmailForChange(ctx, sender.emailOldToken); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfirmEmailChange(ctx, sender.emailNewToken); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(ctx, resetToken, "MustNotWinPassword123"); !errors.Is(err, service.ErrExpiredToken) {
		t.Fatalf("password-reset authorization survived email change: %v", err)
	}
	if err := svc.ConfirmAccountDeletion(ctx, deleteToken); !errors.Is(err, service.ErrExpiredToken) {
		t.Fatalf("account-delete authorization survived email change: %v", err)
	}
	updated, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Email != "credential-change@test.com" {
		t.Fatalf("email change was not retained: %q", updated.Email)
	}
}

func TestLatestAccountDeletionRequestReplacesEarlierLink(t *testing.T) {
	svc, repo, sender, _, u := setupFlowAuth(t)
	ctx := context.Background()
	if err := svc.RequestAccountDeletion(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	first := sender.deleteToken
	if err := svc.RequestAccountDeletion(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	second := sender.deleteToken
	if err := svc.ConfirmAccountDeletion(ctx, first); !errors.Is(err, service.ErrExpiredToken) {
		t.Fatalf("superseded delete link error = %v, want expired", err)
	}
	if _, err := repo.GetByID(ctx, u.ID); err != nil {
		t.Fatalf("superseded link deleted user: %v", err)
	}
	if err := svc.ConfirmAccountDeletion(ctx, second); err != nil {
		t.Fatalf("latest delete link failed: %v", err)
	}
}

func TestConcurrentDistinctPasswordResetTokensHaveOneWinner(t *testing.T) {
	svc, repo, sender, db, u := setupFlowAuth(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ForgotPassword(ctx, u.Email); err != nil {
		t.Fatal(err)
	}
	firstToken := sender.resetToken
	if err := svc.ForgotPassword(ctx, u.Email); err != nil {
		t.Fatal(err)
	}
	secondToken := sender.resetToken

	type resetAttempt struct {
		password string
		err      error
	}
	start := make(chan struct{})
	results := make(chan resetAttempt, 2)
	for _, attempt := range []struct {
		token    string
		password string
	}{
		{firstToken, "ConcurrentFirstPassword123"},
		{secondToken, "ConcurrentSecondPassword123"},
	} {
		attempt := attempt
		go func() {
			<-start
			results <- resetAttempt{password: attempt.password, err: svc.ResetPassword(ctx, attempt.token, attempt.password)}
		}()
	}
	close(start)

	var winner string
	for i := 0; i < 2; i++ {
		attempt := <-results
		if attempt.err == nil {
			if winner != "" {
				t.Fatal("both distinct reset tokens succeeded")
			}
			winner = attempt.password
			continue
		}
		if !errors.Is(attempt.err, service.ErrExpiredToken) && !errors.Is(attempt.err, service.ErrAlreadyUsed) {
			t.Fatalf("losing reset error = %v", attempt.err)
		}
	}
	if winner == "" {
		t.Fatal("neither reset token succeeded")
	}
	updated, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.CheckPassword(updated.PasswordHash, winner); err != nil {
		t.Fatal("final password does not match the sole winning reset")
	}
	var completed, expired int
	if err := db.QueryRowContext(ctx, `
		SELECT SUM(status = 'completed'), SUM(status = 'expired')
		FROM action_requests
		WHERE user_id = ? AND action = ?`, u.ID, service.ActionPasswordReset).Scan(&completed, &expired); err != nil {
		t.Fatal(err)
	}
	if completed != 1 || expired != 1 {
		t.Fatalf("reset terminal counts = completed:%d expired:%d, want 1/1", completed, expired)
	}
}

func TestConcurrentBackupExportTokenConsumptionHasOneWinner(t *testing.T) {
	svc, _, _, _, u := setupFlowAuth(t)
	ctx := context.Background()
	token, err := svc.RequestBackupExport(ctx, u.ID, "Password123")
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			results <- svc.ConsumeBackupExportToken(ctx, u.ID, token)
		}()
	}
	close(start)

	successes := 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, service.ErrAlreadyUsed) {
			t.Fatalf("losing backup export consume error = %v, want already used", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful backup export consumes = %d, want 1", successes)
	}
}

func TestEmailChangeRequiresReauthAndConcurrentConsume(t *testing.T) {
	svc, repo, sender, _, u := setupFlowAuth(t)
	ctx := context.Background()
	if err := svc.RequestEmailChange(ctx, u.ID, "bad-pass", "new@test.com"); err != service.ErrInvalidPassword {
		t.Fatalf("want invalid_password, got %v", err)
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

func TestEmailChangeReplacementInvalidatesOldFlowAndBindsPendingEmail(t *testing.T) {
	svc, repo, sender, db, u := setupFlowAuth(t)
	ctx := context.Background()
	actionTokens := auth.NewActionTokenService("test-secret")

	if err := svc.RequestEmailChange(ctx, u.ID, "Password123", "first-new@test.com"); err != nil {
		t.Fatal(err)
	}
	firstOldToken := sender.emailOldToken
	firstOldClaims, err := actionTokens.Verify(firstOldToken, service.ActionEmailChangeOld)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfirmOldEmailForChange(ctx, firstOldToken); err != nil {
		t.Fatal(err)
	}
	firstNewToken := sender.emailNewToken
	firstNewClaims, err := actionTokens.Verify(firstNewToken, service.ActionEmailChangeNew)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.RequestEmailChange(ctx, u.ID, "Password123", "second-new@test.com"); err != nil {
		t.Fatal(err)
	}
	secondOldToken := sender.emailOldToken

	var firstOldStatus, firstNewStatus string
	if err := db.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE jti = ?`, firstOldClaims.ID).Scan(&firstOldStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE jti = ?`, firstNewClaims.ID).Scan(&firstNewStatus); err != nil {
		t.Fatal(err)
	}
	if firstOldStatus != "completed" || firstNewStatus != "expired" {
		t.Fatalf("replaced email-flow statuses = old:%s new:%s", firstOldStatus, firstNewStatus)
	}
	if err := svc.ConfirmEmailChange(ctx, firstNewToken); !errors.Is(err, service.ErrExpiredToken) {
		t.Fatalf("replaced new-email token error = %v, want expired", err)
	}

	// Simulate a stale/legacy row that was not invalidated. The final step must
	// still bind the signed destination to the user's current pending_email.
	if _, err := db.ExecContext(ctx, `UPDATE action_requests SET status = 'pending' WHERE jti = ?`, firstNewClaims.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfirmEmailChange(ctx, firstNewToken); !errors.Is(err, service.ErrInvalidToken) {
		t.Fatalf("stale mismatched new-email token error = %v, want invalid", err)
	}
	current, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Email != u.Email || current.PendingEmail != "second-new@test.com" {
		t.Fatalf("stale token changed replacement state: email=%q pending=%q", current.Email, current.PendingEmail)
	}

	if err := svc.ConfirmOldEmailForChange(ctx, secondOldToken); err != nil {
		t.Fatal(err)
	}
	secondNewToken := sender.emailNewToken
	if err := svc.ConfirmEmailChange(ctx, secondNewToken); err != nil {
		t.Fatal(err)
	}
	current, err = repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Email != "second-new@test.com" || current.PendingEmail != "" {
		t.Fatalf("replacement email flow did not complete: email=%q pending=%q", current.Email, current.PendingEmail)
	}
}

func TestEmailChangeReplacementRollsBackIfTokenCreationFails(t *testing.T) {
	svc, repo, sender, db, u := setupFlowAuth(t)
	ctx := context.Background()
	if err := svc.RequestEmailChange(ctx, u.ID, "Password123", "preserved@test.com"); err != nil {
		t.Fatal(err)
	}
	preservedClaims, err := auth.NewActionTokenService("test-secret").Verify(sender.emailOldToken, service.ActionEmailChangeOld)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TRIGGER fail_email_change_action_insert
		BEFORE INSERT ON action_requests
		WHEN NEW.action = 'email_change_old'
		BEGIN
			SELECT RAISE(ABORT, 'injected email-change action insert failure');
		END`); err != nil {
		t.Fatal(err)
	}

	if err := svc.RequestEmailChange(ctx, u.ID, "Password123", "must-rollback@test.com"); err == nil {
		t.Fatal("replacement flow succeeded despite action insert failure")
	}
	current, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.PendingEmail != "preserved@test.com" {
		t.Fatalf("pending email escaped failed replacement transaction: %q", current.PendingEmail)
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE jti = ?`, preservedClaims.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("prior email-change token was invalidated despite rollback: %s", status)
	}
}

func TestChangePasswordInvalidatesPendingResetTokens(t *testing.T) {
	svc, _, sender, db, u := setupFlowAuth(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ForgotPassword(ctx, u.Email); err != nil {
		t.Fatal(err)
	}
	resetToken := sender.resetToken
	if err := svc.ChangePassword(ctx, u.ID, "Password123", "UserChosenPassword123"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(ctx, resetToken, "StaleResetPassword123"); !errors.Is(err, service.ErrExpiredToken) {
		t.Fatalf("reset token after password change error = %v, want expired", err)
	}
}

func TestConcurrentTokenConsumption(t *testing.T) {
	svc, _, sender, db, u := setupFlowAuth(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ForgotPassword(ctx, u.Email); err != nil {
		t.Fatal(err)
	}

	const numGoroutines = 15
	errCh := make(chan error, numGoroutines)
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- svc.ResetPassword(ctx, sender.resetToken, "NewSecurePassword123")
		}()
	}

	wg.Wait()
	close(errCh)

	successCount := 0
	for err := range errCh {
		if err == nil {
			successCount++
		}
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful token consumption, got %d", successCount)
	}
}

func TestDisasterRecoveryTokenVerifyDoesNotConsume(t *testing.T) {
	svc, _, _, db, u := setupFlowAuth(t)
	ctx := context.Background()

	token, err := svc.RequestDisasterRecovery(ctx, u.ID, "Password123")
	if err != nil {
		t.Fatal(err)
	}
	req, err := svc.VerifyDisasterRecoveryToken(ctx, u.ID, token)
	if err != nil {
		t.Fatal(err)
	}

	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE jti = ?`, req.JTI).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("verify consumed token: status=%s", status)
	}
	if got := auditEventCount(t, db, u.ID, "disaster_recovery_executed"); got != 0 {
		t.Fatalf("verify wrote execution audit event: got %d", got)
	}
	if got := auditEventCount(t, db, u.ID, "disaster_recovery_started"); got != 0 {
		t.Fatalf("verify wrote started audit event: got %d", got)
	}

	if err := svc.ReserveDisasterRecoveryToken(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE jti = ?`, req.JTI).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("reserve did not consume token: status=%s", status)
	}
	if got := auditEventCount(t, db, u.ID, "disaster_recovery_started"); got != 1 {
		t.Fatalf("reserve audit count = %d, want 1", got)
	}
	if got := auditEventCount(t, db, u.ID, "disaster_recovery_executed"); got != 0 {
		t.Fatalf("reserve wrote execution audit event: got %d", got)
	}
	if err := svc.ReserveDisasterRecoveryToken(ctx, req); !errors.Is(err, service.ErrAlreadyUsed) {
		t.Fatalf("second reserve error = %v, want ErrAlreadyUsed", err)
	}

	svc.RecordDisasterRecoveryExecuted(ctx, req.UserID)
	if got := auditEventCount(t, db, u.ID, "disaster_recovery_executed"); got != 1 {
		t.Fatalf("execution audit count = %d, want 1", got)
	}
}

func TestDisasterRecoveryExecutionAuditDoesNotDependOnActionRow(t *testing.T) {
	svc, _, _, db, u := setupFlowAuth(t)
	ctx := context.Background()

	token, err := svc.RequestDisasterRecovery(ctx, u.ID, "Password123")
	if err != nil {
		t.Fatal(err)
	}
	req, err := svc.VerifyDisasterRecoveryToken(ctx, u.ID, token)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ReserveDisasterRecoveryToken(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM action_requests WHERE jti = ?`, req.JTI); err != nil {
		t.Fatal(err)
	}

	svc.RecordDisasterRecoveryExecuted(ctx, req.UserID)
	if got := auditEventCount(t, db, u.ID, "disaster_recovery_executed"); got != 1 {
		t.Fatalf("execution audit count = %d, want 1", got)
	}
	if _, err := svc.VerifyDisasterRecoveryToken(ctx, u.ID, token); !errors.Is(err, service.ErrInvalidToken) {
		t.Fatalf("verify after replacement error = %v, want ErrInvalidToken", err)
	}
	if err := svc.ReserveDisasterRecoveryToken(ctx, req); !errors.Is(err, service.ErrInvalidToken) {
		t.Fatalf("reserve after replacement error = %v, want ErrInvalidToken", err)
	}
}

func auditEventCount(t *testing.T, db *sql.DB, userID, eventType string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(context.Background(), `
		SELECT COUNT(1)
		FROM audit_log
		WHERE user_id = ?
		  AND table_name = 'security_events'
		  AND new_data LIKE ?`, userID, `%"event_type":"`+eventType+`"%`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
