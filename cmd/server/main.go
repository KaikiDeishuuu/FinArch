// cmd/server/main.go is the production standalone HTTP server entry-point.
// It is designed to run inside Docker and reads all configuration from env vars.
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	"finarch/internal/infrastructure/db"
	"finarch/internal/infrastructure/email"
	sqliterepo "finarch/internal/infrastructure/repository"
	"finarch/internal/interface/apiv1"
)

func main() {
	ctx := context.Background()

	dsn := os.Getenv("FINARCH_DB")
	if dsn == "" {
		dsn = "/data/finarch.db"
	}

	addr := os.Getenv("FINARCH_ADDR")
	if addr == "" {
		addr = "0.0.0.0:8080"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET env var is required in production")
	}
	if len(jwtSecret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 characters")
	}

	database, err := db.OpenSQLite(ctx, dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()
	configureDBPool(database)

	if err := db.Migrate(ctx, database); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	jwtSvc := auth.NewJWTService(jwtSecret)
	deletionTokenSvc := auth.NewActionTokenService(jwtSecret)

	// Auth brute-force protection:
	// - IP rate limiter: max 10 login/register requests per IP per minute
	// - Account lockout: lock 15 min after 5 consecutive failures
	authLimiter := auth.NewIPRateLimiter(10, 60*time.Second)
	loginTracker := auth.NewLoginAttemptTracker(5, 15*time.Minute)

	// Cloudflare Turnstile CAPTCHA (set TURNSTILE_SECRET to enable).
	captchaVerifier := auth.NewTurnstileVerifier(os.Getenv("TURNSTILE_SECRET"))
	turnstileSiteKey := os.Getenv("TURNSTILE_SITE_KEY")

	// Email service (Resend). Set RESEND_API_KEY to enable email verification.
	appBaseURL := os.Getenv("APP_BASE_URL")
	if appBaseURL == "" {
		appBaseURL = "https://farc.dev"
	}
	emailSvc := email.NewResendSender(
		os.Getenv("RESEND_API_KEY"),
		os.Getenv("RESEND_FROM_EMAIL"),
		appBaseURL,
	)

	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	reimRepo := sqliterepo.NewSQLiteReimbursementRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	userRepo := sqliterepo.NewSQLiteUserRepository(database)
	tagRepo := sqliterepo.NewSQLiteTagRepository(database)
	budgetRepo := sqliterepo.NewSQLiteBudgetRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)

	txSvc := service.NewTransactionService(txRepo, acctRepo, service.NewHTTPExchangeRateService())
	reimSvc := service.NewReimbursementService(tm, txRepo, reimRepo)
	matchSvc := service.NewMatchingService(txRepo)
	authSvc := service.NewAuthService(userRepo, jwtSvc, deletionTokenSvc, loginTracker, emailSvc, email.IsConfigured(), appBaseURL, tm)
	statsSvc := service.NewStatsService(database)
	budgetSvc := service.NewBudgetService(budgetRepo)
	acctSvc := service.NewAccountService(acctRepo, txRepo, tm)

	srv := apiv1.NewServer(addr, database, dsn, txRepo, tagRepo, tm, txSvc, reimSvc, matchSvc, authSvc, statsSvc, budgetSvc, jwtSvc, authLimiter, captchaVerifier, turnstileSiteKey, acctSvc, emailSvc)

	// Background goroutine: purge unverified accounts older than 24 hours.
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			n, err := authSvc.CleanupExpiredUnverified(context.Background())
			if err != nil {
				log.Printf("[cleanup] failed to purge unverified users: %v", err)
			} else if n > 0 {
				log.Printf("[cleanup] purged %d expired unverified user(s)", n)
			}
		}
	}()

	// Background goroutine: clean up stale device heartbeat entries every 5 minutes.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			srv.CleanupStaleDevices()
		}
	}()

	log.Printf("FinArch API server listening on %s", addr)
	log.Fatal(srv.Run())
}

func configureDBPool(database interface {
	SetMaxOpenConns(int)
	SetMaxIdleConns(int)
	SetConnMaxIdleTime(time.Duration)
	SetConnMaxLifetime(time.Duration)
}) {
	maxOpen := envInt("FINARCH_DB_MAX_OPEN_CONNS", 8)
	maxIdle := envInt("FINARCH_DB_MAX_IDLE_CONNS", 4)
	idleTime := envDuration("FINARCH_DB_CONN_MAX_IDLE_TIME", 5*time.Minute)
	lifeTime := envDuration("FINARCH_DB_CONN_MAX_LIFETIME", 30*time.Minute)

	database.SetMaxOpenConns(maxOpen)
	database.SetMaxIdleConns(maxIdle)
	database.SetConnMaxIdleTime(idleTime)
	database.SetConnMaxLifetime(lifeTime)
	log.Printf("SQLite pool configured: max_open=%d max_idle=%d idle_time=%s lifetime=%s", maxOpen, maxIdle, idleTime, lifeTime)
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		log.Printf("invalid %s=%q, using %d", name, value, fallback)
		return fallback
	}
	return parsed
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < 0 {
		log.Printf("invalid %s=%q, using %s", name, value, fallback)
		return fallback
	}
	return parsed
}
