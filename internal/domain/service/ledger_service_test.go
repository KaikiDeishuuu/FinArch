package service_test

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

func setupTestLedger(t *testing.T) (*sql.DB, *service.LedgerService, func()) {
	t.Helper()
	ctx := context.Background()
	database, err := db.OpenSQLite(ctx, "file:ledger_test.db?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ledgerRepo := sqliterepo.NewSQLiteLedgerRepository(database)
	svc := service.NewLedgerService(database, ledgerRepo)
	cleanup := func() {
		_ = database.Close()
	}
	return database, svc, cleanup
}

func seedLedgerAccount(t *testing.T, database *sql.DB, userID string) string {
	t.Helper()
	// Seed minimal user row to satisfy foreign key.
	_, err := database.Exec(`
		INSERT INTO users (id, email, name, password_hash, role, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, 'test-hash', 'owner', ?, ?, NULL)
	`, userID, userID+"@example.com", "Ledger User", time.Now().Unix(), time.Now().Unix())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	id := uuid.NewString()
	_, err = database.Exec(`
		INSERT INTO ledger_accounts (id, user_id, name, type, currency, status, created_at)
		VALUES (?, ?, 'Cash', 'asset', 'CNY', 'active', ?)
	`, id, userID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed ledger account: %v", err)
	}
	return id
}

// Test that a balanced entry is accepted and updates the balance cache.
func TestLedger_PostEntry_Balanced(t *testing.T) {
	ctx := context.Background()
	database, svc, cleanup := setupTestLedger(t)
	defer cleanup()

	userID := uuid.NewString()
	acctID := seedLedgerAccount(t, database, userID)

	err := svc.PostEntry(ctx, service.PostEntryRequest{
		UserID:      userID,
		Description: "Test income",
		Source:      model.LedgerSourceTransaction,
		Lines: []service.PostEntryLine{
			{AccountID: acctID, DebitCents: 0, CreditCents: 10000, Currency: "CNY"},
			{AccountID: acctID, DebitCents: 10000, CreditCents: 0, Currency: "CNY"},
		},
	})
	if err != nil {
		t.Fatalf("PostEntry returned error: %v", err)
	}

	var balance int64
	if err := database.QueryRow(`
		SELECT balance_cents FROM ledger_balance_cache WHERE user_id = ? AND account_id = ?
	`, userID, acctID).Scan(&balance); err != nil {
		t.Fatalf("query balance cache: %v", err)
	}
	if balance != 0 {
		t.Fatalf("expected zero net balance, got %d", balance)
	}
}

// Test that unbalanced entry is rejected.
func TestLedger_PostEntry_UnbalancedFails(t *testing.T) {
	ctx := context.Background()
	_, svc, cleanup := setupTestLedger(t)
	defer cleanup()

	userID := uuid.NewString()
	err := svc.PostEntry(ctx, service.PostEntryRequest{
		UserID:      userID,
		Description: "Unbalanced",
		Source:      model.LedgerSourceTransaction,
		Lines: []service.PostEntryLine{
			{AccountID: "a1", DebitCents: 100, CreditCents: 0, Currency: "CNY"},
			{AccountID: "a2", DebitCents: 0, CreditCents: 50, Currency: "CNY"},
		},
	})
	if err == nil {
		t.Fatalf("expected error for unbalanced entry, got nil")
	}
}

