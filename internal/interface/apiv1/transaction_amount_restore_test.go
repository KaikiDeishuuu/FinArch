package apiv1

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"finarch/internal/domain/model"
	findb "finarch/internal/infrastructure/db"
)

func insertRestoreAmountTestTransaction(t *testing.T, database *sql.DB, userID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := database.Exec(`
		INSERT INTO accounts (
			id, user_id, name, type, currency, balance_cents, version,
			is_active, created_at, updated_at
		) VALUES (
			'restore-amount-account', ?, 'Restore Amount Account', 'personal',
			'CNY', 0, 0, 1, ?, ?
		)
	`, userID, now, now); err != nil {
		t.Fatalf("insert source account: %v", err)
	}
	if _, err := database.Exec(`
		INSERT INTO transactions (
			id, user_id, group_id, direction, account_id,
			amount_cents, currency, exchange_rate, exchange_rate_source, exchange_rate_at,
			base_currency, base_amount_cents, type, category, reimb_status, mode, note,
			uploaded, txn_date, transaction_time, created_at, updated_at
		) VALUES (
			'restore-amount-transaction', ?, 'restore-amount-transaction', 'credit',
			'restore-amount-account', 1, 'CNY', 1, 'restore-test', 1700000000,
			'CNY', 1, 'income', 'restore-test', 'none', 'work', '',
			0, '2026-01-01', 1700000000, ?, ?
		)
	`, userID, now, now); err != nil {
		t.Fatalf("insert source transaction: %v", err)
	}
}

func corruptRestoreAmountTestTransaction(t *testing.T, database *sql.DB, column string) {
	t.Helper()
	if column != "amount_cents" && column != "base_amount_cents" {
		t.Fatalf("unsupported amount column %q", column)
	}
	if _, err := database.Exec(`DROP TRIGGER trg_transaction_amount_update`); err != nil {
		t.Fatalf("drop amount update trigger: %v", err)
	}
	query := `UPDATE transactions SET ` + column + ` = ? WHERE id = 'restore-amount-transaction'`
	if _, err := database.Exec(query, model.MaxTransactionAmountCents+1); err != nil {
		t.Fatalf("corrupt source %s: %v", column, err)
	}
}

func restoreAmountTargetCounts(t *testing.T, database *sql.DB) (accounts, transactions int) {
	t.Helper()
	if err := database.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&accounts); err != nil {
		t.Fatalf("count target accounts: %v", err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&transactions); err != nil {
		t.Fatalf("count target transactions: %v", err)
	}
	return accounts, transactions
}

func requireNoRestoreAmountArtifacts(t *testing.T, baseDir string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(baseDir, "safety_backups"))
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatalf("read restore safety directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("amount preflight left %d restore artifact(s)", len(entries))
	}
}

func TestReplacementRestoreRejectsOutOfRangeTransactionAmountsBeforeTargetWrites(t *testing.T) {
	for _, column := range []string{"amount_cents", "base_amount_cents"} {
		t.Run(column, func(t *testing.T) {
			baseDir := t.TempDir()
			livePath := filepath.Join(baseDir, "live.db")
			liveDB := openRestoreTestDatabase(t, livePath, "original", true)
			defer liveDB.Close()
			beforeAccounts, beforeTransactions := restoreAmountTargetCounts(t, liveDB)

			sourcePath := filepath.Join(baseDir, "source.db")
			sourceDB := openRestoreTestDatabase(t, sourcePath, "replacement", true)
			insertRestoreAmountTestTransaction(t, sourceDB, "restore-test-user")
			corruptRestoreAmountTestTransaction(t, sourceDB, column)
			uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
			if err := sourceDB.Close(); err != nil {
				t.Fatalf("close source database: %v", err)
			}

			server := &Server{db: liveDB, dbPath: livePath}
			_, err := server.executeRestoreEngine(context.Background(), restoreEngineRequest{
				TempDBPath:      sourcePath,
				UploadedVersion: uploadedVersion,
				SchemaBefore:    restoreTestSchemaVersion(t, liveDB),
			})
			if !errors.Is(err, model.ErrTransactionAmountOutOfRange) {
				t.Fatalf("replacement restore error = %v, want ErrTransactionAmountOutOfRange", err)
			}

			assertRestoreTestMarker(t, liveDB, "original")
			afterAccounts, afterTransactions := restoreAmountTargetCounts(t, liveDB)
			if afterAccounts != beforeAccounts || afterTransactions != beforeTransactions {
				t.Fatalf(
					"rejected replacement mutated target counts: accounts %d->%d, transactions %d->%d",
					beforeAccounts,
					afterAccounts,
					beforeTransactions,
					afterTransactions,
				)
			}
			requireNoRestoreAmountArtifacts(t, baseDir)
			if state := findb.Global().State(); state != findb.StateNormal {
				t.Fatalf("global guard state = %v, want normal", state)
			}
		})
	}
}

func TestCrossAccountRestoreRejectsOutOfRangeTransactionAmountsBeforeTargetWrites(t *testing.T) {
	for _, column := range []string{"amount_cents", "base_amount_cents"} {
		t.Run(column, func(t *testing.T) {
			baseDir := t.TempDir()
			livePath := filepath.Join(baseDir, "live.db")
			liveDB := openRestoreTestDatabase(t, livePath, "target", true)
			defer liveDB.Close()
			beforeAccounts, beforeTransactions := restoreAmountTargetCounts(t, liveDB)

			sourceUserID := "restore-amount-source-user"
			sourcePath := filepath.Join(baseDir, "source.db")
			sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
			insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)
			insertRestoreAmountTestTransaction(t, sourceDB, sourceUserID)
			corruptRestoreAmountTestTransaction(t, sourceDB, column)
			uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
			if err := sourceDB.Close(); err != nil {
				t.Fatalf("close source database: %v", err)
			}

			server := newCrossAccountRestoreTestServer(t, liveDB, livePath, nil)
			_, err := server.performCrossAccountMergeRestore(
				context.Background(),
				sourcePath,
				uploadedVersion,
				restoreTestSchemaVersion(t, liveDB),
				restoreContext{
					RequesterUserID: "restore-test-user",
					BackupUserID:    sourceUserID,
					RestoreMode:     "CROSS_ACCOUNT",
					DataScope:       "both",
				},
			)
			if !errors.Is(err, model.ErrTransactionAmountOutOfRange) {
				t.Fatalf("cross-account restore error = %v, want ErrTransactionAmountOutOfRange", err)
			}

			assertRestoreTestMarker(t, liveDB, "target")
			afterAccounts, afterTransactions := restoreAmountTargetCounts(t, liveDB)
			if afterAccounts != beforeAccounts || afterTransactions != beforeTransactions {
				t.Fatalf(
					"rejected cross-account restore mutated target counts: accounts %d->%d, transactions %d->%d",
					beforeAccounts,
					afterAccounts,
					beforeTransactions,
					afterTransactions,
				)
			}
			requireNoRestoreAmountArtifacts(t, baseDir)
			if state := findb.Global().State(); state != findb.StateNormal {
				t.Fatalf("global guard state = %v, want normal", state)
			}
		})
	}
}

func TestValidateRestoreTransactionAmountsSupportsLegacyYuanSchema(t *testing.T) {
	ctx := context.Background()
	database, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer database.Close()
	if _, err := database.Exec(`
		CREATE TABLE transactions (
			id TEXT PRIMARY KEY,
			amount_yuan REAL NOT NULL
		)
	`); err != nil {
		t.Fatalf("create legacy transactions: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO transactions (id, amount_yuan) VALUES ('valid', ?)`,
		float64(model.MaxTransactionAmountCents)/100,
	); err != nil {
		t.Fatalf("insert valid legacy amount: %v", err)
	}
	if err := validateRestoreTransactionAmounts(ctx, database); err != nil {
		t.Fatalf("exact legacy maximum rejected: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO transactions (id, amount_yuan) VALUES ('invalid', ?)`,
		float64(model.MaxTransactionAmountCents)/100+1,
	); err != nil {
		t.Fatalf("insert invalid legacy amount: %v", err)
	}
	if err := validateRestoreTransactionAmounts(ctx, database); !errors.Is(err, model.ErrTransactionAmountOutOfRange) {
		t.Fatalf("legacy out-of-range error = %v, want ErrTransactionAmountOutOfRange", err)
	}
}
