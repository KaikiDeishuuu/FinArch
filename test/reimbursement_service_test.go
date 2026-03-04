package test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/db"
	sqliterepo "finarch/internal/infrastructure/repository"

	"github.com/google/uuid"
)

const testUserID = "test-user-0000-0000-0000-000000000001"

func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	// Use unique in-memory DB per test to avoid shared state.
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	database, err := db.OpenSQLite(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Create a test user so account FK constraints are satisfied.
	_, err = database.ExecContext(ctx,
		`INSERT OR IGNORE INTO users(id, email, name, password_hash, role, created_at, updated_at)
		 VALUES (?, 'test@example.com', 'Test User', 'x', 'user', ?, ?)`,
		testUserID, time.Now().Unix(), time.Now().Unix(),
	)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	// Create default accounts for the test user.
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	acctSvc := service.NewAccountService(acctRepo, tm)
	if err := acctSvc.EnsureDefaultAccounts(ctx, testUserID); err != nil {
		t.Fatalf("ensure accounts: %v", err)
	}
	return database
}

// TestReimburse_RejectDuplicateTxIDs verifies duplicate IDs are rejected before transaction.
func TestReimburse_RejectDuplicateTxIDs(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()

	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	reimRepo := sqliterepo.NewSQLiteReimbursementRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	txSvc := service.NewTransactionService(txRepo, acctRepo)
	reimSvc := service.NewReimbursementService(tm, txRepo, reimRepo)

	tx, err := txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID:     testUserID,
		OccurredAt: time.Now(),
		Direction:  model.DirectionExpense,
		Source:     model.SourcePersonal,
		Category:   "差旅",
		AmountYuan: 10,
		Currency:   "CNY",
	})
	if err != nil {
		t.Fatalf("create tx: %v", err)
	}

	_, err = reimSvc.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		Applicant:      "alice",
		TransactionIDs: []string{tx.ID, tx.ID},
		RequestNo:      "REIM-" + uuid.NewString(),
	})
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
}

// TestReimburse_AtomicAndSingleUse verifies transaction can be reimbursed only once.
func TestReimburse_AtomicAndSingleUse(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()

	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	reimRepo := sqliterepo.NewSQLiteReimbursementRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	txSvc := service.NewTransactionService(txRepo, acctRepo)
	reimSvc := service.NewReimbursementService(tm, txRepo, reimRepo)

	tx, err := txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
		OccurredAt: time.Now(),
		UserID:     testUserID,
		Direction:  model.DirectionExpense,
		Source:     model.SourcePersonal,
		Category:   "材料",
		AmountYuan: 35,
		Currency:   "CNY",
	})
	if err != nil {
		t.Fatalf("create tx: %v", err)
	}

	_, err = reimSvc.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		Applicant:      "alice",
		TransactionIDs: []string{tx.ID},
		RequestNo:      "REIM-1-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("first reimbursement failed: %v", err)
	}

	_, err = reimSvc.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		Applicant:      "alice",
		TransactionIDs: []string{tx.ID},
		RequestNo:      "REIM-2-" + uuid.NewString(),
	})
	if err == nil {
		t.Fatal("expected second reimbursement failure")
	}
}
