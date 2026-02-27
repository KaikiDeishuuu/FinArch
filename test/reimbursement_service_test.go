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

func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	database, err := db.OpenSQLite(ctx, "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("migrate: %v", err)
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
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	txSvc := service.NewTransactionService(txRepo)
	reimSvc := service.NewReimbursementService(tm, txRepo, reimRepo)

	tx, err := txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
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
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	txSvc := service.NewTransactionService(txRepo)
	reimSvc := service.NewReimbursementService(tm, txRepo, reimRepo)

	tx, err := txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
		OccurredAt: time.Now(),
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
