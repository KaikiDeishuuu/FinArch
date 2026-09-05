package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"finarch/internal/domain/model"
)

func TestCreateTransactionRejectsInactiveAccount(t *testing.T) {
	txRepo := &fakeTxRepo{}
	accountRepo := fakeAcctRepo{acct: model.Account{
		ID: "inactive", UserID: "u1", Type: model.AccountTypePublic,
		Currency: "CNY", IsActive: false,
	}}
	svc := NewTransactionService(txRepo, accountRepo, nil)
	_, err := svc.CreateTransaction(context.Background(), CreateTransactionRequest{
		UserID: "u1", AccountID: "inactive", TxType: model.TxTypeExpense,
		AmountCents: 100, Currency: "CNY", Category: "test",
	})
	if err == nil || !strings.Contains(err.Error(), "停用") {
		t.Fatalf("expected inactive-account error, got %v", err)
	}
	if txRepo.created.ID != "" {
		t.Fatal("inactive account transaction was persisted")
	}
}

func TestCreateTransactionRoundsLegacyYuanToCents(t *testing.T) {
	txRepo := &fakeTxRepo{}
	accountRepo := fakeAcctRepo{acct: model.Account{
		ID: "a1", UserID: "u1", Type: model.AccountTypePublic,
		Currency: "CNY", IsActive: true,
	}}
	svc := NewTransactionService(txRepo, accountRepo, nil)
	tx, err := svc.CreateTransaction(context.Background(), CreateTransactionRequest{
		UserID: "u1", AccountID: "a1", TxType: model.TxTypeExpense,
		AmountYuan: 0.29, Currency: "CNY", Category: "rounding",
	})
	if err != nil {
		t.Fatalf("create 0.29 transaction: %v", err)
	}
	if tx.AmountCents != 29 || txRepo.created.AmountCents != 29 {
		t.Fatalf("amount cents = returned %d, persisted %d; want 29", tx.AmountCents, txRepo.created.AmountCents)
	}
}

func TestCreateTransactionRejectsInvalidLegacyYuan(t *testing.T) {
	accountRepo := fakeAcctRepo{acct: model.Account{
		ID: "a1", UserID: "u1", Type: model.AccountTypePublic,
		Currency: "CNY", IsActive: true,
	}}
	for _, amount := range []model.Money{-0.29, model.Money(math.NaN()), model.Money(math.Inf(1)), model.Money(math.MaxFloat64)} {
		t.Run(fmt.Sprint(float64(amount)), func(t *testing.T) {
			txRepo := &fakeTxRepo{}
			svc := NewTransactionService(txRepo, accountRepo, nil)
			_, err := svc.CreateTransaction(context.Background(), CreateTransactionRequest{
				UserID: "u1", AccountID: "a1", TxType: model.TxTypeExpense,
				AmountYuan: amount, Currency: "CNY", Category: "invalid",
			})
			if err == nil {
				t.Fatalf("amount %v unexpectedly accepted", amount)
			}
			if txRepo.created.ID != "" {
				t.Fatal("invalid transaction was persisted")
			}
		})
	}
}

func TestCreateTransactionRejectsSingleLegTransfer(t *testing.T) {
	txRepo := &fakeTxRepo{}
	accountRepo := fakeAcctRepo{acct: model.Account{
		ID: "a1", UserID: "u1", Type: model.AccountTypePublic,
		Currency: "CNY", IsActive: true,
	}}
	svc := NewTransactionService(txRepo, accountRepo, nil)
	_, err := svc.CreateTransaction(context.Background(), CreateTransactionRequest{
		UserID: "u1", AccountID: "a1", TxType: model.TxTypeTransfer,
		AmountCents: 100, Currency: "CNY", Category: "transfer",
	})
	if err == nil || !strings.Contains(err.Error(), "同时指定转出和转入账户") {
		t.Fatalf("expected unsupported transfer error, got %v", err)
	}
	if txRepo.created.ID != "" {
		t.Fatal("single-leg transfer was persisted")
	}
}
