package test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"finarch/internal/domain/model"
	domainrepo "finarch/internal/domain/repository"
	"finarch/internal/domain/service"
	sqliterepo "finarch/internal/infrastructure/repository"

	"github.com/google/uuid"
)

func backendReviewAddUser(t *testing.T, database *sql.DB, id string) {
	t.Helper()
	ctx := context.Background()
	username := "u_" + strings.ReplaceAll(id, "-", "")
	_, err := database.ExecContext(ctx, `
		INSERT INTO users (
		  id, email, name, username, nickname, password_hash, role,
		  email_verified, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 'x', 'user', 1, ?, ?)`,
		id, id+"@example.test", username, username, username, time.Now().Unix(), time.Now().Unix(),
	)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	if err := service.NewAccountService(accountRepo, txRepo, tm).EnsureDefaultAccounts(ctx, id); err != nil {
		t.Fatalf("create default accounts: %v", err)
	}
}

func TestReimbursementCannotSelectAnotherUsersTransaction(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	otherUserID := uuid.NewString()
	backendReviewAddUser(t, database, otherUserID)

	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	txService := service.NewTransactionService(txRepo, accountRepo, nil)
	transaction, err := txService.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: otherUserID, Direction: model.DirectionExpense,
		Source: model.SourcePersonal, Category: "travel",
		AmountCents: 5_000, Currency: "CNY", OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	reimbursementService := service.NewReimbursementService(
		sqliterepo.NewSQLiteTransactionManager(database),
		txRepo,
		sqliterepo.NewSQLiteReimbursementRepository(database),
	)
	_, err = reimbursementService.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		UserID: testUserID, Applicant: "owner",
		TransactionIDs: []string{transaction.ID},
		RequestNo:      "scope-" + uuid.NewString(),
	})
	if err == nil {
		t.Fatal("cross-user reimbursement unexpectedly succeeded")
	}

	var status string
	if err := database.QueryRowContext(ctx,
		`SELECT reimb_status FROM transactions WHERE id=?`, transaction.ID,
	).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(model.ReimbStatusPending) {
		t.Fatalf("foreign transaction status changed: %s", status)
	}
}

func TestTransactionTagMutationsRequireCommonOwner(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	otherUserID := uuid.NewString()
	backendReviewAddUser(t, database, otherUserID)

	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	transaction, err := service.NewTransactionService(
		txRepo, sqliterepo.NewSQLiteAccountRepository(database), nil,
	).CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, Direction: model.DirectionExpense,
		Source: model.SourcePersonal, Category: "meal",
		AmountCents: 1_000, Currency: "CNY", OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	tagRepo := sqliterepo.NewSQLiteTagRepository(database)
	foreignTag := model.Tag{
		ID: uuid.NewString(), OwnerID: otherUserID, Name: "foreign",
		Color: "#000000", CreatedAt: time.Now(),
	}
	if err := tagRepo.Create(ctx, foreignTag); err != nil {
		t.Fatal(err)
	}
	if err := tagRepo.AddToTransaction(ctx, testUserID, transaction.ID, foreignTag.ID); err == nil {
		t.Fatal("attached another user's tag")
	}

	ownedTag := model.Tag{
		ID: uuid.NewString(), OwnerID: testUserID, Name: "owned",
		Color: "#ffffff", CreatedAt: time.Now(),
	}
	if err := tagRepo.Create(ctx, ownedTag); err != nil {
		t.Fatal(err)
	}
	if err := tagRepo.AddToTransaction(ctx, testUserID, transaction.ID, ownedTag.ID); err != nil {
		t.Fatal(err)
	}
	if err := tagRepo.RemoveFromTransaction(ctx, otherUserID, transaction.ID, ownedTag.ID); err == nil {
		t.Fatal("another user removed the transaction tag")
	}
	var count int
	if err := database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transaction_tags WHERE transaction_id=? AND tag_id=?`,
		transaction.ID, ownedTag.ID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("authorized tag association was modified")
	}
}

func TestDeleteUserRemovesLegacyAndNonCascadeDependants(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()

	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	transaction, err := service.NewTransactionService(
		txRepo, sqliterepo.NewSQLiteAccountRepository(database), nil,
	).CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, Direction: model.DirectionExpense,
		Source: model.SourcePersonal, Category: "travel",
		AmountCents: 2_000, Currency: "CNY", OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	reimbursement, err := service.NewReimbursementService(
		sqliterepo.NewSQLiteTransactionManager(database),
		txRepo,
		sqliterepo.NewSQLiteReimbursementRepository(database),
	).CreateReimbursement(ctx, service.CreateReimbursementRequest{
		UserID: testUserID, Applicant: "owner",
		TransactionIDs: []string{transaction.ID},
		RequestNo:      "delete-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}

	tagRepo := sqliterepo.NewSQLiteTagRepository(database)
	tag := model.Tag{
		ID: uuid.NewString(), OwnerID: testUserID, Name: "delete-me",
		Color: "#123456", CreatedAt: time.Now(),
	}
	if err := tagRepo.Create(ctx, tag); err != nil {
		t.Fatal(err)
	}
	if err := tagRepo.AddToTransaction(ctx, testUserID, transaction.ID, tag.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO fund_pools(id, owner_id, name, pool_type, currency, created_at, updated_at)
		VALUES(?, ?, 'pool', 'company', 'CNY', ?, ?)`,
		uuid.NewString(), testUserID, time.Now().Unix(), time.Now().Unix(),
	); err != nil {
		t.Fatal(err)
	}
	sessionID := uuid.NewString()
	nowUnix := time.Now().UTC().Unix()
	if _, err := database.ExecContext(ctx, `
		INSERT INTO auth_sessions(id, user_id, pwd_version, absolute_expires_at, created_at, last_used_at)
		VALUES(?, ?, 0, ?, ?, ?)`, sessionID, testUserID, nowUnix+3600, nowUnix, nowUnix); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO refresh_tokens(id, session_id, token_hash, generation, expires_at, created_at)
		VALUES(?, ?, ?, 0, ?, ?)`,
		uuid.NewString(), sessionID, make([]byte, 32), nowUnix+3600, nowUnix,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO idempotency_keys(id, user_id, endpoint, created_at)
		VALUES(?, ?, '/test', ?)`,
		uuid.NewString(), testUserID, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO ledger_events(event_type, entity_id, user_id)
		VALUES('test', ?, ?)`, transaction.ID, testUserID); err != nil {
		t.Fatal(err)
	}

	if err := sqliterepo.NewSQLiteUserRepository(database).DeleteUser(ctx, testUserID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	checks := []struct {
		name  string
		query string
		arg   string
	}{
		{"user", "SELECT COUNT(*) FROM users WHERE id=?", testUserID},
		{"transaction", "SELECT COUNT(*) FROM transactions WHERE user_id=?", testUserID},
		{"tag", "SELECT COUNT(*) FROM tags WHERE owner_id=?", testUserID},
		{"fund pool", "SELECT COUNT(*) FROM fund_pools WHERE owner_id=?", testUserID},
		{"auth session", "SELECT COUNT(*) FROM auth_sessions WHERE user_id=?", testUserID},
		{"refresh token", "SELECT COUNT(*) FROM refresh_tokens WHERE session_id=?", sessionID},
		{"idempotency key", "SELECT COUNT(*) FROM idempotency_keys WHERE user_id=?", testUserID},
		{"ledger event", "SELECT COUNT(*) FROM ledger_events WHERE user_id=?", testUserID},
		{"reimbursement item", "SELECT COUNT(*) FROM reimbursement_items WHERE reimbursement_id=?", reimbursement.ID},
		{"reimbursement", "SELECT COUNT(*) FROM reimbursements WHERE id=?", reimbursement.ID},
	}
	for _, check := range checks {
		var count int
		if err := database.QueryRowContext(ctx, check.query, check.arg).Scan(&count); err != nil {
			t.Fatalf("%s count: %v", check.name, err)
		}
		if count != 0 {
			t.Errorf("%s was not deleted", check.name)
		}
	}
}

func TestDeleteAccountEndsBoundRecurringRules(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	accountService := service.NewAccountService(accountRepo, txRepo, tm)
	account, err := accountService.CreateAccount(
		ctx, testUserID, "secondary", model.AccountTypePersonal, "CNY", model.ModeLife,
	)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
	rule := model.RecurringTransactionRule{
		ID: uuid.NewString(), UserID: testUserID, Mode: model.ModeLife,
		Name: "rent", Status: model.RecurringRuleStatusActive,
		AccountID: account.ID, TxType: model.TxTypeExpense, Category: "rent",
		AmountCents: 100, Currency: "CNY", ExchangeRate: 1,
		Frequency: model.RecurringFrequencyMonthly, Interval: 1,
		StartDate: now.Format("2006-01-02"), TimeOfDay: "09:00:00",
		Timezone: "UTC", MonthEndPolicy: model.MonthEndClamp,
		NextRunAt: now.Unix(), CatchUpEnabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := ruleRepo.CreateRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	endedRule := rule
	endedRule.ID = uuid.NewString()
	endedRule.Name = "ended rent"
	endedRule.Status = model.RecurringRuleStatusEnded
	if err := ruleRepo.CreateRule(ctx, endedRule); err != nil {
		t.Fatal(err)
	}
	staleEndedRule, err := ruleRepo.GetRuleByID(ctx, endedRule.ID, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if err := accountService.DeleteAccount(ctx, account.ID, testUserID); err != nil {
		t.Fatal(err)
	}
	got, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.RecurringRuleStatusEnded {
		t.Fatalf("recurring rule status = %s", got.Status)
	}
	endedAfterDelete, err := ruleRepo.GetRuleByID(ctx, endedRule.ID, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if endedAfterDelete.Version <= staleEndedRule.Version {
		t.Fatalf("ended rule version=%d, want greater than stale version %d", endedAfterDelete.Version, staleEndedRule.Version)
	}
	staleEndedRule.Status = model.RecurringRuleStatusActive
	if err := ruleRepo.UpdateRule(ctx, staleEndedRule); !errors.Is(err, domainrepo.ErrConcurrentModification) {
		t.Fatalf("stale ended rule update after account deletion = %v, want concurrent modification", err)
	}
}

func TestCurrencylessReportsRejectOnlyParticipatingForeignData(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	accountService := service.NewAccountService(accountRepo, txRepo, tm)
	foreignAccount, err := accountService.CreateAccount(
		ctx, testUserID, "USD account", model.AccountTypePublic, "USD", model.ModeWork,
	)
	if err != nil {
		t.Fatal(err)
	}
	stats := service.NewStatsService(database)
	if _, err := stats.Summary(ctx, testUserID); err != nil {
		t.Fatalf("empty foreign account blocked summary: %v", err)
	}

	txService := service.NewTransactionService(txRepo, accountRepo, nil)
	when := time.Now()
	if _, err := txService.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, AccountID: foreignAccount.ID, TxType: model.TxTypeIncome,
		Category: "income", AmountCents: 10_000, Currency: "USD", OccurredAt: when,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := stats.Summary(ctx, testUserID); !errors.Is(err, domainrepo.ErrMultiCurrencyReportingUnavailable) {
		t.Fatalf("summary error = %v", err)
	}
	if _, err := stats.Monthly(ctx, testUserID, when.Year()); !errors.Is(err, domainrepo.ErrMultiCurrencyReportingUnavailable) {
		t.Fatalf("monthly error = %v", err)
	}
	if _, err := stats.ByCategory(ctx, testUserID, "", ""); err != nil {
		t.Fatalf("foreign income should not affect expense category report: %v", err)
	}

	if _, err := txService.CreateTransaction(ctx, service.CreateTransactionRequest{
		UserID: testUserID, AccountID: foreignAccount.ID, TxType: model.TxTypeExpense,
		Category: "expense", AmountCents: 100, Currency: "USD", OccurredAt: when,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := stats.ByCategory(ctx, testUserID, "", ""); !errors.Is(err, domainrepo.ErrMultiCurrencyReportingUnavailable) {
		t.Fatalf("category error = %v", err)
	}
	_, err = sqliterepo.NewSQLiteBudgetRepository(database).GetMonthlyExpenseActuals(
		ctx, testUserID, model.ModeWork, when.Format("2006-01"),
	)
	if !errors.Is(err, domainrepo.ErrMultiCurrencyReportingUnavailable) {
		t.Fatalf("budget actual error = %v", err)
	}

	_, _, err = txRepo.SumPoolBalance(ctx, testUserID, model.ModeWork)
	if !errors.Is(err, domainrepo.ErrMultiCurrencyReportingUnavailable) {
		t.Fatalf("balance error = %v", err)
	}
}

func TestMultiCurrencySentinelIsStable(t *testing.T) {
	if got := domainrepo.ErrMultiCurrencyReportingUnavailable.Error(); got != "MULTI_CURRENCY_REPORTING_UNAVAILABLE" {
		t.Fatalf("unexpected sentinel: %s", got)
	}
	_ = fmt.Sprintf("%v", domainrepo.ErrMultiCurrencyReportingUnavailable)
}
