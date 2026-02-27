package service

import (
	"context"
	"fmt"
	"slices"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"

	"github.com/google/uuid"
)

// ReimbursementService handles reimbursement use cases.
type ReimbursementService struct {
	transactionManager repository.TransactionManager
	transactions       repository.TransactionRepository
	reimbursements     repository.ReimbursementRepository
}

// NewReimbursementService creates a new ReimbursementService.
func NewReimbursementService(
	transactionManager repository.TransactionManager,
	transactions repository.TransactionRepository,
	reimbursements repository.ReimbursementRepository,
) *ReimbursementService {
	return &ReimbursementService{
		transactionManager: transactionManager,
		transactions:       transactions,
		reimbursements:     reimbursements,
	}
}

// CreateReimbursementRequest is the input of CreateReimbursement.
type CreateReimbursementRequest struct {
	Applicant      string
	TransactionIDs []string
	RequestNo      string
}

// CreateReimbursement creates one reimbursement from transactions atomically.
func (s *ReimbursementService) CreateReimbursement(ctx context.Context, req CreateReimbursementRequest) (model.Reimbursement, error) {
	if req.Applicant == "" {
		return model.Reimbursement{}, fmt.Errorf("applicant is required")
	}
	if len(req.TransactionIDs) == 0 {
		return model.Reimbursement{}, fmt.Errorf("transaction_ids is empty")
	}

	unique := deduplicate(req.TransactionIDs)
	if len(unique) != len(req.TransactionIDs) {
		return model.Reimbursement{}, fmt.Errorf("transaction_ids contains duplicates")
	}

	if req.RequestNo == "" {
		req.RequestNo = "REIM-" + time.Now().Format("20060102-150405")
	}

	var created model.Reimbursement
	err := s.transactionManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		txs, err := s.transactions.GetByIDs(txCtx, unique)
		if err != nil {
			return err
		}
		if len(txs) != len(unique) {
			return fmt.Errorf("some transaction IDs do not exist")
		}

		var total model.Money
		items := make([]model.ReimbursementItem, 0, len(txs))
		reimID := uuid.NewString()
		for _, t := range txs {
			if t.Source != model.SourcePersonal || t.Direction != model.DirectionExpense {
				return fmt.Errorf("transaction %s is not personal expense", t.ID)
			}
			if t.Reimbursed {
				return fmt.Errorf("transaction %s already reimbursed", t.ID)
			}
			total += t.AmountYuan
			items = append(items, model.ReimbursementItem{
				ReimbursementID: reimID,
				TransactionID:   t.ID,
				AmountYuan:      t.AmountYuan,
			})
		}

		now := time.Now()
		reimbursement := model.Reimbursement{
			ID:        reimID,
			RequestNo: req.RequestNo,
			Applicant: req.Applicant,
			TotalYuan: total,
			Status:    "submitted",
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := s.reimbursements.Create(txCtx, reimbursement); err != nil {
			return err
		}
		if err := s.reimbursements.AddItems(txCtx, items); err != nil {
			return err
		}
		if err := s.transactions.MarkReimbursed(txCtx, unique, reimID); err != nil {
			return err
		}
		created = reimbursement
		return nil
	})
	if err != nil {
		return model.Reimbursement{}, err
	}
	return created, nil
}

func deduplicate(input []string) []string {
	result := make([]string, 0, len(input))
	for _, id := range input {
		if !slices.Contains(result, id) {
			result = append(result, id)
		}
	}
	return result
}
