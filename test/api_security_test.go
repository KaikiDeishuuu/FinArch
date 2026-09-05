package test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	domainrepo "finarch/internal/domain/repository"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	"finarch/internal/infrastructure/email"
	"finarch/internal/infrastructure/ocr"
	sqliterepo "finarch/internal/infrastructure/repository"
	filestorage "finarch/internal/infrastructure/storage"
	"finarch/internal/interface/apiv1"

	"github.com/golang-jwt/jwt/v5"
)

const testOperationsSecret = "0123456789abcdef0123456789abcdef"

func enableTestSystemOperations(t *testing.T) {
	t.Helper()
	t.Setenv("FINARCH_ENABLE_SYSTEM_OPERATIONS", "true")
	t.Setenv("FINARCH_SYSTEM_OPERATIONS_SECRET", testOperationsSecret)
}

func authorizeTestSystemOperations(req *http.Request) {
	req.Header.Set("X-FinArch-Operations-Secret", testOperationsSecret)
}

func newTestServer(t *testing.T, database *sql.DB, jwtSvc *auth.JWTService) *apiv1.Server {
	return newTestServerWithTransactionDependencies(t, database, jwtSvc, service.NewHTTPExchangeRateService(), nil)
}

func newTestServerWithTransactionDependencies(
	t *testing.T,
	database *sql.DB,
	jwtSvc *auth.JWTService,
	rateSvc service.ExchangeRateService,
	serverTransactionManager domainrepo.TransactionManager,
) *apiv1.Server {
	t.Helper()
	t.Setenv("FINARCH_STATIC", t.TempDir())
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	reimRepo := sqliterepo.NewSQLiteReimbursementRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	userRepo := sqliterepo.NewSQLiteUserRepository(database)
	tagRepo := sqliterepo.NewSQLiteTagRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	if serverTransactionManager == nil {
		serverTransactionManager = tm
	}
	attachmentStorage, err := filestorage.NewLocalAttachmentStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	authSvc := service.NewAuthService(userRepo, auth.NewActionTokenService("test-secret"), auth.NewLoginAttemptTracker(5, time.Minute), &email.NoopSender{}, false, "http://localhost", tm, newTestSessionService(t, database, jwtSvc, "test-secret"))
	return apiv1.NewServer("127.0.0.1:0", database, ":memory:", txRepo, tagRepo, serverTransactionManager,
		service.NewTransactionService(txRepo, acctRepo, rateSvc),
		service.NewReimbursementService(tm, txRepo, reimRepo),
		service.NewMatchingService(txRepo),
		authSvc,
		service.NewStatsService(database),
		service.NewBudgetService(sqliterepo.NewSQLiteBudgetRepository(database)),
		service.NewRecurringTransactionService(sqliterepo.NewSQLiteRecurringTransactionRepository(database), txRepo, service.NewTransactionService(txRepo, acctRepo, service.NewHTTPExchangeRateService()), tm, acctRepo),
		service.NewAttachmentService(sqliterepo.NewSQLiteAttachmentRepository(database), txRepo, attachmentStorage, ocr.NoneProvider{}, service.DefaultAttachmentMaxBytes, tm),
		auth.NewIPRateLimiter(10, time.Minute),
		auth.NewTurnstileVerifier(""),
		"",
		service.NewAccountService(acctRepo, txRepo, tm),
		&email.NoopSender{},
		apiv1.BrowserSessionOptions{Secure: false, AllowOriginless: true},
	)
}

func serveTestRequest(s *apiv1.Server, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func apiErrorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode API error %q: %v", response.Body.String(), err)
	}
	if body.Success || body.Error.Code == "" {
		t.Fatalf("invalid API error envelope: %s", response.Body.String())
	}
	return body.Error.Code
}

func expireTestAccessToken(t *testing.T, rawToken, secret string) string {
	t.Helper()
	claims := &auth.Claims{}
	if _, _, err := jwt.NewParser().ParseUnverified(rawToken, claims); err != nil {
		t.Fatalf("parse issued access token: %v", err)
	}
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign expired access token: %v", err)
	}
	return signed
}

func expireTestAccessTokenWithInvalidIssuer(t *testing.T, rawToken, secret string) string {
	t.Helper()
	claims := &auth.Claims{}
	if _, _, err := jwt.NewParser().ParseUnverified(rawToken, claims); err != nil {
		t.Fatalf("parse issued access token: %v", err)
	}
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	claims.Issuer = "not-finarch"
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign composite-invalid access token: %v", err)
	}
	return signed
}

func issueTestAccessSession(t *testing.T, database *sql.DB, jwtSvc *auth.JWTService, userID string) string {
	t.Helper()
	user, err := sqliterepo.NewSQLiteUserRepository(database).GetByID(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := newTestSessionService(t, database, jwtSvc, "test-secret").CreateSession(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	return tokens.AccessToken
}

func TestLoginInvalidCredentialsReturnsUnauthorized(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	srv := newTestServer(t, database, auth.NewJWTService("test-secret"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"test@example.com","password":"wrong-password"}`))
	req.Header.Set("Content-Type", "application/json")
	w := serveTestRequest(srv, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid login: got %d body=%s, want %d", w.Code, w.Body.String(), http.StatusUnauthorized)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "invalid_credentials" {
		t.Fatalf("error code = %q, want invalid_credentials", body.Error.Code)
	}
	if body.Error.Message == "Something went wrong. Please try again." {
		t.Fatalf("invalid login returned generic error message")
	}
}

func TestEmailVerificationGETEndpointsNeverConsumeTokens(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	srv := newTestServer(t, database, auth.NewJWTService("test-secret"))

	for _, path := range []string{
		"/verify-email?token=mail-scanner-token",
		"/api/v1/auth/verify-email?token=mail-scanner-token",
	} {
		response := serveTestRequest(srv, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("GET %s status=%d location=%q body=%s, want read-only 404/fallback", path, response.Code, response.Header().Get("Location"), response.Body.String())
		}
		if location := response.Header().Get("Location"); location != "" {
			t.Errorf("GET %s redirected to %q; verification GET must have no action result", path, location)
		}
	}
}

func TestResetPasswordHasIndependentIPRateLimit(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	srv := newTestServer(t, database, auth.NewJWTService("test-secret"))

	for attempt := 1; attempt <= 11; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", bytes.NewBufferString(`{"token":"invalid","new_password":"ReplacementPassword123"}`))
		req.Header.Set("Content-Type", "application/json")
		response := serveTestRequest(srv, req)
		if attempt <= 10 && response.Code == http.StatusTooManyRequests {
			t.Fatalf("reset attempt %d was limited too early", attempt)
		}
		if attempt == 11 {
			if response.Code != http.StatusTooManyRequests || apiErrorCode(t, response) != "reset_password_rate_limited" {
				t.Fatalf("reset limit status=%d body=%s", response.Code, response.Body.String())
			}
		}
	}

	// The dedicated reset budget must not consume the shared login budget.
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"test@example.com","password":"wrong-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	if response := serveTestRequest(srv, loginReq); response.Code == http.StatusTooManyRequests {
		t.Fatalf("reset attempts exhausted the login rate limit: %s", response.Body.String())
	}
}

func TestLoginStoreFailureReturnsServiceUnavailableWithoutLockout(t *testing.T) {
	database := setupDB(t)
	srv := newTestServer(t, database, auth.NewJWTService("test-secret"))
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	for attempt := 1; attempt <= 6; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString("{\"email\":\"test@example.com\",\"password\":\"Password123\"}"))
		req.Header.Set("Content-Type", "application/json")
		response := serveTestRequest(srv, req)
		if response.Code != http.StatusServiceUnavailable || apiErrorCode(t, response) != "system_unavailable" {
			t.Fatalf("closed-store login attempt %d: status=%d body=%s", attempt, response.Code, response.Body.String())
		}
	}
}

func TestPublicAuthJSONBodyLimitRejectsContentLengthAndChunked(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	srv := newTestServer(t, database, auth.NewJWTService("test-secret"))
	padding := strings.Repeat("x", (1<<20)+128)
	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "login",
			path: "/api/v1/auth/login",
			body: "{\"email\":\"test@example.com\",\"password\":\"Password123\",\"padding\":\"" + padding + "\"}",
		},
		{
			name: "register",
			path: "/api/v1/auth/register",
			body: "{\"email\":\"new@example.com\",\"username\":\"new-user\",\"password\":\"Password123\",\"padding\":\"" + padding + "\"}",
		},
	}
	for _, tc := range tests {
		for _, chunked := range []bool{false, true} {
			mode := "content-length"
			if chunked {
				mode = "chunked"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
				if chunked {
					req.ContentLength = -1
					req.TransferEncoding = []string{"chunked"}
				}
				response := serveTestRequest(srv, req)
				if response.Code != http.StatusRequestEntityTooLarge || apiErrorCode(t, response) != "request_too_large" {
					t.Fatalf("status=%d body=%s, want 413/request_too_large", response.Code, response.Body.String())
				}
			})
		}
	}
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

func TestPublicUploadRestoreRoutesAreRemoved(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	srv := newTestServer(t, database, auth.NewJWTService("test-secret"))

	for _, path := range []string{
		"/api/v1/backup/restore-request",
		"/api/v1/backup/restore-confirm",
	} {
		w := serveTestRequest(srv, httptest.NewRequest(http.MethodPost, path, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("public restore endpoint %s: got %d body=%s, want %d", path, w.Code, w.Body.String(), http.StatusNotFound)
		}
	}
}

func TestBackupExportRouterContract(t *testing.T) {
	enableTestSystemOperations(t)
	database := setupDB(t)
	defer database.Close()
	if _, err := database.ExecContext(context.Background(),
		`UPDATE users SET password_hash = ? WHERE id = ?`,
		mustHashPassword(t, "Password123"), testUserID,
	); err != nil {
		t.Fatal(err)
	}

	jwtSvc := auth.NewJWTService("test-secret")
	accessToken := issueTestAccessSession(t, database, jwtSvc, testUserID)
	srv := newTestServer(t, database, jwtSvc)
	authorizedRequest := func(method, target string, body *bytes.Buffer) *http.Request {
		t.Helper()
		var requestBody *bytes.Reader
		if body == nil {
			requestBody = bytes.NewReader(nil)
		} else {
			requestBody = bytes.NewReader(body.Bytes())
		}
		req := httptest.NewRequest(method, target, requestBody)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		authorizeTestSystemOperations(req)
		return req
	}

	exportRequest := authorizedRequest(http.MethodPost, "/api/v1/backup/export-request", bytes.NewBufferString(`{"current_password":"Password123"}`))
	exportRequest.Header.Set("Content-Type", "application/json")
	exportResponse := serveTestRequest(srv, exportRequest)
	if exportResponse.Code != http.StatusOK {
		t.Fatalf("export request: got %d body=%s, want 200", exportResponse.Code, exportResponse.Body.String())
	}
	if cacheControl := exportResponse.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("export request Cache-Control = %q, want no-store", cacheControl)
	}
	var exportBody struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(exportResponse.Body.Bytes(), &exportBody); err != nil {
		t.Fatal(err)
	}
	if exportBody.Data.Token == "" {
		t.Fatalf("export response has no token: %s", exportResponse.Body.String())
	}

	legacyGET := authorizedRequest(http.MethodGet, "/api/v1/backup/download?export_token="+exportBody.Data.Token, nil)
	if response := serveTestRequest(srv, legacyGET); response.Code != http.StatusNotFound {
		t.Fatalf("legacy GET download: got %d body=%s, want 404", response.Code, response.Body.String())
	}

	queryOnly := authorizedRequest(http.MethodPost, "/api/v1/backup/download?export_token="+exportBody.Data.Token, nil)
	if response := serveTestRequest(srv, queryOnly); response.Code != http.StatusForbidden {
		t.Fatalf("query-only POST download: got %d body=%s, want 403", response.Code, response.Body.String())
	}

	headerRequest := authorizedRequest(http.MethodPost, "/api/v1/backup/download", nil)
	headerRequest.Header.Set("X-FinArch-Export-Token", exportBody.Data.Token)
	if response := serveTestRequest(srv, headerRequest); response.Code != http.StatusOK {
		t.Fatalf("header-authorized POST download: got %d body=%s, want 200", response.Code, response.Body.String())
	}
}

func TestOrdinaryTenantCannotAccessPhysicalDatabaseOperations(t *testing.T) {
	t.Setenv("FINARCH_ENABLE_SYSTEM_OPERATIONS", "")
	t.Setenv("FINARCH_SYSTEM_OPERATIONS_SECRET", "")
	database := setupDB(t)
	defer database.Close()
	if _, err := database.ExecContext(context.Background(), `
		INSERT INTO users(id, email, username, name, password_hash, role, created_at, updated_at)
		VALUES ('other-tenant', 'other@example.com', 'other-tenant', 'Other Tenant', 'x', 'owner', ?, ?)
	`, time.Now().Unix(), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}

	jwtSvc := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, jwtSvc, testUserID)
	srv := newTestServer(t, database, jwtSvc)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/backup/export-request", `{"current_password":"irrelevant"}`},
		{http.MethodPost, "/api/v1/backup/download", ""},
		{http.MethodGet, "/api/v1/backup/litestream-health", ""},
		{http.MethodGet, "/api/v1/disaster-recovery/snapshots", ""},
		{http.MethodPost, "/api/v1/backup/restore", ""},
		{http.MethodPost, "/api/v1/backup/restore/send-verification", ""},
		{http.MethodPost, "/api/v1/backup/restore/verify", `{"restore_id":"x","code":"000000"}`},
		{http.MethodPost, "/api/v1/backup/restore/execute", `{"restore_token":"x"}`},
		{http.MethodPost, "/api/v1/disaster-recovery/authorize", `{"current_password":"irrelevant"}`},
		{http.MethodPost, "/api/v1/disaster-recovery/restore", `{"snapshot_id":"x","confirm":true,"authorization_token":"x"}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		authorizeTestSystemOperations(req)
		w := serveTestRequest(srv, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("ordinary tenant %s %s: got %d body=%s, want %d", tc.method, tc.path, w.Code, w.Body.String(), http.StatusForbidden)
		}
	}
}

func TestEnabledSystemOperationsStillRequireIndependentSecret(t *testing.T) {
	enableTestSystemOperations(t)
	database := setupDB(t)
	defer database.Close()
	jwtSvc := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, jwtSvc, testUserID)
	srv := newTestServer(t, database, jwtSvc)

	for _, supplied := range []string{"", "wrong-secret"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/disaster-recovery/snapshots", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if supplied != "" {
			req.Header.Set("X-FinArch-Operations-Secret", supplied)
		}
		w := serveTestRequest(srv, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("operations secret %q: got %d body=%s, want %d", supplied, w.Code, w.Body.String(), http.StatusForbidden)
		}
	}
}

func TestConfigExposesOnlySystemOperationsAvailability(t *testing.T) {
	enableTestSystemOperations(t)
	database := setupDB(t)
	defer database.Close()
	srv := newTestServer(t, database, auth.NewJWTService("test-secret"))

	w := serveTestRequest(srv, httptest.NewRequest(http.MethodGet, "/api/v1/config", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("config: got %d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), testOperationsSecret) {
		t.Fatal("public config leaked the operations secret")
	}
	var body struct {
		Data struct {
			SystemOperationsEnabled bool `json:"system_operations_enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Data.SystemOperationsEnabled {
		t.Fatal("config should expose enabled operations as a boolean")
	}
}

func TestJWTMiddlewareRejectsDeletedUserToken(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	jwtSvc := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, jwtSvc, testUserID)
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

func TestProtectedAccessErrorContract(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	jwtSvc := auth.NewJWTService("test-secret")
	freshToken := issueTestAccessSession(t, database, jwtSvc, testUserID)
	srv := newTestServer(t, database, jwtSvc)

	request := func(authorization string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		return serveTestRequest(srv, req)
	}
	tests := []struct {
		name          string
		authorization string
		wantCode      string
	}{
		{name: "missing", wantCode: "access_missing"},
		{name: "malformed", authorization: "Basic invalid", wantCode: "access_invalid"},
		{name: "bad signature", authorization: "Bearer " + freshToken + "x", wantCode: "access_invalid"},
		{name: "expired", authorization: "Bearer " + expireTestAccessToken(t, freshToken, "test-secret"), wantCode: "access_expired"},
		{name: "expired with wrong issuer", authorization: "Bearer " + expireTestAccessTokenWithInvalidIssuer(t, freshToken, "test-secret"), wantCode: "access_invalid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := request(tc.authorization)
			if response.Code != http.StatusUnauthorized || apiErrorCode(t, response) != tc.wantCode {
				t.Fatalf("status=%d body=%s, want 401/%s", response.Code, response.Body.String(), tc.wantCode)
			}
		})
	}

	if _, err := database.ExecContext(context.Background(), `UPDATE auth_sessions SET revoked_at = ? WHERE user_id = ?`, time.Now().Unix(), testUserID); err != nil {
		t.Fatal(err)
	}
	revoked := request("Bearer " + freshToken)
	if revoked.Code != http.StatusUnauthorized || apiErrorCode(t, revoked) != "session_invalid" {
		t.Fatalf("revoked session: status=%d body=%s", revoked.Code, revoked.Body.String())
	}
}

func TestProtectedAccessStoreFailureReturnsServiceUnavailable(t *testing.T) {
	database := setupDB(t)
	jwtSvc := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, jwtSvc, testUserID)
	srv := newTestServer(t, database, jwtSvc)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response := serveTestRequest(srv, req)
	if response.Code != http.StatusServiceUnavailable || apiErrorCode(t, response) != "system_unavailable" {
		t.Fatalf("closed session store: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMalformedRefreshCookiesNeverReachClosedStore(t *testing.T) {
	database := setupDB(t)
	jwtSvc := auth.NewJWTService("test-secret")
	srv := newTestServer(t, database, jwtSvc)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	canonical := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	tests := []struct {
		name       string
		cookies    []string
		rawHeaders []string
	}{
		{name: "missing"},
		{name: "wrong length", cookies: []string{"short"}},
		{name: "invalid alphabet", cookies: []string{strings.Repeat("!", 43)}},
		{name: "duplicate", cookies: []string{canonical, canonical}},
		{
			name: "valid plus same-name malformed raw pair",
			rawHeaders: []string{
				"finarch_refresh=" + canonical,
				"finarch_refresh=" + canonical + "\\invalid",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			newRequest := func(path string) *http.Request {
				req := httptest.NewRequest(http.MethodPost, path, nil)
				for _, value := range tc.cookies {
					req.AddCookie(&http.Cookie{Name: "finarch_refresh", Value: value})
				}
				for _, header := range tc.rawHeaders {
					req.Header.Add("Cookie", header)
				}
				return req
			}
			refresh := serveTestRequest(srv, newRequest("/api/v1/auth/refresh"))
			if refresh.Code != http.StatusUnauthorized || apiErrorCode(t, refresh) != "session_invalid" {
				t.Fatalf("refresh status=%d body=%s", refresh.Code, refresh.Body.String())
			}
			logout := serveTestRequest(srv, newRequest("/api/v1/auth/logout"))
			if logout.Code != http.StatusOK {
				t.Fatalf("logout touched closed store: status=%d body=%s", logout.Code, logout.Body.String())
			}
		})
	}
}

func TestRefreshCookieSessionLifecycle(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, mustHashPassword(t, "Password123"), testUserID); err != nil {
		t.Fatal(err)
	}
	jwtSvc := auth.NewJWTService("test-secret")
	srv := newTestServer(t, database, jwtSvc)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"test@example.com","password":"Password123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	login := serveTestRequest(srv, loginReq)
	if login.Code != http.StatusOK {
		t.Fatalf("login: got %d body=%s", login.Code, login.Body.String())
	}
	loginCookies := login.Result().Cookies()
	if len(loginCookies) != 1 || loginCookies[0].Name != "finarch_refresh" || !loginCookies[0].HttpOnly || loginCookies[0].Path != "/" {
		t.Fatalf("login refresh cookie = %#v", loginCookies)
	}
	initialCookie := loginCookies[0]
	if strings.Contains(login.Body.String(), initialCookie.Value) || strings.Contains(login.Body.String(), "refresh_token") {
		t.Fatal("login response leaked the refresh bearer")
	}

	refreshRequest := func(cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		req.AddCookie(cookie)
		return serveTestRequest(srv, req)
	}
	firstRefresh := refreshRequest(initialCookie)
	if firstRefresh.Code != http.StatusOK {
		t.Fatalf("refresh: got %d body=%s", firstRefresh.Code, firstRefresh.Body.String())
	}
	rotatedCookies := firstRefresh.Result().Cookies()
	if len(rotatedCookies) != 1 || rotatedCookies[0].Value == "" || rotatedCookies[0].Value == initialCookie.Value {
		t.Fatalf("rotation did not replace refresh cookie: %#v", rotatedCookies)
	}
	rotatedCookie := rotatedCookies[0]
	if strings.Contains(firstRefresh.Body.String(), rotatedCookie.Value) || strings.Contains(firstRefresh.Body.String(), "refresh_token") {
		t.Fatal("refresh response leaked the refresh bearer")
	}
	retryRefresh := refreshRequest(initialCookie)
	if retryRefresh.Code != http.StatusOK {
		t.Fatalf("lost-response retry: got %d body=%s", retryRefresh.Code, retryRefresh.Body.String())
	}
	retryCookies := retryRefresh.Result().Cookies()
	if len(retryCookies) != 1 || retryCookies[0].Value != rotatedCookie.Value {
		t.Fatal("lost-response retry did not recover the same successor cookie")
	}

	var refreshedBody struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(firstRefresh.Body.Bytes(), &refreshedBody); err != nil {
		t.Fatal(err)
	}
	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+refreshedBody.Data.Token)
	if me := serveTestRequest(srv, meReq); me.Code != http.StatusOK {
		t.Fatalf("fresh access session: got %d body=%s", me.Code, me.Body.String())
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutReq.AddCookie(rotatedCookie)
	logout := serveTestRequest(srv, logoutReq)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout: got %d body=%s", logout.Code, logout.Body.String())
	}
	cleared := logout.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Fatalf("logout did not clear refresh cookie: %#v", cleared)
	}
	meReq = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+refreshedBody.Data.Token)
	if me := serveTestRequest(srv, meReq); me.Code != http.StatusUnauthorized {
		t.Fatalf("access token remained valid after logout: got %d", me.Code)
	}
	invalidRefresh := refreshRequest(rotatedCookie)
	if invalidRefresh.Code != http.StatusUnauthorized {
		t.Fatalf("refresh remained valid after logout: got %d body=%s", invalidRefresh.Code, invalidRefresh.Body.String())
	}
}

func TestDisasterRecoveryExecuteRequiresStepUpToken(t *testing.T) {
	enableTestSystemOperations(t)
	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, "snapshots.json")
	if err := os.WriteFile(metadataPath, []byte(`[{"snapshot_id":"x","created_at":"2026-01-01T00:00:00Z","schema_version":1,"app_version":"test","environment":"production","db_size":1}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DISASTER_SNAPSHOT_METADATA_PATH", metadataPath)
	database := setupDB(t)
	defer database.Close()
	jwtSvc := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, jwtSvc, testUserID)
	srv := newTestServer(t, database, jwtSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/disaster-recovery/restore", bytes.NewBufferString(`{"snapshot_id":"x","confirm":true,"authorization_token":"not-a-token"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	authorizeTestSystemOperations(req)
	w := serveTestRequest(srv, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("restore with invalid step-up token: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDisasterRecoveryFailedRestoreConsumesStepUpToken(t *testing.T) {
	enableTestSystemOperations(t)
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
	jwtToken := issueTestAccessSession(t, database, jwtSvc, testUserID)
	srv := newTestServer(t, database, jwtSvc)

	authReq := httptest.NewRequest(http.MethodPost, "/api/v1/disaster-recovery/authorize", bytes.NewBufferString(`{"current_password":"Password123"}`))
	authReq.Header.Set("Authorization", "Bearer "+jwtToken)
	authReq.Header.Set("Content-Type", "application/json")
	authorizeTestSystemOperations(authReq)
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
	authorizeTestSystemOperations(restoreReq)
	restoreResp := serveTestRequest(srv, restoreReq)
	if restoreResp.Code != http.StatusInternalServerError {
		t.Fatalf("restore: got %d, want %d body=%s", restoreResp.Code, http.StatusInternalServerError, restoreResp.Body.String())
	}

	var status string
	if err := database.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE action = ?`, service.ActionDisasterRecovery).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("failed restore did not consume step-up token: status=%s", status)
	}
	if got := auditEventCount(t, database, testUserID, "disaster_recovery_started"); got != 1 {
		t.Fatalf("failed restore started audit count = %d, want 1", got)
	}
	if got := auditEventCount(t, database, testUserID, "disaster_recovery_executed"); got != 0 {
		t.Fatalf("failed restore wrote execution audit event: got %d", got)
	}

	retryReq := httptest.NewRequest(http.MethodPost, "/api/v1/disaster-recovery/restore", bytes.NewBufferString(`{"snapshot_id":"x","confirm":true,"authorization_token":"`+authBody.Data.Token+`"}`))
	retryReq.Header.Set("Authorization", "Bearer "+jwtToken)
	retryReq.Header.Set("Content-Type", "application/json")
	authorizeTestSystemOperations(retryReq)
	retryResp := serveTestRequest(srv, retryReq)
	if retryResp.Code != http.StatusConflict {
		t.Fatalf("retry with consumed token: got %d body=%s, want %d", retryResp.Code, retryResp.Body.String(), http.StatusConflict)
	}
	if got := auditEventCount(t, database, testUserID, "disaster_recovery_started"); got != 1 {
		t.Fatalf("retry wrote another started audit event: got %d", got)
	}
}

func TestCreateTransactionIdempotencyReplaysAndRejectsPayloadMismatch(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	jwtSvc := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, jwtSvc, testUserID)
	srv := newTestServer(t, database, jwtSvc)
	payload := `{"occurred_at":"2026-09-04 12:00:00","direction":"expense","source":"personal","mode":"life","category":"food","amount_cents":1234,"currency":"CNY"}`

	first := createTransactionRequestForTest(srv, token, "retry-key-1", payload)
	second := createTransactionRequestForTest(srv, token, "retry-key-1", payload)
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("idempotent creates: first=%d %s second=%d %s", first.Code, first.Body.String(), second.Code, second.Body.String())
	}
	firstID := createdTransactionID(t, first)
	if secondID := createdTransactionID(t, second); secondID != firstID {
		t.Fatalf("retry created a second transaction: first=%s second=%s", firstID, secondID)
	}
	if second.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatal("replayed response is not marked as such")
	}

	mismatchPayload := strings.Replace(payload, `"amount_cents":1234`, `"amount_cents":5678,"project_id":"rejected-project"`, 1)
	mismatch := createTransactionRequestForTest(srv, token, "retry-key-1", mismatchPayload)
	if mismatch.Code != http.StatusConflict {
		t.Fatalf("same key with different payload: got %d body=%s, want 409", mismatch.Code, mismatch.Body.String())
	}
	var transactionCount, keyCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM transactions WHERE user_id = ?`, testUserID).Scan(&transactionCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM idempotency_keys WHERE user_id = ?`, testUserID).Scan(&keyCount); err != nil {
		t.Fatal(err)
	}
	if transactionCount != 1 || keyCount != 1 {
		t.Fatalf("idempotency state: transactions=%d keys=%d, want 1/1", transactionCount, keyCount)
	}
	var rejectedProjectCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = 'rejected-project'`).Scan(&rejectedProjectCount); err != nil {
		t.Fatal(err)
	}
	if rejectedProjectCount != 0 {
		t.Fatalf("payload-mismatch replay left %d rejected project row(s)", rejectedProjectCount)
	}
}

func TestCreateTransactionIdempotencyIsSafeUnderConcurrency(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	database.SetMaxOpenConns(12)
	jwtSvc := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, jwtSvc, testUserID)
	srv := newTestServer(t, database, jwtSvc)
	payload := `{"occurred_at":"2026-09-04 12:00:00","direction":"expense","source":"personal","mode":"life","category":"food","amount_cents":1234,"currency":"CNY"}`

	const workers = 8
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			responses <- createTransactionRequestForTest(srv, token, "concurrent-retry-key", payload)
		}()
	}
	close(start)
	wg.Wait()
	close(responses)

	var expectedID string
	for response := range responses {
		if response.Code != http.StatusCreated {
			t.Fatalf("concurrent retry: got %d body=%s", response.Code, response.Body.String())
		}
		id := createdTransactionID(t, response)
		if expectedID == "" {
			expectedID = id
		} else if id != expectedID {
			t.Fatalf("concurrent retry returned different transactions: %s and %s", expectedID, id)
		}
	}
	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM transactions WHERE user_id = ?`, testUserID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("concurrent retry inserted %d transactions, want 1", count)
	}
}

func TestCreateTransactionIdempotencyKeyIsUserScoped(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	const secondUserID = "test-user-0000-0000-0000-000000000002"
	if _, err := database.Exec(`
		INSERT INTO users(id, email, username, name, password_hash, role, created_at, updated_at)
		VALUES (?, 'second@example.com', 'second-user', 'Second User', 'x', 'user', ?, ?)`,
		secondUserID, time.Now().Unix(), time.Now().Unix(),
	); err != nil {
		t.Fatal(err)
	}
	jwtSvc := auth.NewJWTService("test-secret")
	firstToken := issueTestAccessSession(t, database, jwtSvc, testUserID)
	secondToken := issueTestAccessSession(t, database, jwtSvc, secondUserID)
	srv := newTestServer(t, database, jwtSvc)
	firstPayload := `{"occurred_at":"2026-09-04","direction":"expense","source":"personal","mode":"life","category":"food","amount_cents":100,"currency":"CNY"}`
	secondPayload := strings.Replace(firstPayload, `"amount_cents":100`, `"amount_cents":200`, 1)
	first := createTransactionRequestForTest(srv, firstToken, "shared-client-key", firstPayload)
	second := createTransactionRequestForTest(srv, secondToken, "shared-client-key", secondPayload)
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("user-scoped creates: first=%d %s second=%d %s", first.Code, first.Body.String(), second.Code, second.Body.String())
	}
	if createdTransactionID(t, first) == createdTransactionID(t, second) {
		t.Fatal("different users unexpectedly shared an idempotent result")
	}
	var keyCount, distinctKeys int
	if err := database.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT id) FROM idempotency_keys`).Scan(&keyCount, &distinctKeys); err != nil {
		t.Fatal(err)
	}
	if keyCount != 2 || distinctKeys != 2 {
		t.Fatalf("scoped idempotency keys: count=%d distinct=%d, want 2/2", keyCount, distinctKeys)
	}
}

func TestCreateTransactionRejectsInvalidIdempotencyKeys(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	jwtSvc := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, jwtSvc, testUserID)
	srv := newTestServer(t, database, jwtSvc)
	payload := `{"occurred_at":"2026-09-04","direction":"expense","source":"personal","mode":"life","category":"food","amount_cents":100,"currency":"CNY"}`

	tests := []struct {
		name   string
		values []string
	}{
		{name: "empty", values: []string{""}},
		{name: "whitespace", values: []string{" bad"}},
		{name: "separator", values: []string{"bad/key"}},
		{name: "non ascii", values: []string{"你好"}},
		{name: "too long", values: []string{strings.Repeat("a", 129)}},
		{name: "duplicate", values: []string{"first", "second"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewBufferString(payload))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			for _, value := range tc.values {
				req.Header.Add("Idempotency-Key", value)
			}
			response := serveTestRequest(srv, req)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("got %d body=%s, want 400", response.Code, response.Body.String())
			}
		})
	}
	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM transactions WHERE user_id = ?`, testUserID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid keys created %d transactions", count)
	}
}

func TestTransactionCORSPreflightAllowsIdempotencyKey(t *testing.T) {
	t.Setenv("FINARCH_CORS_ALLOWED_ORIGINS", "https://app.example.com")
	database := setupDB(t)
	defer database.Close()
	srv := newTestServer(t, database, auth.NewJWTService("test-secret"))
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/transactions", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,idempotency-key")
	response := serveTestRequest(srv, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight: got %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(strings.ToLower(response.Header().Get("Access-Control-Allow-Headers")), "idempotency-key") {
		t.Fatalf("preflight did not allow Idempotency-Key: %q", response.Header().Get("Access-Control-Allow-Headers"))
	}
}

func createTransactionRequestForTest(srv *apiv1.Server, token, key, payload string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	return serveTestRequest(srv, req)
}

func createdTransactionID(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.ID == "" {
		t.Fatalf("response has no transaction id: %s", response.Body.String())
	}
	return body.Data.ID
}

func mustHashPassword(t *testing.T, password string) string {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
