package repository

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"finarch/internal/domain/model"
	findb "finarch/internal/infrastructure/db"
)

func openTransactionAmountRepositoryTestDB(t *testing.T) (context.Context, *sql.DB, *SQLiteTransactionRepository) {
	t.Helper()
	ctx := context.Background()
	database, err := findb.OpenSQLite(ctx, filepath.Join(t.TempDir(), "transaction-amount.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := findb.Migrate(ctx, database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := database.ExecContext(ctx, `
		INSERT INTO users (id, email, name, password_hash, role, created_at, updated_at)
		VALUES ('amount-user', 'amount@example.test', 'Amount User', 'x', 'user', ?, ?)
	`, now, now); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO accounts (
			id, user_id, name, type, currency, balance_cents, version,
			is_active, created_at, updated_at
		) VALUES (
			'amount-account', 'amount-user', 'Amount Account', 'personal', 'CNY',
			0, 0, 1, ?, ?
		)
	`, now, now); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	return ctx, database, NewSQLiteTransactionRepository(database)
}

func transactionAmountRepositoryModel(id string, amount, baseAmount int64) model.Transaction {
	return model.Transaction{
		ID:                 id,
		UserID:             "amount-user",
		GroupID:            id,
		AccountID:          "amount-account",
		LedgerDir:          model.LedgerCredit,
		TxType:             model.TxTypeIncome,
		AmountCents:        amount,
		Currency:           "CNY",
		ExchangeRate:       1,
		ExchangeRateSource: "repository-test",
		ExchangeRateAt:     1_700_000_000,
		BaseCurrency:       "CNY",
		BaseAmountCents:    baseAmount,
		Category:           "test",
		ReimbStatus:        model.ReimbStatusNone,
		Mode:               model.ModeWork,
		TxnDate:            "2026-01-01",
		TransactionTime:    1_700_000_000,
	}
}

func readTransactionAmountRepositoryState(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
) (transactionCount int, balance int64, version int64) {
	t.Helper()
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM transactions WHERE user_id = 'amount-user'
	`).Scan(&transactionCount); err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if err := database.QueryRowContext(ctx, `
		SELECT balance_cents, version FROM accounts WHERE id = 'amount-account'
	`).Scan(&balance, &version); err != nil {
		t.Fatalf("read account state: %v", err)
	}
	return transactionCount, balance, version
}

func TestSQLiteTransactionRepositoryCreateAllowsExactAmountMaximum(t *testing.T) {
	ctx, database, transactionRepository := openTransactionAmountRepositoryTestDB(t)
	transaction := transactionAmountRepositoryModel(
		"amount-at-maximum",
		model.MaxTransactionAmountCents,
		model.MaxTransactionAmountCents,
	)
	if err := transactionRepository.Create(ctx, transaction); err != nil {
		t.Fatalf("create exact maximum transaction: %v", err)
	}
	count, balance, version := readTransactionAmountRepositoryState(t, ctx, database)
	if count != 1 || balance != model.MaxTransactionAmountCents || version != 1 {
		t.Fatalf(
			"repository state after exact maximum = count %d, balance %d, version %d",
			count,
			balance,
			version,
		)
	}
}

func TestSQLiteTransactionRepositoryCreateRejectsOutOfRangeAmountsAtomically(t *testing.T) {
	tests := []struct {
		name       string
		amount     int64
		baseAmount int64
	}{
		{
			name:       "zero source amount",
			amount:     0,
			baseAmount: 1,
		},
		{
			name:       "negative source amount",
			amount:     -1,
			baseAmount: 1,
		},
		{
			name:       "source amount",
			amount:     model.MaxTransactionAmountCents + 1,
			baseAmount: 1,
		},
		{
			name:       "negative base amount",
			amount:     1,
			baseAmount: -1,
		},
		{
			name:       "base amount",
			amount:     1,
			baseAmount: model.MaxTransactionAmountCents + 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, database, transactionRepository := openTransactionAmountRepositoryTestDB(t)
			err := transactionRepository.Create(
				ctx,
				transactionAmountRepositoryModel("amount-out-of-range", tt.amount, tt.baseAmount),
			)
			if !errors.Is(err, model.ErrTransactionAmountOutOfRange) {
				t.Fatalf("Create error = %v, want ErrTransactionAmountOutOfRange", err)
			}
			count, balance, version := readTransactionAmountRepositoryState(t, ctx, database)
			if count != 0 || balance != 0 || version != 0 {
				t.Fatalf(
					"rejected create mutated state: count %d, balance %d, version %d",
					count,
					balance,
					version,
				)
			}
		})
	}
}
