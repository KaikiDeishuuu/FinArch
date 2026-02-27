package test

import (
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
)

// TestFindBestMatches_OrderByErrorThenProjectCount verifies sorting strategy.
func TestFindBestMatches_OrderByErrorThenProjectCount(t *testing.T) {
	p1 := "P1"
	p2 := "P2"
	now := time.Now()
	candidates := []model.Transaction{
		{ID: "A", AmountYuan: 60, OccurredAt: now, ProjectID: &p1},
		{ID: "B", AmountYuan: 40, OccurredAt: now.Add(time.Second), ProjectID: &p1},
		{ID: "C", AmountYuan: 50, OccurredAt: now.Add(2 * time.Second), ProjectID: &p2},
		{ID: "D", AmountYuan: 49, OccurredAt: now.Add(3 * time.Second), ProjectID: &p1},
	}

	res := service.FindBestMatches(candidates, 100, 2, 3, 10)
	if len(res) == 0 {
		t.Fatal("expected non-empty result")
	}
	for i := 1; i < len(res); i++ {
		if res[i-1].AbsErrorYuan > res[i].AbsErrorYuan {
			t.Fatalf("result not sorted by error at %d", i)
		}
		if res[i-1].AbsErrorYuan == res[i].AbsErrorYuan && res[i-1].ProjectCount > res[i].ProjectCount {
			t.Fatalf("result not sorted by project count on same error at %d", i)
		}
	}
}
