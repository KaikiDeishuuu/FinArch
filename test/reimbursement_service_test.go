package test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	"finarch/internal/infrastructure/db"
	sqliterepo "finarch/internal/infrastructure/repository"

	"github.com/google/uuid"
)

const testUserID = "test-user-0000-0000-0000-000000000001"

func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	// Use unique in-memory DB per test to avoid shared state.
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	database, err := db.OpenSQLite(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Create a test user so account FK constraints are satisfied.
	_, err = database.ExecContext(ctx,
		`INSERT OR IGNORE INTO users(id, email, name, password_hash, role, created_at, updated_at)
		 VALUES (?, 'test@example.com', 'Test User', 'x', 'user', ?, ?)`,
		testUserID, time.Now().Unix(), time.Now().Unix(),
	)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	// Create default accounts for the test user.
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	acctSvc := service.NewAccountService(acctRepo, txRepo, tm)
	if err := acctSvc.EnsureDefaultAccounts(ctx, testUserID); err != nil {
		t.Fatalf("ensure accounts: %v", err)
	}
	return database
}

func newTestSessionService(t *testing.T, database *sql.DB, jwt *auth.JWTService, secret string) *service.SessionService {
	t.Helper()
	sessions, err := service.NewSessionService(sqliterepo.NewSQLiteRefreshTokenRepository(database), jwt, secret)
	if err != nil {
		t.Fatal(err)
	}
	return sessions
}

// TestReimburse_RejectDuplicateTxIDs verifies duplicate IDs are rejected before transaction.
func TestReimburse_RejectDuplicateTxIDs(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()

	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	reimRepo := sqliterepo.NewSQLiteReimbursementRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	txSvc := service.NewTransactionService(txRepo, acctRepo, nil)
	reimSvc := service.NewReimbursementService(tm, txRepo, reimRepo)

	tx, err := txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID:     testUserID,
		OccurredAt: time.Now(),
		Direction:  model.DirectionExpense,
		Source:     model.SourcePersonal,
		Category:   "差旅",
		AmountYuan: 10,
		Currency:   "CNY",
	})
	if err != nil {
		t.Fatalf("create tx: %v", err)
	}

	_, err = reimSvc.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		UserID:         testUserID,
		Applicant:      "alice",
		TransactionIDs: []string{tx.ID, tx.ID},
		RequestNo:      "REIM-" + uuid.NewString(),
	})
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
}

// TestReimburse_AtomicAndSingleUse verifies transaction can be reimbursed only once.
func TestReimburse_AtomicAndSingleUse(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()

	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	reimRepo := sqliterepo.NewSQLiteReimbursementRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	txSvc := service.NewTransactionService(txRepo, acctRepo, nil)
	reimSvc := service.NewReimbursementService(tm, txRepo, reimRepo)

	tx, err := txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
		OccurredAt: time.Now(),
		UserID:     testUserID,
		Direction:  model.DirectionExpense,
		Source:     model.SourcePersonal,
		Category:   "材料",
		AmountYuan: 35,
		Currency:   "CNY",
	})
	if err != nil {
		t.Fatalf("create tx: %v", err)
	}

	_, err = reimSvc.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		UserID:         testUserID,
		Applicant:      "alice",
		TransactionIDs: []string{tx.ID},
		RequestNo:      "REIM-1-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("first reimbursement failed: %v", err)
	}

	_, err = reimSvc.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		UserID:         testUserID,
		Applicant:      "alice",
		TransactionIDs: []string{tx.ID},
		RequestNo:      "REIM-2-" + uuid.NewString(),
	})
	if err == nil {
		t.Fatal("expected second reimbursement failure")
	}
}

func TestSetReimbursedEnforcesPersonalExpenseLifecycle(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	txSvc := service.NewTransactionService(txRepo, acctRepo, nil)

	personal, err := acctRepo.GetByUserAndType(ctx, testUserID, model.AccountTypePersonal)
	if err != nil {
		t.Fatal(err)
	}
	public, err := acctRepo.GetByUserAndType(ctx, testUserID, model.AccountTypePublic)
	if err != nil {
		t.Fatal(err)
	}
	create := func(accountID string, txType model.TxType, source model.Source) model.Transaction {
		t.Helper()
		tx, err := txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
			UserID: testUserID, OccurredAt: time.Now(), AccountID: accountID,
			TxType: txType, Source: source, Category: "lifecycle", AmountCents: 100, Currency: "CNY",
		})
		if err != nil {
			t.Fatal(err)
		}
		return tx
	}

	companyExpense := create(public.ID, model.TxTypeExpense, model.SourceCompany)
	personalIncome := create(personal.ID, model.TxTypeIncome, model.SourcePersonal)
	for _, transaction := range []model.Transaction{companyExpense, personalIncome} {
		if _, err := database.Exec(`UPDATE transactions SET uploaded = 1, reported_at = ? WHERE id = ?`, time.Now().Unix(), transaction.ID); err != nil {
			t.Fatal(err)
		}
		if err := txRepo.SetReimbursed(ctx, transaction.ID, testUserID, true); err == nil {
			t.Fatalf("ineligible transaction %s was marked reimbursed", transaction.ID)
		}
	}

	personalExpense := create(personal.ID, model.TxTypeExpense, model.SourcePersonal)
	if err := txRepo.SetReimbursed(ctx, personalExpense.ID, testUserID, true); err == nil {
		t.Fatal("unreported personal expense was marked reimbursed")
	}
	if _, err := database.Exec(`UPDATE transactions SET uploaded = 1, reported_at = ? WHERE id = ?`, time.Now().Unix(), personalExpense.ID); err != nil {
		t.Fatal(err)
	}
	if err := txRepo.SetReimbursed(ctx, personalExpense.ID, testUserID, true); err != nil {
		t.Fatal(err)
	}
	var reimbursedAt sql.NullInt64
	if err := database.QueryRow(`SELECT reimbursed_at FROM transactions WHERE id = ?`, personalExpense.ID).Scan(&reimbursedAt); err != nil {
		t.Fatal(err)
	}
	if !reimbursedAt.Valid {
		t.Fatal("reimbursed transaction is missing reimbursed_at")
	}
	if err := txRepo.SetReimbursed(ctx, personalExpense.ID, testUserID, false); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := database.QueryRow(`SELECT reimb_status, reimbursed_at FROM transactions WHERE id = ?`, personalExpense.ID).Scan(&status, &reimbursedAt); err != nil {
		t.Fatal(err)
	}
	if status != string(model.ReimbStatusPending) || reimbursedAt.Valid {
		t.Fatalf("pending lifecycle = status:%q reimbursed_at:%v", status, reimbursedAt)
	}
}

func TestSetSettledEnforcesPublicExpenseLifecycle(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	txSvc := service.NewTransactionService(txRepo, acctRepo, nil)

	personal, err := acctRepo.GetByUserAndType(ctx, testUserID, model.AccountTypePersonal)
	if err != nil {
		t.Fatal(err)
	}
	public, err := acctRepo.GetByUserAndType(ctx, testUserID, model.AccountTypePublic)
	if err != nil {
		t.Fatal(err)
	}
	create := func(accountID string, txType model.TxType, source model.Source, mode model.Mode) model.Transaction {
		t.Helper()
		tx, err := txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
			UserID: testUserID, OccurredAt: time.Now(), AccountID: accountID,
			TxType: txType, Source: source, Category: "settlement", AmountCents: 100, Currency: "CNY",
			Mode: mode,
		})
		if err != nil {
			t.Fatal(err)
		}
		return tx
	}
	markUploaded := func(id string) {
		t.Helper()
		if _, err := database.Exec(`UPDATE transactions SET uploaded = 1 WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}

	// Settlement is public-account, WORK-mode, expense-only. Personal advances
	// keep using the reimbursement lane.
	personalExpense := create(personal.ID, model.TxTypeExpense, model.SourcePersonal, model.ModeWork)
	publicIncome := create(public.ID, model.TxTypeIncome, model.SourceCompany, model.ModeWork)
	for _, transaction := range []model.Transaction{personalExpense, publicIncome} {
		markUploaded(transaction.ID)
		if err := txRepo.SetSettled(ctx, transaction.ID, testUserID, true); err == nil {
			t.Fatalf("ineligible transaction %s was marked settled", transaction.ID)
		}
	}

	publicExpense := create(public.ID, model.TxTypeExpense, model.SourceCompany, model.ModeWork)
	if err := txRepo.SetSettled(ctx, publicExpense.ID, testUserID, true); err == nil {
		t.Fatal("unreported public expense was marked settled")
	}
	markUploaded(publicExpense.ID)
	if err := txRepo.SetSettled(ctx, publicExpense.ID, testUserID, true); err != nil {
		t.Fatal(err)
	}
	var settledAt sql.NullInt64
	if err := database.QueryRow(`SELECT settled_at FROM transactions WHERE id = ?`, publicExpense.ID).Scan(&settledAt); err != nil {
		t.Fatal(err)
	}
	if !settledAt.Valid {
		t.Fatal("settled transaction is missing settled_at")
	}

	// Settling must never touch the reimbursement lane: WORK statistics add
	// reimbursed amounts back into net, and public money was never fronted.
	var reimbStatus string
	if err := database.QueryRow(`SELECT reimb_status FROM transactions WHERE id = ?`, publicExpense.ID).Scan(&reimbStatus); err != nil {
		t.Fatal(err)
	}
	if reimbStatus != string(model.ReimbStatusNone) {
		t.Fatalf("settling a public expense changed reimb_status to %q", reimbStatus)
	}

	if err := txRepo.SetSettled(ctx, publicExpense.ID, testUserID, false); err != nil {
		t.Fatal(err)
	}
	var settled int
	if err := database.QueryRow(`SELECT settled, settled_at FROM transactions WHERE id = ?`, publicExpense.ID).Scan(&settled, &settledAt); err != nil {
		t.Fatal(err)
	}
	if settled != 0 || settledAt.Valid {
		t.Fatalf("pending lifecycle = settled:%d settled_at:%v", settled, settledAt)
	}
}

func TestReimbursementRequestsSetAnExplicitState(t *testing.T) {
	ctx := context.Background()
	database := setupFileDB(t, ctx)
	defer database.Close()
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	account, err := accountRepo.GetByUserAndType(ctx, testUserID, model.AccountTypePersonal)
	if err != nil {
		t.Fatal(err)
	}
	item, err := service.NewTransactionService(txRepo, accountRepo, nil).CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, AccountID: account.ID, OccurredAt: time.Now(), TxType: model.TxTypeExpense,
		Source: model.SourcePersonal, Category: "office", AmountCents: 100, Currency: "CNY",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := txRepo.SetUploaded(ctx, item.ID, testUserID, true); err != nil {
		t.Fatal(err)
	}
	jwtSvc := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, jwtSvc, testUserID)
	handler := newTestServer(t, database, jwtSvc).Handler()
	request := func(method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/transactions/"+item.ID+"/reimburse", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; responses <- request(http.MethodPut, `{"reimbursed":true}`) }()
	}
	close(start)
	for i := 0; i < 2; i++ {
		rec := <-responses
		var reply struct {
			Data struct {
				Reimbursed bool `json:"reimbursed"`
			} `json:"data"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &reply) != nil || !reply.Data.Reimbursed {
			t.Fatalf("confirmation %d: %d %s", i, rec.Code, rec.Body)
		}
	}
	// Sentinels ensure retries preserve the audit trail even within the same second.
	if _, err := database.Exec("UPDATE transactions SET updated_at='2026-01-02T00:00:00Z', reimbursed_at=42 WHERE id=?", item.ID); err != nil {
		t.Fatal(err)
	}
	if rec := request(http.MethodPut, `{"reimbursed":true}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body)
	}
	if rec := request(http.MethodPatch, ""); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected legacy request to return 422, got %d", rec.Code)
	}
	var status, updated string
	var stamp int64
	if err := database.QueryRow("SELECT reimb_status,updated_at,reimbursed_at FROM transactions WHERE id=?", item.ID).Scan(&status, &updated, &stamp); err != nil {
		t.Fatal(err)
	}
	if status != "reimbursed" || updated != "2026-01-02T00:00:00Z" || stamp != 42 {
		t.Fatalf("retry changed audit trail: %s %s %d", status, updated, stamp)
	}
	for i := 0; i < 2; i++ {
		if rec := request(http.MethodPut, `{"reimbursed":false}`); rec.Code != http.StatusOK {
			t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
		}
	}
	var pending bool
	if err := database.QueryRow("SELECT reimb_status='pending' AND reimbursed_at IS NULL FROM transactions WHERE id=?", item.ID).Scan(&pending); err != nil || !pending {
		t.Fatalf("cancel left wrong state: %t %v", pending, err)
	}
}

func TestExplicitUploadAndSettlementRequests(t *testing.T) {
	ctx := context.Background()
	database := setupFileDB(t, ctx)
	defer database.Close()
	accounts := sqliterepo.NewSQLiteAccountRepository(database)
	public, err := accounts.GetByUserAndType(ctx, testUserID, model.AccountTypePublic)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewTransactionService(sqliterepo.NewSQLiteTransactionRepository(database), accounts, nil)
	tx, err := svc.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, AccountID: public.ID, OccurredAt: time.Now(), Mode: model.ModeWork,
		TxType: model.TxTypeExpense, Source: model.SourceCompany, Category: "office", AmountCents: 100, Currency: "CNY",
	})
	if err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, jwt, testUserID)
	handler := newTestServer(t, database, jwt).Handler()
	request := func(method, endpoint, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/transactions/"+tx.ID+"/"+endpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	for _, tc := range []struct{ path, field string }{{"upload", "uploaded"}, {"settle", "settled"}} {
		body := `{"` + tc.field + `":true}`
		responses := make(chan *httptest.ResponseRecorder, 2)
		start := make(chan struct{})
		for range 2 {
			go func() { <-start; responses <- request(http.MethodPut, tc.path, body) }()
		}
		close(start)
		for range 2 {
			r := <-responses
			var reply struct{ Data map[string]any }
			if r.Code != http.StatusOK || json.Unmarshal(r.Body.Bytes(), &reply) != nil || reply.Data[tc.field] != true {
				t.Fatalf("concurrent %s: %d %s", tc.path, r.Code, r.Body)
			}
		}
	}
	if _, err := database.Exec("UPDATE transactions SET reported_at=123, settled_at=456, updated_at='audit-marker' WHERE id=?", tx.ID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, field string }{{"upload", "uploaded"}, {"settle", "settled"}} {
		if r := request(http.MethodPut, tc.path, `{"`+tc.field+`":true}`); r.Code != http.StatusOK {
			t.Fatalf("repeat %s: %s", tc.path, r.Body)
		}
		if r := request(http.MethodPatch, tc.path, ""); r.Code != http.StatusUnprocessableEntity {
			t.Fatalf("legacy %s returned %d", tc.path, r.Code)
		}
	}
	if r := request(http.MethodPut, "upload", `{"uploaded":false}`); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unreported a settled expense: %d", r.Code)
	}
	var auditPreserved bool
	if err := database.QueryRow("SELECT uploaded AND settled AND reported_at=123 AND settled_at=456 AND updated_at='audit-marker' FROM transactions WHERE id=?", tx.ID).Scan(&auditPreserved); err != nil || !auditPreserved {
		t.Fatalf("audit fields changed on retry: %v", err)
	}
	for _, tc := range []struct{ path, field string }{{"settle", "settled"}, {"upload", "uploaded"}} {
		for range 2 {
			r := request(http.MethodPut, tc.path, `{"`+tc.field+`":false}`)
			if r.Code != http.StatusOK {
				t.Fatalf("cancel %s: %s", tc.path, r.Body)
			}
		}
	}
	var cancelled bool
	if err := database.QueryRow("SELECT NOT uploaded AND NOT settled AND settled_at IS NULL AND reported_at=123 FROM transactions WHERE id=?", tx.ID).Scan(&cancelled); err != nil || !cancelled {
		t.Fatalf("incorrect cancellation state: %v", err)
	}
}
