package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyTriggers_RecreatesLegacyBalanceTriggers(t *testing.T) {
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

	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO users (id,email,name,password_hash,role,created_at,updated_at)
		VALUES ('u1','u1@example.com','u1','x','user',?,?)`, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO accounts (id,user_id,name,type,currency,balance_cents,version,is_active,created_at,updated_at)
		VALUES ('a1','u1','Personal','personal','CNY',0,0,1,?,?)`, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert account: %v", err)
	}

	if _, err := sqlDB.ExecContext(ctx, `DROP TRIGGER IF EXISTS trg_balance_insert`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx, `
		CREATE TRIGGER trg_balance_insert
		AFTER INSERT ON transactions
		BEGIN
		  UPDATE accounts SET balance_cents = balance_cents +
		    (CASE NEW.direction WHEN 'credit' THEN NEW.amount_cents ELSE -NEW.amount_cents END)
		  WHERE id = NEW.account_id;
		END`); err != nil {
		t.Fatalf("create legacy trigger: %v", err)
	}

	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO transactions (
			id,user_id,group_id,direction,account_id,amount_cents,currency,exchange_rate,exchange_rate_source,exchange_rate_at,base_currency,base_amount_cents,
			type,category,reimb_status,mode,note,uploaded,txn_date,transaction_time,created_at,updated_at
		) VALUES (
			'tx1','u1','tx1','debit','a1',1568,'EUR',7.6123,'test',1700000000,'CNY',11936,
			'expense','meal','none','life','',0,'2026-01-01',1700000000,?,?
		)`, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert tx1: %v", err)
	}

	if err := ApplyTriggers(ctx, sqlDB); err != nil {
		t.Fatalf("reapply triggers: %v", err)
	}

	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO transactions (
			id,user_id,group_id,direction,account_id,amount_cents,currency,exchange_rate,exchange_rate_source,exchange_rate_at,base_currency,base_amount_cents,
			type,category,reimb_status,mode,note,uploaded,txn_date,transaction_time,created_at,updated_at
		) VALUES (
			'tx2','u1','tx2','debit','a1',100,'EUR',7.0,'test',1700000001,'CNY',700,
			'expense','meal','none','life','',0,'2026-01-02',1700000001,?,?
		)`, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert tx2: %v", err)
	}

	var balance int64
	if err := sqlDB.QueryRowContext(ctx, `SELECT balance_cents FROM accounts WHERE id='a1'`).Scan(&balance); err != nil {
		t.Fatalf("query balance: %v", err)
	}
	if balance != -2268 {
		t.Fatalf("expected mixed legacy/new trigger balance -2268, got %d", balance)
	}
}

func TestMigrationV21_RepairsCorruptedAccountBalances(t *testing.T) {
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

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO users (id,email,name,password_hash,role,created_at,updated_at)
		VALUES ('u1','u1@example.com','u1','x','user',?,?)`, now, now); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO accounts (id,user_id,name,type,currency,balance_cents,version,is_active,created_at,updated_at)
		VALUES ('a1','u1','Personal','personal','CNY',-1569,0,1,?,?)`, now, now); err != nil {
		t.Fatalf("insert account: %v", err)
	}

	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO transactions (
			id,user_id,group_id,direction,account_id,amount_cents,currency,exchange_rate,exchange_rate_source,exchange_rate_at,base_currency,base_amount_cents,
			type,category,reimb_status,mode,note,uploaded,txn_date,transaction_time,created_at,updated_at
		) VALUES
			('tx1','u1','tx1','debit','a1',1,'CNY',1,'test',1700000000,'CNY',1,'expense','misc','none','life','',0,'2026-01-01',1700000000,?,?),
			('tx2','u1','tx2','debit','a1',1568,'EUR',7.6123,'test',1700000001,'CNY',11936,'expense','misc','none','life','',0,'2026-01-02',1700000001,?,?)
	`, now, now, now, now); err != nil {
		t.Fatalf("insert tx: %v", err)
	}

	if _, err := sqlDB.ExecContext(ctx, migrationV21SQL); err != nil {
		t.Fatalf("apply repair migration: %v", err)
	}

	var balance int64
	if err := sqlDB.QueryRowContext(ctx, `SELECT balance_cents FROM accounts WHERE id='a1'`).Scan(&balance); err != nil {
		t.Fatalf("query balance: %v", err)
	}
	if balance != -11937 {
		t.Fatalf("expected repaired balance -11937, got %d", balance)
	}
}
