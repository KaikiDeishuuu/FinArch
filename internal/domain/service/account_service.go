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
	accounts     repository.AccountRepository
	transactions repository.TransactionRepository // needed for deletion guard
	txManager    repository.TransactionManager
}

// NewAccountService creates an AccountService.
func NewAccountService(
	accounts repository.AccountRepository,
	transactions repository.TransactionRepository,
	txManager repository.TransactionManager,
) *AccountService {
	return &AccountService{
		accounts:     accounts,
		transactions: transactions,
		txManager:    txManager,
	}
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
//
// Backend enforcement: WORK mode only allows public accounts; LIFE mode only
// allows personal accounts. These rules mirror the frontend UI restrictions but
// are enforced here so no API client can bypass them.
func (s *AccountService) CreateAccount(
	ctx context.Context,
	userID, name string,
	t model.AccountType,
	currency string,
	mode model.Mode,
) (model.Account, error) {
	if name == "" {
		return model.Account{}, fmt.Errorf("账户名称不能为空")
	}
	if currency == "" {
		currency = "CNY"
	}

	// ── Mode-based type restriction ───────────────────────────────────────────
	switch mode {
	case model.ModeWork:
		if t != model.AccountTypePublic {
			return model.Account{}, fmt.Errorf("工作模式下只能创建公共账户（public），不允许创建个人账户")
		}
	case model.ModeLife:
		if t != model.AccountTypePersonal {
			return model.Account{}, fmt.Errorf("生活模式下只能创建个人账户（personal），不允许创建公共账户")
		}
	default:
		return model.Account{}, fmt.Errorf("无效的模式：%s", mode)
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
		return model.Account{}, fmt.Errorf("创建账户失败，请稍后重试")
	}
	return a, nil
}

// DeleteAccount soft-deletes an account. Enforces two backend-side guards:
//  1. The last account of each type (personal / public) cannot be deleted.
//  2. Accounts with unreimbursed expense transactions cannot be deleted.
func (s *AccountService) DeleteAccount(ctx context.Context, accountID, userID string) error {
	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		a, err := s.accounts.GetByID(txCtx, accountID)
		if err != nil {
			return fmt.Errorf("账户不存在")
		}
		if a.UserID != userID {
			return fmt.Errorf("无权操作该账户")
		}

		// Guard 1: must keep at least one account of each type.
		count, err := s.accounts.CountByUserAndType(txCtx, userID, a.Type)
		if err != nil {
			return fmt.Errorf("删除失败，请稍后重试")
		}
		if count <= 1 {
			typeLabel := "个人"
			if a.Type == model.AccountTypePublic {
				typeLabel = "公共"
			}
			return fmt.Errorf("至少需要保留一个%s账户，无法删除", typeLabel)
		}

		// Guard 2: disallow deletion while unreimbursed expenses are bound to this account.
		hasUnreimbursed, err := s.transactions.HasUnreimbursedByAccount(txCtx, accountID, userID)
		if err != nil {
			return fmt.Errorf("删除失败，请稍后重试")
		}
		if hasUnreimbursed {
			return fmt.Errorf("该子账户存在未报销的交易，无法删除，请先完成报销后再操作")
		}

		return s.accounts.Delete(txCtx, accountID, userID)
	})
}

// RenameAccount renames an account.
func (s *AccountService) RenameAccount(ctx context.Context, accountID, userID, newName string) error {
	return s.accounts.UpdateName(ctx, accountID, userID, newName)
}
