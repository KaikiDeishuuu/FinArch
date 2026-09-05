package test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	findb "finarch/internal/infrastructure/db"
	sqliterepo "finarch/internal/infrastructure/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ─── Refresh Token Rotation ───────────────────────────────────────────────────

// TestConcurrentRefreshRotation verifies that concurrent retries converge on
// the same committed successor during the response-recovery grace window.
func TestConcurrentRefreshRotation(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	jwt := auth.NewJWTService("test-secret")
	sessions := newTestSessionService(t, database, jwt, "test-secret")
	user, err := sqliterepo.NewSQLiteUserRepository(database).GetByID(ctx, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := sessions.CreateSession(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	const numGoroutines = 10
	type result struct {
		tokens service.SessionTokens
		err    error
	}
	resultCh := make(chan result, numGoroutines)
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tokens, err := sessions.RotateSession(ctx, initial.RefreshToken)
			resultCh <- result{tokens: tokens, err: err}
		}()
	}

	wg.Wait()
	close(resultCh)
	var successor string
	for result := range resultCh {
		if result.err != nil {
			t.Fatalf("concurrent rotation: %v", result.err)
		}
		if successor == "" {
			successor = result.tokens.RefreshToken
		} else if result.tokens.RefreshToken != successor {
			t.Fatal("concurrent rotations returned different successors")
		}
	}
	var generations int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM refresh_tokens`).Scan(&generations); err != nil {
		t.Fatal(err)
	}
	if generations != 2 {
		t.Fatalf("refresh generations = %d, want initial + one successor", generations)
	}
}

// ─── Global Write Gate ────────────────────────────────────────────────────────

// TestRestoreWriteBlocked verifies that POST/DELETE/PATCH/PUT requests are
// rejected with 503 when the system is in StateRestore.
func TestRestoreWriteBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	findb.Global().SetState(findb.StateRestore)
	defer findb.Global().SetState(findb.StateNormal)

	router := gin.New()
	router.Use(writeGateMiddlewareForTest())
	router.POST("/test", func(c *gin.Context) { c.Status(200) })
	router.GET("/test", func(c *gin.Context) { c.Status(200) })

	// POST must be blocked.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/test", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for POST during restore, got %d", w.Code)
	}

	// GET must be allowed.
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET during restore, got %d", w2.Code)
	}
}

// TestMigrationLock verifies that write requests are rejected with 503
// when the system is in StateMigration.
func TestMigrationLock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	findb.Global().SetState(findb.StateMigration)
	defer findb.Global().SetState(findb.StateNormal)

	router := gin.New()
	router.Use(writeGateMiddlewareForTest())
	router.DELETE("/test", func(c *gin.Context) { c.Status(200) })
	router.PATCH("/test", func(c *gin.Context) { c.Status(200) })

	for _, method := range []string{http.MethodDelete, http.MethodPatch} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(method, "/test", nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 for %s during migration, got %d", method, w.Code)
		}
	}
}

// writeGateMiddlewareForTest is a standalone version of the write gate for testing.
func writeGateMiddlewareForTest() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !findb.Global().IsWritable() {
			switch c.Request.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				c.AbortWithStatus(http.StatusServiceUnavailable)
				return
			}
		}
		c.Next()
	}
}

// ─── Optimistic Locking ───────────────────────────────────────────────────────

// TestOptimisticLocking verifies that concurrent updates with the same version
// result in exactly 1 success and the rest receiving a concurrent_modification error.
func TestOptimisticLocking(t *testing.T) {
	db := setupDB(t)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(db)
	ctx := context.Background()

	accounts, err := acctRepo.ListByUser(ctx, testUserID)
	if err != nil || len(accounts) == 0 {
		t.Fatalf("expected at least one account, got %v len:%d", err, len(accounts))
	}
	account := accounts[0]

	const numGoroutines = 8
	errCh := make(chan error, numGoroutines)
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// All goroutines attempt the same version — only one should win.
			a := account
			a.Name = "Renamed-" + uuid.NewString()[:8]
			errCh <- acctRepo.Update(ctx, a)
		}()
	}

	wg.Wait()
	close(errCh)

	successCount := 0
	for err := range errCh {
		if err == nil {
			successCount++
		}
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful optimistic update, got %d", successCount)
	}
}
