package test

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainrepo "finarch/internal/domain/repository"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	sqliterepo "finarch/internal/infrastructure/repository"
)

type observedTransactionRateService struct {
	started chan struct{}
	release <-chan struct{}
	onCall  func() error
	once    sync.Once
	calls   atomic.Int32
}

func (s *observedTransactionRateService) GetRate(ctx context.Context, _, _ string, at time.Time) (service.ExchangeRateResult, error) {
	s.calls.Add(1)
	s.once.Do(func() {
		if s.started != nil {
			close(s.started)
		}
	})
	if s.onCall != nil {
		if err := s.onCall(); err != nil {
			return service.ExchangeRateResult{}, err
		}
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return service.ExchangeRateResult{}, ctx.Err()
		}
	}
	return service.ExchangeRateResult{
		Rate:      big.NewRat(3, 2),
		RateFloat: 1.5,
		Source:    "test-provider",
		At:        at.UTC(),
	}, nil
}

type beforeFirstTransactionManager struct {
	inner  domainrepo.TransactionManager
	before func() error
	once   sync.Once
	err    error
}

func (m *beforeFirstTransactionManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	m.once.Do(func() { m.err = m.before() })
	if m.err != nil {
		return m.err
	}
	return m.inner.WithinTransaction(ctx, fn)
}

func transactionRateTestAccountID(t *testing.T, database interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) string {
	t.Helper()
	var accountID string
	if err := database.QueryRowContext(context.Background(),
		`SELECT id FROM accounts WHERE user_id = ? AND type = 'public' AND is_active = 1 LIMIT 1`,
		testUserID,
	).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	return accountID
}

func TestCreateTransactionRateLookupDoesNotHoldWriteLockAndReplaySkipsProvider(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	database.SetMaxOpenConns(8)
	accountID := transactionRateTestAccountID(t, database)
	j := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, j, testUserID)

	started := make(chan struct{})
	release := make(chan struct{})
	rateSvc := &observedTransactionRateService{started: started, release: release}
	srv := newTestServerWithTransactionDependencies(t, database, j, rateSvc, nil)
	payload := fmt.Sprintf(`{"occurred_at":"2026-09-04 12:00:00","account_id":%q,"type":"expense","mode":"work","category":"food","amount_cents":1234,"currency":"EUR"}`, accountID)

	responseCh := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		responseCh <- createTransactionRequestForTest(srv, token, "blocked-rate-key", payload)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("exchange-rate provider was not called")
	}

	writeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	_, writeErr := database.ExecContext(writeCtx, `UPDATE users SET updated_at = updated_at + 1 WHERE id = ?`, testUserID)
	cancel()
	close(release)
	if writeErr != nil {
		t.Fatalf("provider held SQLite writer lock: %v", writeErr)
	}

	var first *httptest.ResponseRecorder
	select {
	case first = <-responseCh:
	case <-time.After(2 * time.Second):
		t.Fatal("create did not finish after provider release")
	}
	if first.Code != http.StatusCreated {
		t.Fatalf("first create: status=%d body=%s", first.Code, first.Body.String())
	}
	firstID := createdTransactionID(t, first)

	replay := createTransactionRequestForTest(srv, token, "blocked-rate-key", payload)
	if replay.Code != http.StatusCreated || createdTransactionID(t, replay) != firstID {
		t.Fatalf("replay: status=%d body=%s", replay.Code, replay.Body.String())
	}
	if replay.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatal("replay response was not marked")
	}
	if calls := rateSvc.calls.Load(); calls != 1 {
		t.Fatalf("provider calls after replay=%d, want 1", calls)
	}

	vanishingManager := &beforeFirstTransactionManager{
		inner: sqliterepo.NewSQLiteTransactionManager(database),
		before: func() error {
			if _, err := database.ExecContext(context.Background(), `DELETE FROM transactions WHERE user_id = ?`, testUserID); err != nil {
				return err
			}
			_, err := database.ExecContext(context.Background(), `DELETE FROM idempotency_keys WHERE user_id = ?`, testUserID)
			return err
		},
	}
	vanishingServer := newTestServerWithTransactionDependencies(t, database, j, rateSvc, vanishingManager)
	vanished := createTransactionRequestForTest(vanishingServer, token, "blocked-rate-key", payload)
	if vanished.Code != http.StatusConflict || apiErrorCode(t, vanished) != "concurrent_modification" {
		t.Fatalf("vanished replay: status=%d body=%s", vanished.Code, vanished.Body.String())
	}
	if calls := rateSvc.calls.Load(); calls != 1 {
		t.Fatalf("vanished replay called provider: calls=%d", calls)
	}
	var transactionCount, keyCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM transactions WHERE user_id = ?`, testUserID).Scan(&transactionCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM idempotency_keys WHERE user_id = ?`, testUserID).Scan(&keyCount); err != nil {
		t.Fatal(err)
	}
	if transactionCount != 0 || keyCount != 0 {
		t.Fatalf("vanished replay was recreated: transactions=%d keys=%d", transactionCount, keyCount)
	}
}

func TestCreateTransactionPreparedRateMismatchRollsBackReservation(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	accountID := transactionRateTestAccountID(t, database)
	j := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, j, testUserID)
	rateSvc := &observedTransactionRateService{onCall: func() error {
		_, err := database.ExecContext(context.Background(), `UPDATE accounts SET currency = 'USD' WHERE id = ?`, accountID)
		return err
	}}
	srv := newTestServerWithTransactionDependencies(t, database, j, rateSvc, nil)
	payload := fmt.Sprintf(`{"occurred_at":"2026-09-04 12:00:00","account_id":%q,"type":"expense","mode":"work","category":"food","amount_cents":1234,"currency":"EUR"}`, accountID)

	response := createTransactionRequestForTest(srv, token, "rate-mismatch-key", payload)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("mismatched evidence: status=%d body=%s", response.Code, response.Body.String())
	}
	if calls := rateSvc.calls.Load(); calls != 1 {
		t.Fatalf("mismatched evidence provider calls=%d, want 1", calls)
	}
	var transactionCount, keyCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM transactions WHERE user_id = ?`, testUserID).Scan(&transactionCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM idempotency_keys WHERE user_id = ?`, testUserID).Scan(&keyCount); err != nil {
		t.Fatal(err)
	}
	if transactionCount != 0 || keyCount != 0 {
		t.Fatalf("mismatch did not roll back: transactions=%d keys=%d", transactionCount, keyCount)
	}
}

func TestCreateTransactionTagFailureRollsBackPreparedFinancialWrite(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	accountID := transactionRateTestAccountID(t, database)
	j := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, j, testUserID)
	rateSvc := &observedTransactionRateService{}
	srv := newTestServerWithTransactionDependencies(t, database, j, rateSvc, nil)
	payload := fmt.Sprintf(`{"occurred_at":"2026-09-04 12:00:00","account_id":%q,"type":"expense","mode":"work","category":"food","amount_cents":1234,"currency":"EUR","project_id":"rolled-back-project","tag_ids":["missing-tag"]}`, accountID)

	response := createTransactionRequestForTest(srv, token, "tag-rollback-key", payload)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("tag failure: status=%d body=%s", response.Code, response.Body.String())
	}
	if calls := rateSvc.calls.Load(); calls != 1 {
		t.Fatalf("tag failure provider calls=%d, want 1", calls)
	}
	var transactionCount, keyCount, projectCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM transactions WHERE user_id = ?`, testUserID).Scan(&transactionCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM idempotency_keys WHERE user_id = ?`, testUserID).Scan(&keyCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = 'rolled-back-project'`).Scan(&projectCount); err != nil {
		t.Fatal(err)
	}
	if transactionCount != 0 || keyCount != 0 || projectCount != 0 {
		t.Fatalf("tag failure did not roll back: transactions=%d keys=%d projects=%d", transactionCount, keyCount, projectCount)
	}
}
