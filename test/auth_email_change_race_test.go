package test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"finarch/internal/domain/model"
	domainrepo "finarch/internal/domain/repository"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	sqliterepo "finarch/internal/infrastructure/repository"
)

// Race instrumentation plus concurrent package tests can make password
// hashing take several seconds on a loaded CI worker. This timeout protects
// against a true synchronization hang without treating normal CPU contention
// as a functional failure.
const credentialRaceSignalTimeout = 10 * time.Second

type blockingCreateSessionRepository struct {
	domainrepo.RefreshTokenRepository
	createEntered chan struct{}
	allowCreate   chan struct{}
}

type pausingCredentialReadUserRepository struct {
	domainrepo.UserRepository
	pauseGetByEmail bool
	pauseGetByID    bool
	readEntered     chan struct{}
	allowReturn     chan struct{}
	pauseOnce       sync.Once
}

func (r *pausingCredentialReadUserRepository) pauseFirstRead(ctx context.Context) error {
	paused := false
	r.pauseOnce.Do(func() {
		paused = true
		close(r.readEntered)
	})
	if !paused {
		return nil
	}
	select {
	case <-r.allowReturn:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *pausingCredentialReadUserRepository) GetByEmail(ctx context.Context, email string) (model.User, error) {
	user, err := r.UserRepository.GetByEmail(ctx, email)
	if err == nil && r.pauseGetByEmail {
		if pauseErr := r.pauseFirstRead(ctx); pauseErr != nil {
			return model.User{}, pauseErr
		}
	}
	return user, err
}

func (r *pausingCredentialReadUserRepository) GetByID(ctx context.Context, id string) (model.User, error) {
	user, err := r.UserRepository.GetByID(ctx, id)
	if err == nil && r.pauseGetByID {
		if pauseErr := r.pauseFirstRead(ctx); pauseErr != nil {
			return model.User{}, pauseErr
		}
	}
	return user, err
}

func (r *blockingCreateSessionRepository) CreateSession(ctx context.Context, session model.AuthSession, token model.RefreshToken) error {
	close(r.createEntered)
	select {
	case <-r.allowCreate:
	case <-ctx.Done():
		return ctx.Err()
	}
	return r.RefreshTokenRepository.CreateSession(ctx, session, token)
}

func TestEmailChangeRacingLoginCannotCreatePostRevocationSession(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	oldEmail := "email-race-old@example.com"
	newEmail := "email-race-new@example.com"
	password := "Password123"
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE users
		SET email = ?, password_hash = ?, email_verified = 1
		WHERE id = ?`, oldEmail, passwordHash, testUserID); err != nil {
		t.Fatal(err)
	}

	createEntered := make(chan struct{})
	allowCreate := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(allowCreate)
		}
	}()
	refreshRepository := &blockingCreateSessionRepository{
		RefreshTokenRepository: sqliterepo.NewSQLiteRefreshTokenRepository(database),
		createEntered:          createEntered,
		allowCreate:            allowCreate,
	}
	jwt := auth.NewJWTService("test-secret")
	sessions, err := service.NewSessionService(refreshRepository, jwt, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	userRepository := sqliterepo.NewSQLiteUserRepository(database)
	sender := &flowSender{}
	authService := service.NewAuthService(
		userRepository,
		auth.NewActionTokenService("test-secret"),
		auth.NewLoginAttemptTracker(5, time.Minute),
		sender,
		true,
		"http://localhost",
		sqliterepo.NewSQLiteTransactionManager(database),
		sessions,
	)

	if err := authService.RequestEmailChange(ctx, testUserID, password, newEmail); err != nil {
		t.Fatal(err)
	}
	if err := authService.ConfirmOldEmailForChange(ctx, sender.emailOldToken); err != nil {
		t.Fatal(err)
	}

	loginResult := make(chan error, 1)
	go func() {
		_, err := authService.Login(ctx, oldEmail, password)
		loginResult <- err
	}()
	select {
	case <-createEntered:
	case <-time.After(credentialRaceSignalTimeout):
		t.Fatal("login did not reach the paused session creation")
	}

	if err := authService.ConfirmEmailChange(ctx, sender.emailNewToken); err != nil {
		t.Fatalf("confirm email change: %v", err)
	}
	close(allowCreate)
	released = true
	if err := <-loginResult; err == nil {
		t.Fatal("login authenticated with the old email created a session after email change")
	}

	current, err := userRepository.GetByID(ctx, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Email != newEmail || current.PwdVersion != 1 {
		t.Fatalf("credential epoch after email change = email:%q version:%d, want %q/1", current.Email, current.PwdVersion, newEmail)
	}
	var activeSessions int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM auth_sessions
		WHERE user_id = ? AND revoked_at IS NULL`, testUserID).Scan(&activeSessions); err != nil {
		t.Fatal(err)
	}
	if activeSessions != 0 {
		t.Fatalf("email-change race left %d active session(s)", activeSessions)
	}
}

func TestCredentialBoundActionIssuanceDoesNotCrossCredentialMutation(t *testing.T) {
	for _, testCase := range []struct {
		name            string
		action          string
		pauseGetByEmail bool
		changeEmail     bool
		wantErr         error
	}{
		{
			name:            "password reset after email change",
			action:          service.ActionPasswordReset,
			pauseGetByEmail: true,
			changeEmail:     true,
		},
		{
			name:        "account deletion after email change",
			action:      service.ActionAccountDelete,
			changeEmail: true,
			wantErr:     service.ErrSessionInvalid,
		},
		{
			name:        "email change request after email change",
			action:      service.ActionEmailChangeOld,
			changeEmail: true,
			wantErr:     service.ErrInvalidPassword,
		},
		{
			name:    "backup export after password change",
			action:  service.ActionBackupExport,
			wantErr: service.ErrInvalidPassword,
		},
		{
			name:    "disaster recovery after password change",
			action:  service.ActionDisasterRecovery,
			wantErr: service.ErrInvalidPassword,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			database := setupDB(t)
			defer database.Close()
			ctx := context.Background()
			oldEmail := "action-race-old@example.com"
			password := "Password123"
			passwordHash, err := auth.HashPassword(password)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := database.ExecContext(ctx, `
				UPDATE users
				SET email = ?, password_hash = ?, email_verified = 1
				WHERE id = ?`, oldEmail, passwordHash, testUserID); err != nil {
				t.Fatal(err)
			}

			baseUsers := sqliterepo.NewSQLiteUserRepository(database)
			readEntered := make(chan struct{})
			allowReturn := make(chan struct{})
			released := false
			defer func() {
				if !released {
					close(allowReturn)
				}
			}()
			users := &pausingCredentialReadUserRepository{
				UserRepository:  baseUsers,
				pauseGetByEmail: testCase.pauseGetByEmail,
				pauseGetByID:    !testCase.pauseGetByEmail,
				readEntered:     readEntered,
				allowReturn:     allowReturn,
			}
			jwt := auth.NewJWTService("test-secret")
			sender := &flowSender{}
			authService := service.NewAuthService(
				users,
				auth.NewActionTokenService("test-secret"),
				auth.NewLoginAttemptTracker(5, time.Minute),
				sender,
				true,
				"http://localhost",
				sqliterepo.NewSQLiteTransactionManager(database),
				newTestSessionService(t, database, jwt, "test-secret"),
			)

			type issuanceResult struct {
				token string
				err   error
			}
			result := make(chan issuanceResult, 1)
			go func() {
				var token string
				var issueErr error
				switch testCase.action {
				case service.ActionPasswordReset:
					issueErr = authService.ForgotPassword(ctx, oldEmail)
					token = sender.resetToken
				case service.ActionAccountDelete:
					issueErr = authService.RequestAccountDeletion(ctx, testUserID)
					token = sender.deleteToken
				case service.ActionEmailChangeOld:
					issueErr = authService.RequestEmailChange(ctx, testUserID, password, "action-race-target@example.com")
					token = sender.emailOldToken
				case service.ActionBackupExport:
					token, issueErr = authService.RequestBackupExport(ctx, testUserID, password)
				case service.ActionDisasterRecovery:
					token, issueErr = authService.RequestDisasterRecovery(ctx, testUserID, password)
				default:
					issueErr = errors.New("unsupported test action")
				}
				result <- issuanceResult{token: token, err: issueErr}
			}()
			select {
			case <-readEntered:
			case <-time.After(credentialRaceSignalTimeout):
				t.Fatal("action request did not reach the paused credential read")
			}

			if testCase.changeEmail {
				if err := baseUsers.UpdateEmail(ctx, testUserID, "action-race-new@example.com"); err != nil {
					t.Fatal(err)
				}
			} else {
				replacementHash, err := auth.HashPassword("ReplacementPassword123")
				if err != nil {
					t.Fatal(err)
				}
				if err := baseUsers.UpdatePassword(ctx, testUserID, replacementHash); err != nil {
					t.Fatal(err)
				}
			}
			close(allowReturn)
			released = true
			issued := <-result
			if testCase.wantErr == nil {
				if issued.err != nil {
					t.Fatalf("stale public issuance returned error: %v", issued.err)
				}
			} else if !errors.Is(issued.err, testCase.wantErr) {
				t.Fatalf("stale issuance error = %v, want %v", issued.err, testCase.wantErr)
			}
			if issued.token != "" {
				t.Fatal("stale credential state produced an action bearer")
			}
			var actionRows int
			if err := database.QueryRowContext(ctx, `
				SELECT COUNT(*)
				FROM action_requests
				WHERE user_id = ? AND action = ?`, testUserID, testCase.action).Scan(&actionRows); err != nil {
				t.Fatal(err)
			}
			if actionRows != 0 {
				t.Fatalf("stale credential state persisted %d action request(s)", actionRows)
			}
		})
	}
}
