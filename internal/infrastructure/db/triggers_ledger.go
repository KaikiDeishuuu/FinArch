package db

import (
	"context"
	"database/sql"
	"fmt"
)

// ledgerTriggers contain immutability guarantees for ledger tables.
var ledgerTriggers = []string{
	// Prevent UPDATE on ledger_journal_entries
	`CREATE TRIGGER IF NOT EXISTS trg_ledger_entries_immutable_update
BEFORE UPDATE ON ledger_journal_entries
BEGIN
  SELECT RAISE(ABORT, 'immutable_ledger_entry');
END`,

	// Prevent DELETE on ledger_journal_entries
	`CREATE TRIGGER IF NOT EXISTS trg_ledger_entries_immutable_delete
BEFORE DELETE ON ledger_journal_entries
BEGIN
  SELECT RAISE(ABORT, 'immutable_ledger_entry');
END`,

	// Prevent UPDATE on ledger_journal_lines
	`CREATE TRIGGER IF NOT EXISTS trg_ledger_lines_immutable_update
BEFORE UPDATE ON ledger_journal_lines
BEGIN
  SELECT RAISE(ABORT, 'immutable_ledger_line');
END`,

	// Prevent DELETE on ledger_journal_lines
	`CREATE TRIGGER IF NOT EXISTS trg_ledger_lines_immutable_delete
BEFORE DELETE ON ledger_journal_lines
BEGIN
  SELECT RAISE(ABORT, 'immutable_ledger_line');
END`,
}

// applyLedgerTriggers is invoked from ApplyTriggers to keep concerns separate.
func applyLedgerTriggers(ctx context.Context, db *sql.DB) error {
	for _, ddl := range ledgerTriggers {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("apply ledger trigger: %w", err)
		}
	}
	return nil
}

