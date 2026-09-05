package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"finarch/internal/domain/model"
)

func TestTransactionServiceSourceAmountLimit(t *testing.T) {
	newService := func() (*TransactionService, *fakeTxRepo) {
		txRepo := &fakeTxRepo{}
		accountRepo := fakeAcctRepo{acct: model.Account{
			ID: "a1", UserID: "u1", Type: model.AccountTypePublic,
			Currency: "CNY", IsActive: true,
		}}
		return NewTransactionService(txRepo, accountRepo, nil), txRepo
	}
	request := func(amount int64) CreateTransactionRequest {
		return CreateTransactionRequest{
			UserID: "u1", AccountID: "a1", TxType: model.TxTypeIncome,
			Category: "limit", Currency: "CNY", AmountCents: amount,
			OccurredAt: time.Unix(1_700_000_000, 0).UTC(),
		}
	}

	t.Run("maximum is accepted by prepare and create", func(t *testing.T) {
		svc, txRepo := newService()
		prepared, err := svc.PrepareCreateTransaction(
			context.Background(),
			request(model.MaxTransactionAmountCents),
		)
		if err != nil {
			t.Fatalf("prepare maximum amount: %v", err)
		}
		created, err := svc.CreateTransaction(context.Background(), prepared)
		if err != nil {
			t.Fatalf("create maximum amount: %v", err)
		}
		if created.AmountCents != model.MaxTransactionAmountCents ||
			created.BaseAmountCents != model.MaxTransactionAmountCents ||
			txRepo.created.AmountCents != model.MaxTransactionAmountCents {
			t.Fatalf("maximum amount changed during creation: returned=%+v persisted=%+v", created, txRepo.created)
		}
	})

	t.Run("maximum plus one is rejected before persistence", func(t *testing.T) {
		for _, path := range []string{"prepare", "create"} {
			t.Run(path, func(t *testing.T) {
				svc, txRepo := newService()
				var err error
				if path == "prepare" {
					_, err = svc.PrepareCreateTransaction(
						context.Background(),
						request(model.MaxTransactionAmountCents+1),
					)
				} else {
					_, err = svc.CreateTransaction(
						context.Background(),
						request(model.MaxTransactionAmountCents+1),
					)
				}
				if !errors.Is(err, model.ErrTransactionAmountOutOfRange) {
					t.Fatalf("%s error = %v, want ErrTransactionAmountOutOfRange", path, err)
				}
				if txRepo.created.ID != "" {
					t.Fatalf("%s persisted an out-of-range transaction: %+v", path, txRepo.created)
				}
			})
		}
	})
}

func TestTransactionServiceConvertedBaseAmountLimit(t *testing.T) {
	newService := func() (*TransactionService, *fakeTxRepo) {
		txRepo := &fakeTxRepo{}
		accountRepo := fakeAcctRepo{acct: model.Account{
			ID: "a1", UserID: "u1", Type: model.AccountTypePublic,
			Currency: "CNY", IsActive: true,
		}}
		return NewTransactionService(txRepo, accountRepo, nil), txRepo
	}
	request := func(amount int64) CreateTransactionRequest {
		return CreateTransactionRequest{
			UserID: "u1", AccountID: "a1", TxType: model.TxTypeExpense,
			Category: "converted-limit", Currency: "USD", AmountCents: amount,
			ExchangeRate: 2, OccurredAt: time.Unix(1_700_000_000, 0).UTC(),
		}
	}

	t.Run("converted maximum is accepted", func(t *testing.T) {
		svc, txRepo := newService()
		prepared, err := svc.PrepareCreateTransaction(
			context.Background(),
			request(model.MaxTransactionAmountCents/2),
		)
		if err != nil {
			t.Fatalf("prepare converted maximum: %v", err)
		}
		created, err := svc.CreateTransaction(context.Background(), prepared)
		if err != nil {
			t.Fatalf("create converted maximum: %v", err)
		}
		if created.BaseAmountCents != model.MaxTransactionAmountCents ||
			txRepo.created.BaseAmountCents != model.MaxTransactionAmountCents {
			t.Fatalf("base amount = returned %d persisted %d, want %d",
				created.BaseAmountCents, txRepo.created.BaseAmountCents, model.MaxTransactionAmountCents)
		}
	})

	t.Run("converted maximum plus two is rejected", func(t *testing.T) {
		for _, path := range []string{"prepare", "create"} {
			t.Run(path, func(t *testing.T) {
				svc, txRepo := newService()
				var err error
				if path == "prepare" {
					_, err = svc.PrepareCreateTransaction(
						context.Background(),
						request(model.MaxTransactionAmountCents/2+1),
					)
				} else {
					_, err = svc.CreateTransaction(
						context.Background(),
						request(model.MaxTransactionAmountCents/2+1),
					)
				}
				if !errors.Is(err, model.ErrTransactionAmountOutOfRange) {
					t.Fatalf("%s error = %v, want ErrTransactionAmountOutOfRange", path, err)
				}
				if txRepo.created.ID != "" {
					t.Fatalf("%s persisted an out-of-range base amount: %+v", path, txRepo.created)
				}
			})
		}
	})
}
