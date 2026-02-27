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
}

// NewTransactionService creates a new TransactionService.
func NewTransactionService(transactions repository.TransactionRepository) *TransactionService {
	return &TransactionService{transactions: transactions}
}

// CreateTransactionRequest is the input of CreateTransaction.
type CreateTransactionRequest struct {
	UserID     string
	OccurredAt time.Time
	Direction  model.Direction
	Source     model.Source
	Category   string
	AmountYuan model.Money
	Currency   string
	Note       string
	ProjectID  *string
}

// CreateTransaction validates and persists one transaction.
func (s *TransactionService) CreateTransaction(ctx context.Context, req CreateTransactionRequest) (model.Transaction, error) {
	if req.AmountYuan <= 0 {
		return model.Transaction{}, fmt.Errorf("amount_yuan must be positive")
	}
	if req.Direction != model.DirectionIncome && req.Direction != model.DirectionExpense {
		return model.Transaction{}, fmt.Errorf("invalid direction")
	}
	if req.Source != model.SourceCompany && req.Source != model.SourcePersonal {
		return model.Transaction{}, fmt.Errorf("invalid source")
	}
	if req.Category == "" {
		return model.Transaction{}, fmt.Errorf("category is required")
	}
	if req.Currency == "" {
		req.Currency = "CNY"
	}

	now := time.Now()
	t := model.Transaction{
		ID:         uuid.NewString(),
		UserID:     req.UserID,
		OccurredAt: req.OccurredAt,
		Direction:  req.Direction,
		Source:     req.Source,
		Category:   req.Category,
		AmountYuan: req.AmountYuan,
		Currency:   req.Currency,
		Note:       req.Note,
		ProjectID:  req.ProjectID,
		Reimbursed: false,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.transactions.Create(ctx, t); err != nil {
		return model.Transaction{}, err
	}
	return t, nil
}

// GetBalances returns company balance and personal outstanding in yuan for a user.
func (s *TransactionService) GetBalances(ctx context.Context, userID string) (model.Money, model.Money, error) {
	return s.transactions.SumPoolBalance(ctx, userID)
}
