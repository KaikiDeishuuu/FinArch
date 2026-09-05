package test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"finarch/internal/domain/model"
	domainrepo "finarch/internal/domain/repository"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	sqliterepo "finarch/internal/infrastructure/repository"

	"github.com/google/uuid"
)

func TestAccountBalanceHistorySupportsSelectedForeignAccount(t *testing.T) {
	tests := []struct {
		name           string
		baseCurrency   string
		sourceCurrency string
		rate           float64
		firstBalance   float64
		finalBalance   float64
	}{
		{name: "USD account", baseCurrency: "USD", sourceCurrency: "EUR", rate: 2, firstBalance: 20, finalBalance: 15},
		{name: "EUR account", baseCurrency: "EUR", sourceCurrency: "USD", rate: 0.5, firstBalance: 5, finalBalance: 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			database := setupDB(t)
			defer database.Close()
			ctx := context.Background()
			txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
			accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
			tm := sqliterepo.NewSQLiteTransactionManager(database)
			accountService := service.NewAccountService(accountRepo, txRepo, tm)
			transactionService := service.NewTransactionService(txRepo, accountRepo, nil)

			account, err := accountService.CreateAccount(
				ctx, testUserID, tc.name, model.AccountTypePublic, tc.baseCurrency, model.ModeWork,
			)
			if err != nil {
				t.Fatal(err)
			}
			firstDay := time.Date(2025, time.January, 2, 12, 0, 0, 0, time.UTC)
			if _, err := transactionService.CreateTransaction(ctx, service.CreateTransactionRequest{
				UserID: testUserID, AccountID: account.ID, TxType: model.TxTypeIncome,
				Mode: model.ModeWork, Category: "income", AmountCents: 1_000,
				Currency: tc.sourceCurrency, ExchangeRate: tc.rate, OccurredAt: firstDay,
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := transactionService.CreateTransaction(ctx, service.CreateTransactionRequest{
				UserID: testUserID, AccountID: account.ID, TxType: model.TxTypeExpense,
				Mode: model.ModeWork, Category: "expense", AmountCents: int64((tc.firstBalance - tc.finalBalance) * 100),
				Currency: tc.baseCurrency, OccurredAt: firstDay.AddDate(0, 0, 1),
			}); err != nil {
				t.Fatal(err)
			}

			stats := service.NewStatsService(database)
			points, err := stats.AccountBalanceHistory(ctx, testUserID, model.ModeWork, "all", account.ID)
			if err != nil {
				t.Fatalf("selected %s history: %v", tc.baseCurrency, err)
			}
			if len(points) != 2 || points[0].Balance != tc.firstBalance || points[1].Balance != tc.finalBalance {
				t.Fatalf("unexpected %s history: %#v", tc.baseCurrency, points)
			}
			if _, err := stats.AccountBalanceHistory(ctx, testUserID, model.ModeWork, "all", ""); !errors.Is(err, domainrepo.ErrMultiCurrencyReportingUnavailable) {
				t.Fatalf("cross-account history error = %v", err)
			}

			jwtService := auth.NewJWTService("test-secret")
			server := newTestServer(t, database, jwtService)
			token := issueTestAccessSession(t, database, jwtService, testUserID)
			req := httptest.NewRequest(
				http.MethodGet,
				"/api/v1/stats/account-balance-history?mode=work&range=all&account_id="+account.ID,
				nil,
			)
			req.Header.Set("Authorization", "Bearer "+token)
			response := serveTestRequest(server, req)
			if response.Code != http.StatusOK {
				t.Fatalf("selected-account API status=%d body=%s", response.Code, response.Body.String())
			}
			var body struct {
				Success bool                          `json:"success"`
				Data    []service.BalanceHistoryPoint `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if !body.Success || len(body.Data) != 2 || body.Data[1].Balance != tc.finalBalance {
				t.Fatalf("unexpected selected-account API body: %s", response.Body.String())
			}

			aggregateReq := httptest.NewRequest(
				http.MethodGet,
				"/api/v1/stats/account-balance-history?mode=work&range=all",
				nil,
			)
			aggregateReq.Header.Set("Authorization", "Bearer "+token)
			aggregateResponse := serveTestRequest(server, aggregateReq)
			if aggregateResponse.Code != http.StatusUnprocessableEntity || apiErrorCode(t, aggregateResponse) != "multi_currency_reporting_unavailable" {
				t.Fatalf("aggregate history API status=%d body=%s", aggregateResponse.Code, aggregateResponse.Body.String())
			}
		})
	}
}

func TestDeleteAccountWithAnyTransactionHistoryIsConflict(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	accountService := service.NewAccountService(accountRepo, txRepo, tm)
	transactionService := service.NewTransactionService(txRepo, accountRepo, nil)

	account, err := accountService.CreateAccount(
		ctx, testUserID, "historical", model.AccountTypePublic, "CNY", model.ModeWork,
	)
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := transactionService.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, AccountID: account.ID, TxType: model.TxTypeIncome,
		Mode: model.ModeWork, Category: "income", AmountCents: 1_000,
		Currency: "CNY", OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if transaction.ReimbStatus != model.ReimbStatusNone {
		t.Fatalf("test requires non-pending history, got %s", transaction.ReimbStatus)
	}
	hasHistory, err := txRepo.HasTransactionsByAccount(ctx, account.ID, testUserID)
	if err != nil || !hasHistory {
		t.Fatalf("history query = %v, %v", hasHistory, err)
	}

	err = accountService.DeleteAccount(ctx, account.ID, testUserID)
	if !errors.Is(err, service.ErrResourceConflict) {
		t.Fatalf("delete service error = %v", err)
	}

	jwtService := auth.NewJWTService("test-secret")
	server := newTestServer(t, database, jwtService)
	token := issueTestAccessSession(t, database, jwtService, testUserID)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/"+account.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response := serveTestRequest(server, req)
	if response.Code != http.StatusConflict || apiErrorCode(t, response) != "resource_conflict" {
		t.Fatalf("delete API status=%d body=%s", response.Code, response.Body.String())
	}

	var active int
	if err := database.QueryRowContext(ctx, `SELECT is_active FROM accounts WHERE id = ?`, account.ID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatal("account with transaction history was deactivated")
	}
}

type staleActiveAccountRepository struct {
	domainrepo.AccountRepository
}

func (r staleActiveAccountRepository) GetByID(ctx context.Context, id string) (model.Account, error) {
	account, err := r.AccountRepository.GetByID(ctx, id)
	if err == nil {
		account.IsActive = true
	}
	return account, err
}

func TestTransactionInsertRechecksAccountActiveAtomically(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	account, err := service.NewAccountService(accountRepo, txRepo, tm).CreateAccount(
		ctx, testUserID, "stale-active", model.AccountTypePublic, "CNY", model.ModeWork,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE accounts SET is_active = 0 WHERE id = ?`, account.ID); err != nil {
		t.Fatal(err)
	}

	staleAccounts := staleActiveAccountRepository{AccountRepository: accountRepo}
	_, err = service.NewTransactionService(txRepo, staleAccounts, nil).CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, AccountID: account.ID, TxType: model.TxTypeIncome,
		Mode: model.ModeWork, Category: "stale", AmountCents: 100,
		Currency: "CNY", OccurredAt: time.Now(),
	})
	if err == nil {
		t.Fatal("transaction was inserted into an account deactivated after validation")
	}
	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE account_id = ?`, account.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("inactive account received %d transactions", count)
	}
}

func TestReimbursementRejectsLifeModeTransaction(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	transactionService := service.NewTransactionService(txRepo, accountRepo, nil)
	reimbursementService := service.NewReimbursementService(tm, txRepo, sqliterepo.NewSQLiteReimbursementRepository(database))

	transaction, err := transactionService.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, Direction: model.DirectionExpense, Source: model.SourcePersonal,
		Mode: model.ModeLife, Category: "生活支出", AmountCents: 1_000,
		Currency: "CNY", OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if transaction.ReimbStatus != model.ReimbStatusNone {
		t.Fatalf("LIFE transaction status = %s", transaction.ReimbStatus)
	}
	_, err = reimbursementService.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		UserID: testUserID, Applicant: "alice", TransactionIDs: []string{transaction.ID},
		RequestNo: "life-" + uuid.NewString(),
	})
	if err == nil {
		t.Fatal("LIFE transaction was reimbursed")
	}
	assertNoReimbursementArtifacts(t, database, transaction.ID, model.ReimbStatusNone)
}

func TestReimbursementUsesCNYBaseCentsThroughAPI(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	transactionService := service.NewTransactionService(txRepo, accountRepo, nil)

	euroTransaction, err := transactionService.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, Direction: model.DirectionExpense, Source: model.SourcePersonal,
		Mode: model.ModeWork, Category: "差旅", AmountCents: 1_000,
		Currency: "EUR", ExchangeRate: 7.5, OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	cnyTransaction, err := transactionService.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, Direction: model.DirectionExpense, Source: model.SourcePersonal,
		Mode: model.ModeWork, Category: "材料", AmountCents: 125,
		Currency: "CNY", OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	payload, err := json.Marshal(map[string]any{
		"applicant":       "alice",
		"transaction_ids": []string{euroTransaction.ID, cnyTransaction.ID},
		"request_no":      "base-cents-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	jwtService := auth.NewJWTService("test-secret")
	server := newTestServer(t, database, jwtService)
	token := issueTestAccessSession(t, database, jwtService, testUserID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reimbursements", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response := serveTestRequest(server, req)
	if response.Code != http.StatusCreated {
		t.Fatalf("reimbursement API status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			ID         string  `json:"id"`
			TotalCents int64   `json:"total_cents"`
			TotalYuan  float64 `json:"total_yuan"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Success || body.Data.TotalCents != 7_625 || body.Data.TotalYuan != 76.25 {
		t.Fatalf("normalized reimbursement response: %s", response.Body.String())
	}

	var total, euroItem float64
	var totalCents, euroItemCents int64
	if err := database.QueryRowContext(ctx, `SELECT total_yuan, total_cents FROM reimbursements WHERE id = ?`, body.Data.ID).Scan(&total, &totalCents); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT amount_yuan, amount_cents FROM reimbursement_items WHERE reimbursement_id = ? AND transaction_id = ?`, body.Data.ID, euroTransaction.ID).Scan(&euroItem, &euroItemCents); err != nil {
		t.Fatal(err)
	}
	if total != 76.25 || totalCents != 7_625 || euroItem != 75 || euroItemCents != 7_500 {
		t.Fatalf("stored normalized amounts: total=%v/%d EUR item=%v/%d", total, totalCents, euroItem, euroItemCents)
	}
}

func TestReimbursementPersistsLargeCanonicalCents(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	transactionService := service.NewTransactionService(txRepo, accountRepo, nil)
	const amountCents int64 = model.MaxTransactionAmountCents

	transaction, err := transactionService.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, Direction: model.DirectionExpense, Source: model.SourcePersonal,
		Mode: model.ModeWork, Category: "large", AmountCents: amountCents,
		Currency: "CNY", OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"applicant":       "alice",
		"transaction_ids": []string{transaction.ID},
		"request_no":      "large-cents-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	jwtService := auth.NewJWTService("test-secret")
	server := newTestServer(t, database, jwtService)
	token := issueTestAccessSession(t, database, jwtService, testUserID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reimbursements", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response := serveTestRequest(server, req)
	if response.Code != http.StatusCreated {
		t.Fatalf("large reimbursement API status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			ID         string `json:"id"`
			TotalCents int64  `json:"total_cents"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.TotalCents != amountCents {
		t.Fatalf("API canonical cents=%d want=%d body=%s", body.Data.TotalCents, amountCents, response.Body.String())
	}
	var storedTotal, storedItem int64
	if err := database.QueryRowContext(ctx, `SELECT total_cents FROM reimbursements WHERE id = ?`, body.Data.ID).Scan(&storedTotal); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT amount_cents FROM reimbursement_items WHERE reimbursement_id = ? AND transaction_id = ?`, body.Data.ID, transaction.ID).Scan(&storedItem); err != nil {
		t.Fatal(err)
	}
	if storedTotal != amountCents || storedItem != amountCents {
		t.Fatalf("stored canonical cents total=%d item=%d want=%d", storedTotal, storedItem, amountCents)
	}
}

func TestReimbursementRejectsIncompatibleBaseCurrencies(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	accountService := service.NewAccountService(accountRepo, txRepo, tm)
	transactionService := service.NewTransactionService(txRepo, accountRepo, nil)
	reimbursementService := service.NewReimbursementService(tm, txRepo, sqliterepo.NewSQLiteReimbursementRepository(database))

	cnyTransaction, err := transactionService.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, Direction: model.DirectionExpense, Source: model.SourcePersonal,
		Mode: model.ModeWork, Category: "CNY", AmountCents: 100,
		Currency: "CNY", OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	usdAccount, err := accountService.CreateAccount(
		ctx, testUserID, "USD personal", model.AccountTypePersonal, "USD", model.ModeLife,
	)
	if err != nil {
		t.Fatal(err)
	}
	usdTransaction, err := transactionService.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, AccountID: usdAccount.ID, TxType: model.TxTypeExpense,
		Mode: model.ModeWork, Category: "USD", AmountCents: 100,
		Currency: "USD", OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = reimbursementService.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		UserID: testUserID, Applicant: "alice",
		TransactionIDs: []string{cnyTransaction.ID, usdTransaction.ID},
		RequestNo:      "mixed-base-" + uuid.NewString(),
	})
	if !errors.Is(err, domainrepo.ErrMultiCurrencyReportingUnavailable) {
		t.Fatalf("mixed-base reimbursement error = %v", err)
	}
	jwtService := auth.NewJWTService("test-secret")
	server := newTestServer(t, database, jwtService)
	token := issueTestAccessSession(t, database, jwtService, testUserID)
	payload, err := json.Marshal(map[string]any{
		"applicant":       "alice",
		"transaction_ids": []string{cnyTransaction.ID, usdTransaction.ID},
		"request_no":      "mixed-base-api-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reimbursements", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response := serveTestRequest(server, req)
	if response.Code != http.StatusUnprocessableEntity || apiErrorCode(t, response) != "multi_currency_reporting_unavailable" {
		t.Fatalf("mixed-base API status=%d body=%s", response.Code, response.Body.String())
	}
	assertNoReimbursementArtifacts(t, database, cnyTransaction.ID, model.ReimbStatusPending)
	assertNoReimbursementArtifacts(t, database, usdTransaction.ID, model.ReimbStatusPending)
}

type stalePendingTransactionRepository struct {
	domainrepo.TransactionRepository
	markCalled bool
}

func (r *stalePendingTransactionRepository) GetByIDs(ctx context.Context, userID string, ids []string) ([]model.Transaction, error) {
	transactions, err := r.TransactionRepository.GetByIDs(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	for i := range transactions {
		transactions[i].ReimbStatus = model.ReimbStatusPending
		transactions[i].Reimbursed = false
	}
	return transactions, nil
}

func (r *stalePendingTransactionRepository) MarkReimbursed(ctx context.Context, userID string, transactionIDs []string, reimbursementID string) error {
	r.markCalled = true
	return r.TransactionRepository.MarkReimbursed(ctx, userID, transactionIDs, reimbursementID)
}

func TestReimbursementStatusRaceRollsBackAllArtifacts(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	realTxRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	transactionService := service.NewTransactionService(realTxRepo, accountRepo, nil)

	transaction, err := transactionService.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, Direction: model.DirectionExpense, Source: model.SourcePersonal,
		Mode: model.ModeLife, Category: "race", AmountCents: 1_000,
		Currency: "CNY", OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	staleRepo := &stalePendingTransactionRepository{TransactionRepository: realTxRepo}
	reimbursementService := service.NewReimbursementService(
		tm, staleRepo, sqliterepo.NewSQLiteReimbursementRepository(database),
	)
	_, err = reimbursementService.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		UserID: testUserID, Applicant: "alice", TransactionIDs: []string{transaction.ID},
		RequestNo: "race-" + uuid.NewString(),
	})
	if !errors.Is(err, service.ErrConcurrentModification) {
		t.Fatalf("status race error = %v", err)
	}
	if !staleRepo.markCalled {
		t.Fatal("test did not reach the conditional status update")
	}
	assertNoReimbursementArtifacts(t, database, transaction.ID, model.ReimbStatusNone)
}

func assertNoReimbursementArtifacts(t *testing.T, database *sql.DB, transactionID string, expectedStatus model.ReimbStatus) {
	t.Helper()
	ctx := context.Background()
	var reimbursements, items int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM reimbursements`).Scan(&reimbursements); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM reimbursement_items`).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if reimbursements != 0 || items != 0 {
		t.Fatalf("partial reimbursement persisted: reimbursements=%d items=%d", reimbursements, items)
	}
	var status string
	var reimbursementID sql.NullString
	if err := database.QueryRowContext(ctx, `SELECT reimb_status, reimbursement_id FROM transactions WHERE id = ?`, transactionID).Scan(&status, &reimbursementID); err != nil {
		t.Fatal(err)
	}
	if status != string(expectedStatus) || reimbursementID.Valid {
		t.Fatalf("transaction mutated after failed reimbursement: status=%s reimbursement_id=%v", status, reimbursementID)
	}
}
