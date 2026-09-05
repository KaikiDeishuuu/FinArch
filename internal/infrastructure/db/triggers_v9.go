package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Balance-update triggers — each string is a single complete DDL statement
// executed via ExecContext (no semicolon splitting).
var balanceTriggers = []string{
	// Enforce the domain transaction range even for direct SQL writers. SQLite
	// column affinity alone accepts REAL values in INTEGER columns, so typeof()
	// is part of the invariant rather than relying only on numeric comparisons.
	`CREATE TRIGGER IF NOT EXISTS trg_transaction_amount_insert
BEFORE INSERT ON transactions
WHEN typeof(NEW.amount_cents) != 'integer'
  OR NEW.amount_cents <= 0
  OR NEW.amount_cents > 9000000000000000
  OR typeof(NEW.base_amount_cents) != 'integer'
  OR NEW.base_amount_cents <= 0
  OR NEW.base_amount_cents > 9000000000000000
BEGIN
  SELECT RAISE(ABORT, 'transaction_amount_out_of_range');
END`,

	`CREATE TRIGGER IF NOT EXISTS trg_transaction_amount_update
BEFORE UPDATE OF amount_cents, base_amount_cents ON transactions
WHEN typeof(NEW.amount_cents) != 'integer'
  OR NEW.amount_cents <= 0
  OR NEW.amount_cents > 9000000000000000
  OR typeof(NEW.base_amount_cents) != 'integer'
  OR NEW.base_amount_cents <= 0
  OR NEW.base_amount_cents > 9000000000000000
BEGIN
  SELECT RAISE(ABORT, 'transaction_amount_out_of_range');
END`,

	// INSERT → credit the account balance
	`CREATE TRIGGER IF NOT EXISTS trg_balance_insert
AFTER INSERT ON transactions
BEGIN
  SELECT CASE
    WHEN typeof(NEW.base_amount_cents) != 'integer'
      OR NEW.base_amount_cents <= 0
      OR NEW.direction NOT IN ('credit', 'debit')
    THEN RAISE(ABORT, 'account_balance_overflow')
    WHEN EXISTS (
      SELECT 1
      FROM accounts
      WHERE id = NEW.account_id
        AND (
          typeof(balance_cents) != 'integer'
          OR typeof(version) != 'integer'
          OR version >= 9223372036854775807
          OR (
            NEW.direction = 'credit'
            AND balance_cents > 9223372036854775807 - NEW.base_amount_cents
          )
          OR (
            NEW.direction = 'debit'
            AND balance_cents < (-9223372036854775807 - 1) + NEW.base_amount_cents
          )
        )
    )
    THEN RAISE(ABORT, 'account_balance_overflow')
  END;

  UPDATE accounts SET
    balance_cents = CASE NEW.direction
      WHEN 'credit' THEN balance_cents + NEW.base_amount_cents
      ELSE balance_cents - NEW.base_amount_cents
    END,
    version    = version + 1,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
  WHERE id = NEW.account_id;
END`,

	// DELETE → reverse the balance effect
	`CREATE TRIGGER IF NOT EXISTS trg_balance_delete
AFTER DELETE ON transactions
BEGIN
  SELECT CASE
    WHEN typeof(OLD.base_amount_cents) != 'integer'
      OR OLD.base_amount_cents <= 0
      OR OLD.direction NOT IN ('credit', 'debit')
    THEN RAISE(ABORT, 'account_balance_overflow')
    WHEN EXISTS (
      SELECT 1
      FROM accounts
      WHERE id = OLD.account_id
        AND (
          typeof(balance_cents) != 'integer'
          OR typeof(version) != 'integer'
          OR version >= 9223372036854775807
          OR (
            OLD.direction = 'credit'
            AND balance_cents < (-9223372036854775807 - 1) + OLD.base_amount_cents
          )
          OR (
            OLD.direction = 'debit'
            AND balance_cents > 9223372036854775807 - OLD.base_amount_cents
          )
        )
    )
    THEN RAISE(ABORT, 'account_balance_overflow')
  END;

  UPDATE accounts SET
    balance_cents = CASE OLD.direction
      WHEN 'credit' THEN balance_cents - OLD.base_amount_cents
      ELSE balance_cents + OLD.base_amount_cents
    END,
    version    = version + 1,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
  WHERE id = OLD.account_id;
END`,

	// UPDATE of financial fields → remove old effect, apply new effect
	`CREATE TRIGGER IF NOT EXISTS trg_balance_update
AFTER UPDATE OF base_amount_cents, direction, account_id ON transactions
BEGIN
  SELECT CASE
    WHEN typeof(OLD.base_amount_cents) != 'integer'
      OR OLD.base_amount_cents <= 0
      OR OLD.direction NOT IN ('credit', 'debit')
      OR typeof(NEW.base_amount_cents) != 'integer'
      OR NEW.base_amount_cents <= 0
      OR NEW.direction NOT IN ('credit', 'debit')
    THEN RAISE(ABORT, 'account_balance_overflow')
  END;

  -- Undo OLD row's effect on OLD account
  SELECT CASE WHEN EXISTS (
    SELECT 1
    FROM accounts
    WHERE id = OLD.account_id
      AND (
        typeof(balance_cents) != 'integer'
        OR typeof(version) != 'integer'
        OR version >= 9223372036854775807
        OR (
          OLD.direction = 'credit'
          AND balance_cents < (-9223372036854775807 - 1) + OLD.base_amount_cents
        )
        OR (
          OLD.direction = 'debit'
          AND balance_cents > 9223372036854775807 - OLD.base_amount_cents
        )
      )
  ) THEN RAISE(ABORT, 'account_balance_overflow') END;

  UPDATE accounts SET
    balance_cents = CASE OLD.direction
      WHEN 'credit' THEN balance_cents - OLD.base_amount_cents
      ELSE balance_cents + OLD.base_amount_cents
    END,
    version    = version + 1,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
  WHERE id = OLD.account_id;

  -- Apply NEW row's effect on NEW account (may be different if account_id changed)
  SELECT CASE WHEN EXISTS (
    SELECT 1
    FROM accounts
    WHERE id = NEW.account_id
      AND (
        typeof(balance_cents) != 'integer'
        OR typeof(version) != 'integer'
        OR version >= 9223372036854775807
        OR (
          NEW.direction = 'credit'
          AND balance_cents > 9223372036854775807 - NEW.base_amount_cents
        )
        OR (
          NEW.direction = 'debit'
          AND balance_cents < (-9223372036854775807 - 1) + NEW.base_amount_cents
        )
      )
  ) THEN RAISE(ABORT, 'account_balance_overflow') END;

  UPDATE accounts SET
    balance_cents = CASE NEW.direction
      WHEN 'credit' THEN balance_cents + NEW.base_amount_cents
      ELSE balance_cents - NEW.base_amount_cents
    END,
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

// dropBeforeApply lists triggers that must be removed before the canonical set
// is installed. The two reimbursement triggers are intentionally retired:
// reimbursement is workflow state, while balances are derived exclusively from
// posted transactions. Keeping the old triggers made cached balances disagree
// with migrations, restore recalculation, and balance history.
var dropBeforeApply = []string{
	"trg_transaction_amount_insert",
	"trg_transaction_amount_update",
	"trg_balance_insert",
	"trg_balance_delete",
	"trg_balance_update",
	"trg_balance_reimburse",
	"trg_balance_unreimburse",
}

// ApplyTriggers (re-)creates all balance and audit triggers.
// Called after each Migrate() run so triggers survive a fresh DB as well as upgrades.
// Triggers in dropBeforeApply are explicitly dropped first so updated definitions
// apply to existing databases and retired definitions are removed.
func ApplyTriggers(ctx context.Context, db *sql.DB) error {
	for _, name := range dropBeforeApply {
		if _, err := db.ExecContext(ctx, "DROP TRIGGER IF EXISTS "+name); err != nil {
			return fmt.Errorf("drop trigger %s: %w", name, err)
		}
	}
	for _, ddl := range balanceTriggers {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("apply trigger: %w", err)
		}
	}
	// Ledger immutability triggers (idempotent)
	if err := applyLedgerTriggers(ctx, db); err != nil {
		return err
	}
	return nil
}
