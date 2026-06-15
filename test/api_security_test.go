package test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	"finarch/internal/infrastructure/email"
	"finarch/internal/infrastructure/ocr"
	sqliterepo "finarch/internal/infrastructure/repository"
	filestorage "finarch/internal/infrastructure/storage"
	"finarch/internal/interface/apiv1"
)

func newTestServer(t *testing.T, database *sql.DB, jwtSvc *auth.JWTService) *apiv1.Server {
	t.Helper()
	t.Setenv("FINARCH_STATIC", t.TempDir())
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	reimRepo := sqliterepo.NewSQLiteReimbursementRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	userRepo := sqliterepo.NewSQLiteUserRepository(database)
	tagRepo := sqliterepo.NewSQLiteTagRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	attachmentStorage, err := filestorage.NewLocalAttachmentStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	authSvc := service.NewAuthService(userRepo, jwtSvc, auth.NewActionTokenService("test-secret"), auth.NewLoginAttemptTracker(5, time.Minute), &email.NoopSender{}, false, "http://localhost", tm)
	return apiv1.NewServer(":0", database, ":memory:", txRepo, tagRepo, tm,
		service.NewTransactionService(txRepo, acctRepo, service.NewHTTPExchangeRateService()),
		service.NewReimbursementService(tm, txRepo, reimRepo),
		service.NewMatchingService(txRepo),
		authSvc,
		service.NewStatsService(database),
		service.NewBudgetService(sqliterepo.NewSQLiteBudgetRepository(database)),
		service.NewRecurringTransactionService(sqliterepo.NewSQLiteRecurringTransactionRepository(database), txRepo, service.NewTransactionService(txRepo, acctRepo, service.NewHTTPExchangeRateService()), tm, acctRepo),
		service.NewAttachmentService(sqliterepo.NewSQLiteAttachmentRepository(database), txRepo, attachmentStorage, ocr.NoneProvider{}, service.DefaultAttachmentMaxBytes, tm),
		jwtSvc,
		auth.NewIPRateLimiter(10, time.Minute),
		auth.NewTurnstileVerifier(""),
		"",
		service.NewAccountService(acctRepo, txRepo, tm),
		&email.NoopSender{},
	)
}

func serveTestRequest(s *apiv1.Server, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func TestDisasterRecoveryRoutesRequireJWT(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	srv := newTestServer(t, database, auth.NewJWTService("test-secret"))

	w := serveTestRequest(srv, httptest.NewRequest(http.MethodGet, "/api/v1/disaster-recovery/snapshots", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("snapshots without JWT: got %d, want %d", w.Code, http.StatusUnauthorized)
	}

	body := bytes.NewBufferString(`{"snapshot_id":"x","confirm":true,"authorization_token":"x"}`)
	w = serveTestRequest(srv, httptest.NewRequest(http.MethodPost, "/api/v1/disaster-recovery/restore", body))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("restore without JWT: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddlewareRejectsDeletedUserToken(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	jwtSvc := auth.NewJWTService("test-secret")
	token, _, err := jwtSvc.Issue(testUserID, "test@example.com", "user", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(context.Background(), `UPDATE users SET deleted_at = ? WHERE id = ?`, time.Now().Unix(), testUserID); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, database, jwtSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveTestRequest(srv, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("deleted user token: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestRefreshRejectsDeletedUserToken(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	jwtSvc := auth.NewJWTService("test-secret")
	token, _, err := jwtSvc.Issue(testUserID, "test@example.com", "user", 0)
	if err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, database, jwtSvc)
	if _, err := database.ExecContext(context.Background(), `UPDATE users SET deleted_at = ? WHERE id = ?`, time.Now().Unix(), testUserID); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveTestRequest(srv, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh for deleted user: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDisasterRecoveryExecuteRequiresStepUpToken(t *testing.T) {
	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, "snapshots.json")
	if err := os.WriteFile(metadataPath, []byte(`[{"snapshot_id":"x","created_at":"2026-01-01T00:00:00Z","schema_version":1,"app_version":"test","environment":"production","db_size":1}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DISASTER_SNAPSHOT_METADATA_PATH", metadataPath)
	database := setupDB(t)
	defer database.Close()
	jwtSvc := auth.NewJWTService("test-secret")
	token, _, err := jwtSvc.Issue(testUserID, "test@example.com", "user", 0)
	if err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, database, jwtSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/disaster-recovery/restore", bytes.NewBufferString(`{"snapshot_id":"x","confirm":true,"authorization_token":"not-a-token"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := serveTestRequest(srv, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("restore with invalid step-up token: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDisasterRecoveryFailedRestoreDoesNotConsumeStepUpToken(t *testing.T) {
	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, "snapshots.json")
	if err := os.WriteFile(metadataPath, []byte(`[{"snapshot_id":"x","created_at":"2026-01-01T00:00:00Z","schema_version":1,"app_version":"test","environment":"production","db_size":1}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DISASTER_SNAPSHOT_METADATA_PATH", metadataPath)
	t.Setenv("PATH", tmpDir)

	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, mustHashPassword(t, "Password123"), testUserID); err != nil {
		t.Fatal(err)
	}
	jwtSvc := auth.NewJWTService("test-secret")
	jwtToken, _, err := jwtSvc.Issue(testUserID, "test@example.com", "user", 0)
	if err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, database, jwtSvc)

	authReq := httptest.NewRequest(http.MethodPost, "/api/v1/disaster-recovery/authorize", bytes.NewBufferString(`{"current_password":"Password123"}`))
	authReq.Header.Set("Authorization", "Bearer "+jwtToken)
	authReq.Header.Set("Content-Type", "application/json")
	authResp := serveTestRequest(srv, authReq)
	if authResp.Code != http.StatusOK {
		t.Fatalf("authorize: got %d body=%s", authResp.Code, authResp.Body.String())
	}
	var authBody struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(authResp.Body.Bytes(), &authBody); err != nil {
		t.Fatal(err)
	}
	if authBody.Data.Token == "" {
		t.Fatal("missing disaster recovery authorization token")
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/disaster-recovery/restore", bytes.NewBufferString(`{"snapshot_id":"x","confirm":true,"authorization_token":"`+authBody.Data.Token+`"}`))
	restoreReq.Header.Set("Authorization", "Bearer "+jwtToken)
	restoreReq.Header.Set("Content-Type", "application/json")
	restoreResp := serveTestRequest(srv, restoreReq)
	if restoreResp.Code != http.StatusInternalServerError {
		t.Fatalf("restore: got %d, want %d body=%s", restoreResp.Code, http.StatusInternalServerError, restoreResp.Body.String())
	}

	var status string
	if err := database.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE action = ?`, service.ActionDisasterRecovery).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("failed restore consumed step-up token: status=%s", status)
	}
	if got := auditEventCount(t, database, testUserID, "disaster_recovery_executed"); got != 0 {
		t.Fatalf("failed restore wrote execution audit event: got %d", got)
	}
}

func mustHashPassword(t *testing.T, password string) string {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
