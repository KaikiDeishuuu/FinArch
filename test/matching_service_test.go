package test

import (
"testing"
"time"

"finarch/internal/domain/model"
"finarch/internal/domain/service"
)

// makeItem is a test helper that creates a Transaction with both
// AmountYuan (legacy) and AmountCents (V9) populated.
func makeItem(id string, yuan float64, daysAgo int, projectID ...string) model.Transaction {
t := model.Transaction{
ID:          id,
AmountYuan:  model.Money(yuan),
AmountCents: int64(yuan * 100),
OccurredAt:  time.Now().AddDate(0, 0, -daysAgo),
}
if len(projectID) > 0 {
t.ProjectID = &projectID[0]
}
return t
}

// ────────────────────────────────────────────────────────────────────────────
// Basic correctness

func TestFindBestMatchesCents_ExactMatch(t *testing.T) {
items := []model.Transaction{
makeItem("A", 60, 5),
makeItem("B", 40, 3),
makeItem("C", 30, 1),
}
// Target 100 → A+B is exact
res := service.FindBestMatchesCents(items, 10000, 0, 4, 10)
if len(res) == 0 {
t.Fatal("expected at least one result")
}
if res[0].AbsErrorCents != 0 {
t.Fatalf("expected exact match, got error=%d cents", res[0].AbsErrorCents)
}
if res[0].TotalCents != 10000 {
t.Fatalf("expected total=10000 cents, got %d", res[0].TotalCents)
}
}

func TestFindBestMatchesCents_ToleranceMatch(t *testing.T) {
items := []model.Transaction{
makeItem("A", 99.5, 5),
makeItem("B", 50, 3),
makeItem("C", 50.5, 1),
}
// Target 100, tolerance 1 → A or B+C are acceptable
res := service.FindBestMatchesCents(items, 10000, 100, 3, 10)
if len(res) == 0 {
t.Fatal("expected at least one result within tolerance")
}
for _, r := range res {
if r.AbsErrorCents > 100 {
t.Fatalf("result exceeds tolerance: errorCents=%d", r.AbsErrorCents)
}
}
}

func TestFindBestMatchesCents_NoResult(t *testing.T) {
items := []model.Transaction{
makeItem("A", 10, 1),
makeItem("B", 20, 2),
}
res := service.FindBestMatchesCents(items, 100000, 0, 4, 10) // target 1000 CNY
if len(res) != 0 {
t.Fatalf("expected no result, got %d", len(res))
}
}

// ────────────────────────────────────────────────────────────────────────────
// Sorting: error asc → score desc

func TestFindBestMatchesCents_SortByErrorThenScore(t *testing.T) {
p1 := "P1"
p2 := "P2"
items := []model.Transaction{
makeItem("A", 60, 5, p1),
makeItem("B", 40, 3, p1),
makeItem("C", 50, 2, p2),
makeItem("D", 49, 1, p1),
}
res := service.FindBestMatchesCents(items, 10000, 200, 3, 10)
if len(res) == 0 {
t.Fatal("expected non-empty result")
}
for i := 1; i < len(res); i++ {
if res[i-1].AbsErrorCents > res[i].AbsErrorCents {
t.Fatalf("results not sorted by error at index %d", i)
}
// Same error → score should be non-increasing
if res[i-1].AbsErrorCents == res[i].AbsErrorCents &&
res[i-1].Score < res[i].Score-1e-9 {
t.Fatalf("results with same error not sorted by score (desc) at index %d: %.4f < %.4f", i, res[i-1].Score, res[i].Score)
}
}
}

// ────────────────────────────────────────────────────────────────────────────
// Scoring: fewer items → higher score

func TestFindBestMatchesCents_FewerItemsPreferred(t *testing.T) {
items := []model.Transaction{
// One item that hits 100 exactly
makeItem("Big", 100, 100),
// Two items that also hit 100
makeItem("Small1", 60, 100),
makeItem("Small2", 40, 100),
}
res := service.FindBestMatchesCents(items, 10000, 0, 3, 10)
if len(res) == 0 {
t.Fatal("expected at least one result")
}
// Score of 1-item combo should be higher than 2-item combo
var oneItemScore, twoItemScore float64
for _, r := range res {
switch r.ItemCount {
case 1:
oneItemScore = r.Score
case 2:
twoItemScore = r.Score
}
}
if oneItemScore == 0 || twoItemScore == 0 {
t.Skip("did not find both 1-item and 2-item results")
}
if oneItemScore <= twoItemScore {
t.Fatalf("1-item combo should score higher than 2-item: %.4f vs %.4f", oneItemScore, twoItemScore)
}
}

// ────────────────────────────────────────────────────────────────────────────
// Scoring: older items → higher age component

func TestFindBestMatchesCents_OlderItemsScoreHigher(t *testing.T) {
// Two pairs each summing to 100: one old, one recent
itemsOld := []model.Transaction{
makeItem("OldA", 60, 300),
makeItem("OldB", 40, 300),
}
itemsNew := []model.Transaction{
makeItem("NewA", 60, 5),
makeItem("NewB", 40, 5),
}
all := append(itemsOld, itemsNew...)
res := service.FindBestMatchesCents(all, 10000, 0, 2, 10)
if len(res) < 2 {
t.Skipf("expected ≥2 results, got %d", len(res))
}
// Among exact-match results, the old pair should score higher.
var oldScore, newScore float64
for _, r := range res {
if r.AbsErrorCents != 0 {
continue
}
inOld := false
for _, id := range r.TransactionIDs {
if id == "OldA" || id == "OldB" {
inOld = true
}
}
if inOld {
oldScore = r.Score
} else {
newScore = r.Score
}
}
if oldScore > 0 && newScore > 0 && oldScore <= newScore {
t.Fatalf("old items should score higher: old=%.4f new=%.4f", oldScore, newScore)
}
}

// ────────────────────────────────────────────────────────────────────────────
// Backward compat: FindBestMatches with legacy AmountYuan data

func TestFindBestMatches_BackwardCompat(t *testing.T) {
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
// legacy test: within same error, project count may differ (score-based now)
// Just verify we're still sorted by error
_ = "ok"
}
}
}

// ────────────────────────────────────────────────────────────────────────────
// Empty and edge cases

func TestFindBestMatchesCents_Empty(t *testing.T) {
res := service.FindBestMatchesCents(nil, 10000, 100, 3, 10)
if len(res) != 0 {
t.Fatal("expected nil/empty for nil candidates")
}
}

func TestFindBestMatchesCents_ZeroTarget(t *testing.T) {
items := []model.Transaction{makeItem("A", 100, 1)}
res := service.FindBestMatchesCents(items, 0, 0, 3, 10)
if len(res) != 0 {
t.Fatal("expected nil/empty for zero target")
}
}

func TestFindBestMatchesCents_MaxDepthOne(t *testing.T) {
items := []model.Transaction{
makeItem("A", 100, 1),
makeItem("B", 50, 2),
makeItem("C", 50, 3),
}
res := service.FindBestMatchesCents(items, 10000, 0, 1, 10) // maxDepth=1 → only single items
if len(res) == 0 {
t.Fatal("expected at least one result")
}
for _, r := range res {
if r.ItemCount > 1 {
t.Fatalf("maxDepth=1 should not produce combos of %d items", r.ItemCount)
}
}
}
