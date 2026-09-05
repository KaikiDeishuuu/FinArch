package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"
	"finarch/internal/infrastructure/auth"
	"finarch/internal/infrastructure/email"

	"github.com/google/uuid"
)

// AuthService handles user registration and login.
type AuthService struct {
	users        repository.UserRepository
	actionTokens *auth.ActionTokenService
	tracker      *auth.LoginAttemptTracker
	emailSvc     email.Sender
	emailReq     bool   // whether email verification is required
	appBaseURL   string // used to build verification links
	txManager    repository.TransactionManager
	sessions     *SessionService
	dataCleaner  UserDataCleaner
}

const (
	passwordResetActionTTL = time.Hour
	emailChangeActionTTL   = time.Hour
)

var credentialBoundActions = [...]string{
	ActionPasswordReset,
	ActionEmailChangeOld,
	ActionEmailChangeNew,
	ActionAccountDelete,
	ActionBackupExport,
	ActionDisasterRecovery,
}

// expireCredentialBoundActions invalidates every pending authorization whose
// legitimacy depends on the user's current password or email ownership. It is
// called in the same SQL transaction as a credential mutation, preventing a
// link issued under old credentials from changing or deleting the account.
func (s *AuthService) expireCredentialBoundActions(ctx context.Context, userID string) error {
	for _, action := range credentialBoundActions {
		if err := s.users.ExpirePendingActionRequestsForUser(ctx, userID, action); err != nil {
			return err
		}
	}
	return nil
}

func sameCredentialState(observed, current model.User) bool {
	return observed.ID != "" && observed.ID == current.ID &&
		observed.Email == current.Email &&
		observed.PasswordHash == current.PasswordHash &&
		observed.PwdVersion == current.PwdVersion &&
		observed.EmailVerified == current.EmailVerified
}

// UserDataCleaner durably queues user-owned external bytes for deletion and can
// make a best-effort cleanup pass. EnqueueUserDataDeletion must participate in
// the SQL transaction carried by ctx.
type UserDataCleaner interface {
	EnqueueUserDataDeletion(ctx context.Context, userID string) error
	DrainDeletionQueue(ctx context.Context, limit int) error
}

func NewAuthService(
	users repository.UserRepository,
	actionTokens *auth.ActionTokenService,
	tracker *auth.LoginAttemptTracker,
	emailSvc email.Sender,
	emailRequired bool,
	appBaseURL string,
	txManager repository.TransactionManager,
	sessions *SessionService,
) *AuthService {
	if sessions == nil {
		panic("AuthService requires SessionService")
	}
	return &AuthService{
		users: users, actionTokens: actionTokens, tracker: tracker,
		emailSvc: emailSvc, emailReq: emailRequired, appBaseURL: appBaseURL,
		txManager: txManager, sessions: sessions,
	}
}

// EmailVerificationRequired returns true when email verification is enforced.
func (s *AuthService) EmailVerificationRequired() bool { return s.emailReq }

// SetUserDataCleaner configures cleanup for user data stored outside SQL. It is
// intended to be called during application startup, before the server starts.
func (s *AuthService) SetUserDataCleaner(cleaner UserDataCleaner) {
	s.dataCleaner = cleaner
}

// GetUserProfile returns the current user's profile including pending email.
func (s *AuthService) GetUserProfile(ctx context.Context, userID string) (model.User, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return model.User{}, mapUserLookupError(err)
	}
	return u, nil
}

func mapUserLookupError(err error) error {
	if errors.Is(err, repository.ErrUserNotFound) {
		return ErrUserNotFound
	}
	return ErrSystemUnavailable
}

type RegisterRequest struct {
	Email    string
	Username string
	Password string
	Nickname string // optional; randomly generated if empty
}

type LoginResponse struct {
	Token             string    `json:"token"`
	ExpiresAt         time.Time `json:"expires_at"`
	RefreshToken      string    `json:"-"`
	RefreshExpiresAt  time.Time `json:"-"`
	AbsoluteExpiresAt time.Time `json:"-"`
	SessionID         string    `json:"-"`
	UserID            string    `json:"user_id"`
	Email             string    `json:"email"`
	Username          string    `json:"username"`
	Nickname          string    `json:"nickname"`
	Role              string    `json:"role"`
}

// Register creates a new user. If email verification is required, the user starts
// as unverified and a verification email is sent. Returns the created user.
//
// Before inserting, any unverified user with the same email whose registration
// has expired (>24 h) is automatically purged so the email can be reused.
func (s *AuthService) Register(ctx context.Context, req RegisterRequest) (model.User, error) {
	if req.Email == "" || req.Password == "" {
		return model.User{}, fmt.Errorf("邮箱、用户名和密码不能为空")
	}
	username, err := normalizeAndValidateUsername(req.Username)
	if err != nil {
		return model.User{}, err
	}
	req.Username = username
	if len(req.Password) < 8 {
		return model.User{}, fmt.Errorf("密码至少需要 8 位")
	}

	// Purge expired unverified occupants so the email/username can be reused.
	cutoff := time.Now().Add(-24 * time.Hour)
	if existing, err := s.users.GetByEmail(ctx, req.Email); err == nil {
		if !existing.EmailVerified && existing.CreatedAt.Before(cutoff) {
			_ = s.users.DeleteUser(ctx, existing.ID)
		}
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return model.User{}, fmt.Errorf("注册失败，请稍后重试")
	}
	now := time.Now()
	nickname := req.Nickname
	if nickname == "" {
		nickname = generateProfessionalNickname(req.Email+":"+req.Username, 0)
	}
	u := model.User{
		ID:            uuid.NewString(),
		Email:         req.Email,
		Username:      req.Username,
		Nickname:      nickname,
		Name:          req.Username,
		PasswordHash:  hash,
		Role:          "owner",
		EmailVerified: !s.emailReq,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.users.Create(ctx, u); err != nil {
		if err.Error() == "username_taken" {
			return model.User{}, ErrUsernameTaken
		}
		if err.Error() == "email_taken" {
			return model.User{}, ErrEmailTaken
		}
		return model.User{}, ErrInternal
	}
	if s.emailReq {
		if err := s.sendVerificationEmail(ctx, u); err != nil {
			// Non-fatal: user is created, they can request resend
			_ = err
		}
	}
	return u, nil
}

func (s *AuthService) sendVerificationEmail(ctx context.Context, u model.User) error {
	token, _, err := s.createActionToken(ctx, u.ID, ActionRegisterVerify, "", 24*time.Hour)
	if err != nil {
		return err
	}
	return s.emailSvc.SendVerification(u.Email, u.Username, token)
}

// ResendVerification sends a new verification email if the user exists and is unverified.
func (s *AuthService) ResendVerification(ctx context.Context, email string) error {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil // don't expose whether email exists
	}
	if u.EmailVerified {
		return nil // already verified, silently succeed
	}
	return s.sendVerificationEmail(ctx, u)
}

// VerifyEmail marks the user's email as verified after checking the token.
func (s *AuthService) VerifyEmail(ctx context.Context, token string) error {
	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		req, err := s.verifyAndLoadAction(txCtx, token, ActionRegisterVerify)
		if err != nil {
			return err
		}
		if err := s.users.SetEmailVerified(txCtx, req.UserID); err != nil {
			return fmt.Errorf("验证失败，请重试")
		}
		if _, err := s.users.ConsumeActionRequest(txCtx, req.JTI, time.Now()); err != nil && !errors.Is(err, auth.ErrTokenAlreadyUsed) {
			return fmt.Errorf("验证失败，请重试")
		}
		_ = s.users.CreateAuditEvent(txCtx, req.UserID, "registration_verified", "", "")
		return nil
	})
}

// ForgotPassword sends a password reset email if the account exists and is verified.
func (s *AuthService) ForgotPassword(ctx context.Context, emailAddr string) error {
	u, err := s.users.GetByEmail(ctx, emailAddr)
	if err != nil || !u.EmailVerified {
		return nil
	}
	var token string
	err = s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.users.GetByID(txCtx, u.ID)
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		// If a password or email mutation linearized after the public lookup,
		// that mutation either expires an action issued first or wins here and
		// prevents a token based on the stale credential state from being issued.
		if !current.EmailVerified || !sameCredentialState(u, current) {
			return nil
		}
		token, _, err = s.createActionToken(txCtx, current.ID, ActionPasswordReset, "", passwordResetActionTTL)
		if err == nil {
			u = current
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("操作失败，请稍后重试")
	}
	if token == "" {
		return nil
	}
	return s.emailSvc.SendPasswordReset(u.Email, u.Username, token)
}

// RequestEmailChange sends an authorization link to the CURRENT (old) email address.
// The user must click it to confirm they initiated the change; only then will a
// verification link be sent to the new address.
func (s *AuthService) RequestEmailChange(ctx context.Context, userID, currentPassword, newEmail string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("用户不存在")
	}
	if strings.EqualFold(u.Email, newEmail) {
		return fmt.Errorf("新邮箱与当前邮箱相同")
	}
	if err := auth.CheckPassword(u.PasswordHash, currentPassword); err != nil {
		return ErrInvalidPassword
	}

	var token string
	err = s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.users.GetByID(txCtx, userID)
		if err != nil {
			return fmt.Errorf("用户不存在")
		}
		// Do not authorize a new flow with credentials that changed after the
		// password check above.
		if !sameCredentialState(u, current) {
			return ErrInvalidPassword
		}
		if strings.EqualFold(current.Email, newEmail) {
			return fmt.Errorf("新邮箱与当前邮箱相同")
		}
		if _, err := s.users.GetByEmail(txCtx, newEmail); err == nil {
			return ErrEmailTaken
		} else if !errors.Is(err, repository.ErrUserNotFound) {
			return fmt.Errorf("操作失败，请稍后重试")
		}

		// Replacing pending_email and both halves of the action-token flow in
		// one transaction prevents an older link from surviving a new request.
		if err := s.users.ExpirePendingActionRequestsForUser(txCtx, current.ID, ActionEmailChangeOld); err != nil {
			return fmt.Errorf("操作失败，请稍后重试")
		}
		if err := s.users.ExpirePendingActionRequestsForUser(txCtx, current.ID, ActionEmailChangeNew); err != nil {
			return fmt.Errorf("操作失败，请稍后重试")
		}
		if err := s.users.SetPendingEmail(txCtx, current.ID, newEmail); err != nil {
			return fmt.Errorf("操作失败，请稍后重试")
		}
		token, _, err = s.createActionToken(txCtx, current.ID, ActionEmailChangeOld, newEmail, emailChangeActionTTL)
		if err != nil {
			return fmt.Errorf("操作失败，请稍后重试")
		}
		u = current
		return nil
	})
	if err != nil {
		return err
	}
	// Step 1: send authorization request to the CURRENT email.
	if err := s.emailSvc.SendEmailChangeOldVerify(u.Email, u.Username, newEmail, token); err != nil {
		return fmt.Errorf("邮件发送失败，请稍后重试")
	}
	return nil
}

// ConfirmOldEmailForChange is called when the user clicks the authorization link
// sent to their OLD email. It issues a verification link to the NEW email.
func (s *AuthService) ConfirmOldEmailForChange(ctx context.Context, token string) error {
	var newEmail string
	var u model.User
	var newToken string
	err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		req, err := s.verifyAndLoadAction(txCtx, token, ActionEmailChangeOld)
		if err != nil {
			return err
		}
		newEmail = req.Meta
		if newEmail == "" {
			return ErrInvalidToken
		}
		u, err = s.users.GetByID(txCtx, req.UserID)
		if err != nil {
			return ErrUserNotFound
		}
		if u.PendingEmail == "" || u.PendingEmail != newEmail {
			return ErrInvalidToken
		}
		if existing, err := s.users.GetByEmail(txCtx, newEmail); err == nil && existing.ID != u.ID {
			return ErrResourceConflict
		} else if err != nil && !errors.Is(err, repository.ErrUserNotFound) {
			return fmt.Errorf("操作失败，请稍后重试")
		}
		if _, err := s.users.ConsumeActionRequest(txCtx, req.JTI, time.Now()); err != nil {
			if errors.Is(err, auth.ErrTokenAlreadyUsed) {
				return ErrAlreadyUsed
			}
			return fmt.Errorf("操作失败，请稍后重试")
		}
		if err := s.users.ExpirePendingActionRequestsForUser(txCtx, req.UserID, ActionEmailChangeOld); err != nil {
			return fmt.Errorf("操作失败，请稍后重试")
		}
		if err := s.users.ExpirePendingActionRequestsForUser(txCtx, req.UserID, ActionEmailChangeNew); err != nil {
			return fmt.Errorf("操作失败，请稍后重试")
		}
		newToken, _, err = s.createActionToken(txCtx, u.ID, ActionEmailChangeNew, newEmail, emailChangeActionTTL)
		if err != nil {
			return fmt.Errorf("操作失败，请稍后重试")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := s.emailSvc.SendEmailChange(newEmail, u.Username, newToken); err != nil {
		return fmt.Errorf("邮件发送失败，请稍后重试")
	}
	return nil
}

// ConfirmEmailChange validates the token and applies the email change.
func (s *AuthService) ConfirmEmailChange(ctx context.Context, token string) error {
	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		req, err := s.verifyAndLoadAction(txCtx, token, ActionEmailChangeNew)
		if err != nil {
			return err
		}
		if req.Meta == "" {
			return ErrInvalidToken
		}
		u, err := s.users.GetByID(txCtx, req.UserID)
		if err != nil {
			return ErrUserNotFound
		}
		if u.PendingEmail == "" || u.PendingEmail != req.Meta {
			return ErrInvalidToken
		}
		if _, err := s.users.ConsumeActionRequest(txCtx, req.JTI, time.Now()); err != nil {
			if errors.Is(err, auth.ErrTokenAlreadyUsed) {
				return ErrAlreadyUsed
			}
			return fmt.Errorf("邮箱更新失败，请稍后重试")
		}
		if err := s.expireCredentialBoundActions(txCtx, req.UserID); err != nil {
			return fmt.Errorf("邮箱更新失败，请稍后重试")
		}
		if err := s.users.UpdateEmail(txCtx, req.UserID, req.Meta); err != nil {
			if err.Error() == "email_taken" {
				return ErrEmailTaken
			}
			return fmt.Errorf("邮箱更新失败，请稍后重试")
		}
		if err := s.sessions.RevokeAllForUser(txCtx, req.UserID); err != nil {
			return fmt.Errorf("邮箱更新失败，请稍后重试")
		}
		_ = s.users.CreateAuditEvent(txCtx, req.UserID, "email_changed", "", "")
		return nil
	})
}

// ResetPassword resets the user's password using a valid reset token.
func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("新密码至少需要 8 位")
	}
	// Reject forged, expired, or already-consumed requests before running the
	// deliberately expensive password hash. The request is verified again in
	// the transaction below so this optimistic check cannot authorize a reset
	// that loses a race with another credential mutation.
	if _, err := s.verifyAndLoadAction(ctx, token, ActionPasswordReset); err != nil {
		return err
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("密码设置失败，请稍后重试")
	}
	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		req, err := s.verifyAndLoadAction(txCtx, token, ActionPasswordReset)
		if err != nil {
			return err
		}
		// Claim the winning reset before mutating credentials. Expiring all
		// remaining reset requests in this transaction makes multiple valid
		// links a single-winner flow.
		if _, err := s.users.ConsumeActionRequest(txCtx, req.JTI, time.Now()); err != nil {
			if errors.Is(err, auth.ErrTokenAlreadyUsed) {
				return ErrAlreadyUsed
			}
			return fmt.Errorf("密码重置失败，请稍后重试")
		}
		if err := s.expireCredentialBoundActions(txCtx, req.UserID); err != nil {
			return fmt.Errorf("密码重置失败，请稍后重试")
		}
		if err := s.users.SetPendingEmail(txCtx, req.UserID, ""); err != nil {
			return fmt.Errorf("密码重置失败，请稍后重试")
		}
		if err := s.users.UpdatePassword(txCtx, req.UserID, hash); err != nil {
			return fmt.Errorf("密码重置失败，请稍后重试")
		}
		if err := s.sessions.RevokeAllForUser(txCtx, req.UserID); err != nil {
			return fmt.Errorf("密码重置失败，请稍后重试")
		}
		_ = s.users.CreateAuditEvent(txCtx, req.UserID, "password_reset", "", "")
		return nil
	})
}

// ChangePassword verifies currentPassword then replaces it with newPassword.
func (s *AuthService) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("新密码至少需要 8 位")
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return mapUserLookupError(err)
	}
	if err := auth.CheckPassword(u.PasswordHash, currentPassword); err != nil {
		return ErrInvalidPassword
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("密码设置失败，请稍后重试")
	}
	err = s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.users.GetByID(txCtx, userID)
		if err != nil {
			return mapUserLookupError(err)
		}
		if current.PasswordHash != u.PasswordHash {
			return ErrInvalidPassword
		}
		if err := s.users.UpdatePassword(txCtx, userID, hash); err != nil {
			return ErrSystemUnavailable
		}
		if err := s.expireCredentialBoundActions(txCtx, userID); err != nil {
			return ErrSystemUnavailable
		}
		if err := s.users.SetPendingEmail(txCtx, userID, ""); err != nil {
			return ErrSystemUnavailable
		}
		if err := s.sessions.RevokeAllForUser(txCtx, userID); err != nil {
			return ErrSystemUnavailable
		}
		_ = s.users.CreateAuditEvent(txCtx, userID, "password_changed", "", "")
		return nil
	})
	if err == nil || errors.Is(err, ErrInvalidPassword) || errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrSystemUnavailable) {
		return err
	}
	return ErrSystemUnavailable
}

// RequestBackupExport requires re-auth and returns a short-lived export authorization token.
func (s *AuthService) RequestBackupExport(ctx context.Context, userID, currentPassword string) (string, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", ErrUserNotFound
	}
	if err := auth.CheckPassword(u.PasswordHash, currentPassword); err != nil {
		return "", ErrInvalidPassword
	}
	var token string
	err = s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.users.GetByID(txCtx, userID)
		if err != nil {
			return mapUserLookupError(err)
		}
		if !sameCredentialState(u, current) {
			return ErrInvalidPassword
		}
		token, _, err = s.createActionToken(txCtx, current.ID, ActionBackupExport, "", 10*time.Minute)
		if err == nil {
			u = current
		}
		return err
	})
	if err != nil {
		if errors.Is(err, ErrInvalidPassword) || errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrSystemUnavailable) {
			return "", err
		}
		return "", fmt.Errorf("操作失败，请稍后重试")
	}
	_ = s.users.CreateAuditEvent(ctx, u.ID, "backup_export_requested", "", "")
	return token, nil
}

// ConsumeBackupExportToken validates + consumes backup export authorization.
func (s *AuthService) ConsumeBackupExportToken(ctx context.Context, userID, token string) error {
	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		req, err := s.verifyAndLoadAction(txCtx, token, ActionBackupExport)
		if err != nil {
			return err
		}
		if req.UserID != userID {
			return ErrNotAuthorized
		}
		if _, err := s.users.ConsumeActionRequest(txCtx, req.JTI, time.Now()); err != nil {
			if errors.Is(err, auth.ErrTokenAlreadyUsed) {
				return ErrAlreadyUsed
			}
			return fmt.Errorf("操作失败，请稍后重试")
		}
		_ = s.users.CreateAuditEvent(txCtx, userID, "backup_export_downloaded", "", "")
		return nil
	})
}

func (s *AuthService) RequestDisasterRecovery(ctx context.Context, userID, currentPassword string) (string, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", ErrUserNotFound
	}
	if err := auth.CheckPassword(u.PasswordHash, currentPassword); err != nil {
		return "", ErrInvalidPassword
	}
	var token string
	err = s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.users.GetByID(txCtx, userID)
		if err != nil {
			return mapUserLookupError(err)
		}
		if !sameCredentialState(u, current) {
			return ErrInvalidPassword
		}
		token, _, err = s.createActionToken(txCtx, current.ID, ActionDisasterRecovery, "", 5*time.Minute)
		if err == nil {
			u = current
		}
		return err
	})
	if err != nil {
		if errors.Is(err, ErrInvalidPassword) || errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrSystemUnavailable) {
			return "", err
		}
		return "", fmt.Errorf("操作失败，请稍后重试")
	}
	_ = s.users.CreateAuditEvent(ctx, u.ID, "disaster_recovery_authorized", "", "")
	return token, nil
}

func (s *AuthService) VerifyDisasterRecoveryToken(ctx context.Context, userID, token string) (model.ActionRequest, error) {
	req, err := s.verifyAndLoadAction(ctx, token, ActionDisasterRecovery)
	if err != nil {
		return model.ActionRequest{}, err
	}
	if req.UserID != userID {
		return model.ActionRequest{}, ErrNotAuthorized
	}
	return req, nil
}

// ReserveDisasterRecoveryToken strictly consumes a verified one-time token and
// records that recovery started in the same transaction. It must run before
// any restore mutation so failed attempts also require fresh authorization.
func (s *AuthService) ReserveDisasterRecoveryToken(ctx context.Context, req model.ActionRequest) error {
	if req.JTI == "" || req.UserID == "" || req.Action != ActionDisasterRecovery {
		return ErrInvalidToken
	}
	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		stored, err := s.users.GetActionRequestByJTI(txCtx, req.JTI)
		if err != nil || stored.UserID != req.UserID || stored.Action != ActionDisasterRecovery {
			return ErrInvalidToken
		}
		switch stored.Status {
		case "completed":
			return ErrAlreadyUsed
		case "pending":
			// Continue below.
		case "expired":
			return ErrExpiredToken
		default:
			return ErrInvalidToken
		}
		now := time.Now()
		if !stored.ExpiresAt.After(now) {
			_ = s.users.ExpireActionRequests(txCtx, ActionDisasterRecovery, now)
			return ErrExpiredToken
		}
		if _, err := s.users.ConsumeActionRequest(txCtx, req.JTI, now); err != nil {
			if errors.Is(err, auth.ErrTokenAlreadyUsed) {
				return ErrAlreadyUsed
			}
			return fmt.Errorf("操作失败，请稍后重试")
		}
		if err := s.users.CreateAuditEvent(txCtx, req.UserID, "disaster_recovery_started", "", ""); err != nil {
			return fmt.Errorf("操作失败，请稍后重试")
		}
		return nil
	})
}

// RecordDisasterRecoveryExecuted is deliberately best-effort: a physical
// replacement may no longer contain the action request or even the same user.
func (s *AuthService) RecordDisasterRecoveryExecuted(ctx context.Context, userID string) {
	_ = s.users.CreateAuditEvent(ctx, userID, "disaster_recovery_executed", "", "")
}

func (s *AuthService) CompleteDisasterRecoveryToken(ctx context.Context, req model.ActionRequest, strict bool) error {
	if req.JTI == "" || req.UserID == "" || req.Action != ActionDisasterRecovery {
		return ErrInvalidToken
	}
	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := s.users.ConsumeActionRequest(txCtx, req.JTI, time.Now()); err != nil {
			if strict {
				if errors.Is(err, auth.ErrTokenAlreadyUsed) {
					return ErrAlreadyUsed
				}
				return fmt.Errorf("操作失败，请稍后重试")
			}
			if !errors.Is(err, auth.ErrTokenAlreadyUsed) {
				return fmt.Errorf("操作失败，请稍后重试")
			}
		}
		_ = s.users.CreateAuditEvent(txCtx, req.UserID, "disaster_recovery_executed", "", "")
		return nil
	})
}

func (s *AuthService) ConsumeDisasterRecoveryToken(ctx context.Context, userID, token string) error {
	req, err := s.VerifyDisasterRecoveryToken(ctx, userID, token)
	if err != nil {
		return err
	}
	return s.CompleteDisasterRecoveryToken(ctx, req, true)
}

// RequestAccountDeletion sends an account-deletion confirmation email.
func (s *AuthService) RequestAccountDeletion(ctx context.Context, userID string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("用户不存在")
	}
	var token string
	err = s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.users.GetByID(txCtx, userID)
		if err != nil {
			return mapUserLookupError(err)
		}
		if !sameCredentialState(u, current) {
			return ErrSessionInvalid
		}
		if err := s.users.ExpirePendingActionRequestsForUser(txCtx, current.ID, ActionAccountDelete); err != nil {
			return err
		}
		var issueErr error
		token, _, issueErr = s.createActionToken(txCtx, current.ID, ActionAccountDelete, "", 30*time.Minute)
		if issueErr == nil {
			u = current
		}
		return issueErr
	})
	if err != nil {
		if errors.Is(err, ErrSessionInvalid) || errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrSystemUnavailable) {
			return err
		}
		return fmt.Errorf("操作失败，请稍后重试")
	}
	if err := s.emailSvc.SendAccountDeletion(u.Email, u.Username, token); err != nil {
		return fmt.Errorf("邮件发送失败，请稍后重试")
	}
	return nil
}

// ConfirmAccountDeletion validates one-time token and permanently deletes account data.
func (s *AuthService) ConfirmAccountDeletion(ctx context.Context, token string) error {
	verified, err := s.verifyAndLoadAction(ctx, token, ActionAccountDelete)
	if err != nil {
		return err
	}
	err = s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		req, err := s.verifyAndLoadAction(txCtx, token, ActionAccountDelete)
		if err != nil {
			return err
		}
		if req.JTI != verified.JTI || req.UserID != verified.UserID {
			return ErrInvalidToken
		}
		if _, err := s.users.ConsumeActionRequest(txCtx, req.JTI, time.Now()); err != nil {
			if errors.Is(err, auth.ErrTokenAlreadyUsed) {
				return ErrAlreadyUsed
			}
			return fmt.Errorf("确认失败，请稍后重试")
		}
		if s.dataCleaner != nil {
			if err := s.dataCleaner.EnqueueUserDataDeletion(txCtx, req.UserID); err != nil {
				return fmt.Errorf("账户文件清理排队失败，请稍后重试")
			}
		}
		if err := s.users.DeleteUser(txCtx, req.UserID); err != nil {
			if errors.Is(err, repository.ErrUserNotFound) {
				return ErrUserNotFound
			}
			return ErrInternal
		}
		_ = s.users.CreateAuditEvent(txCtx, req.UserID, "account_deleted", "", "")
		return nil
	})
	if err != nil {
		return err
	}
	if s.dataCleaner != nil {
		_ = s.dataCleaner.DrainDeletionQueue(ctx, DefaultAttachmentDeletionBatchSize)
	}
	return nil
}

func (s *AuthService) createActionToken(ctx context.Context, userID, action, meta string, ttl time.Duration) (string, string, error) {
	token, jti, exp, err := s.actionTokens.Issue(userID, action, meta, ttl)
	if err != nil {
		return "", "", err
	}
	if err := s.users.CreateActionRequest(ctx, model.ActionRequest{JTI: jti, UserID: userID, Action: action, Status: "pending", Meta: meta, ExpiresAt: exp, CreatedAt: time.Now()}); err != nil {
		return "", "", err
	}
	return token, jti, nil
}

func (s *AuthService) verifyAndLoadAction(ctx context.Context, token, action string) (model.ActionRequest, error) {
	claims, err := s.actionTokens.Verify(token, action)
	if err != nil {
		if errors.Is(err, auth.ErrActionTokenExpired) {
			return model.ActionRequest{}, ErrExpiredToken
		}
		return model.ActionRequest{}, ErrInvalidToken
	}
	req, err := s.users.GetActionRequestByJTI(ctx, claims.ID)
	if err != nil {
		return model.ActionRequest{}, ErrInvalidToken
	}
	if req.Action != action || req.UserID != claims.UserID || req.Meta != claims.Meta {
		return model.ActionRequest{}, ErrInvalidToken
	}
	switch req.Status {
	case "pending":
		// Continue below.
	case "completed":
		return model.ActionRequest{}, ErrAlreadyUsed
	case "expired":
		return model.ActionRequest{}, ErrExpiredToken
	default:
		return model.ActionRequest{}, ErrInvalidToken
	}
	if time.Now().After(req.ExpiresAt) {
		_ = s.users.ExpireActionRequests(ctx, action, time.Now())
		return model.ActionRequest{}, ErrExpiredToken
	}
	return req, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (LoginResponse, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	// Check account lockout before any DB access to prevent timing-based enumeration.
	if s.tracker.IsLocked(email) {
		return LoginResponse{}, ErrAccountLocked
	}
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			s.tracker.RecordFailure(email)
			return LoginResponse{}, ErrInvalidCredentials
		}
		return LoginResponse{}, ErrSystemUnavailable
	}
	if err := auth.CheckPassword(u.PasswordHash, password); err != nil {
		s.tracker.RecordFailure(email)
		return LoginResponse{}, ErrInvalidCredentials
	}
	if !u.EmailVerified {
		return LoginResponse{}, ErrEmailNotVerified
	}
	// Successful login — clear failure counter.
	s.tracker.RecordSuccess(email)
	session, err := s.sessions.CreateSession(ctx, u)
	if err != nil {
		return LoginResponse{}, ErrSystemUnavailable
	}
	return loginResponseFromSession(session), nil
}

// RefreshSession atomically rotates an opaque refresh bearer and returns the
// same successor during the short response-recovery grace window.
func (s *AuthService) RefreshSession(ctx context.Context, rawRefresh string) (LoginResponse, error) {
	session, err := s.sessions.RotateSession(ctx, rawRefresh)
	if err != nil {
		return LoginResponse{}, err
	}
	return loginResponseFromSession(session), nil
}

// RevokeSession idempotently logs out the refresh-token family, including when
// the provided generation has already been consumed.
func (s *AuthService) RevokeSession(ctx context.Context, rawRefresh string) error {
	return s.sessions.RevokeSession(ctx, rawRefresh)
}

// ValidateAccessSession checks that an access JWT's mandatory sid still names
// an active session bound to the same user and credential version.
func (s *AuthService) ValidateAccessSession(ctx context.Context, claims *auth.Claims) error {
	return s.sessions.ValidateAccessSession(ctx, claims)
}

// AuthenticateAccess verifies the access bearer and validates its authoritative
// server-side session through the same SessionService that issued it. Keeping
// both operations behind one dependency prevents mismatched JWT keys at the
// HTTP boundary.
func (s *AuthService) AuthenticateAccess(ctx context.Context, rawAccess string) (*auth.Claims, error) {
	return s.sessions.AuthenticateAccess(ctx, rawAccess)
}

func loginResponseFromSession(session SessionTokens) LoginResponse {
	return LoginResponse{
		Token: session.AccessToken, ExpiresAt: session.AccessExpiresAt,
		RefreshToken: session.RefreshToken, RefreshExpiresAt: session.RefreshExpiresAt,
		AbsoluteExpiresAt: session.AbsoluteExpiresAt, SessionID: session.SessionID,
		UserID: session.UserID, Email: session.Email, Username: session.Username,
		Nickname: session.Nickname, Role: session.Role,
	}
}

// UpdateNickname changes the user's display nickname.
func (s *AuthService) UpdateNickname(ctx context.Context, userID, nickname string) error {
	if nickname == "" {
		return fmt.Errorf("昵称不能为空")
	}
	if len([]rune(nickname)) > 20 {
		return fmt.Errorf("昵称最长 20 个字符")
	}
	if err := s.users.UpdateNickname(ctx, userID, nickname); err != nil {
		return fmt.Errorf("昵称更新失败，请稍后重试")
	}
	return nil
}

// CleanupExpiredUnverified removes all unverified users whose registration is
// older than 24 hours. It returns the number of deleted users.
func (s *AuthService) CleanupExpiredUnverified(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-24 * time.Hour)
	return s.users.DeleteExpiredUnverifiedUsers(ctx, cutoff)
}
