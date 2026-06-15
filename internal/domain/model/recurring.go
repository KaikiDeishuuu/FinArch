package model

import "time"

// RecurringFrequency describes how often a rule creates transactions.
type RecurringFrequency string

const (
	RecurringFrequencyDaily   RecurringFrequency = "daily"
	RecurringFrequencyWeekly  RecurringFrequency = "weekly"
	RecurringFrequencyMonthly RecurringFrequency = "monthly"
	RecurringFrequencyYearly  RecurringFrequency = "yearly"
)

// RecurringRuleStatus controls whether a rule is eligible for generation.
type RecurringRuleStatus string

const (
	RecurringRuleStatusActive RecurringRuleStatus = "active"
	RecurringRuleStatusPaused RecurringRuleStatus = "paused"
	RecurringRuleStatusEnded  RecurringRuleStatus = "ended"
)

// RecurringInstanceStatus tracks one scheduled occurrence.
type RecurringInstanceStatus string

const (
	RecurringInstanceGenerating RecurringInstanceStatus = "generating"
	RecurringInstanceGenerated  RecurringInstanceStatus = "generated"
	RecurringInstanceSkipped    RecurringInstanceStatus = "skipped"
	RecurringInstanceFailed     RecurringInstanceStatus = "failed"
)

// MonthEndPolicy defines how monthly/yearly schedules handle missing dates.
type MonthEndPolicy string

const (
	MonthEndClamp MonthEndPolicy = "clamp"
	MonthEndSkip  MonthEndPolicy = "skip"
)

// RecurringTransactionRule is a transaction template plus a schedule.
type RecurringTransactionRule struct {
	ID               string
	UserID           string
	Mode             Mode
	Name             string
	Status           RecurringRuleStatus
	AccountID        string
	TxType           TxType
	Category         string
	AmountCents      int64
	Currency         string
	ExchangeRate     float64
	Note             string
	ProjectID        *string
	Frequency        RecurringFrequency
	Interval         int
	StartDate        string // YYYY-MM-DD
	EndDate          *string
	TimeOfDay        string // HH:mm:ss
	Timezone         string
	DayOfWeek        *int
	DayOfMonth       *int
	MonthEndPolicy   MonthEndPolicy
	NextRunAt        int64
	LastGeneratedFor *string
	CatchUpEnabled   bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// RecurringTransactionInstance records the generation result for one occurrence.
type RecurringTransactionInstance struct {
	ID             string
	RuleID         string
	UserID         string
	OccurrenceDate string
	ScheduledAt    int64
	TransactionID  *string
	IdempotencyKey string
	Status         RecurringInstanceStatus
	Error          *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
