package test

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
	findb "finarch/internal/infrastructure/db"
	sqliterepo "finarch/internal/infrastructure/repository"
)

func setupFileDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	database, err := findb.OpenSQLite(ctx, filepath.Join(t.TempDir(), "finarch.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := findb.Migrate(ctx, database); err != nil {
		database.Close()
		t.Fatalf("migrate: %v", err)
	}
	_, err = database.ExecContext(ctx,
		`INSERT OR IGNORE INTO users(id, email, name, password_hash, role, created_at, updated_at)
		 VALUES (?, 'test@example.com', 'Test User', 'x', 'user', ?, ?)`,
		testUserID, time.Now().Unix(), time.Now().Unix(),
	)
	if err != nil {
		database.Close()
		t.Fatalf("create test user: %v", err)
	}
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	acctSvc := service.NewAccountService(acctRepo, txRepo, tm)
	if err := acctSvc.EnsureDefaultAccounts(ctx, testUserID); err != nil {
		database.Close()
		t.Fatalf("ensure accounts: %v", err)
	}
	return database
}

func TestSQLiteConcurrentTransactionReadWriteSmoke(t *testing.T) {
	ctx := context.Background()
	database := setupFileDB(t, ctx)
	defer database.Close()
	database.SetMaxOpenConns(8)
	database.SetMaxIdleConns(4)

	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	txSvc := service.NewTransactionService(txRepo, acctRepo, nil)
	statsSvc := service.NewStatsService(database)

	const writers = 6
	const readers = 6
	errCh := make(chan error, writers+readers)
	var wg sync.WaitGroup

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
				UserID:      testUserID,
				OccurredAt:  time.Date(2026, time.January, i+1, 12, 0, 0, 0, time.UTC),
				TxType:      model.TxTypeExpense,
				Mode:        model.ModeWork,
				Category:    "concurrency",
				AmountCents: int64(100 + i),
				Currency:    "CNY",
			})
			errCh <- err
		}(i)
	}

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := txRepo.ListByUser(ctx, testUserID, model.ModeWork); err != nil {
				errCh <- err
				return
			}
			_, err := statsSvc.Monthly(ctx, testUserID, 2026)
			errCh <- err
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent transaction read/write failed: %v", err)
		}
	}
}
