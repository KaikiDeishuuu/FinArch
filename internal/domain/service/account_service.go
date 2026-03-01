package service

import (
	"context"
	"fmt"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"

	"github.com/google/uuid"
)

// AccountService manages account creation and queries.
type AccountService struct {
	accounts repository.AccountRepository
}

// NewAccountService creates an AccountService.
func NewAccountService(accounts repository.AccountRepository) *AccountService {
	return &AccountService{accounts: accounts}
}

// EnsureDefaultAccounts idempotently creates the two default accounts for a user.
// Called after registration so every user always has a personal + public account.
func (s *AccountService) EnsureDefaultAccounts(ctx context.Context, userID string) error {
	for _, t := range []model.AccountType{model.AccountTypePersonal, model.AccountTypePublic} {
		if _, err := s.accounts.GetByUserAndType(ctx, userID, t); err == nil {
			continue // already exists
		}
		name := "个人账户"
		if t == model.AccountTypePublic {
			name = "公司账户"
		}
		now := time.Now()
		if err := s.accounts.Create(ctx, model.Account{
			ID:        uuid.NewString(),
			UserID:    userID,
			Name:      name,
			Type:      t,
			Currency:  "CNY",
			IsActive:  true,
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("ensure default account (%s): %w", t, err)
		}
	}
	return nil
}

// ListAccounts returns all active accounts for a user.
func (s *AccountService) ListAccounts(ctx context.Context, userID string) ([]model.Account, error) {
	return s.accounts.ListByUser(ctx, userID)
}

// CreateAccount creates a new named account for a user.
func (s *AccountService) CreateAccount(ctx context.Context, userID, name string, t model.AccountType, currency string) (model.Account, error) {
	if name == "" {
		return model.Account{}, fmt.Errorf("account name is required")
	}
	if currency == "" {
		currency = "CNY"
	}
	now := time.Now()
	a := model.Account{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      name,
		Type:      t,
		Currency:  currency,
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.accounts.Create(ctx, a); err != nil {
		return model.Account{}, fmt.Errorf("create account: %w", err)
	}
	return a, nil
}

// RenameAccount renames an account.
func (s *AccountService) RenameAccount(ctx context.Context, accountID, userID, newName string) error {
	a, err := s.accounts.GetByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("account not found")
	}
	if a.UserID != userID {
		return fmt.Errorf("permission denied")
	}
	a.Name = newName
	return s.accounts.Update(ctx, a)
}
