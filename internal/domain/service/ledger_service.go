package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"

	"github.com/google/uuid"
)

// LedgerService orchestrates the Financial Ledger Engine across repositories.
type LedgerService struct {
	db     *sql.DB
	ledger repository.LedgerRepository
}

// NewLedgerService creates a new LedgerService.
func NewLedgerService(db *sql.DB, ledger repository.LedgerRepository) *LedgerService {
	return &LedgerService{
		db:     db,
		ledger: ledger,
	}
}

// PostEntryRequest describes one double-entry journal event.
type PostEntryRequest struct {
	UserID      string
	ReferenceID *string
	Description string
	Source      model.LedgerEntrySource
	Lines       []PostEntryLine
}

type PostEntryLine struct {
	AccountID   string
	DebitCents  int64
	CreditCents int64
	Currency    string
}

// PostEntry validates and posts one immutable journal entry.
func (s *LedgerService) PostEntry(ctx context.Context, req PostEntryRequest) error {
	if len(req.Lines) == 0 {
		return fmt.Errorf("ledger entry must have at least one line")
	}
	if req.Source == "" {
		req.Source = model.LedgerSourceTransaction
	}
	var totalDebit, totalCredit int64
	for _, l := range req.Lines {
		if l.DebitCents < 0 || l.CreditCents < 0 {
			return fmt.Errorf("negative amounts are not allowed")
		}
		totalDebit += l.DebitCents
		totalCredit += l.CreditCents
	}
	if totalDebit != totalCredit {
		return fmt.Errorf("unbalanced entry: debit=%d credit=%d", totalDebit, totalCredit)
	}

	entry := model.LedgerJournalEntry{
		ID:          uuid.NewString(),
		UserID:      req.UserID,
		ReferenceID: req.ReferenceID,
		Description: req.Description,
		Source:      req.Source,
		Status:      model.LedgerEntryPosted,
		CreatedAt:   time.Now().UTC(),
	}
	lines := make([]model.LedgerJournalLine, 0, len(req.Lines))
	for _, l := range req.Lines {
		lines = append(lines, model.LedgerJournalLine{
			ID:          uuid.NewString(),
			EntryID:     entry.ID,
			AccountID:   l.AccountID,
			DebitCents:  l.DebitCents,
			CreditCents: l.CreditCents,
			Currency:    l.Currency,
			CreatedAt:   time.Now().UTC(),
		})
	}

	payload := map[string]any{
		"user_id":     req.UserID,
		"reference_id": req.ReferenceID,
		"description": req.Description,
		"source":       req.Source,
	}
	payloadJSON, _ := json.Marshal(payload)

	if err := s.ledger.CreateJournalEntry(ctx, entry, lines, "ledger_entry_posted", string(payloadJSON)); err != nil {
		return err
	}

	// File-system audit log
	msg := fmt.Sprintf("entry_created entry_id=%s user_id=%s source=%s", entry.ID, entry.UserID, entry.Source)
	log.Printf("[LEDGER] %s", msg)
	_ = appendLedgerFileLog(msg)
	return nil
}

// ReverseEntry posts a reversal entry followed by a corrected entry (if any).
func (s *LedgerService) ReverseEntry(ctx context.Context, original model.LedgerJournalEntry, lines []model.LedgerJournalLine, corrected *PostEntryRequest) error {
	// 1) reversal: swap debit/credit
	reversalLines := make([]PostEntryLine, 0, len(lines))
	for _, l := range lines {
		reversalLines = append(reversalLines, PostEntryLine{
			AccountID:   l.AccountID,
			DebitCents:  l.CreditCents,
			CreditCents: l.DebitCents,
			Currency:    l.Currency,
		})
	}
	refID := original.ID
	if err := s.PostEntry(ctx, PostEntryRequest{
		UserID:      original.UserID,
		ReferenceID: &refID,
		Description: "Reversal of " + original.ID,
		Source:      model.LedgerSourceSystemRepair,
		Lines:       reversalLines,
	}); err != nil {
		return err
	}

	if corrected != nil {
		// 2) corrected entry
		if corrected.ReferenceID == nil {
			corrected.ReferenceID = &refID
		}
		if err := s.PostEntry(ctx, *corrected); err != nil {
			return err
		}
	}
	return nil
}

// StartBackgroundValidator periodically validates ledger integrity for the given user.
// It is safe to run this in a goroutine; failures are logged and do not panic.
func (s *LedgerService) StartBackgroundValidator(ctx context.Context, userID string, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.ledger.ValidateIntegrity(ctx, userID); err != nil {
				log.Printf("[LEDGER] validator_error user_id=%s err=%v", userID, err)
				_ = appendLedgerFileLog(fmt.Sprintf("validator_error user_id=%s err=%v", userID, err))
			}
		}
	}
}

// CreateSnapshot computes a snapshot of all ledger balances for a user and stores it.
func (s *LedgerService) CreateSnapshot(ctx context.Context, userID string) (model.LedgerSnapshot, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(balance_cents),0)
		FROM ledger_balance_cache
		WHERE user_id = ?
	`, userID).Scan(&total); err != nil {
		return model.LedgerSnapshot{}, fmt.Errorf("compute snapshot balance: %w", err)
	}
	now := time.Now().UTC()
	snap := model.LedgerSnapshot{
		ID:                   uuid.NewString(),
		UserID:               userID,
		SnapshotBalanceCents: total,
		CreatedAt:            now,
	}
	// Simple checksum over user + total + timestamp.
	payload := fmt.Sprintf("%s:%d:%s", userID, total, now.Format(time.RFC3339Nano))
	sum := sha256.Sum256([]byte(payload))
	snap.Checksum = hex.EncodeToString(sum[:])

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO ledger_snapshots (id, user_id, snapshot_balance_cents, checksum, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, snap.ID, snap.UserID, snap.SnapshotBalanceCents, snap.Checksum, snap.CreatedAt.Format(time.RFC3339)); err != nil {
		return model.LedgerSnapshot{}, fmt.Errorf("insert ledger snapshot: %w", err)
	}
	return snap, nil
}

// appendLedgerFileLog appends a single line to /data/logs/ledger.log (best-effort).
func appendLedgerFileLog(line string) error {
	const logDir = "/data/logs"
	const logFile = "ledger.log"

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(logDir, logFile)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	timestamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := fmt.Fprintf(f, "[LEDGER] %s %s\n", timestamp, line); err != nil {
		return err
	}
	return nil
}


