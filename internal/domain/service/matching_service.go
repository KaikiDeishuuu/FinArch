package service

import (
"context"
"sort"
"time"

"finarch/internal/domain/model"
"finarch/internal/domain/repository"
)

const (
// dpThreshold: if N × targetCents > dpThreshold, activate time-pruning.
dpThreshold = int64(1_000_000_000)

// timePruneDays: fallback window when N×W exceeds threshold.
timePruneDays = 90

// maxMatchDepth is a hard upper bound on the number of items considered in a match.
// It prevents excessive memory allocation from untrusted maxDepth values.
maxMatchDepth = 50
)

// MatchResult represents one matching combination candidate.
type MatchResult struct {
TransactionIDs []string
TotalCents     int64       // precise integer cents
AbsErrorCents  int64       // |total - target| in cents
TotalYuan      model.Money // derived: TotalCents/100  (backward compat)
AbsErrorYuan   model.Money // derived: AbsErrorCents/100 (backward compat)
ProjectCount   int
ItemCount      int
Score          float64 // multi-objective score — higher is better
TimePruned     bool    // true if result came from 90-day pruned set
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
if maxDepth <= 0 {
maxDepth = 6
} else if maxDepth > maxMatchDepth {
maxDepth = maxMatchDepth
}
if limit <= 0 {
limit = 20
}
candidates, err := s.transactions.ListUnreimbursedPersonalExpenses(ctx, userID, projectID, 2000)
if err != nil {
return nil, err
}
targetCents := int64(targetYuan * 100)
toleranceCents := int64(toleranceYuan * 100)
return FindBestMatchesCents(candidates, targetCents, toleranceCents, maxDepth, limit), nil
}

// FindBestMatchesCents is the primary matching algorithm using integer-cent arithmetic.
//
// Strategy:
//   - N × targetCents <= dpThreshold: DFS+backtracking over all candidates.
//   - N × targetCents >  dpThreshold: try DFS on last-90-days subset first (time-pruning
//     Fallback 2); if no results found, retry on full set.
//
// Results scored by multi-objective function (fewer items + older receipts = higher score).
// Top-3 best-scored solutions among equally-close matches appear first.
func FindBestMatchesCents(
candidates []model.Transaction,
targetCents int64,
toleranceCents int64,
maxDepth int,
limit int,
) []MatchResult {
if len(candidates) == 0 || maxDepth <= 0 || limit <= 0 || targetCents <= 0 {
return nil
}

// Ensure maxDepth is within safe, bounded limits before any allocations.
if maxDepth > maxMatchDepth {
	maxDepth = maxMatchDepth
} else if maxDepth < 1 {
	maxDepth = 1
}

N := int64(len(candidates))
workSet := candidates
timePruned := false

if N*targetCents > dpThreshold {
// Fallback 2 – time pruning: restrict to last timePruneDays days.
cutoff := time.Now().AddDate(0, 0, -timePruneDays)
pruned := make([]model.Transaction, 0, len(candidates))
for _, c := range candidates {
if c.OccurredAt.After(cutoff) {
pruned = append(pruned, c)
}
}
if len(pruned) > 0 {
workSet = pruned
timePruned = true
}
}

results := dfsCents(workSet, targetCents, toleranceCents, maxDepth)

// If time-pruned set yielded nothing, retry on the full set.
if len(results) == 0 && timePruned {
results = dfsCents(candidates, targetCents, toleranceCents, maxDepth)
timePruned = false
}
if len(results) == 0 {
return nil
}

// Build index for age-scoring.
txIndex := make(map[string]model.Transaction, len(candidates))
for _, c := range candidates {
txIndex[c.ID] = c
}

now := time.Now()
for i := range results {
var totalDays float64
for _, id := range results[i].TransactionIDs {
if t, ok := txIndex[id]; ok {
totalDays += now.Sub(t.OccurredAt).Hours() / 24
}
}
avgDays := totalDays / float64(results[i].ItemCount)
ageScore := avgDays / 365.0
if ageScore > 1.0 {
ageScore = 1.0
}
// Weight: 60% minimalist (fewer items) + 40% age preference (older receipts first).
minScore := 1.0 / float64(results[i].ItemCount)
results[i].Score = 0.6*minScore + 0.4*ageScore
results[i].TimePruned = timePruned
results[i].TotalYuan = model.Money(results[i].TotalCents) / 100
results[i].AbsErrorYuan = model.Money(results[i].AbsErrorCents) / 100
}

// Primary: absolute error asc. Secondary: score desc. Tertiary: item count asc.
sort.Slice(results, func(i, j int) bool {
if results[i].AbsErrorCents != results[j].AbsErrorCents {
return results[i].AbsErrorCents < results[j].AbsErrorCents
}
if results[i].Score != results[j].Score {
return results[i].Score > results[j].Score
}
return results[i].ItemCount < results[j].ItemCount
})

if len(results) > limit {
results = results[:limit]
}
return results
}

// dfsCents runs Fallback 1: greedy-sorted DFS with backtracking.
// Items are sorted by amount desc (large items tried first) and date asc (older first on tie).
func dfsCents(
items []model.Transaction,
targetCents int64,
toleranceCents int64,
maxDepth int,
) []MatchResult {
sorted := make([]model.Transaction, len(items))
copy(sorted, items)
sort.Slice(sorted, func(i, j int) bool {
if sorted[i].AmountCents != sorted[j].AmountCents {
return sorted[i].AmountCents > sorted[j].AmountCents
}
return sorted[i].OccurredAt.Before(sorted[j].OccurredAt)
})

suffix := make([]int64, len(sorted)+1)
for i := len(sorted) - 1; i >= 0; i-- {
suffix[i] = suffix[i+1] + sorted[i].AmountCents
}

var results []MatchResult
pickedIdx := make([]int, 0, maxDepth)

var dfs func(index int, sum int64)
dfs = func(index int, sum int64) {
if len(pickedIdx) > maxDepth {
return
}
if sum > targetCents+toleranceCents {
return
}
if sum+suffix[index] < targetCents-toleranceCents {
return
}

absErr := sum - targetCents
if absErr < 0 {
absErr = -absErr
}
if absErr <= toleranceCents && len(pickedIdx) > 0 {
ids := make([]string, len(pickedIdx))
for k, idx := range pickedIdx {
ids[k] = sorted[idx].ID
}
projectSet := make(map[string]struct{})
for _, idx := range pickedIdx {
if sorted[idx].ProjectID != nil {
projectSet[*sorted[idx].ProjectID] = struct{}{}
}
}
results = append(results, MatchResult{
TransactionIDs: ids,
TotalCents:     sum,
AbsErrorCents:  absErr,
ProjectCount:   len(projectSet),
ItemCount:      len(pickedIdx),
})
}

if index >= len(sorted) || len(pickedIdx) == maxDepth {
return
}

var prevCents int64 = -1
for i := index; i < len(sorted); i++ {
if sorted[i].AmountCents == prevCents {
continue
}
prevCents = sorted[i].AmountCents
pickedIdx = append(pickedIdx, i)
dfs(i+1, sum+sorted[i].AmountCents)
pickedIdx = pickedIdx[:len(pickedIdx)-1]
}
}

dfs(0, 0)
return results
}

// FindBestMatches is kept for backward compatibility with existing tests.
// Delegates to FindBestMatchesCents after filling AmountCents from AmountYuan for legacy data.
func FindBestMatches(
candidates []model.Transaction,
target model.Money,
tolerance model.Money,
maxDepth int,
limit int,
) []MatchResult {
for i := range candidates {
if candidates[i].AmountCents == 0 && candidates[i].AmountYuan != 0 {
candidates[i].AmountCents = int64(candidates[i].AmountYuan * 100)
}
}
return FindBestMatchesCents(candidates, int64(target*100), int64(tolerance*100), maxDepth, limit)
}
