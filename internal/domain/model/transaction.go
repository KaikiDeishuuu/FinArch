package model

import "time"

// ── Backward-compatible Direction (income / expense) ─────────────────────────
// These values appear in DTO output so existing frontend code keeps working.

// Direction indicates the economic nature from a user perspective.
type Direction string

const (
	DirectionIncome  Direction = "income"
	DirectionExpense Direction = "expense"
)

// Source mirrors AccountType for backward compatibility.
type Source string

const (
	SourceCompany  Source = "company"
	SourcePersonal Source = "personal"
)

// ── V9 types ──────────────────────────────────────────────────────────────────

type Mode string

const (
	ModeWork Mode = "work"
	ModeLife Mode = "life"
)

// LedgerDir is the double-entry direction from the account's perspective.
type LedgerDir string

const (
	LedgerDebit  LedgerDir = "debit"  // money flows OUT of the account
	LedgerCredit LedgerDir = "credit" // money flows IN to the account
)

// TxType classifies the economic nature.
type TxType string

const (
	TxTypeIncome   TxType = "income"
	TxTypeExpense  TxType = "expense"
	TxTypeTransfer TxType = "transfer"
)

// ReimbStatus tracks reimbursement lifecycle.
type ReimbStatus string

const (
	ReimbStatusNone       ReimbStatus = "none"
	ReimbStatusPending    ReimbStatus = "pending"
	ReimbStatusReimbursed ReimbStatus = "reimbursed"
)

// ── Transaction ───────────────────────────────────────────────────────────────

// Transaction records one leg of a cash movement.
//
// Backward-compatible fields (Direction, Source, AmountYuan, OccurredAt, Reimbursed)
// are derived by the repository scan so existing service code compiles unchanged.
type Transaction struct {
	// ── Identity ──────────────────────────────────────────────────────────────
	ID     string
	UserID string
	// GroupID: shared by both legs of a transfer; equals ID for single-entry records.
	GroupID string

	// ── V9: account & ledger ─────────────────────────────────────────────────
	AccountID   string
	AccountType AccountType // 'personal' | 'public' — denormalized from accounts JOIN
	LedgerDir   LedgerDir   // 'debit' | 'credit'
	TxType      TxType      // 'income' | 'expense' | 'transfer'

	// ── Amount ────────────────────────────────────────────────────────────────
	AmountCents     int64
	Currency        string
	ExchangeRate    float64
	BaseAmountCents int64 // CNY equivalent

	// ── Category ──────────────────────────────────────────────────────────────
	CategoryID *string
	Category   string // denormalized free-text (always present)

	// ── Reimbursement ─────────────────────────────────────────────────────────
	ReimbStatus     ReimbStatus
	ReimbToAccount  *string
	ReimbursementID *string

	// ── Project ───────────────────────────────────────────────────────────────
	ProjectID *string
	Project   *string // denormalized project name

	// ── Misc ──────────────────────────────────────────────────────────────────
	Mode            Mode // 'work' | 'life'
	Note            string
	AttachmentKey   *string
	Uploaded        bool
	IdempotencyKey  *string
	TxnDate         string // legacy 'YYYY-MM-DD' (backward compatible)
	TransactionTime int64  // unix seconds (UTC)

	// ── Backward-compat (derived during scan) ─────────────────────────────────
	Direction  Direction // mirrors TxType: income→income, expense→expense
	Source     Source    // mirrors AccountType: personal→personal, public→company
	AmountYuan Money     // = AmountCents / 100
	OccurredAt time.Time // parsed from TxnDate (midnight UTC)
	Reimbursed bool      // = ReimbStatus == ReimbStatusReimbursed

	Version      int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ReportedAt   *time.Time
	ReimbursedAt *time.Time
}
