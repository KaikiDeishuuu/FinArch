package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigrationV23CreatesBudgetsTable(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "finarch.db")
	sqlDB, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlDB.Close()
	if err := Migrate(ctx, sqlDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var tableName string
	if err := sqlDB.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='budgets'`).Scan(&tableName); err != nil {
		t.Fatalf("budget table missing: %v", err)
	}
	var idxName string
	if err := sqlDB.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='index' AND name='idx_budget_active_scope'`).Scan(&idxName); err != nil {
		t.Fatalf("budget unique index missing: %v", err)
	}
}
