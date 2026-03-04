package service

import (
	"context"
	"errors"
	"fmt"
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
	jwt          *auth.JWTService
	actionTokens *auth.ActionTokenService
	tracker      *auth.LoginAttemptTracker
	emailSvc     email.Sender
	emailReq     bool   // whether email verification is required
	appBaseURL   string // used to build verification links
}

func NewAuthService(
	users repository.UserRepository,
	jwt *auth.JWTService,
	actionTokens *auth.ActionTokenService,
	tracker *auth.LoginAttemptTracker,
	emailSvc email.Sender,
	emailRequired bool,
	appBaseURL string,
) *AuthService {
	return &AuthService{
		users: users, jwt: jwt, actionTokens: actionTokens, tracker: tracker,
		emailSvc: emailSvc, emailReq: emailRequired, appBaseURL: appBaseURL,
	}
}

// EmailVerificationRequired returns true when email verification is enforced.
func (s *AuthService) EmailVerificationRequired() bool { return s.emailReq }

// GetUserProfile returns the current user's profile including pending email.
func (s *AuthService) GetUserProfile(ctx context.Context, userID string) (model.User, error) {
	return s.users.GetByID(ctx, userID)
}

type RegisterRequest struct {
	Email    string
	Username string
	Password string
	Nickname string // optional; randomly generated if empty
}

type LoginResponse struct {
	Token     string
	ExpiresAt time.Time
	UserID    string
	Email     string
	Username  string
	Nickname  string
	Role      string
}

// Register creates a new user. If email verification is required, the user starts
// as unverified and a verification email is sent. Returns the created user.
//
// Before inserting, any unverified user with the same email whose registration
// has expired (>24 h) is automatically purged so the email can be reused.
func (s *AuthService) Register(ctx context.Context, req RegisterRequest) (model.User, error) {
	if req.Email == "" || req.Password == "" || req.Username == "" {
		return model.User{}, fmt.Errorf("邮箱、用户名和密码不能为空")
	}
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
	baseNickname := req.Nickname
	if baseNickname == "" {
		baseNickname = generateProfessionalNickname(req.Email+":"+req.Username, 0)
	}
	var u model.User
	for attempt := 0; attempt < 6; attempt++ {
		nickname := baseNickname
		if req.Nickname == "" {
			nickname = generateProfessionalNickname(req.Email+":"+req.Username, attempt)
		}
		u = model.User{
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
		err = s.users.Create(ctx, u)
		if err == nil {
			break
		}
		if err.Error() == "nickname_taken" && req.Nickname == "" {
			continue
		}
		if err.Error() == "username_taken" || err.Error() == "email_taken" || err.Error() == "nickname_taken" {
			return model.User{}, err
		}
		return model.User{}, fmt.Errorf("注册失败，请稍后重试")
	}
	if err != nil {
		return model.User{}, fmt.Errorf("注册失败，请稍后重试")
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
	req, err := s.verifyAndLoadAction(ctx, token, ActionRegisterVerify)
	if err != nil {
		return err
	}
	if err := s.users.SetEmailVerified(ctx, req.UserID); err != nil {
		return fmt.Errorf("验证失败，请重试")
	}
	if _, err := s.users.ConsumeActionRequest(ctx, req.JTI, time.Now()); err != nil && !errors.Is(err, auth.ErrTokenAlreadyUsed) {
		return fmt.Errorf("验证失败，请重试")
	}
	_ = s.users.CreateAuditEvent(ctx, req.UserID, "registration_verified", "", "")
	return nil
}

// ForgotPassword sends a password reset email if the account exists and is verified.
func (s *AuthService) ForgotPassword(ctx context.Context, emailAddr string) error {
	u, err := s.users.GetByEmail(ctx, emailAddr)
	if err != nil || !u.EmailVerified {
		return nil
	}
	token, _, err := s.createActionToken(ctx, u.ID, ActionPasswordReset, "", 30*time.Minute)
	if err != nil {
		return fmt.Errorf("操作失败，请稍后重试")
	}
	return s.emailSvc.SendPasswordReset(u.Email, u.Username, token)
}

// RequestEmailChange sends an authorization link to the CURRENT (old) email address.
// The user must click it to confirm they initiated the change; only then will a
// verification link be sent to the new address.
func (s *AuthService) RequestEmailChange(ctx context.Context, userID, currentPassword, newEmail string) error {
	// New email must not already be in use.
	if _, err := s.users.GetByEmail(ctx, newEmail); err == nil {
		return fmt.Errorf("该邮箱已被其他账户使用")
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("用户不存在")
	}
	if u.Email == newEmail {
		return fmt.Errorf("新邮箱与当前邮箱相同")
	}
	if err := auth.CheckPassword(u.PasswordHash, currentPassword); err != nil {
		return ErrNotAuthorized
	}
	if err := s.users.SetPendingEmail(ctx, u.ID, newEmail); err != nil {
		return fmt.Errorf("操作失败，请稍后重试")
	}
	// Clear any pre-existing old-verify or new-verify tokens.
	token, _, err := s.createActionToken(ctx, u.ID, ActionEmailChangeOld, newEmail, 30*time.Minute)
	if err != nil {
		return fmt.Errorf("操作失败，请稍后重试")
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
	req, err := s.verifyAndLoadAction(ctx, token, ActionEmailChangeOld)
	if err != nil {
		return err
	}
	newEmail := req.Meta
	if newEmail == "" {
		return ErrInvalidToken
	}
	u, err := s.users.GetByID(ctx, req.UserID)
	if err != nil {
		return ErrUserNotFound
	}
	if _, err := s.users.GetByEmail(ctx, newEmail); err == nil {
		return ErrResourceConflict
	}
	if _, err := s.users.ConsumeActionRequest(ctx, req.JTI, time.Now()); err != nil && !errors.Is(err, auth.ErrTokenAlreadyUsed) {
		return err
	}
	newToken, _, err := s.createActionToken(ctx, u.ID, ActionEmailChangeNew, newEmail, 30*time.Minute)
	if err != nil {
		return fmt.Errorf("操作失败，请稍后重试")
	}
	if err := s.emailSvc.SendEmailChange(newEmail, u.Username, newToken); err != nil {
		return fmt.Errorf("邮件发送失败，请稍后重试")
	}
	return nil
}

// ConfirmEmailChange validates the token and applies the email change.
func (s *AuthService) ConfirmEmailChange(ctx context.Context, token string) error {
	req, err := s.verifyAndLoadAction(ctx, token, ActionEmailChangeNew)
	if err != nil {
		return err
	}
	if req.Meta == "" {
		return ErrInvalidToken
	}
	if err := s.users.UpdateEmail(ctx, req.UserID, req.Meta); err != nil {
		if err.Error() == "email_taken" {
			return ErrResourceConflict
		}
		return fmt.Errorf("邮箱更新失败，请稍后重试")
	}
	if _, err := s.users.ConsumeActionRequest(ctx, req.JTI, time.Now()); err != nil && !errors.Is(err, auth.ErrTokenAlreadyUsed) {
		return fmt.Errorf("邮箱更新失败，请稍后重试")
	}
	_ = s.users.CreateAuditEvent(ctx, req.UserID, "email_changed", "", "")
	return nil
}

// ResetPassword resets the user's password using a valid reset token.
func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("新密码至少需要 8 位")
	}
	req, err := s.verifyAndLoadAction(ctx, token, ActionPasswordReset)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("密码设置失败，请稍后重试")
	}
	if err := s.users.UpdatePassword(ctx, req.UserID, hash); err != nil {
		return fmt.Errorf("密码重置失败，请稍后重试")
	}
	if _, err := s.users.ConsumeActionRequest(ctx, req.JTI, time.Now()); err != nil && !errors.Is(err, auth.ErrTokenAlreadyUsed) {
		return fmt.Errorf("密码重置失败，请稍后重试")
	}
	_ = s.users.CreateAuditEvent(ctx, req.UserID, "password_reset", "", "")
	return nil
}

// ChangePassword verifies currentPassword then replaces it with newPassword.
func (s *AuthService) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("新密码至少需要 8 位")
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("用户不存在")
	}
	if err := auth.CheckPassword(u.PasswordHash, currentPassword); err != nil {
		return fmt.Errorf("当前密码不正确")
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("密码设置失败，请稍后重试")
	}
	if err := s.users.UpdatePassword(ctx, userID, hash); err != nil {
		return fmt.Errorf("密码修改失败，请稍后重试")
	}
	_ = s.users.CreateAuditEvent(ctx, userID, "password_changed", "", "")
	return nil
}

// RequestBackupExport requires re-auth and returns a short-lived export authorization token.
func (s *AuthService) RequestBackupExport(ctx context.Context, userID, currentPassword string) (string, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", ErrUserNotFound
	}
	if err := auth.CheckPassword(u.PasswordHash, currentPassword); err != nil {
		return "", ErrNotAuthorized
	}
	token, _, err := s.createActionToken(ctx, u.ID, ActionBackupExport, "", 10*time.Minute)
	if err != nil {
		return "", fmt.Errorf("操作失败，请稍后重试")
	}
	_ = s.users.CreateAuditEvent(ctx, u.ID, "backup_export_requested", "", "")
	return token, nil
}

// ConsumeBackupExportToken validates + consumes backup export authorization.
func (s *AuthService) ConsumeBackupExportToken(ctx context.Context, userID, token string) error {
	req, err := s.verifyAndLoadAction(ctx, token, ActionBackupExport)
	if err != nil {
		return err
	}
	if req.UserID != userID {
		return ErrNotAuthorized
	}
	if _, err := s.users.ConsumeActionRequest(ctx, req.JTI, time.Now()); err != nil && !errors.Is(err, auth.ErrTokenAlreadyUsed) {
		return fmt.Errorf("操作失败，请稍后重试")
	}
	_ = s.users.CreateAuditEvent(ctx, userID, "backup_export_downloaded", "", "")
	return nil
}

// RequestAccountDeletion sends an account-deletion confirmation email.
func (s *AuthService) RequestAccountDeletion(ctx context.Context, userID string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("用户不存在")
	}
	token, jti, exp, err := s.actionTokens.Issue(u.ID, ActionAccountDelete, "", 30*time.Minute)
	if err != nil {
		return fmt.Errorf("操作失败，请稍后重试")
	}
	if err := s.users.CreateActionRequest(ctx, model.ActionRequest{
		JTI:       jti,
		UserID:    u.ID,
		Action:    ActionAccountDelete,
		Status:    "pending",
		ExpiresAt: exp,
		CreatedAt: time.Now(),
	}); err != nil {
		return fmt.Errorf("操作失败，请稍后重试")
	}
	if err := s.emailSvc.SendAccountDeletion(u.Email, u.Username, token); err != nil {
		return fmt.Errorf("邮件发送失败，请稍后重试")
	}
	return nil
}

// ConfirmAccountDeletion validates one-time token and permanently deletes account data.
func (s *AuthService) ConfirmAccountDeletion(ctx context.Context, token string) error {
	req, err := s.verifyAndLoadAction(ctx, token, ActionAccountDelete)
	if err != nil {
		return err
	}
	if err := s.users.DeleteUser(ctx, req.UserID); err != nil {
		if err.Error() == "user not found" {
			return ErrUserNotFound
		}
		return fmt.Errorf("删除失败，请稍后重试")
	}
	if _, err := s.users.ConsumeActionRequest(ctx, req.JTI, time.Now()); err != nil && !errors.Is(err, auth.ErrTokenAlreadyUsed) {
		return fmt.Errorf("确认失败，请稍后重试")
	}
	_ = s.users.CreateAuditEvent(ctx, req.UserID, "account_deleted", "", "")
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
	if req.Action != action {
		return model.ActionRequest{}, ErrInvalidToken
	}
	if req.Status == "completed" {
		return model.ActionRequest{}, ErrAlreadyUsed
	}
	if time.Now().After(req.ExpiresAt) {
		_ = s.users.ExpireActionRequests(ctx, action, time.Now())
		return model.ActionRequest{}, ErrExpiredToken
	}
	return req, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (LoginResponse, error) {
	// Check account lockout before any DB access to prevent timing-based enumeration.
	if s.tracker.IsLocked(email) {
		return LoginResponse{}, fmt.Errorf("账户已因多次失败尝试被锁定，请稍后再试")
	}
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		s.tracker.RecordFailure(email)
		return LoginResponse{}, fmt.Errorf("邮箱或密码错误")
	}
	if err := auth.CheckPassword(u.PasswordHash, password); err != nil {
		s.tracker.RecordFailure(email)
		return LoginResponse{}, fmt.Errorf("邮箱或密码错误")
	}
	if !u.EmailVerified {
		return LoginResponse{}, fmt.Errorf("email_not_verified")
	}
	// Successful login — clear failure counter.
	s.tracker.RecordSuccess(email)
	token, exp, err := s.jwt.Issue(u.ID, u.Email, u.Role, u.PwdVersion)
	if err != nil {
		return LoginResponse{}, fmt.Errorf("登录失败，请稍后重试")
	}
	return LoginResponse{
		Token: token, ExpiresAt: exp,
		UserID: u.ID, Email: u.Email, Username: u.Username, Nickname: u.Nickname, Role: u.Role,
	}, nil
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
