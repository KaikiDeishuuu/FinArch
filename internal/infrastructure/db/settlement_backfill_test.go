package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigrationV34BackfillsOnlyPreV33PublicExpenses pins the scope of the
// settlement backfill. It runs the V34 statement a second time against rows
// planted after migration, which is equivalent to how it sees a database that
// already held history when V33 introduced the column.
func TestMigrationV34BackfillsOnlyPreV33PublicExpenses(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "finarch.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlDB.Close()
	if err := Migrate(ctx, sqlDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var v33AppliedAt int64
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT applied_at FROM schema_migrations WHERE version = 33`).Scan(&v33AppliedAt); err != nil {
		t.Fatalf("read v33 applied_at: %v", err)
	}
	// created_at is TEXT, so the boundary has to be expressed the same way the
	// application writes it.
	stamp := func(offset int64) string {
		var s string
		if err := sqlDB.QueryRowContext(ctx,
			`SELECT strftime('%Y-%m-%dT%H:%M:%fZ', ?, 'unixepoch')`, v33AppliedAt+offset).Scan(&s); err != nil {
			t.Fatalf("format timestamp: %v", err)
		}
		return s
	}
	before, after := stamp(-3600), stamp(3600)

	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO users(id, email, name, password_hash, role, created_at, updated_at)
		 VALUES ('u1', 'u1@example.com', 'U', 'x', 'user', 0, 0)`); err != nil {
		t.Fatalf("create user: %v", err)
	}
	for _, a := range []struct{ id, kind string }{{"acc-public", "public"}, {"acc-personal", "personal"}} {
		if _, err := sqlDB.ExecContext(ctx,
			`INSERT INTO accounts(id, user_id, name, type) VALUES (?, 'u1', ?, ?)`, a.id, a.id, a.kind); err != nil {
			t.Fatalf("create account %s: %v", a.id, err)
		}
	}

	type row struct {
		id        string
		account   string
		txType    string
		mode      string
		createdAt string
		settled   int
		want      int
	}
	rows := []row{
		{"old-public", "acc-public", "expense", "work", before, 0, 1},
		// Recorded after V33 shipped, so leaving it unsettled was a choice.
		{"new-public", "acc-public", "expense", "work", after, 0, 0},
		// Personal advances belong to the reimbursement lane.
		{"old-personal", "acc-personal", "expense", "work", before, 0, 0},
		// Only expenses settle, and only in work mode — matching ToggleSettled,
		// so nothing the backfill marks can be stranded as un-toggleable.
		{"old-public-income", "acc-public", "income", "work", before, 0, 0},
		{"old-public-life", "acc-public", "expense", "life", before, 0, 0},
		{"already-settled", "acc-public", "expense", "work", before, 1, 1},
	}
	for _, r := range rows {
		direction := "debit"
		if r.txType == "income" {
			direction = "credit"
		}
		if _, err := sqlDB.ExecContext(ctx,
			`INSERT INTO transactions(
			   id, user_id, group_id, direction, account_id, amount_cents, base_amount_cents,
			   type, mode, txn_date, settled, created_at, updated_at)
			 VALUES (?, 'u1', ?, ?, ?, 100, 100, ?, ?, '2026-01-01', ?, ?, ?)`,
			r.id, r.id, direction, r.account, r.txType, r.mode, r.settled, r.createdAt, r.createdAt,
		); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}

	if _, err := sqlDB.ExecContext(ctx, migrationV34SQL); err != nil {
		t.Fatalf("apply v34: %v", err)
	}

	for _, r := range rows {
		var settled int
		var settledAt sql.NullInt64
		if err := sqlDB.QueryRowContext(ctx,
			`SELECT settled, settled_at FROM transactions WHERE id = ?`, r.id).Scan(&settled, &settledAt); err != nil {
			t.Fatalf("read %s: %v", r.id, err)
		}
		if settled != r.want {
			t.Errorf("%s settled = %d, want %d", r.id, settled, r.want)
		}
		// The backfill never invents a settlement timestamp: these rows were
		// never settled through the app, so there is no real time to record.
		if settledAt.Valid {
			t.Errorf("%s got settled_at %v, want NULL", r.id, settledAt.Int64)
		}
	}

	var stranded int
	if err := sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM transactions t
		WHERE t.settled = 1
		  AND (t.type != 'expense' OR t.mode != 'work'
		       OR NOT EXISTS (SELECT 1 FROM accounts a
		                      WHERE a.id = t.account_id AND a.user_id = t.user_id AND a.type = 'public'))
	`).Scan(&stranded); err != nil {
		t.Fatalf("check stranded rows: %v", err)
	}
	if stranded != 0 {
		t.Fatalf("%d settled rows cannot be un-settled through ToggleSettled", stranded)
	}
}
