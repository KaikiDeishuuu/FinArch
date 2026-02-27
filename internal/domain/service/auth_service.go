package service

import (
	"context"
	"fmt"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"
	"finarch/internal/infrastructure/auth"

	"github.com/google/uuid"
)

// AuthService handles user registration and login.
type AuthService struct {
	users repository.UserRepository
	jwt   *auth.JWTService
}

func NewAuthService(users repository.UserRepository, jwt *auth.JWTService) *AuthService {
	return &AuthService{users: users, jwt: jwt}
}

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
		ID:           uuid.NewString(),
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: hash,
		Role:         "owner",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return model.User{}, fmt.Errorf("register: %w", err)
	}
	return u, nil
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
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return LoginResponse{}, fmt.Errorf("invalid credentials")
	}
	if err := auth.CheckPassword(u.PasswordHash, password); err != nil {
		return LoginResponse{}, fmt.Errorf("invalid credentials")
	}
	token, exp, err := s.jwt.Issue(u.ID, u.Email, u.Role)
	if err != nil {
		return LoginResponse{}, err
	}
	return LoginResponse{
		Token: token, ExpiresAt: exp,
		UserID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role,
	}, nil
}
