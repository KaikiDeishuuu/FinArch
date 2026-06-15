package model

import "time"

// AttachmentKind describes the business purpose of an uploaded file.
type AttachmentKind string

const (
	AttachmentKindReceipt AttachmentKind = "receipt"
	AttachmentKindInvoice AttachmentKind = "invoice"
	AttachmentKindOther   AttachmentKind = "other"
)

// OCRStatus tracks extraction progress for an attachment.
type OCRStatus string

const (
	OCRStatusNotRequested OCRStatus = "not_requested"
	OCRStatusPending      OCRStatus = "pending"
	OCRStatusProcessing   OCRStatus = "processing"
	OCRStatusDone         OCRStatus = "done"
	OCRStatusFailed       OCRStatus = "failed"
	OCRStatusUnavailable  OCRStatus = "unavailable"
)

// Attachment stores metadata for a receipt/invoice file. The bytes live in
// AttachmentStorage and are addressed by StorageKey.
type Attachment struct {
	ID               string
	UserID           string
	TransactionID    *string
	StorageKey       string
	OriginalFilename string
	ContentType      string
	SizeBytes        int64
	SHA256           string
	Kind             AttachmentKind
	OCRStatus        OCRStatus
	OCRProvider      *string
	OCRText          *string
	OCRJSON          *string
	OCRError         *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// OCRSuggestion is advisory parsed output from OCR. The frontend decides which
// fields to apply to a transaction form.
type OCRSuggestion struct {
	AmountCents   *int64   `json:"amount_cents,omitempty"`
	AmountYuan    *float64 `json:"amount_yuan,omitempty"`
	Currency      string   `json:"currency,omitempty"`
	OccurredAt    string   `json:"occurred_at,omitempty"`
	Merchant      string   `json:"merchant,omitempty"`
	InvoiceNumber string   `json:"invoice_number,omitempty"`
	Category      string   `json:"category,omitempty"`
	Note          string   `json:"note,omitempty"`
	Confidence    float64  `json:"confidence,omitempty"`
}

// OCRResult contains raw and structured OCR output.
type OCRResult struct {
	Provider   string        `json:"provider"`
	Text       string        `json:"text"`
	Suggestion OCRSuggestion `json:"suggestion"`
	Raw        any           `json:"raw,omitempty"`
}
