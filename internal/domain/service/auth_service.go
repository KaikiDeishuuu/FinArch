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

type RegisterRequest struct {
	Email    string
	Name     string
	Password string
}

type LoginResponse struct {
	Token     string
	ExpiresAt time.Time
	UserID    string
	Email     string
	Name      string
	Role      string
}

// Register creates a new user. If email verification is required, the user starts
// as unverified and a verification email is sent. Returns the created user.
func (s *AuthService) Register(ctx context.Context, req RegisterRequest) (model.User, error) {
	if req.Email == "" || req.Password == "" || req.Name == "" {
		return model.User{}, fmt.Errorf("email, name and password are required")
	}
	if len(req.Password) < 8 {
		return model.User{}, fmt.Errorf("password must be at least 8 characters")
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return model.User{}, err
	}
	now := time.Now()
	u := model.User{
		ID:            uuid.NewString(),
		Email:         req.Email,
		Name:          req.Name,
		PasswordHash:  hash,
		Role:          "owner",
		EmailVerified: !s.emailReq, // auto-verified when email is not required
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return model.User{}, fmt.Errorf("register: %w", err)
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
	return s.emailSvc.SendVerification(u.Email, u.Name, token)
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
		return err
	}
	return s.emailSvc.SendPasswordReset(u.Email, u.Name, token)
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
		return err
	}
	if err := s.users.UpdatePassword(ctx, et.UserID, hash); err != nil {
		return err
	}
	_ = s.users.DeleteEmailToken(ctx, token)
	return nil
}

// ChangePassword verifies currentPassword then replaces it with newPassword.
func (s *AuthService) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("new password must be at least 8 characters")
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}
	if err := auth.CheckPassword(u.PasswordHash, currentPassword); err != nil {
		return fmt.Errorf("current password is incorrect")
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, userID, hash)
}

func (s *AuthService) Login(ctx context.Context, email, password string) (LoginResponse, error) {
	// Check account lockout before any DB access to prevent timing-based enumeration.
	if s.tracker.IsLocked(email) {
		return LoginResponse{}, fmt.Errorf("账户已因多次失败尝试被锁定，请稍后再试")
	}
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		s.tracker.RecordFailure(email)
		return LoginResponse{}, fmt.Errorf("invalid credentials")
	}
	if err := auth.CheckPassword(u.PasswordHash, password); err != nil {
		s.tracker.RecordFailure(email)
		return LoginResponse{}, fmt.Errorf("invalid credentials")
	}
	if !u.EmailVerified {
		return LoginResponse{}, fmt.Errorf("email_not_verified")
	}
	// Successful login — clear failure counter.
	s.tracker.RecordSuccess(email)
	token, exp, err := s.jwt.Issue(u.ID, u.Email, u.Role)
	if err != nil {
		return LoginResponse{}, err
	}
	return LoginResponse{
		Token: token, ExpiresAt: exp,
		UserID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role,
	}, nil
}
