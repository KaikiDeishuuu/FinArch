package wails

import (
	"context"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
)

// App is Wails binding root.
type App struct {
	Matching       *service.MatchingService
	Transactions   *service.TransactionService
	Reimbursements *service.ReimbursementService
}

// MatchRequest is match input DTO for frontend.
type MatchRequest struct {
	TargetYuan    float64 `json:"targetYuan"`
	ToleranceYuan float64 `json:"toleranceYuan"`
	MaxDepth      int     `json:"maxDepth"`
	Limit         int     `json:"limit"`
	ProjectID     *string `json:"projectId"`
}

// ReimburseRequest is reimbursement input DTO for frontend.
type ReimburseRequest struct {
	Applicant      string   `json:"applicant"`
	TransactionIDs []string `json:"transactionIds"`
	RequestNo      string   `json:"requestNo"`
}

// MatchReimbursement suggests reimbursement combinations.
func (a *App) MatchReimbursement(ctx context.Context, req MatchRequest) ([]service.MatchResult, error) {
	// Desktop mode is single-user; use empty string as the user scope.
	return a.Matching.Match(ctx, "", model.Money(req.TargetYuan), model.Money(req.ToleranceYuan), req.MaxDepth, req.ProjectID, req.Limit)
}

// CreateReimbursement submits one reimbursement.
func (a *App) CreateReimbursement(ctx context.Context, req ReimburseRequest) (model.Reimbursement, error) {
	return a.Reimbursements.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		Applicant:      req.Applicant,
		TransactionIDs: req.TransactionIDs,
		RequestNo:      req.RequestNo,
	})
}

// GetBalance returns company balance and personal outstanding.
func (a *App) GetBalance(ctx context.Context) (map[string]float64, error) {
	// Desktop mode is single-user; use empty string as the user scope.
	company, personal, err := a.Transactions.GetBalances(ctx, "")
	if err != nil {
		return nil, err
	}
	return map[string]float64{
		"companyBalanceYuan":      company.Float64(),
		"personalOutstandingYuan": personal.Float64(),
	}, nil
}
