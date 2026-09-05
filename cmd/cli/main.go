package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	"finarch/internal/infrastructure/db"
	"finarch/internal/infrastructure/email"
	"finarch/internal/infrastructure/ocr"
	sqliterepo "finarch/internal/infrastructure/repository"
	filestorage "finarch/internal/infrastructure/storage"
	"finarch/internal/interface/apiv1"
)

// main runs CLI demo for fund management system.
func main() {
	ctx := context.Background()

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := os.Args[1]
	// A physical restore must run before SQLite is opened. Opening a missing
	// target creates it, while litestream restore requires its output path not
	// to exist. It can also leave SQLite handles referring to the replaced file.
	if command == "restore" {
		if err := runRestoreFromR2(ctx, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}

	dsn := os.Getenv("FINARCH_DB")
	if dsn == "" {
		dsn = "finarch.db"
	}

	database, err := db.OpenSQLite(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	switch command {
	case "init":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		fmt.Println("migration done")
	case "seed":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		if err := runSeed(ctx, database); err != nil {
			log.Fatal(err)
		}
		fmt.Println("seed done")
	case "addtx":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		if err := runAddTx(ctx, database, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "match":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		if err := runMatch(ctx, database, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "reimburse":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		if err := runReimburse(ctx, database, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "balance":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		if err := runBalance(ctx, database); err != nil {
			log.Fatal(err)
		}
	case "list":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		if err := runList(ctx, database); err != nil {
			log.Fatal(err)
		}
	case "serve":
		if err := db.Migrate(ctx, database); err != nil {
			log.Fatal(err)
		}
		addr := "127.0.0.1:8080"
		if len(os.Args) >= 3 {
			addr = os.Args[2]
		}
		jwtSecret, err := cliJWTSecret(addr, os.Getenv("JWT_SECRET"))
		if err != nil {
			log.Fatal(err)
		}
		if os.Getenv("JWT_SECRET") == "" {
			log.Print("[WARN] JWT_SECRET is unset; using an ephemeral secret for this loopback-only CLI server")
		}
		jwtSvc := auth.NewJWTService(jwtSecret)
		deletionTokenSvc := auth.NewActionTokenService(jwtSecret)
		// Auth brute-force protection
		authLimiter := auth.NewIPRateLimiter(10, 60*time.Second)
		loginTracker := auth.NewLoginAttemptTracker(5, 15*time.Minute)
		// Cloudflare Turnstile CAPTCHA (set TURNSTILE_SECRET to enable)
		captchaVerifier := auth.NewTurnstileVerifier(os.Getenv("TURNSTILE_SECRET"))
		turnstileSiteKey := os.Getenv("TURNSTILE_SITE_KEY")
		userRepo := sqliterepo.NewSQLiteUserRepository(database)
		tagRepo := sqliterepo.NewSQLiteTagRepository(database)
		txSvc, reimSvc, matchSvc, txRepo, acctSvc := buildServicesWithRepo(database)
		// Email service (no-op in dev unless RESEND_API_KEY is set)
		appBaseURL := os.Getenv("APP_BASE_URL")
		if appBaseURL == "" {
			appBaseURL = "http://localhost:8080"
		}
		emailSvc := email.NewResendSender(os.Getenv("RESEND_API_KEY"), os.Getenv("RESEND_FROM_EMAIL"), appBaseURL)
		tm := sqliterepo.NewSQLiteTransactionManager(database)
		sessionSvc, err := service.NewSessionService(sqliterepo.NewSQLiteRefreshTokenRepository(database), jwtSvc, jwtSecret)
		if err != nil {
			log.Fatal(err)
		}
		authSvc := service.NewAuthService(userRepo, deletionTokenSvc, loginTracker, emailSvc, email.IsConfigured(), appBaseURL, tm, sessionSvc)
		statsSvc := service.NewStatsService(database)
		budgetSvc := service.NewBudgetService(sqliterepo.NewSQLiteBudgetRepository(database))
		recurringSvc := service.NewRecurringTransactionService(sqliterepo.NewSQLiteRecurringTransactionRepository(database), txRepo, txSvc, tm, sqliterepo.NewSQLiteAccountRepository(database))
		attachmentStorage, err := filestorage.NewLocalAttachmentStorage(cliAttachmentDir(dsn))
		if err != nil {
			log.Fatal(err)
		}
		attachmentSvc := service.NewAttachmentService(
			sqliterepo.NewSQLiteAttachmentRepository(database),
			txRepo,
			attachmentStorage,
			ocr.NoneProvider{},
			cliEnvPositiveInt64("FINARCH_ATTACHMENT_MAX_BYTES", service.DefaultAttachmentMaxBytes),
			tm,
		)
		attachmentSvc.ConfigureQuota(
			cliEnvPositiveInt64("FINARCH_ATTACHMENT_MAX_FILES_PER_USER", service.DefaultAttachmentMaxFilesPerUser),
			cliEnvPositiveInt64("FINARCH_ATTACHMENT_MAX_TOTAL_BYTES_PER_USER", service.DefaultAttachmentMaxTotalBytesPerUser),
		)
		attachmentSvc.ConfigureUploadRateLimit(
			cliEnvPositiveInt("FINARCH_ATTACHMENT_UPLOADS_PER_MINUTE", service.DefaultAttachmentUploadsPerMinute),
			service.DefaultAttachmentUploadWindow,
		)
		authSvc.SetUserDataCleaner(attachmentSvc)
		loopbackSession := isLoopbackListenAddr(addr)
		sessionOptions := apiv1.BrowserSessionOptions{
			Secure:          !loopbackSession,
			AllowOriginless: loopbackSession,
		}
		srv := apiv1.NewServer(addr, database, dsn, txRepo, tagRepo, tm, txSvc, reimSvc, matchSvc, authSvc, statsSvc, budgetSvc, recurringSvc, attachmentSvc, authLimiter, captchaVerifier, turnstileSiteKey, acctSvc, emailSvc, sessionOptions)
		deletionWorkerCtx, stopDeletionWorker := context.WithCancel(ctx)
		deletionWorkerDone := make(chan struct{})
		go func() {
			defer close(deletionWorkerDone)
			runCLIPeriodicWorker(deletionWorkerCtx, service.DefaultAttachmentDeletionInterval, func(taskCtx context.Context) {
				withCLIWriteLease(taskCtx, func(leasedCtx context.Context) {
					if err := attachmentSvc.DrainDeletionQueue(leasedCtx, service.DefaultAttachmentDeletionBatchSize); err != nil && leasedCtx.Err() == nil {
						log.Printf("[attachments] deletion queue drain failed: %v", err)
					}
				})
			})
		}()
		orphanWorkerCtx, stopOrphanWorker := context.WithCancel(ctx)
		orphanWorkerDone := make(chan struct{})
		orphanTTL := cliEnvPositiveDuration("FINARCH_ATTACHMENT_ORPHAN_TTL", service.DefaultAttachmentOrphanTTL)
		orphanCleanupInterval := cliEnvPositiveDuration("FINARCH_ATTACHMENT_ORPHAN_CLEANUP_INTERVAL", service.DefaultAttachmentOrphanCleanupInterval)
		orphanCleanupBatchSize := cliEnvPositiveInt("FINARCH_ATTACHMENT_ORPHAN_CLEANUP_BATCH_SIZE", service.DefaultAttachmentOrphanCleanupBatchSize)
		go func() {
			defer close(orphanWorkerDone)
			runCLIPeriodicWorker(orphanWorkerCtx, orphanCleanupInterval, func(taskCtx context.Context) {
				withCLIWriteLease(taskCtx, func(leasedCtx context.Context) {
					cleaned, err := attachmentSvc.CleanupExpiredOrphans(leasedCtx, time.Now().UTC().Add(-orphanTTL), orphanCleanupBatchSize)
					if err != nil {
						if leasedCtx.Err() == nil {
							log.Printf("[attachments] orphan cleanup failed: %v", err)
						}
					} else if cleaned > 0 {
						log.Printf("[attachments] queued %d expired orphan(s) for deletion", cleaned)
					}
				})
			})
		}()
		sessionWorkerCtx, stopSessionWorker := context.WithCancel(ctx)
		sessionWorkerDone := make(chan struct{})
		go func() {
			defer close(sessionWorkerDone)
			runCLIPeriodicWorker(sessionWorkerCtx, service.DefaultSessionCleanupInterval, func(taskCtx context.Context) {
				withCLIWriteLease(taskCtx, func(leasedCtx context.Context) {
					if err := sessionSvc.DeleteExpired(leasedCtx); err != nil && leasedCtx.Err() == nil {
						log.Printf("[auth] session cleanup failed: %v", err)
					}
				})
			})
		}()
		log.Printf("FinArch API v1: http://%s", addr)
		runErr := srv.Run()
		stopDeletionWorker()
		stopOrphanWorker()
		stopSessionWorker()
		<-deletionWorkerDone
		<-orphanWorkerDone
		<-sessionWorkerDone
		if runErr != nil {
			log.Fatal(runErr)
		}
	default:
		printUsage()
	}
}

func withCLIWriteLease(ctx context.Context, run func(context.Context)) {
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

func runCLIPeriodicWorker(ctx context.Context, interval time.Duration, run func(context.Context)) {
	if interval <= 0 {
		return
	}
	if ctx.Err() == nil {
		run(ctx)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run(ctx)
		}
	}
}

func cliEnvPositiveInt(name string, fallback int) int {
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

// cliAttachmentDir keeps local CLI serving and physical restore validation on
// the same attachment tree. Production uses its explicit /data/attachments
// configuration and does not rely on this CLI-only default.
func cliAttachmentDir(databasePath string) string {
	if configured := strings.TrimSpace(os.Getenv("FINARCH_ATTACHMENTS_DIR")); configured != "" {
		return configured
	}

	// Keep the implicit attachment tree beside the database. This preserves the
	// convenient local layout (finarch.db + ./attachments) while a restore of
	// /data/finarch.db validates /data/attachments, matching the production
	// volume layout without requiring a cwd-dependent override.
	databasePath = strings.TrimSpace(databasePath)
	if strings.HasPrefix(databasePath, "file:") {
		if parsed, err := url.Parse(databasePath); err == nil && parsed.Scheme == "file" {
			if parsed.Path != "" {
				databasePath = parsed.Path
			} else if parsed.Opaque != "" {
				databasePath = parsed.Opaque
			}
		}
	}
	if databasePath == "" || databasePath == ":memory:" {
		return "attachments"
	}
	return filepath.Join(filepath.Dir(databasePath), "attachments")
}

func cliEnvPositiveInt64(name string, fallback int64) int64 {
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

func cliEnvPositiveDuration(name string, fallback time.Duration) time.Duration {
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

func cliJWTSecret(addr, configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if !isLoopbackListenAddr(addr) {
		return "", fmt.Errorf("CLI serve supports loopback listeners only; use cmd/server behind HTTPS for remote access")
	}
	if configured != "" {
		if len(configured) < 32 {
			return "", fmt.Errorf("JWT_SECRET must be at least 32 characters")
		}
		return configured, nil
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate ephemeral JWT secret: %w", err)
	}
	return hex.EncodeToString(random), nil
}

func isLoopbackListenAddr(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// runSeed inserts one project and demo transactions.

func runRestoreFromR2(ctx context.Context, args []string) (returnErr error) {
	fromR2 := false
	target := strings.TrimSpace(os.Getenv("FINARCH_DB"))
	if target == "" {
		target = "finarch.db"
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--from-r2":
			fromR2 = true
		case a == "--target":
			if i+1 >= len(args) {
				return fmt.Errorf("restore requires a value after --target")
			}
			i++
			target = strings.TrimSpace(args[i])
		case strings.HasPrefix(a, "--target="):
			target = strings.TrimSpace(strings.TrimPrefix(a, "--target="))
		default:
			return fmt.Errorf("unknown restore argument %q", a)
		}
	}
	if !fromR2 {
		return fmt.Errorf("restore currently supports only --from-r2")
	}
	if os.Getenv("ALLOW_R2_RESTORE") != "true" {
		return fmt.Errorf("restore blocked: set ALLOW_R2_RESTORE=true to confirm environment")
	}
	for _, k := range []string{"LITESTREAM_ACCESS_KEY_ID", "LITESTREAM_SECRET_ACCESS_KEY", "LITESTREAM_BUCKET", "LITESTREAM_ENDPOINT"} {
		if os.Getenv(k) == "" {
			return fmt.Errorf("missing required env %s", k)
		}
	}
	if target == "" {
		return fmt.Errorf("restore target must not be empty")
	}
	if target == ":memory:" || strings.HasPrefix(strings.ToLower(target), "file:") {
		return fmt.Errorf("restore target must be a filesystem path, not a SQLite DSN")
	}
	target, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve restore target: %w", err)
	}
	targetDir := filepath.Dir(target)
	dirInfo, err := os.Stat(targetDir)
	if err != nil {
		return fmt.Errorf("inspect restore target directory: %w", err)
	}
	if !dirInfo.IsDir() {
		return fmt.Errorf("restore target parent is not a directory: %s", targetDir)
	}

	log.Printf(`{"event":"restore_start","source":"cli_r2","target":%q}`, target)
	defer func() {
		if returnErr != nil {
			log.Printf(`{"event":"restore_failed","source":"cli_r2","target":%q,"error":%q}`, target, returnErr.Error())
		}
	}()

	if err := ensureRestoreTargetQuiescent(target); err != nil {
		return err
	}
	safety, targetExisted, err := createRestoreSafetyBackup(target)
	if err != nil {
		return fmt.Errorf("create pre-restore safety backup: %w", err)
	}
	if targetExisted {
		log.Printf(`{"event":"restore_safety_backup","path":%q}`, safety)
	}

	restoredPath, err := newAbsentRestorePath(targetDir)
	if err != nil {
		return fmt.Errorf("reserve temporary restore path: %w", err)
	}
	defer cleanupRestoreTemp(restoredPath)

	cmd := exec.CommandContext(ctx, "litestream", "restore", "-config", "/etc/litestream.yml", restoredPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("litestream restore: %w", err)
	}
	if err := validateRestoredSQLite(ctx, restoredPath); err != nil {
		return fmt.Errorf("validate restored database: %w", err)
	}
	if err := prepareRestoredSQLiteForActivation(ctx, restoredPath); err != nil {
		return fmt.Errorf("prepare restored database: %w", err)
	}
	if err := validateRestoredSQLite(ctx, restoredPath); err != nil {
		return fmt.Errorf("validate prepared database: %w", err)
	}
	if err := validateRestoredAttachmentFiles(ctx, restoredPath, cliAttachmentDir(target)); err != nil {
		return fmt.Errorf("validate restored attachments: %w", err)
	}
	if err := os.Chmod(restoredPath, 0o600); err != nil {
		return fmt.Errorf("secure restored database permissions: %w", err)
	}
	if err := syncRestoreFile(restoredPath); err != nil {
		return fmt.Errorf("sync restored database: %w", err)
	}
	if err := ensureRestoreTargetQuiescent(target); err != nil {
		return err
	}
	if err := os.Rename(restoredPath, target); err != nil {
		return fmt.Errorf("atomically replace restore target: %w", err)
	}
	if err := syncRestoreDirectory(targetDir); err != nil {
		// The restored file itself was synced before the atomic rename. Directory
		// sync is not supported by every filesystem, and the rename cannot be
		// safely undone here, so retain the safety copy and surface a warning.
		log.Printf(`{"event":"restore_directory_sync_warning","directory":%q,"error":%q}`, targetDir, err.Error())
	}
	log.Printf(`{"event":"restore_success","source":"cli_r2","target":%q,"safety_backup":%q}`, target, safety)
	return nil
}

func prepareRestoredSQLiteForActivation(ctx context.Context, path string) (returnErr error) {
	database, err := sql.Open("sqlite3", path)
	if err != nil {
		return err
	}
	database.SetMaxOpenConns(1)
	defer func() {
		if database != nil {
			returnErr = errors.Join(returnErr, database.Close())
		}
	}()

	for _, pragma := range []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA journal_mode = DELETE`,
		`PRAGMA synchronous = FULL`,
		`PRAGMA busy_timeout = 5000`,
	} {
		if _, err := database.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("configure restored database: %w", err)
		}
	}
	if err := db.Migrate(ctx, database); err != nil {
		return fmt.Errorf("migrate restored database: %w", err)
	}
	if err := db.InvalidateRestoredAuthenticationState(ctx, database); err != nil {
		return err
	}
	if err := database.Close(); err != nil {
		database = nil
		return err
	}
	database = nil
	return nil
}

func ensureRestoreTargetQuiescent(target string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		sidecar := target + suffix
		if _, err := os.Lstat(sidecar); err == nil {
			return fmt.Errorf("restore target has SQLite sidecar %s; stop all database and Litestream processes and checkpoint the database first", sidecar)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect SQLite sidecar %s: %w", sidecar, err)
		}
	}
	return nil
}

func createRestoreSafetyBackup(target string) (safetyPath string, exists bool, returnErr error) {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect current database: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", true, fmt.Errorf("current database is not a regular file")
	}

	source, err := os.Open(target)
	if err != nil {
		return "", true, fmt.Errorf("open current database: %w", err)
	}
	var destination *os.File
	keep := false
	defer func() {
		var cleanupErr error
		if destination != nil {
			cleanupErr = errors.Join(cleanupErr, destination.Close())
		}
		if source != nil {
			cleanupErr = errors.Join(cleanupErr, source.Close())
		}
		if !keep && safetyPath != "" {
			if err := os.Remove(safetyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove incomplete safety backup: %w", err))
			}
		}
		returnErr = errors.Join(returnErr, cleanupErr)
	}()

	pattern := filepath.Base(target) + ".pre-restore." + time.Now().UTC().Format("20060102_150405") + "-*"
	destination, err = os.CreateTemp(filepath.Dir(target), pattern)
	if err != nil {
		return "", true, fmt.Errorf("create safety backup file: %w", err)
	}
	safetyPath = destination.Name()
	if err := destination.Chmod(0o600); err != nil {
		return safetyPath, true, fmt.Errorf("secure safety backup permissions: %w", err)
	}

	copied, err := io.CopyBuffer(destination, source, make([]byte, 1024*1024))
	if err != nil {
		return safetyPath, true, fmt.Errorf("stream current database to safety backup: %w", err)
	}
	if copied != info.Size() {
		return safetyPath, true, fmt.Errorf("current database changed during safety backup: copied %d bytes, expected %d", copied, info.Size())
	}
	afterCopy, err := source.Stat()
	if err != nil {
		return safetyPath, true, fmt.Errorf("reinspect current database: %w", err)
	}
	if afterCopy.Size() != info.Size() || !afterCopy.ModTime().Equal(info.ModTime()) {
		return safetyPath, true, fmt.Errorf("current database changed during safety backup")
	}
	if err := destination.Sync(); err != nil {
		return safetyPath, true, fmt.Errorf("fsync safety backup: %w", err)
	}
	if err := destination.Close(); err != nil {
		destination = nil
		return safetyPath, true, fmt.Errorf("close safety backup: %w", err)
	}
	destination = nil
	if err := source.Close(); err != nil {
		source = nil
		return safetyPath, true, fmt.Errorf("close current database: %w", err)
	}
	source = nil
	if err := syncRestoreDirectory(filepath.Dir(target)); err != nil {
		return safetyPath, true, fmt.Errorf("fsync safety backup directory: %w", err)
	}
	keep = true
	return safetyPath, true, nil
}

func newAbsentRestorePath(directory string) (string, error) {
	placeholder, err := os.CreateTemp(directory, ".finarch-r2-restore-*.db")
	if err != nil {
		return "", err
	}
	path := placeholder.Name()
	closeErr := placeholder.Close()
	removeErr := os.Remove(path)
	if closeErr != nil || removeErr != nil {
		return "", errors.Join(closeErr, removeErr)
	}
	return path, nil
}

func cleanupRestoreTemp(path string) {
	for _, candidate := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		if err := os.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf(`{"event":"restore_temp_cleanup_failed","path":%q,"error":%q}`, candidate, err.Error())
		}
	}
}

func validateRestoredSQLite(ctx context.Context, path string) (returnErr error) {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("restored SQLite database is not a non-empty regular file")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	uri := url.URL{Scheme: "file", Path: absolutePath}
	query := uri.Query()
	query.Set("mode", "ro")
	query.Set("immutable", "1")
	query.Set("_query_only", "1")
	uri.RawQuery = query.Encode()
	database, err := sql.Open("sqlite3", uri.String())
	if err != nil {
		return err
	}
	database.SetMaxOpenConns(1)
	defer func() {
		returnErr = errors.Join(returnErr, database.Close())
	}()
	if err := database.PingContext(ctx); err != nil {
		return err
	}

	rows, err := database.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("run integrity_check: %w", err)
	}
	sawIntegrityResult := false
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read integrity_check: %w", err)
		}
		sawIntegrityResult = true
		if !strings.EqualFold(strings.TrimSpace(result), "ok") {
			_ = rows.Close()
			return fmt.Errorf("integrity_check=%s", result)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read integrity_check: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close integrity_check: %w", err)
	}
	if !sawIntegrityResult {
		return fmt.Errorf("integrity_check returned no result")
	}

	requiredSchema := []struct {
		name  string
		query string
	}{
		{name: "schema_migrations", query: `SELECT version, applied_at FROM schema_migrations LIMIT 0`},
		{name: "users", query: `SELECT id FROM users LIMIT 0`},
		{name: "transactions", query: `SELECT id, user_id, account_id, amount_cents FROM transactions LIMIT 0`},
		{name: "accounts", query: `SELECT id, user_id, balance_cents FROM accounts LIMIT 0`},
		{name: "categories", query: `SELECT id, user_id, type FROM categories LIMIT 0`},
	}
	for _, required := range requiredSchema {
		schemaRows, err := database.QueryContext(ctx, required.query)
		if err != nil {
			return fmt.Errorf("required schema %s is missing or incompatible: %w", required.name, err)
		}
		if err := schemaRows.Close(); err != nil {
			return fmt.Errorf("close schema check %s: %w", required.name, err)
		}
	}
	var appliedMigrations int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&appliedMigrations); err != nil {
		return fmt.Errorf("read schema migration history: %w", err)
	}
	if appliedMigrations == 0 {
		return fmt.Errorf("schema migration history is empty")
	}

	foreignKeyRows, err := database.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("run foreign_key_check: %w", err)
	}
	if foreignKeyRows.Next() {
		var table, parent string
		var rowID any
		var foreignKeyID int
		scanErr := foreignKeyRows.Scan(&table, &rowID, &parent, &foreignKeyID)
		_ = foreignKeyRows.Close()
		if scanErr != nil {
			return fmt.Errorf("read foreign_key_check: %w", scanErr)
		}
		return fmt.Errorf("foreign_key_check failed for table %s row %v", table, rowID)
	}
	if err := foreignKeyRows.Err(); err != nil {
		_ = foreignKeyRows.Close()
		return fmt.Errorf("read foreign_key_check: %w", err)
	}
	if err := foreignKeyRows.Close(); err != nil {
		return fmt.Errorf("close foreign_key_check: %w", err)
	}
	return nil
}

// validateRestoredAttachmentFiles prevents a database-only Litestream snapshot
// from being reported as a complete restore. Litestream replicates finarch.db,
// not the attachment directory, so every referenced object must already have
// been restored independently and must match the immutable database metadata
// before the live database is replaced.
func validateRestoredAttachmentFiles(ctx context.Context, databasePath, attachmentRoot string) (returnErr error) {
	absolutePath, err := filepath.Abs(databasePath)
	if err != nil {
		return err
	}
	uri := url.URL{Scheme: "file", Path: absolutePath}
	query := uri.Query()
	query.Set("mode", "ro")
	query.Set("immutable", "1")
	query.Set("_query_only", "1")
	uri.RawQuery = query.Encode()
	database, err := sql.Open("sqlite3", uri.String())
	if err != nil {
		return err
	}
	database.SetMaxOpenConns(1)
	defer func() { returnErr = errors.Join(returnErr, database.Close()) }()

	var tableCount int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM sqlite_master
		WHERE type = 'table' AND name = 'attachments'
	`).Scan(&tableCount); err != nil {
		return fmt.Errorf("inspect attachment metadata: %w", err)
	}
	if tableCount == 0 {
		return nil
	}

	rows, err := database.QueryContext(ctx, `
		SELECT storage_key, size_bytes, sha256
		FROM attachments
		ORDER BY storage_key ASC
	`)
	if err != nil {
		return fmt.Errorf("read attachment metadata: %w", err)
	}
	type attachmentMetadata struct {
		storageKey string
		sizeBytes  int64
		sha256     string
	}
	attachments := make([]attachmentMetadata, 0)
	for rows.Next() {
		var attachment attachmentMetadata
		if err := rows.Scan(&attachment.storageKey, &attachment.sizeBytes, &attachment.sha256); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read attachment metadata: %w", err)
		}
		attachments = append(attachments, attachment)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read attachment metadata: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close attachment metadata: %w", err)
	}
	if len(attachments) == 0 {
		return nil
	}

	storage, err := filestorage.NewLocalAttachmentStorage(attachmentRoot)
	if err != nil {
		return fmt.Errorf("open attachment storage: %w", err)
	}
	for _, attachment := range attachments {
		expectedSHA := strings.TrimSpace(attachment.sha256)
		decodedSHA, decodeErr := hex.DecodeString(expectedSHA)
		if decodeErr != nil || len(decodedSHA) != sha256.Size {
			return fmt.Errorf("attachment %q has invalid sha256 metadata", attachment.storageKey)
		}
		reader, err := storage.Open(ctx, attachment.storageKey)
		if err != nil {
			return fmt.Errorf("attachment %q is unavailable: %w", attachment.storageKey, err)
		}
		hasher := sha256.New()
		written, copyErr := io.Copy(hasher, reader)
		closeErr := reader.Close()
		if copyErr != nil || closeErr != nil {
			return fmt.Errorf("read attachment %q: %w", attachment.storageKey, errors.Join(copyErr, closeErr))
		}
		if written != attachment.sizeBytes {
			return fmt.Errorf("attachment %q size does not match database metadata", attachment.storageKey)
		}
		if !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), expectedSHA) {
			return fmt.Errorf("attachment %q checksum does not match database metadata", attachment.storageKey)
		}
	}
	return nil
}

func syncRestoreFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(syncErr, closeErr)
}

func syncRestoreDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if errors.Is(syncErr, syscall.EINVAL) || errors.Is(syncErr, syscall.ENOTSUP) {
		syncErr = nil
	}
	return errors.Join(syncErr, closeErr)
}

func runSeed(ctx context.Context, database *sql.DB) error {
	txSvc, _, _, projectRepo := buildServices(database)
	return seedData(ctx, projectRepo, txSvc)
}

// runAddTx creates one transaction from CLI args.
func runAddTx(ctx context.Context, database *sql.DB, args []string) error {
	if len(args) < 5 {
		return fmt.Errorf("addtx args: occurredAtUnix direction source category amountYuan [projectId] [note]")
	}
	txSvc, _, _, _ := buildServices(database)

	occurredAt := time.Unix(int64(mustParseFloat64(args[0])), 0)
	direction := model.Direction(args[1])
	source := model.Source(args[2])
	category := args[3]
	amount := model.Money(mustParseFloat64(args[4]))

	var projectID *string
	if len(args) >= 6 && args[5] != "" {
		projectID = &args[5]
	}
	note := ""
	if len(args) >= 7 {
		note = strings.Join(args[6:], " ")
	}

	t, err := txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
		OccurredAt: occurredAt,
		Direction:  direction,
		Source:     source,
		Category:   category,
		AmountYuan: amount,
		Currency:   "CNY",
		ProjectID:  projectID,
		Note:       note,
	})
	if err != nil {
		return err
	}
	fmt.Printf("created tx: id=%s amount=%.2f\n", t.ID, t.AmountYuan)
	return nil
}

// runMatch executes reimbursement matching.
func runMatch(ctx context.Context, database *sql.DB, args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("match args: targetYuan toleranceYuan maxDepth limit [projectId]")
	}
	_, _, matchingSvc, _ := buildServices(database)

	target := model.Money(mustParseFloat64(args[0]))
	tolerance := model.Money(mustParseFloat64(args[1]))
	maxDepth, err := strconv.Atoi(args[2])
	if err != nil {
		return fmt.Errorf("invalid maxDepth: %w", err)
	}
	limit, err := strconv.Atoi(args[3])
	if err != nil {
		return fmt.Errorf("invalid limit: %w", err)
	}

	var projectID *string
	if len(args) >= 5 && args[4] != "" {
		projectID = &args[4]
	}

	// CLI is single-user; use empty string as the user scope.
	results, err := matchingSvc.Match(ctx, "", target, tolerance, maxDepth, projectID, limit)
	if err != nil {
		return err
	}
	for i, r := range results {
		fmt.Printf("%d) total=%.2f error=%.2f projectCount=%d itemCount=%d ids=%v\n", i+1, r.TotalYuan, r.AbsErrorYuan, r.ProjectCount, r.ItemCount, r.TransactionIDs)
	}
	if len(results) == 0 {
		fmt.Println("no match")
	}
	return nil
}

// runReimburse creates reimbursement for transaction IDs.
func runReimburse(ctx context.Context, database *sql.DB, args []string) error {
	userID, positional, err := parseReimburseArgs(args)
	if err != nil {
		return err
	}
	_, reimbursementSvc, _, _ := buildServices(database)

	applicant := positional[0]
	txIDs := splitCSV(positional[1])
	requestNo := ""
	if len(positional) >= 3 {
		requestNo = positional[2]
	}

	reim, err := reimbursementSvc.CreateReimbursement(ctx, service.CreateReimbursementRequest{
		UserID:         userID,
		Applicant:      applicant,
		TransactionIDs: txIDs,
		RequestNo:      requestNo,
	})
	if err != nil {
		return err
	}
	fmt.Printf("reimbursement created: id=%s requestNo=%s total=%.2f\n", reim.ID, reim.RequestNo, reim.TotalYuan)
	return nil
}

func parseReimburseArgs(args []string) (string, []string, error) {
	userID := ""
	positional := make([]string, 0, 3)
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--user-id":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("reimburse requires a value after --user-id")
			}
			i++
			userID = strings.TrimSpace(args[i])
		case strings.HasPrefix(args[i], "--user-id="):
			userID = strings.TrimSpace(strings.TrimPrefix(args[i], "--user-id="))
		default:
			positional = append(positional, args[i])
		}
	}
	if userID == "" {
		return "", nil, fmt.Errorf("reimburse requires --user-id")
	}
	if len(positional) < 2 || len(positional) > 3 {
		return "", nil, fmt.Errorf("reimburse args: --user-id USER_ID applicant transactionIdsCsv [requestNo]")
	}
	return userID, positional, nil
}

// runBalance prints computed balances.
func runBalance(ctx context.Context, database *sql.DB) error {
	txSvc, _, _, _ := buildServices(database)
	// CLI is single-user; use empty string as the user scope.
	company, personal, err := txSvc.GetBalances(ctx, "")
	if err != nil {
		return err
	}
	fmt.Printf("company_balance_yuan=%.2f personal_outstanding_yuan=%.2f\n", company, personal)
	return nil
}

// runList prints all transactions to stdout.
func runList(ctx context.Context, database *sql.DB) error {
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	txs, err := txRepo.ListByUser(ctx, "", model.ModeWork)
	if err != nil {
		return err
	}
	if len(txs) == 0 {
		fmt.Println("no transactions")
		return nil
	}
	fmt.Printf("%-36s  %-10s  %-8s  %-8s  %-12s  %10s  %s\n",
		"ID", "日期", "方向", "来源", "分类", "金额(元)", "备注")
	for _, t := range txs {
		reimStatus := ""
		if t.Reimbursed {
			reimStatus = " [已报销]"
		}
		fmt.Printf("%-36s  %-10s  %-8s  %-8s  %-12s  %10.2f  %s%s\n",
			t.ID, t.OccurredAt.Format("2006-01-02"),
			string(t.Direction), string(t.Source),
			t.Category, t.AmountYuan.Float64(),
			t.Note, reimStatus)
	}
	return nil
}

func buildServicesWithRepo(database *sql.DB) (*service.TransactionService, *service.ReimbursementService, *service.MatchingService, *sqliterepo.SQLiteTransactionRepository, *service.AccountService) {
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	reimRepo := sqliterepo.NewSQLiteReimbursementRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)
	txSvc := service.NewTransactionService(txRepo, acctRepo, service.NewHTTPExchangeRateService())
	reimSvc := service.NewReimbursementService(tm, txRepo, reimRepo)
	matchSvc := service.NewMatchingService(txRepo)
	acctSvc := service.NewAccountService(acctRepo, txRepo, tm)
	return txSvc, reimSvc, matchSvc, txRepo, acctSvc
}

func printUsage() {
	fmt.Println("usage:")
	fmt.Println("  cli init")
	fmt.Println("  cli seed")
	fmt.Println("  cli list")
	fmt.Println("  cli addtx occurredAtUnix direction source category amountYuan [projectId] [note]")
	fmt.Println("  cli match targetYuan toleranceYuan maxDepth limit [projectId]")
	fmt.Println("  cli reimburse --user-id USER_ID applicant transactionIdsCsv [requestNo]")
	fmt.Println("  cli balance")
	fmt.Println("  cli restore --from-r2 [--target=/data/finarch.db]")
	fmt.Println("  cli serve [addr]   (默认 127.0.0.1:8080，仅支持回环地址)")
}

func mustParseFloat64(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Fatalf("invalid float64 %q: %v", s, err)
	}
	return v
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			res = append(res, p)
		}
	}
	return res
}

func buildServices(database *sql.DB) (*service.TransactionService, *service.ReimbursementService, *service.MatchingService, *sqliterepo.SQLiteProjectRepository) {
	txRepo := sqliterepo.NewSQLiteTransactionRepository(database)
	reimRepo := sqliterepo.NewSQLiteReimbursementRepository(database)
	projectRepo := sqliterepo.NewSQLiteProjectRepository(database)
	acctRepo := sqliterepo.NewSQLiteAccountRepository(database)
	tm := sqliterepo.NewSQLiteTransactionManager(database)

	txSvc := service.NewTransactionService(txRepo, acctRepo, service.NewHTTPExchangeRateService())
	reimSvc := service.NewReimbursementService(tm, txRepo, reimRepo)
	matchSvc := service.NewMatchingService(txRepo)
	return txSvc, reimSvc, matchSvc, projectRepo
}

func seedData(ctx context.Context, projectRepo *sqliterepo.SQLiteProjectRepository, txSvc *service.TransactionService) error {
	projectID := "PJT-001"
	err := projectRepo.Create(ctx, model.Project{
		ID:        projectID,
		Name:      "科研项目A",
		Code:      "PJT-001",
		CreatedAt: time.Now(),
	})
	if err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		return err
	}

	seed := []service.CreateTransactionRequest{
		{OccurredAt: time.Now().AddDate(0, 0, -10), Direction: model.DirectionIncome, Source: model.SourceCompany, Category: "资金注入", AmountYuan: 5000},
		{OccurredAt: time.Now().AddDate(0, 0, -9), Direction: model.DirectionExpense, Source: model.SourceCompany, Category: "CNC", AmountYuan: 1200, ProjectID: &projectID},
		{OccurredAt: time.Now().AddDate(0, 0, -8), Direction: model.DirectionExpense, Source: model.SourcePersonal, Category: "钣金", AmountYuan: 800, ProjectID: &projectID},
		{OccurredAt: time.Now().AddDate(0, 0, -7), Direction: model.DirectionExpense, Source: model.SourcePersonal, Category: "3D打印", AmountYuan: 400, ProjectID: &projectID},
		{OccurredAt: time.Now().AddDate(0, 0, -6), Direction: model.DirectionExpense, Source: model.SourcePersonal, Category: "差旅", AmountYuan: 205, ProjectID: &projectID},
		{OccurredAt: time.Now().AddDate(0, 0, -5), Direction: model.DirectionExpense, Source: model.SourcePersonal, Category: "材料", AmountYuan: 595, ProjectID: &projectID},
	}

	for _, req := range seed {
		if _, err := txSvc.CreateTransaction(ctx, req); err != nil && !strings.Contains(err.Error(), "UNIQUE") {
			return err
		}
	}
	return nil
}
