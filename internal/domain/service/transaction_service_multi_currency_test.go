package service

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"finarch/internal/domain/model"
)

type fakeTxRepo struct {
	created      model.Transaction
	recentRate   float64
	recentAt     int64
	recentSource string
}

func (f *fakeTxRepo) Create(_ context.Context, t model.Transaction) error { f.created = t; return nil }
func (f *fakeTxRepo) GetByIDs(context.Context, []string) ([]model.Transaction, error) {
	return nil, nil
}
func (f *fakeTxRepo) ListByUser(context.Context, string, model.Mode) ([]model.Transaction, error) {
	return nil, nil
}
func (f *fakeTxRepo) ListUnreimbursedPersonalExpenses(context.Context, string, *string, int, model.Mode) ([]model.Transaction, error) {
	return nil, nil
}
func (f *fakeTxRepo) MarkReimbursed(context.Context, []string, string) error { return nil }
func (f *fakeTxRepo) ToggleReimbursed(context.Context, string, string) (bool, error) {
	return false, nil
}
func (f *fakeTxRepo) ToggleUploaded(context.Context, string, string) (bool, error) { return false, nil }
func (f *fakeTxRepo) SumPoolBalance(context.Context, string, model.Mode) (model.Money, model.Money, error) {
	return 0, 0, nil
}
func (f *fakeTxRepo) HasUnreimbursedByAccount(context.Context, string, string) (bool, error) {
	return false, nil
}
func (f *fakeTxRepo) GetRecentRate(context.Context, string, string, string) (float64, int64, string, error) {
	if f.recentRate <= 0 {
		return 0, 0, "", errors.New("not found")
	}
	return f.recentRate, f.recentAt, f.recentSource, nil
}

type fakeAcctRepo struct{ acct model.Account }

func (f fakeAcctRepo) Create(context.Context, model.Account) error            { return nil }
func (f fakeAcctRepo) GetByID(context.Context, string) (model.Account, error) { return f.acct, nil }
func (f fakeAcctRepo) ListByUser(context.Context, string) ([]model.Account, error) {
	return []model.Account{f.acct}, nil
}
func (f fakeAcctRepo) GetByUserAndType(context.Context, string, model.AccountType) (model.Account, error) {
	return f.acct, nil
}
func (f fakeAcctRepo) Update(context.Context, model.Account) error              { return nil }
func (f fakeAcctRepo) UpdateName(context.Context, string, string, string) error { return nil }
func (f fakeAcctRepo) Delete(context.Context, string, string) error             { return nil }
func (f fakeAcctRepo) CountByUserAndType(context.Context, string, model.AccountType) (int, error) {
	return 1, nil
}

type fakeRateSvc struct {
	result ExchangeRateResult
	err    error
}

func (f fakeRateSvc) GetRate(context.Context, string, string, time.Time) (ExchangeRateResult, error) {
	if f.err != nil {
		return ExchangeRateResult{}, f.err
	}
	return f.result, nil
}

func TestCreateTransaction_ConvertsToAccountBaseCurrency(t *testing.T) {
	txRepo := &fakeTxRepo{}
	acctRepo := fakeAcctRepo{acct: model.Account{ID: "a1", Type: model.AccountTypePublic, Currency: "CNY"}}
	rate := big.NewRat(76123, 10000) // 7.6123
	svc := NewTransactionService(txRepo, acctRepo, fakeRateSvc{result: ExchangeRateResult{Rate: rate, RateFloat: 7.6123, Source: "test", At: time.Unix(1700000000, 0)}})

	_, err := svc.CreateTransaction(context.Background(), CreateTransactionRequest{UserID: "u1", AccountID: "a1", TxType: model.TxTypeExpense, Category: "meal", Currency: "EUR", AmountCents: 1568, OccurredAt: time.Unix(1700000000, 0)})
	if err != nil {
		t.Fatalf("create transaction: %v", err)
	}
	if txRepo.created.BaseCurrency != "CNY" {
		t.Fatalf("expected base currency CNY got %s", txRepo.created.BaseCurrency)
	}
	if txRepo.created.BaseAmountCents != 11936 {
		t.Fatalf("unexpected converted cents %d", txRepo.created.BaseAmountCents)
	}
}

func TestCreateTransaction_FallbackToStoredRate(t *testing.T) {
	txRepo := &fakeTxRepo{recentRate: 8.0, recentAt: 1700000100, recentSource: "history"}
	acctRepo := fakeAcctRepo{acct: model.Account{ID: "a1", Type: model.AccountTypePublic, Currency: "CNY"}}
	svc := NewTransactionService(txRepo, acctRepo, fakeRateSvc{err: errors.New("api down")})

	_, err := svc.CreateTransaction(context.Background(), CreateTransactionRequest{UserID: "u1", AccountID: "a1", TxType: model.TxTypeExpense, Category: "meal", Currency: "EUR", AmountCents: 1568, OccurredAt: time.Unix(1700000000, 0)})
	if err != nil {
		t.Fatalf("create transaction with fallback: %v", err)
	}
	if txRepo.created.ExchangeRate != 8.0 {
		t.Fatalf("expected fallback rate 8 got %f", txRepo.created.ExchangeRate)
	}
	if txRepo.created.BaseAmountCents != 12544 {
		t.Fatalf("unexpected converted cents %d", txRepo.created.BaseAmountCents)
	}
}
