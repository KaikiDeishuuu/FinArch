package db

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"finarch/internal/domain/model"
)

const balanceOverflowMessage = "account_balance_overflow"
const transactionAmountMessage = "transaction_amount_out_of_range"

func openBalanceOverflowTestDB(t *testing.T) (context.Context, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	database, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "finarch.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := database.ExecContext(ctx, `
		INSERT INTO users (id,email,name,password_hash,role,created_at,updated_at)
		VALUES ('overflow-user','overflow@example.com','overflow','x','user',?,?)`,
		now, now,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return ctx, database
}

func addBalanceOverflowTestAccount(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	id string,
	balance int64,
	version int64,
) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := database.ExecContext(ctx, `
		INSERT INTO accounts (
			id,user_id,name,type,currency,balance_cents,version,is_active,created_at,updated_at
		) VALUES (?, 'overflow-user', ?, 'public', 'CNY', ?, ?, 1, ?, ?)`,
		id, id, balance, version, now, now,
	); err != nil {
		t.Fatalf("insert account %s: %v", id, err)
	}
}

func insertBalanceOverflowTestTransaction(
	ctx context.Context,
	database *sql.DB,
	id string,
	accountID string,
	direction string,
	baseAmount any,
	reimbStatus string,
) error {
	return insertBalanceOverflowTestTransactionAmounts(
		ctx, database, id, accountID, direction, int64(1), baseAmount, reimbStatus,
	)
}

func insertBalanceOverflowTestTransactionAmounts(
	ctx context.Context,
	database *sql.DB,
	id string,
	accountID string,
	direction string,
	amount any,
	baseAmount any,
	reimbStatus string,
) error {
	txType := "income"
	if direction == "debit" {
		txType = "expense"
	}
	if reimbStatus == "" {
		reimbStatus = "none"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := database.ExecContext(ctx, `
		INSERT INTO transactions (
			id,user_id,group_id,direction,account_id,
			amount_cents,currency,exchange_rate,exchange_rate_source,exchange_rate_at,
			base_currency,base_amount_cents,type,category,reimb_status,mode,note,
			uploaded,txn_date,transaction_time,created_at,updated_at
		) VALUES (
			?, 'overflow-user', ?, ?, ?,
			?, 'CNY', 1, 'overflow-test', 1700000000,
			'CNY', ?, ?, 'overflow', ?, 'work', '',
			0, '2026-01-01', 1700000000, ?, ?
		)`,
		id, id, direction, accountID, amount, baseAmount, txType, reimbStatus, now, now,
	)
	return err
}

type balanceOverflowAccountState struct {
	balance     any
	balanceType string
	version     any
	versionType string
}

func readBalanceOverflowAccountState(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	accountID string,
) balanceOverflowAccountState {
	t.Helper()
	var state balanceOverflowAccountState
	if err := database.QueryRowContext(ctx, `
		SELECT balance_cents, typeof(balance_cents), version, typeof(version)
		FROM accounts
		WHERE id = ?`,
		accountID,
	).Scan(&state.balance, &state.balanceType, &state.version, &state.versionType); err != nil {
		t.Fatalf("read account %s: %v", accountID, err)
	}
	return state
}

func requireBalanceOverflowIntegerState(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	accountID string,
	wantBalance int64,
	wantVersion int64,
) {
	t.Helper()
	state := readBalanceOverflowAccountState(t, ctx, database, accountID)
	gotBalance, balanceOK := state.balance.(int64)
	gotVersion, versionOK := state.version.(int64)
	if !balanceOK || state.balanceType != "integer" || gotBalance != wantBalance {
		t.Fatalf(
			"account %s balance = %#v typeof=%s, want %d/integer",
			accountID, state.balance, state.balanceType, wantBalance,
		)
	}
	if !versionOK || state.versionType != "integer" || gotVersion != wantVersion {
		t.Fatalf(
			"account %s version = %#v typeof=%s, want %d/integer",
			accountID, state.version, state.versionType, wantVersion,
		)
	}
}

func requireBalanceOverflowError(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), balanceOverflowMessage) {
		t.Fatalf("error = %v, want %q", err, balanceOverflowMessage)
	}
}

func requireTransactionAmountError(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), transactionAmountMessage) {
		t.Fatalf("error = %v, want %q", err, transactionAmountMessage)
	}
}

func requireBalanceOverflowTransactionCount(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	transactionID string,
	want int,
) {
	t.Helper()
	var got int
	if err := database.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM transactions WHERE id = ?`,
		transactionID,
	).Scan(&got); err != nil {
		t.Fatalf("count transaction %s: %v", transactionID, err)
	}
	if got != want {
		t.Fatalf("transaction %s count = %d, want %d", transactionID, got, want)
	}
}

func TestTransactionAmountInsertTriggerEnforcesDomainRangeAndIntegerStorage(t *testing.T) {
	t.Run("exact maximum", func(t *testing.T) {
		ctx, database := openBalanceOverflowTestDB(t)
		addBalanceOverflowTestAccount(t, ctx, database, "amount-max-account", 0, 0)
		if err := insertBalanceOverflowTestTransactionAmounts(
			ctx,
			database,
			"amount-max-transaction",
			"amount-max-account",
			"credit",
			model.MaxTransactionAmountCents,
			model.MaxTransactionAmountCents,
			"none",
		); err != nil {
			t.Fatalf("insert exact maximum: %v", err)
		}
		requireBalanceOverflowTransactionCount(t, ctx, database, "amount-max-transaction", 1)
		requireBalanceOverflowIntegerState(
			t, ctx, database, "amount-max-account", model.MaxTransactionAmountCents, 1,
		)
	})

	tests := []struct {
		name       string
		amount     any
		baseAmount any
	}{
		{name: "source above maximum", amount: model.MaxTransactionAmountCents + 1, baseAmount: int64(1)},
		{name: "base above maximum", amount: int64(1), baseAmount: model.MaxTransactionAmountCents + 1},
		{name: "source real", amount: float64(0.5), baseAmount: int64(1)},
		{name: "base real", amount: int64(1), baseAmount: float64(0.5)},
		{name: "source zero", amount: int64(0), baseAmount: int64(1)},
		{name: "base zero", amount: int64(1), baseAmount: int64(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, database := openBalanceOverflowTestDB(t)
			addBalanceOverflowTestAccount(t, ctx, database, "invalid-insert-account", 0, 0)
			err := insertBalanceOverflowTestTransactionAmounts(
				ctx,
				database,
				"invalid-insert-transaction",
				"invalid-insert-account",
				"credit",
				tt.amount,
				tt.baseAmount,
				"none",
			)
			requireTransactionAmountError(t, err)
			requireBalanceOverflowTransactionCount(t, ctx, database, "invalid-insert-transaction", 0)
			requireBalanceOverflowIntegerState(t, ctx, database, "invalid-insert-account", 0, 0)
		})
	}
}

func TestTransactionAmountUpdateTriggerIsAtomic(t *testing.T) {
	t.Run("exact maximum", func(t *testing.T) {
		ctx, database := openBalanceOverflowTestDB(t)
		addBalanceOverflowTestAccount(t, ctx, database, "update-max-account", 0, 0)
		if err := insertBalanceOverflowTestTransactionAmounts(
			ctx, database, "update-max-transaction", "update-max-account", "credit", int64(1), int64(1), "none",
		); err != nil {
			t.Fatalf("seed transaction: %v", err)
		}
		if _, err := database.ExecContext(ctx, `
			UPDATE transactions
			SET amount_cents = ?, base_amount_cents = ?
			WHERE id = 'update-max-transaction'
		`, model.MaxTransactionAmountCents, model.MaxTransactionAmountCents); err != nil {
			t.Fatalf("update to exact maximum: %v", err)
		}
		requireBalanceOverflowIntegerState(
			t, ctx, database, "update-max-account", model.MaxTransactionAmountCents, 3,
		)
	})

	tests := []struct {
		name       string
		amount     any
		baseAmount any
	}{
		{name: "source above maximum", amount: model.MaxTransactionAmountCents + 1, baseAmount: int64(1)},
		{name: "base above maximum", amount: int64(1), baseAmount: model.MaxTransactionAmountCents + 1},
		{name: "source real", amount: float64(0.5), baseAmount: int64(1)},
		{name: "base real", amount: int64(1), baseAmount: float64(0.5)},
		{name: "source zero", amount: int64(0), baseAmount: int64(1)},
		{name: "base zero", amount: int64(1), baseAmount: int64(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, database := openBalanceOverflowTestDB(t)
			addBalanceOverflowTestAccount(t, ctx, database, "invalid-update-account", 0, 0)
			if err := insertBalanceOverflowTestTransactionAmounts(
				ctx, database, "invalid-update-transaction", "invalid-update-account", "credit", int64(1), int64(1), "none",
			); err != nil {
				t.Fatalf("seed transaction: %v", err)
			}
			_, err := database.ExecContext(ctx, `
				UPDATE transactions
				SET amount_cents = ?, base_amount_cents = ?
				WHERE id = 'invalid-update-transaction'
			`, tt.amount, tt.baseAmount)
			requireTransactionAmountError(t, err)
			requireBalanceOverflowIntegerState(t, ctx, database, "invalid-update-account", 1, 1)

			var amount, baseAmount int64
			if err := database.QueryRowContext(ctx, `
				SELECT amount_cents, base_amount_cents
				FROM transactions
				WHERE id = 'invalid-update-transaction'
			`).Scan(&amount, &baseAmount); err != nil {
				t.Fatalf("read transaction after rejected update: %v", err)
			}
			if amount != 1 || baseAmount != 1 {
				t.Fatalf("rejected update persisted amounts %d/%d", amount, baseAmount)
			}
		})
	}
}

func TestBalanceInsertTriggerProtectsInt64Boundaries(t *testing.T) {
	ctx, database := openBalanceOverflowTestDB(t)

	addBalanceOverflowTestAccount(t, ctx, database, "credit-boundary", math.MaxInt64-10, 0)
	if err := insertBalanceOverflowTestTransaction(
		ctx, database, "credit-exact", "credit-boundary", "credit", int64(10), "none",
	); err != nil {
		t.Fatalf("insert exact credit boundary: %v", err)
	}
	requireBalanceOverflowIntegerState(
		t, ctx, database, "credit-boundary", math.MaxInt64, 1,
	)
	err := insertBalanceOverflowTestTransaction(
		ctx, database, "credit-overflow", "credit-boundary", "credit", int64(1), "none",
	)
	requireBalanceOverflowError(t, err)
	requireBalanceOverflowTransactionCount(t, ctx, database, "credit-overflow", 0)
	requireBalanceOverflowIntegerState(
		t, ctx, database, "credit-boundary", math.MaxInt64, 1,
	)

	addBalanceOverflowTestAccount(t, ctx, database, "debit-boundary", math.MinInt64+10, 0)
	if err := insertBalanceOverflowTestTransaction(
		ctx, database, "debit-exact", "debit-boundary", "debit", int64(10), "none",
	); err != nil {
		t.Fatalf("insert exact debit boundary: %v", err)
	}
	requireBalanceOverflowIntegerState(
		t, ctx, database, "debit-boundary", math.MinInt64, 1,
	)
	err = insertBalanceOverflowTestTransaction(
		ctx, database, "debit-overflow", "debit-boundary", "debit", int64(1), "none",
	)
	requireBalanceOverflowError(t, err)
	requireBalanceOverflowTransactionCount(t, ctx, database, "debit-overflow", 0)
	requireBalanceOverflowIntegerState(
		t, ctx, database, "debit-boundary", math.MinInt64, 1,
	)
}

func TestBalanceInsertTriggerRejectsNonIntegerOperands(t *testing.T) {
	ctx, database := openBalanceOverflowTestDB(t)

	addBalanceOverflowTestAccount(t, ctx, database, "real-balance", 0, 0)
	if _, err := database.ExecContext(
		ctx,
		`UPDATE accounts SET balance_cents = CAST(0.5 AS REAL) WHERE id = 'real-balance'`,
	); err != nil {
		t.Fatalf("seed REAL balance: %v", err)
	}
	err := insertBalanceOverflowTestTransaction(
		ctx, database, "real-balance-tx", "real-balance", "credit", int64(1), "none",
	)
	requireBalanceOverflowError(t, err)
	requireBalanceOverflowTransactionCount(t, ctx, database, "real-balance-tx", 0)
	state := readBalanceOverflowAccountState(t, ctx, database, "real-balance")
	if state.balanceType != "real" {
		t.Fatalf("pre-existing REAL balance type changed after abort: %s", state.balanceType)
	}

	addBalanceOverflowTestAccount(t, ctx, database, "real-amount", 0, 0)
	err = insertBalanceOverflowTestTransaction(
		ctx, database, "real-amount-tx", "real-amount", "credit", float64(0.5), "none",
	)
	requireTransactionAmountError(t, err)
	requireBalanceOverflowTransactionCount(t, ctx, database, "real-amount-tx", 0)
	requireBalanceOverflowIntegerState(t, ctx, database, "real-amount", 0, 0)
}

func TestBalanceDeleteTriggerRollsBackOverflow(t *testing.T) {
	tests := []struct {
		name            string
		direction       string
		overflowBalance int64
	}{
		{name: "credit reversal underflow", direction: "credit", overflowBalance: math.MinInt64 + 4},
		{name: "debit reversal overflow", direction: "debit", overflowBalance: math.MaxInt64 - 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, database := openBalanceOverflowTestDB(t)
			addBalanceOverflowTestAccount(t, ctx, database, "delete-account", 0, 0)
			if err := insertBalanceOverflowTestTransaction(
				ctx, database, "delete-tx", "delete-account", tt.direction, int64(5), "none",
			); err != nil {
				t.Fatalf("seed transaction: %v", err)
			}
			if _, err := database.ExecContext(
				ctx,
				`UPDATE accounts SET balance_cents = ?, version = 7 WHERE id = 'delete-account'`,
				tt.overflowBalance,
			); err != nil {
				t.Fatalf("seed boundary balance: %v", err)
			}

			_, err := database.ExecContext(ctx, `DELETE FROM transactions WHERE id = 'delete-tx'`)
			requireBalanceOverflowError(t, err)
			requireBalanceOverflowTransactionCount(t, ctx, database, "delete-tx", 1)
			requireBalanceOverflowIntegerState(
				t, ctx, database, "delete-account", tt.overflowBalance, 7,
			)
			var auditDeletes int
			if err := database.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM audit_log
				WHERE table_name = 'transactions' AND row_id = 'delete-tx' AND action = 'DELETE'
			`).Scan(&auditDeletes); err != nil {
				t.Fatalf("count delete audits: %v", err)
			}
			if auditDeletes != 0 {
				t.Fatalf("failed delete left %d audit rows", auditDeletes)
			}
		})
	}
}

func TestBalanceUpdateTriggerRollsBackBothAccounts(t *testing.T) {
	ctx, database := openBalanceOverflowTestDB(t)
	addBalanceOverflowTestAccount(t, ctx, database, "update-old", 0, 0)
	addBalanceOverflowTestAccount(t, ctx, database, "update-new", 0, 0)
	if err := insertBalanceOverflowTestTransaction(
		ctx, database, "update-tx", "update-old", "debit", int64(10), "none",
	); err != nil {
		t.Fatalf("seed transaction: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE accounts
		SET balance_cents = ?, version = 3
		WHERE id = 'update-new'`,
		math.MaxInt64,
	); err != nil {
		t.Fatalf("seed target boundary: %v", err)
	}

	_, err := database.ExecContext(ctx, `
		UPDATE transactions
		SET account_id = 'update-new',
		    direction = 'credit',
		    amount_cents = 1,
		    base_amount_cents = 1,
		    type = 'income'
		WHERE id = 'update-tx'`)
	requireBalanceOverflowError(t, err)

	requireBalanceOverflowIntegerState(t, ctx, database, "update-old", -10, 1)
	requireBalanceOverflowIntegerState(t, ctx, database, "update-new", math.MaxInt64, 3)
	var accountID, direction string
	var baseAmount int64
	if err := database.QueryRowContext(ctx, `
		SELECT account_id, direction, base_amount_cents
		FROM transactions
		WHERE id = 'update-tx'`,
	).Scan(&accountID, &direction, &baseAmount); err != nil {
		t.Fatalf("read rolled-back transaction: %v", err)
	}
	if accountID != "update-old" || direction != "debit" || baseAmount != 10 {
		t.Fatalf(
			"financial update was partially applied: account=%s direction=%s base=%d",
			accountID, direction, baseAmount,
		)
	}
	var auditUpdates int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM audit_log
		WHERE table_name = 'transactions' AND row_id = 'update-tx' AND action = 'UPDATE'
	`).Scan(&auditUpdates); err != nil {
		t.Fatalf("count update audits: %v", err)
	}
	if auditUpdates != 0 {
		t.Fatalf("failed financial update left %d audit rows", auditUpdates)
	}
}

func TestReimbursementStatusDoesNotMutateAccountBalance(t *testing.T) {
	ctx, database := openBalanceOverflowTestDB(t)
	addBalanceOverflowTestAccount(t, ctx, database, "reimburse-account", 0, 0)
	if err := insertBalanceOverflowTestTransaction(
		ctx, database, "reimburse-tx", "reimburse-account", "debit", int64(5), "pending",
	); err != nil {
		t.Fatalf("seed pending reimbursement: %v", err)
	}
	requireBalanceOverflowIntegerState(t, ctx, database, "reimburse-account", -5, 1)

	for _, status := range []string{"reimbursed", "pending"} {
		if _, err := database.ExecContext(ctx, `
			UPDATE transactions SET reimb_status = ? WHERE id = 'reimburse-tx'`, status,
		); err != nil {
			t.Fatalf("set reimbursement status %s: %v", status, err)
		}
		requireBalanceOverflowIntegerState(t, ctx, database, "reimburse-account", -5, 1)
	}

	for _, trigger := range []string{"trg_balance_reimburse", "trg_balance_unreimburse"} {
		var count int
		if err := database.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name = ?`, trigger,
		).Scan(&count); err != nil {
			t.Fatalf("inspect trigger %s: %v", trigger, err)
		}
		if count != 0 {
			t.Fatalf("retired trigger %s is still installed", trigger)
		}
	}
}
