package model

import "time"

// LedgerAccountType is the formal accounting classification.
type LedgerAccountType string

const (
	LedgerAccountAsset     LedgerAccountType = "asset"
	LedgerAccountLiability LedgerAccountType = "liability"
	LedgerAccountExpense   LedgerAccountType = "expense"
	LedgerAccountIncome    LedgerAccountType = "income"
	LedgerAccountEquity    LedgerAccountType = "equity"
)

type LedgerAccountStatus string

const (
	LedgerAccountActive   LedgerAccountStatus = "active"
	LedgerAccountArchived LedgerAccountStatus = "archived"
)

// LedgerAccount mirrors ledger_accounts row.
type LedgerAccount struct {
	ID        string
	UserID    string
	Name      string
	Type      LedgerAccountType
	Currency  string
	Status    LedgerAccountStatus
	CreatedAt time.Time
}

// LedgerEntrySource captures how an entry was created.
type LedgerEntrySource string

const (
	LedgerSourceTransaction      LedgerEntrySource = "transaction"
	LedgerSourceReimbursement    LedgerEntrySource = "reimbursement"
	LedgerSourceManualAdjustment LedgerEntrySource = "manual_adjustment"
	LedgerSourceSystemRepair     LedgerEntrySource = "system_repair"
)

type LedgerEntryStatus string

const (
	LedgerEntryDraft  LedgerEntryStatus = "draft"
	LedgerEntryPosted LedgerEntryStatus = "posted"
	LedgerEntryVoid   LedgerEntryStatus = "void"
)

// LedgerJournalEntry is one immutable financial event.
type LedgerJournalEntry struct {
	ID           string
	UserID       string
	ReferenceID  *string
	Description  string
	Source       LedgerEntrySource
	Status       LedgerEntryStatus
	CreatedAt    time.Time
	EntryHash    string
	PreviousHash *string
}

// LedgerJournalLine is one leg in double-entry.
type LedgerJournalLine struct {
	ID          string
	EntryID     string
	AccountID   string
	DebitCents  int64
	CreditCents int64
	Currency    string
	CreatedAt   time.Time
}

// LedgerEvent is a high-level event for auditing / replay.
type LedgerEvent struct {
	ID          int64
	EventType   string
	EntityID    string
	UserID      *string
	PayloadJSON string
	CreatedAt   time.Time
}

// LedgerBalance represents a cached balance in cents.
type LedgerBalance struct {
	UserID       string
	AccountID    string
	BalanceCents int64
	UpdatedAt    time.Time
}

// LedgerSnapshot for periodic balance snapshots.
type LedgerSnapshot struct {
	ID                  string
	UserID              string
	SnapshotBalanceCents int64
	Checksum            string
	CreatedAt           time.Time
}

