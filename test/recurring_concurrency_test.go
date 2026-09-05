package test

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"finarch/internal/domain/model"
	domainrepo "finarch/internal/domain/repository"
	"finarch/internal/domain/service"
	sqliterepo "finarch/internal/infrastructure/repository"

	"github.com/google/uuid"
)

type controlledCreateTransactionRepository struct {
	domainrepo.TransactionRepository
	fail atomic.Bool
}

func (r *controlledCreateTransactionRepository) Create(ctx context.Context, tx model.Transaction) error {
	if r.fail.Load() {
		return errors.New("forced recurring create failure")
	}
	return r.TransactionRepository.Create(ctx, tx)
}

type failingIdempotencyTransactionRepository struct {
	domainrepo.TransactionRepository
	err error
}

func (r *failingIdempotencyTransactionRepository) GetByIdempotencyKey(context.Context, string, string) (model.Transaction, error) {
	return model.Transaction{}, r.err
}

type vanishingIdempotencyTransactionRepository struct {
	domainrepo.TransactionRepository
	preexisting model.Transaction
	lookups     atomic.Int32
	creates     atomic.Int32
}

func (r *vanishingIdempotencyTransactionRepository) GetByIdempotencyKey(context.Context, string, string) (model.Transaction, error) {
	if r.lookups.Add(1) == 1 {
		return r.preexisting, nil
	}
	return model.Transaction{}, domainrepo.ErrTransactionNotFound
}

func (r *vanishingIdempotencyTransactionRepository) Create(ctx context.Context, tx model.Transaction) error {
	r.creates.Add(1)
	return r.TransactionRepository.Create(ctx, tx)
}

type observingTransactionManager struct {
	domainrepo.TransactionManager
	active atomic.Bool
}

func (m *observingTransactionManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return m.TransactionManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		if !m.active.CompareAndSwap(false, true) {
			return errors.New("nested observed transaction")
		}
		defer m.active.Store(false)
		return fn(txCtx)
	})
}

type observingRecurringRateService struct {
	txActive *atomic.Bool
	calls    atomic.Int32
	inside   atomic.Bool
	rate     service.ExchangeRateResult
	onCall   func() error
}

func (s *observingRecurringRateService) GetRate(context.Context, string, string, time.Time) (service.ExchangeRateResult, error) {
	s.calls.Add(1)
	if s.txActive != nil && s.txActive.Load() {
		s.inside.Store(true)
	}
	if s.onCall != nil {
		if err := s.onCall(); err != nil {
			return service.ExchangeRateResult{}, err
		}
	}
	return s.rate, nil
}

type deleteAfterAccountReadRepository struct {
	domainrepo.AccountRepository
	targetID string
	delete   func() error
	once     sync.Once
}

func (r *deleteAfterAccountReadRepository) GetByID(ctx context.Context, id string) (model.Account, error) {
	account, err := r.AccountRepository.GetByID(ctx, id)
	if err == nil && id == r.targetID {
		r.once.Do(func() {
			if deleteErr := r.delete(); deleteErr != nil {
				err = deleteErr
			}
		})
	}
	return account, err
}

func recurringAccount(t *testing.T, repo domainrepo.AccountRepository, accountType model.AccountType) model.Account {
	t.Helper()
	accounts, err := repo.ListByUser(context.Background(), testUserID)
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		if account.Type == accountType {
			return account
		}
	}
	t.Fatalf("missing %s account", accountType)
	return model.Account{}
}

func dueRecurringRule(account model.Account, now time.Time, currency string, exchangeRate float64) model.RecurringTransactionRule {
	dueAt := now.Add(-time.Minute).UTC().Truncate(time.Second)
	mode := model.ModeLife
	if account.Type == model.AccountTypePublic {
		mode = model.ModeWork
	}
	return model.RecurringTransactionRule{
		ID: uuid.NewString(), UserID: testUserID, Mode: mode,
		Name: "reliable recurring", Status: model.RecurringRuleStatusActive,
		AccountID: account.ID, TxType: model.TxTypeExpense, Category: "test",
		AmountCents: 100, Currency: currency, ExchangeRate: exchangeRate,
		Frequency: model.RecurringFrequencyDaily, Interval: 1,
		StartDate: dueAt.Format("2006-01-02"), TimeOfDay: dueAt.Format("15:04:05"),
		Timezone: "UTC", MonthEndPolicy: model.MonthEndClamp,
		NextRunAt: dueAt.Unix(), CatchUpEnabled: true, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestRecurringRuleCASPreventsStaleGenerationAndStateRevival(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()

	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	accounts, err := accountRepo.ListByUser(ctx, testUserID)
	if err != nil || len(accounts) == 0 {
		t.Fatalf("load test account: %v", err)
	}

	now := time.Now().UTC()
	ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
	rule := model.RecurringTransactionRule{
		ID: uuid.NewString(), UserID: testUserID, Mode: model.ModeLife,
		Name: "safe recurring", Status: model.RecurringRuleStatusActive,
		AccountID: accounts[0].ID, TxType: model.TxTypeExpense, Category: "test",
		AmountCents: 100, Currency: accounts[0].Currency, ExchangeRate: 1,
		Frequency: model.RecurringFrequencyDaily, Interval: 1,
		StartDate: now.Format("2006-01-02"), TimeOfDay: "09:00:00",
		Timezone: "UTC", MonthEndPolicy: model.MonthEndClamp,
		NextRunAt: now.Unix(), CatchUpEnabled: true, Version: 0,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := ruleRepo.CreateRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	stale, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Version != 1 {
		t.Fatalf("initial version=%d, want 1", stale.Version)
	}

	edited := stale
	edited.AmountCents = 250
	if err := ruleRepo.UpdateRule(ctx, edited); err != nil {
		t.Fatal(err)
	}
	current, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Version != 2 || current.AmountCents != 250 {
		t.Fatalf("edited rule=%+v, want version 2 and amount 250", current)
	}
	zeroVersion := current
	zeroVersion.Version = 0
	zeroVersion.AmountCents = 999
	if err := ruleRepo.UpdateRule(ctx, zeroVersion); !errors.Is(err, domainrepo.ErrConcurrentModification) {
		t.Fatalf("zero-version update error=%v, want concurrent modification", err)
	}
	if advanced, err := ruleRepo.AdvanceRule(ctx, current.ID, current.UserID, 0, current.NextRunAt, current.NextRunAt+86400, nil, model.RecurringRuleStatusActive); err != nil {
		t.Fatal(err)
	} else if advanced {
		t.Fatal("zero-version generator advanced a versioned rule")
	}
	if claimed, err := ruleRepo.ClaimInstance(ctx, model.RecurringTransactionInstance{
		ID: uuid.NewString(), RuleID: current.ID, UserID: current.UserID,
		OccurrenceDate: now.Format("2006-01-02"), ScheduledAt: current.NextRunAt,
		IdempotencyKey: uuid.NewString(), Status: model.RecurringInstanceGenerating,
		CreatedAt: now, UpdatedAt: now,
	}, 0, current.NextRunAt); err != nil {
		t.Fatal(err)
	} else if claimed {
		t.Fatal("zero-version generator claimed a versioned rule")
	}

	claimed, err := ruleRepo.ClaimInstance(ctx, model.RecurringTransactionInstance{
		ID: uuid.NewString(), RuleID: stale.ID, UserID: stale.UserID,
		OccurrenceDate: now.Format("2006-01-02"), ScheduledAt: stale.NextRunAt,
		IdempotencyKey: uuid.NewString(), Status: model.RecurringInstanceGenerating,
		CreatedAt: now, UpdatedAt: now,
	}, stale.Version, stale.NextRunAt)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("stale generator claimed an occurrence after the rule was edited")
	}

	paused := current
	paused.Status = model.RecurringRuleStatusPaused
	if err := ruleRepo.UpdateRule(ctx, paused); err != nil {
		t.Fatal(err)
	}
	advanced, err := ruleRepo.AdvanceRule(
		ctx, current.ID, current.UserID, current.Version, current.NextRunAt,
		current.NextRunAt+86400, nil, model.RecurringRuleStatusActive,
	)
	if err != nil {
		t.Fatal(err)
	}
	if advanced {
		t.Fatal("stale generator advanced a paused rule")
	}
	if err := ruleRepo.UpdateRule(ctx, current); err == nil {
		t.Fatal("stale full update revived a paused rule")
	}
	afterPause, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if afterPause.Status != model.RecurringRuleStatusPaused || afterPause.NextRunAt != current.NextRunAt {
		t.Fatalf("paused rule was changed by stale generation: %+v", afterPause)
	}

	if err := ruleRepo.DeleteRule(ctx, rule.ID, testUserID); err != nil {
		t.Fatal(err)
	}
	if err := ruleRepo.UpdateRule(ctx, afterPause); err == nil {
		t.Fatal("stale update revived an ended rule")
	}
	afterDelete, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if afterDelete.Status != model.RecurringRuleStatusEnded {
		t.Fatalf("deleted rule status=%s, want ended", afterDelete.Status)
	}
}

func TestRecurringGenerateDueAdvancesClaimedRule(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()

	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	accounts, err := accountRepo.ListByUser(ctx, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	var account model.Account
	for _, candidate := range accounts {
		if candidate.Type == model.AccountTypePersonal {
			account = candidate
			break
		}
	}
	if account.ID == "" {
		t.Fatal("test user has no personal account")
	}

	now := time.Now().UTC().Truncate(time.Second)
	dueAt := now.Add(-time.Minute)
	rule := model.RecurringTransactionRule{
		ID: uuid.NewString(), UserID: testUserID, Mode: model.ModeLife,
		Name: "working recurring", Status: model.RecurringRuleStatusActive,
		AccountID: account.ID, TxType: model.TxTypeExpense, Category: "test",
		AmountCents: 100, Currency: account.Currency, ExchangeRate: 1,
		Frequency: model.RecurringFrequencyDaily, Interval: 1,
		StartDate: dueAt.Format("2006-01-02"), TimeOfDay: dueAt.Format("15:04:05"),
		Timezone: "UTC", MonthEndPolicy: model.MonthEndClamp,
		NextRunAt: dueAt.Unix(), CatchUpEnabled: true, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
	if err := ruleRepo.CreateRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	txManager := sqliterepo.NewSQLiteTransactionManager(database)
	recurringSvc := service.NewRecurringTransactionService(
		ruleRepo,
		txRepo,
		service.NewTransactionService(txRepo, accountRepo, nil),
		txManager,
		accountRepo,
	)

	result, err := recurringSvc.GenerateDue(ctx, now, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Generated != 1 || result.Failed != 0 || len(result.Errors) != 0 {
		t.Fatalf("generation result=%+v, want one successful transaction", result)
	}
	advanced, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if advanced.Version != 2 || advanced.NextRunAt <= rule.NextRunAt {
		t.Fatalf("rule was not advanced: before=%+v after=%+v", rule, advanced)
	}
	wantOccurrenceDate := dueAt.Format("2006-01-02")
	if advanced.LastGeneratedFor == nil || *advanced.LastGeneratedFor != wantOccurrenceDate {
		t.Fatalf("last_generated_for=%v, want %s", advanced.LastGeneratedFor, wantOccurrenceDate)
	}
	var transactionCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(1) FROM transactions WHERE recurring_rule_id = ?`, rule.ID).Scan(&transactionCount); err != nil {
		t.Fatal(err)
	}
	if transactionCount != 1 {
		t.Fatalf("generated transaction count=%d, want 1", transactionCount)
	}
	var instanceStatus string
	if err := database.QueryRowContext(ctx, `SELECT status FROM recurring_transaction_instances WHERE rule_id = ?`, rule.ID).Scan(&instanceStatus); err != nil {
		t.Fatal(err)
	}
	if instanceStatus != string(model.RecurringInstanceGenerated) {
		t.Fatalf("instance status=%s, want generated", instanceStatus)
	}
}

func TestRecurringCreateFailureRollsBackAndRetriesSameOccurrence(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	account := recurringAccount(t, accountRepo, model.AccountTypePersonal)
	now := time.Now().UTC().Truncate(time.Second)
	rule := dueRecurringRule(account, now, account.Currency, 1)
	ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
	if err := ruleRepo.CreateRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	realTxRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	txRepo := &controlledCreateTransactionRepository{TransactionRepository: realTxRepo}
	txRepo.fail.Store(true)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	recurringSvc := service.NewRecurringTransactionService(
		ruleRepo, txRepo, service.NewTransactionService(txRepo, accountRepo, nil), tm, accountRepo,
	)

	failed, err := recurringSvc.GenerateDue(ctx, now, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Failed != 1 || len(failed.Errors) != 1 || !strings.Contains(failed.Errors[0], "forced recurring create failure") {
		t.Fatalf("failed generation result=%+v", failed)
	}
	afterFailure, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if afterFailure.Version != rule.Version || afterFailure.NextRunAt != rule.NextRunAt || afterFailure.LastGeneratedFor != nil {
		t.Fatalf("failed generation advanced rule: before=%+v after=%+v", rule, afterFailure)
	}
	var instances, transactions int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM recurring_transaction_instances WHERE rule_id = ?`, rule.ID).Scan(&instances); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE recurring_rule_id = ?`, rule.ID).Scan(&transactions); err != nil {
		t.Fatal(err)
	}
	if instances != 0 || transactions != 0 {
		t.Fatalf("failed generation leaked state: instances=%d transactions=%d", instances, transactions)
	}

	txRepo.fail.Store(false)
	retried, err := recurringSvc.GenerateDue(ctx, now, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Generated != 1 || retried.Failed != 0 {
		t.Fatalf("retry result=%+v, want one generated transaction", retried)
	}
	afterRetry, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if afterRetry.Version != rule.Version+1 || afterRetry.NextRunAt <= rule.NextRunAt {
		t.Fatalf("successful retry did not advance rule: %+v", afterRetry)
	}
}

func TestRecurringIdempotencyLookupFailureRollsBack(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	account := recurringAccount(t, accountRepo, model.AccountTypePersonal)
	now := time.Now().UTC().Truncate(time.Second)
	rule := dueRecurringRule(account, now, account.Currency, 1)
	ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
	if err := ruleRepo.CreateRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	realTxRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	lookupErr := errors.New("forced idempotency lookup outage")
	txRepo := &failingIdempotencyTransactionRepository{TransactionRepository: realTxRepo, err: lookupErr}
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	recurringSvc := service.NewRecurringTransactionService(
		ruleRepo, txRepo, service.NewTransactionService(txRepo, accountRepo, nil), tm, accountRepo,
	)

	result, err := recurringSvc.GenerateDue(ctx, now, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || len(result.Errors) != 1 || !strings.Contains(result.Errors[0], lookupErr.Error()) {
		t.Fatalf("lookup failure result=%+v", result)
	}
	current, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Version != rule.Version || current.NextRunAt != rule.NextRunAt {
		t.Fatalf("lookup failure advanced rule: %+v", current)
	}
	var instanceCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM recurring_transaction_instances WHERE rule_id = ?`, rule.ID).Scan(&instanceCount); err != nil {
		t.Fatal(err)
	}
	if instanceCount != 0 {
		t.Fatalf("lookup failure leaked %d claimed instances", instanceCount)
	}
}

func TestRecurringPreexistingTransactionDisappearsBeforeAuthoritativeRecheck(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	account := recurringAccount(t, accountRepo, model.AccountTypePersonal)
	now := time.Now().UTC().Truncate(time.Second)
	rule := dueRecurringRule(account, now, "EUR", 0)
	ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
	if err := ruleRepo.CreateRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	occurrenceDate := time.Unix(rule.NextRunAt, 0).UTC().Format("2006-01-02")
	key := "recurring:" + rule.ID + ":" + occurrenceDate
	ruleID := rule.ID
	wrapper := &vanishingIdempotencyTransactionRepository{
		TransactionRepository: sqliterepo.NewSQLiteTransactionRepository(database),
		preexisting: model.Transaction{
			ID: "vanishing-transaction", UserID: testUserID,
			IdempotencyKey: &key, RecurringRuleID: &ruleID, RecurringOccurrenceDate: &occurrenceDate,
		},
	}
	rateSvc := &observingRecurringRateService{onCall: func() error {
		return errors.New("rate provider must not be used after a successful preflight lookup")
	}}
	recurringSvc := service.NewRecurringTransactionService(
		ruleRepo, wrapper, service.NewTransactionService(wrapper, accountRepo, rateSvc),
		sqliterepo.NewSQLiteTransactionManager(database), accountRepo,
	)

	result, err := recurringSvc.GenerateRuleNow(ctx, testUserID, rule.ID, false)
	if !errors.Is(err, service.ErrConcurrentModification) {
		t.Fatalf("vanishing preflight transaction error=%v, want concurrent modification", err)
	}
	if result.Failed != 1 || result.Generated != 0 {
		t.Fatalf("vanishing preflight transaction result=%+v", result)
	}
	if wrapper.lookups.Load() != 2 || wrapper.creates.Load() != 0 || rateSvc.calls.Load() != 0 {
		t.Fatalf("unexpected side effects: lookups=%d creates=%d rate_calls=%d", wrapper.lookups.Load(), wrapper.creates.Load(), rateSvc.calls.Load())
	}
	current, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Version != rule.Version || current.NextRunAt != rule.NextRunAt || current.LastGeneratedFor != nil {
		t.Fatalf("vanishing transaction advanced rule: before=%+v after=%+v", rule, current)
	}
	var instanceCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM recurring_transaction_instances WHERE rule_id = ?`, rule.ID).Scan(&instanceCount); err != nil {
		t.Fatal(err)
	}
	if instanceCount != 0 {
		t.Fatalf("vanishing transaction leaked %d claimed instances", instanceCount)
	}
}

func TestRecurringResolvesAndPreservesRateBeforeWriteTransaction(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	account := recurringAccount(t, accountRepo, model.AccountTypePersonal)
	now := time.Now().UTC().Truncate(time.Second)
	rule := dueRecurringRule(account, now, "EUR", 0)
	ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
	if err := ruleRepo.CreateRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	observedTM := &observingTransactionManager{TransactionManager: sqliterepo.NewSQLiteTransactionManager(database)}
	rateAt := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	rateSvc := &observingRecurringRateService{
		txActive: &observedTM.active,
		rate: service.ExchangeRateResult{
			Rate: big.NewRat(7, 4), RateFloat: 1.75, Source: "deterministic-rate", At: rateAt,
		},
	}
	recurringSvc := service.NewRecurringTransactionService(
		ruleRepo, txRepo, service.NewTransactionService(txRepo, accountRepo, rateSvc), observedTM, accountRepo,
	)

	result, err := recurringSvc.GenerateDue(ctx, now, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Generated != 1 || result.Failed != 0 {
		t.Fatalf("generation result=%+v", result)
	}
	if rateSvc.calls.Load() != 1 || rateSvc.inside.Load() {
		t.Fatalf("rate resolution calls=%d inside_write_tx=%t", rateSvc.calls.Load(), rateSvc.inside.Load())
	}
	var rate float64
	var source, baseCurrency string
	var storedRateAt, baseAmount int64
	if err := database.QueryRowContext(ctx, `
		SELECT exchange_rate, exchange_rate_source, exchange_rate_at, base_currency, base_amount_cents
		FROM transactions WHERE recurring_rule_id = ?`, rule.ID,
	).Scan(&rate, &source, &storedRateAt, &baseCurrency, &baseAmount); err != nil {
		t.Fatal(err)
	}
	if rate != 1.75 || source != "deterministic-rate" || storedRateAt != rateAt.Unix() || baseCurrency != account.Currency || baseAmount != 175 {
		t.Fatalf("persisted rate audit mismatch: rate=%v source=%q at=%d base=%s amount=%d", rate, source, storedRateAt, baseCurrency, baseAmount)
	}
}

func TestRecurringPreparedSnapshotCASRejectsConcurrentRuleEdit(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
	account := recurringAccount(t, accountRepo, model.AccountTypePersonal)
	now := time.Now().UTC().Truncate(time.Second)
	rule := dueRecurringRule(account, now, "EUR", 0)
	ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
	if err := ruleRepo.CreateRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	rateSvc := &observingRecurringRateService{
		rate: service.ExchangeRateResult{
			Rate: big.NewRat(3, 2), RateFloat: 1.5, Source: "snapshot-rate", At: now,
		},
		onCall: func() error {
			_, err := database.ExecContext(ctx, `
				UPDATE recurring_transaction_rules
				SET amount_cents = 250, version = version + 1
				WHERE id = ?`, rule.ID)
			return err
		},
	}
	recurringSvc := service.NewRecurringTransactionService(
		ruleRepo, txRepo, service.NewTransactionService(txRepo, accountRepo, rateSvc),
		sqliterepo.NewSQLiteTransactionManager(database), accountRepo,
	)

	result, err := recurringSvc.GenerateRuleNow(ctx, testUserID, rule.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped != 1 || result.Generated != 0 || result.Failed != 0 {
		t.Fatalf("stale prepared snapshot result=%+v", result)
	}
	var txCount, instanceCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE recurring_rule_id = ?`, rule.ID).Scan(&txCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM recurring_transaction_instances WHERE rule_id = ?`, rule.ID).Scan(&instanceCount); err != nil {
		t.Fatal(err)
	}
	if txCount != 0 || instanceCount != 0 {
		t.Fatalf("stale prepared snapshot leaked state: transactions=%d instances=%d", txCount, instanceCount)
	}
}

func TestRecurringReconcilesOrphanTransactionAndInstance(t *testing.T) {
	t.Run("transaction without instance", func(t *testing.T) {
		database := setupDB(t)
		defer database.Close()
		ctx := context.Background()
		accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
		account := recurringAccount(t, accountRepo, model.AccountTypePersonal)
		now := time.Now().UTC().Truncate(time.Second)
		foreignCurrency := "EUR"
		if strings.EqualFold(account.Currency, foreignCurrency) {
			foreignCurrency = "USD"
		}
		rule := dueRecurringRule(account, now, foreignCurrency, 0)
		ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
		if err := ruleRepo.CreateRule(ctx, rule); err != nil {
			t.Fatal(err)
		}
		txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
		rateSvc := &observingRecurringRateService{onCall: func() error {
			return errors.New("rate provider must not be used for orphan reconciliation")
		}}
		txSvc := service.NewTransactionService(txRepo, accountRepo, rateSvc)
		occurrenceDate := time.Unix(rule.NextRunAt, 0).UTC().Format("2006-01-02")
		key := "recurring:" + rule.ID + ":" + occurrenceDate
		orphan, err := txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
			UserID: testUserID, Mode: rule.Mode, OccurredAt: time.Unix(rule.NextRunAt, 0).UTC(),
			AccountID: rule.AccountID, TxType: rule.TxType, Category: rule.Category,
			AmountCents: rule.AmountCents, Currency: rule.Currency, ExchangeRate: 1.25,
			IdempotencyKey: &key, RecurringRuleID: &rule.ID, RecurringOccurrenceDate: &occurrenceDate,
		})
		if err != nil {
			t.Fatal(err)
		}
		recurringSvc := service.NewRecurringTransactionService(
			ruleRepo, txRepo, txSvc, sqliterepo.NewSQLiteTransactionManager(database), accountRepo,
		)

		result, err := recurringSvc.GenerateDue(ctx, now, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		if result.Skipped != 1 || result.Generated != 0 || result.Failed != 0 {
			t.Fatalf("orphan transaction reconciliation result=%+v", result)
		}
		if rateSvc.calls.Load() != 0 {
			t.Fatalf("orphan reconciliation performed %d unnecessary rate lookups", rateSvc.calls.Load())
		}
		var txCount int
		var linkedID string
		var status string
		if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE idempotency_key = ?`, key).Scan(&txCount); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `SELECT transaction_id, status FROM recurring_transaction_instances WHERE rule_id = ?`, rule.ID).Scan(&linkedID, &status); err != nil {
			t.Fatal(err)
		}
		if txCount != 1 || linkedID != orphan.ID || status != string(model.RecurringInstanceGenerated) {
			t.Fatalf("orphan transaction not reconciled: count=%d linked=%q status=%q", txCount, linkedID, status)
		}
	})

	t.Run("generating instance without transaction", func(t *testing.T) {
		database := setupDB(t)
		defer database.Close()
		ctx := context.Background()
		accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
		account := recurringAccount(t, accountRepo, model.AccountTypePersonal)
		now := time.Now().UTC().Truncate(time.Second)
		rule := dueRecurringRule(account, now, account.Currency, 1)
		ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
		if err := ruleRepo.CreateRule(ctx, rule); err != nil {
			t.Fatal(err)
		}
		occurrenceDate := time.Unix(rule.NextRunAt, 0).UTC().Format("2006-01-02")
		key := "recurring:" + rule.ID + ":" + occurrenceDate
		instanceID := uuid.NewString()
		if _, err := database.ExecContext(ctx, `
			INSERT INTO recurring_transaction_instances (
				id, rule_id, user_id, occurrence_date, scheduled_at, transaction_id,
				idempotency_key, status, error, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, NULL, ?, 'generating', NULL, ?, ?)`,
			instanceID, rule.ID, testUserID, occurrenceDate, rule.NextRunAt, key,
			now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
		); err != nil {
			t.Fatal(err)
		}
		txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
		recurringSvc := service.NewRecurringTransactionService(
			ruleRepo, txRepo, service.NewTransactionService(txRepo, accountRepo, nil),
			sqliterepo.NewSQLiteTransactionManager(database), accountRepo,
		)

		result, err := recurringSvc.GenerateDue(ctx, now, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		if result.Generated != 1 || result.Failed != 0 {
			t.Fatalf("orphan instance reconciliation result=%+v", result)
		}
		var linkedID, status string
		if err := database.QueryRowContext(ctx, `SELECT transaction_id, status FROM recurring_transaction_instances WHERE id = ?`, instanceID).Scan(&linkedID, &status); err != nil {
			t.Fatal(err)
		}
		if linkedID == "" || status != string(model.RecurringInstanceGenerated) {
			t.Fatalf("orphan instance not reconciled: linked=%q status=%q", linkedID, status)
		}
	})

	t.Run("same-day schedule edit with generating instance", func(t *testing.T) {
		database := setupDB(t)
		defer database.Close()
		ctx := context.Background()
		accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
		account := recurringAccount(t, accountRepo, model.AccountTypePersonal)
		now := time.Date(2026, time.June, 1, 14, 0, 0, 0, time.UTC)
		rule := dueRecurringRule(account, now, account.Currency, 1)
		ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
		if err := ruleRepo.CreateRule(ctx, rule); err != nil {
			t.Fatal(err)
		}
		occurrenceDate := time.Unix(rule.NextRunAt, 0).UTC().Format("2006-01-02")
		key := "recurring:" + rule.ID + ":" + occurrenceDate
		instanceID := uuid.NewString()
		if _, err := database.ExecContext(ctx, `
			INSERT INTO recurring_transaction_instances (
				id, rule_id, user_id, occurrence_date, scheduled_at, transaction_id,
				idempotency_key, status, error, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, NULL, ?, 'generating', NULL, ?, ?)`,
			instanceID, rule.ID, testUserID, occurrenceDate, rule.NextRunAt, key,
			now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
		); err != nil {
			t.Fatal(err)
		}
		edited := rule
		edited.NextRunAt = rule.NextRunAt + int64((30 * time.Minute).Seconds())
		edited.TimeOfDay = time.Unix(edited.NextRunAt, 0).UTC().Format("15:04:05")
		if err := ruleRepo.UpdateRule(ctx, edited); err != nil {
			t.Fatal(err)
		}
		txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
		recurringSvc := service.NewRecurringTransactionService(
			ruleRepo, txRepo, service.NewTransactionService(txRepo, accountRepo, nil),
			sqliterepo.NewSQLiteTransactionManager(database), accountRepo,
		)

		result, err := recurringSvc.GenerateRuleNow(ctx, testUserID, rule.ID, false)
		if err != nil {
			t.Fatal(err)
		}
		if result.Generated != 1 || result.Failed != 0 {
			t.Fatalf("same-day orphan retry result=%+v", result)
		}
		var linkedID, status string
		var instanceScheduledAt, transactionTime int64
		if err := database.QueryRowContext(ctx, `
			SELECT transaction_id, status, scheduled_at
			FROM recurring_transaction_instances WHERE id = ?`, instanceID,
		).Scan(&linkedID, &status, &instanceScheduledAt); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `SELECT transaction_time FROM transactions WHERE id = ?`, linkedID).Scan(&transactionTime); err != nil {
			t.Fatal(err)
		}
		if status != string(model.RecurringInstanceGenerated) || instanceScheduledAt != edited.NextRunAt || transactionTime != edited.NextRunAt {
			t.Fatalf("reclaimed audit schedule mismatch: status=%q instance=%d transaction=%d want=%d", status, instanceScheduledAt, transactionTime, edited.NextRunAt)
		}
		current, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Version != rule.Version+2 || current.NextRunAt <= edited.NextRunAt {
			t.Fatalf("same-day orphan retry did not advance rule: %+v", current)
		}
	})

	t.Run("same-day schedule edit after generated occurrence", func(t *testing.T) {
		database := setupDB(t)
		defer database.Close()
		ctx := context.Background()
		accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
		account := recurringAccount(t, accountRepo, model.AccountTypePersonal)
		now := time.Date(2026, time.June, 1, 14, 0, 0, 0, time.UTC)
		rule := dueRecurringRule(account, now, account.Currency, 1)
		ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
		if err := ruleRepo.CreateRule(ctx, rule); err != nil {
			t.Fatal(err)
		}
		txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
		txSvc := service.NewTransactionService(txRepo, accountRepo, nil)
		occurrenceDate := time.Unix(rule.NextRunAt, 0).UTC().Format("2006-01-02")
		key := "recurring:" + rule.ID + ":" + occurrenceDate
		generated, err := txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
			UserID: testUserID, Mode: rule.Mode, OccurredAt: time.Unix(rule.NextRunAt, 0).UTC(),
			AccountID: rule.AccountID, TxType: rule.TxType, Category: rule.Category,
			AmountCents: rule.AmountCents, Currency: rule.Currency, ExchangeRate: rule.ExchangeRate,
			IdempotencyKey: &key, RecurringRuleID: &rule.ID, RecurringOccurrenceDate: &occurrenceDate,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.ExecContext(ctx, `
			INSERT INTO recurring_transaction_instances (
				id, rule_id, user_id, occurrence_date, scheduled_at, transaction_id,
				idempotency_key, status, error, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, 'generated', NULL, ?, ?)`,
			uuid.NewString(), rule.ID, testUserID, occurrenceDate, rule.NextRunAt, generated.ID, key,
			now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
		); err != nil {
			t.Fatal(err)
		}
		edited := rule
		edited.NextRunAt = rule.NextRunAt + int64((30 * time.Minute).Seconds())
		if err := ruleRepo.UpdateRule(ctx, edited); err != nil {
			t.Fatal(err)
		}
		recurringSvc := service.NewRecurringTransactionService(
			ruleRepo, txRepo, txSvc, sqliterepo.NewSQLiteTransactionManager(database), accountRepo,
		)

		result, err := recurringSvc.GenerateRuleNow(ctx, testUserID, rule.ID, false)
		if err != nil {
			t.Fatal(err)
		}
		if result.Skipped != 1 || result.Failed != 0 {
			t.Fatalf("same-day schedule reconciliation result=%+v", result)
		}
		current, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
		if err != nil {
			t.Fatal(err)
		}
		if current.NextRunAt <= edited.NextRunAt || current.Version != rule.Version+2 {
			t.Fatalf("same-day schedule edit remained stuck: %+v", current)
		}
	})
}

func TestRecurringRuleWritesRejectAccountsDeletedAfterValidation(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		database := setupDB(t)
		defer database.Close()
		ctx := context.Background()
		accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
		account := recurringAccount(t, accountRepo, model.AccountTypePersonal)
		wrappedAccounts := &deleteAfterAccountReadRepository{
			AccountRepository: accountRepo,
			targetID:          account.ID,
			delete: func() error {
				return accountRepo.Delete(ctx, account.ID, testUserID)
			},
		}
		txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
		ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
		recurringSvc := service.NewRecurringTransactionService(
			ruleRepo, txRepo, service.NewTransactionService(txRepo, wrappedAccounts, nil),
			sqliterepo.NewSQLiteTransactionManager(database), wrappedAccounts,
		)
		_, err := recurringSvc.CreateRule(ctx, service.UpsertRecurringRuleRequest{
			UserID: testUserID, Mode: model.ModeLife, Status: model.RecurringRuleStatusActive,
			Name: "create race", AccountID: account.ID, TxType: model.TxTypeExpense,
			Category: "test", AmountCents: 100, AmountProvided: true, Currency: account.Currency,
			Frequency: model.RecurringFrequencyDaily, Interval: 1,
			StartDate: time.Now().UTC().Format("2006-01-02"), TimeOfDay: "09:00:00",
			Timezone: "UTC", MonthEndPolicy: model.MonthEndClamp,
		})
		if err == nil {
			t.Fatal("created an active recurring rule after its account was deleted")
		}
		var count int
		if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM recurring_transaction_rules WHERE account_id = ?`, account.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("created %d rules for inactive account", count)
		}
	})

	t.Run("move existing rule", func(t *testing.T) {
		database := setupDB(t)
		defer database.Close()
		ctx := context.Background()
		accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
		source := recurringAccount(t, accountRepo, model.AccountTypePersonal)
		target := model.Account{
			ID: uuid.NewString(), UserID: testUserID, Name: "move target",
			Type: model.AccountTypePersonal, Currency: source.Currency, IsActive: true,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		if err := accountRepo.Create(ctx, target); err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC().Truncate(time.Second)
		rule := dueRecurringRule(source, now, source.Currency, 1)
		ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
		if err := ruleRepo.CreateRule(ctx, rule); err != nil {
			t.Fatal(err)
		}
		wrappedAccounts := &deleteAfterAccountReadRepository{
			AccountRepository: accountRepo,
			targetID:          target.ID,
			delete: func() error {
				return accountRepo.Delete(ctx, target.ID, testUserID)
			},
		}
		txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
		recurringSvc := service.NewRecurringTransactionService(
			ruleRepo, txRepo, service.NewTransactionService(txRepo, wrappedAccounts, nil),
			sqliterepo.NewSQLiteTransactionManager(database), wrappedAccounts,
		)
		_, err := recurringSvc.UpdateRule(ctx, service.UpsertRecurringRuleRequest{
			ID: rule.ID, UserID: testUserID, AccountID: target.ID,
		})
		if !errors.Is(err, service.ErrConcurrentModification) {
			t.Fatalf("move to concurrently deleted account = %v, want concurrent modification", err)
		}
		current, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
		if err != nil {
			t.Fatal(err)
		}
		if current.AccountID != source.ID || current.Version != rule.Version {
			t.Fatalf("rule moved to inactive account: %+v", current)
		}
	})

	t.Run("reactivate ended rule", func(t *testing.T) {
		database := setupDB(t)
		defer database.Close()
		ctx := context.Background()
		accountRepo := sqliterepo.NewSQLiteAccountRepository(database)
		account := recurringAccount(t, accountRepo, model.AccountTypePersonal)
		now := time.Now().UTC().Truncate(time.Second)
		rule := dueRecurringRule(account, now, account.Currency, 1)
		ruleRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
		if err := ruleRepo.CreateRule(ctx, rule); err != nil {
			t.Fatal(err)
		}
		if err := accountRepo.Delete(ctx, account.ID, testUserID); err != nil {
			t.Fatal(err)
		}
		txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
		recurringSvc := service.NewRecurringTransactionService(
			ruleRepo, txRepo, service.NewTransactionService(txRepo, accountRepo, nil),
			sqliterepo.NewSQLiteTransactionManager(database), accountRepo,
		)
		if _, err := recurringSvc.SetStatus(ctx, testUserID, rule.ID, model.RecurringRuleStatusActive); err == nil {
			t.Fatal("reactivated recurring rule bound to an inactive account")
		}
		current, err := ruleRepo.GetRuleByID(ctx, rule.ID, testUserID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status != model.RecurringRuleStatusEnded {
			t.Fatalf("inactive-account rule status=%s, want ended", current.Status)
		}
	})
}
