package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Balance-update triggers — each string is a single complete DDL statement
// executed via ExecContext (no semicolon splitting).
var balanceTriggers = []string{
	// INSERT → credit the account balance
	`CREATE TRIGGER IF NOT EXISTS trg_balance_insert
AFTER INSERT ON transactions
BEGIN
  UPDATE accounts SET
    balance_cents = balance_cents + (
      CASE NEW.direction WHEN 'credit' THEN NEW.base_amount_cents ELSE -NEW.base_amount_cents END
    ),
    version    = version + 1,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
  WHERE id = NEW.account_id;
END`,

	// DELETE → reverse the balance effect
	`CREATE TRIGGER IF NOT EXISTS trg_balance_delete
AFTER DELETE ON transactions
BEGIN
  UPDATE accounts SET
    balance_cents = balance_cents - (
      CASE OLD.direction WHEN 'credit' THEN OLD.base_amount_cents ELSE -OLD.base_amount_cents END
    ),
    version    = version + 1,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
  WHERE id = OLD.account_id;
END`,

	// UPDATE of financial fields → remove old effect, apply new effect
	`CREATE TRIGGER IF NOT EXISTS trg_balance_update
AFTER UPDATE OF base_amount_cents, direction, account_id ON transactions
BEGIN
  -- Undo OLD row's effect on OLD account
  UPDATE accounts SET
    balance_cents = balance_cents - (
      CASE OLD.direction WHEN 'credit' THEN OLD.base_amount_cents ELSE -OLD.base_amount_cents END
    ),
    version    = version + 1,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
  WHERE id = OLD.account_id;

  -- Apply NEW row's effect on NEW account (may be different if account_id changed)
  UPDATE accounts SET
    balance_cents = balance_cents + (
      CASE NEW.direction WHEN 'credit' THEN NEW.base_amount_cents ELSE -NEW.base_amount_cents END
    ),
    version    = version + 1,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
  WHERE id = NEW.account_id;
END`,

	// Audit: capture financial field changes on transactions
	`CREATE TRIGGER IF NOT EXISTS trg_audit_txn_update
AFTER UPDATE OF amount_cents, base_amount_cents, reimb_status, note, category, account_id
ON transactions
BEGIN
  INSERT INTO audit_log(user_id, table_name, row_id, action, old_data, new_data)
  VALUES (
    NEW.user_id,
    'transactions',
    NEW.id,
    'UPDATE',
    json_object(
      'amount_cents',      OLD.amount_cents,
      'base_amount_cents', OLD.base_amount_cents,
      'reimb_status',      OLD.reimb_status,
      'note',              OLD.note,
      'category',          OLD.category,
      'account_id',        OLD.account_id
    ),
    json_object(
      'amount_cents',      NEW.amount_cents,
      'base_amount_cents', NEW.base_amount_cents,
      'reimb_status',      NEW.reimb_status,
      'note',              NEW.note,
      'category',          NEW.category,
      'account_id',        NEW.account_id
    )
  );
END`,

	// Audit: capture transaction deletes
	`CREATE TRIGGER IF NOT EXISTS trg_audit_txn_delete
AFTER DELETE ON transactions
BEGIN
  INSERT INTO audit_log(user_id, table_name, row_id, action, old_data, new_data)
  VALUES (
    OLD.user_id,
    'transactions',
    OLD.id,
    'DELETE',
    json_object(
      'amount_cents',      OLD.amount_cents,
      'base_amount_cents', OLD.base_amount_cents,
      'type',              OLD.type,
      'direction',         OLD.direction,
      'reimb_status',      OLD.reimb_status,
      'txn_date',          OLD.txn_date
    ),
    NULL
  );
END`,

	// Prevent hard-delete of accounts that have transactions
	`CREATE TRIGGER IF NOT EXISTS trg_prevent_account_delete
BEFORE DELETE ON accounts
WHEN EXISTS (SELECT 1 FROM transactions WHERE account_id = OLD.id LIMIT 1)
BEGIN
  SELECT RAISE(ABORT, 'Cannot delete account with existing transactions');
END`,
}

// ApplyTriggers (re-)creates all balance and audit triggers.
// Called after each Migrate() run so triggers survive a fresh DB as well as upgrades.
// Uses IF NOT EXISTS so it is idempotent.
func ApplyTriggers(ctx context.Context, db *sql.DB) error {
	for _, ddl := range balanceTriggers {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("apply trigger: %w", err)
		}
	}
	return nil
}
