package service

import (
	"context"
	"fmt"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"

	"github.com/google/uuid"
)

// TransactionService handles transaction use cases.
type TransactionService struct {
	transactions repository.TransactionRepository
	accounts     repository.AccountRepository
}

// NewTransactionService creates a new TransactionService.
func NewTransactionService(transactions repository.TransactionRepository, accounts repository.AccountRepository) *TransactionService {
	return &TransactionService{transactions: transactions, accounts: accounts}
}

// CreateTransactionRequest is the input of CreateTransaction.
type CreateTransactionRequest struct {
	UserID     string
	OccurredAt time.Time
	// V9 fields (preferred)
	AccountID string // if empty, derived from Source
	TxType    model.TxType
	// Backward-compat fields
	Direction model.Direction // 'income' | 'expense'
	Source    model.Source    // 'company' | 'personal'
	// Common
	Mode         model.Mode
	Category     string
	AmountYuan   model.Money
	AmountCents  int64 // if 0, derived from AmountYuan * 100
	Currency     string
	ExchangeRate float64 // if 0, defaults to 1.0
	Note         string
	ProjectID    *string
}

// CreateTransaction validates and persists one transaction.
func (s *TransactionService) CreateTransaction(ctx context.Context, req CreateTransactionRequest) (model.Transaction, error) {
	// ── Normalize amount ──────────────────────────────────────────────────────
	if req.AmountCents == 0 && req.AmountYuan > 0 {
		req.AmountCents = int64(req.AmountYuan * 100)
	}
	if req.AmountCents <= 0 {
		return model.Transaction{}, fmt.Errorf("金额必须为正数")
	}
	if req.ExchangeRate <= 0 {
		req.ExchangeRate = 1.0
	}
	if req.Currency == "" {
		req.Currency = "CNY"
	}
	if req.Mode == "" {
		req.Mode = model.ModeWork
	}
	if req.Mode != model.ModeWork && req.Mode != model.ModeLife {
		return model.Transaction{}, fmt.Errorf("无效的模式")
	}
	// ── Normalize type / direction ────────────────────────────────────────────
	txType := req.TxType
	if txType == "" {
		if req.Direction == model.DirectionExpense {
			txType = model.TxTypeExpense
		} else {
			txType = model.TxTypeIncome
		}
	}
	if txType != model.TxTypeIncome && txType != model.TxTypeExpense && txType != model.TxTypeTransfer {
		return model.Transaction{}, fmt.Errorf("无效的交易类型")
	}
	ledgerDir := model.LedgerCredit
	if txType == model.TxTypeExpense || txType == model.TxTypeTransfer {
		ledgerDir = model.LedgerDebit
	}
	// ── Resolve account ───────────────────────────────────────────────────────
	accountID := req.AccountID
	var accountType model.AccountType
	if accountID == "" {
		acctT := model.AccountTypePublic
		if req.Source == model.SourcePersonal {
			acctT = model.AccountTypePersonal
		}
		acct, err := s.accounts.GetByUserAndType(ctx, req.UserID, acctT)
		if err != nil {
			return model.Transaction{}, fmt.Errorf("未找到对应账户，请先在设置中创建")
		}
		accountID = acct.ID
		accountType = acct.Type
	} else {
		acct, err := s.accounts.GetByID(ctx, accountID)
		if err != nil {
			return model.Transaction{}, fmt.Errorf("所选账户不存在")
		}
		accountType = acct.Type
	}
	// ── Derive reimbursement status ───────────────────────────────────────────
	reimb := model.ReimbStatusNone
	if req.Mode != model.ModeLife && txType == model.TxTypeExpense && accountType == model.AccountTypePersonal {
		reimb = model.ReimbStatusPending
	}
	if req.Category == "" {
		return model.Transaction{}, fmt.Errorf("请选择分类")
	}
	now := time.Now()
	txnDate := req.OccurredAt.Format("2006-01-02")
	baseAmountCents := int64(float64(req.AmountCents) * req.ExchangeRate)
	t := model.Transaction{
		ID:              uuid.NewString(),
		UserID:          req.UserID,
		GroupID:         "", // set in Create
		LedgerDir:       ledgerDir,
		TxType:          txType,
		AccountID:       accountID,
		AccountType:     accountType,
		AmountCents:     req.AmountCents,
		AmountYuan:      model.Money(float64(req.AmountCents) / 100.0),
		BaseAmountCents: baseAmountCents,
		ExchangeRate:    req.ExchangeRate,
		Currency:        req.Currency,
		Category:        req.Category,
		ReimbStatus:     reimb,
		Mode:            req.Mode,
		Note:            req.Note,
		ProjectID:       req.ProjectID,
		Uploaded:        false,
		TxnDate:         txnDate,
		OccurredAt:      req.OccurredAt,
		// backward-compat
		Direction:  model.Direction(txType),
		Source:     sourceFromAccountType(accountType),
		Reimbursed: false,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.transactions.Create(ctx, t); err != nil {
		return model.Transaction{}, fmt.Errorf("创建交易失败，请稍后重试")
	}
	return t, nil
}

// GetBalances returns company balance and personal outstanding in yuan for a user.
func (s *TransactionService) GetBalances(ctx context.Context, userID string) (model.Money, model.Money, error) {
	return s.transactions.SumPoolBalance(ctx, userID, model.ModeWork)
}

func sourceFromAccountType(t model.AccountType) model.Source {
	if t == model.AccountTypePersonal {
		return model.SourcePersonal
	}
	return model.SourceCompany
}

// CreateTransactionRequest is the input of CreateTransaction.
