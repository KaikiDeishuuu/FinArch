package service

import (
	"context"
	"sort"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"
)

// MatchResult represents one matching combination candidate.
type MatchResult struct {
	TransactionIDs []string
	TotalYuan      model.Money
	AbsErrorYuan   model.Money
	ProjectCount   int
	ItemCount      int
}

// MatchingService finds reimbursement combinations from unreimbursed personal expenses.
type MatchingService struct {
	transactions repository.TransactionRepository
}

// NewMatchingService creates a new MatchingService.
func NewMatchingService(transactions repository.TransactionRepository) *MatchingService {
	return &MatchingService{transactions: transactions}
}

// Match finds candidate combinations sorted by minimum error then minimum project count then item count.
func (s *MatchingService) Match(
	ctx context.Context,
	userID string,
	target model.Money,
	tolerance model.Money,
	maxDepth int,
	projectID *string,
	limit int,
) ([]MatchResult, error) {
	if maxDepth <= 0 {
		maxDepth = 6
	}
	if limit <= 0 {
		limit = 20
	}
	candidates, err := s.transactions.ListUnreimbursedPersonalExpenses(ctx, userID, projectID, 2000)
	if err != nil {
		return nil, err
	}
	return FindBestMatches(candidates, target, tolerance, maxDepth, limit), nil
}

// FindBestMatches finds combinations with pruning optimization.
func FindBestMatches(
	candidates []model.Transaction,
	target model.Money,
	tolerance model.Money,
	maxDepth int,
	limit int,
) []MatchResult {
	if len(candidates) == 0 || maxDepth <= 0 || limit <= 0 {
		return nil
	}

	sorted := make([]model.Transaction, len(candidates))
	copy(sorted, candidates)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].AmountYuan == sorted[j].AmountYuan {
			return sorted[i].OccurredAt.Before(sorted[j].OccurredAt)
		}
		return sorted[i].AmountYuan > sorted[j].AmountYuan
	})

	suffix := make([]model.Money, len(sorted)+1)
	for i := len(sorted) - 1; i >= 0; i-- {
		suffix[i] = suffix[i+1] + sorted[i].AmountYuan
	}

	results := make([]MatchResult, 0, limit*2)
	pickedIdx := make([]int, 0, maxDepth)
	projectSet := make(map[string]struct{})

	var dfs func(index int, sum model.Money)
	dfs = func(index int, sum model.Money) {
		if len(pickedIdx) > maxDepth {
			return
		}
		if sum > target+tolerance {
			return
		}
		if sum+suffix[index] < target-tolerance {
			return
		}

		err := (sum - target).Abs()
		if err <= tolerance && len(pickedIdx) > 0 {
			ids := make([]string, 0, len(pickedIdx))
			projectSet = make(map[string]struct{})
			for _, idx := range pickedIdx {
				ids = append(ids, sorted[idx].ID)
				if sorted[idx].ProjectID != nil {
					projectSet[*sorted[idx].ProjectID] = struct{}{}
				}
			}
			results = append(results, MatchResult{
				TransactionIDs: ids,
				TotalYuan:      sum,
				AbsErrorYuan:   err,
				ProjectCount:   len(projectSet),
				ItemCount:      len(ids),
			})
		}

		if index >= len(sorted) || len(pickedIdx) == maxDepth {
			return
		}

		var previous model.Money
		hasPrevious := false
		for i := index; i < len(sorted); i++ {
			if hasPrevious && (sorted[i].AmountYuan-previous).Abs() <= 1e-9 {
				continue
			}
			previous = sorted[i].AmountYuan
			hasPrevious = true
			pickedIdx = append(pickedIdx, i)
			dfs(i+1, sum+sorted[i].AmountYuan)
			pickedIdx = pickedIdx[:len(pickedIdx)-1]
		}
	}

	dfs(0, 0)

	sort.Slice(results, func(i, j int) bool {
		if results[i].AbsErrorYuan != results[j].AbsErrorYuan {
			return results[i].AbsErrorYuan < results[j].AbsErrorYuan
		}
		if results[i].ProjectCount != results[j].ProjectCount {
			return results[i].ProjectCount < results[j].ProjectCount
		}
		if results[i].ItemCount != results[j].ItemCount {
			return results[i].ItemCount < results[j].ItemCount
		}
		return results[i].TotalYuan < results[j].TotalYuan
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results
}
