package httpserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"
	"finarch/internal/domain/service"
)

// Server is the HTTP API server for FinArch.
type Server struct {
	txRepo   repository.TransactionRepository
	txSvc    *service.TransactionService
	reimSvc  *service.ReimbursementService
	matchSvc *service.MatchingService
	addr     string
}

// NewServer creates a new HTTP server.
func NewServer(
	addr string,
	txRepo repository.TransactionRepository,
	txSvc *service.TransactionService,
	reimSvc *service.ReimbursementService,
	matchSvc *service.MatchingService,
) *Server {
	return &Server{
		addr:     addr,
		txRepo:   txRepo,
		txSvc:    txSvc,
		reimSvc:  reimSvc,
		matchSvc: matchSvc,
	}
}

// Run starts the HTTP server.
func (s *Server) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/balance", s.handleBalance)
	mux.HandleFunc("/api/transactions", s.handleTransactions)
	mux.HandleFunc("/api/transactions/{id}/reimburse", s.handleToggleReimbursed)
	mux.HandleFunc("/api/match", s.handleMatch)
	mux.HandleFunc("/api/reimburse", s.handleReimburse)

	log.Printf("FinArch Web UI: http://%s\n", s.addr)
	return http.ListenAndServe(s.addr, corsMiddleware(mux))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleBalance returns fund pool balance.
func (s *Server) handleBalance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	company, personal, err := s.txSvc.GetBalances(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]float64{
		"company_balance_yuan":      company.Float64(),
		"personal_outstanding_yuan": personal.Float64(),
	})
}

// handleTransactions handles GET (list) and POST (create).
func (s *Server) handleTransactions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		txs, err := s.txRepo.ListAll(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		type txDTO struct {
			ID         string  `json:"id"`
			OccurredAt string  `json:"occurred_at"`
			Direction  string  `json:"direction"`
			Source     string  `json:"source"`
			Category   string  `json:"category"`
			AmountYuan float64 `json:"amount_yuan"`
			Currency   string  `json:"currency"`
			Note       string  `json:"note"`
			ProjectID  *string `json:"project_id"`
			Reimbursed bool    `json:"reimbursed"`
		}
		dtos := make([]txDTO, 0, len(txs))
		for _, t := range txs {
			dtos = append(dtos, txDTO{
				ID:         t.ID,
				OccurredAt: t.OccurredAt.Format("2006-01-02"),
				Direction:  string(t.Direction),
				Source:     string(t.Source),
				Category:   t.Category,
				AmountYuan: t.AmountYuan.Float64(),
				Currency:   t.Currency,
				Note:       t.Note,
				ProjectID:  t.ProjectID,
				Reimbursed: t.Reimbursed,
			})
		}
		writeJSON(w, http.StatusOK, dtos)

	case http.MethodPost:
		var req struct {
			OccurredAt string  `json:"occurred_at"` // YYYY-MM-DD
			Direction  string  `json:"direction"`
			Source     string  `json:"source"`
			Category   string  `json:"category"`
			AmountYuan float64 `json:"amount_yuan"`
			Currency   string  `json:"currency"`
			Note       string  `json:"note"`
			ProjectID  *string `json:"project_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
			return
		}
		var t time.Time
		if req.OccurredAt != "" {
			var err error
			t, err = time.Parse("2006-01-02", req.OccurredAt)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid occurred_at: use YYYY-MM-DD")
				return
			}
		} else {
			t = time.Now()
		}
		created, err := s.txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
			OccurredAt: t,
			Direction:  model.Direction(req.Direction),
			Source:     model.Source(req.Source),
			Category:   req.Category,
			AmountYuan: model.Money(req.AmountYuan),
			Currency:   req.Currency,
			Note:       req.Note,
			ProjectID:  req.ProjectID,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":          created.ID,
			"amount_yuan": created.AmountYuan.Float64(),
		})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleMatch performs subset-sum reimbursement search.
func (s *Server) handleMatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	ctx := r.Context()
	var req struct {
		TargetYuan    float64 `json:"target_yuan"`
		ToleranceYuan float64 `json:"tolerance_yuan"`
		MaxDepth      int     `json:"max_depth"`
		Limit         int     `json:"limit"`
		ProjectID     *string `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	results, err := s.matchSvc.Match(
		ctx,
		model.Money(req.TargetYuan),
		model.Money(req.ToleranceYuan),
		req.MaxDepth,
		req.ProjectID,
		req.Limit,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type resultDTO struct {
		TransactionIDs []string `json:"transaction_ids"`
		TotalYuan      float64  `json:"total_yuan"`
		AbsErrorYuan   float64  `json:"abs_error_yuan"`
		ProjectCount   int      `json:"project_count"`
		ItemCount      int      `json:"item_count"`
	}
	dtos := make([]resultDTO, 0, len(results))
	for _, res := range results {
		dtos = append(dtos, resultDTO{
			TransactionIDs: res.TransactionIDs,
			TotalYuan:      res.TotalYuan.Float64(),
			AbsErrorYuan:   res.AbsErrorYuan.Float64(),
			ProjectCount:   res.ProjectCount,
			ItemCount:      res.ItemCount,
		})
	}
	writeJSON(w, http.StatusOK, dtos)
}

// handleReimburse creates a reimbursement.
func (s *Server) handleReimburse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	ctx := r.Context()
	var req struct {
		Applicant      string   `json:"applicant"`
		TransactionIDs []string `json:"transaction_ids"`
		RequestNo      string   `json:"request_no"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	reim, err := s.reimSvc.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		Applicant:      req.Applicant,
		TransactionIDs: req.TransactionIDs,
		RequestNo:      req.RequestNo,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         reim.ID,
		"request_no": reim.RequestNo,
		"total_yuan": reim.TotalYuan.Float64(),
		"status":     reim.Status,
	})
}

// handleToggleReimbursed flips the reimbursed flag for a single transaction.
// PATCH /api/transactions/{id}/reimburse
func (s *Server) handleToggleReimbursed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "PATCH only")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing transaction id")
		return
	}
	newState, err := s.txRepo.ToggleReimbursed(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "reimbursed": newState})
}

// handleIndex serves the embedded single-page frontend.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, indexHTML)
}
