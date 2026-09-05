package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"
)

const (
	dpThreshold   = int64(1_000_000_000)
	timePruneDays = 90

	maxMatchingCandidateFetch = 2000
	maxMatchingCandidates     = 500
	maxMatchingSearchNodes    = 250_000
	maxMatchingTargetCents    = int64(1_000_000_000_000)
	maxMatchingToleranceCents = int64(100_000)
	contextCheckInterval      = int64(256)
)

func normalizeMatchDepth(maxDepth int) int {
	if maxDepth <= 0 {
		return DefaultDepth
	}
	if maxDepth > MaxAllowedDepth {
		return MaxAllowedDepth
	}
	return maxDepth
}

// MatchResult represents one matching combination candidate.
type MatchResult struct {
	TransactionIDs []string
	TotalCents     int64
	AbsErrorCents  int64
	TotalYuan      model.Money
	AbsErrorYuan   model.Money
	ProjectCount   int
	ItemCount      int
	Score          float64
	TimePruned     bool
}

// MatchingService finds reimbursement combinations.
type MatchingService struct {
	transactions repository.TransactionRepository
}

func NewMatchingService(transactions repository.TransactionRepository) *MatchingService {
	return &MatchingService{transactions: transactions}
}

func (s *MatchingService) Match(
	ctx context.Context,
	userID string,
	targetYuan model.Money,
	toleranceYuan model.Money,
	maxDepth int,
	projectID *string,
	limit int,
) ([]MatchResult, error) {
	targetCents, toleranceCents, err := validateMatchingAmounts(targetYuan, toleranceYuan)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	maxDepth = normalizeMatchDepth(maxDepth)
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxAllowedLimit {
		limit = MaxAllowedLimit
	}

	candidates, err := s.transactions.ListUnreimbursedPersonalExpenses(
		ctx, userID, projectID, maxMatchingCandidateFetch, model.ModeWork,
	)
	if err != nil {
		return nil, err
	}

	// The target is denominated in yuan (CNY). Do not add amounts normalized
	// to a different account base currency without a trustworthy cross-rate.
	cnyCandidates := make([]model.Transaction, 0, len(candidates))
	for _, candidate := range candidates {
		baseCurrency := strings.ToUpper(strings.TrimSpace(candidate.BaseCurrency))
		if baseCurrency == "" {
			baseCurrency = strings.ToUpper(strings.TrimSpace(candidate.Currency))
			if baseCurrency == "" {
				baseCurrency = "CNY"
			}
		}
		if baseCurrency != "CNY" {
			continue
		}
		if candidate.BaseAmountCents == 0 {
			// Safe legacy fallback because this candidate is CNY-denominated.
			candidate.BaseAmountCents = candidate.AmountCents
		}
		cnyCandidates = append(cnyCandidates, candidate)
	}
	return findBestMatchesCentsContext(
		ctx, cnyCandidates, targetCents, toleranceCents, maxDepth, limit,
	)
}

func validateMatchingAmounts(targetYuan, toleranceYuan model.Money) (int64, int64, error) {
	targetCents, err := targetYuan.Cents()
	if err != nil || targetCents <= 0 {
		return 0, 0, fmt.Errorf("请输入有效的目标金额")
	}
	toleranceCents, err := toleranceYuan.Cents()
	if err != nil || toleranceCents < 0 {
		return 0, 0, fmt.Errorf("请输入有效的容差金额")
	}
	if targetCents > maxMatchingTargetCents {
		return 0, 0, fmt.Errorf("目标金额过大")
	}
	if toleranceCents > maxMatchingToleranceCents || toleranceCents > targetCents {
		return 0, 0, fmt.Errorf("容差金额过大")
	}
	return targetCents, toleranceCents, nil
}

// FindBestMatchesCents is the compatibility entry point. Request paths use the
// cancellable search through MatchingService.Match.
func FindBestMatchesCents(
	candidates []model.Transaction,
	targetCents int64,
	toleranceCents int64,
	maxDepth int,
	limit int,
) []MatchResult {
	results, _ := findBestMatchesCentsContext(
		context.Background(), candidates, targetCents, toleranceCents, maxDepth, limit,
	)
	return results
}

func findBestMatchesCentsContext(
	ctx context.Context,
	candidates []model.Transaction,
	targetCents int64,
	toleranceCents int64,
	maxDepth int,
	limit int,
) ([]MatchResult, error) {
	if len(candidates) == 0 || targetCents <= 0 || targetCents > maxMatchingTargetCents {
		return nil, nil
	}
	if toleranceCents < 0 || toleranceCents > maxMatchingToleranceCents || toleranceCents > targetCents {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	maxDepth = normalizeMatchDepth(maxDepth)
	if limit <= 0 {
		return nil, nil
	}
	if limit > MaxAllowedLimit {
		limit = MaxAllowedLimit
	}

	upper := targetCents + toleranceCents
	workCandidates := prepareMatchingCandidates(candidates, upper)
	if len(workCandidates) == 0 {
		return nil, nil
	}
	workSet := workCandidates
	timePruned := false
	n := int64(len(workCandidates))
	if targetCents > dpThreshold/n {
		cutoff := time.Now().AddDate(0, 0, -timePruneDays)
		pruned := make([]model.Transaction, 0, len(workCandidates))
		for _, candidate := range workCandidates {
			if candidate.OccurredAt.After(cutoff) {
				pruned = append(pruned, candidate)
			}
		}
		if len(pruned) > 0 {
			workSet = pruned
			timePruned = true
		}
	}

	remainingNodes := int64(maxMatchingSearchNodes)
	now := time.Now()
	results, err := searchMatchesCents(
		ctx, workSet, targetCents, toleranceCents, maxDepth, limit,
		timePruned, now, &remainingNodes,
	)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 && timePruned && remainingNodes > 0 {
		results, err = searchMatchesCents(
			ctx, workCandidates, targetCents, toleranceCents, maxDepth, limit,
			false, now, &remainingNodes,
		)
		if err != nil {
			return nil, err
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		return betterMatch(results[i], results[j])
	})
	return results, nil
}

func prepareMatchingCandidates(candidates []model.Transaction, upper int64) []model.Transaction {
	scanLimit := len(candidates)
	if scanLimit > maxMatchingCandidateFetch {
		scanLimit = maxMatchingCandidateFetch
	}
	capacity := scanLimit
	if capacity > maxMatchingCandidates {
		capacity = maxMatchingCandidates
	}
	prepared := make([]model.Transaction, 0, capacity)
	for i := 0; i < scanLimit && len(prepared) < maxMatchingCandidates; i++ {
		candidate := candidates[i]
		if candidate.BaseAmountCents == 0 {
			// Legacy cents-helper callers did not populate base cents.
			candidate.BaseAmountCents = candidate.AmountCents
		}
		if candidate.BaseAmountCents <= 0 || candidate.BaseAmountCents > upper {
			continue
		}
		prepared = append(prepared, candidate)
	}
	return prepared
}

type matchSearch struct {
	ctx            context.Context
	items          []model.Transaction
	ages           []float64
	suffix         []int64
	target         int64
	tolerance      int64
	upper          int64
	maxDepth       int
	limit          int
	timePruned     bool
	remainingNodes *int64
	results        []MatchResult
	picked         []int
	err            error
}

func searchMatchesCents(
	ctx context.Context,
	items []model.Transaction,
	targetCents int64,
	toleranceCents int64,
	maxDepth int,
	limit int,
	timePruned bool,
	now time.Time,
	remainingNodes *int64,
) ([]MatchResult, error) {
	sortedItems := append([]model.Transaction(nil), items...)
	sort.SliceStable(sortedItems, func(i, j int) bool {
		if sortedItems[i].BaseAmountCents != sortedItems[j].BaseAmountCents {
			return sortedItems[i].BaseAmountCents > sortedItems[j].BaseAmountCents
		}
		if !sortedItems[i].OccurredAt.Equal(sortedItems[j].OccurredAt) {
			return sortedItems[i].OccurredAt.Before(sortedItems[j].OccurredAt)
		}
		return sortedItems[i].ID < sortedItems[j].ID
	})

	upper := targetCents + toleranceCents
	suffix := make([]int64, len(sortedItems)+1)
	ages := make([]float64, len(sortedItems))
	for i := len(sortedItems) - 1; i >= 0; i-- {
		amount := sortedItems[i].BaseAmountCents
		if suffix[i+1] >= upper-amount {
			suffix[i] = upper
		} else {
			suffix[i] = suffix[i+1] + amount
		}
		ageDays := now.Sub(sortedItems[i].OccurredAt).Hours() / 24
		if ageDays < 0 {
			ageDays = 0
		} else if ageDays > 365 {
			ageDays = 365
		}
		ages[i] = ageDays
	}
	search := matchSearch{
		ctx: ctx, items: sortedItems, ages: ages, suffix: suffix,
		target: targetCents, tolerance: toleranceCents, upper: upper,
		maxDepth: maxDepth, limit: limit, timePruned: timePruned,
		remainingNodes: remainingNodes,
		results:        make([]MatchResult, 0, limit),
		picked:         make([]int, 0, maxDepth),
	}
	search.dfs(0, 0, 0)
	return search.results, search.err
}

func (s *matchSearch) dfs(index int, sum int64, totalAgeDays float64) {
	if s.err != nil || *s.remainingNodes <= 0 {
		return
	}
	(*s.remainingNodes)--
	if *s.remainingNodes%contextCheckInterval == 0 {
		if err := s.ctx.Err(); err != nil {
			s.err = err
			return
		}
	}
	if sum > s.upper {
		return
	}
	lower := s.target - s.tolerance
	if sum+s.suffix[index] < lower {
		return
	}

	absErr := sum - s.target
	if absErr < 0 {
		absErr = -absErr
	}
	if absErr <= s.tolerance && len(s.picked) > 0 {
		s.consider(sum, absErr, totalAgeDays)
	}
	if index >= len(s.items) || len(s.picked) == s.maxDepth {
		return
	}

	var previousAmount int64 = -1
	for i := index; i < len(s.items); i++ {
		amount := s.items[i].BaseAmountCents
		if amount == previousAmount {
			continue
		}
		previousAmount = amount
		if amount > s.upper-sum {
			continue
		}
		s.picked = append(s.picked, i)
		s.dfs(i+1, sum+amount, totalAgeDays+s.ages[i])
		s.picked = s.picked[:len(s.picked)-1]
		if s.err != nil || *s.remainingNodes <= 0 {
			return
		}
	}
}

func (s *matchSearch) consider(sum, absErr int64, totalAgeDays float64) {
	itemCount := len(s.picked)
	ageScore := (totalAgeDays / float64(itemCount)) / 365
	minScore := 1 / float64(itemCount)
	candidate := MatchResult{
		TotalCents: sum, AbsErrorCents: absErr,
		TotalYuan:    model.Money(sum) / 100,
		AbsErrorYuan: model.Money(absErr) / 100,
		ItemCount:    itemCount,
		Score:        0.6*minScore + 0.4*ageScore,
		TimePruned:   s.timePruned,
	}

	slot := len(s.results)
	if slot >= s.limit {
		slot = 0
		for i := 1; i < len(s.results); i++ {
			if betterMatch(s.results[slot], s.results[i]) {
				slot = i
			}
		}
		if !betterMatch(candidate, s.results[slot]) {
			return
		}
	}

	candidate.TransactionIDs = make([]string, itemCount)
	projects := make(map[string]struct{}, itemCount)
	for i, pickedIndex := range s.picked {
		transaction := s.items[pickedIndex]
		candidate.TransactionIDs[i] = transaction.ID
		if transaction.ProjectID != nil {
			projects[*transaction.ProjectID] = struct{}{}
		}
	}
	candidate.ProjectCount = len(projects)
	if len(s.results) < s.limit {
		s.results = append(s.results, candidate)
	} else {
		s.results[slot] = candidate
	}
}

func betterMatch(left, right MatchResult) bool {
	if left.AbsErrorCents != right.AbsErrorCents {
		return left.AbsErrorCents < right.AbsErrorCents
	}
	if left.Score != right.Score {
		return left.Score > right.Score
	}
	return left.ItemCount < right.ItemCount
}

// FindBestMatches is kept for backward compatibility with legacy Money values.
func FindBestMatches(
	candidates []model.Transaction,
	target model.Money,
	tolerance model.Money,
	maxDepth int,
	limit int,
) []MatchResult {
	copied := append([]model.Transaction(nil), candidates...)
	for i := range copied {
		if copied[i].AmountCents == 0 && copied[i].AmountYuan != 0 {
			amountCents, err := copied[i].AmountYuan.Cents()
			if err != nil {
				return nil
			}
			copied[i].AmountCents = amountCents
		}
		if copied[i].BaseAmountCents == 0 {
			copied[i].BaseAmountCents = copied[i].AmountCents
		}
	}
	targetCents, err := target.Cents()
	if err != nil {
		return nil
	}
	toleranceCents, err := tolerance.Cents()
	if err != nil {
		return nil
	}
	return FindBestMatchesCents(copied, targetCents, toleranceCents, maxDepth, limit)
}
