package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"

	"github.com/google/uuid"
)

const (
	defaultRecurringLimit     = 100
	defaultMaxCatchUpPerRule  = 24
	defaultRecurringTimezone  = "Local"
	defaultRecurringTimeOfDay = "09:00:00"
)

// RecurringTransactionService manages recurring transaction rules and generation.
type RecurringTransactionService struct {
	recurring    repository.RecurringTransactionRepository
	transactions repository.TransactionRepository
	accounts     repository.AccountRepository
	txSvc        *TransactionService
	txManager    repository.TransactionManager
}

// NewRecurringTransactionService creates a recurring service.
func NewRecurringTransactionService(recurring repository.RecurringTransactionRepository, transactions repository.TransactionRepository, txSvc *TransactionService, txManager repository.TransactionManager, accounts ...repository.AccountRepository) *RecurringTransactionService {
	var accountRepo repository.AccountRepository
	if len(accounts) > 0 {
		accountRepo = accounts[0]
	}
	return &RecurringTransactionService{recurring: recurring, transactions: transactions, accounts: accountRepo, txSvc: txSvc, txManager: txManager}
}

type UpsertRecurringRuleRequest struct {
	ID             string
	UserID         string
	Mode           model.Mode
	Name           string
	Status         model.RecurringRuleStatus
	AccountID      string
	TxType         model.TxType
	Direction      model.Direction
	Category       string
	AmountCents    int64
	AmountYuan     model.Money
	Currency       string
	ExchangeRate   float64
	Note           string
	ProjectID      *string
	Frequency      model.RecurringFrequency
	Interval       int
	StartDate      string
	EndDate        *string
	TimeOfDay      string
	Timezone       string
	DayOfWeek      *int
	DayOfMonth     *int
	MonthEndPolicy model.MonthEndPolicy
	CatchUpEnabled *bool
}

type GenerateRecurringResult struct {
	Generated int      `json:"generated"`
	Skipped   int      `json:"skipped"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors,omitempty"`
}

func (s *RecurringTransactionService) ListRules(ctx context.Context, userID string, mode model.Mode) ([]model.RecurringTransactionRule, error) {
	mode, err := normalizeMode(mode)
	if err != nil {
		return nil, err
	}
	return s.recurring.ListRulesByUser(ctx, userID, mode)
}

func (s *RecurringTransactionService) GetRule(ctx context.Context, userID, id string) (model.RecurringTransactionRule, error) {
	if strings.TrimSpace(id) == "" {
		return model.RecurringTransactionRule{}, fmt.Errorf("周期规则不存在")
	}
	return s.recurring.GetRuleByID(ctx, id, userID)
}

func (s *RecurringTransactionService) CreateRule(ctx context.Context, req UpsertRecurringRuleRequest) (model.RecurringTransactionRule, error) {
	rule, err := s.normalizeRule(req, nil)
	if err != nil {
		return model.RecurringTransactionRule{}, err
	}
	if err := s.validateRuleAccount(ctx, rule); err != nil {
		return model.RecurringTransactionRule{}, err
	}
	if err := s.recurring.CreateRule(ctx, rule); err != nil {
		return model.RecurringTransactionRule{}, fmt.Errorf("周期规则保存失败: %w", err)
	}
	return rule, nil
}

func (s *RecurringTransactionService) UpdateRule(ctx context.Context, req UpsertRecurringRuleRequest) (model.RecurringTransactionRule, error) {
	if strings.TrimSpace(req.ID) == "" {
		return model.RecurringTransactionRule{}, fmt.Errorf("周期规则不存在")
	}
	existing, err := s.recurring.GetRuleByID(ctx, req.ID, req.UserID)
	if err != nil {
		return model.RecurringTransactionRule{}, fmt.Errorf("周期规则不存在")
	}
	rule, err := s.normalizeRule(req, &existing)
	if err != nil {
		return model.RecurringTransactionRule{}, err
	}
	if err := s.validateRuleAccount(ctx, rule); err != nil {
		return model.RecurringTransactionRule{}, err
	}
	if err := s.recurring.UpdateRule(ctx, rule); err != nil {
		return model.RecurringTransactionRule{}, fmt.Errorf("周期规则更新失败: %w", err)
	}
	return rule, nil
}

func (s *RecurringTransactionService) SetStatus(ctx context.Context, userID, id string, status model.RecurringRuleStatus) (model.RecurringTransactionRule, error) {
	rule, err := s.recurring.GetRuleByID(ctx, id, userID)
	if err != nil {
		return model.RecurringTransactionRule{}, fmt.Errorf("周期规则不存在")
	}
	if status != model.RecurringRuleStatusActive && status != model.RecurringRuleStatusPaused && status != model.RecurringRuleStatusEnded {
		return model.RecurringTransactionRule{}, fmt.Errorf("无效的周期规则状态")
	}
	rule.Status = status
	rule.UpdatedAt = time.Now()
	if status == model.RecurringRuleStatusActive {
		next, err := NextRecurringOccurrence(rule, time.Now())
		if err != nil {
			return model.RecurringTransactionRule{}, err
		}
		rule.NextRunAt = next.ScheduledAt
	}
	if err := s.recurring.UpdateRule(ctx, rule); err != nil {
		return model.RecurringTransactionRule{}, err
	}
	return rule, nil
}

func (s *RecurringTransactionService) DeleteRule(ctx context.Context, userID, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("周期规则不存在")
	}
	return s.recurring.DeleteRule(ctx, id, userID)
}

func (s *RecurringTransactionService) ListInstances(ctx context.Context, userID, ruleID string, limit int) ([]model.RecurringTransactionInstance, error) {
	if _, err := s.recurring.GetRuleByID(ctx, ruleID, userID); err != nil {
		return nil, fmt.Errorf("周期规则不存在")
	}
	return s.recurring.ListInstances(ctx, ruleID, userID, limit)
}

type RecurringOccurrence struct {
	OccurrenceDate string `json:"occurrence_date"`
	ScheduledAt    int64  `json:"scheduled_at"`
	OccurredAt     string `json:"occurred_at"`
}

func (s *RecurringTransactionService) validateRuleAccount(ctx context.Context, rule model.RecurringTransactionRule) error {
	if s.accounts == nil {
		return nil
	}
	acct, err := s.accounts.GetByID(ctx, rule.AccountID)
	if err != nil || acct.UserID != rule.UserID || !acct.IsActive {
		return fmt.Errorf("所选账户不存在")
	}
	if rule.Mode == model.ModeWork && acct.Type != model.AccountTypePublic {
		return fmt.Errorf("工作模式下请选择公共账户")
	}
	if rule.Mode == model.ModeLife && acct.Type != model.AccountTypePersonal {
		return fmt.Errorf("生活模式下请选择个人账户")
	}
	return nil
}

func (s *RecurringTransactionService) PreviewOccurrences(rule model.RecurringTransactionRule, count int) ([]RecurringOccurrence, error) {
	if count <= 0 || count > 12 {
		count = 5
	}
	out := make([]RecurringOccurrence, 0, count)
	cursor := time.Unix(rule.NextRunAt, 0)
	if rule.NextRunAt == 0 {
		start, err := ruleStartTime(rule)
		if err != nil {
			return nil, err
		}
		cursor = start.Add(-time.Second)
	}
	for len(out) < count {
		next, err := NextRecurringOccurrence(rule, cursor)
		if err != nil {
			return nil, err
		}
		if rule.EndDate != nil && *rule.EndDate != "" && next.OccurrenceDate > *rule.EndDate {
			break
		}
		out = append(out, next)
		cursor = time.Unix(next.ScheduledAt, 0).Add(time.Second)
	}
	return out, nil
}

func (s *RecurringTransactionService) PreviewFromRequest(req UpsertRecurringRuleRequest, count int) ([]RecurringOccurrence, error) {
	rule, err := s.normalizeRule(req, nil)
	if err != nil {
		return nil, err
	}
	return s.PreviewOccurrences(rule, count)
}

func (s *RecurringTransactionService) GenerateDue(ctx context.Context, now time.Time, limit int, dryRun bool) (GenerateRecurringResult, error) {
	if limit <= 0 {
		limit = defaultRecurringLimit
	}
	rules, err := s.recurring.ListDueRules(ctx, now.Unix(), limit)
	if err != nil {
		return GenerateRecurringResult{}, err
	}
	result := GenerateRecurringResult{}
	for _, rule := range rules {
		count := 0
		for rule.Status == model.RecurringRuleStatusActive && rule.NextRunAt <= now.Unix() {
			if count >= defaultMaxCatchUpPerRule {
				break
			}
			if !rule.CatchUpEnabled && count > 0 {
				break
			}
			generated, skipped, err := s.generateOne(ctx, rule, dryRun)
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, err.Error())
				break
			}
			if generated {
				result.Generated++
			}
			if skipped {
				result.Skipped++
			}
			if dryRun {
				break
			}
			fresh, ferr := s.recurring.GetRuleByID(ctx, rule.ID, rule.UserID)
			if ferr != nil {
				break
			}
			rule = fresh
			count++
		}
	}
	return result, nil
}

func (s *RecurringTransactionService) GenerateRuleNow(ctx context.Context, userID, id string, dryRun bool) (GenerateRecurringResult, error) {
	rule, err := s.recurring.GetRuleByID(ctx, id, userID)
	if err != nil {
		return GenerateRecurringResult{}, fmt.Errorf("周期规则不存在")
	}
	generated, skipped, err := s.generateOne(ctx, rule, dryRun)
	if err != nil {
		return GenerateRecurringResult{Failed: 1, Errors: []string{err.Error()}}, err
	}
	res := GenerateRecurringResult{}
	if generated {
		res.Generated = 1
	}
	if skipped {
		res.Skipped = 1
	}
	return res, nil
}

func (s *RecurringTransactionService) generateOne(ctx context.Context, rule model.RecurringTransactionRule, dryRun bool) (generated bool, skipped bool, err error) {
	occ := RecurringOccurrence{
		OccurrenceDate: time.Unix(rule.NextRunAt, 0).In(locationForRule(rule)).Format("2006-01-02"),
		ScheduledAt:    rule.NextRunAt,
		OccurredAt:     time.Unix(rule.NextRunAt, 0).In(locationForRule(rule)).Format("2006-01-02 15:04:05"),
	}
	if rule.EndDate != nil && *rule.EndDate != "" && occ.OccurrenceDate > *rule.EndDate {
		if dryRun {
			return false, true, nil
		}
		rule.Status = model.RecurringRuleStatusEnded
		rule.UpdatedAt = time.Now()
		return false, true, s.recurring.UpdateRule(ctx, rule)
	}
	key := recurringIdempotencyKey(rule.ID, occ.OccurrenceDate)
	if dryRun {
		return false, true, nil
	}
	inst := model.RecurringTransactionInstance{
		ID:             uuid.NewString(),
		RuleID:         rule.ID,
		UserID:         rule.UserID,
		OccurrenceDate: occ.OccurrenceDate,
		ScheduledAt:    occ.ScheduledAt,
		IdempotencyKey: key,
		Status:         model.RecurringInstanceGenerating,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	var created bool
	var failureMessage string
	err = s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		claimed, claimErr := s.recurring.ClaimInstance(txCtx, inst)
		if claimErr != nil {
			return claimErr
		}
		if !claimed {
			skipped = true
			return s.advanceRule(txCtx, rule, occ.OccurrenceDate)
		}
		if existing, getErr := s.transactions.GetByIdempotencyKey(txCtx, rule.UserID, key); getErr == nil && existing.ID != "" {
			created = false
			if markErr := s.recurring.MarkInstanceGenerated(txCtx, inst.ID, existing.ID); markErr != nil {
				return markErr
			}
			return s.advanceRule(txCtx, rule, occ.OccurrenceDate)
		}
		tx, createErr := s.txSvc.CreateTransaction(txCtx, CreateTransactionRequest{
			UserID:                  rule.UserID,
			Mode:                    rule.Mode,
			OccurredAt:              time.Unix(occ.ScheduledAt, 0).UTC(),
			AccountID:               rule.AccountID,
			TxType:                  rule.TxType,
			Category:                rule.Category,
			AmountCents:             rule.AmountCents,
			Currency:                rule.Currency,
			ExchangeRate:            rule.ExchangeRate,
			Note:                    rule.Note,
			ProjectID:               rule.ProjectID,
			IdempotencyKey:          &key,
			RecurringRuleID:         &rule.ID,
			RecurringOccurrenceDate: &occ.OccurrenceDate,
		})
		if createErr != nil {
			failureMessage = truncateError(createErr.Error())
			if markErr := s.recurring.MarkInstanceFailed(txCtx, inst.ID, failureMessage); markErr != nil {
				return markErr
			}
			return s.advanceRule(txCtx, rule, occ.OccurrenceDate)
		}
		created = true
		if markErr := s.recurring.MarkInstanceGenerated(txCtx, inst.ID, tx.ID); markErr != nil {
			return markErr
		}
		return s.advanceRule(txCtx, rule, occ.OccurrenceDate)
	})
	if err != nil {
		return false, skipped, err
	}
	if failureMessage != "" {
		return false, skipped, errors.New(failureMessage)
	}
	return created, skipped, nil
}

func (s *RecurringTransactionService) advanceRule(ctx context.Context, rule model.RecurringTransactionRule, occurrenceDate string) error {
	next, err := NextRecurringOccurrence(rule, time.Unix(rule.NextRunAt, 0).Add(time.Second))
	if err != nil {
		return err
	}
	rule.NextRunAt = next.ScheduledAt
	rule.LastGeneratedFor = &occurrenceDate
	rule.UpdatedAt = time.Now()
	if rule.EndDate != nil && *rule.EndDate != "" && next.OccurrenceDate > *rule.EndDate {
		rule.Status = model.RecurringRuleStatusEnded
	}
	return s.recurring.UpdateRule(ctx, rule)
}

func (s *RecurringTransactionService) normalizeRule(req UpsertRecurringRuleRequest, existing *model.RecurringTransactionRule) (model.RecurringTransactionRule, error) {
	now := time.Now()
	r := model.RecurringTransactionRule{ID: uuid.NewString(), UserID: req.UserID, Status: model.RecurringRuleStatusActive, Interval: 1, Currency: "CNY", TimeOfDay: defaultRecurringTimeOfDay, Timezone: defaultRecurringTimezone, MonthEndPolicy: model.MonthEndClamp, CatchUpEnabled: true, CreatedAt: now, UpdatedAt: now}
	if existing != nil {
		r = *existing
		r.UpdatedAt = now
	}
	if req.ID != "" {
		r.ID = req.ID
	}
	if strings.TrimSpace(req.UserID) != "" {
		r.UserID = req.UserID
	}
	if strings.TrimSpace(r.UserID) == "" {
		return model.RecurringTransactionRule{}, fmt.Errorf("用户不存在")
	}
	if req.Mode != "" {
		r.Mode = req.Mode
	}
	mode, err := normalizeMode(r.Mode)
	if err != nil {
		return model.RecurringTransactionRule{}, err
	}
	r.Mode = mode
	if strings.TrimSpace(req.Name) != "" || existing == nil {
		r.Name = strings.TrimSpace(req.Name)
	}
	if r.Name == "" {
		r.Name = "周期交易"
	}
	if req.Status != "" {
		r.Status = req.Status
	}
	if r.Status != model.RecurringRuleStatusActive && r.Status != model.RecurringRuleStatusPaused && r.Status != model.RecurringRuleStatusEnded {
		return model.RecurringTransactionRule{}, fmt.Errorf("无效的周期规则状态")
	}
	if strings.TrimSpace(req.AccountID) != "" || existing == nil {
		r.AccountID = strings.TrimSpace(req.AccountID)
	}
	if r.AccountID == "" {
		return model.RecurringTransactionRule{}, fmt.Errorf("请选择账户")
	}
	if req.TxType != "" {
		r.TxType = req.TxType
	} else if req.Direction != "" {
		r.TxType = model.TxType(req.Direction)
	}
	if r.TxType == "" {
		r.TxType = model.TxTypeExpense
	}
	if r.TxType != model.TxTypeIncome && r.TxType != model.TxTypeExpense {
		return model.RecurringTransactionRule{}, fmt.Errorf("周期交易仅支持收入或支出")
	}
	if strings.TrimSpace(req.Category) != "" || existing == nil {
		r.Category = strings.TrimSpace(req.Category)
	}
	if r.Category == "" {
		return model.RecurringTransactionRule{}, fmt.Errorf("请选择分类")
	}
	amountCents := req.AmountCents
	if amountCents == 0 && req.AmountYuan > 0 {
		amountCents = int64(req.AmountYuan * 100)
	}
	if amountCents > 0 {
		r.AmountCents = amountCents
	}
	if r.AmountCents <= 0 {
		return model.RecurringTransactionRule{}, fmt.Errorf("金额必须为正数")
	}
	if strings.TrimSpace(req.Currency) != "" || existing == nil {
		r.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
		if r.Currency == "" {
			r.Currency = "CNY"
		}
	}
	if req.ExchangeRate > 0 || existing == nil {
		r.ExchangeRate = req.ExchangeRate
	}
	if req.Note != "" || existing == nil {
		r.Note = strings.TrimSpace(req.Note)
	}
	if req.ProjectID != nil || existing == nil {
		r.ProjectID = req.ProjectID
		if r.ProjectID != nil {
			projectID := strings.TrimSpace(*r.ProjectID)
			if projectID == "" {
				r.ProjectID = nil
			} else {
				r.ProjectID = &projectID
			}
		}
	}
	if req.Frequency != "" || existing == nil {
		r.Frequency = req.Frequency
	}
	if r.Frequency == "" {
		r.Frequency = model.RecurringFrequencyMonthly
	}
	if !validFrequency(r.Frequency) {
		return model.RecurringTransactionRule{}, fmt.Errorf("无效的周期频率")
	}
	if req.Interval > 0 || existing == nil {
		r.Interval = req.Interval
		if r.Interval <= 0 {
			r.Interval = 1
		}
	}
	if strings.TrimSpace(req.StartDate) != "" || existing == nil {
		r.StartDate = strings.TrimSpace(req.StartDate)
	}
	if r.StartDate == "" {
		r.StartDate = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", r.StartDate); err != nil {
		return model.RecurringTransactionRule{}, fmt.Errorf("开始日期格式必须为 YYYY-MM-DD")
	}
	if req.EndDate != nil || existing == nil {
		r.EndDate = cleanOptionalDate(req.EndDate)
	}
	if r.EndDate != nil {
		if _, err := time.Parse("2006-01-02", *r.EndDate); err != nil {
			return model.RecurringTransactionRule{}, fmt.Errorf("结束日期格式必须为 YYYY-MM-DD")
		}
		if *r.EndDate < r.StartDate {
			return model.RecurringTransactionRule{}, fmt.Errorf("结束日期不能早于开始日期")
		}
	}
	if strings.TrimSpace(req.TimeOfDay) != "" || existing == nil {
		r.TimeOfDay = normalizeTimeOfDay(req.TimeOfDay)
	}
	if _, err := time.Parse("15:04:05", r.TimeOfDay); err != nil {
		return model.RecurringTransactionRule{}, fmt.Errorf("执行时间格式必须为 HH:mm:ss")
	}
	if strings.TrimSpace(req.Timezone) != "" || existing == nil {
		r.Timezone = strings.TrimSpace(req.Timezone)
		if r.Timezone == "" {
			r.Timezone = defaultRecurringTimezone
		}
	}
	if _, err := time.LoadLocation(locationName(r.Timezone)); err != nil {
		return model.RecurringTransactionRule{}, fmt.Errorf("无效的时区")
	}
	if req.DayOfWeek != nil || existing == nil {
		r.DayOfWeek = req.DayOfWeek
	}
	if r.DayOfWeek != nil && (*r.DayOfWeek < 0 || *r.DayOfWeek > 6) {
		return model.RecurringTransactionRule{}, fmt.Errorf("星期必须在 0-6 之间")
	}
	if req.DayOfMonth != nil || existing == nil {
		r.DayOfMonth = req.DayOfMonth
	}
	if r.DayOfMonth != nil && (*r.DayOfMonth < 1 || *r.DayOfMonth > 31) {
		return model.RecurringTransactionRule{}, fmt.Errorf("日期必须在 1-31 之间")
	}
	if req.MonthEndPolicy != "" || existing == nil {
		r.MonthEndPolicy = req.MonthEndPolicy
		if r.MonthEndPolicy == "" {
			r.MonthEndPolicy = model.MonthEndClamp
		}
	}
	if r.MonthEndPolicy != model.MonthEndClamp && r.MonthEndPolicy != model.MonthEndSkip {
		return model.RecurringTransactionRule{}, fmt.Errorf("无效的月末策略")
	}
	if req.CatchUpEnabled != nil || existing == nil {
		r.CatchUpEnabled = true
		if req.CatchUpEnabled != nil {
			r.CatchUpEnabled = *req.CatchUpEnabled
		}
	}
	if existing == nil || req.StartDate != "" || req.TimeOfDay != "" || req.Timezone != "" || req.Frequency != "" || req.Interval > 0 || req.DayOfWeek != nil || req.DayOfMonth != nil || req.MonthEndPolicy != "" {
		next, err := NextRecurringOccurrence(r, time.Now().Add(-time.Second))
		if err != nil {
			return model.RecurringTransactionRule{}, err
		}
		r.NextRunAt = next.ScheduledAt
	}
	return r, nil
}

func NextRecurringOccurrence(rule model.RecurringTransactionRule, after time.Time) (RecurringOccurrence, error) {
	loc := locationForRule(rule)
	start, err := ruleStartTime(rule)
	if err != nil {
		return RecurringOccurrence{}, err
	}
	afterLocal := after.In(loc)
	candidate := start
	for i := 0; i < 5000; i++ {
		if candidate.After(afterLocal) || candidate.Equal(afterLocal) {
			return RecurringOccurrence{OccurrenceDate: candidate.Format("2006-01-02"), ScheduledAt: candidate.UTC().Unix(), OccurredAt: candidate.Format("2006-01-02 15:04:05")}, nil
		}
		next, ok := advanceCandidate(rule, candidate)
		if !ok {
			candidate = candidate.AddDate(0, 1, 0)
		} else {
			candidate = next
		}
	}
	return RecurringOccurrence{}, fmt.Errorf("无法计算下一次周期执行时间")
}

func advanceCandidate(rule model.RecurringTransactionRule, current time.Time) (time.Time, bool) {
	interval := rule.Interval
	if interval <= 0 {
		interval = 1
	}
	switch rule.Frequency {
	case model.RecurringFrequencyDaily:
		return current.AddDate(0, 0, interval), true
	case model.RecurringFrequencyWeekly:
		return current.AddDate(0, 0, 7*interval), true
	case model.RecurringFrequencyMonthly:
		return addMonthsWithPolicy(current, intervalMonths(rule), rule)
	case model.RecurringFrequencyYearly:
		return addMonthsWithPolicy(current, intervalMonths(rule), rule)
	default:
		return time.Time{}, false
	}
}

func intervalMonths(rule model.RecurringTransactionRule) int {
	interval := rule.Interval
	if interval <= 0 {
		interval = 1
	}
	if rule.Frequency == model.RecurringFrequencyYearly {
		return 12 * interval
	}
	return interval
}

func addMonthsWithPolicy(current time.Time, months int, rule model.RecurringTransactionRule) (time.Time, bool) {
	loc := current.Location()
	targetDay := current.Day()
	if rule.DayOfMonth != nil {
		targetDay = *rule.DayOfMonth
	}
	base := time.Date(current.Year(), current.Month(), 1, current.Hour(), current.Minute(), current.Second(), 0, loc).AddDate(0, months, 0)
	for i := 0; i < 240; i++ {
		lastDay := time.Date(base.Year(), base.Month()+1, 0, current.Hour(), current.Minute(), current.Second(), 0, loc).Day()
		if targetDay <= lastDay {
			return time.Date(base.Year(), base.Month(), targetDay, current.Hour(), current.Minute(), current.Second(), 0, loc), true
		}
		if rule.MonthEndPolicy != model.MonthEndSkip {
			return time.Date(base.Year(), base.Month(), lastDay, current.Hour(), current.Minute(), current.Second(), 0, loc), true
		}
		base = base.AddDate(0, months, 0)
	}
	return time.Time{}, false
}

func ruleStartTime(rule model.RecurringTransactionRule) (time.Time, error) {
	loc := locationForRule(rule)
	date, err := time.ParseInLocation("2006-01-02", rule.StartDate, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("开始日期格式必须为 YYYY-MM-DD")
	}
	tod, err := time.Parse("15:04:05", rule.TimeOfDay)
	if err != nil {
		return time.Time{}, fmt.Errorf("执行时间格式必须为 HH:mm:ss")
	}
	start := time.Date(date.Year(), date.Month(), date.Day(), tod.Hour(), tod.Minute(), tod.Second(), 0, loc)
	if rule.Frequency == model.RecurringFrequencyWeekly && rule.DayOfWeek != nil {
		for int(start.Weekday()) != *rule.DayOfWeek {
			start = start.AddDate(0, 0, 1)
		}
	}
	if (rule.Frequency == model.RecurringFrequencyMonthly || rule.Frequency == model.RecurringFrequencyYearly) && rule.DayOfMonth != nil {
		lastDay := time.Date(start.Year(), start.Month()+1, 0, start.Hour(), start.Minute(), start.Second(), 0, loc).Day()
		day := *rule.DayOfMonth
		if day > lastDay {
			if rule.MonthEndPolicy == model.MonthEndSkip {
				next, ok := addMonthsWithPolicy(start, intervalMonths(rule), rule)
				if !ok {
					return time.Time{}, fmt.Errorf("无法计算下一次周期执行时间")
				}
				start = next
			} else {
				day = lastDay
				start = time.Date(start.Year(), start.Month(), day, start.Hour(), start.Minute(), start.Second(), 0, loc)
			}
		} else {
			start = time.Date(start.Year(), start.Month(), day, start.Hour(), start.Minute(), start.Second(), 0, loc)
			if start.Before(date) {
				next, ok := addMonthsWithPolicy(start, intervalMonths(rule), rule)
				if !ok {
					return time.Time{}, fmt.Errorf("无法计算下一次周期执行时间")
				}
				start = next
			}
		}
	}
	return start, nil
}

func normalizeMode(mode model.Mode) (model.Mode, error) {
	if mode == "" {
		return model.ModeWork, nil
	}
	if mode != model.ModeWork && mode != model.ModeLife {
		return "", fmt.Errorf("无效的模式")
	}
	return mode, nil
}

func validFrequency(freq model.RecurringFrequency) bool {
	switch freq {
	case model.RecurringFrequencyDaily, model.RecurringFrequencyWeekly, model.RecurringFrequencyMonthly, model.RecurringFrequencyYearly:
		return true
	default:
		return false
	}
}

func cleanOptionalDate(value *string) *string {
	if value == nil {
		return nil
	}
	v := strings.TrimSpace(*value)
	if v == "" {
		return nil
	}
	return &v
}

func normalizeTimeOfDay(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultRecurringTimeOfDay
	}
	if len(value) == len("15:04") {
		return value + ":00"
	}
	return value
}

func locationName(name string) string {
	if strings.TrimSpace(name) == "" || name == defaultRecurringTimezone {
		return "Local"
	}
	return name
}

func locationForRule(rule model.RecurringTransactionRule) *time.Location {
	loc, err := time.LoadLocation(locationName(rule.Timezone))
	if err != nil {
		return time.Local
	}
	return loc
}

func recurringIdempotencyKey(ruleID, occurrenceDate string) string {
	return "recurring:" + ruleID + ":" + occurrenceDate
}

func truncateError(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > 240 {
		return msg[:240]
	}
	return msg
}
