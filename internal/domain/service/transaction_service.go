package service

import (
	"context"
	"fmt"
	"log"
	"math"
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
	UserID                  string
	OccurredAt              time.Time
	AccountID               string
	TxType                  model.TxType
	Direction               model.Direction
	Source                  model.Source
	Mode                    model.Mode
	Category                string
	AmountYuan              model.Money
	AmountCents             int64
	Currency                string
	ExchangeRate            float64
	Note                    string
	ProjectID               *string
	AttachmentKey           *string
	IdempotencyKey          *string
	RecurringRuleID         *string
	RecurringOccurrenceDate *string
	resolvedRate            *resolvedTransactionRateEvidence
}

// resolvedTransactionRateEvidence is intentionally package-private so HTTP
// callers cannot assert trusted exchange-rate provenance. Recurring generation
// resolves it before opening a write transaction, then CreateTransaction
// adopts it only when every field that determines the rate still matches.
type resolvedTransactionRateEvidence struct {
	userID         string
	accountID      string
	fromCurrency   string
	baseCurrency   string
	occurredAtUnix int64
	rate           float64
	source         string
	rateAt         time.Time
}

func (e *resolvedTransactionRateEvidence) matches(userID, accountID, fromCurrency, baseCurrency string, occurredAt time.Time) bool {
	return e != nil &&
		e.userID == userID &&
		e.accountID == accountID &&
		e.fromCurrency == fromCurrency &&
		e.baseCurrency == baseCurrency &&
		e.occurredAtUnix == occurredAt.UTC().Unix() &&
		e.rate > 0 && !math.IsNaN(e.rate) && !math.IsInf(e.rate, 0) &&
		strings.TrimSpace(e.source) != "" && !e.rateAt.IsZero()
}

func normalizeTransactionCurrency(currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return "CNY"
	}
	return currency
}

func normalizeTransactionOccurredAt(occurredAt time.Time) time.Time {
	occurredAt = occurredAt.UTC().Truncate(time.Second)
	if occurredAt.IsZero() {
		return time.Now().UTC().Truncate(time.Second)
	}
	return occurredAt
}

func validateSourceTransactionAmount(amountCents int64) error {
	if amountCents <= 0 {
		return fmt.Errorf("金额必须为正数")
	}
	if err := model.ValidateTransactionAmountCents(amountCents); err != nil {
		return fmt.Errorf("金额超过单笔上限（%d 分）: %w", model.MaxTransactionAmountCents, err)
	}
	return nil
}

// PrepareCreateTransaction resolves the target account and every potentially
// remote exchange-rate lookup before callers enter their financial write
// transaction. CreateTransaction accepts the resulting private evidence only
// while the authoritative account snapshot still matches it.
func (s *TransactionService) PrepareCreateTransaction(ctx context.Context, req CreateTransactionRequest) (CreateTransactionRequest, error) {
	if req.AmountCents == 0 && req.AmountYuan != 0 {
		amountCents, err := req.AmountYuan.Cents()
		if err != nil {
			return CreateTransactionRequest{}, fmt.Errorf("金额格式无效: %w", err)
		}
		req.AmountCents = amountCents
	}
	if err := validateSourceTransactionAmount(req.AmountCents); err != nil {
		return CreateTransactionRequest{}, err
	}
	req.Currency = normalizeTransactionCurrency(req.Currency)
	req.OccurredAt = normalizeTransactionOccurredAt(req.OccurredAt)

	if strings.TrimSpace(req.AccountID) == "" {
		acctType := model.AccountTypePublic
		if req.Source == model.SourcePersonal {
			acctType = model.AccountTypePersonal
		}
		acct, err := s.accounts.GetByUserAndType(ctx, req.UserID, acctType)
		if err != nil {
			return CreateTransactionRequest{}, fmt.Errorf("解析默认账户失败: %w", err)
		}
		if acct.UserID != req.UserID {
			return CreateTransactionRequest{}, fmt.Errorf("未找到对应账户，请先在设置中创建")
		}
		if !acct.IsActive {
			return CreateTransactionRequest{}, fmt.Errorf("所选账户已停用")
		}
		req.AccountID = acct.ID
	}

	// A preparation is authoritative only for the snapshot it reads now; never
	// carry evidence supplied by an earlier attempt into a new preparation.
	req.resolvedRate = nil
	evidence, err := s.preResolveRateEvidence(ctx, req)
	if err != nil {
		return CreateTransactionRequest{}, err
	}
	req.resolvedRate = evidence
	return req, nil
}

// preResolveRateEvidence performs all potentially remote rate lookup before a
// caller acquires SQLite's IMMEDIATE write transaction.
func (s *TransactionService) preResolveRateEvidence(ctx context.Context, req CreateTransactionRequest) (*resolvedTransactionRateEvidence, error) {
	if s.accounts == nil || strings.TrimSpace(req.AccountID) == "" {
		return nil, fmt.Errorf("所选账户不存在")
	}
	acct, err := s.accounts.GetByID(ctx, req.AccountID)
	if err != nil {
		return nil, fmt.Errorf("读取所选账户失败: %w", err)
	}
	if acct.UserID != req.UserID {
		return nil, fmt.Errorf("所选账户不存在")
	}
	if !acct.IsActive {
		return nil, fmt.Errorf("所选账户已停用")
	}
	fromCurrency := normalizeTransactionCurrency(req.Currency)
	baseCurrency := normalizeTransactionCurrency(acct.Currency)
	occurredAt := normalizeTransactionOccurredAt(req.OccurredAt)
	rate, source, rateAt, err := s.resolveRate(ctx, req.UserID, fromCurrency, baseCurrency, occurredAt, req.ExchangeRate)
	if err != nil {
		return nil, err
	}
	if _, err := convertByRate(req.AmountCents, rate); err != nil {
		return nil, fmt.Errorf("汇率换算失败: %w", err)
	}
	evidence := &resolvedTransactionRateEvidence{
		userID: req.UserID, accountID: req.AccountID,
		fromCurrency: fromCurrency, baseCurrency: baseCurrency,
		occurredAtUnix: occurredAt.Unix(), rate: rate, source: source, rateAt: rateAt.UTC(),
	}
	if !evidence.matches(req.UserID, req.AccountID, fromCurrency, baseCurrency, occurredAt) {
		return nil, fmt.Errorf("汇率解析结果无效")
	}
	return evidence, nil
}

func (s *TransactionService) CreateTransaction(ctx context.Context, req CreateTransactionRequest) (model.Transaction, error) {
	if req.AmountCents == 0 && req.AmountYuan != 0 {
		amountCents, err := req.AmountYuan.Cents()
		if err != nil {
			return model.Transaction{}, fmt.Errorf("金额格式无效: %w", err)
		}
		req.AmountCents = amountCents
	}
	if err := validateSourceTransactionAmount(req.AmountCents); err != nil {
		return model.Transaction{}, err
	}
	req.Currency = normalizeTransactionCurrency(req.Currency)
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
	if txType == model.TxTypeTransfer {
		return model.Transaction{}, fmt.Errorf("转账需要同时指定转出和转入账户，当前接口暂不支持")
	}
	ledgerDir := model.LedgerCredit
	if txType == model.TxTypeExpense {
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
		if !acct.IsActive {
			return model.Transaction{}, fmt.Errorf("所选账户已停用")
		}
		accountID = acct.ID
		accountType = acct.Type
		accountCurrency = normalizeTransactionCurrency(acct.Currency)
	} else {
		acct, err := s.accounts.GetByID(ctx, accountID)
		if err != nil || acct.UserID != req.UserID {
			return model.Transaction{}, fmt.Errorf("所选账户不存在")
		}
		if !acct.IsActive {
			return model.Transaction{}, fmt.Errorf("所选账户已停用")
		}
		accountType = acct.Type
		accountCurrency = normalizeTransactionCurrency(acct.Currency)
	}

	reimb := model.ReimbStatusNone
	if req.Mode != model.ModeLife && txType == model.TxTypeExpense && accountType == model.AccountTypePersonal {
		reimb = model.ReimbStatusPending
	}
	if req.Category == "" {
		return model.Transaction{}, fmt.Errorf("请选择分类")
	}
	occurredAt := normalizeTransactionOccurredAt(req.OccurredAt)

	var rateValue float64
	var rateSource string
	var rateAt time.Time
	if req.resolvedRate.matches(req.UserID, accountID, req.Currency, accountCurrency, occurredAt) {
		rateValue = req.resolvedRate.rate
		rateSource = req.resolvedRate.source
		rateAt = req.resolvedRate.rateAt.UTC()
	} else if req.resolvedRate != nil {
		return model.Transaction{}, fmt.Errorf("%w: 预解析汇率证据与交易快照不一致", ErrResolvedRateEvidenceMismatch)
	} else {
		var err error
		rateValue, rateSource, rateAt, err = s.resolveRate(ctx, req.UserID, req.Currency, accountCurrency, occurredAt, req.ExchangeRate)
		if err != nil {
			return model.Transaction{}, err
		}
	}
	converted, err := convertByRate(req.AmountCents, rateValue)
	if err != nil {
		return model.Transaction{}, fmt.Errorf("汇率换算失败: %w", err)
	}

	now := time.Now()
	t := model.Transaction{
		ID:                      uuid.NewString(),
		UserID:                  req.UserID,
		GroupID:                 "",
		LedgerDir:               ledgerDir,
		TxType:                  txType,
		AccountID:               accountID,
		AccountType:             accountType,
		AmountCents:             req.AmountCents,
		AmountYuan:              model.Money(float64(req.AmountCents) / 100.0),
		Currency:                req.Currency,
		ExchangeRate:            rateValue,
		ExchangeRateSource:      rateSource,
		ExchangeRateAt:          rateAt.Unix(),
		BaseCurrency:            accountCurrency,
		BaseAmountCents:         converted,
		Category:                req.Category,
		ReimbStatus:             reimb,
		Mode:                    req.Mode,
		Note:                    req.Note,
		ProjectID:               req.ProjectID,
		AttachmentKey:           req.AttachmentKey,
		IdempotencyKey:          req.IdempotencyKey,
		RecurringRuleID:         req.RecurringRuleID,
		RecurringOccurrenceDate: req.RecurringOccurrenceDate,
		Uploaded:                false,
		TxnDate:                 occurredAt.Format("2006-01-02"),
		TransactionTime:         occurredAt.Unix(),
		OccurredAt:              occurredAt,
		Direction:               model.Direction(txType),
		Source:                  sourceFromAccountType(accountType),
		Reimbursed:              false,
		CreatedAt:               now,
		UpdatedAt:               now,
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
	converted, err := model.ConvertCentsByRate(amountCents, rate)
	if err != nil {
		return 0, err
	}
	if err := model.ValidateTransactionAmountCents(converted); err != nil {
		return 0, fmt.Errorf(
			"换算后的本位币金额超过单笔上限（%d 分）: %w",
			model.MaxTransactionAmountCents,
			err,
		)
	}
	return converted, nil
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
