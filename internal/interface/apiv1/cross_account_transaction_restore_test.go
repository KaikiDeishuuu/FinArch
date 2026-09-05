package apiv1

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
	sqliterepo "finarch/internal/infrastructure/repository"
)

type restoredLifecycleTimes struct {
	transaction int64
	reported    int64
	reimbursed  int64
}

func TestCrossAccountMergeKeepsLegitimateDuplicateTransactionsAndExactTimes(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	sourceUserID := "duplicate-transaction-source"
	targetUserID := "restore-test-user"
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
	insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)

	sourceTxRepo := sqliterepo.NewSQLiteTransactionRepository(sourceDB)
	sourceAccountRepo := sqliterepo.NewSQLiteAccountRepository(sourceDB)
	sourceTM := sqliterepo.NewSQLiteTransactionManager(sourceDB)
	sourceAccountSvc := service.NewAccountService(sourceAccountRepo, sourceTxRepo, sourceTM)
	sourceAccount, err := sourceAccountSvc.CreateAccount(
		context.Background(), sourceUserID, "Duplicate Source",
		model.AccountTypePublic, "CNY", model.ModeWork,
	)
	if err != nil {
		t.Fatal(err)
	}
	sourceTxSvc := service.NewTransactionService(sourceTxRepo, sourceAccountRepo, nil)
	request := service.CreateTransactionRequest{
		UserID: sourceUserID, Mode: model.ModeWork, AccountID: sourceAccount.ID,
		TxType: model.TxTypeExpense, Category: "same-business-event", AmountCents: 9_876,
		Currency: "CNY", OccurredAt: time.Date(2026, 7, 8, 9, 10, 11, 0, time.UTC),
		Note: "legitimate duplicate",
	}
	first, err := sourceTxSvc.CreateTransaction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := sourceTxSvc.CreateTransaction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]restoredLifecycleTimes{
		first.ID:  {transaction: 1_783_502_411, reported: 1_783_502_500, reimbursed: 1_783_502_600},
		second.ID: {transaction: 1_783_502_412, reported: 1_783_502_501, reimbursed: 1_783_502_601},
	}
	for sourceID, times := range expected {
		if _, err := sourceDB.Exec(`
			UPDATE transactions
			SET transaction_time = ?, reported_at = ?, reimbursed_at = ?
			WHERE id = ? AND user_id = ?
		`, times.transaction, times.reported, times.reimbursed, sourceID, sourceUserID); err != nil {
			t.Fatal(err)
		}
	}

	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	targetTxRepo := sqliterepo.NewSQLiteTransactionRepository(liveDB)
	targetAccountRepo := sqliterepo.NewSQLiteAccountRepository(liveDB)
	targetTM := sqliterepo.NewSQLiteTransactionManager(liveDB)
	s := &Server{
		db: liveDB, dbPath: livePath,
		acctSvc: service.NewAccountService(targetAccountRepo, targetTxRepo, targetTM),
	}
	restore := func() {
		t.Helper()
		if _, err := s.performCrossAccountMergeRestore(
			context.Background(), sourcePath, uploadedVersion, restoreTestSchemaVersion(t, liveDB),
			restoreContext{
				RequesterUserID: targetUserID,
				BackupUserID:    sourceUserID,
				RestoreMode:     "CROSS_ACCOUNT",
				DataScope:       "both",
			},
		); err != nil {
			t.Fatalf("cross-account restore: %v", err)
		}
	}

	restore()
	assertRestoredDuplicateTransactionTimes(t, liveDB, sourceUserID, targetUserID, expected)
	restore()
	assertRestoredDuplicateTransactionTimes(t, liveDB, sourceUserID, targetUserID, expected)
}

func assertRestoredDuplicateTransactionTimes(
	t *testing.T,
	database *sql.DB,
	sourceUserID, targetUserID string,
	expected map[string]restoredLifecycleTimes,
) {
	t.Helper()
	var count, distinctHashes int
	if err := database.QueryRow(`
		SELECT COUNT(*), COUNT(DISTINCT restore_txn_hash)
		FROM transactions
		WHERE user_id = ? AND restore_source_backup_id IS NOT NULL
	`, targetUserID).Scan(&count, &distinctHashes); err != nil {
		t.Fatal(err)
	}
	if count != len(expected) {
		t.Fatalf("restored transaction count=%d, want %d", count, len(expected))
	}
	if distinctHashes != 1 {
		t.Fatalf("business-identical source rows produced %d restore hashes, want 1", distinctHashes)
	}

	for sourceID, want := range expected {
		targetID := deterministicRestoreID("transaction", sourceUserID, targetUserID, sourceID)
		var transactionTime sql.NullInt64
		var reportedAt, reimbursedAt sql.NullInt64
		if err := database.QueryRow(`
			SELECT transaction_time, reported_at, reimbursed_at
			FROM transactions
			WHERE id = ? AND user_id = ?
		`, targetID, targetUserID).Scan(&transactionTime, &reportedAt, &reimbursedAt); err != nil {
			t.Fatalf("read restored transaction %s: %v", sourceID, err)
		}
		if !transactionTime.Valid || transactionTime.Int64 != want.transaction ||
			!reportedAt.Valid || reportedAt.Int64 != want.reported ||
			!reimbursedAt.Valid || reimbursedAt.Int64 != want.reimbursed {
			t.Fatalf(
				"restored lifecycle times for %s=(%v,%v,%v), want=(%d,%d,%d)",
				sourceID, transactionTime, reportedAt, reimbursedAt,
				want.transaction, want.reported, want.reimbursed,
			)
		}
	}
}
