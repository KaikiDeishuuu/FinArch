package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
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
	UserID         string
	Applicant      string
	TransactionIDs []string
	RequestNo      string
}

// CreateReimbursement creates one reimbursement from transactions atomically.
func (s *ReimbursementService) CreateReimbursement(ctx context.Context, req CreateReimbursementRequest) (model.Reimbursement, error) {
	if req.UserID == "" {
		return model.Reimbursement{}, fmt.Errorf("用户不能为空")
	}
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
		txs, err := s.transactions.GetByIDs(txCtx, req.UserID, unique)
		if err != nil {
			return fmt.Errorf("查询交易记录失败")
		}
		if len(txs) != len(unique) {
			return fmt.Errorf("部分交易记录不存在")
		}

		var totalCents int64
		items := make([]model.ReimbursementItem, 0, len(txs))
		reimID := uuid.NewString()
		for _, t := range txs {
			if t.Source != model.SourcePersonal || t.Direction != model.DirectionExpense {
				return fmt.Errorf("包含非个人支出的交易，无法报销")
			}
			if t.ReimbStatus != model.ReimbStatusPending {
				return fmt.Errorf("包含不处于待报销状态的交易，请检查")
			}
			baseCurrency := strings.ToUpper(strings.TrimSpace(t.BaseCurrency))
			if baseCurrency == "" {
				baseCurrency = "CNY"
			}
			if baseCurrency != "CNY" {
				return fmt.Errorf("%w: 报销金额必须以 CNY 为本位币（发现 %s）", repository.ErrMultiCurrencyReportingUnavailable, baseCurrency)
			}
			if t.BaseAmountCents <= 0 || totalCents > math.MaxInt64-t.BaseAmountCents {
				return fmt.Errorf("交易本位币金额无效")
			}
			totalCents += t.BaseAmountCents
			items = append(items, model.ReimbursementItem{
				ReimbursementID: reimID,
				TransactionID:   t.ID,
				AmountCents:     t.BaseAmountCents,
				AmountYuan:      model.Money(float64(t.BaseAmountCents) / 100.0),
			})
		}

		now := time.Now()
		reimbursement := model.Reimbursement{
			ID:         reimID,
			RequestNo:  req.RequestNo,
			Applicant:  req.Applicant,
			TotalCents: totalCents,
			TotalYuan:  model.Money(float64(totalCents) / 100.0),
			Status:     "submitted",
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := s.reimbursements.Create(txCtx, reimbursement); err != nil {
			return fmt.Errorf("创建报销单失败，请稍后重试")
		}
		if err := s.reimbursements.AddItems(txCtx, items); err != nil {
			return fmt.Errorf("创建报销单失败，请稍后重试")
		}
		if err := s.transactions.MarkReimbursed(txCtx, req.UserID, unique, reimID); err != nil {
			if errors.Is(err, repository.ErrConcurrentModification) {
				return fmt.Errorf("%w: 报销交易状态已发生变化", ErrConcurrentModification)
			}
			return fmt.Errorf("标记报销失败，请稍后重试: %w", err)
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
