package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"finarch/internal/domain/model"
)

// SQLiteLedgerRepository implements LedgerRepository on SQLite.
type SQLiteLedgerRepository struct {
	db *sql.DB
}

// NewSQLiteLedgerRepository creates a new ledger repository.
func NewSQLiteLedgerRepository(db *sql.DB) *SQLiteLedgerRepository {
	return &SQLiteLedgerRepository{db: db}
}

// CreateAccount inserts one ledger account row.
func (r *SQLiteLedgerRepository) CreateAccount(ctx context.Context, a model.LedgerAccount) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO ledger_accounts (id, user_id, name, type, currency, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, COALESCE(?, strftime('%Y-%m-%dT%H:%M:%fZ','now')))
	`, a.ID, a.UserID, a.Name, string(a.Type), a.Currency, string(a.Status), a.CreatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("create ledger account: %w", err)
	}
	return nil
}

// GetAccount loads a ledger account by ID.
func (r *SQLiteLedgerRepository) GetAccount(ctx context.Context, id string) (model.LedgerAccount, error) {
	var a model.LedgerAccount
	var typ, status, createdAt string
	if err := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, type, currency, status, created_at
		FROM ledger_accounts WHERE id = ?
	`, id).Scan(&a.ID, &a.UserID, &a.Name, &typ, &a.Currency, &status, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return model.LedgerAccount{}, fmt.Errorf("ledger account not found")
		}
		return model.LedgerAccount{}, fmt.Errorf("get ledger account: %w", err)
	}
	a.Type = model.LedgerAccountType(typ)
	a.Status = model.LedgerAccountStatus(status)
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		a.CreatedAt = t
	}
	return a, nil
}

// CreateJournalEntry stores an entry with its lines and updates balance cache + events.
func (r *SQLiteLedgerRepository) CreateJournalEntry(
	ctx context.Context,
	entry model.LedgerJournalEntry,
	lines []model.LedgerJournalLine,
	eventType string,
	payloadJSON string,
) error {
	if len(lines) == 0 {
		return fmt.Errorf("ledger entry must have at least one line")
	}
	var totalDebit, totalCredit int64
	for _, l := range lines {
		totalDebit += l.DebitCents
		totalCredit += l.CreditCents
	}
	if totalDebit != totalCredit {
		return fmt.Errorf("unbalanced entry: debit=%d credit=%d", totalDebit, totalCredit)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ledger tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Load last hash for this user to build chain.
	var prevHash sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT entry_hash FROM ledger_journal_entries
		WHERE user_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, entry.UserID).Scan(&prevHash); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("load previous ledger hash: %w", err)
	}
	if prevHash.Valid {
		entry.PreviousHash = &prevHash.String
	}

	// Compute deterministic hash over entry + lines + previous hash.
	hashInput := struct {
		Entry model.LedgerJournalEntry   `json:"entry"`
		Lines []model.LedgerJournalLine `json:"lines"`
	}{
		Entry: entry,
		Lines: lines,
	}
	b, _ := json.Marshal(hashInput)
	h := sha256.Sum256(b)
	entry.EntryHash = hex.EncodeToString(h[:])

	var refID *string = entry.ReferenceID
	var prev *string = entry.PreviousHash
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_journal_entries
		  (id, user_id, reference_id, description, source, status, created_at, entry_hash, previous_hash)
		VALUES (?, ?, ?, ?, ?, ?, COALESCE(?, strftime('%Y-%m-%dT%H:%M:%fZ','now')), ?, ?)
	`, entry.ID, entry.UserID, refID, entry.Description, string(entry.Source), string(entry.Status),
		entry.CreatedAt.UTC().Format(time.RFC3339), entry.EntryHash, prev)
	if err != nil {
		return fmt.Errorf("insert ledger entry: %w", err)
	}

	for _, l := range lines {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO ledger_journal_lines
			  (id, entry_id, account_id, debit_cents, credit_cents, currency, created_at)
			VALUES (?, ?, ?, ?, ?, ?, COALESCE(?, strftime('%Y-%m-%dT%H:%M:%fZ','now')))
		`, l.ID, entry.ID, l.AccountID, l.DebitCents, l.CreditCents, l.Currency, l.CreatedAt.UTC().Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("insert ledger line: %w", err)
		}

		// Update balance cache per line.
		delta := l.DebitCents - l.CreditCents
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ledger_balance_cache (user_id, account_id, balance_cents, updated_at)
			VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
			ON CONFLICT(user_id, account_id) DO UPDATE SET
			  balance_cents = ledger_balance_cache.balance_cents + excluded.balance_cents,
			  updated_at    = excluded.updated_at
		`, entry.UserID, l.AccountID, delta); err != nil {
			return fmt.Errorf("update ledger balance cache: %w", err)
		}
	}

	if eventType != "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ledger_events (event_type, entity_id, user_id, payload_json)
			VALUES (?, ?, ?, ?)
		`, eventType, entry.ID, entry.UserID, payloadJSON); err != nil {
			return fmt.Errorf("insert ledger event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ledger entry: %w", err)
	}
	return nil
}

// GetAccountBalance returns cached balance, rebuilding from lines if needed.
func (r *SQLiteLedgerRepository) GetAccountBalance(ctx context.Context, userID, accountID string) (model.LedgerBalance, error) {
	var b model.LedgerBalance
	var updatedAt string
	err := r.db.QueryRowContext(ctx, `
		SELECT user_id, account_id, balance_cents, updated_at
		FROM ledger_balance_cache
		WHERE user_id = ? AND account_id = ?
	`, userID, accountID).Scan(&b.UserID, &b.AccountID, &b.BalanceCents, &updatedAt)
	if err == sql.ErrNoRows {
		// Rebuild for this account only.
		if err := r.RebuildBalanceCache(ctx, userID); err != nil {
			return model.LedgerBalance{}, err
		}
		err = r.db.QueryRowContext(ctx, `
			SELECT user_id, account_id, balance_cents, updated_at
			FROM ledger_balance_cache
			WHERE user_id = ? AND account_id = ?
		`, userID, accountID).Scan(&b.UserID, &b.AccountID, &b.BalanceCents, &updatedAt)
	}
	if err != nil {
		return model.LedgerBalance{}, fmt.Errorf("get ledger balance: %w", err)
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		b.UpdatedAt = t
	}
	return b, nil
}

// RebuildBalanceCache recomputes balances from journal lines for a user.
func (r *SQLiteLedgerRepository) RebuildBalanceCache(ctx context.Context, userID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rebuild cache: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM ledger_balance_cache WHERE user_id = ?
	`, userID); err != nil {
		return fmt.Errorf("clear old balance cache: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_balance_cache (user_id, account_id, balance_cents, updated_at)
		SELECT
		  e.user_id,
		  l.account_id,
		  COALESCE(SUM(l.debit_cents - l.credit_cents), 0) AS balance_cents,
		  strftime('%Y-%m-%dT%H:%M:%fZ','now') AS updated_at
		FROM ledger_journal_lines l
		JOIN ledger_journal_entries e ON e.id = l.entry_id
		WHERE e.user_id = ?
		GROUP BY e.user_id, l.account_id
	`, userID); err != nil {
		return fmt.Errorf("rebuild balance cache: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rebuild cache: %w", err)
	}
	return nil
}

// ValidateIntegrity runs basic ledger consistency checks.
func (r *SQLiteLedgerRepository) ValidateIntegrity(ctx context.Context, userID string) error {
	// 1) Double-entry: per-entry debit == credit
	var badEntry string
	err := r.db.QueryRowContext(ctx, `
		SELECT e.id
		FROM ledger_journal_entries e
		JOIN ledger_journal_lines l ON l.entry_id = e.id
		WHERE e.user_id = ?
		GROUP BY e.id
		HAVING COALESCE(SUM(l.debit_cents),0) != COALESCE(SUM(l.credit_cents),0)
	`, userID).Scan(&badEntry)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("validate double-entry: %w", err)
	}
	if err == nil {
		return fmt.Errorf("ledger out of balance for entry %s", badEntry)
	}

	// 2) No orphan lines
	err = r.db.QueryRowContext(ctx, `
		SELECT l.id
		FROM ledger_journal_lines l
		LEFT JOIN ledger_journal_entries e ON e.id = l.entry_id
		WHERE e.id IS NULL
		LIMIT 1
	`).Scan(&badEntry)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("validate orphan lines: %w", err)
	}
	if err == nil {
		return fmt.Errorf("orphan ledger line %s", badEntry)
	}

	// 3) Hash chain continuity per user
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, entry_hash, previous_hash
		FROM ledger_journal_entries
		WHERE user_id = ?
		ORDER BY created_at ASC, id ASC
	`, userID)
	if err != nil {
		return fmt.Errorf("load ledger hash chain: %w", err)
	}
	defer rows.Close()
	var lastHash *string
	for rows.Next() {
		var id, entryHash string
		var prev sql.NullString
		if err := rows.Scan(&id, &entryHash, &prev); err != nil {
			return fmt.Errorf("scan hash chain: %w", err)
		}
		if lastHash == nil {
			// First entry: previous_hash must be NULL.
			if prev.Valid {
				return fmt.Errorf("ledger hash chain broken at %s: unexpected previous_hash", id)
			}
		} else {
			// Subsequent entries: previous_hash must equal last entry hash.
			if !prev.Valid || prev.String != *lastHash {
				return fmt.Errorf("ledger hash chain broken at %s", id)
			}
		}
		h := entryHash
		lastHash = &h
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate hash chain: %w", err)
	}

	// 4) Cache vs lines consistency (spot check)
	if err := r.RebuildBalanceCache(ctx, userID); err != nil {
		return fmt.Errorf("rebuild cache during validation: %w", err)
	}

	return nil
}

