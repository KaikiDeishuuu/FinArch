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
		return model.Reimbursement{}, fmt.Errorf("申请人不能为空")
	}
	if len(req.TransactionIDs) == 0 {
		return model.Reimbursement{}, fmt.Errorf("请选择至少一笔交易")
	}

	unique := deduplicate(req.TransactionIDs)
	if len(unique) != len(req.TransactionIDs) {
		return model.Reimbursement{}, fmt.Errorf("交易记录不能重复")
	}

	if req.RequestNo == "" {
		req.RequestNo = "REIM-" + time.Now().Format("20060102-150405")
	}

	var created model.Reimbursement
	err := s.transactionManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		txs, err := s.transactions.GetByIDs(txCtx, unique)
		if err != nil {
			return fmt.Errorf("查询交易记录失败")
		}
		if len(txs) != len(unique) {
			return fmt.Errorf("部分交易记录不存在")
		}

		var total model.Money
		items := make([]model.ReimbursementItem, 0, len(txs))
		reimID := uuid.NewString()
		for _, t := range txs {
			if t.Source != model.SourcePersonal || t.Direction != model.DirectionExpense {
				return fmt.Errorf("包含非个人支出的交易，无法报销")
			}
			if t.Reimbursed {
				return fmt.Errorf("包含已报销的交易，请检查")
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
			return fmt.Errorf("创建报销单失败，请稍后重试")
		}
		if err := s.reimbursements.AddItems(txCtx, items); err != nil {
			return fmt.Errorf("创建报销单失败，请稍后重试")
		}
		if err := s.transactions.MarkReimbursed(txCtx, unique, reimID); err != nil {
			return fmt.Errorf("标记报销失败，请稍后重试")
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
