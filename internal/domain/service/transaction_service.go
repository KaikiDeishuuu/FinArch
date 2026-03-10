package service

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"

	"github.com/google/uuid"
)

// TransactionService handles transaction use cases.
type TransactionService struct {
	transactions repository.TransactionRepository
	accounts     repository.AccountRepository
	rates        ExchangeRateService
}

// NewTransactionService creates a new TransactionService.
func NewTransactionService(transactions repository.TransactionRepository, accounts repository.AccountRepository, rates ExchangeRateService) *TransactionService {
	return &TransactionService{transactions: transactions, accounts: accounts, rates: rates}
}

type CreateTransactionRequest struct {
	UserID       string
	OccurredAt   time.Time
	AccountID    string
	TxType       model.TxType
	Direction    model.Direction
	Source       model.Source
	Mode         model.Mode
	Category     string
	AmountYuan   model.Money
	AmountCents  int64
	Currency     string
	ExchangeRate float64
	Note         string
	ProjectID    *string
}

func (s *TransactionService) CreateTransaction(ctx context.Context, req CreateTransactionRequest) (model.Transaction, error) {
	if req.AmountCents == 0 && req.AmountYuan > 0 {
		req.AmountCents = int64(req.AmountYuan * 100)
	}
	if req.AmountCents <= 0 {
		return model.Transaction{}, fmt.Errorf("金额必须为正数")
	}
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	if req.Currency == "" {
		req.Currency = "CNY"
	}
	if req.Mode == "" {
		req.Mode = model.ModeWork
	}
	if req.Mode != model.ModeWork && req.Mode != model.ModeLife {
		return model.Transaction{}, fmt.Errorf("无效的模式")
	}
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

	accountID := req.AccountID
	var accountType model.AccountType
	var accountCurrency string
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
		accountCurrency = strings.ToUpper(acct.Currency)
	} else {
		acct, err := s.accounts.GetByID(ctx, accountID)
		if err != nil {
			return model.Transaction{}, fmt.Errorf("所选账户不存在")
		}
		accountType = acct.Type
		accountCurrency = strings.ToUpper(acct.Currency)
	}
	if accountCurrency == "" {
		accountCurrency = "CNY"
	}

	reimb := model.ReimbStatusNone
	if req.Mode != model.ModeLife && txType == model.TxTypeExpense && accountType == model.AccountTypePersonal {
		reimb = model.ReimbStatusPending
	}
	if req.Category == "" {
		return model.Transaction{}, fmt.Errorf("请选择分类")
	}
	occurredAt := req.OccurredAt.UTC().Truncate(time.Second)
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC().Truncate(time.Second)
	}

	rateValue, rateSource, rateAt, err := s.resolveRate(ctx, req.UserID, req.Currency, accountCurrency, occurredAt, req.ExchangeRate)
	if err != nil {
		return model.Transaction{}, err
	}
	converted, err := convertByRate(req.AmountCents, rateValue)
	if err != nil {
		return model.Transaction{}, fmt.Errorf("汇率换算失败: %w", err)
	}

	now := time.Now()
	t := model.Transaction{
		ID:                 uuid.NewString(),
		UserID:             req.UserID,
		GroupID:            "",
		LedgerDir:          ledgerDir,
		TxType:             txType,
		AccountID:          accountID,
		AccountType:        accountType,
		AmountCents:        req.AmountCents,
		AmountYuan:         model.Money(float64(req.AmountCents) / 100.0),
		Currency:           req.Currency,
		ExchangeRate:       rateValue,
		ExchangeRateSource: rateSource,
		ExchangeRateAt:     rateAt.Unix(),
		BaseCurrency:       accountCurrency,
		BaseAmountCents:    converted,
		Category:           req.Category,
		ReimbStatus:        reimb,
		Mode:               req.Mode,
		Note:               req.Note,
		ProjectID:          req.ProjectID,
		Uploaded:           false,
		TxnDate:            occurredAt.Format("2006-01-02"),
		TransactionTime:    occurredAt.Unix(),
		OccurredAt:         occurredAt,
		Direction:          model.Direction(txType),
		Source:             sourceFromAccountType(accountType),
		Reimbursed:         false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := s.transactions.Create(ctx, t); err != nil {
		return model.Transaction{}, fmt.Errorf("创建交易失败，请稍后重试: %w", err)
	}
	return t, nil
}

func (s *TransactionService) resolveRate(ctx context.Context, userID, from, to string, occurredAt time.Time, provided float64) (float64, string, time.Time, error) {
	if from == to {
		return 1, "identity", occurredAt, nil
	}
	if provided > 0 {
		return provided, "client_provided", occurredAt, nil
	}
	if s.rates != nil {
		rate, err := s.rates.GetRate(ctx, from, to, occurredAt)
		if err == nil {
			return rate.RateFloat, rate.Source, rate.At, nil
		}
		log.Printf("exchange-rate primary fetch failed from=%s to=%s at=%s err=%v", from, to, occurredAt.Format(time.RFC3339), err)
	}
	rate, rateAt, source, err := s.transactions.GetRecentRate(ctx, userID, from, to)
	if err == nil && rate > 0 {
		at := time.Unix(rateAt, 0).UTC()
		if rateAt == 0 {
			at = occurredAt
		}
		return rate, "db_fallback:" + source, at, nil
	}
	return 0, "", time.Time{}, fmt.Errorf("无法获取汇率（%s→%s）", from, to)
}

func convertByRate(amountCents int64, rate float64) (int64, error) {
	if amountCents <= 0 || rate <= 0 {
		return 0, fmt.Errorf("invalid amount or rate")
	}
	rateRat, ok := new(big.Rat).SetString(fmt.Sprintf("%.10f", rate))
	if !ok {
		return 0, fmt.Errorf("invalid rate")
	}
	result := new(big.Rat).Mul(big.NewRat(amountCents, 1), rateRat)
	n := new(big.Int).Set(result.Num())
	d := new(big.Int).Set(result.Denom())
	half := new(big.Int).Div(d, big.NewInt(2))
	n.Add(n, half)
	n.Div(n, d)
	return n.Int64(), nil
}

func (s *TransactionService) GetBalances(ctx context.Context, userID string) (model.Money, model.Money, error) {
	return s.transactions.SumPoolBalance(ctx, userID, model.ModeWork)
}

func sourceFromAccountType(t model.AccountType) model.Source {
	if t == model.AccountTypePersonal {
		return model.SourcePersonal
	}
	return model.SourceCompany
}
