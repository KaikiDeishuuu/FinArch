package service

import (
	"context"
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
	users      repository.UserRepository
	jwt        *auth.JWTService
	tracker    *auth.LoginAttemptTracker
	emailSvc   email.Sender
	emailReq   bool   // whether email verification is required
	appBaseURL string // used to build verification links
}

func NewAuthService(
	users repository.UserRepository,
	jwt *auth.JWTService,
	tracker *auth.LoginAttemptTracker,
	emailSvc email.Sender,
	emailRequired bool,
	appBaseURL string,
) *AuthService {
	return &AuthService{
		users: users, jwt: jwt, tracker: tracker,
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
	nickname := req.Nickname
	if nickname == "" {
		nickname = randomNickname()
	}
	u := model.User{
		ID:            uuid.NewString(),
		Email:         req.Email,
		Username:      req.Username,
		Nickname:      nickname,
		Name:          req.Username,
		PasswordHash:  hash,
		Role:          "owner",
		EmailVerified: !s.emailReq, // auto-verified when email is not required
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.users.Create(ctx, u); err != nil {
		// pass through specific repo sentinel errors so the handler can
		// return a precise 409 response
		if err.Error() == "username_taken" || err.Error() == "email_taken" {
			return model.User{}, err
		}
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
	// Remove any prior verify tokens for this user
	_ = s.users.DeleteEmailTokensByUser(ctx, u.ID, "verify")
	token := uuid.NewString()
	et := model.EmailToken{
		Token: token, UserID: u.ID, Kind: "verify",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}
	if err := s.users.CreateEmailToken(ctx, et); err != nil {
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
	et, err := s.users.GetEmailToken(ctx, token)
	if err != nil || et.Kind != "verify" {
		return fmt.Errorf("无效或已过期的验证链接")
	}
	if time.Now().After(et.ExpiresAt) {
		_ = s.users.DeleteEmailToken(ctx, token)
		return fmt.Errorf("验证链接已过期，请重新发送")
	}
	if err := s.users.SetEmailVerified(ctx, et.UserID); err != nil {
		return fmt.Errorf("验证失败，请重试")
	}
	_ = s.users.DeleteEmailToken(ctx, token)
	return nil
}

// ForgotPassword sends a password reset email if the account exists and is verified.
func (s *AuthService) ForgotPassword(ctx context.Context, emailAddr string) error {
	u, err := s.users.GetByEmail(ctx, emailAddr)
	if err != nil || !u.EmailVerified {
		return nil // don't expose account existence
	}
	_ = s.users.DeleteEmailTokensByUser(ctx, u.ID, "reset")
	token := uuid.NewString()
	et := model.EmailToken{
		Token: token, UserID: u.ID, Kind: "reset",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		CreatedAt: time.Now(),
	}
	if err := s.users.CreateEmailToken(ctx, et); err != nil {
		return fmt.Errorf("操作失败，请稍后重试")
	}
	return s.emailSvc.SendPasswordReset(u.Email, u.Username, token)
}

// RequestEmailChange sends an authorization link to the CURRENT (old) email address.
// The user must click it to confirm they initiated the change; only then will a
// verification link be sent to the new address.
func (s *AuthService) RequestEmailChange(ctx context.Context, userID, newEmail string) error {
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
	if err := s.users.SetPendingEmail(ctx, u.ID, newEmail); err != nil {
		return fmt.Errorf("操作失败，请稍后重试")
	}
	// Clear any pre-existing old-verify or new-verify tokens.
	_ = s.users.DeleteEmailTokensByUser(ctx, u.ID, "change_email_old")
	_ = s.users.DeleteEmailTokensByUser(ctx, u.ID, "change_email")
	token := uuid.NewString()
	et := model.EmailToken{
		Token: token, UserID: u.ID, Kind: "change_email_old", Meta: newEmail,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		CreatedAt: time.Now(),
	}
	if err := s.users.CreateEmailToken(ctx, et); err != nil {
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
	et, err := s.users.GetEmailToken(ctx, token)
	if err != nil || et.Kind != "change_email_old" {
		return fmt.Errorf("无效或已过期的授权链接")
	}
	if time.Now().After(et.ExpiresAt) {
		_ = s.users.DeleteEmailToken(ctx, token)
		return fmt.Errorf("链接已过期，请重新申请")
	}
	newEmail := et.Meta
	if newEmail == "" {
		return fmt.Errorf("无效的邮箱变更请求")
	}
	u, err := s.users.GetByID(ctx, et.UserID)
	if err != nil {
		return fmt.Errorf("用户不存在")
	}
	// Check new email still available.
	if _, err := s.users.GetByEmail(ctx, newEmail); err == nil {
		return fmt.Errorf("该邮箱已被其他账户使用，请重新申请")
	}
	// Consume the old-email token and create a new-email verification token.
	_ = s.users.DeleteEmailToken(ctx, token)
	_ = s.users.DeleteEmailTokensByUser(ctx, u.ID, "change_email")
	newToken := uuid.NewString()
	net := model.EmailToken{
		Token: newToken, UserID: u.ID, Kind: "change_email", Meta: newEmail,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		CreatedAt: time.Now(),
	}
	if err := s.users.CreateEmailToken(ctx, net); err != nil {
		return fmt.Errorf("操作失败，请稍后重试")
	}
	// Step 2: send verification link to the NEW email.
	if err := s.emailSvc.SendEmailChange(newEmail, u.Username, newToken); err != nil {
		return fmt.Errorf("邮件发送失败，请稍后重试")
	}
	return nil
}

// ConfirmEmailChange validates the token and applies the email change.
func (s *AuthService) ConfirmEmailChange(ctx context.Context, token string) error {
	et, err := s.users.GetEmailToken(ctx, token)
	if err != nil || et.Kind != "change_email" {
		return fmt.Errorf("无效或已过期的邮箱变更链接")
	}
	if time.Now().After(et.ExpiresAt) {
		_ = s.users.DeleteEmailToken(ctx, token)
		return fmt.Errorf("链接已过期，请重新申请")
	}
	if et.Meta == "" {
		return fmt.Errorf("无效的邮箱变更请求")
	}
	if err := s.users.UpdateEmail(ctx, et.UserID, et.Meta); err != nil {
		if err.Error() == "email_taken" {
			return fmt.Errorf("该邮箱已被其他账户使用，请重新申请")
		}
		return fmt.Errorf("邮箱更新失败，请稍后重试")
	}
	_ = s.users.DeleteEmailToken(ctx, token)
	return nil
}

// ResetPassword resets the user's password using a valid reset token.
func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("新密码至少需要 8 位")
	}
	et, err := s.users.GetEmailToken(ctx, token)
	if err != nil || et.Kind != "reset" {
		return fmt.Errorf("无效或已过期的重置链接")
	}
	if time.Now().After(et.ExpiresAt) {
		_ = s.users.DeleteEmailToken(ctx, token)
		return fmt.Errorf("重置链接已过期，请重新申请")
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("密码设置失败，请稍后重试")
	}
	if err := s.users.UpdatePassword(ctx, et.UserID, hash); err != nil {
		return fmt.Errorf("密码重置失败，请稍后重试")
	}
	_ = s.users.DeleteEmailToken(ctx, token)
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
	return nil
}

// RequestAccountDeletion sends an account-deletion confirmation email.
func (s *AuthService) RequestAccountDeletion(ctx context.Context, userID string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("用户不存在")
	}
	// Invalidate any existing delete tokens for this user.
	_ = s.users.DeleteEmailTokensByUser(ctx, u.ID, "delete")
	token := uuid.NewString()
	et := model.EmailToken{
		Token:     token,
		UserID:    u.ID,
		Kind:      "delete",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		CreatedAt: time.Now(),
	}
	if err := s.users.CreateEmailToken(ctx, et); err != nil {
		return fmt.Errorf("操作失败，请稍后重试")
	}
	if err := s.emailSvc.SendAccountDeletion(u.Email, u.Username, token); err != nil {
		return fmt.Errorf("邮件发送失败，请稍后重试")
	}
	return nil
}

// ConfirmAccountDeletion validates the token and permanently deletes the user and all their data.
func (s *AuthService) ConfirmAccountDeletion(ctx context.Context, token string) error {
	et, err := s.users.GetEmailToken(ctx, token)
	if err != nil || et.Kind != "delete" {
		return fmt.Errorf("无效或已过期的注销链接")
	}
	if time.Now().After(et.ExpiresAt) {
		_ = s.users.DeleteEmailToken(ctx, token)
		return fmt.Errorf("注销链接已过期，请重新申请")
	}
	return s.users.DeleteUser(ctx, et.UserID)
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

// randomNickname produces a fun random display name for users who don't set one.
func randomNickname() string {
	adj := []string{"快乐的", "努力的", "认真的", "聪明的", "活力的", "优秀的", "可爱的", "勤奋的", "机智的", "阳光的", "温暖的", "真诚的"}
	noun := []string{"小猫", "小狗", "兔子", "松鼠", "企鹅", "海豚", "熊猫", "考拉", "柴犬", "仓鼠", "水獭", "树袋熊"}
	return adj[time.Now().UnixNano()%int64(len(adj))] + noun[time.Now().UnixNano()/7%int64(len(noun))]
}

// CleanupExpiredUnverified removes all unverified users whose registration is
// older than 24 hours. It returns the number of deleted users.
func (s *AuthService) CleanupExpiredUnverified(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-24 * time.Hour)
	return s.users.DeleteExpiredUnverifiedUsers(ctx, cutoff)
}
