package wails

import (
	"context"
	"strings"
	"testing"
)

func TestCreateReimbursementRequiresConfiguredUserScope(t *testing.T) {
	app := &App{}
	_, err := app.CreateReimbursement(context.Background(), ReimburseRequest{
		Applicant: "alice", TransactionIDs: []string{"tx-1"},
	})
	if err == nil || !strings.Contains(err.Error(), "user scope") {
		t.Fatalf("expected missing desktop user scope error, got %v", err)
	}
}

func TestAllDesktopOperationsRequireConfiguredUserScope(t *testing.T) {
	app := &App{}
	if _, err := app.MatchReimbursement(context.Background(), MatchRequest{}); err == nil {
		t.Fatal("matching accepted an empty desktop user scope")
	}
	if _, err := app.GetBalance(context.Background()); err == nil {
		t.Fatal("balance accepted an empty desktop user scope")
	}
}
