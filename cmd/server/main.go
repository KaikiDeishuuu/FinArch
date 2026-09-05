// cmd/server/main.go is the production standalone HTTP server entry-point.
// It is designed to run inside Docker and reads all configuration from env vars.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	"finarch/internal/infrastructure/db"
	"finarch/internal/infrastructure/email"
	"finarch/internal/infrastructure/ocr"
	sqliterepo "finarch/internal/infrastructure/repository"
	filestorage "finarch/internal/infrastructure/storage"
	"finarch/internal/interface/apiv1"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	proxyConfig, err := apiv1.LoadProxyConfigFromEnv()
	if err != nil {
		return fmt.Errorf("proxy configuration: %w", err)
	}

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
		return errors.New("JWT_SECRET env var is required in production")
	}
	if len(jwtSecret) < 32 {
		return errors.New("JWT_SECRET must be at least 32 characters")
	}

	database, err := db.OpenSQLite(ctx, dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("close db: %v", err)
		}
	}()
	configureDBPool(database)

	if err := db.Migrate(ctx, database); err != nil {
		return fmt.Errorf("migrate: %w", err)
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
	recurringRepo := sqliterepo.NewSQLiteRecurringTransactionRepository(database)
	attachmentRepo := sqliterepo.NewSQLiteAttachmentRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	sessionSvc, err := service.NewSessionService(sqliterepo.NewSQLiteRefreshTokenRepository(database), jwtSvc, jwtSecret)
	if err != nil {
		return fmt.Errorf("session service: %w", err)
	}

	txSvc := service.NewTransactionService(txRepo, acctRepo, service.NewHTTPExchangeRateService())
	reimSvc := service.NewReimbursementService(tm, txRepo, reimRepo)
	matchSvc := service.NewMatchingService(txRepo)
	authSvc := service.NewAuthService(userRepo, deletionTokenSvc, loginTracker, emailSvc, email.IsConfigured(), appBaseURL, tm, sessionSvc)
	statsSvc := service.NewStatsService(database)
	budgetSvc := service.NewBudgetService(budgetRepo)
	recurringSvc := service.NewRecurringTransactionService(recurringRepo, txRepo, txSvc, tm, acctRepo)
	attachmentStorage, err := filestorage.NewLocalAttachmentStorage(os.Getenv("FINARCH_ATTACHMENTS_DIR"))
	if err != nil {
		return fmt.Errorf("attachment storage: %w", err)
	}
	attachmentSvc := service.NewAttachmentService(attachmentRepo, txRepo, attachmentStorage, buildOCRProvider(), int64(envInt("FINARCH_ATTACHMENT_MAX_BYTES", int(service.DefaultAttachmentMaxBytes))), tm)
	attachmentSvc.ConfigureQuota(
		int64(envPositiveInt("FINARCH_ATTACHMENT_MAX_FILES_PER_USER", int(service.DefaultAttachmentMaxFilesPerUser))),
		envPositiveInt64("FINARCH_ATTACHMENT_MAX_TOTAL_BYTES_PER_USER", service.DefaultAttachmentMaxTotalBytesPerUser),
	)
	attachmentSvc.ConfigureUploadRateLimit(
		envPositiveInt("FINARCH_ATTACHMENT_UPLOADS_PER_MINUTE", service.DefaultAttachmentUploadsPerMinute),
		service.DefaultAttachmentUploadWindow,
	)
	attachmentSvc.ConfigureOCRLimits(
		envInt("FINARCH_OCR_MAX_CONCURRENT_PER_USER", service.DefaultOCRMaxConcurrentPerUser),
		envInt("FINARCH_OCR_MAX_CONCURRENT_GLOBAL", service.DefaultOCRMaxConcurrentGlobal),
		envInt("FINARCH_OCR_MAX_RUNS_PER_HOUR", service.DefaultOCRMaxRunsPerHour),
	)
	authSvc.SetUserDataCleaner(attachmentSvc)
	acctSvc := service.NewAccountService(acctRepo, txRepo, tm)

	srv := apiv1.NewServer(addr, database, dsn, txRepo, tagRepo, tm, txSvc, reimSvc, matchSvc, authSvc, statsSvc, budgetSvc, recurringSvc, attachmentSvc, authLimiter, captchaVerifier, turnstileSiteKey, acctSvc, emailSvc)
	srv.ConfigureProxy(proxyConfig)
	workerCtx, stopWorkers := context.WithCancel(ctx)
	workers := &backgroundWorkers{}
	// This defer is registered after database.Close, so all workers have stopped
	// before the shared database handle is closed.
	defer func() {
		stopWorkers()
		workers.Wait()
	}()

	// Background goroutine: purge unverified accounts older than 24 hours.
	workers.Go(workerCtx, func(ctx context.Context) {
		runPeriodicWorker(ctx, time.Hour, time.Hour, withWriteLease(func(taskCtx context.Context) {
			n, err := authSvc.CleanupExpiredUnverified(taskCtx)
			if err != nil {
				if taskCtx.Err() == nil {
					log.Printf("[cleanup] failed to purge unverified users: %v", err)
				}
			} else if n > 0 {
				log.Printf("[cleanup] purged %d expired unverified user(s)", n)
			}
		}))
	})

	// Background goroutine: clean up stale device heartbeat entries every 5 minutes.
	workers.Go(workerCtx, func(ctx context.Context) {
		runPeriodicWorker(ctx, 5*time.Minute, 5*time.Minute, func(context.Context) {
			srv.CleanupStaleDevices()
		})
	})

	startRecurringScheduler(workerCtx, workers, recurringSvc)
	orphanTTL := envPositiveDuration("FINARCH_ATTACHMENT_ORPHAN_TTL", service.DefaultAttachmentOrphanTTL)
	orphanCleanupInterval := envPositiveDuration("FINARCH_ATTACHMENT_ORPHAN_CLEANUP_INTERVAL", service.DefaultAttachmentOrphanCleanupInterval)
	orphanCleanupBatchSize := envPositiveInt("FINARCH_ATTACHMENT_ORPHAN_CLEANUP_BATCH_SIZE", service.DefaultAttachmentOrphanCleanupBatchSize)
	workers.Go(workerCtx, func(ctx context.Context) {
		runPeriodicWorker(ctx, 0, orphanCleanupInterval, withWriteLease(func(taskCtx context.Context) {
			cleaned, err := attachmentSvc.CleanupExpiredOrphans(taskCtx, time.Now().UTC().Add(-orphanTTL), orphanCleanupBatchSize)
			if err != nil {
				if taskCtx.Err() == nil {
					log.Printf("[attachments] orphan cleanup failed: %v", err)
				}
			} else if cleaned > 0 {
				log.Printf("[attachments] queued %d expired orphan(s) for deletion", cleaned)
			}
		}))
	})
	workers.Go(workerCtx, func(ctx context.Context) {
		runPeriodicWorker(ctx, 0, service.DefaultAttachmentDeletionInterval, withWriteLease(func(taskCtx context.Context) {
			if err := attachmentSvc.DrainDeletionQueue(taskCtx, service.DefaultAttachmentDeletionBatchSize); err != nil && taskCtx.Err() == nil {
				log.Printf("[attachments] deletion queue drain failed: %v", err)
			}
		}))
	})
	workers.Go(workerCtx, func(ctx context.Context) {
		runPeriodicWorker(ctx, 0, service.DefaultSessionCleanupInterval, withWriteLease(func(taskCtx context.Context) {
			if err := sessionSvc.DeleteExpired(taskCtx); err != nil && taskCtx.Err() == nil {
				log.Printf("[auth] session cleanup failed: %v", err)
			}
		}))
	})

	httpConfig := loadHTTPServerConfig()
	httpServer := newProductionHTTPServer(addr, srv.Handler(), httpConfig)
	log.Printf("FinArch API server listening on %s", addr)
	return serveHTTPServer(workerCtx, httpServer, httpConfig.shutdownTimeout)
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

func envPositiveDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		log.Printf("invalid %s=%q, using %s", name, value, fallback)
		return fallback
	}
	return parsed
}

func envPositiveInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		log.Printf("invalid %s=%q, using %d", name, value, fallback)
		return fallback
	}
	return parsed
}

func envPositiveInt64(name string, fallback int64) int64 {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		log.Printf("invalid %s=%q, using %d", name, value, fallback)
		return fallback
	}
	return parsed
}

func envCSV(name string) []string {
	return strings.FieldsFunc(os.Getenv(name), func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	})
}

func buildOCRProvider() service.OCRProvider {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("FINARCH_OCR_PROVIDER")))
	if provider == "" || provider == "none" {
		return ocr.NoneProvider{}
	}
	timeout := envDuration("FINARCH_OCR_TIMEOUT", 45*time.Second)
	switch provider {
	case "paddle":
		return ocr.NewPaddleProvider(os.Getenv("FINARCH_OCR_URL"), os.Getenv("FINARCH_OCR_LANG"), timeout)
	case "paddle_aistudio", "paddleocr_aistudio", "aistudio":
		return ocr.NewPaddleAIStudioProvider(ocr.PaddleAIStudioConfig{
			JobURL:             os.Getenv("FINARCH_OCR_AISTUDIO_JOB_URL"),
			Token:              os.Getenv("FINARCH_OCR_AISTUDIO_TOKEN"),
			Model:              os.Getenv("FINARCH_OCR_AISTUDIO_MODEL"),
			OptionalPayload:    os.Getenv("FINARCH_OCR_AISTUDIO_OPTIONAL_PAYLOAD"),
			Timeout:            timeout,
			PollInterval:       envDuration("FINARCH_OCR_AISTUDIO_POLL_INTERVAL", ocr.DefaultPaddleAIStudioPollInterval),
			MaxResultBytes:     int64(envInt("FINARCH_OCR_AISTUDIO_MAX_RESULT_BYTES", int(ocr.DefaultPaddleAIStudioMaxResult))),
			AllowedResultHosts: envCSV("FINARCH_OCR_AISTUDIO_ALLOWED_RESULT_HOSTS"),
		})
	}
	log.Printf("unknown FINARCH_OCR_PROVIDER=%q, OCR disabled", provider)
	return ocr.NoneProvider{}
}

func startRecurringScheduler(ctx context.Context, workers *backgroundWorkers, recurringSvc *service.RecurringTransactionService) {
	if recurringSvc == nil || strings.EqualFold(os.Getenv("FINARCH_RECURRING_AUTORUN"), "false") {
		return
	}
	dryRun := strings.EqualFold(os.Getenv("FINARCH_RECURRING_DRY_RUN"), "true")
	interval := envPositiveDuration("FINARCH_RECURRING_INTERVAL", 15*time.Minute)
	run := func(taskCtx context.Context) {
		res, err := recurringSvc.GenerateDue(taskCtx, time.Now(), envInt("FINARCH_RECURRING_BATCH_LIMIT", 100), dryRun)
		if err != nil {
			if taskCtx.Err() == nil {
				log.Printf("[recurring] generation failed: %v", err)
			}
			return
		}
		if res.Generated > 0 || res.Skipped > 0 || res.Failed > 0 {
			log.Printf("[recurring] generated=%d skipped=%d failed=%d", res.Generated, res.Skipped, res.Failed)
		}
	}
	workers.Go(ctx, func(ctx context.Context) {
		runPeriodicWorker(ctx, 10*time.Second, interval, withWriteLease(run))
	})
}

func withWriteLease(run func(context.Context)) func(context.Context) {
	return func(ctx context.Context) {
		if ctx.Err() != nil {
			return
		}
		release, admitted := db.Global().TryBeginWrite()
		if !admitted {
			return
		}
		defer release()
		if ctx.Err() == nil {
			run(ctx)
		}
	}
}

const (
	defaultHTTPReadHeaderTimeout = 10 * time.Second
	// Body and response deadlines are deliberately generous because physical
	// restore/backup can transfer about 100 MiB and OCR may wait on a provider.
	defaultHTTPReadTimeout     = 30 * time.Minute
	defaultHTTPWriteTimeout    = 30 * time.Minute
	defaultHTTPIdleTimeout     = 2 * time.Minute
	defaultHTTPShutdownTimeout = 5 * time.Minute
	defaultHTTPMaxHeaderBytes  = 64 << 10
)

type httpServerConfig struct {
	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	shutdownTimeout   time.Duration
	maxHeaderBytes    int
}

func loadHTTPServerConfig() httpServerConfig {
	return httpServerConfig{
		readHeaderTimeout: envPositiveDuration("FINARCH_HTTP_READ_HEADER_TIMEOUT", defaultHTTPReadHeaderTimeout),
		readTimeout:       envPositiveDuration("FINARCH_HTTP_READ_TIMEOUT", defaultHTTPReadTimeout),
		writeTimeout:      envPositiveDuration("FINARCH_HTTP_WRITE_TIMEOUT", defaultHTTPWriteTimeout),
		idleTimeout:       envPositiveDuration("FINARCH_HTTP_IDLE_TIMEOUT", defaultHTTPIdleTimeout),
		shutdownTimeout:   envPositiveDuration("FINARCH_HTTP_SHUTDOWN_TIMEOUT", defaultHTTPShutdownTimeout),
		maxHeaderBytes:    envPositiveInt("FINARCH_HTTP_MAX_HEADER_BYTES", defaultHTTPMaxHeaderBytes),
	}
}

func newProductionHTTPServer(addr string, handler http.Handler, config httpServerConfig) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: config.readHeaderTimeout,
		ReadTimeout:       config.readTimeout,
		WriteTimeout:      config.writeTimeout,
		IdleTimeout:       config.idleTimeout,
		MaxHeaderBytes:    config.maxHeaderBytes,
	}
}

type managedHTTPServer interface {
	ListenAndServe() error
	Shutdown(context.Context) error
	Close() error
}

func serveHTTPServer(ctx context.Context, server managedHTTPServer, shutdownTimeout time.Duration) error {
	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErrCh:
		return normalizeHTTPServeError(err)
	case <-ctx.Done():
	}

	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultHTTPShutdownTimeout
	}
	// The signal has already cancelled ctx. WithoutCancel preserves any values
	// while ensuring graceful shutdown receives its own live timeout context.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	shutdownErr := server.Shutdown(shutdownCtx)
	cancel()
	if shutdownErr != nil {
		shutdownErr = errors.Join(
			fmt.Errorf("graceful HTTP shutdown: %w", shutdownErr),
			wrapError("force-close HTTP server", server.Close()),
		)
	}

	serveErr := normalizeHTTPServeError(<-serveErrCh)
	return errors.Join(shutdownErr, serveErr)
}

func normalizeHTTPServeError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("serve HTTP: %w", err)
}

func wrapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

type backgroundWorkers struct {
	wg sync.WaitGroup
}

func (w *backgroundWorkers) Go(ctx context.Context, worker func(context.Context)) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		worker(ctx)
	}()
}

func (w *backgroundWorkers) Wait() {
	w.wg.Wait()
}

func runPeriodicWorker(ctx context.Context, initialDelay, interval time.Duration, run func(context.Context)) {
	if ctx.Err() != nil {
		return
	}
	if initialDelay > 0 {
		timer := time.NewTimer(initialDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
	if ctx.Err() != nil {
		return
	}
	run(ctx)
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			run(ctx)
		}
	}
}
