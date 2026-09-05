package apiv1

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"mime/multipart"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	findb "finarch/internal/infrastructure/db"
	"finarch/internal/infrastructure/email"
	sqliterepo "finarch/internal/infrastructure/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	sqlite3 "github.com/mattn/go-sqlite3"
)

// Server is the Gin-based API v1 server.
type Server struct {
	engine                       *gin.Engine
	addr                         string
	db                           *sql.DB
	dbPath                       string
	authSvc                      *service.AuthService
	txSvc                        *service.TransactionService
	reimSvc                      *service.ReimbursementService
	matchSvc                     *service.MatchingService
	statsSvc                     *service.StatsService
	budgetSvc                    *service.BudgetService
	recurringSvc                 *service.RecurringTransactionService
	attachmentSvc                *service.AttachmentService
	txRepo                       repository.TransactionRepository
	tagRepo                      repository.TagRepository
	txManager                    repository.TransactionManager
	authLimiter                  *auth.IPRateLimiter
	resetPasswordLimiter         *auth.IPRateLimiter
	refreshIPLimiter             *auth.IPRateLimiter
	refreshTokenLimiter          *auth.IPRateLimiter
	logoutIPLimiter              *auth.IPRateLimiter
	logoutTokenLimiter           *auth.IPRateLimiter
	captchaVerifier              *auth.TurnstileVerifier
	turnstileSiteKey             string
	acctSvc                      *service.AccountService
	emailSvc                     email.Sender
	pendingRestores              sync.Map // restoreID → *pendingRestore
	pendingRestoreMu             sync.Mutex
	activeDevices                sync.Map // "userID:deviceID" → *deviceSession
	disasterRecoveryMu           sync.Mutex
	operationsEnabled            bool
	operationsKeyHash            [sha256.Size]byte
	trustedProxyNets             []netip.Prefix
	corsAllowedOrigins           map[string]struct{}
	sessionCookieSecure          bool
	allowOriginlessSessionRoutes bool
	crossRestoreCommit           func(*sql.Tx) error
}

// BrowserSessionOptions controls only the HTTP cookie boundary. Production
// callers should use the secure defaults; the originless/non-Secure mode is
// reserved for the loopback-only CLI development server.
type BrowserSessionOptions struct {
	Secure          bool
	AllowOriginless bool
}

// deviceSession tracks a single device's last heartbeat.
type deviceSession struct {
	LastSeen  time.Time
	UserAgent string
}

// pendingRestore holds temporary state for a disaster recovery restore session.
type pendingRestore struct {
	mu            sync.Mutex
	cleanupOnce   sync.Once
	code          string    // 6-digit verification code
	tmpPath       string    // path to the uploaded .db tmp file
	attachmentDir string    // extracted attachment files for ZIP restores
	cleanupPath   string    // file or directory to remove when session is done
	expiresAt     time.Time // when this session expires
	email         string    // owner email extracted from backup
	name          string    // owner name extracted from backup
	ownerID       string
	requester     string
	attempts      int // wrong code attempts
	verified      bool
	token         string
	consumed      bool
}

const (
	operationsEnabledEnv                 = "FINARCH_ENABLE_SYSTEM_OPERATIONS"
	operationsSecretEnv                  = "FINARCH_SYSTEM_OPERATIONS_SECRET"
	operationsSecretHdr                  = "X-FinArch-Operations-Secret"
	backupExportTokenHdr                 = "X-FinArch-Export-Token"
	idempotencyHeader                    = "Idempotency-Key"
	trustedProxiesEnv                    = "FINARCH_TRUSTED_PROXY_CIDRS"
	behindProxyEnv                       = "FINARCH_BEHIND_PROXY"
	corsAllowedOriginsEnv                = "FINARCH_CORS_ALLOWED_ORIGINS"
	secureRefreshCookieName              = "__Host-finarch_refresh"
	loopbackRefreshCookieName            = "finarch_refresh"
	minimumOpsSecretBytes                = 32
	maxIdempotencyKeyBytes               = 128
	maxIdempotencyTransactionAttempts    = 12
	maxPendingRestores                   = 4
	createTransactionIdempotencyEndpoint = "POST /api/v1/transactions"
	refreshTokenEncodedBytes             = 43
	refreshTokenDecodedBytes             = 32
	jsonRequestBodyMaxBytes              = 1 << 20
)

var (
	errIdempotencyPayloadMismatch = errors.New("idempotency key was already used with a different request")
	errIdempotencyState           = errors.New("idempotency state is incomplete")
	errIdempotencyChanged         = errors.New("idempotency state changed during request")
)

type requestWriteLeaseContextKey struct{}

type createTransactionRequest struct {
	OccurredAt string `json:"occurred_at"`
	// V9 preferred fields
	AccountID    string  `json:"account_id"`
	Type         string  `json:"type"`
	AmountCents  int64   `json:"amount_cents"`
	ExchangeRate float64 `json:"exchange_rate"`
	// Backward-compat fields
	Direction  string  `json:"direction"`
	Source     string  `json:"source"`
	AmountYuan float64 `json:"amount_yuan"`
	// Common fields
	Mode      string   `json:"mode"`
	Category  string   `json:"category" binding:"required"`
	Currency  string   `json:"currency"`
	Note      string   `json:"note"`
	ProjectID *string  `json:"project_id"`
	TagIDs    []string `json:"tag_ids"`
}

func parseIdempotencyKey(r *http.Request) (string, bool, error) {
	values := r.Header.Values(idempotencyHeader)
	if len(values) == 0 {
		return "", false, nil
	}
	if len(values) != 1 {
		return "", true, fmt.Errorf("%s must be sent exactly once", idempotencyHeader)
	}
	key := values[0]
	if key == "" {
		return "", true, fmt.Errorf("%s must not be empty", idempotencyHeader)
	}
	if len(key) > maxIdempotencyKeyBytes {
		return "", true, fmt.Errorf("%s must not exceed %d bytes", idempotencyHeader, maxIdempotencyKeyBytes)
	}
	for i := 0; i < len(key); i++ {
		ch := key[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '.' || ch == '_' || ch == ':' || ch == '-' {
			continue
		}
		return "", true, fmt.Errorf("%s contains an invalid character", idempotencyHeader)
	}
	return key, true, nil
}

func scopedIdempotencyKey(userID, endpoint, rawKey string) string {
	digest := sha256.Sum256([]byte(endpoint + "\x00" + userID + "\x00" + rawKey))
	return "v1:" + hex.EncodeToString(digest[:])
}

func normalizeTagIDs(tagIDs []string) []string {
	normalized := append([]string(nil), tagIDs...)
	sort.Strings(normalized)
	unique := normalized[:0]
	for _, tagID := range normalized {
		if len(unique) == 0 || unique[len(unique)-1] != tagID {
			unique = append(unique, tagID)
		}
	}
	return unique
}

func retryIdempotentSQLiteTransaction(ctx context.Context, attempts int, operation func() error) error {
	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		if err = operation(); err == nil || !isSQLiteContention(err) {
			return err
		}
		if attempt == attempts-1 {
			break
		}
		exponent := attempt
		if exponent > 5 {
			exponent = 5
		}
		timer := time.NewTimer(time.Duration(1<<exponent) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

func isSQLiteContention(err error) bool {
	var sqliteErr sqlite3.Error
	return errors.As(err, &sqliteErr) && (sqliteErr.Code == sqlite3.ErrBusy || sqliteErr.Code == sqlite3.ErrLocked)
}

func createTransactionRequestHash(req createTransactionRequest, txType model.TxType, projectID *string) (string, error) {
	amountCents := req.AmountCents
	if amountCents == 0 && req.AmountYuan > 0 {
		var err error
		amountCents, err = model.Money(req.AmountYuan).Cents()
		if err != nil {
			return "", fmt.Errorf("invalid legacy transaction amount: %w", err)
		}
	}
	mode := model.Mode(req.Mode)
	if mode == "" {
		mode = model.ModeWork
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "CNY"
	}
	source := req.Source
	if req.AccountID != "" {
		// Source is ignored when the caller explicitly chooses an account.
		source = ""
	}
	project := ""
	if projectID != nil {
		project = *projectID
	}
	uniqueTagIDs := normalizeTagIDs(req.TagIDs)

	canonical, err := json.Marshal(struct {
		OccurredAt   string       `json:"occurred_at"`
		AccountID    string       `json:"account_id"`
		TxType       model.TxType `json:"type"`
		AmountCents  int64        `json:"amount_cents"`
		ExchangeRate float64      `json:"exchange_rate"`
		Source       string       `json:"source"`
		Mode         model.Mode   `json:"mode"`
		Category     string       `json:"category"`
		Currency     string       `json:"currency"`
		Note         string       `json:"note"`
		ProjectID    string       `json:"project_id"`
		TagIDs       []string     `json:"tag_ids"`
	}{
		OccurredAt: strings.TrimSpace(req.OccurredAt), AccountID: req.AccountID,
		TxType: txType, AmountCents: amountCents, ExchangeRate: req.ExchangeRate,
		Source: source, Mode: mode, Category: req.Category, Currency: currency,
		Note: req.Note, ProjectID: project, TagIDs: uniqueTagIDs,
	})
	if err != nil {
		return "", fmt.Errorf("encode idempotent transaction request: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func loadOperationsAccess() (bool, [sha256.Size]byte) {
	var empty [sha256.Size]byte
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(operationsEnabledEnv)), "true") {
		return false, empty
	}
	secret := strings.TrimSpace(os.Getenv(operationsSecretEnv))
	if len(secret) < minimumOpsSecretBytes {
		log.Printf("[WARN] %s=true ignored: %s must be at least %d characters", operationsEnabledEnv, operationsSecretEnv, minimumOpsSecretBytes)
		return false, empty
	}
	return true, sha256.Sum256([]byte(secret))
}

// ProxyConfig controls whether forwarding headers may influence the client IP.
// Its network list is deliberately private so callers can only construct a
// validated configuration through ParseProxyConfig.
type ProxyConfig struct {
	trustedProxyNets []netip.Prefix
}

// ParseProxyConfig validates the reverse-proxy trust boundary. Direct servers
// ignore forwarding headers. Proxy deployments must explicitly opt in and
// name the direct proxy and every controlled intermediary in the trusted XFF
// suffix; malformed or contradictory settings fail closed instead of silently
// weakening per-IP rate limits.
func ParseProxyConfig(behindProxyRaw, trustedProxyCIDRsRaw string) (ProxyConfig, error) {
	var config ProxyConfig
	behindProxyRaw = strings.TrimSpace(behindProxyRaw)
	behindProxy := false
	if behindProxyRaw != "" {
		if behindProxyRaw != "true" && behindProxyRaw != "false" {
			return config, fmt.Errorf("%s must be exactly true or false", behindProxyEnv)
		}
		behindProxy = behindProxyRaw == "true"
	}

	trustedProxyCIDRsRaw = strings.TrimSpace(trustedProxyCIDRsRaw)
	if !behindProxy {
		if trustedProxyCIDRsRaw != "" {
			return config, fmt.Errorf("%s requires %s=true", trustedProxiesEnv, behindProxyEnv)
		}
		return config, nil
	}
	if trustedProxyCIDRsRaw == "" {
		return config, fmt.Errorf("%s is required when %s=true", trustedProxiesEnv, behindProxyEnv)
	}

	networks, err := parseTrustedProxyCIDRs(trustedProxyCIDRsRaw)
	if err != nil {
		return config, err
	}
	if len(networks) == 0 {
		return config, fmt.Errorf("%s must contain at least one trusted proxy", trustedProxiesEnv)
	}
	config.trustedProxyNets = networks
	return config, nil
}

// LoadProxyConfigFromEnv reads and strictly validates production proxy config.
func LoadProxyConfigFromEnv() (ProxyConfig, error) {
	return ParseProxyConfig(os.Getenv(behindProxyEnv), os.Getenv(trustedProxiesEnv))
}

func parseTrustedProxyCIDRs(raw string) ([]netip.Prefix, error) {
	var networks []netip.Prefix
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("%s contains an empty entry", trustedProxiesEnv)
		}

		var network netip.Prefix
		if !strings.Contains(value, "/") {
			ip, err := netip.ParseAddr(value)
			if err != nil {
				return nil, fmt.Errorf("%s contains invalid IP address %q", trustedProxiesEnv, value)
			}
			ip = ip.WithZone("").Unmap()
			network = netip.PrefixFrom(ip, ip.BitLen())
		} else {
			parsed, err := netip.ParsePrefix(value)
			if err != nil {
				return nil, fmt.Errorf("%s contains invalid CIDR %q: %w", trustedProxiesEnv, value, err)
			}
			// Normalize a mapped IPv4 prefix such as ::ffff:10.0.0.0/120
			// to the equivalent IPv4 prefix. This also makes the mapped
			// IPv4 trust-all form ::ffff:0:0/96 subject to the /0 ban.
			if parsed.Addr().Is4In6() && parsed.Bits() >= 96 {
				network = netip.PrefixFrom(parsed.Addr().Unmap(), parsed.Bits()-96).Masked()
			} else {
				network = parsed.Masked()
			}
		}
		if network.Bits() == 0 {
			return nil, fmt.Errorf("%s must not contain an all-address /0 network %q", trustedProxiesEnv, value)
		}
		networks = append(networks, network)
	}
	return networks, nil
}

func canonicalCORSOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" || strings.Contains(hostname, "%") {
		return "", false
	}
	port := parsed.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	host := hostname
	if strings.Contains(hostname, ":") {
		if port == "" {
			host = "[" + hostname + "]"
		} else {
			host = net.JoinHostPort(hostname, port)
		}
	} else if port != "" {
		host = net.JoinHostPort(hostname, port)
	}
	return scheme + "://" + host, true
}

func parseCORSAllowedOrigins(raw string) map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, value := range strings.Split(raw, ",") {
		if origin, ok := canonicalCORSOrigin(value); ok {
			allowed[origin] = struct{}{}
		}
	}
	return allowed
}

type restoreContext struct {
	RequesterUserID      string
	BackupUserID         string
	RestoreMode          string
	DataScope            string
	AttachmentRestoreDir string
}

type attachmentBackupManifest struct {
	Version     int                    `json:"version"`
	GeneratedAt string                 `json:"generated_at"`
	Files       []attachmentBackupFile `json:"files"`
}

type attachmentBackupFile struct {
	StorageKey string `json:"storage_key"`
	SizeBytes  int64  `json:"size_bytes"`
	SHA256     string `json:"sha256"`
}

type attachmentRestoreFile struct {
	SourceKey string
	TargetKey string
	SizeBytes int64
	SHA256    string
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func normalizeRestoreScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "":
		return "both"
	case "both":
		return "both"
	case "work":
		return string(model.ModeWork)
	case "life":
		return string(model.ModeLife)
	default:
		return ""
	}
}

func scopeAllowsMode(scope, mode string) bool {
	normalizedScope := normalizeRestoreScope(scope)
	if normalizedScope == "both" {
		return true
	}
	return normalizedScope == strings.ToLower(strings.TrimSpace(mode))
}

func primaryUserID(ctx context.Context, db *sql.DB) (string, error) {
	var uid string
	err := db.QueryRowContext(ctx, `SELECT id FROM users WHERE deleted_at IS NULL ORDER BY CASE WHEN role='admin' THEN 0 ELSE 1 END, CASE WHEN email_verified=1 THEN 0 ELSE 1 END, created_at ASC LIMIT 1`).Scan(&uid)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(uid), nil
}

type backupIdentity struct {
	MetadataUserID     string
	MetadataUserEmail  string
	MetadataSchema     int
	MetadataCreatedAt  string
	MetadataAppVersion string
	MetadataPresent    bool
	FallbackOwnerID    string
	FallbackOwnerEmail string
	FallbackOwnerName  string
}

func readBackupIdentity(srcDB *sql.DB) (backupIdentity, error) {
	identity := backupIdentity{}

	var tableCount int
	if err := srcDB.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='backup_metadata'`).Scan(&tableCount); err != nil {
		return identity, err
	}
	if tableCount > 0 {
		row := srcDB.QueryRow(`SELECT user_id, user_email, COALESCE(schema_version, 0), COALESCE(created_at, ''), COALESCE(app_version, '') FROM backup_metadata ORDER BY created_at DESC LIMIT 1`)
		var uid, email, createdAt, appVersion string
		var schemaVersion int
		if err := row.Scan(&uid, &email, &schemaVersion, &createdAt, &appVersion); err == nil {
			identity.MetadataPresent = true
			identity.MetadataUserID = strings.TrimSpace(uid)
			identity.MetadataUserEmail = strings.TrimSpace(email)
			identity.MetadataSchema = schemaVersion
			identity.MetadataCreatedAt = createdAt
			identity.MetadataAppVersion = appVersion
		} else if !errors.Is(err, sql.ErrNoRows) {
			return identity, err
		}
	}

	row := srcDB.QueryRow(`SELECT id, email, COALESCE(nickname, username, '') FROM users WHERE deleted_at IS NULL ORDER BY CASE WHEN role='admin' THEN 0 ELSE 1 END, CASE WHEN email_verified=1 THEN 0 ELSE 1 END, created_at ASC LIMIT 1`)
	switch err := row.Scan(&identity.FallbackOwnerID, &identity.FallbackOwnerEmail, &identity.FallbackOwnerName); err {
	case nil:
		identity.FallbackOwnerID = strings.TrimSpace(identity.FallbackOwnerID)
		identity.FallbackOwnerEmail = strings.TrimSpace(identity.FallbackOwnerEmail)
	case sql.ErrNoRows:
		return identity, fmt.Errorf("备份文件中未找到用户数据")
	default:
		return identity, err
	}

	return identity, nil
}

func detectRestoreMode(identity backupIdentity, requestUserID, requestUserEmail string) (restoreMode, backupUserID, backupUserEmail string, sameID, sameEmail bool) {
	backupUserID = strings.TrimSpace(identity.MetadataUserID)
	backupUserEmail = strings.TrimSpace(identity.MetadataUserEmail)
	if backupUserID == "" {
		backupUserID = strings.TrimSpace(identity.FallbackOwnerID)
	}
	if backupUserEmail == "" {
		backupUserEmail = strings.TrimSpace(identity.FallbackOwnerEmail)
	}

	sameID = strings.TrimSpace(requestUserID) != "" && backupUserID != "" && strings.TrimSpace(requestUserID) == backupUserID
	sameEmail = normalizeEmail(requestUserEmail) != "" && normalizeEmail(backupUserEmail) != "" && normalizeEmail(requestUserEmail) == normalizeEmail(backupUserEmail)

	restoreMode = "CROSS_ACCOUNT"
	if sameID || sameEmail {
		restoreMode = "SAME_ACCOUNT"
	}

	return restoreMode, backupUserID, backupUserEmail, sameID, sameEmail
}

func NewServer(
	addr string,
	db *sql.DB,
	dbPath string,
	txRepo repository.TransactionRepository,
	tagRepo repository.TagRepository,
	txManager repository.TransactionManager,
	txSvc *service.TransactionService,
	reimSvc *service.ReimbursementService,
	matchSvc *service.MatchingService,
	authSvc *service.AuthService,
	statsSvc *service.StatsService,
	budgetSvc *service.BudgetService,
	recurringSvc *service.RecurringTransactionService,
	attachmentSvc *service.AttachmentService,
	authLimiter *auth.IPRateLimiter,
	captchaVerifier *auth.TurnstileVerifier,
	turnstileSiteKey string,
	acctSvc *service.AccountService,
	emailSvc email.Sender,
	browserSessionOptions ...BrowserSessionOptions,
) *Server {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	operationsEnabled, operationsKeyHash := loadOperationsAccess()
	sessionOptions := BrowserSessionOptions{Secure: true}
	if len(browserSessionOptions) > 0 {
		sessionOptions = browserSessionOptions[0]
	}
	if (!sessionOptions.Secure || sessionOptions.AllowOriginless) && !isLoopbackBindAddress(addr) {
		log.Printf("[WARN] insecure browser-session options ignored for non-loopback listen address %q", addr)
		sessionOptions = BrowserSessionOptions{Secure: true}
	}
	s := &Server{
		engine:                       gin.New(),
		addr:                         addr,
		db:                           db,
		dbPath:                       dbPath,
		authSvc:                      authSvc,
		txSvc:                        txSvc,
		reimSvc:                      reimSvc,
		matchSvc:                     matchSvc,
		statsSvc:                     statsSvc,
		budgetSvc:                    budgetSvc,
		recurringSvc:                 recurringSvc,
		attachmentSvc:                attachmentSvc,
		txRepo:                       txRepo,
		tagRepo:                      tagRepo,
		txManager:                    txManager,
		authLimiter:                  authLimiter,
		resetPasswordLimiter:         auth.NewIPRateLimiter(10, time.Minute),
		refreshIPLimiter:             auth.NewIPRateLimiter(1200, time.Minute),
		refreshTokenLimiter:          auth.NewIPRateLimiter(60, time.Minute),
		logoutIPLimiter:              auth.NewIPRateLimiter(300, time.Minute),
		logoutTokenLimiter:           auth.NewIPRateLimiter(30, time.Minute),
		captchaVerifier:              captchaVerifier,
		turnstileSiteKey:             turnstileSiteKey,
		acctSvc:                      acctSvc,
		emailSvc:                     emailSvc,
		operationsEnabled:            operationsEnabled,
		operationsKeyHash:            operationsKeyHash,
		corsAllowedOrigins:           parseCORSAllowedOrigins(os.Getenv(corsAllowedOriginsEnv)),
		sessionCookieSecure:          sessionOptions.Secure,
		allowOriginlessSessionRoutes: sessionOptions.AllowOriginless,
	}
	s.registerRoutes()
	return s
}

// ConfigureProxy installs a previously validated proxy trust boundary. Call it
// before serving requests. The zero value keeps forwarded headers untrusted.
func (s *Server) ConfigureProxy(config ProxyConfig) {
	s.trustedProxyNets = append([]netip.Prefix(nil), config.trustedProxyNets...)
}

func (s *Server) Run() error {
	return s.engine.Run(s.addr)
}

func (s *Server) Handler() http.Handler {
	return s.engine
}

func (s *Server) registerRoutes() {
	r := s.engine
	r.Use(gin.Recovery(), s.securityHeaders(), s.corsMiddleware(), s.jsonRequestBodyLimitMiddleware(), s.writeGateMiddleware())

	// ─── Public routes ───────────────────────────────────────────
	pub := r.Group("/api/v1")
	pub.GET("/health", s.handleHealth)
	pub.GET("/config", s.handleConfig)
	pub.POST("/auth/register", s.sessionNoStoreMiddleware(), s.sessionOriginMiddleware(), s.authRateLimitMiddleware(), s.handleRegister)
	pub.POST("/auth/login", s.sessionNoStoreMiddleware(), s.sessionOriginMiddleware(), s.authRateLimitMiddleware(), s.handleLogin)
	pub.POST("/auth/refresh", s.sessionNoStoreMiddleware(), s.sessionOriginMiddleware(), s.refreshRateLimitMiddleware(), s.handleRefreshToken)
	pub.POST("/auth/logout", s.sessionNoStoreMiddleware(), s.sessionOriginMiddleware(), s.logoutRateLimitMiddleware(), s.handleLogout)
	pub.POST("/auth/verify-email", s.sessionNoStoreMiddleware(), s.handleVerifyEmailJSON)
	pub.POST("/auth/resend-verification", s.authRateLimitMiddleware(), s.handleResendVerification)
	pub.POST("/auth/forgot-password", s.authRateLimitMiddleware(), s.handleForgotPassword)
	pub.POST("/auth/reset-password", s.sessionNoStoreMiddleware(), s.resetPasswordRateLimitMiddleware(), s.handleResetPassword)
	pub.POST("/auth/confirm-delete-account", s.sessionNoStoreMiddleware(), s.handleConfirmDeleteAccount)
	pub.POST("/auth/confirm-email-change-old", s.sessionNoStoreMiddleware(), s.handleConfirmOldEmailChange)
	pub.POST("/auth/confirm-email-change", s.sessionNoStoreMiddleware(), s.handleConfirmEmailChange)

	// ─── Protected routes (JWT required) ──────────────────────────
	api := r.Group("/api/v1", s.jwtMiddleware())

	// User
	api.GET("/auth/me", s.handleGetMe)
	api.POST("/auth/change-password", s.handleChangePassword)
	api.POST("/auth/request-delete-account", s.authRateLimitMiddleware(), s.handleRequestDeleteAccount)
	api.POST("/auth/request-email-change", s.authRateLimitMiddleware(), s.handleRequestEmailChange)
	api.PATCH("/auth/nickname", s.handleUpdateNickname)
	api.POST("/auth/heartbeat", s.handleHeartbeat)
	api.GET("/auth/devices/online", s.handleOnlineDevices)

	// Transactions
	api.GET("/transactions", s.handleListTransactions)
	api.POST("/transactions", s.handleCreateTransaction)
	api.PATCH("/transactions/:id/reimburse", s.handleToggleReimbursed)
	api.PATCH("/transactions/:id/upload", s.handleToggleUploaded)
	api.POST("/transactions/:id/tags", s.handleAddTag)
	api.DELETE("/transactions/:id/tags/:tagID", s.handleRemoveTag)

	// Accounts (V9)
	api.GET("/accounts", s.handleListAccounts)
	api.POST("/accounts", s.handleCreateAccount)
	api.PATCH("/accounts/:id", s.handleUpdateAccount)
	api.DELETE("/accounts/:id", s.handleDeleteAccount)

	// Categories (V9)
	api.GET("/categories", s.handleListCategories)
	api.POST("/categories", s.handleCreateCategory)

	// Tags
	api.GET("/tags", s.handleListTags)
	api.POST("/tags", s.handleCreateTag)
	api.DELETE("/tags/:id", s.handleDeleteTag)

	// Match
	api.POST("/match/subset-sum", s.handleMatch)

	// Reimbursements
	api.POST("/reimbursements", s.handleCreateReimbursement)

	// Stats
	api.GET("/stats/summary", s.handleStatsSummary)
	api.GET("/stats/monthly", s.handleStatsMonthly)
	api.GET("/stats/by-category", s.handleStatsByCategory)
	api.GET("/stats/by-project", s.handleStatsByProject)
	api.GET("/stats/account-balance-history", s.handleStatsAccountBalanceHistory)

	// Budgets
	api.GET("/budgets", s.handleListBudgets)
	api.GET("/budgets/summary", s.handleBudgetSummary)
	api.POST("/budgets", s.handleCreateBudget)
	api.PATCH("/budgets/:id", s.handleUpdateBudget)
	api.DELETE("/budgets/:id", s.handleDeleteBudget)

	// Recurring transactions
	api.GET("/recurring-rules", s.handleListRecurringRules)
	api.GET("/recurring-rules/preview", s.handlePreviewRecurringRules)
	api.POST("/recurring-rules", s.handleCreateRecurringRule)
	api.PATCH("/recurring-rules/:id", s.handleUpdateRecurringRule)
	api.PATCH("/recurring-rules/:id/status", s.handleUpdateRecurringRuleStatus)
	api.DELETE("/recurring-rules/:id", s.handleDeleteRecurringRule)
	api.GET("/recurring-rules/:id/instances", s.handleListRecurringInstances)
	api.POST("/recurring-rules/:id/generate-now", s.handleGenerateRecurringRuleNow)

	// Attachments and OCR
	api.POST("/attachments", s.handleUploadAttachment)
	api.GET("/attachments/:id", s.handleGetAttachment)
	api.GET("/attachments/:id/download", s.handleDownloadAttachment)
	api.DELETE("/attachments/:id", s.handleDeleteAttachment)
	api.POST("/attachments/:id/link", s.handleLinkAttachment)
	api.POST("/attachments/:id/ocr", s.handleRunAttachmentOCR)
	api.GET("/attachments/:id/ocr", s.handleGetAttachmentOCR)
	api.POST("/transactions/:id/attachments", s.handleUploadTransactionAttachment)
	api.GET("/transactions/:id/attachments", s.handleListTransactionAttachments)

	// Backup metadata is tenant-scoped. Physical database operations are disabled
	// by default and require a separate server-side operations secret when enabled.
	api.GET("/backup/info", s.handleBackupInfo)
	operations := api.Group("", s.operationsAccessMiddleware())
	operations.POST("/backup/export-request", s.sessionNoStoreMiddleware(), s.handleBackupExportRequest)
	operations.POST("/backup/download", s.handleBackupDownload)
	operations.GET("/backup/litestream-health", s.handleLitestreamHealth)
	operations.GET("/disaster-recovery/snapshots", s.handleDisasterRecoverySnapshots)
	operations.POST("/disaster-recovery/authorize", s.authRateLimitMiddleware(), s.handleDisasterRecoveryAuthorize)
	operations.POST("/disaster-recovery/restore", s.handleDisasterRecoveryExecute)
	operations.POST("/backup/restore", s.handleRestore)
	operations.POST("/backup/restore/send-verification", s.handleRestoreSendVerification)
	operations.POST("/backup/restore/verify", s.handleRestoreVerify)
	operations.POST("/backup/restore/execute", s.handleRestoreExecute)

	// ─── Frontend static files ────────────────────────────────────
	staticDir := os.Getenv("FINARCH_STATIC")
	if staticDir == "" {
		staticDir = "./frontend/dist"
	}
	absStaticDir, err := filepath.Abs(staticDir)
	if err != nil {
		log.Fatalf("failed to resolve FINARCH_STATIC path %q: %v", staticDir, err)
	}
	absStaticDir, err = filepath.EvalSymlinks(absStaticDir)
	if err != nil {
		log.Fatalf("failed to resolve symlinks for FINARCH_STATIC path %q: %v", absStaticDir, err)
	}
	r.Static("/assets", filepath.Join(absStaticDir, "assets"))
	// Serve any other static file that exists in dist root (favicon, etc.)
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		// API routes that truly don't exist → 404 JSON
		if strings.HasPrefix(path, "/api/") {
			fail(c, http.StatusNotFound, "not_found", "The requested resource was not found.")
			return
		}

		candidate, ok := safeStaticPath(absStaticDir, path)
		if !ok {
			fail(c, http.StatusNotFound, "not_found", "The requested resource was not found.")
			return
		}
		// Try to serve the file directly first (e.g. /favicon.svg)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			c.File(candidate)
			return
		}
		// SPA fallback: let React Router handle the path
		indexPath, ok := safeStaticPath(absStaticDir, "/index.html")
		if !ok {
			fail(c, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		c.File(indexPath)
	})
}

func safeStaticPath(baseDir, requestPath string) (string, bool) {
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return "", false
	}
	resolvedBaseDir, err := filepath.EvalSymlinks(absBaseDir)
	if err != nil {
		return "", false
	}

	cleanedPath := filepath.Clean(requestPath)
	relRequestPath := strings.TrimPrefix(cleanedPath, "/")
	joinedPath := filepath.Join(resolvedBaseDir, relRequestPath)
	absTargetPath, err := filepath.Abs(joinedPath)
	if err != nil {
		return "", false
	}

	resolvedTargetPath, err := filepath.EvalSymlinks(absTargetPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", false
		}
		resolvedParent, parentErr := filepath.EvalSymlinks(filepath.Dir(absTargetPath))
		if parentErr != nil {
			return "", false
		}
		resolvedTargetPath = filepath.Join(resolvedParent, filepath.Base(absTargetPath))
	}

	relToBase, err := filepath.Rel(resolvedBaseDir, resolvedTargetPath)
	if err != nil {
		return "", false
	}
	if filepath.IsAbs(relToBase) || relToBase == ".." || strings.HasPrefix(relToBase, ".."+string(os.PathSeparator)) {
		return "", false
	}

	return resolvedTargetPath, true
}

// ─── Middleware ──────────────────────────────────────────────────────────────

func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		originHeader := strings.TrimSpace(c.GetHeader("Origin"))
		if originHeader == "" {
			c.Next()
			return
		}

		c.Writer.Header().Add("Vary", "Origin")
		origin, valid := canonicalCORSOrigin(originHeader)
		_, allowed := s.corsAllowedOrigins[origin]
		if !valid || !allowed {
			if c.Request.Method == http.MethodOptions {
				fail(c, http.StatusForbidden, "CORS_ORIGIN_FORBIDDEN", "Cross-origin request is not allowed.")
				return
			}
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Origin", originHeader)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")
		// Authentication is bearer-only; omitting ACA-Credentials is equivalent
		// to false and prevents ambient cookie credentials from being sent.
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func isLoopbackBindAddress(addr string) bool {
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

func (s *Server) expectedSessionOrigin(c *gin.Context) (string, bool) {
	scheme := "http"
	if s.sessionCookieSecure {
		scheme = "https"
	}
	return canonicalCORSOrigin(scheme + "://" + c.Request.Host)
}

// sessionOriginMiddleware is an exact Origin/Host CSRF gate for endpoints that
// mint, rotate, or revoke ambient browser credentials. Production requests
// must carry Origin; only the explicitly loopback-bound development server may
// accept originless non-browser clients.
func (s *Server) sessionOriginMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		originValues := c.Request.Header.Values("Origin")
		fetchSite := strings.ToLower(strings.TrimSpace(c.GetHeader("Sec-Fetch-Site")))
		if fetchSite != "" && fetchSite != "same-origin" && fetchSite != "none" {
			fail(c, http.StatusForbidden, "session_origin_forbidden", "This session request must be same-origin.")
			return
		}
		if len(originValues) == 0 {
			if s.allowOriginlessSessionRoutes && !s.sessionCookieSecure {
				c.Next()
				return
			}
			fail(c, http.StatusForbidden, "session_origin_required", "A same-origin Origin header is required.")
			return
		}
		if len(originValues) != 1 {
			fail(c, http.StatusForbidden, "session_origin_forbidden", "This session request must be same-origin.")
			return
		}
		provided, validProvided := canonicalCORSOrigin(originValues[0])
		expected, validExpected := s.expectedSessionOrigin(c)
		if !validProvided || !validExpected || provided != expected {
			fail(c, http.StatusForbidden, "session_origin_forbidden", "This session request must be same-origin.")
			return
		}
		c.Next()
	}
}

func (s *Server) sessionNoStoreMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}
}

func (s *Server) jwtMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		headers := c.Request.Header.Values("Authorization")
		if len(headers) == 0 || strings.TrimSpace(headers[0]) == "" {
			fail(c, http.StatusUnauthorized, "access_missing", "An access token is required.")
			return
		}
		fields := strings.Fields(headers[0])
		if len(headers) != 1 || len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || fields[1] == "" {
			fail(c, http.StatusUnauthorized, "access_invalid", "The access token is invalid.")
			return
		}
		claims, err := s.authSvc.AuthenticateAccess(c.Request.Context(), fields[1])
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrAccessTokenExpired):
				fail(c, http.StatusUnauthorized, "access_expired", "The access token has expired.")
			case errors.Is(err, auth.ErrAccessTokenInvalid):
				fail(c, http.StatusUnauthorized, "access_invalid", "The access token is invalid.")
			case errors.Is(err, service.ErrSessionInvalid):
				fail(c, http.StatusUnauthorized, "session_invalid", "The session is invalid or expired.")
			default:
				log.Printf("[ERROR] authenticate access session: %v", err)
				fail(c, http.StatusServiceUnavailable, "system_unavailable", "System temporarily unavailable. Please try again later.")
			}
			return
		}
		c.Set("userID", claims.UserID)
		c.Set("userEmail", claims.Email)
		c.Set("userRole", claims.Role)
		c.Next()
	}
}

func userID(c *gin.Context) string { return c.GetString("userID") }

// parseForwardedIP accepts the address forms emitted by common HTTP proxies:
// a bare IP or an IP:port pair (bracketed for IPv6). IPv6 zones identify the
// local interface, not a distinct address for CIDR trust or rate-limit keys, so
// they are deliberately removed before the address is returned.
func parseForwardedIP(raw string) (netip.Addr, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return netip.Addr{}, false
	}

	addr, err := netip.ParseAddr(value)
	if err != nil {
		addrPort, portErr := netip.ParseAddrPort(value)
		if portErr != nil {
			return netip.Addr{}, false
		}
		addr = addrPort.Addr()
	}
	addr = addr.WithZone("").Unmap()
	return addr, true
}

func requestPeerIP(c *gin.Context) netip.Addr {
	peer, _ := parseForwardedIP(c.Request.RemoteAddr)
	return peer
}

func (s *Server) isTrustedProxy(ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	for _, network := range s.trustedProxyNets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// realIP ignores forwarding headers unless the direct peer is an explicitly
// configured trusted proxy. It walks X-Forwarded-For from right to left so a
// client-supplied leftmost value cannot bypass rate limits through an appending
// reverse proxy.
func (s *Server) realIP(c *gin.Context) string {
	peer := requestPeerIP(c)
	if !peer.IsValid() {
		// net/http normally supplies an IP:port peer. If a custom listener cannot
		// provide a parseable address, use one conservative shared bucket rather
		// than including a variable source port in the rate-limit key.
		return ""
	}
	if !s.isTrustedProxy(peer) {
		return peer.String()
	}

	xffHeaders := c.Request.Header.Values("X-Forwarded-For")
	if len(xffHeaders) > 0 {
		var chain []netip.Addr
		for _, header := range xffHeaders {
			parts := strings.Split(header, ",")
			for _, part := range parts {
				candidate, valid := parseForwardedIP(part)
				if !valid {
					// Never skip across an ambiguous hop: doing so could select an
					// attacker-controlled value farther to the left.
					return peer.String()
				}
				chain = append(chain, candidate)
			}
		}
		for i := len(chain) - 1; i >= 0; i-- {
			if !s.isTrustedProxy(chain[i]) {
				return chain[i].String()
			}
		}
		// A complete XFF chain made only of trusted proxies has no trustworthy
		// client boundary. Do not mix in a potentially contradictory header.
		return peer.String()
	}

	realIPHeaders := c.Request.Header.Values("X-Real-IP")
	if len(realIPHeaders) != 1 {
		return peer.String()
	}
	if candidate, valid := parseForwardedIP(realIPHeaders[0]); valid {
		return candidate.String()
	}
	return peer.String()
}

func (s *Server) operationsAccessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.operationsEnabled {
			fail(c, http.StatusForbidden, "SYSTEM_OPERATIONS_DISABLED", "System backup and restore operations are disabled.")
			return
		}
		provided := sha256.Sum256([]byte(c.GetHeader(operationsSecretHdr)))
		if subtle.ConstantTimeCompare(provided[:], s.operationsKeyHash[:]) != 1 {
			fail(c, http.StatusForbidden, "SYSTEM_OPERATIONS_FORBIDDEN", "A valid operations credential is required.")
			return
		}
		c.Next()
	}
}

// securityHeaders adds standard security headers to every response.
func (s *Server) securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		if isActionTokenPage(c.Request.URL.Path) {
			// Legacy links may still carry a token in the query string. Prevent
			// subresources or outbound navigation from receiving that URL before
			// the SPA replaces it with a clean history entry.
			c.Header("Referrer-Policy", "no-referrer")
			c.Header("Cache-Control", "no-store")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
		} else {
			c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		}
		c.Next()
	}
}

func isActionTokenPage(path string) bool {
	switch path {
	case "/verify-email",
		"/reset-password",
		"/confirm-delete-account",
		"/confirm-email-change-old",
		"/confirm-email-change":
		return true
	default:
		return false
	}
}

// authRateLimitMiddleware restricts auth endpoints by IP to prevent brute-force
// and credential-stuffing attacks.
func (s *Server) authRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := s.realIP(c)
		if !s.authLimiter.Allow(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    42901,
				"message": "请求过于频繁，请稍后再试",
			})
			return
		}
		c.Next()
	}
}

// resetPasswordRateLimitMiddleware keeps password-hashing work on its own IP
// budget. A flood of reset attempts therefore cannot consume the shared
// register/login allowance for users behind the same address.
func (s *Server) resetPasswordRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.resetPasswordLimiter != nil && !s.resetPasswordLimiter.Allow(s.realIP(c)) {
			fail(c, http.StatusTooManyRequests, "reset_password_rate_limited", "Too many password reset attempts. Please try again later.")
			return
		}
		c.Next()
	}
}

// sessionRateLimitMiddleware uses a high-capacity IP flood gate for all
// requests and a separate digest-keyed budget only for canonical opaque
// refresh bearers. Invalid cookies cannot consume a legitimate session
// family fine-grained allowance behind a shared proxy.
func (s *Server) sessionRateLimitMiddleware(ipLimiter, tokenLimiter *auth.IPRateLimiter, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawRefresh, validCookie := s.refreshTokenFromCookie(c)
		if ipLimiter != nil && !ipLimiter.Allow(s.realIP(c)) {
			fail(c, http.StatusTooManyRequests, "session_rate_limited", "Too many session "+action+" attempts. Please try again later.")
			return
		}
		if validCookie && tokenLimiter != nil {
			digest := sha256.Sum256([]byte(rawRefresh))
			if !tokenLimiter.Allow(hex.EncodeToString(digest[:])) {
				fail(c, http.StatusTooManyRequests, "session_rate_limited", "Too many session "+action+" attempts. Please try again later.")
				return
			}
		}
		c.Next()
	}
}

func (s *Server) refreshRateLimitMiddleware() gin.HandlerFunc {
	return s.sessionRateLimitMiddleware(s.refreshIPLimiter, s.refreshTokenLimiter, "refresh")
}

func (s *Server) logoutRateLimitMiddleware() gin.HandlerFunc {
	return s.sessionRateLimitMiddleware(s.logoutIPLimiter, s.logoutTokenLimiter, "logout")
}

type apiErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": data})
}

func fail(c *gin.Context, status int, code any, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"success": false, "error": apiErrorPayload{Code: fmt.Sprint(code), Message: msg}})
}

func failBind(c *gin.Context, _ ...int) {
	fail(c, http.StatusUnprocessableEntity, "invalid_request", "Please check your input and try again.")
}

func failInternal(c *gin.Context, err error) {
	log.Printf("[ERROR] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	fail(c, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
}

func failDomain(c *gin.Context, err error) {
	status, payload := mapDomainError(err)
	if status == http.StatusInternalServerError {
		log.Printf("[ERROR] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	}
	fail(c, status, payload.Code, payload.Message)
}

func mapDomainError(err error) (int, apiErrorPayload) {
	switch {
	case errors.Is(err, service.ErrUsernameTaken):
		return http.StatusConflict, apiErrorPayload{Code: "username_taken", Message: "This username is already in use."}
	case errors.Is(err, service.ErrEmailTaken):
		return http.StatusConflict, apiErrorPayload{Code: "email_taken", Message: "This email is already in use."}
	case errors.Is(err, service.ErrInvalidToken):
		return http.StatusBadRequest, apiErrorPayload{Code: "invalid_token", Message: "The token is invalid."}
	case errors.Is(err, service.ErrExpiredToken):
		return http.StatusBadRequest, apiErrorPayload{Code: "expired_token", Message: "The token has expired."}
	case errors.Is(err, service.ErrAlreadyUsed):
		return http.StatusConflict, apiErrorPayload{Code: "already_used", Message: "This action was already completed."}
	case errors.Is(err, service.ErrInvalidPassword):
		return http.StatusForbidden, apiErrorPayload{Code: "invalid_password", Message: "Incorrect password. Please try again."}
	case errors.Is(err, service.ErrInvalidCredentials):
		return http.StatusUnauthorized, apiErrorPayload{Code: "invalid_credentials", Message: "Invalid email or password."}
	case errors.Is(err, service.ErrAccountLocked):
		return http.StatusTooManyRequests, apiErrorPayload{Code: "account_locked", Message: "Too many failed login attempts. Please try again later."}
	case errors.Is(err, service.ErrLoginFailed):
		return http.StatusInternalServerError, apiErrorPayload{Code: "login_failed", Message: "Login failed. Please try again later."}
	case errors.Is(err, service.ErrNotAuthorized):
		return http.StatusForbidden, apiErrorPayload{Code: "not_authorized", Message: "You are not authorized to perform this action."}
	case errors.Is(err, service.ErrUserNotFound):
		return http.StatusNotFound, apiErrorPayload{Code: "user_not_found", Message: "User not found."}
	case errors.Is(err, service.ErrResourceConflict):
		return http.StatusConflict, apiErrorPayload{Code: "resource_conflict", Message: "The request conflicts with current data."}
	case errors.Is(err, service.ErrEmailNotVerified):
		return http.StatusForbidden, apiErrorPayload{Code: "email_not_verified", Message: "Please verify your email before logging in."}
	case errors.Is(err, service.ErrConcurrentModification):
		return http.StatusConflict, apiErrorPayload{Code: "concurrent_modification", Message: "The resource was modified by another request. Please refresh and try again."}
	case errors.Is(err, repository.ErrMultiCurrencyReportingUnavailable):
		return http.StatusUnprocessableEntity, apiErrorPayload{Code: "multi_currency_reporting_unavailable", Message: "This report cannot combine amounts with different base currencies."}
	case errors.Is(err, service.ErrInvalidOrUsedToken),
		errors.Is(err, service.ErrRefreshTokenReuse),
		errors.Is(err, service.ErrSessionInvalid):
		return http.StatusUnauthorized, apiErrorPayload{Code: "session_invalid", Message: "The session is invalid or expired."}
	case errors.Is(err, service.ErrSystemUnavailable):
		return http.StatusServiceUnavailable, apiErrorPayload{Code: "system_unavailable", Message: "System temporarily unavailable. Please try again later."}
	default:
		return http.StatusInternalServerError, apiErrorPayload{Code: "internal_error", Message: "Something went wrong. Please try again."}
	}
}

func mapAuthError(c *gin.Context, err error, _ int) {
	failDomain(c, err)
}

func requestUsesDedicatedBodyLimit(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.Method != http.MethodPost {
		return false
	}
	switch c.FullPath() {
	case "/api/v1/attachments",
		"/api/v1/transactions/:id/attachments",
		"/api/v1/backup/restore",
		"/api/v1/backup/restore/send-verification":
		return true
	default:
		return false
	}
}

// jsonRequestBodyLimitMiddleware caps ordinary API request bodies before any
// authentication, binding, or database work. Multipart upload endpoints keep
// their larger, endpoint-specific MaxBytesReader limits.
func (s *Server) jsonRequestBodyLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil || c.Request.Body == nil || c.Request.Body == http.NoBody || requestUsesDedicatedBodyLimit(c) {
			c.Next()
			return
		}
		if c.Request.ContentLength > jsonRequestBodyMaxBytes {
			fail(c, http.StatusRequestEntityTooLarge, "request_too_large", "The request body is too large.")
			return
		}
		limited := http.MaxBytesReader(c.Writer, c.Request.Body, jsonRequestBodyMaxBytes)
		body, err := io.ReadAll(limited)
		_ = limited.Close()
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				fail(c, http.StatusRequestEntityTooLarge, "request_too_large", "The request body is too large.")
				return
			}
			fail(c, http.StatusBadRequest, "invalid_request", "The request body could not be read.")
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		c.Next()
	}
}

func requestRequiresWriteLease(request *http.Request) bool {
	if request == nil {
		return false
	}
	switch request.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func requestExecutesMaintenance(request *http.Request) bool {
	if request == nil {
		return false
	}
	if request.Method != http.MethodPost {
		return false
	}
	switch request.URL.Path {
	case "/api/v1/backup/download",
		"/api/v1/disaster-recovery/restore",
		"/api/v1/backup/restore",
		"/api/v1/backup/restore/execute":
		return true
	default:
		return false
	}
}

func releaseRequestWriteLease(ctx context.Context) {
	if ctx == nil {
		return
	}
	if release, ok := ctx.Value(requestWriteLeaseContextKey{}).(func()); ok && release != nil {
		release()
	}
}

// writeGateMiddleware holds a shared write lease for every logical mutation.
// Backup and restore upgrade that lease to exclusive maintenance. They must
// acquire the serializer before the shared lease so two simultaneous upgrades
// cannot deadlock while each waits for the other's lease to drain.
func (s *Server) writeGateMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requestRequiresWriteLease(c.Request) {
			c.Next()
			return
		}
		// Restore requests must wait for the restore serializer before taking a
		// shared write lease. Taking these locks in the opposite order lets one
		// restore wait for maintenance while a second restore holds the shared
		// lease and waits for the serializer.
		if requestExecutesMaintenance(c.Request) {
			s.disasterRecoveryMu.Lock()
			defer s.disasterRecoveryMu.Unlock()
		}
		release, admitted := findb.Global().TryBeginWrite()
		if !admitted {
			fail(c, http.StatusServiceUnavailable, "system_unavailable", "System temporarily unavailable due to maintenance.")
			return
		}
		defer release()
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestWriteLeaseContextKey{}, release))
		c.Next()
	}
}

// ─── Config ──────────────────────────────────────────────────────────────────

func (s *Server) handleHealth(c *gin.Context) {
	if state := findb.Global().State(); state != findb.StateNormal {
		fail(c, http.StatusServiceUnavailable, "maintenance", "Service is not ready while database maintenance is active.")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		fail(c, http.StatusServiceUnavailable, "unhealthy", "Service unavailable.")
		return
	}
	var ready int
	if err := s.db.QueryRowContext(ctx, `SELECT 1`).Scan(&ready); err != nil {
		fail(c, http.StatusServiceUnavailable, "unhealthy", "Service unavailable.")
		return
	}
	ok(c, gin.H{"status": "ok"})
}

// handleConfig returns public runtime configuration consumed by the frontend.
func (s *Server) handleConfig(c *gin.Context) {
	ok(c, gin.H{
		"turnstile_site_key":          s.turnstileSiteKey,
		"captcha_enabled":             s.captchaVerifier.Enabled(),
		"email_verification_required": s.authSvc.EmailVerificationRequired(),
		"system_operations_enabled":   s.operationsEnabled,
	})
}

// ─── Auth ────────────────────────────────────────────────────────────────────

func (s *Server) refreshCookieName() string {
	if s.sessionCookieSecure {
		return secureRefreshCookieName
	}
	return loopbackRefreshCookieName
}

func (s *Server) setRefreshCookie(c *gin.Context, rawToken string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     s.refreshCookieName(),
		Value:    rawToken,
		Path:     "/",
		Expires:  expiresAt.UTC(),
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.sessionCookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}

func (s *Server) clearRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     s.refreshCookieName(),
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(1, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.sessionCookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}

func (s *Server) refreshTokenFromCookie(c *gin.Context) (string, bool) {
	name := s.refreshCookieName()
	var raw string
	count := 0
	validSyntax := true
	for _, header := range c.Request.Header.Values("Cookie") {
		for _, segment := range strings.Split(header, ";") {
			segment = strings.TrimLeft(segment, " \t")
			equals := strings.IndexByte(segment, '=')
			if equals < 0 {
				if strings.TrimSpace(segment) == name {
					count++
					validSyntax = false
				}
				continue
			}
			rawName := segment[:equals]
			if strings.TrimSpace(rawName) != name {
				continue
			}
			count++
			if rawName != name {
				validSyntax = false
				continue
			}
			raw = segment[equals+1:]
		}
	}
	if count != 1 || !validSyntax || !isCanonicalRefreshToken(raw) {
		return "", false
	}
	return raw, true
}

func isCanonicalRefreshToken(raw string) bool {
	if len(raw) != refreshTokenEncodedBytes {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || len(decoded) != refreshTokenDecodedBytes {
		return false
	}
	return base64.RawURLEncoding.EncodeToString(decoded) == raw
}

func authResponsePayload(resp service.LoginResponse) gin.H {
	return gin.H{
		"token":      resp.Token,
		"expires_at": resp.ExpiresAt.Format(time.RFC3339),
		"user_id":    resp.UserID,
		"email":      resp.Email,
		"username":   resp.Username,
		"nickname":   resp.Nickname,
		"role":       resp.Role,
	}
}

func (s *Server) revokeAndClearBrowserSession(c *gin.Context) {
	if raw, present := s.refreshTokenFromCookie(c); present {
		if err := s.authSvc.RevokeSession(c.Request.Context(), raw); err != nil {
			log.Printf("[ERROR] clear browser session after credential change: %v", err)
		}
	}
	s.clearRefreshCookie(c)
}

func (s *Server) handleRegister(c *gin.Context) {
	var req struct {
		Email        string `json:"email"         binding:"required,email"`
		Username     string `json:"username"      binding:"required"`
		Password     string `json:"password"      binding:"required,min=8"`
		Nickname     string `json:"nickname"`
		CaptchaToken string `json:"captcha_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	if err := s.captchaVerifier.Verify(req.CaptchaToken, s.realIP(c)); err != nil {
		fail(c, http.StatusBadRequest, "invalid_captcha", "Captcha verification failed.")
		return
	}
	newUser, err := s.authSvc.Register(c.Request.Context(), service.RegisterRequest{
		Email: req.Email, Username: req.Username, Password: req.Password, Nickname: req.Nickname,
	})
	if err != nil {
		failDomain(c, err)
		return
	}
	// Create default personal & public accounts for the new user.
	if err := s.acctSvc.EnsureDefaultAccounts(c.Request.Context(), newUser.ID); err != nil {
		log.Printf("[WARN] failed to create default accounts for user %s: %v", newUser.ID, err)
	}
	// If email verification is required, don't auto-login.
	if s.authSvc.EmailVerificationRequired() {
		c.JSON(http.StatusAccepted, gin.H{"success": true, "data": gin.H{"message": "Registration successful. Verification email sent."}})
		return
	}
	// Auto-login after registration (email not required)
	resp, err := s.authSvc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		failDomain(c, err)
		return
	}
	s.setRefreshCookie(c, resp.RefreshToken, resp.RefreshExpiresAt)
	created(c, authResponsePayload(resp))
}

func (s *Server) handleLogin(c *gin.Context) {
	var req struct {
		Email        string `json:"email"         binding:"required"`
		Password     string `json:"password"      binding:"required"`
		CaptchaToken string `json:"captcha_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	if err := s.captchaVerifier.Verify(req.CaptchaToken, s.realIP(c)); err != nil {
		fail(c, http.StatusBadRequest, "invalid_captcha", "Captcha verification failed.")
		return
	}
	resp, err := s.authSvc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		failDomain(c, err)
		return
	}
	s.setRefreshCookie(c, resp.RefreshToken, resp.RefreshExpiresAt)
	ok(c, authResponsePayload(resp))
}

// handleVerifyEmailJSON consumes a token only after the frontend's explicit
// confirmation. GET requests merely serve the SPA and never mutate state.
func (s *Server) handleVerifyEmailJSON(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, 40001, "缺少验证令牌")
		return
	}
	if err := s.authSvc.VerifyEmail(c.Request.Context(), req.Token); err != nil {
		fail(c, 400, 40002, err.Error())
		return
	}
	ok(c, gin.H{"message": "邮箱验证成功"})
}

func (s *Server) handleResendVerification(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	_ = s.authSvc.ResendVerification(c.Request.Context(), req.Email)
	ok(c, gin.H{"message": "如果该邮箱已注册，验证邮件将在几分钟内发送"})
}

func (s *Server) handleForgotPassword(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	_ = s.authSvc.ForgotPassword(c.Request.Context(), req.Email)
	ok(c, gin.H{"message": "如果该邮箱已注册，重置密码邮件将在几分钟内发送"})
}

func (s *Server) handleResetPassword(c *gin.Context) {
	var req struct {
		Token       string `json:"token"        binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	if err := s.authSvc.ResetPassword(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		mapAuthError(c, err, 40002)
		return
	}
	s.revokeAndClearBrowserSession(c)
	ok(c, gin.H{"message": "密码重置成功，请使用新密码登录"})
}

func (s *Server) handleChangePassword(c *gin.Context) {
	userID := c.GetString("userID")
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password"     binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	if err := s.authSvc.ChangePassword(c.Request.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		failDomain(c, err)
		return
	}
	s.revokeAndClearBrowserSession(c)
	ok(c, gin.H{"message": "密码修改成功"})
}

func (s *Server) handleRequestDeleteAccount(c *gin.Context) {
	if err := s.authSvc.RequestAccountDeletion(c.Request.Context(), userID(c)); err != nil {
		fail(c, 400, 40010, err.Error())
		return
	}
	ok(c, gin.H{"message": "注销确认邮件已发送，请在 30 分钟内点击邮件中的链接完成操作"})
}

func (s *Server) handleConfirmDeleteAccount(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c, 40011)
		return
	}
	if err := s.authSvc.ConfirmAccountDeletion(c.Request.Context(), req.Token); err != nil {
		mapAuthError(c, err, 40012)
		return
	}
	s.revokeAndClearBrowserSession(c)
	ok(c, gin.H{"message": "账户已注销，感谢您使用 FinArch"})
}

func (s *Server) handleGetMe(c *gin.Context) {
	u, err := s.authSvc.GetUserProfile(c.Request.Context(), userID(c))
	if err != nil {
		failDomain(c, err)
		return
	}
	ok(c, gin.H{
		"id":            u.ID,
		"email":         u.Email,
		"username":      u.Username,
		"nickname":      u.Nickname,
		"pending_email": u.PendingEmail,
		"role":          u.Role,
	})
}

// handleRefreshToken atomically rotates an opaque HttpOnly refresh bearer.
func (s *Server) handleRefreshToken(c *gin.Context) {
	rawRefresh, present := s.refreshTokenFromCookie(c)
	if !present {
		s.clearRefreshCookie(c)
		fail(c, http.StatusUnauthorized, "session_invalid", "The session is missing or expired.")
		return
	}
	resp, err := s.authSvc.RefreshSession(c.Request.Context(), rawRefresh)
	if err != nil {
		if errors.Is(err, service.ErrInvalidOrUsedToken) ||
			errors.Is(err, service.ErrRefreshTokenReuse) ||
			errors.Is(err, service.ErrSessionInvalid) {
			s.clearRefreshCookie(c)
			fail(c, http.StatusUnauthorized, "session_invalid", "The session is invalid or expired.")
			return
		}
		log.Printf("[ERROR] refresh session: %v", err)
		fail(c, http.StatusServiceUnavailable, "system_unavailable", "System temporarily unavailable. Please try again later.")
		return
	}
	s.setRefreshCookie(c, resp.RefreshToken, resp.RefreshExpiresAt)
	ok(c, authResponsePayload(resp))
}

func (s *Server) handleLogout(c *gin.Context) {
	rawRefresh, present := s.refreshTokenFromCookie(c)
	if present {
		if err := s.authSvc.RevokeSession(c.Request.Context(), rawRefresh); err != nil {
			// Keep the cookie so an explicit client retry can still revoke the
			// server-side family after a transient database failure.
			log.Printf("[ERROR] revoke session: %v", err)
			fail(c, http.StatusServiceUnavailable, "system_unavailable", "System temporarily unavailable. Please try again later.")
			return
		}
	}
	s.clearRefreshCookie(c)
	ok(c, gin.H{"message": "Logged out."})
}

func (s *Server) handleRequestEmailChange(c *gin.Context) {
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewEmail        string `json:"new_email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c, 40020)
		return
	}
	if err := s.authSvc.RequestEmailChange(c.Request.Context(), userID(c), req.CurrentPassword, req.NewEmail); err != nil {
		mapAuthError(c, err, 40021)
		return
	}
	ok(c, gin.H{"message": "验证邮件已发送至当前邮箱，请在 1 小时内点击授权链接"})
}

func (s *Server) handleConfirmOldEmailChange(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c, 40024)
		return
	}
	if err := s.authSvc.ConfirmOldEmailForChange(c.Request.Context(), req.Token); err != nil {
		mapAuthError(c, err, 40025)
		return
	}
	ok(c, gin.H{"message": "授权成功，验证邮件已发送至新邮箱"})
}

func (s *Server) handleConfirmEmailChange(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c, 40022)
		return
	}
	if err := s.authSvc.ConfirmEmailChange(c.Request.Context(), req.Token); err != nil {
		mapAuthError(c, err, 40023)
		return
	}
	s.revokeAndClearBrowserSession(c)
	ok(c, gin.H{"message": "邮箱已更新"})
}

func (s *Server) handleUpdateNickname(c *gin.Context) {
	var req struct {
		Nickname string `json:"nickname" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c, 40030)
		return
	}
	if err := s.authSvc.UpdateNickname(c.Request.Context(), userID(c), req.Nickname); err != nil {
		fail(c, 400, 40031, err.Error())
		return
	}
	ok(c, gin.H{"message": "昵称已更新", "nickname": req.Nickname})
}

// ─── Device Heartbeat & Online Count ─────────────────────────────────────────

// handleHeartbeat records a device heartbeat for the current user.
// POST /auth/heartbeat  { "device_id": "..." }
func (s *Server) handleHeartbeat(c *gin.Context) {
	var req struct {
		DeviceID string `json:"device_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, 40040, "缺少设备标识")
		return
	}
	key := userID(c) + ":" + req.DeviceID
	s.activeDevices.Store(key, &deviceSession{
		LastSeen:  time.Now(),
		UserAgent: c.GetHeader("User-Agent"),
	})
	ok(c, gin.H{"status": "ok"})
}

// handleOnlineDevices returns the count of recently-active devices for the current user.
// GET /auth/devices/online
func (s *Server) handleOnlineDevices(c *gin.Context) {
	uid := userID(c)
	prefix := uid + ":"
	cutoff := time.Now().Add(-5 * time.Minute)
	count := 0
	s.activeDevices.Range(func(key, value any) bool {
		k := key.(string)
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			ds := value.(*deviceSession)
			if ds.LastSeen.After(cutoff) {
				count++
			}
		}
		return true
	})
	ok(c, gin.H{"count": count})
}

// CleanupStaleDevices removes device sessions that haven't sent a heartbeat in 10 minutes.
func (s *Server) CleanupStaleDevices() {
	cutoff := time.Now().Add(-10 * time.Minute)
	s.activeDevices.Range(func(key, value any) bool {
		ds := value.(*deviceSession)
		if ds.LastSeen.Before(cutoff) {
			s.activeDevices.Delete(key)
		}
		return true
	})
}

func parseMode(raw string) (model.Mode, bool) {
	if raw == "" {
		return model.ModeWork, true
	}
	mode := model.Mode(strings.ToLower(raw))
	if mode != model.ModeWork && mode != model.ModeLife {
		return "", false
	}
	return mode, true
}

func parseOccurredAt(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().UTC().Truncate(time.Second), nil
	}
	layouts := []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			if layout == "2006-01-02" {
				return t.UTC(), nil
			}
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid occurred_at format")
}

func formatSecond(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Local().Format("2006-01-02 15:04:05")
}

func formatLifecycleSecond(ts *time.Time, createdAt time.Time) *string {
	if ts == nil || ts.IsZero() {
		return nil
	}
	effective := *ts
	if !createdAt.IsZero() && effective.Before(createdAt) {
		effective = createdAt
	}
	v := formatSecond(effective)
	return &v
}

// ─── Transactions ────────────────────────────────────────────────────────────

func (s *Server) handleListTransactions(c *gin.Context) {
	mode, modeOK := parseMode(c.Query("mode"))
	if !modeOK {
		fail(c, 400, 40001, "invalid mode")
		return
	}
	txs, err := s.txRepo.ListByUser(c.Request.Context(), userID(c), mode)
	if err != nil {
		failInternal(c, err)
		return
	}
	type txDTO struct {
		// V9 fields
		ID                 string  `json:"id"`
		GroupID            string  `json:"group_id"`
		AccountID          string  `json:"account_id"`
		AccountType        string  `json:"account_type"`
		LedgerDir          string  `json:"ledger_dir"`
		TxType             string  `json:"type"`
		AmountCents        int64   `json:"amount_cents"`
		BaseAmountCents    int64   `json:"base_amount_cents"`
		ExchangeRate       float64 `json:"exchange_rate"`
		ExchangeRateSource string  `json:"exchange_rate_source"`
		ExchangeRateAt     int64   `json:"exchange_rate_at"`
		BaseCurrency       string  `json:"base_currency"`
		ReimbStatus        string  `json:"reimb_status"`
		TxnDate            string  `json:"txn_date"`
		TransactionTime    int64   `json:"transaction_time"`
		CreatedAt          string  `json:"created_at"`
		UpdatedAt          string  `json:"updated_at"`
		ReportedAt         *string `json:"reported_at"`
		ReimbursedAt       *string `json:"reimbursed_at"`
		// Backward-compat fields retained for frontend
		OccurredAt              string   `json:"occurred_at"`
		Direction               string   `json:"direction"`
		Source                  string   `json:"source"`
		Category                string   `json:"category"`
		AmountYuan              float64  `json:"amount_yuan"`
		Currency                string   `json:"currency"`
		Note                    string   `json:"note"`
		ProjectID               *string  `json:"project_id"`
		Project                 *string  `json:"project"`
		Reimbursed              bool     `json:"reimbursed"`
		Uploaded                bool     `json:"uploaded"`
		AttachmentKey           *string  `json:"attachment_key"`
		HasAttachment           bool     `json:"has_attachment"`
		RecurringRuleID         *string  `json:"recurring_rule_id"`
		RecurringOccurrenceDate *string  `json:"recurring_occurrence_date"`
		Mode                    string   `json:"mode"`
		Tags                    []string `json:"tags"`
	}
	dtos := make([]txDTO, 0, len(txs))
	for _, t := range txs {
		tags, _ := s.tagRepo.ListByTransaction(c.Request.Context(), t.ID)
		tagNames := make([]string, 0, len(tags))
		for _, tg := range tags {
			tagNames = append(tagNames, tg.Name)
		}
		reportedAt := formatLifecycleSecond(t.ReportedAt, t.CreatedAt)
		reimbursedAt := formatLifecycleSecond(t.ReimbursedAt, t.CreatedAt)
		dtos = append(dtos, txDTO{
			ID: t.ID, GroupID: t.GroupID,
			AccountID: t.AccountID, AccountType: string(t.AccountType),
			LedgerDir: string(t.LedgerDir), TxType: string(t.TxType),
			AmountCents: t.AmountCents, BaseAmountCents: t.BaseAmountCents,
			ExchangeRate: t.ExchangeRate, ExchangeRateSource: t.ExchangeRateSource, ExchangeRateAt: t.ExchangeRateAt, BaseCurrency: t.BaseCurrency, ReimbStatus: string(t.ReimbStatus),
			TxnDate: t.TxnDate, TransactionTime: t.TransactionTime,
			CreatedAt: formatSecond(t.CreatedAt), UpdatedAt: formatSecond(t.UpdatedAt),
			ReportedAt: reportedAt, ReimbursedAt: reimbursedAt,
			// backward-compat
			OccurredAt: formatSecond(t.OccurredAt),
			Direction:  string(t.Direction), Source: string(t.Source),
			Category: t.Category, AmountYuan: t.AmountYuan.Float64(),
			Currency: t.Currency, Note: t.Note,
			ProjectID: t.ProjectID, Project: t.Project,
			Reimbursed: t.Reimbursed, Uploaded: t.Uploaded,
			AttachmentKey: t.AttachmentKey, HasAttachment: t.AttachmentKey != nil,
			RecurringRuleID: t.RecurringRuleID, RecurringOccurrenceDate: t.RecurringOccurrenceDate,
			Mode: string(t.Mode), Tags: tagNames,
		})
	}
	ok(c, dtos)
}

func (s *Server) handleCreateTransaction(c *gin.Context) {
	rawIdempotencyKey, hasIdempotencyKey, err := parseIdempotencyKey(c.Request)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_idempotency_key", err.Error())
		return
	}

	var req createTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	if req.AmountYuan <= 0 && req.AmountCents <= 0 {
		fail(c, 422, 40001, "请输入有效的金额")
		return
	}
	txDate, err := parseOccurredAt(req.OccurredAt)
	if err != nil {
		fail(c, 422, 40001, "时间格式必须为 YYYY-MM-DD HH:mm:ss")
		return
	}
	// Normalize type from direction for backward compat
	txType := model.TxType(req.Type)
	if txType == "" {
		if req.Direction == "expense" {
			txType = model.TxTypeExpense
		} else if req.Direction == "income" {
			txType = model.TxTypeIncome
		}
	}
	projID := req.ProjectID
	if projID != nil && *projID == "" {
		projID = nil
	}
	// Tags are a set in storage. Normalize the actual write as well as the
	// request fingerprint so reordering/duplicates have identical semantics.
	req.TagIDs = normalizeTagIDs(req.TagIDs)
	var scopedKey, requestHash string
	if hasIdempotencyKey {
		scopedKey = scopedIdempotencyKey(userID(c), createTransactionIdempotencyEndpoint, rawIdempotencyKey)
		requestHash, err = createTransactionRequestHash(req, txType, projID)
		if err != nil {
			fail(c, http.StatusUnprocessableEntity, "invalid_amount", "Please enter a valid amount.")
			return
		}
	}
	requestUserID := userID(c)
	expectedReplayID := ""
	if hasIdempotencyKey {
		lookupErr := retryIdempotentSQLiteTransaction(c.Request.Context(), maxIdempotencyTransactionAttempts, func() error {
			preflightTransaction, getErr := s.txRepo.GetByIdempotencyKey(c.Request.Context(), requestUserID, scopedKey)
			if getErr == nil {
				expectedReplayID = preflightTransaction.ID
			}
			return getErr
		})
		switch {
		case lookupErr == nil:
			// This is only a read-side optimization. The reservation hash and
			// transaction are loaded again under the authoritative write lock.
		case errors.Is(lookupErr, repository.ErrTransactionNotFound):
			// A genuinely new request needs a prepared rate below.
		default:
			failInternal(c, fmt.Errorf("%w: preflight replay transaction: %w", errIdempotencyState, lookupErr))
			return
		}
	}
	expectedReplay := expectedReplayID != ""

	preparedCreateReq := service.CreateTransactionRequest{
		UserID:       requestUserID,
		Mode:         model.Mode(req.Mode),
		OccurredAt:   txDate,
		AccountID:    req.AccountID,
		TxType:       txType,
		Direction:    model.Direction(req.Direction),
		Source:       model.Source(req.Source),
		Category:     req.Category,
		AmountYuan:   model.Money(req.AmountYuan),
		AmountCents:  req.AmountCents,
		ExchangeRate: req.ExchangeRate,
		Currency:     req.Currency,
		Note:         req.Note,
		ProjectID:    projID,
	}
	if !expectedReplay {
		// Resolve/create legacy default accounts before rate preparation. This
		// method completes any individual account writes before returning and,
		// on the common existing-account path, never acquires a write lock.
		if preparedCreateReq.AccountID == "" {
			ensureErr := retryIdempotentSQLiteTransaction(c.Request.Context(), maxIdempotencyTransactionAttempts, func() error {
				return s.acctSvc.EnsureDefaultAccounts(c.Request.Context(), requestUserID)
			})
			if ensureErr != nil {
				fail(c, 422, 40001, ensureErr.Error())
				return
			}
		}
		unpreparedCreateReq := preparedCreateReq
		err = retryIdempotentSQLiteTransaction(c.Request.Context(), maxIdempotencyTransactionAttempts, func() error {
			candidate, prepareErr := s.txSvc.PrepareCreateTransaction(c.Request.Context(), unpreparedCreateReq)
			if prepareErr == nil {
				preparedCreateReq = candidate
			}
			return prepareErr
		})
		if err != nil {
			fail(c, 422, 40001, err.Error())
			return
		}
	}

	var createdTx model.Transaction
	var tagFoundErr = errors.New("标签不存在")
	var projectCreateErr = errors.New("创建项目失败")
	replayed := false
	projectCreatedAt := time.Now().UTC()
	projectRepo := sqliterepo.NewSQLiteProjectRepository(s.db)

	attempts := 1
	if hasIdempotencyKey {
		attempts = maxIdempotencyTransactionAttempts
	}
	err = retryIdempotentSQLiteTransaction(c.Request.Context(), attempts, func() error {
		createdTx = model.Transaction{}
		replayed = false
		return s.txManager.WithinTransaction(c.Request.Context(), func(ctx context.Context) error {
			if hasIdempotencyKey {
				storedRequestHash, claimed, claimErr := s.txRepo.ClaimIdempotencyKey(
					ctx, requestUserID, createTransactionIdempotencyEndpoint, scopedKey, requestHash,
				)
				if claimErr != nil {
					return fmt.Errorf("%w: claim key: %w", errIdempotencyState, claimErr)
				}
				if claimed && expectedReplay {
					// Preflight observed a committed transaction. If its reservation
					// disappeared, never turn this replay into a fresh financial write.
					return errIdempotencyChanged
				}
				if !claimed {
					if storedRequestHash != requestHash {
						return errIdempotencyPayloadMismatch
					}
					existing, getErr := s.txRepo.GetByIdempotencyKey(ctx, requestUserID, scopedKey)
					if getErr != nil {
						if expectedReplay && errors.Is(getErr, repository.ErrTransactionNotFound) {
							return errIdempotencyChanged
						}
						return fmt.Errorf("%w: load replay transaction: %w", errIdempotencyState, getErr)
					}
					if expectedReplay && existing.ID != expectedReplayID {
						return errIdempotencyChanged
					}
					createdTx = existing
					replayed = true
					return nil
				}
			}

			// This row is a dependency of the financial write, not a request
			// preflight side effect. Create it only after an idempotency replay has
			// been ruled out and keep it in the same transaction as the ledger row.
			if projID != nil {
				if inErr := projectRepo.Ensure(ctx, model.Project{
					ID: *projID, Name: *projID, Code: *projID, CreatedAt: projectCreatedAt,
				}); inErr != nil {
					return fmt.Errorf("%w: %v", projectCreateErr, inErr)
				}
			}

			var idempotencyKey *string
			if hasIdempotencyKey {
				idempotencyKey = &scopedKey
			}
			createReq := preparedCreateReq
			createReq.IdempotencyKey = idempotencyKey
			var inErr error
			createdTx, inErr = s.txSvc.CreateTransaction(ctx, createReq)
			if inErr != nil {
				return inErr
			}
			for _, tagID := range req.TagIDs {
				if inErr := s.tagRepo.AddToTransaction(ctx, requestUserID, createdTx.ID, tagID); inErr != nil {
					return tagFoundErr
				}
			}
			return nil
		})
	})

	if err != nil {
		if errors.Is(err, errIdempotencyPayloadMismatch) {
			fail(c, http.StatusConflict, "idempotency_key_reused", "Idempotency-Key was already used with a different request.")
		} else if errors.Is(err, errIdempotencyChanged) {
			fail(c, http.StatusConflict, "concurrent_modification", "Idempotency state changed during the request; please retry.")
		} else if errors.Is(err, errIdempotencyState) {
			failInternal(c, err)
		} else if errors.Is(err, projectCreateErr) {
			fail(c, http.StatusInternalServerError, 50001, "创建项目失败，请稍后重试")
		} else if errors.Is(err, tagFoundErr) {
			fail(c, 422, 40002, "标签不存在")
		} else {
			fail(c, 422, 40001, err.Error())
		}
		return
	}
	if replayed {
		c.Header("Idempotency-Replayed", "true")
	}

	created(c, gin.H{
		"id":                   createdTx.ID,
		"amount_yuan":          createdTx.AmountYuan.Float64(),
		"amount_cents":         createdTx.AmountCents,
		"reimb_status":         string(createdTx.ReimbStatus),
		"account_id":           createdTx.AccountID,
		"mode":                 string(createdTx.Mode),
		"occurred_at":          formatSecond(createdTx.OccurredAt),
		"transaction_time":     createdTx.TransactionTime,
		"base_amount_cents":    createdTx.BaseAmountCents,
		"base_currency":        createdTx.BaseCurrency,
		"exchange_rate":        createdTx.ExchangeRate,
		"exchange_rate_source": createdTx.ExchangeRateSource,
		"exchange_rate_at":     createdTx.ExchangeRateAt,
	})
}

func (s *Server) handleToggleReimbursed(c *gin.Context) {
	id := c.Param("id")
	newState, err := s.txRepo.ToggleReimbursed(c.Request.Context(), id, userID(c))
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrConcurrentModification):
			fail(c, http.StatusConflict, "concurrent_modification", "The transaction changed concurrently. Please refresh and try again.")
		case errors.Is(err, repository.ErrTransactionNotFound):
			fail(c, http.StatusNotFound, "transaction_not_found", "Transaction not found.")
		default:
			fail(c, http.StatusUnprocessableEntity, 40001, err.Error())
		}
		return
	}
	ok(c, gin.H{"id": id, "reimbursed": newState})
}

func (s *Server) handleToggleUploaded(c *gin.Context) {
	id := c.Param("id")
	newState, err := s.txRepo.ToggleUploaded(c.Request.Context(), id, userID(c))
	if err != nil {
		fail(c, 400, 40001, err.Error())
		return
	}
	ok(c, gin.H{"id": id, "uploaded": newState})
}

func (s *Server) handleAddTag(c *gin.Context) {
	txID := c.Param("id")
	var req struct {
		TagID string `json:"tag_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	if err := s.tagRepo.AddToTransaction(c.Request.Context(), userID(c), txID, req.TagID); err != nil {
		failInternal(c, err)
		return
	}
	ok(c, gin.H{"transaction_id": txID, "tag_id": req.TagID})
}

func (s *Server) handleRemoveTag(c *gin.Context) {
	if err := s.tagRepo.RemoveFromTransaction(c.Request.Context(), userID(c), c.Param("id"), c.Param("tagID")); err != nil {
		failInternal(c, err)
		return
	}
	ok(c, gin.H{"removed": true})
}

// ─── Tags ────────────────────────────────────────────────────────────────────

func (s *Server) handleListTags(c *gin.Context) {
	tags, err := s.tagRepo.ListByOwner(c.Request.Context(), userID(c))
	if err != nil {
		failInternal(c, err)
		return
	}
	if tags == nil {
		tags = []model.Tag{}
	}
	ok(c, tags)
}

func (s *Server) handleCreateTag(c *gin.Context) {
	var req struct {
		Name  string `json:"name"  binding:"required"`
		Color string `json:"color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	if req.Color == "" {
		req.Color = "#6366f1"
	}
	tag := model.Tag{
		ID: uuid.NewString(), OwnerID: userID(c),
		Name: req.Name, Color: req.Color, CreatedAt: time.Now(),
	}
	if err := s.tagRepo.Create(c.Request.Context(), tag); err != nil {
		fail(c, 409, 40901, "标签名称已存在")
		return
	}
	created(c, tag)
}

func (s *Server) handleDeleteTag(c *gin.Context) {
	if err := s.tagRepo.Delete(c.Request.Context(), c.Param("id"), userID(c)); err != nil {
		failInternal(c, err)
		return
	}
	ok(c, gin.H{"deleted": true})
}

// ─── Match ───────────────────────────────────────────────────────────────────

func (s *Server) handleMatch(c *gin.Context) {
	var req struct {
		// V2 preferred: integer cents
		TargetCents    int64 `json:"target_cents"`
		ToleranceCents int64 `json:"tolerance_cents"`
		// Backward-compat: yuan floats (still accepted)
		TargetYuan    float64 `json:"target_yuan"`
		ToleranceYuan float64 `json:"tolerance_yuan"`
		MaxItems      int     `json:"max_items"`
		ProjectID     *string `json:"project_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	// Resolve target: cents takes priority over yuan.
	var targetYuan, toleranceYuan model.Money
	if req.TargetCents > 0 {
		targetYuan = model.Money(req.TargetCents) / 100
		toleranceYuan = model.Money(req.ToleranceCents) / 100
	} else if req.TargetYuan > 0 {
		targetYuan = model.Money(req.TargetYuan)
		toleranceYuan = model.Money(req.ToleranceYuan)
	} else {
		fail(c, 422, 40001, "请输入有效的目标金额")
		return
	}
	maxDepth := req.MaxItems
	if maxDepth <= 0 {
		maxDepth = 10
	} else if maxDepth > 50 {
		maxDepth = 50
	}
	limit := 20
	results, err := s.matchSvc.Match(
		c.Request.Context(),
		userID(c),
		targetYuan, toleranceYuan,
		maxDepth, req.ProjectID, limit,
	)
	if err != nil {
		failInternal(c, err)
		return
	}
	// Collect all unique IDs for batch fetch
	idSet := map[string]struct{}{}
	for _, r := range results {
		for _, id := range r.TransactionIDs {
			idSet[id] = struct{}{}
		}
	}
	allIDs := make([]string, 0, len(idSet))
	for id := range idSet {
		allIDs = append(allIDs, id)
	}
	txMap := map[string]model.Transaction{}
	if len(allIDs) > 0 {
		txList, ferr := s.txRepo.GetByIDs(c.Request.Context(), userID(c), allIDs)
		if ferr != nil {
			log.Printf("[ERROR] match fetch: %v", ferr)
			fail(c, 500, 50001, "获取交易详情失败")
			return
		}
		for _, t := range txList {
			txMap[t.ID] = t
		}
	}

	type itemDTO struct {
		ID         string  `json:"id"`
		OccurredAt string  `json:"occurred_at"`
		Direction  string  `json:"direction"`
		Source     string  `json:"source"`
		Category   string  `json:"category"`
		AmountYuan float64 `json:"amount_yuan"`
		Currency   string  `json:"currency"`
		Note       string  `json:"note"`
		ProjectID  *string `json:"project_id"`
		Uploaded   bool    `json:"uploaded"`
	}
	type dto struct {
		IDs          []string  `json:"ids"`
		Total        float64   `json:"total"`
		TotalCents   int64     `json:"total_cents"`
		Error        float64   `json:"error"`
		ErrorCents   int64     `json:"error_cents"`
		ProjectCount int       `json:"project_count"`
		ItemCount    int       `json:"item_count"`
		Score        float64   `json:"score"`
		TimePruned   bool      `json:"time_pruned"`
		Items        []itemDTO `json:"items"`
	}
	dtos := make([]dto, 0, len(results))
	for _, r := range results {
		ids := r.TransactionIDs
		if ids == nil {
			ids = []string{}
		}
		items := make([]itemDTO, 0, len(ids))
		for _, id := range ids {
			if t, ok := txMap[id]; ok {
				items = append(items, itemDTO{
					ID:         t.ID,
					OccurredAt: t.OccurredAt.Format("2006-01-02"),
					Direction:  string(t.Direction),
					Source:     string(t.Source),
					Category:   t.Category,
					AmountYuan: t.AmountYuan.Float64(),
					Currency:   t.Currency,
					Note:       t.Note,
					ProjectID:  t.ProjectID,
					Uploaded:   t.Uploaded,
				})
			}
		}
		dtos = append(dtos, dto{
			IDs:          ids,
			Total:        r.TotalYuan.Float64(),
			TotalCents:   r.TotalCents,
			Error:        r.AbsErrorYuan.Float64(),
			ErrorCents:   r.AbsErrorCents,
			ProjectCount: r.ProjectCount,
			ItemCount:    r.ItemCount,
			Score:        r.Score,
			TimePruned:   r.TimePruned,
			Items:        items,
		})
	}
	ok(c, dtos)
}

// ─── Reimbursements ──────────────────────────────────────────────────────────

func (s *Server) handleCreateReimbursement(c *gin.Context) {
	var req struct {
		Applicant      string   `json:"applicant"       binding:"required"`
		TransactionIDs []string `json:"transaction_ids" binding:"required,min=1"`
		RequestNo      string   `json:"request_no"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	reim, err := s.reimSvc.CreateReimbursement(c.Request.Context(), service.CreateReimbursementRequest{
		UserID: userID(c), Applicant: req.Applicant, TransactionIDs: req.TransactionIDs, RequestNo: req.RequestNo,
	})
	if err != nil {
		if errors.Is(err, service.ErrConcurrentModification) {
			failDomain(c, err)
			return
		}
		if errors.Is(err, repository.ErrMultiCurrencyReportingUnavailable) {
			failDomain(c, err)
			return
		}
		fail(c, 400, 40001, err.Error())
		return
	}
	created(c, gin.H{
		"id": reim.ID, "request_no": reim.RequestNo,
		"total_cents": reim.TotalCents, "total_yuan": reim.TotalYuan.Float64(), "status": reim.Status,
	})
}

// ─── Stats ───────────────────────────────────────────────────────────────────

func (s *Server) handleStatsSummary(c *gin.Context) {
	b, err := s.statsSvc.Summary(c.Request.Context(), userID(c))
	if err != nil {
		failDomain(c, err)
		return
	}
	ok(c, b)
}

func (s *Server) handleStatsMonthly(c *gin.Context) {
	year := time.Now().Year()
	if y := c.Query("year"); y != "" {
		if t, err := time.Parse("2006", y); err == nil {
			year = t.Year()
		}
	}
	stats, err := s.statsSvc.Monthly(c.Request.Context(), userID(c), year)
	if err != nil {
		failDomain(c, err)
		return
	}
	if stats == nil {
		stats = []service.MonthlyStat{}
	}
	ok(c, stats)
}

func (s *Server) handleStatsByCategory(c *gin.Context) {
	stats, err := s.statsSvc.ByCategory(c.Request.Context(), userID(c), c.Query("date_from"), c.Query("date_to"))
	if err != nil {
		failDomain(c, err)
		return
	}
	if stats == nil {
		stats = []service.CategoryStat{}
	}
	ok(c, stats)
}

func (s *Server) handleStatsByProject(c *gin.Context) {
	stats, err := s.statsSvc.ByProject(c.Request.Context(), userID(c))
	if err != nil {
		failDomain(c, err)
		return
	}
	if stats == nil {
		stats = []service.ProjectStat{}
	}
	ok(c, stats)
}

func (s *Server) handleStatsAccountBalanceHistory(c *gin.Context) {
	mode, modeOK := parseMode(c.Query("mode"))
	if !modeOK {
		fail(c, 400, 40001, "invalid mode")
		return
	}
	rangeKey := c.DefaultQuery("range", "30d")
	accountID := strings.TrimSpace(c.Query("account_id"))
	points, err := s.statsSvc.AccountBalanceHistory(c.Request.Context(), userID(c), mode, rangeKey, accountID)
	if err != nil {
		if strings.Contains(err.Error(), "invalid range") {
			fail(c, 400, 40001, "invalid range")
			return
		}
		failDomain(c, err)
		return
	}
	if points == nil {
		points = []service.BalanceHistoryPoint{}
	}
	ok(c, points)
}

// ─── Budgets ──────────────────────────────────────────────────────────────────

type budgetDTO struct {
	ID              string  `json:"id"`
	Mode            string  `json:"mode"`
	PeriodMonth     string  `json:"period_month"`
	Category        string  `json:"category"`
	AmountCents     int64   `json:"amount_cents"`
	AmountYuan      float64 `json:"amount_yuan"`
	Currency        string  `json:"currency"`
	BaseCurrency    string  `json:"base_currency"`
	BaseAmountCents int64   `json:"base_amount_cents"`
	BaseAmountYuan  float64 `json:"base_amount_yuan"`
	IsActive        bool    `json:"is_active"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type budgetProgressDTO struct {
	Budget         budgetDTO `json:"budget"`
	ActualCents    int64     `json:"actual_cents"`
	ActualYuan     float64   `json:"actual_yuan"`
	RemainingCents int64     `json:"remaining_cents"`
	RemainingYuan  float64   `json:"remaining_yuan"`
	UsageRatio     float64   `json:"usage_ratio"`
	Status         string    `json:"status"`
}

func budgetToDTO(b model.Budget) budgetDTO {
	return budgetDTO{
		ID:              b.ID,
		Mode:            string(b.Mode),
		PeriodMonth:     b.PeriodMonth,
		Category:        b.Category,
		AmountCents:     b.AmountCents,
		AmountYuan:      float64(b.AmountCents) / 100,
		Currency:        b.Currency,
		BaseCurrency:    b.BaseCurrency,
		BaseAmountCents: b.BaseAmountCents,
		BaseAmountYuan:  float64(b.BaseAmountCents) / 100,
		IsActive:        b.IsActive,
		CreatedAt:       formatSecond(b.CreatedAt),
		UpdatedAt:       formatSecond(b.UpdatedAt),
	}
}

func budgetProgressToDTO(p service.BudgetProgress) budgetProgressDTO {
	return budgetProgressDTO{
		Budget:         budgetToDTO(p.Budget),
		ActualCents:    p.ActualCents,
		ActualYuan:     float64(p.ActualCents) / 100,
		RemainingCents: p.RemainingCents,
		RemainingYuan:  float64(p.RemainingCents) / 100,
		UsageRatio:     p.UsageRatio,
		Status:         p.Status,
	}
}

func parseBudgetMonth(c *gin.Context) string {
	if period := c.Query("period"); period != "" {
		return period
	}
	return c.Query("period_month")
}

func budgetAmountCents(amountCents int64, amountYuan float64) (int64, error) {
	if amountCents > 0 {
		return amountCents, nil
	}
	if amountCents < 0 || amountYuan < 0 {
		return 0, fmt.Errorf("amount must not be negative")
	}
	if amountYuan == 0 {
		return 0, nil
	}
	converted, err := model.Money(amountYuan).Cents()
	if err != nil || converted <= 0 {
		return 0, fmt.Errorf("invalid amount")
	}
	return converted, nil
}

func (s *Server) handleListBudgets(c *gin.Context) {
	mode, modeOK := parseMode(c.Query("mode"))
	if !modeOK {
		fail(c, 400, 40001, "invalid mode")
		return
	}
	budgets, err := s.budgetSvc.ListBudgets(c.Request.Context(), userID(c), mode, parseBudgetMonth(c))
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	dtos := make([]budgetDTO, 0, len(budgets))
	for _, b := range budgets {
		dtos = append(dtos, budgetToDTO(b))
	}
	ok(c, dtos)
}

func (s *Server) handleCreateBudget(c *gin.Context) {
	var req struct {
		Mode            string  `json:"mode" binding:"required"`
		PeriodMonth     string  `json:"period_month"`
		Period          string  `json:"period"`
		Category        string  `json:"category"`
		AmountCents     int64   `json:"amount_cents"`
		AmountYuan      float64 `json:"amount_yuan"`
		Currency        string  `json:"currency"`
		BaseCurrency    string  `json:"base_currency"`
		BaseAmountCents int64   `json:"base_amount_cents"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	mode, modeOK := parseMode(req.Mode)
	if !modeOK {
		fail(c, 400, 40001, "invalid mode")
		return
	}
	periodMonth := req.PeriodMonth
	if periodMonth == "" {
		periodMonth = req.Period
	}
	amountCents, err := budgetAmountCents(req.AmountCents, req.AmountYuan)
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "invalid_amount", "Please enter a valid amount.")
		return
	}
	budget, err := s.budgetSvc.CreateBudget(c.Request.Context(), service.CreateBudgetRequest{
		UserID:          userID(c),
		Mode:            mode,
		PeriodMonth:     periodMonth,
		Category:        req.Category,
		AmountCents:     amountCents,
		Currency:        req.Currency,
		BaseCurrency:    req.BaseCurrency,
		BaseAmountCents: req.BaseAmountCents,
	})
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	created(c, budgetToDTO(budget))
}

func (s *Server) handleUpdateBudget(c *gin.Context) {
	var req struct {
		Mode            string  `json:"mode"`
		PeriodMonth     string  `json:"period_month"`
		Period          string  `json:"period"`
		Category        *string `json:"category"`
		AmountCents     int64   `json:"amount_cents"`
		AmountYuan      float64 `json:"amount_yuan"`
		Currency        string  `json:"currency"`
		BaseCurrency    string  `json:"base_currency"`
		BaseAmountCents int64   `json:"base_amount_cents"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	mode := model.Mode(req.Mode)
	if req.Mode != "" {
		parsed, modeOK := parseMode(req.Mode)
		if !modeOK {
			fail(c, 400, 40001, "invalid mode")
			return
		}
		mode = parsed
	}
	periodMonth := req.PeriodMonth
	if periodMonth == "" {
		periodMonth = req.Period
	}
	amountCents, err := budgetAmountCents(req.AmountCents, req.AmountYuan)
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "invalid_amount", "Please enter a valid amount.")
		return
	}
	budget, err := s.budgetSvc.UpdateBudget(c.Request.Context(), service.UpdateBudgetRequest{
		ID:              c.Param("id"),
		UserID:          userID(c),
		Mode:            mode,
		PeriodMonth:     periodMonth,
		Category:        req.Category,
		AmountCents:     amountCents,
		Currency:        req.Currency,
		BaseCurrency:    req.BaseCurrency,
		BaseAmountCents: req.BaseAmountCents,
	})
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	ok(c, budgetToDTO(budget))
}

func (s *Server) handleDeleteBudget(c *gin.Context) {
	if err := s.budgetSvc.DeleteBudget(c.Request.Context(), userID(c), c.Param("id")); err != nil {
		fail(c, 400, 40001, err.Error())
		return
	}
	ok(c, gin.H{"id": c.Param("id"), "deleted": true})
}

func (s *Server) handleBudgetSummary(c *gin.Context) {
	mode, modeOK := parseMode(c.Query("mode"))
	if !modeOK {
		fail(c, 400, 40001, "invalid mode")
		return
	}
	summary, err := s.budgetSvc.Summary(c.Request.Context(), userID(c), mode, parseBudgetMonth(c))
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	categoryBudgets := make([]budgetProgressDTO, 0, len(summary.CategoryBudgets))
	for _, p := range summary.CategoryBudgets {
		categoryBudgets = append(categoryBudgets, budgetProgressToDTO(p))
	}
	var totalBudget *budgetProgressDTO
	if summary.TotalBudget != nil {
		dto := budgetProgressToDTO(*summary.TotalBudget)
		totalBudget = &dto
	}
	ok(c, gin.H{
		"mode":               string(summary.Mode),
		"period_month":       summary.PeriodMonth,
		"total_actual_cents": summary.TotalActualCents,
		"total_actual_yuan":  float64(summary.TotalActualCents) / 100,
		"total_budget":       totalBudget,
		"category_budgets":   categoryBudgets,
	})
}

func (s *Server) handleLitestreamHealth(c *gin.Context) {
	statusPath := os.Getenv("LITESTREAM_STATUS_FILE")
	if statusPath == "" {
		statusPath = "/data/litestream_status.json"
	}
	type litestreamStatus struct {
		Status                string `json:"status"`
		CheckedAt             string `json:"checked_at"`
		LastSnapshotAt        string `json:"last_snapshot_at"`
		ReplicationLagSeconds int64  `json:"replication_lag_seconds"`
		Error                 string `json:"error"`
	}
	st := litestreamStatus{Status: "unknown", ReplicationLagSeconds: -1}
	if b, err := os.ReadFile(statusPath); err == nil {
		_ = json.Unmarshal(b, &st)
	} else {
		st.Error = "status_file_unavailable"
	}
	var journalMode string
	_ = s.db.QueryRowContext(c.Request.Context(), `PRAGMA journal_mode`).Scan(&journalMode)
	ok(c, gin.H{"litestream": st, "journal_mode": strings.ToLower(journalMode), "status_file": statusPath})
}

type disasterSnapshotMetadata struct {
	SnapshotID    string `json:"snapshot_id"`
	CreatedAt     string `json:"created_at"`
	SchemaVersion int    `json:"schema_version"`
	AppVersion    string `json:"app_version"`
	Environment   string `json:"environment"`
	DBSize        int64  `json:"db_size"`
	HasMetadata   bool   `json:"has_metadata"`
}

type disasterRecoveryResult struct {
	RecoveryID       string
	SnapshotID       string
	SchemaBefore     int
	SchemaAfter      int
	MigrationApplied bool
	Duration         time.Duration
	Success          bool
	Error            string
}

type restoreSource string

const (
	restoreSourceUserBackup      restoreSource = "USER_BACKUP"
	restoreSourceDisasterRecover restoreSource = "DISASTER_RECOVERY"
)

type restoreEngineRequest struct {
	RestoreID            string
	Source               restoreSource
	SnapshotID           string
	TempDBPath           string
	UploadedVersion      int
	SchemaBefore         int
	AttachmentRestoreDir string
}

type restoreEngineResult struct {
	RestoreID        string
	Source           restoreSource
	SnapshotID       string
	SchemaBefore     int
	SchemaAfter      int
	MigrationApplied bool
	RestoreDuration  time.Duration
}

func (s *Server) loadDisasterSnapshots(ctx context.Context) ([]disasterSnapshotMetadata, error) {
	if metadataPath := strings.TrimSpace(os.Getenv("DISASTER_SNAPSHOT_METADATA_PATH")); metadataPath != "" {
		return loadDisasterSnapshotMetadataFile(metadataPath)
	}
	return s.loadLitestreamSnapshots(ctx)
}

func loadDisasterSnapshotMetadataFile(path string) ([]disasterSnapshotMetadata, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snapshots []disasterSnapshotMetadata
	if err := json.Unmarshal(b, &snapshots); err != nil {
		return nil, err
	}
	for i := range snapshots {
		snapshots[i].HasMetadata = snapshots[i].SnapshotID != "" && snapshots[i].CreatedAt != "" && snapshots[i].SchemaVersion > 0
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].CreatedAt > snapshots[j].CreatedAt })
	return snapshots, nil
}

const maxLitestreamSnapshotsOutputBytes = 1 << 20

type boundedCommandBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (b *boundedCommandBuffer) Write(p []byte) (int, error) {
	originalLength := len(p)
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.overflow = b.overflow || originalLength > 0
		return originalLength, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
		b.overflow = true
	}
	_, _ = b.buffer.Write(p)
	return originalLength, nil
}

func (s *Server) loadLitestreamSnapshots(ctx context.Context) ([]disasterSnapshotMetadata, error) {
	databasePath := strings.TrimSpace(s.dbPath)
	if databasePath == "" {
		databasePath = strings.TrimSpace(os.Getenv("FINARCH_DB"))
	}
	if databasePath == "" {
		databasePath = "/data/finarch.db"
	}
	if databasePath == ":memory:" || strings.HasPrefix(strings.ToLower(databasePath), "file:") || strings.Contains(databasePath, "?") {
		return nil, fmt.Errorf("litestream snapshots requires a configured filesystem database path")
	}

	commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "litestream", "snapshots", "-config", "/etc/litestream.yml", "-replica", "r2", databasePath)
	stdout := &boundedCommandBuffer{limit: maxLitestreamSnapshotsOutputBytes}
	stderr := &boundedCommandBuffer{limit: maxLitestreamSnapshotsOutputBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("list litestream snapshots: %w (%s)", err, strings.TrimSpace(stderr.buffer.String()))
	}
	if stdout.overflow || stderr.overflow {
		return nil, fmt.Errorf("litestream snapshots output exceeded %d bytes", maxLitestreamSnapshotsOutputBytes)
	}
	if message := strings.TrimSpace(stderr.buffer.String()); message != "" {
		return nil, fmt.Errorf("litestream snapshots reported an error: %s", message)
	}
	return parseLitestreamSnapshots(stdout.buffer.String())
}

func parseLitestreamSnapshots(output string) ([]disasterSnapshotMetadata, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return nil, fmt.Errorf("litestream snapshots returned no header")
	}
	header := strings.Fields(lines[0])
	wantHeader := []string{"replica", "generation", "index", "size", "created"}
	if len(header) != len(wantHeader) {
		return nil, fmt.Errorf("unexpected litestream snapshots header")
	}
	for i := range wantHeader {
		if !strings.EqualFold(header[i], wantHeader[i]) {
			return nil, fmt.Errorf("unexpected litestream snapshots header")
		}
	}

	snapshots := make([]disasterSnapshotMetadata, 0, len(lines)-1)
	seenTimestamps := make(map[string]struct{}, len(lines)-1)
	for lineNumber, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 5 || fields[0] != "r2" || strings.TrimSpace(fields[1]) == "" {
			return nil, fmt.Errorf("malformed litestream snapshot on line %d", lineNumber+2)
		}
		index, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || index < 0 {
			return nil, fmt.Errorf("invalid litestream snapshot index on line %d", lineNumber+2)
		}
		size, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil || size < 0 {
			return nil, fmt.Errorf("invalid litestream snapshot size on line %d", lineNumber+2)
		}
		createdAt, err := time.Parse(time.RFC3339, fields[4])
		if err != nil {
			return nil, fmt.Errorf("invalid litestream snapshot timestamp on line %d: %w", lineNumber+2, err)
		}
		snapshotID := createdAt.UTC().Format(time.RFC3339)
		if _, duplicate := seenTimestamps[snapshotID]; duplicate {
			continue
		}
		seenTimestamps[snapshotID] = struct{}{}
		snapshots = append(snapshots, disasterSnapshotMetadata{
			SnapshotID:  snapshotID,
			CreatedAt:   snapshotID,
			DBSize:      size,
			HasMetadata: false,
		})
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].CreatedAt > snapshots[j].CreatedAt })
	return snapshots, nil
}

func (s *Server) handleDisasterRecoverySnapshots(c *gin.Context) {
	snapshots, err := s.loadDisasterSnapshots(c.Request.Context())
	if err != nil {
		fail(c, 500, "SNAPSHOT_LIST_FAILED", "无法读取灾备快照列表")
		return
	}
	ok(c, snapshots)
}

func (s *Server) handleDisasterRecoveryAuthorize(c *gin.Context) {
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	token, err := s.authSvc.RequestDisasterRecovery(c.Request.Context(), userID(c), req.CurrentPassword)
	if err != nil {
		mapAuthError(c, err, 40060)
		return
	}
	ok(c, gin.H{"token": token, "expires_in": 300})
}

func (s *Server) handleDisasterRecoveryExecute(c *gin.Context) {
	var req struct {
		SnapshotID           string `json:"snapshot_id" binding:"required"`
		Confirm              bool   `json:"confirm"`
		AllowMissingMetadata bool   `json:"allow_missing_metadata"`
		ApplyMode            string `json:"apply_mode"`    // replace | merge
		RestoreScope         string `json:"restore_scope"` // both | work | life
		AuthorizationToken   string `json:"authorization_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, "INVALID_INPUT", "请提供 snapshot_id")
		return
	}
	if !req.Confirm {
		fail(c, 400, "CONFIRM_REQUIRED", "请确认后再执行灾难恢复")
		return
	}
	applyMode := strings.ToLower(strings.TrimSpace(req.ApplyMode))
	if applyMode == "" {
		applyMode = "replace"
	}
	if applyMode != "replace" && applyMode != "merge" {
		fail(c, 422, "INVALID_INPUT", "apply_mode must be replace or merge")
		return
	}
	restoreScope := normalizeRestoreScope(req.RestoreScope)
	if restoreScope == "" {
		fail(c, 422, "INVALID_INPUT", "restore_scope must be work, life, or both")
		return
	}
	snapshots, err := s.loadDisasterSnapshots(c.Request.Context())
	if err != nil {
		fail(c, 500, "SNAPSHOT_LIST_FAILED", "无法读取灾备快照列表")
		return
	}
	var selected *disasterSnapshotMetadata
	for i := range snapshots {
		if snapshots[i].SnapshotID == req.SnapshotID {
			selected = &snapshots[i]
			break
		}
	}
	if selected == nil {
		fail(c, 404, "SNAPSHOT_NOT_FOUND", "未找到指定快照")
		return
	}
	if !selected.HasMetadata && !req.AllowMissingMetadata {
		fail(c, 409, "SNAPSHOT_METADATA_MISSING", "快照缺少元数据，请确认风险后重试")
		return
	}
	appEnv := strings.TrimSpace(os.Getenv("APP_ENV"))
	if appEnv == "" {
		appEnv = "production"
	}
	if selected.Environment != "" && selected.Environment != appEnv {
		fail(c, 409, "SNAPSHOT_ENV_MISMATCH", fmt.Sprintf("快照环境(%s)与当前环境(%s)不一致", selected.Environment, appEnv))
		return
	}
	var schemaBefore int
	_ = s.db.QueryRowContext(c.Request.Context(), `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&schemaBefore)
	if selected.SchemaVersion > schemaBefore {
		fail(c, 409, "SNAPSHOT_SCHEMA_TOO_NEW", fmt.Sprintf("快照版本(v%d)高于当前系统(v%d)", selected.SchemaVersion, schemaBefore))
		return
	}

	authReq, err := s.authSvc.VerifyDisasterRecoveryToken(c.Request.Context(), userID(c), req.AuthorizationToken)
	if err != nil {
		mapAuthError(c, err, 40061)
		return
	}
	if err := s.authSvc.ReserveDisasterRecoveryToken(c.Request.Context(), authReq); err != nil {
		mapAuthError(c, err, 40062)
		return
	}

	result := s.executeDisasterRecovery(c.Request.Context(), selected.SnapshotID, schemaBefore, applyMode, restoreScope)
	if !result.Success {
		fail(c, 500, "DISASTER_RECOVERY_FAILED", result.Error)
		return
	}
	auditCtx, cancelAudit := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
	s.authSvc.RecordDisasterRecoveryExecuted(auditCtx, authReq.UserID)
	cancelAudit()
	ok(c, gin.H{
		"message":           "灾难恢复成功",
		"recovery_id":       result.RecoveryID,
		"snapshot_id":       result.SnapshotID,
		"schema_before":     result.SchemaBefore,
		"schema_after":      result.SchemaAfter,
		"migration_applied": result.MigrationApplied,
		"duration_ms":       result.Duration.Milliseconds(),
		"apply_mode":        applyMode,
		"restore_scope":     restoreScope,
	})
}

func (s *Server) executeDisasterRecovery(ctx context.Context, snapshotID string, schemaBefore int, applyMode string, restoreScope string) (result disasterRecoveryResult) {
	start := time.Now()
	recoveryID := uuid.NewString()
	result = disasterRecoveryResult{RecoveryID: recoveryID, SnapshotID: snapshotID, SchemaBefore: schemaBefore}
	defer func() {
		result.Duration = time.Since(start)
		if result.Success {
			log.Printf("DISASTER_RECOVERY_COMPLETE recovery_id=%s snapshot=%s schema_before=%d schema_after=%d migration=%t mode=%s scope=%s duration=%s", result.RecoveryID, result.SnapshotID, result.SchemaBefore, result.SchemaAfter, result.MigrationApplied, applyMode, restoreScope, result.Duration)
		} else {
			log.Printf("DISASTER_RECOVERY_FAILED recovery_id=%s snapshot=%s schema_before=%d mode=%s scope=%s error=%q duration=%s", result.RecoveryID, result.SnapshotID, result.SchemaBefore, applyMode, restoreScope, result.Error, result.Duration)
		}
	}()

	tmpRestoreDir, err := os.MkdirTemp("", "finarch-r2-restore-*")
	if err != nil {
		result.Error = "无法创建临时恢复目录"
		return result
	}
	defer func() {
		if err := os.RemoveAll(tmpRestoreDir); err != nil {
			log.Printf("DISASTER_RECOVERY_TEMP_CLEANUP_FAILED recovery_id=%s error=%q", recoveryID, err)
		}
	}()
	// Litestream requires the output itself not to exist. A private temporary
	// directory gives it an unpredictable, absent path without a remove/recreate
	// race on a pre-created placeholder.
	tmpRestorePath := filepath.Join(tmpRestoreDir, "restored.db")

	args := []string{"restore", "-config", "/etc/litestream.yml"}
	if strings.TrimSpace(snapshotID) != "" {
		args = append(args, "-timestamp", snapshotID)
	}
	args = append(args, tmpRestorePath)
	cmd := exec.CommandContext(ctx, "litestream", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		result.Error = fmt.Sprintf("拉取快照失败: %v (%s)", err, strings.TrimSpace(string(out)))
		return result
	}
	restoredInfo, err := os.Lstat(tmpRestorePath)
	if err != nil || !restoredInfo.Mode().IsRegular() || restoredInfo.Size() == 0 {
		result.Error = "灾备快照不是有效的普通文件"
		return result
	}
	if err := os.Chmod(tmpRestorePath, 0o600); err != nil {
		result.Error = "无法保护灾备快照文件权限"
		return result
	}

	srcDB, err := openExistingSQLiteReadOnly(ctx, tmpRestorePath)
	if err != nil {
		result.Error = "无法打开灾备快照"
		return result
	}
	if err := ensureSQLiteIntegrity(ctx, srcDB); err != nil {
		srcDB.Close()
		result.Error = fmt.Sprintf("灾备快照完整性校验失败: %s", err.Error())
		return result
	}
	var uploadedVersion int
	_ = srcDB.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&uploadedVersion)
	identity, err := readBackupIdentity(srcDB)
	if err != nil {
		srcDB.Close()
		result.Error = "无法识别灾备快照所有者"
		return result
	}
	var sourceHasAttachmentsTable int
	if err := srcDB.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM sqlite_master
		WHERE type = 'table' AND name = 'attachments'
	`).Scan(&sourceHasAttachmentsTable); err != nil {
		srcDB.Close()
		result.Error = "无法核验灾备快照附件"
		return result
	}
	var sourceAttachmentCount int
	if sourceHasAttachmentsTable != 0 {
		if err := srcDB.QueryRowContext(ctx, `SELECT COUNT(1) FROM attachments`).Scan(&sourceAttachmentCount); err != nil {
			srcDB.Close()
			result.Error = "无法核验灾备快照附件"
			return result
		}
	}
	if err := srcDB.Close(); err != nil {
		result.Error = "无法关闭灾备快照"
		return result
	}
	backupUserID := strings.TrimSpace(identity.MetadataUserID)
	if backupUserID == "" {
		backupUserID = strings.TrimSpace(identity.FallbackOwnerID)
	}

	if applyMode == "merge" {
		// Litestream replicates only SQLite. Merge restore has no attachment
		// archive from which it can re-key and copy source objects, so silently
		// importing the financial rows while dropping attachments would be a
		// false successful recovery. Replacement mode performs its own strict
		// size/hash preflight against independently restored attachment storage.
		if sourceAttachmentCount > 0 {
			result.Error = "灾备快照包含附件，但 R2 不包含附件文件，无法安全执行合并恢复"
			return result
		}
		targetUserID, err := primaryUserID(ctx, s.db)
		if err != nil || targetUserID == "" {
			result.Error = "无法识别当前数据库用户，无法执行合并恢复"
			return result
		}
		migratedTo, err := s.performRestoreWithMerge(ctx, tmpRestorePath, uploadedVersion, schemaBefore, restoreContext{
			RequesterUserID: targetUserID,
			BackupUserID:    backupUserID,
			RestoreMode:     "CROSS_ACCOUNT",
			DataScope:       restoreScope,
		})
		if err != nil {
			result.Error = err.Error()
			return result
		}
		result.SchemaAfter = migratedTo
		result.MigrationApplied = migratedTo > schemaBefore
		result.Success = true
		return result
	}

	engineResult, err := s.executeRestoreEngine(ctx, restoreEngineRequest{
		RestoreID:       recoveryID,
		Source:          restoreSourceDisasterRecover,
		SnapshotID:      snapshotID,
		TempDBPath:      tmpRestorePath,
		UploadedVersion: uploadedVersion,
		SchemaBefore:    schemaBefore,
	})
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.SchemaAfter = engineResult.SchemaAfter
	result.MigrationApplied = engineResult.MigrationApplied
	result.Success = true
	return result
}

func (s *Server) validatePostRecovery(ctx context.Context) error {
	required := []string{"users", "transactions", "accounts", "categories", "schema_migrations"}
	for _, t := range required {
		var c int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name=?`, t).Scan(&c); err != nil {
			return fmt.Errorf("恢复后校验失败：无法读取表 %s", t)
		}
		if c == 0 {
			return fmt.Errorf("恢复后校验失败：缺少表 %s", t)
		}
	}
	var users int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&users); err != nil {
		return fmt.Errorf("恢复后校验失败：无法访问 users 表")
	}
	if users == 0 {
		return fmt.Errorf("恢复后校验失败：无用户数据")
	}
	identity, err := readBackupIdentity(s.db)
	if err != nil {
		return fmt.Errorf("恢复后身份校验失败: %w", err)
	}
	if identity.MetadataPresent {
		metadataUserID := strings.TrimSpace(identity.MetadataUserID)
		metadataEmail := normalizeEmail(identity.MetadataUserEmail)
		if metadataUserID == "" && metadataEmail == "" {
			return fmt.Errorf("恢复后身份校验失败：备份元数据缺少所有者")
		}
		var matchingOwners int
		if err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(1)
			FROM users
			WHERE deleted_at IS NULL
			  AND ((? <> '' AND id = ?) OR (? <> '' AND lower(email) = ?))
		`, metadataUserID, metadataUserID, metadataEmail, metadataEmail).Scan(&matchingOwners); err != nil {
			return fmt.Errorf("恢复后身份校验失败：无法核对备份所有者")
		}
		if matchingOwners == 0 {
			return fmt.Errorf("恢复后身份校验失败：备份所有者不存在")
		}
	}
	if err := ensureSQLiteIntegrity(ctx, s.db); err != nil {
		return fmt.Errorf("恢复后完整性校验失败: %w", err)
	}
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("恢复后健康检查失败")
	}
	return nil
}

func removeSQLiteFiles(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	baseErr := os.Remove(path)
	if errors.Is(baseErr, os.ErrNotExist) {
		baseErr = nil
	}
	return errors.Join(baseErr, removeSQLiteSidecars(path))
}

func removeSQLiteSidecars(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	var cleanupErrors []error
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		candidate := path + suffix
		if err := os.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove %s: %w", filepath.Base(candidate), err))
		}
	}
	return errors.Join(cleanupErrors...)
}

// copySQLiteDatabase copies a complete SQLite image using short-lived raw
// connections. Both connections are returned before callers run migrations or
// validations, which is essential for deployments with a one-connection pool.
func copySQLiteDatabase(ctx context.Context, destination, source *sql.DB) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	srcConn, err := source.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire source connection: %w", err)
	}
	destConn, err := destination.Conn(ctx)
	if err != nil {
		srcConn.Close()
		return fmt.Errorf("acquire destination connection: %w", err)
	}

	copyErr := srcConn.Raw(func(srcRaw interface{}) error {
		srcSQLite, ok := srcRaw.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("unexpected source SQLite connection type")
		}
		return destConn.Raw(func(destRaw interface{}) error {
			destSQLite, ok := destRaw.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("unexpected destination SQLite connection type")
			}
			backup, err := destSQLite.Backup("main", srcSQLite, "main")
			if err != nil {
				return fmt.Errorf("start SQLite backup: %w", err)
			}
			done, stepErr := backup.Step(-1)
			finishErr := backup.Finish()
			if stepErr != nil || finishErr != nil {
				return errors.Join(stepErr, finishErr)
			}
			if !done {
				return fmt.Errorf("SQLite backup did not complete")
			}
			return nil
		})
	})
	destCloseErr := destConn.Close()
	srcCloseErr := srcConn.Close()
	if copyErr != nil || destCloseErr != nil || srcCloseErr != nil {
		return errors.Join(copyErr, destCloseErr, srcCloseErr)
	}
	return nil
}

func (s *Server) prepareSafetyBackup(ctx context.Context, prefix string, plannedPaths ...string) (safetyPath string, returnErr error) {
	if strings.TrimSpace(s.dbPath) == "" {
		return "", fmt.Errorf("live database path is unavailable")
	}
	safetyDir, err := restoreSafetyArtifactDirectory(s.dbPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(safetyDir, 0o700); err != nil {
		return "", fmt.Errorf("create safety backup directory: %w", err)
	}
	if err := validateRestoreArtifactDirectory(safetyDir); err != nil {
		return "", fmt.Errorf("validate safety backup directory: %w", err)
	}
	if err := os.Chmod(safetyDir, 0o700); err != nil {
		return "", fmt.Errorf("secure safety backup directory: %w", pathSafeFilesystemError(err))
	}
	if err := validatePrivateRestoreArtifactDirectory(safetyDir); err != nil {
		return "", fmt.Errorf("validate private safety backup directory: %w", err)
	}
	if err := syncRestoreArtifactDirectory(safetyDir); err != nil {
		return "", fmt.Errorf("sync safety backup directory: %w", err)
	}
	if err := syncRestoreArtifactDirectory(filepath.Dir(safetyDir)); err != nil {
		return "", fmt.Errorf("sync safety backup parent directory: %w", err)
	}
	if len(plannedPaths) > 1 {
		return "", fmt.Errorf("multiple planned safety backup paths")
	}
	var tmp *os.File
	if len(plannedPaths) == 1 {
		plannedPath, err := filepath.Abs(plannedPaths[0])
		if err != nil {
			return "", fmt.Errorf("resolve planned safety backup: %w", err)
		}
		if filepath.Dir(plannedPath) != safetyDir || !safeRestoreArtifactBase(filepath.Base(plannedPath)) {
			return "", fmt.Errorf("planned safety backup is outside the restore artifact directory")
		}
		tmp, err = os.OpenFile(plannedPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	} else {
		tmp, err = os.CreateTemp(safetyDir, prefix+"-*.db")
	}
	if err != nil {
		return "", fmt.Errorf("create safety backup file: %w", err)
	}
	safetyPath = tmp.Name()
	cleanupPath := safetyPath
	if err := tmp.Close(); err != nil {
		_ = removeSQLiteFiles(cleanupPath)
		return "", fmt.Errorf("close safety backup file: %w", err)
	}
	defer func() {
		if returnErr != nil {
			_ = removeSQLiteFiles(cleanupPath)
		}
	}()

	safetyDB, err := sql.Open("sqlite3", safetyPath)
	if err != nil {
		return "", fmt.Errorf("open safety backup: %w", err)
	}
	safetyDB.SetMaxOpenConns(1)
	if err := copySQLiteDatabase(ctx, safetyDB, s.db); err != nil {
		_ = safetyDB.Close()
		return "", fmt.Errorf("copy live database into safety backup: %w", err)
	}
	if _, err := safetyDB.ExecContext(ctx, `PRAGMA journal_mode = DELETE`); err != nil {
		_ = safetyDB.Close()
		return "", fmt.Errorf("make safety backup self-contained: %w", err)
	}
	if err := ensureSQLiteIntegrity(ctx, safetyDB); err != nil {
		_ = safetyDB.Close()
		return "", fmt.Errorf("validate safety backup: %w", err)
	}
	if err := safetyDB.Close(); err != nil {
		return "", fmt.Errorf("close safety backup database: %w", err)
	}
	safetyFile, err := os.Open(safetyPath)
	if err != nil {
		return "", fmt.Errorf("open safety backup for sync: %w", err)
	}
	syncErr := safetyFile.Sync()
	closeErr := safetyFile.Close()
	if syncErr != nil || closeErr != nil {
		return "", fmt.Errorf("sync safety backup: %w", errors.Join(syncErr, closeErr))
	}
	if err := syncRestoreArtifactDirectory(safetyDir); err != nil {
		return "", fmt.Errorf("sync published safety backup: %w", err)
	}
	return safetyPath, nil
}

func openExistingSQLiteReadOnly(ctx context.Context, path string) (*sql.DB, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, fmt.Errorf("SQLite backup is not a non-empty regular file")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: absolutePath}
	query := uri.Query()
	query.Set("mode", "ro")
	query.Set("_query_only", "1")
	uri.RawQuery = query.Encode()
	database, err := sql.Open("sqlite3", uri.String())
	if err != nil {
		return nil, err
	}
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}

func (s *Server) rollbackRestore(ctx context.Context, safetyPath string) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	safetyDB, err := openExistingSQLiteReadOnly(rollbackCtx, safetyPath)
	if err != nil {
		pragmaErr := findb.ReapplyPragmas(rollbackCtx, s.db)
		return errors.Join(fmt.Errorf("open safety backup %s: %w", filepath.Base(safetyPath), pathSafeFilesystemError(err)), pragmaErr)
	}
	defer safetyDB.Close()
	if err := ensureSQLiteIntegrity(rollbackCtx, safetyDB); err != nil {
		pragmaErr := findb.ReapplyPragmas(rollbackCtx, s.db)
		return errors.Join(fmt.Errorf("validate safety backup before rollback: %w", err), pragmaErr)
	}

	copyErr := copySQLiteDatabase(rollbackCtx, s.db, safetyDB)
	pragmaErr := findb.ReapplyPragmas(rollbackCtx, s.db)
	var integrityErr error
	if copyErr == nil {
		if err := ensureSQLiteIntegrity(rollbackCtx, s.db); err != nil {
			integrityErr = fmt.Errorf("validate rolled-back database: %w", err)
		}
	}
	return errors.Join(copyErr, pragmaErr, integrityErr)
}

func (s *Server) failRestoreAndRollback(ctx context.Context, safetyPath string, restoreErr error, journals ...*attachmentRollbackJournal) error {
	databaseRollbackErr := runRestoreCompensation("database rollback", func() error {
		return s.rollbackRestore(ctx, safetyPath)
	})
	var attachmentRollbackErr error
	if len(journals) > 0 && journals[0] != nil {
		attachmentRollbackErr = runRestoreCompensation("attachment rollback", func() error {
			return journals[0].rollback(ctx)
		})
	}
	rollbackErr := errors.Join(
		wrapRestoreError("数据库回滚错误", databaseRollbackErr),
		wrapRestoreError("附件回滚错误", attachmentRollbackErr),
	)
	if rollbackErr != nil {
		log.Printf("RESTORE_ROLLBACK_FAILED original_error=%q rollback_error=%q", restoreErr, rollbackErr)
		return &restoreRollbackFailure{restoreErr: restoreErr, rollbackErr: rollbackErr}
	}
	log.Printf("RESTORE_ROLLBACK_COMPLETE original_error=%q", restoreErr)
	return fmt.Errorf("%w；原数据库已自动恢复", restoreErr)
}

func runRestoreCompensation(operation string, run func() error) (returnErr error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			returnErr = fmt.Errorf("%s panic: %v", operation, recovered)
		}
	}()
	return run()
}

func restorePanicError(scope string, recovered any) error {
	if err, ok := recovered.(error); ok {
		return fmt.Errorf("%s发生内部异常: %w", scope, err)
	}
	return fmt.Errorf("%s发生内部异常: %v", scope, recovered)
}

func wrapRestoreError(label string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", label, err)
}

type restoreRollbackFailure struct {
	restoreErr                 error
	rollbackErr                error
	retainedArtifact           string
	retainedAttachmentArtifact string
	retentionErr               error
	attachmentRetentionErr     error
}

func (e *restoreRollbackFailure) Error() string {
	parts := []string{
		fmt.Sprintf("原始恢复错误: %v", e.restoreErr),
		fmt.Sprintf("自动回滚错误: %v", e.rollbackErr),
	}
	if e.retainedArtifact != "" {
		parts = append(parts, fmt.Sprintf("安全备份已保留为 %s", e.retainedArtifact))
	}
	if e.retainedAttachmentArtifact != "" {
		parts = append(parts, fmt.Sprintf("附件回滚日志已保留为 %s", e.retainedAttachmentArtifact))
	}
	if e.retentionErr != nil {
		parts = append(parts, fmt.Sprintf("安全备份保留操作异常: %v", e.retentionErr))
	}
	if e.attachmentRetentionErr != nil {
		parts = append(parts, fmt.Sprintf("附件回滚日志保留操作异常: %v", e.attachmentRetentionErr))
	}
	parts = append(parts, "系统保持维护状态，需人工恢复")
	return strings.Join(parts, "; ")
}

func (e *restoreRollbackFailure) Unwrap() []error {
	causes := []error{e.restoreErr, e.rollbackErr}
	if e.retentionErr != nil {
		causes = append(causes, e.retentionErr)
	}
	if e.attachmentRetentionErr != nil {
		causes = append(causes, e.attachmentRetentionErr)
	}
	return causes
}

func pathSafeFilesystemError(err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return linkErr.Err
	}
	return err
}

func safeRestoreArtifactID(restoreID string) string {
	trimmed := strings.TrimSpace(restoreID)
	if trimmed == "" {
		return "unknown"
	}
	if len(trimmed) <= 64 {
		valid := true
		for i := 0; i < len(trimmed); i++ {
			ch := trimmed[i]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
				continue
			}
			valid = false
			break
		}
		if valid {
			return trimmed
		}
	}
	digest := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(digest[:8])
}

func retainFailedSafetyBackup(safetyPath, restoreID string) (string, error) {
	if err := os.Chmod(safetyPath, 0o600); err != nil {
		return filepath.Base(safetyPath), errors.Join(
			fmt.Errorf("secure safety backup permissions: %w", pathSafeFilesystemError(err)),
			removeSQLiteSidecars(safetyPath),
		)
	}
	stablePrefix := fmt.Sprintf("failed-restore-%s-", safeRestoreArtifactID(restoreID))
	if base := filepath.Base(safetyPath); (strings.HasPrefix(base, stablePrefix) || strings.HasPrefix(base, restoreSafetyFilePrefix)) && strings.HasSuffix(base, ".db") {
		return base, errors.Join(removeSQLiteSidecars(safetyPath), syncRestoreArtifactDirectory(filepath.Dir(safetyPath)))
	}
	retainedName := fmt.Sprintf("%s%s.db", stablePrefix, uuid.NewString())
	retainedPath := filepath.Join(filepath.Dir(safetyPath), retainedName)
	if err := os.Rename(safetyPath, retainedPath); err != nil {
		return filepath.Base(safetyPath), errors.Join(
			fmt.Errorf("retain safety backup: %w", pathSafeFilesystemError(err)),
			removeSQLiteSidecars(safetyPath),
		)
	}
	permissionErr := pathSafeFilesystemError(os.Chmod(retainedPath, 0o600))
	sidecarErr := errors.Join(removeSQLiteSidecars(safetyPath), removeSQLiteSidecars(retainedPath))
	directoryErr := syncRestoreArtifactDirectory(filepath.Dir(retainedPath))
	return retainedName, errors.Join(permissionErr, sidecarErr, directoryErr)
}

func (s *Server) executeRestoreEngine(ctx context.Context, req restoreEngineRequest) (result restoreEngineResult, returnErr error) {
	start := time.Now()
	result = restoreEngineResult{RestoreID: req.RestoreID, Source: req.Source, SnapshotID: req.SnapshotID, SchemaBefore: req.SchemaBefore}
	defer func() { result.RestoreDuration = time.Since(start) }()

	if strings.TrimSpace(req.TempDBPath) == "" {
		return result, fmt.Errorf("恢复源无效：缺少临时数据库")
	}
	restoreSrcDB, err := openExistingSQLiteReadOnly(ctx, req.TempDBPath)
	if err != nil {
		return result, fmt.Errorf("无法打开备份文件")
	}
	defer restoreSrcDB.Close()
	if err := ensureSQLiteIntegrity(ctx, restoreSrcDB); err != nil {
		return result, fmt.Errorf("备份完整性校验失败: %s", err.Error())
	}
	var sourceVersion int
	if err := restoreSrcDB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&sourceVersion); err != nil {
		return result, fmt.Errorf("备份版本校验失败")
	}
	if sourceVersion != req.UploadedVersion {
		return result, fmt.Errorf("备份版本校验失败：检查时为 v%d，恢复时为 v%d", req.UploadedVersion, sourceVersion)
	}
	if sourceVersion > req.SchemaBefore {
		return result, fmt.Errorf("备份版本 v%d 高于当前数据库版本 v%d", sourceVersion, req.SchemaBefore)
	}
	if err := validateRestoreTransactionAmounts(ctx, restoreSrcDB); err != nil {
		return result, fmt.Errorf("备份交易金额校验失败: %w", err)
	}
	var sourceAttachmentFiles []attachmentRestoreFile
	var sourceHasAttachmentsTable int
	if err := restoreSrcDB.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = 'attachments'
	`).Scan(&sourceHasAttachmentsTable); err != nil {
		return result, fmt.Errorf("备份附件校验失败：无法读取附件元数据")
	}
	if sourceHasAttachmentsTable != 0 {
		sourceAttachmentFiles, err = replacementAttachmentFiles(ctx, restoreSrcDB)
		if err != nil {
			return result, fmt.Errorf("备份附件校验失败：无法读取附件元数据")
		}
	}
	if len(sourceAttachmentFiles) > 0 {
		if strings.TrimSpace(req.AttachmentRestoreDir) != "" && s.attachmentSvc == nil {
			return result, fmt.Errorf("备份附件校验失败：附件存储不可用")
		}
	}

	guard := findb.Global()
	releaseRequestWriteLease(ctx)
	previousState := guard.BeginMaintenance(findb.StateRestore)
	defer func() {
		var rollbackFailure *restoreRollbackFailure
		if errors.As(returnErr, &rollbackFailure) {
			guard.EndMaintenance(findb.StateRestore)
			return
		}
		guard.EndMaintenance(previousState)
	}()
	// Raw database restores rely on attachment objects already present in live
	// storage. Validate them only after all admitted attachment writers/deleters
	// have drained and while the exclusive maintenance barrier prevents a
	// check/delete race, but still before any live database mutation or safety
	// artifact is created.
	if len(sourceAttachmentFiles) > 0 && strings.TrimSpace(req.AttachmentRestoreDir) == "" {
		if err := validateStoredAttachmentFiles(ctx, sourceAttachmentFiles, s.attachmentSvc); err != nil {
			return result, fmt.Errorf("备份附件校验失败: %w", err)
		}
	}

	var restoreOperation *durableRestoreOperation
	var attachmentJournal *attachmentRollbackJournal
	needsAttachmentJournal := len(sourceAttachmentFiles) > 0 && strings.TrimSpace(req.AttachmentRestoreDir) != "" && s.attachmentSvc != nil
	replacementCommitRecorded := false
	// Registered before artifact cleanup so the terminal marker is removed
	// last. A crash anywhere in cleanup can therefore be retried on startup.
	defer func() {
		if restoreOperation == nil || restoreOperation.markerPath == "" {
			return
		}
		var rollbackFailure *restoreRollbackFailure
		if errors.As(returnErr, &rollbackFailure) {
			return
		}
		if err := restoreOperation.removeMarker(); err != nil {
			returnErr = &restoreRollbackFailure{
				restoreErr:  errors.Join(returnErr, fmt.Errorf("清理终态恢复操作日志失败: %w", err)),
				rollbackErr: errors.New("终态操作日志删除状态不确定，系统保持维护状态并将在启动时重试"),
			}
			log.Printf("RESTORE_TERMINAL_CLEANUP_REQUIRED restore_id=%s error=%v", safeRestoreArtifactID(req.RestoreID), err)
		}
	}()
	restoreOperation, err = beginReplacementRestorePreparation(s.dbPath, needsAttachmentJournal)
	if err != nil {
		if restoreMarkerWritePublished(err) {
			return result, &restoreRollbackFailure{
				restoreErr:  fmt.Errorf("创建准备阶段恢复操作日志后的目录同步失败: %w", err),
				rollbackErr: errors.New("操作日志发布状态不确定，数据库尚未修改；系统保持维护状态"),
			}
		}
		return result, fmt.Errorf("创建恢复安全备份失败：创建准备阶段恢复操作日志: %w", err)
	}
	safetyPath := filepath.Join(filepath.Dir(restoreOperation.markerPath), restoreOperation.marker.SafetyBackup)
	defer func() {
		var rollbackFailure *restoreRollbackFailure
		if errors.As(returnErr, &rollbackFailure) {
			retainedArtifact, err := retainFailedSafetyBackup(safetyPath, req.RestoreID)
			rollbackFailure.retainedArtifact = retainedArtifact
			rollbackFailure.retentionErr = err
			log.Printf("RESTORE_MANUAL_RECOVERY_REQUIRED restore_id=%s safety_artifact=%s retention_error=%v", safeRestoreArtifactID(req.RestoreID), retainedArtifact, err)
			return
		}
		if err := removeSQLiteFiles(safetyPath); err != nil {
			returnErr = &restoreRollbackFailure{
				restoreErr:  errors.Join(returnErr, fmt.Errorf("清理安全备份失败: %w", err)),
				rollbackErr: errors.New("终态操作日志已保留，启动恢复将重试工件清理"),
			}
			return
		}
		if err := syncRestoreArtifactDirectory(filepath.Dir(safetyPath)); err != nil {
			returnErr = &restoreRollbackFailure{
				restoreErr:  errors.Join(returnErr, fmt.Errorf("同步安全备份清理失败: %w", err)),
				rollbackErr: errors.New("终态操作日志已保留，启动恢复将重试工件清理"),
			}
		}
	}()
	safetyPath, err = s.prepareSafetyBackup(ctx, "failed-restore-"+safeRestoreArtifactID(req.RestoreID), safetyPath)
	if err != nil {
		return result, fmt.Errorf("创建恢复安全备份失败: %w", err)
	}
	defer func() {
		if attachmentJournal != nil {
			var rollbackFailure *restoreRollbackFailure
			if errors.As(returnErr, &rollbackFailure) {
				retainedArtifact, err := attachmentJournal.retain(req.RestoreID, restoreOperation)
				rollbackFailure.retainedAttachmentArtifact = retainedArtifact
				rollbackFailure.attachmentRetentionErr = err
				log.Printf("RESTORE_ATTACHMENT_MANUAL_RECOVERY_REQUIRED restore_id=%s attachment_artifact=%s retention_error=%v", safeRestoreArtifactID(req.RestoreID), retainedArtifact, err)
				return
			}
			if err := attachmentJournal.cleanupPreservingMarker(restoreOperation); err != nil {
				returnErr = &restoreRollbackFailure{
					restoreErr:  errors.Join(returnErr, fmt.Errorf("清理附件回滚日志失败: %w", err)),
					rollbackErr: errors.New("终态操作日志已保留，启动恢复将重试工件清理"),
				}
			}
		}
	}()
	if needsAttachmentJournal {
		journalRoot, err := restoreSafetyArtifactDirectory(s.dbPath)
		if err != nil {
			return result, fmt.Errorf("附件回滚目录无效: %w", err)
		}
		attachmentJournal, err = prepareAttachmentRollbackJournal(ctx, journalRoot, sourceAttachmentFiles, s.attachmentSvc, restoreOperation)
		if err != nil {
			return result, fmt.Errorf("附件回滚日志创建失败: %w", err)
		}
		if err := restoreOperation.attachJournal(attachmentJournal); err != nil {
			return result, fmt.Errorf("附件回滚日志发布失败: %w", err)
		}
	}
	if err := restoreOperation.markPrepared(); err != nil {
		return result, fmt.Errorf("持久化恢复准备完成状态失败: %w", err)
	}
	defer func() {
		if restoreOperation == nil || restoreOperation.markerPath == "" {
			return
		}
		var rollbackFailure *restoreRollbackFailure
		if errors.As(returnErr, &rollbackFailure) {
			return
		}
		if returnErr != nil {
			if err := restoreOperation.markRolledBack(); err != nil {
				markerErr := fmt.Errorf("持久化恢复回滚终态失败: %w", err)
				returnErr = &restoreRollbackFailure{
					restoreErr:  errors.Join(returnErr, markerErr),
					rollbackErr: errors.New("为避免丢失崩溃恢复依据，恢复工件已保留"),
				}
			}
			return
		}
		if restoreOperation.marker.Phase != restoreOperationPhaseCommitted {
			returnErr = &restoreRollbackFailure{
				restoreErr:  errors.New("恢复成功但缺少 durable committed 终态"),
				rollbackErr: errors.New("恢复工件已保留，系统保持维护状态"),
			}
		}
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := restorePanicError("数据恢复", recovered)
			if replacementCommitRecorded {
				returnErr = &restoreRollbackFailure{
					restoreErr:  panicErr,
					rollbackErr: errors.New("恢复提交账本已写入，无法再安全自动回滚；恢复工件已保留"),
				}
			} else {
				returnErr = s.failRestoreAndRollback(ctx, safetyPath, panicErr, attachmentJournal)
			}
			log.Printf("RESTORE_PANIC_COMPENSATED restore_id=%s result=%q", safeRestoreArtifactID(req.RestoreID), returnErr)
			panic(recovered)
		}
	}()

	previousAttachmentOwners, err := attachmentStorageOwners(ctx, s.db)
	if err != nil {
		return result, fmt.Errorf("记录恢复前附件失败: %w", err)
	}
	if err := copySQLiteDatabase(ctx, s.db, restoreSrcDB); err != nil {
		return result, s.failRestoreAndRollback(ctx, safetyPath, fmt.Errorf("数据恢复失败: %w", err))
	}

	if err := findb.ReapplyPragmas(ctx, s.db); err != nil {
		return result, s.failRestoreAndRollback(ctx, safetyPath, fmt.Errorf("恢复后重新应用数据库设置失败: %w", err))
	}
	if err := findb.MigrateWithinMaintenance(ctx, s.db); err != nil {
		return result, s.failRestoreAndRollback(ctx, safetyPath, fmt.Errorf("数据已恢复但自动迁移失败: %w", err))
	}
	if err := reconcileReplacementAttachmentDeletions(ctx, s.db, previousAttachmentOwners); err != nil {
		return result, s.failRestoreAndRollback(ctx, safetyPath, fmt.Errorf("恢复后附件清理对账失败: %w", err))
	}
	if err := findb.InvalidateRestoredAuthenticationState(ctx, s.db); err != nil {
		return result, s.failRestoreAndRollback(ctx, safetyPath, fmt.Errorf("恢复后凭证失效处理失败: %w", err))
	}
	if strings.TrimSpace(req.AttachmentRestoreDir) != "" && s.attachmentSvc != nil {
		attachmentFiles, err := replacementAttachmentFiles(ctx, s.db)
		if err != nil {
			return result, s.failRestoreAndRollback(ctx, safetyPath, fmt.Errorf("附件恢复失败: %w", err))
		}
		if !sameAttachmentRestoreFiles(attachmentFiles, sourceAttachmentFiles) {
			return result, s.failRestoreAndRollback(ctx, safetyPath, fmt.Errorf("附件恢复失败: 迁移后的附件清单与已持久化回滚日志不一致"), attachmentJournal)
		}
		if err := restoreAttachmentFiles(ctx, req.AttachmentRestoreDir, attachmentFiles, s.attachmentSvc); err != nil {
			return result, s.failRestoreAndRollback(ctx, safetyPath, fmt.Errorf("附件恢复失败: %w", err), attachmentJournal)
		}
	}
	if err := s.validatePostRecovery(ctx); err != nil {
		return result, s.failRestoreAndRollback(ctx, safetyPath, err, attachmentJournal)
	}

	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&result.SchemaAfter); err != nil {
		return result, s.failRestoreAndRollback(ctx, safetyPath, fmt.Errorf("恢复校验失败：无法读取 schema 版本"), attachmentJournal)
	}
	if result.SchemaAfter < req.UploadedVersion || result.SchemaAfter < result.SchemaBefore {
		return result, s.failRestoreAndRollback(ctx, safetyPath, fmt.Errorf(
			"恢复后 schema 版本异常：当前 v%d，备份 v%d，恢复前 v%d",
			result.SchemaAfter, req.UploadedVersion, result.SchemaBefore,
		), attachmentJournal)
	}
	result.MigrationApplied = result.SchemaAfter > result.SchemaBefore
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO restore_operations(operation_id, batch_id, kind, created_at)
		VALUES (?, ?, 'replace', ?)
	`, restoreOperation.marker.OperationID, restoreOperation.marker.OperationID, time.Now().UTC().Unix()); err != nil {
		return result, &restoreRollbackFailure{
			restoreErr:  fmt.Errorf("记录恢复提交账本失败: %w", err),
			rollbackErr: errors.New("数据库提交结果不确定，未自动回滚；恢复工件已保留"),
		}
	}
	replacementCommitRecorded = true
	if err := ensureRestoreDatabaseDurable(ctx, s.db, s.dbPath); err != nil {
		return result, &restoreRollbackFailure{
			restoreErr:  fmt.Errorf("持久化恢复数据库失败: %w", err),
			rollbackErr: errors.New("恢复提交账本已写入，未自动回滚；恢复工件已保留"),
		}
	}
	if err := restoreOperation.markCommitted(); err != nil {
		return result, &restoreRollbackFailure{
			restoreErr:  fmt.Errorf("恢复数据已持久化，但 committed 操作日志更新失败: %w", err),
			rollbackErr: errors.New("提交账本将用于启动恢复判定，未自动回滚；恢复工件已保留"),
		}
	}
	log.Printf("RESTORE_COMPLETE restore_id=%s restore_source=%s snapshot_id=%s schema_before=%d schema_after=%d migration=%t duration=%s", result.RestoreID, result.Source, result.SnapshotID, result.SchemaBefore, result.SchemaAfter, result.MigrationApplied, time.Since(start))
	return result, nil
}

func (s *Server) handleBackupExportRequest(c *gin.Context) {
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c, 40050)
		return
	}
	token, err := s.authSvc.RequestBackupExport(c.Request.Context(), userID(c), req.CurrentPassword)
	if err != nil {
		mapAuthError(c, err, 40051)
		return
	}
	ok(c, gin.H{"token": token, "message": "backup export authorized"})
}

// handleBackupDownload creates a consistent snapshot of the database using
// SQLite VACUUM INTO and streams it as a downloadable file.
func (s *Server) handleBackupDownload(c *gin.Context) {
	// Authorization tokens must never be placed in a URL: query strings are
	// routinely retained by access logs, proxies, monitoring, and history.
	exportToken := strings.TrimSpace(c.GetHeader(backupExportTokenHdr))
	if exportToken == "" {
		fail(c, 403, 40052, "not_authorized")
		return
	}
	if err := s.authSvc.ConsumeBackupExportToken(c.Request.Context(), userID(c), exportToken); err != nil {
		mapAuthError(c, err, 40053)
		return
	}

	// Token consumption is an ordinary write admitted by the request lease.
	// Upgrade to exclusive backup maintenance before snapshotting so the
	// database image and the attachment files are taken from one quiescent
	// logical state. Release the maintenance barrier once the temporary
	// artifact is complete; streaming that immutable artifact must not block
	// unrelated writes for the duration of a potentially slow download.
	releaseRequestWriteLease(c.Request.Context())
	guard := findb.Global()
	previousState := guard.BeginMaintenance(findb.StateBackup)
	var releaseBackupMaintenanceOnce sync.Once
	releaseBackupMaintenance := func() {
		releaseBackupMaintenanceOnce.Do(func() {
			guard.EndMaintenance(previousState)
		})
	}
	defer releaseBackupMaintenance()

	// VACUUM INTO refuses an existing output file. Use an absent path inside a
	// private directory rather than unlinking a predictable file in the shared
	// temporary directory, which would introduce a symlink replacement race.
	tmpDir, err := os.MkdirTemp("", "finarch-backup-*")
	if err != nil {
		fail(c, 500, 50001, "无法创建临时文件")
		return
	}
	defer os.RemoveAll(tmpDir)
	tmpPath := filepath.Join(tmpDir, "finarch.db")

	// Use VACUUM INTO to produce a defragmented, consistent snapshot
	// Add a timeout to prevent the connection from hanging indefinitely
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, tmpPath); err != nil {
		log.Printf("[ERROR] handleBackupDownload: VACUUM INTO failed: %v", err)
		fail(c, 500, 50001, "备份生成失败，请稍后重试")
		return
	}
	if info, err := os.Lstat(tmpPath); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		log.Printf("[ERROR] handleBackupDownload: VACUUM INTO produced invalid file: %v", err)
		fail(c, 500, 50001, "备份生成失败，请稍后重试")
		return
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		fail(c, 500, 50001, "备份文件保护失败")
		return
	}

	backupDB, err := sql.Open("sqlite3", tmpPath)
	if err != nil {
		fail(c, 500, 50001, "备份元数据写入失败")
		return
	}
	defer backupDB.Close()
	backupDB.SetMaxOpenConns(1)
	// Read traceability data from the frozen database itself. Even though the
	// maintenance barrier prevents live writes, keeping all archive metadata
	// derived from one image makes the invariant explicit and testable.
	var schemaVer int
	if err := backupDB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&schemaVer); err != nil {
		fail(c, 500, 50001, "备份元数据读取失败")
		return
	}
	if _, err := backupDB.Exec(`
		CREATE TABLE IF NOT EXISTS backup_metadata (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			user_id TEXT NOT NULL,
			user_email TEXT NOT NULL,
			schema_version INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			app_version TEXT NOT NULL
		)
	`); err != nil {
		fail(c, 500, 50001, "备份元数据写入失败")
		return
	}
	if _, err := backupDB.Exec(`DELETE FROM backup_metadata WHERE id = 1`); err != nil {
		fail(c, 500, 50001, "备份元数据写入失败")
		return
	}
	appVersion := strings.TrimSpace(os.Getenv("APP_VERSION"))
	if appVersion == "" {
		appVersion = "unknown"
	}
	if _, err := backupDB.Exec(
		`INSERT INTO backup_metadata (id, user_id, user_email, schema_version, created_at, app_version) VALUES (1, ?, ?, ?, ?, ?)`,
		userID(c), c.GetString("userEmail"), schemaVer, time.Now().UTC().Format(time.RFC3339), appVersion,
	); err != nil {
		fail(c, 500, 50001, "备份元数据写入失败")
		return
	}
	var snapshotAttachments []attachmentBackupFile
	if s.attachmentSvc != nil {
		snapshotAttachments, err = listSnapshotAttachmentFiles(ctx, backupDB, userID(c))
		if err != nil {
			log.Printf("[ERROR] handleBackupDownload: snapshot attachment manifest failed: %v", err)
			fail(c, http.StatusConflict, "backup_scope_conflict", "A complete attachment backup cannot be created for this database scope.")
			return
		}
	}
	if err := backupDB.Close(); err != nil {
		fail(c, 500, 50001, "备份元数据写入失败")
		return
	}
	validationDB, err := openExistingSQLiteReadOnly(ctx, tmpPath)
	if err != nil {
		fail(c, 500, 50001, "备份完整性校验失败")
		return
	}
	validationErr := ensureSQLiteIntegrity(ctx, validationDB)
	validationCloseErr := validationDB.Close()
	if validationErr != nil || validationCloseErr != nil {
		log.Printf("[ERROR] handleBackupDownload: final snapshot validation failed: %v", errors.Join(validationErr, validationCloseErr))
		fail(c, 500, 50001, "备份完整性校验失败")
		return
	}

	ts := time.Now().Format("20060102_150405")
	// Add proper cache-control headers to prevent the browser from caching the backup download
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	if s.attachmentSvc != nil && len(snapshotAttachments) > 0 {
		zipPath, attachmentCount, err := s.createBackupZip(c.Request.Context(), tmpPath, snapshotAttachments, ts)
		if err != nil {
			log.Printf("[ERROR] handleBackupDownload: attachment zip failed: %v", err)
			fail(c, 500, 50001, "附件备份生成失败，请稍后重试")
			return
		}
		if attachmentCount > 0 {
			defer os.Remove(zipPath)
			filename := fmt.Sprintf("finarch_backup_v%d_%s.zip", schemaVer, ts)
			c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, filename))
			c.Header("Content-Type", "application/zip")
			releaseBackupMaintenance()
			c.File(zipPath)
			return
		}
		_ = os.Remove(zipPath)
	}

	filename := fmt.Sprintf("finarch_backup_v%d_%s.db", schemaVer, ts)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, filename))
	c.Header("Content-Type", "application/octet-stream")
	releaseBackupMaintenance()
	c.File(tmpPath)
}

func listSnapshotAttachmentFiles(ctx context.Context, snapshotDB *sql.DB, userID string) ([]attachmentBackupFile, error) {
	var foreignAttachments int
	if err := snapshotDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM attachments WHERE user_id <> ?`, userID,
	).Scan(&foreignAttachments); err != nil {
		return nil, fmt.Errorf("count out-of-scope snapshot attachments: %w", err)
	}
	// This endpoint authenticates one account. Never disclose another user's
	// attachment bytes through it. Refuse to emit a silently incomplete
	// whole-database archive when the snapshot references such files.
	if foreignAttachments > 0 {
		return nil, fmt.Errorf("snapshot contains %d attachment(s) outside the authenticated user", foreignAttachments)
	}

	rows, err := snapshotDB.QueryContext(ctx, `
		SELECT storage_key, size_bytes, sha256
		FROM attachments
		WHERE user_id = ?
		ORDER BY storage_key ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list snapshot attachments: %w", err)
	}
	defer rows.Close()

	files := make([]attachmentBackupFile, 0)
	for rows.Next() {
		var file attachmentBackupFile
		if err := rows.Scan(&file.StorageKey, &file.SizeBytes, &file.SHA256); err != nil {
			return nil, fmt.Errorf("scan snapshot attachment: %w", err)
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate snapshot attachments: %w", err)
	}
	return files, nil
}

func (s *Server) createBackupZip(ctx context.Context, dbPath string, attachments []attachmentBackupFile, ts string) (string, int, error) {
	zipFile, err := os.CreateTemp("", "finarch-backup-*.zip")
	if err != nil {
		return "", 0, err
	}
	zipPath := zipFile.Name()
	zw := zip.NewWriter(zipFile)
	cleanup := func(retErr error) (string, int, error) {
		_ = zw.Close()
		_ = zipFile.Close()
		_ = os.Remove(zipPath)
		return "", 0, retErr
	}

	dbEntry, err := zw.Create("finarch.db")
	if err != nil {
		return cleanup(err)
	}
	dbFile, err := os.Open(dbPath)
	if err != nil {
		return cleanup(err)
	}
	if _, err := io.Copy(dbEntry, dbFile); err != nil {
		dbFile.Close()
		return cleanup(err)
	}
	dbFile.Close()

	manifest := attachmentBackupManifest{Version: 1, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Files: []attachmentBackupFile{}}
	for _, attachment := range attachments {
		r, err := s.attachmentSvc.OpenStorage(ctx, attachment.StorageKey)
		if err != nil {
			return cleanup(err)
		}
		entryName, ok := safeAttachmentZipPath(attachment.StorageKey)
		if !ok {
			r.Close()
			return cleanup(fmt.Errorf("invalid attachment storage key"))
		}
		entry, err := zw.Create(entryName)
		if err != nil {
			r.Close()
			return cleanup(err)
		}
		hasher := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(entry, hasher), r)
		closeErr := r.Close()
		if copyErr != nil || closeErr != nil {
			return cleanup(errors.Join(copyErr, closeErr))
		}
		actualSHA := hex.EncodeToString(hasher.Sum(nil))
		if written != attachment.SizeBytes || !strings.EqualFold(actualSHA, attachment.SHA256) {
			return cleanup(fmt.Errorf("attachment %q does not match snapshot metadata", attachment.StorageKey))
		}
		manifest.Files = append(manifest.Files, attachment)
	}
	manifestEntry, err := zw.Create("attachments/manifest.json")
	if err != nil {
		return cleanup(err)
	}
	if err := json.NewEncoder(manifestEntry).Encode(manifest); err != nil {
		return cleanup(err)
	}
	if err := zw.Close(); err != nil {
		_ = zipFile.Close()
		_ = os.Remove(zipPath)
		return "", 0, err
	}
	if err := zipFile.Close(); err != nil {
		_ = os.Remove(zipPath)
		return "", 0, err
	}
	return zipPath, len(attachments), nil
}

func safeAttachmentZipPath(storageKey string) (string, bool) {
	cleaned := filepath.Clean(strings.TrimPrefix(filepath.FromSlash(storageKey), string(os.PathSeparator)))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) || filepath.IsAbs(cleaned) {
		return "", false
	}
	return filepath.ToSlash(filepath.Join("attachments", "files", cleaned)), true
}

// handleBackupInfo returns metadata about the current database (for UI preview).
func (s *Server) handleBackupInfo(c *gin.Context) {
	ctx := c.Request.Context()

	var txCount, acctCount, schemaVersion int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE user_id = ?`, userID(c)).Scan(&txCount)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts WHERE user_id = ?`, userID(c)).Scan(&acctCount)
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&schemaVersion)

	// DB file size
	var dbSize int64
	if info, err := os.Stat(s.dbPath); err == nil {
		dbSize = info.Size()
	}

	// Check WAL mode (important for Litestream compatibility)
	var journalMode string
	_ = s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode)

	ok(c, gin.H{
		"transactions":   txCount,
		"accounts":       acctCount,
		"schema_version": schemaVersion,
		"db_size_bytes":  dbSize,
		"journal_mode":   journalMode,
	})
}

const (
	maxRestoreSize                int64  = 100 << 20 // compressed upload / raw DB
	maxRestoreRequestSize                = maxRestoreSize + (1 << 20)
	maxRestoreZipEntries                 = 1024
	maxRestoreZipEntryBytes       int64  = 100 << 20
	maxRestoreZipExpandedBytes    int64  = 200 << 20
	maxRestoreZipCompressionRatio uint64 = 100
)

func restoreFormFile(c *gin.Context, maxBodyBytes int64) (*multipart.FileHeader, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	return c.FormFile("file")
}

func restoreBodyTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr) || strings.Contains(strings.ToLower(err.Error()), "request body too large")
}

func zipCompressionRatioExceeded(uncompressed, compressed uint64) bool {
	if uncompressed == 0 {
		return false
	}
	if compressed == 0 {
		return true
	}
	return uncompressed/compressed > maxRestoreZipCompressionRatio
}

func extractRestoreUpload(fh *multipart.FileHeader) (dbPath string, attachmentDir string, cleanupPath string, cleanup func(), err error) {
	cleanupPaths := []string{}
	cleanup = func() {
		for _, path := range cleanupPaths {
			_ = os.RemoveAll(path)
		}
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	switch ext {
	case ".db":
		tmp, createErr := os.CreateTemp("", "finarch-restore-*.db")
		if createErr != nil {
			return "", "", "", cleanup, createErr
		}
		dbPath = tmp.Name()
		cleanupPaths = append(cleanupPaths, dbPath)
		src, openErr := fh.Open()
		if openErr != nil {
			tmp.Close()
			cleanup()
			return "", "", "", cleanup, openErr
		}
		written, copyErr := io.Copy(tmp, io.LimitReader(src, maxRestoreSize+1))
		src.Close()
		closeErr := tmp.Close()
		if copyErr != nil {
			cleanup()
			return "", "", "", cleanup, copyErr
		}
		if closeErr != nil {
			cleanup()
			return "", "", "", cleanup, closeErr
		}
		if written > maxRestoreSize {
			cleanup()
			return "", "", "", cleanup, fmt.Errorf("restore database exceeds size limit")
		}
		return dbPath, "", dbPath, cleanup, nil
	case ".zip":
		baseDir, mkErr := os.MkdirTemp("", "finarch-restore-zip-*")
		if mkErr != nil {
			return "", "", "", cleanup, mkErr
		}
		cleanupPaths = append(cleanupPaths, baseDir)
		src, openErr := fh.Open()
		if openErr != nil {
			cleanup()
			return "", "", "", cleanup, openErr
		}
		zipPath := filepath.Join(baseDir, "upload.zip")
		zipTmp, createErr := os.Create(zipPath)
		if createErr != nil {
			src.Close()
			cleanup()
			return "", "", "", cleanup, createErr
		}
		written, copyErr := io.Copy(zipTmp, io.LimitReader(src, maxRestoreSize+1))
		src.Close()
		closeErr := zipTmp.Close()
		if copyErr != nil {
			cleanup()
			return "", "", "", cleanup, copyErr
		}
		if closeErr != nil {
			cleanup()
			return "", "", "", cleanup, closeErr
		}
		if written > maxRestoreSize {
			cleanup()
			return "", "", "", cleanup, fmt.Errorf("restore archive exceeds size limit")
		}
		dbPath, attachmentDir, err = extractRestoreZip(zipPath, baseDir)
		if err != nil {
			cleanup()
			return "", "", "", cleanup, err
		}
		return dbPath, attachmentDir, baseDir, cleanup, nil
	default:
		return "", "", "", cleanup, fmt.Errorf("unsupported restore file type")
	}
}

func extractRestoreZip(zipPath, baseDir string) (string, string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", "", err
	}
	defer zr.Close()
	if len(zr.File) > maxRestoreZipEntries {
		return "", "", fmt.Errorf("restore archive contains too many entries")
	}
	extractDir := filepath.Join(baseDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o700); err != nil {
		return "", "", err
	}
	var expandedBytes int64
	for _, file := range zr.File {
		name := filepath.Clean(strings.TrimPrefix(filepath.FromSlash(file.Name), string(os.PathSeparator)))
		if name == "." || name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || filepath.IsAbs(name) {
			return "", "", fmt.Errorf("zip path traversal rejected")
		}
		if file.FileInfo().IsDir() {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("zip symbolic links are not allowed")
		}
		if file.UncompressedSize64 > uint64(maxRestoreZipEntryBytes) {
			return "", "", fmt.Errorf("restore archive entry exceeds size limit")
		}
		if zipCompressionRatioExceeded(file.UncompressedSize64, file.CompressedSize64) {
			return "", "", fmt.Errorf("restore archive entry compression ratio is unsafe")
		}
		remaining := maxRestoreZipExpandedBytes - expandedBytes
		if remaining <= 0 || file.UncompressedSize64 > uint64(remaining) {
			return "", "", fmt.Errorf("restore archive exceeds expanded size limit")
		}
		target := filepath.Join(extractDir, name)
		rel, err := filepath.Rel(extractDir, target)
		if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return "", "", fmt.Errorf("zip path traversal rejected")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return "", "", err
		}
		r, err := file.Open()
		if err != nil {
			return "", "", err
		}
		w, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			r.Close()
			return "", "", err
		}
		entryLimit := maxRestoreZipEntryBytes
		if remaining < entryLimit {
			entryLimit = remaining
		}
		written, copyErr := io.Copy(w, io.LimitReader(r, entryLimit+1))
		closeErr := w.Close()
		r.Close()
		if copyErr != nil {
			return "", "", copyErr
		}
		if closeErr != nil {
			return "", "", closeErr
		}
		if written > entryLimit {
			return "", "", fmt.Errorf("restore archive exceeds expanded size limit")
		}
		if zipCompressionRatioExceeded(uint64(written), file.CompressedSize64) {
			return "", "", fmt.Errorf("restore archive entry compression ratio is unsafe")
		}
		expandedBytes += written
	}
	dbPath := filepath.Join(extractDir, "finarch.db")
	if _, err := os.Stat(dbPath); err != nil {
		return "", "", fmt.Errorf("zip backup missing finarch.db")
	}
	attachmentDir := filepath.Join(extractDir, "attachments", "files")
	if _, err := os.Stat(attachmentDir); err != nil {
		attachmentDir = ""
	}
	return dbPath, attachmentDir, nil
}

func validateSQLiteMagic(path string) error {
	magic := make([]byte, 16)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.ReadFull(f, magic); err != nil {
		return err
	}
	if string(magic[:15]) != "SQLite format 3" {
		return fmt.Errorf("not sqlite")
	}
	return nil
}

func restoreReplacementAttachments(ctx context.Context, database *sql.DB, sourceDir string, attachmentSvc *service.AttachmentService) error {
	if strings.TrimSpace(sourceDir) == "" || attachmentSvc == nil {
		return nil
	}
	files, err := replacementAttachmentFiles(ctx, database)
	if err != nil {
		return err
	}
	return restoreAttachmentFiles(ctx, sourceDir, files, attachmentSvc)
}

func replacementAttachmentFiles(ctx context.Context, database *sql.DB) ([]attachmentRestoreFile, error) {
	rows, err := database.QueryContext(ctx, `SELECT storage_key, size_bytes, sha256 FROM attachments ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []attachmentRestoreFile{}
	for rows.Next() {
		var file attachmentRestoreFile
		if err := rows.Scan(&file.SourceKey, &file.SizeBytes, &file.SHA256); err != nil {
			return nil, err
		}
		file.TargetKey = file.SourceKey
		if err := validateAttachmentRestoreMetadata(file); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

func sameAttachmentRestoreFiles(left, right []attachmentRestoreFile) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func validateAttachmentRestoreMetadata(file attachmentRestoreFile) error {
	if file.SizeBytes <= 0 || file.SizeBytes > maxRestoreZipEntryBytes {
		return fmt.Errorf("attachment %q has invalid size metadata", file.SourceKey)
	}
	expected := strings.TrimSpace(file.SHA256)
	if len(expected) != sha256.Size*2 {
		return fmt.Errorf("attachment %q has invalid SHA-256 metadata", file.SourceKey)
	}
	digest, err := hex.DecodeString(expected)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("attachment %q has invalid SHA-256 metadata", file.SourceKey)
	}
	return nil
}

func validateStoredAttachmentFiles(ctx context.Context, files []attachmentRestoreFile, attachmentSvc *service.AttachmentService) error {
	if len(files) == 0 {
		return nil
	}
	if attachmentSvc == nil {
		return fmt.Errorf("attachment storage is unavailable")
	}
	for _, file := range files {
		if err := validateAttachmentRestoreMetadata(file); err != nil {
			return err
		}
		r, err := attachmentSvc.OpenStorage(ctx, file.TargetKey)
		if err != nil {
			return fmt.Errorf("attachment file %q is unavailable: %w", file.TargetKey, err)
		}
		hasher := sha256.New()
		written, copyErr := io.Copy(hasher, r)
		closeErr := r.Close()
		if copyErr != nil || closeErr != nil {
			return fmt.Errorf("read attachment file %q: %w", file.TargetKey, errors.Join(copyErr, closeErr))
		}
		if written != file.SizeBytes {
			return fmt.Errorf("attachment file %q size does not match database metadata", file.TargetKey)
		}
		if expected := strings.TrimSpace(file.SHA256); !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), expected) {
			return fmt.Errorf("attachment file %q checksum does not match database metadata", file.TargetKey)
		}
	}
	return nil
}

func attachmentStorageOwners(ctx context.Context, database *sql.DB) (map[string]string, error) {
	rows, err := database.QueryContext(ctx, `SELECT storage_key, user_id FROM attachments`)
	if err != nil {
		return nil, fmt.Errorf("list attachment storage owners: %w", err)
	}
	defer rows.Close()
	owners := make(map[string]string)
	for rows.Next() {
		var storageKey, userID string
		if err := rows.Scan(&storageKey, &userID); err != nil {
			return nil, fmt.Errorf("scan attachment storage owner: %w", err)
		}
		owners[storageKey] = userID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attachment storage owners: %w", err)
	}
	return owners, nil
}

// reconcileReplacementAttachmentDeletions preserves the invariant that a
// live attachment can never also have a deletion tombstone, then durably
// queues physical objects referenced only by the database being replaced.
// It runs while restore maintenance is exclusive; a later restore failure
// copies the safety database back and therefore rolls these queue changes back.
func reconcileReplacementAttachmentDeletions(ctx context.Context, database *sql.DB, previousOwners map[string]string) error {
	currentOwners, err := attachmentStorageOwners(ctx, database)
	if err != nil {
		return err
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin attachment deletion reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM attachment_deletion_queue
		WHERE storage_key IN (SELECT storage_key FROM attachments)
	`); err != nil {
		return fmt.Errorf("cancel deletion of restored attachments: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for storageKey, ownerID := range previousOwners {
		if _, stillReferenced := currentOwners[storageKey]; stillReferenced {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO attachment_deletion_queue(
				storage_key, user_id, attempts, last_error, created_at, updated_at
			) VALUES (?, ?, 0, NULL, ?, ?)
			ON CONFLICT(storage_key) DO NOTHING
		`, storageKey, ownerID, now, now); err != nil {
			return fmt.Errorf("queue replaced attachment for deletion: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit attachment deletion reconciliation: %w", err)
	}
	return nil
}

type attachmentRollbackEntry struct {
	storageKey     string
	existed        bool
	backupPath     string
	expectedSize   int64
	expectedSHA256 string
}

type attachmentRollbackJournal struct {
	dir           string
	entries       []attachmentRollbackEntry
	attachmentSvc *service.AttachmentService
}

type attachmentRollbackManifest struct {
	Version int                               `json:"version"`
	Entries []attachmentRollbackManifestEntry `json:"entries"`
}

type attachmentRollbackManifestEntry struct {
	StorageKey string `json:"storage_key"`
	Existed    bool   `json:"existed"`
	BackupFile string `json:"backup_file,omitempty"`
	SizeBytes  int64  `json:"size_bytes"`
	SHA256     string `json:"sha256,omitempty"`
}

func prepareAttachmentRollbackJournal(ctx context.Context, rootDir string, files []attachmentRestoreFile, attachmentSvc *service.AttachmentService, operations ...*durableRestoreOperation) (journal *attachmentRollbackJournal, returnErr error) {
	if len(files) == 0 || attachmentSvc == nil {
		return nil, nil
	}
	if len(operations) > 1 {
		return nil, fmt.Errorf("multiple restore operations supplied for attachment journal")
	}
	if err := os.MkdirAll(rootDir, 0o700); err != nil {
		return nil, fmt.Errorf("create attachment rollback directory: %w", pathSafeFilesystemError(err))
	}
	if err := validateRestoreArtifactDirectory(rootDir); err != nil {
		return nil, fmt.Errorf("validate attachment rollback directory: %w", err)
	}
	if err := os.Chmod(rootDir, 0o700); err != nil {
		return nil, fmt.Errorf("secure attachment rollback directory: %w", pathSafeFilesystemError(err))
	}
	if err := validatePrivateRestoreArtifactDirectory(rootDir); err != nil {
		return nil, fmt.Errorf("validate private attachment rollback directory: %w", err)
	}
	if err := syncRestoreArtifactDirectory(rootDir); err != nil {
		return nil, fmt.Errorf("sync attachment rollback directory: %w", err)
	}
	if err := syncRestoreArtifactDirectory(filepath.Dir(rootDir)); err != nil {
		return nil, fmt.Errorf("sync attachment rollback parent: %w", err)
	}
	rootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve attachment rollback directory: %w", err)
	}
	var dir string
	if len(operations) == 1 && operations[0] != nil {
		operation := operations[0]
		validPreparingOperation := operation.marker.Phase == restoreOperationPhasePreparing &&
			(operation.marker.Kind == restoreOperationKindCross || operation.marker.Kind == restoreOperationKindReplace)
		if !validPreparingOperation {
			return nil, fmt.Errorf("attachment journal requires a preparing restore operation")
		}
		if filepath.Clean(filepath.Dir(operation.markerPath)) != filepath.Clean(rootDir) ||
			operation.marker.AttachmentJournal != restoreJournalDirPrefix+operation.marker.OperationID {
			return nil, fmt.Errorf("attachment journal operation is outside its restore artifact directory")
		}
		dir = filepath.Join(rootDir, operation.marker.AttachmentJournal)
		err = os.Mkdir(dir, 0o700)
	} else {
		dir, err = os.MkdirTemp(rootDir, restoreJournalDirPrefix+"*")
	}
	if err != nil {
		return nil, err
	}
	journal = &attachmentRollbackJournal{dir: dir, attachmentSvc: attachmentSvc}
	if err := syncRestoreArtifactDirectory(rootDir); err != nil {
		_ = journal.cleanup()
		return journal, fmt.Errorf("sync attachment journal creation: %w", err)
	}
	cleanupJournal := journal
	operationManaged := len(operations) == 1 && operations[0] != nil
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = cleanupJournal.cleanup()
			panic(recovered)
		}
		if returnErr != nil && !operationManaged {
			_ = cleanupJournal.cleanup()
		}
	}()

	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		storageKey := file.TargetKey
		if strings.TrimSpace(storageKey) == "" {
			return journal, fmt.Errorf("attachment rollback key is empty")
		}
		if _, duplicate := seen[storageKey]; duplicate {
			continue
		}
		seen[storageKey] = struct{}{}

		original, err := attachmentSvc.OpenStorage(ctx, storageKey)
		if errors.Is(err, os.ErrNotExist) {
			if original != nil {
				_ = original.Close()
			}
			journal.entries = append(journal.entries, attachmentRollbackEntry{storageKey: storageKey})
			continue
		}
		if err != nil {
			if original != nil {
				_ = original.Close()
			}
			return journal, fmt.Errorf("read original attachment %q: %w", storageKey, err)
		}

		backup, err := os.CreateTemp(dir, "original-*.bin")
		if err != nil {
			_ = original.Close()
			return journal, fmt.Errorf("create attachment journal entry: %w", err)
		}
		backupPath := backup.Name()
		digest := sha256.New()
		copied, copyErr := io.Copy(io.MultiWriter(backup, digest), io.LimitReader(original, maxRollbackBackupBytes+1))
		var sizeErr error
		if copied > maxRollbackBackupBytes {
			sizeErr = fmt.Errorf("attachment exceeds rollback snapshot limit")
		}
		syncErr := backup.Sync()
		backupCloseErr := backup.Close()
		originalCloseErr := original.Close()
		if copyErr != nil || sizeErr != nil || syncErr != nil || backupCloseErr != nil || originalCloseErr != nil {
			_ = os.Remove(backupPath)
			return journal, fmt.Errorf("snapshot original attachment %q: %w", storageKey, errors.Join(copyErr, sizeErr, syncErr, backupCloseErr, originalCloseErr))
		}
		journal.entries = append(journal.entries, attachmentRollbackEntry{
			storageKey:     storageKey,
			existed:        true,
			backupPath:     backupPath,
			expectedSize:   copied,
			expectedSHA256: hex.EncodeToString(digest.Sum(nil)),
		})
	}
	if err := journal.writeManifest(); err != nil {
		return journal, err
	}
	return journal, nil
}

func (j *attachmentRollbackJournal) writeManifest() error {
	if j == nil || strings.TrimSpace(j.dir) == "" {
		return fmt.Errorf("attachment rollback journal is unavailable")
	}
	if err := validatePrivateRestoreArtifactDirectory(j.dir); err != nil {
		return fmt.Errorf("validate attachment rollback journal: %w", err)
	}
	manifest := attachmentRollbackManifest{Version: rollbackManifestVersion, Entries: make([]attachmentRollbackManifestEntry, 0, len(j.entries))}
	for _, entry := range j.entries {
		backupFile := ""
		if entry.backupPath != "" {
			backupFile = filepath.Base(entry.backupPath)
		}
		manifest.Entries = append(manifest.Entries, attachmentRollbackManifestEntry{
			StorageKey: entry.storageKey,
			Existed:    entry.existed,
			BackupFile: backupFile,
			SizeBytes:  entry.expectedSize,
			SHA256:     entry.expectedSHA256,
		})
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode attachment rollback manifest: %w", err)
	}
	if len(payload) == 0 || len(payload) > maxRollbackManifestBytes {
		return fmt.Errorf("attachment rollback manifest exceeds its size limit")
	}
	tmp, err := os.CreateTemp(j.dir, ".manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("create attachment rollback manifest: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write attachment rollback manifest: %w", err)
	}
	syncErr := tmp.Sync()
	closeErr := tmp.Close()
	if syncErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sync attachment rollback manifest: %w", errors.Join(syncErr, closeErr))
	}
	if err := os.Rename(tmpPath, filepath.Join(j.dir, "manifest.json")); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("publish attachment rollback manifest: %w", err)
	}
	if err := syncRestoreArtifactDirectory(j.dir); err != nil {
		return fmt.Errorf("sync attachment rollback manifest directory: %w", err)
	}
	return nil
}

func (j *attachmentRollbackJournal) rollback(ctx context.Context) error {
	if j == nil {
		return nil
	}
	if err := validatePrivateRestoreArtifactDirectory(j.dir); err != nil {
		return fmt.Errorf("validate attachment rollback journal: %w", err)
	}
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	var rollbackErrors []error
	for i := len(j.entries) - 1; i >= 0; i-- {
		entry := j.entries[i]
		if !entry.existed {
			if err := j.attachmentSvc.DeleteStorage(rollbackCtx, entry.storageKey); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("delete new attachment %q: %w", entry.storageKey, err))
			}
			continue
		}
		if filepath.Dir(filepath.Clean(entry.backupPath)) != filepath.Clean(j.dir) {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("attachment journal entry %q is outside its journal", entry.storageKey))
			continue
		}
		original, err := os.Open(entry.backupPath)
		if err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("open attachment journal entry %q: %w", entry.storageKey, pathSafeFilesystemError(err)))
			continue
		}
		verifyErr := verifyAttachmentRollbackBackup(original, entry.expectedSize, entry.expectedSHA256)
		if verifyErr != nil {
			closeErr := original.Close()
			rollbackErrors = append(rollbackErrors, fmt.Errorf("verify attachment journal entry %q: %w", entry.storageKey, errors.Join(verifyErr, closeErr)))
			continue
		}
		restoreErr := j.attachmentSvc.RestoreStorage(rollbackCtx, entry.storageKey, io.LimitReader(original, entry.expectedSize))
		closeErr := original.Close()
		if restoreErr != nil || closeErr != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore original attachment %q: %w", entry.storageKey, errors.Join(restoreErr, closeErr)))
		}
	}
	return errors.Join(rollbackErrors...)
}

func validRollbackSHA256(encoded string) bool {
	if len(encoded) != sha256.Size*2 || encoded != strings.ToLower(encoded) {
		return false
	}
	digest, err := hex.DecodeString(encoded)
	return err == nil && len(digest) == sha256.Size
}

// verifyAttachmentRollbackBackup binds rollback to the exact bytes captured in
// the durable manifest. The caller must restore from this same open descriptor
// after the seek, so a path replacement cannot swap in unchecked bytes.
func verifyAttachmentRollbackBackup(file *os.File, expectedSize int64, expectedSHA256 string) error {
	if file == nil {
		return fmt.Errorf("attachment journal entry is unavailable")
	}
	if expectedSize < 0 || expectedSize > maxRollbackBackupBytes || !validRollbackSHA256(expectedSHA256) {
		return fmt.Errorf("attachment journal integrity metadata is invalid")
	}
	info, err := file.Stat()
	if err != nil {
		return pathSafeFilesystemError(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Size() != expectedSize {
		return fmt.Errorf("attachment journal entry size or type changed")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek attachment journal entry: %w", err)
	}
	digest := sha256.New()
	copied, copyErr := io.CopyN(digest, file, expectedSize)
	if copyErr != nil || copied != expectedSize {
		return fmt.Errorf("hash attachment journal entry: %w", copyErr)
	}
	var extra [1]byte
	n, readErr := file.Read(extra[:])
	if n != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
		return fmt.Errorf("attachment journal entry changed while hashing")
	}
	expectedDigest, _ := hex.DecodeString(expectedSHA256)
	if subtle.ConstantTimeCompare(digest.Sum(nil), expectedDigest) != 1 {
		return fmt.Errorf("attachment journal entry digest mismatch")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind attachment journal entry: %w", err)
	}
	return nil
}

func (j *attachmentRollbackJournal) cleanup() error {
	if j == nil || strings.TrimSpace(j.dir) == "" {
		return nil
	}
	return removeRestoreArtifactTree(j.dir)
}

func (j *attachmentRollbackJournal) retain(restoreID string, operation *durableRestoreOperation) (string, error) {
	if j == nil || strings.TrimSpace(j.dir) == "" {
		return "", nil
	}
	if operation == nil || operation.markerPath == "" ||
		filepath.Clean(filepath.Dir(operation.markerPath)) != filepath.Clean(j.dir) ||
		filepath.Base(operation.markerPath) != restoreOperationJournalFile {
		return filepath.Base(j.dir), fmt.Errorf("attachment journal is missing its operation-bound marker")
	}
	journalParent := filepath.Dir(j.dir)
	if err := validateRestoreOperationMarkerLocation(operation.marker, operation.markerPath, journalParent); err != nil {
		return filepath.Base(j.dir), fmt.Errorf("validate attachment journal marker: %w", err)
	}
	if err := validatePrivateRestoreArtifactDirectory(j.dir); err != nil {
		return filepath.Base(j.dir), fmt.Errorf("validate attachment journal: %w", err)
	}
	if err := validatePrivateRestoreArtifactDirectory(journalParent); err != nil {
		return filepath.Base(j.dir), fmt.Errorf("validate attachment journal parent: %w", err)
	}
	retainedName := retainedRestoreJournalName(restoreID, operation.marker.OperationID)
	retainedPath := filepath.Join(journalParent, retainedName)
	var permissionErrors []error
	if err := os.Chmod(j.dir, 0o700); err != nil {
		permissionErrors = append(permissionErrors, fmt.Errorf("secure attachment journal directory: %w", pathSafeFilesystemError(err)))
	}
	entries, err := os.ReadDir(j.dir)
	if err != nil {
		permissionErrors = append(permissionErrors, fmt.Errorf("inspect attachment journal: %w", pathSafeFilesystemError(err)))
	} else {
		for _, entry := range entries {
			entryPath := filepath.Join(j.dir, entry.Name())
			info, statErr := os.Lstat(entryPath)
			if statErr != nil {
				permissionErrors = append(permissionErrors, fmt.Errorf("inspect attachment journal file %s: %w", entry.Name(), pathSafeFilesystemError(statErr)))
				continue
			}
			if !info.Mode().IsRegular() {
				permissionErrors = append(permissionErrors, fmt.Errorf("attachment journal file %s is not regular", entry.Name()))
				continue
			}
			if err := os.Chmod(entryPath, 0o600); err != nil {
				permissionErrors = append(permissionErrors, fmt.Errorf("secure attachment journal file %s: %w", entry.Name(), pathSafeFilesystemError(err)))
			}
		}
	}
	if len(permissionErrors) > 0 {
		return filepath.Base(j.dir), errors.Join(permissionErrors...)
	}
	if err := os.Rename(j.dir, retainedPath); err != nil {
		return filepath.Base(j.dir), fmt.Errorf("retain attachment journal %s: %w", filepath.Base(j.dir), pathSafeFilesystemError(err))
	}
	j.dir = retainedPath
	operation.markerPath = filepath.Join(retainedPath, restoreOperationJournalFile)
	return retainedName, syncRestoreArtifactDirectory(journalParent)
}

func restoreAttachmentFiles(ctx context.Context, sourceDir string, files []attachmentRestoreFile, attachmentSvc *service.AttachmentService) error {
	if strings.TrimSpace(sourceDir) == "" || attachmentSvc == nil || len(files) == 0 {
		return nil
	}
	sourceRoot, err := filepath.Abs(sourceDir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := validateAttachmentRestoreMetadata(file); err != nil {
			return err
		}
		cleaned := filepath.Clean(strings.TrimPrefix(filepath.FromSlash(file.SourceKey), string(os.PathSeparator)))
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) || filepath.IsAbs(cleaned) {
			return fmt.Errorf("invalid attachment key in backup")
		}
		sourcePath := filepath.Join(sourceRoot, cleaned)
		rel, err := filepath.Rel(sourceRoot, sourcePath)
		if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("invalid attachment key in backup")
		}
		f, err := os.Open(sourcePath)
		if err != nil {
			return fmt.Errorf("attachment file missing: %w", err)
		}
		info, statErr := f.Stat()
		if statErr != nil {
			f.Close()
			return statErr
		}
		if info.Size() != file.SizeBytes {
			f.Close()
			return fmt.Errorf("attachment file size mismatch")
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return err
		}
		if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), strings.TrimSpace(file.SHA256)) {
			f.Close()
			return fmt.Errorf("attachment checksum mismatch")
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			f.Close()
			return err
		}
		err = attachmentSvc.RestoreStorage(ctx, file.TargetKey, f)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func cleanupPendingRestore(pr *pendingRestore) {
	if pr == nil {
		return
	}
	pr.cleanupOnce.Do(func() {
		if strings.TrimSpace(pr.cleanupPath) != "" {
			_ = os.RemoveAll(pr.cleanupPath)
			return
		}
		if strings.TrimSpace(pr.tmpPath) != "" {
			_ = os.Remove(pr.tmpPath)
		}
	})
}

func (s *Server) expirePendingRestore(restoreID string, pr *pendingRestore, now time.Time) {
	pr.mu.Lock()
	if pr.consumed || now.Before(pr.expiresAt) {
		pr.mu.Unlock()
		return
	}
	pr.consumed = true
	pr.mu.Unlock()
	if s.pendingRestores.CompareAndDelete(restoreID, pr) {
		cleanupPendingRestore(pr)
	}
}

func (s *Server) cleanupExpiredPendingRestores(now time.Time) {
	s.pendingRestores.Range(func(key, value any) bool {
		restoreID, idOK := key.(string)
		pr, restoreOK := value.(*pendingRestore)
		if idOK && restoreOK {
			s.expirePendingRestore(restoreID, pr, now)
		}
		return true
	})
}

func (s *Server) storePendingRestore(restoreID string, pr *pendingRestore) bool {
	s.pendingRestoreMu.Lock()
	defer s.pendingRestoreMu.Unlock()

	s.cleanupExpiredPendingRestores(time.Now())
	count := 0
	s.pendingRestores.Range(func(_, _ any) bool {
		count++
		return count < maxPendingRestores
	})
	if count >= maxPendingRestores {
		cleanupPendingRestore(pr)
		return false
	}
	s.pendingRestores.Store(restoreID, pr)
	delay := time.Until(pr.expiresAt)
	if delay < 0 {
		delay = 0
	}
	time.AfterFunc(delay, func() {
		s.expirePendingRestore(restoreID, pr, time.Now())
	})
	return true
}

func (s *Server) consumePendingRestoreToken(token, requester string, now time.Time) (string, *pendingRestore) {
	var restoreID string
	var selected *pendingRestore
	s.pendingRestores.Range(func(key, value any) bool {
		candidate := value.(*pendingRestore)
		candidate.mu.Lock()
		expired := !now.Before(candidate.expiresAt)
		matches := !candidate.consumed && !expired && candidate.verified && candidate.token == token && candidate.requester == requester
		if matches {
			candidate.consumed = true
		}
		candidate.mu.Unlock()
		if expired {
			s.expirePendingRestore(key.(string), candidate, now)
		}
		if matches {
			restoreID = key.(string)
			selected = candidate
			return false
		}
		return true
	})
	if selected != nil {
		s.pendingRestores.CompareAndDelete(restoreID, selected)
	}
	return restoreID, selected
}

func safeStorageSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// handleRestore accepts a SQLite database file upload and restores it into the
// live database using the SQLite Online Backup API — no restart needed.
// Safety: automatically creates a pre-restore snapshot so data is never lost.
func (s *Server) handleRestore(c *gin.Context) {
	fh, err := restoreFormFile(c, maxRestoreRequestSize)
	if err != nil {
		if restoreBodyTooLarge(err) {
			fail(c, http.StatusRequestEntityTooLarge, 40004, "恢复请求体过大")
			return
		}
		fail(c, 400, 40001, "请上传数据库文件（multipart field: file）")
		return
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext != ".db" && ext != ".zip" {
		fail(c, 400, 40002, "仅接受 .db 或 .zip 备份文件")
		return
	}
	if fh.Size > maxRestoreSize {
		fail(c, 400, 40004, fmt.Sprintf("文件过大（%d MB），上限 100 MB", fh.Size>>20))
		return
	}

	tmpPath, attachmentDir, _, cleanup, err := extractRestoreUpload(fh)
	if err != nil {
		fail(c, 400, 40003, "备份文件解压或读取失败")
		return
	}
	defer cleanup()

	if err := validateSQLiteMagic(tmpPath); err != nil {
		fail(c, 400, 40003, "文件不是有效的 SQLite 数据库")
		return
	}

	// Authorization for a physical restore comes exclusively from the operations
	// gate. Identity fields inside the uploaded database are untrusted data and
	// must not decide whether a whole-database replacement is allowed.
	srcDB, err := sql.Open("sqlite3", tmpPath+"?mode=ro")
	if err != nil {
		fail(c, 500, 50002, "无法打开备份文件")
		return
	}
	if err := ensureSQLiteIntegrity(c.Request.Context(), srcDB); err != nil {
		srcDB.Close()
		fail(c, 400, 40003, "备份文件完整性校验失败")
		return
	}

	var uploadedVersion int
	row := srcDB.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`)
	if err := row.Scan(&uploadedVersion); err != nil {
		srcDB.Close()
		fail(c, 400, 40005, "备份文件缺少 schema_migrations 表，不是有效的 FinArch 备份")
		return
	}
	srcDB.Close()

	var currentVersion int
	_ = s.db.QueryRowContext(c.Request.Context(), `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&currentVersion)

	if uploadedVersion > currentVersion {
		fail(c, 400, 40006, fmt.Sprintf(
			"备份文件版本 (v%d) 高于当前系统版本 (v%d)，请先升级系统再恢复",
			uploadedVersion, currentVersion,
		))
		return
	}

	ctxRestore := restoreContext{
		RestoreMode:          "OPERATIONS_REPLACE",
		DataScope:            "both",
		AttachmentRestoreDir: attachmentDir,
	}
	migratedTo, err := s.performRestoreWithMerge(c.Request.Context(), tmpPath, uploadedVersion, currentVersion, ctxRestore)
	if err != nil {
		fail(c, 500, "RESTORE_EXECUTE_FAILED", err.Error())
		return
	}

	ok(c, gin.H{
		"code":             "SUCCESS",
		"message":          "Restore completed",
		"restored_version": uploadedVersion,
		"migrated_to":      migratedTo,
	})
}

func (s *Server) handleRestoreSendVerification(c *gin.Context) {
	fh, err := restoreFormFile(c, maxRestoreRequestSize)
	if err != nil {
		if restoreBodyTooLarge(err) {
			fail(c, http.StatusRequestEntityTooLarge, 40004, "恢复请求体过大")
			return
		}
		fail(c, 400, 40001, "请上传数据库文件（multipart field: file）")
		return
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext != ".db" && ext != ".zip" {
		fail(c, 400, 40002, "仅接受 .db 或 .zip 备份文件")
		return
	}
	if fh.Size > maxRestoreSize {
		fail(c, 400, 40004, fmt.Sprintf("文件过大（%d MB），上限 100 MB", fh.Size>>20))
		return
	}
	originalEmail := strings.TrimSpace(c.PostForm("original_email"))
	if originalEmail == "" {
		fail(c, 400, "INVALID_INPUT", "请填写原账号邮箱")
		return
	}

	tmpPath, attachmentDir, cleanupPath, cleanup, err := extractRestoreUpload(fh)
	if err != nil {
		fail(c, 400, 40003, "备份文件解压或读取失败")
		return
	}

	if err := validateSQLiteMagic(tmpPath); err != nil {
		cleanup()
		fail(c, 400, 40003, "文件不是有效的 SQLite 数据库")
		return
	}
	srcDB, err := openExistingSQLiteReadOnly(c.Request.Context(), tmpPath)
	if err != nil {
		cleanup()
		fail(c, 500, 50002, "无法打开备份文件")
		return
	}
	defer srcDB.Close()
	if err := ensureSQLiteIntegrity(c.Request.Context(), srcDB); err != nil {
		cleanup()
		fail(c, 400, 40003, "备份文件完整性校验失败")
		return
	}

	var uploadedVersion int
	if err := srcDB.QueryRowContext(c.Request.Context(), `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&uploadedVersion); err != nil {
		cleanup()
		fail(c, 400, 40005, "备份文件缺少 schema_migrations 表，不是有效的 FinArch 备份")
		return
	}
	identity, err := readBackupIdentity(srcDB)
	if err != nil {
		cleanup()
		if strings.Contains(err.Error(), "备份文件中未找到用户数据") {
			fail(c, 400, 40007, "备份文件中未找到用户数据")
			return
		}
		fail(c, 400, 40007, "读取备份所有者信息失败")
		return
	}

	currentUserID := userID(c)
	restoreMode, backupUserID, backupEmail, sameID, sameEmail := detectRestoreMode(identity, currentUserID, c.GetString("userEmail"))
	backupName := identity.FallbackOwnerName
	if normalizeEmail(backupEmail) != normalizeEmail(originalEmail) {
		cleanup()
		fail(c, http.StatusForbidden, "RESTORE_EMAIL_MISMATCH", "原账号邮箱与备份不匹配")
		return
	}

	log.Printf("[RESTORE] verification_request restore_mode=%s same_id=%t same_email=%t metadata_present=%t request_user_id=%s backup_user_id=%s", restoreMode, sameID, sameEmail, identity.MetadataPresent, currentUserID, backupUserID)
	if restoreMode == "SAME_ACCOUNT" {
		cleanup()
		fail(c, http.StatusBadRequest, "RESTORE_VERIFICATION_NOT_REQUIRED", "当前备份属于本账号，无需邮箱验证")
		return
	}

	var currentVersion int
	if err := s.db.QueryRowContext(c.Request.Context(), `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&currentVersion); err != nil {
		cleanup()
		fail(c, 500, 50002, "无法读取当前数据库版本")
		return
	}
	if uploadedVersion > currentVersion {
		cleanup()
		fail(c, 400, 40006, fmt.Sprintf("备份文件版本 (v%d) 高于当前系统版本 (v%d)，请先升级系统再恢复", uploadedVersion, currentVersion))
		return
	}

	code := generateCode()
	restoreID := uuid.NewString()
	pr := &pendingRestore{code: code, tmpPath: tmpPath, attachmentDir: attachmentDir, cleanupPath: cleanupPath, expiresAt: time.Now().Add(10 * time.Minute), email: backupEmail, name: backupName, ownerID: backupUserID, requester: currentUserID}
	if !s.storePendingRestore(restoreID, pr) {
		cleanupPendingRestore(pr)
		fail(c, http.StatusTooManyRequests, "RESTORE_CAPACITY_REACHED", "恢复会话过多，请稍后重试")
		return
	}
	if err := s.emailSvc.SendRestoreCode(backupEmail, backupName, code); err != nil {
		s.pendingRestores.CompareAndDelete(restoreID, pr)
		cleanupPendingRestore(pr)
		fail(c, 500, 50005, "验证码发送失败，请检查邮件服务配置")
		return
	}

	ok(c, gin.H{"restore_id": restoreID, "masked_email": maskEmail(backupEmail), "expires_in": 600, "message": "验证码已发送"})
}

func (s *Server) handleRestoreVerify(c *gin.Context) {
	var req struct {
		RestoreID string `json:"restore_id" binding:"required"`
		Code      string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, "INVALID_INPUT", "请提供 restore_id 和验证码")
		return
	}
	val, ok_ := s.pendingRestores.Load(req.RestoreID)
	if !ok_ {
		fail(c, 400, 40008, "恢复会话不存在或已过期，请重新上传备份文件")
		return
	}
	pr := val.(*pendingRestore)
	pr.mu.Lock()
	if pr.consumed {
		pr.mu.Unlock()
		fail(c, 400, 40008, "恢复会话不存在或已过期，请重新上传备份文件")
		return
	}
	if !time.Now().Before(pr.expiresAt) {
		pr.consumed = true
		pr.mu.Unlock()
		s.pendingRestores.CompareAndDelete(req.RestoreID, pr)
		cleanupPendingRestore(pr)
		fail(c, 400, 40009, "验证码已过期，请重新上传备份文件")
		return
	}
	if pr.attempts >= 5 {
		pr.consumed = true
		pr.mu.Unlock()
		s.pendingRestores.CompareAndDelete(req.RestoreID, pr)
		cleanupPendingRestore(pr)
		fail(c, 400, 40010, "验证码错误次数过多，请重新上传备份文件")
		return
	}
	if req.Code != pr.code {
		pr.attempts++
		remaining := 5 - pr.attempts
		exhausted := remaining == 0
		if exhausted {
			pr.consumed = true
		}
		pr.mu.Unlock()
		if exhausted {
			s.pendingRestores.CompareAndDelete(req.RestoreID, pr)
			cleanupPendingRestore(pr)
		}
		fail(c, 400, 40011, fmt.Sprintf("验证码错误，剩余 %d 次尝试", remaining))
		return
	}
	if !pr.verified {
		pr.verified = true
		pr.token = uuid.NewString()
	}
	token := pr.token
	pr.mu.Unlock()
	ok(c, gin.H{"restore_token": token, "message": "验证成功"})
}

func (s *Server) handleRestoreExecute(c *gin.Context) {
	var req struct {
		RestoreToken string `json:"restore_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, "INVALID_INPUT", "请提供 restore_token")
		return
	}
	requestUserID := userID(c)
	_, pr := s.consumePendingRestoreToken(req.RestoreToken, requestUserID, time.Now())
	if pr == nil {
		fail(c, http.StatusForbidden, "RESTORE_TOKEN_INVALID", "恢复授权已失效，请重新验证")
		return
	}

	log.Printf("[RESTORE] execute_after_verification restore_mode=%s same_id=%t same_email=%t metadata_present=%t request_user_id=%s backup_user_id=%s", "CROSS_ACCOUNT", false, false, true, requestUserID, pr.ownerID)

	tmpPath := pr.tmpPath
	attachmentDir := pr.attachmentDir
	defer cleanupPendingRestore(pr)

	if err := validateSQLiteMagic(tmpPath); err != nil {
		fail(c, 400, 40003, "文件不是有效的 SQLite 数据库")
		return
	}
	srcDB, err := openExistingSQLiteReadOnly(c.Request.Context(), tmpPath)
	if err != nil {
		fail(c, 500, 50002, "无法打开备份文件")
		return
	}
	if err := ensureSQLiteIntegrity(c.Request.Context(), srcDB); err != nil {
		srcDB.Close()
		fail(c, 400, 40003, "备份文件完整性校验失败")
		return
	}
	var uploadedVersion int
	if err := srcDB.QueryRowContext(c.Request.Context(), `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&uploadedVersion); err != nil {
		srcDB.Close()
		fail(c, 400, 40005, "备份文件缺少 schema_migrations 表，不是有效的 FinArch 备份")
		return
	}
	if err := srcDB.Close(); err != nil {
		fail(c, 500, 50002, "无法关闭备份文件")
		return
	}
	var currentVersion int
	if err := s.db.QueryRowContext(c.Request.Context(), `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&currentVersion); err != nil {
		fail(c, 500, 50002, "无法读取当前数据库版本")
		return
	}

	if uploadedVersion > currentVersion {
		fail(c, 400, 40006, fmt.Sprintf("备份文件版本 (v%d) 高于当前系统版本 (v%d)，请先升级系统再恢复", uploadedVersion, currentVersion))
		return
	}

	ctxRestore := restoreContext{
		RequesterUserID:      requestUserID,
		BackupUserID:         strings.TrimSpace(pr.ownerID),
		RestoreMode:          "CROSS_ACCOUNT",
		DataScope:            "both",
		AttachmentRestoreDir: attachmentDir,
	}
	migratedTo, err := s.performRestoreWithMerge(c.Request.Context(), tmpPath, uploadedVersion, currentVersion, ctxRestore)
	if err != nil {
		fail(c, 500, "RESTORE_EXECUTE_FAILED", err.Error())
		return
	}
	ok(c, gin.H{"code": "SUCCESS", "message": "Restore completed", "restored_version": uploadedVersion, "migrated_to": migratedTo})
}

func (s *Server) performRestoreWithMerge(ctx context.Context, tmpPath string, uploadedVersion, currentVersion int, restoreCtx restoreContext) (int, error) {
	if restoreCtx.RestoreMode == "CROSS_ACCOUNT" && strings.TrimSpace(restoreCtx.RequesterUserID) != "" {
		return s.performCrossAccountMergeRestore(ctx, tmpPath, uploadedVersion, currentVersion, restoreCtx)
	}

	return s.performReplaceRestore(ctx, tmpPath, uploadedVersion, currentVersion, restoreCtx)
}

func (s *Server) performReplaceRestore(ctx context.Context, tmpPath string, uploadedVersion, currentVersion int, restoreCtx restoreContext) (int, error) {
	_ = currentVersion
	engineResult, err := s.executeRestoreEngine(ctx, restoreEngineRequest{
		RestoreID:            uuid.NewString(),
		Source:               restoreSourceUserBackup,
		SnapshotID:           restoreCtx.RestoreMode,
		TempDBPath:           tmpPath,
		UploadedVersion:      uploadedVersion,
		SchemaBefore:         currentVersion,
		AttachmentRestoreDir: restoreCtx.AttachmentRestoreDir,
	})
	if err != nil {
		return 0, err
	}
	return engineResult.SchemaAfter, nil
}

func deterministicRestoreID(entity, sourceUserID, targetUserID, oldID string) string {
	seed := strings.Join([]string{"finarch-restore", entity, sourceUserID, targetUserID, oldID}, ":")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed)).String()
}

func modeFromAccountType(accountType string) string {
	switch strings.ToLower(strings.TrimSpace(accountType)) {
	case string(model.AccountTypePersonal):
		return string(model.ModeLife)
	case string(model.AccountTypePublic):
		return string(model.ModeWork)
	default:
		return string(model.ModeWork)
	}
}

func ensureSQLiteIntegrity(ctx context.Context, db *sql.DB) error {
	var integrityResult string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrityResult); err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(integrityResult)) != "ok" {
		return fmt.Errorf("integrity_check=%s", integrityResult)
	}
	foreignKeyRows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	defer foreignKeyRows.Close()
	if foreignKeyRows.Next() {
		var table, parent string
		var rowID any
		var foreignKeyID int
		if err := foreignKeyRows.Scan(&table, &rowID, &parent, &foreignKeyID); err != nil {
			return fmt.Errorf("read foreign_key_check: %w", err)
		}
		return fmt.Errorf("foreign_key_check failed for table %s row %v parent %s constraint %d", table, rowID, parent, foreignKeyID)
	}
	if err := foreignKeyRows.Err(); err != nil {
		return fmt.Errorf("read foreign_key_check: %w", err)
	}
	return nil
}

func resolveRecoveredAccountName(existingNames map[string]struct{}, baseName string) string {
	trimmed := strings.TrimSpace(baseName)
	if trimmed == "" {
		trimmed = "Recovered"
	}
	candidate := fmt.Sprintf("%s (Recovered)", trimmed)
	if _, exists := existingNames[strings.ToLower(candidate)]; !exists {
		existingNames[strings.ToLower(candidate)] = struct{}{}
		return candidate
	}
	for i := 2; ; i++ {
		candidate = fmt.Sprintf("%s (Recovered %d)", trimmed, i)
		if _, exists := existingNames[strings.ToLower(candidate)]; !exists {
			existingNames[strings.ToLower(candidate)] = struct{}{}
			return candidate
		}
	}
}

func (s *Server) performCrossAccountMergeRestore(ctx context.Context, tmpPath string, uploadedVersion, currentVersion int, restoreCtx restoreContext) (restoredVersion int, returnErr error) {
	targetUserID := strings.TrimSpace(restoreCtx.RequesterUserID)
	if targetUserID == "" {
		return 0, fmt.Errorf("跨账号恢复失败：无法识别目标账号")
	}

	restoreScope := normalizeRestoreScope(restoreCtx.DataScope)
	if restoreScope == "" {
		return 0, fmt.Errorf("跨账号恢复失败：restore_scope 必须是 work、life 或 both")
	}

	// Validate the exact source image before allowing a migration to write to
	// it. Cross-account uploads are untrusted, and SQLite's integrity_check does
	// not include foreign keys, so both checks are required here.
	if err := validateSQLiteMagic(tmpPath); err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：备份文件不是有效的 SQLite 数据库")
	}
	preflightDB, err := openExistingSQLiteReadOnly(ctx, tmpPath)
	if err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：无法打开备份文件")
	}
	if err := ensureSQLiteIntegrity(ctx, preflightDB); err != nil {
		preflightDB.Close()
		return 0, fmt.Errorf("跨账号恢复失败：备份完整性校验失败: %w", err)
	}
	var sourceVersion int
	if err := preflightDB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&sourceVersion); err != nil {
		preflightDB.Close()
		return 0, fmt.Errorf("跨账号恢复失败：备份缺少有效的 schema 版本")
	}
	if err := validateRestoreTransactionAmounts(ctx, preflightDB); err != nil {
		preflightDB.Close()
		return 0, fmt.Errorf("跨账号恢复失败：备份交易金额校验失败: %w", err)
	}
	if err := preflightDB.Close(); err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：无法关闭备份文件")
	}
	if sourceVersion != uploadedVersion {
		return 0, fmt.Errorf("跨账号恢复失败：备份版本在校验期间发生变化")
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&currentVersion); err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：无法读取当前数据库版本")
	}
	if sourceVersion > currentVersion {
		return 0, fmt.Errorf("跨账号恢复失败：备份版本 v%d 高于当前数据库版本 v%d", sourceVersion, currentVersion)
	}

	guard := findb.Global()
	releaseRequestWriteLease(ctx)
	previousState := guard.BeginMaintenance(findb.StateRestore)
	keepMaintenanceState := false
	defer func() {
		if keepMaintenanceState {
			guard.EndMaintenance(findb.StateRestore)
			return
		}
		guard.EndMaintenance(previousState)
	}()

	if sourceVersion < currentVersion {
		backupDB, err := sql.Open("sqlite3", tmpPath)
		if err != nil {
			return 0, fmt.Errorf("无法打开备份文件")
		}
		if err := findb.MigrateWithinMaintenance(ctx, backupDB); err != nil {
			backupDB.Close()
			return 0, fmt.Errorf("跨账号恢复失败：备份数据迁移失败: %s", err.Error())
		}
		if err := backupDB.Close(); err != nil {
			return 0, fmt.Errorf("跨账号恢复失败：无法关闭迁移后的备份文件")
		}
	}

	// Reopen the migrated image read-only and validate it again. Migrations can
	// expose new constraints, and all subsequent imports must read this exact
	// validated connection.
	srcDB, err := openExistingSQLiteReadOnly(ctx, tmpPath)
	if err != nil {
		return 0, fmt.Errorf("无法打开备份文件")
	}
	defer srcDB.Close()
	if err := ensureSQLiteIntegrity(ctx, srcDB); err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：迁移后备份完整性校验失败: %w", err)
	}
	if err := srcDB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&restoredVersion); err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：无法读取迁移后的备份版本")
	}
	if restoredVersion != currentVersion {
		return 0, fmt.Errorf("跨账号恢复失败：迁移后备份版本 v%d 与当前数据库版本 v%d 不一致", restoredVersion, currentVersion)
	}
	if err := validateRestoreTransactionAmounts(ctx, srcDB); err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：迁移后备份交易金额校验失败: %w", err)
	}

	sourceUserID := strings.TrimSpace(restoreCtx.BackupUserID)
	if sourceUserID == "" {
		ownerRow := srcDB.QueryRow(`SELECT id FROM users WHERE deleted_at IS NULL ORDER BY CASE WHEN role='admin' THEN 0 ELSE 1 END, CASE WHEN email_verified=1 THEN 0 ELSE 1 END, created_at ASC LIMIT 1`)
		if err := ownerRow.Scan(&sourceUserID); err != nil {
			return 0, fmt.Errorf("跨账号恢复失败：无法识别备份所有者")
		}
	}
	log.Printf("[RESTORE_EXECUTE] restore_mode=%s request_user_id=%s backup_user_id=%s", restoreCtx.RestoreMode, targetUserID, sourceUserID)
	var sourceAttachmentCount int
	if err := srcDB.QueryRowContext(ctx, `SELECT COUNT(1) FROM attachments WHERE user_id = ?`, sourceUserID).Scan(&sourceAttachmentCount); err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：无法核对备份附件")
	}
	if sourceAttachmentCount > 0 && (strings.TrimSpace(restoreCtx.AttachmentRestoreDir) == "" || s.attachmentSvc == nil) {
		return 0, fmt.Errorf("跨账号恢复失败：备份包含附件，但没有可验证的附件归档或附件存储")
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：无法开启事务")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	importBatchID := uuid.NewString()
	var attachmentJournal *attachmentRollbackJournal
	var restoreOperation *durableRestoreOperation
	attachmentRestoreStarted := false
	commitAttempted := false
	txCommitted := false
	defer func() {
		retainFailure := func(failure *restoreRollbackFailure, event string) {
			if attachmentJournal != nil {
				retainedArtifact, err := attachmentJournal.retain(importBatchID, restoreOperation)
				failure.retainedAttachmentArtifact = retainedArtifact
				failure.attachmentRetentionErr = err
				log.Printf("%s restore_id=%s attachment_artifact=%s retention_error=%v", event, safeRestoreArtifactID(importBatchID), retainedArtifact, err)
			}
			returnErr = failure
			keepMaintenanceState = true
		}
		terminalCleanupFailure := func(err error) {
			failure := &restoreRollbackFailure{
				restoreErr:  errors.Join(returnErr, err),
				rollbackErr: errors.New("终态操作日志已保留，启动恢复将重试工件清理"),
			}
			if attachmentJournal != nil {
				failure.retainedAttachmentArtifact = filepath.Base(attachmentJournal.dir)
			}
			returnErr = failure
			keepMaintenanceState = true
			log.Printf("CROSS_RESTORE_TERMINAL_CLEANUP_REQUIRED restore_id=%s error=%v", safeRestoreArtifactID(importBatchID), err)
		}
		finishTerminalArtifacts := func() error {
			if restoreOperation != nil && restoreOperation.marker.Phase == restoreOperationPhasePrepared {
				return errors.New("refusing to clean an unresolved cross-account restore marker")
			}
			if attachmentJournal != nil {
				if err := attachmentJournal.cleanupPreservingMarker(restoreOperation); err != nil {
					return fmt.Errorf("cleanup attachment rollback journal: %w", err)
				}
			}
			if restoreOperation != nil {
				if err := restoreOperation.removeMarker(); err != nil {
					return fmt.Errorf("cleanup terminal restore marker: %w", err)
				}
			}
			return nil
		}

		var databaseRollbackErr error
		if !commitAttempted {
			rollbackErr := runRestoreCompensation("cross-account database rollback", tx.Rollback)
			if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				databaseRollbackErr = fmt.Errorf("数据库事务回滚错误: %w", rollbackErr)
			}
		}

		if returnErr == nil {
			if err := finishTerminalArtifacts(); err != nil {
				terminalCleanupFailure(err)
			}
			return
		}

		if txCommitted || commitAttempted {
			rollbackReason := errors.New("数据库事务已提交，无法安全自动回滚")
			logEvent := "CROSS_RESTORE_COMMITTED_MANUAL_RECOVERY_REQUIRED"
			if !txCommitted {
				rollbackReason = errors.New("数据库提交结果不确定，无法安全自动回滚")
				logEvent = "CROSS_RESTORE_COMMIT_UNCERTAIN_MANUAL_RECOVERY_REQUIRED"
			}
			rollbackFailure := &restoreRollbackFailure{
				restoreErr:  returnErr,
				rollbackErr: rollbackReason,
			}
			retainFailure(rollbackFailure, logEvent)
			return
		}

		var attachmentRollbackErr error
		if attachmentRestoreStarted && attachmentJournal != nil {
			attachmentRollbackErr = runRestoreCompensation("cross-account attachment rollback", func() error {
				return attachmentJournal.rollback(ctx)
			})
		}
		rollbackErr := errors.Join(databaseRollbackErr, wrapRestoreError("附件回滚错误", attachmentRollbackErr))
		if rollbackErr != nil {
			rollbackFailure := &restoreRollbackFailure{restoreErr: returnErr, rollbackErr: rollbackErr}
			retainFailure(rollbackFailure, "CROSS_RESTORE_ATTACHMENT_MANUAL_RECOVERY_REQUIRED")
			return
		}

		if restoreOperation != nil {
			if err := restoreOperation.markRolledBack(); err != nil {
				retainFailure(&restoreRollbackFailure{
					restoreErr:  errors.Join(returnErr, fmt.Errorf("持久化跨账号恢复回滚终态失败: %w", err)),
					rollbackErr: errors.New("为避免丢失崩溃恢复依据，恢复工件已保留"),
				}, "CROSS_RESTORE_ROLLBACK_MARKER_REQUIRED")
				return
			}
		}
		if err := finishTerminalArtifacts(); err != nil {
			terminalCleanupFailure(err)
		}
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			returnErr = restorePanicError("跨账号恢复", recovered)
			log.Printf("CROSS_RESTORE_PANIC restore_id=%s commit_attempted=%t committed=%t error=%q", safeRestoreArtifactID(importBatchID), commitAttempted, txCommitted, returnErr)
			panic(recovered)
		}
	}()
	recoveryTimestamp := now
	sourceBackupID := strings.TrimSpace(sourceUserID)
	_ = srcDB.QueryRowContext(ctx, `SELECT COALESCE(user_id,'') || '@' || COALESCE(created_at,'') FROM backup_metadata ORDER BY created_at DESC LIMIT 1`).Scan(&sourceBackupID)
	if strings.TrimSpace(sourceBackupID) == "" {
		sourceBackupID = strings.TrimSpace(sourceUserID)
	}
	accountMap := map[string]string{}
	categoryMap := map[string]string{}
	projectMap := map[string]string{}
	groupMap := map[string]string{}
	transactionMap := map[string]string{}
	attachmentFiles := []attachmentRestoreFile{}
	logTableRows := func(table string, rowsInserted int64) {
		log.Printf("[RESTORE_EXECUTE] table=%s rows_inserted=%d request_user_id=%s backup_user_id=%s restore_mode=%s", table, rowsInserted, targetUserID, sourceUserID, restoreCtx.RestoreMode)
	}
	logTableRowsByMode := func(table, mode string, rowsInserted int64) {
		log.Printf("RESTORE_TABLE table=%s mode=%s rows_inserted=%d request_user_id=%s backup_user_id=%s restore_mode=%s", table, strings.ToUpper(mode), rowsInserted, targetUserID, sourceUserID, restoreCtx.RestoreMode)
	}
	var insertedAccounts int64
	var insertedCategories int64
	var insertedProjects int64
	var insertedTransactions int64
	var insertedAttachments int64
	var skippedDuplicateTransactions int64
	var renamedAccounts int64
	insertedAccountsByMode := map[string]int64{string(model.ModeWork): 0, string(model.ModeLife): 0}
	insertedTransactionsByMode := map[string]int64{string(model.ModeWork): 0, string(model.ModeLife): 0}
	finishRows := func(resultRows *sql.Rows, failureMessage string) error {
		iterationErr := resultRows.Err()
		closeErr := resultRows.Close()
		if iterationErr != nil || closeErr != nil {
			return errors.New(failureMessage)
		}
		return nil
	}

	// accounts: deterministic insert + conflict-safe rename in same mode
	rows, err := tx.QueryContext(ctx, `SELECT id, lower(name), type, currency FROM accounts WHERE user_id=?`, targetUserID)
	if err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：读取目标账户失败")
	}
	existingAccounts := map[string]string{}
	existingNamesByMode := map[string]map[string]struct{}{
		string(model.ModeWork): {},
		string(model.ModeLife): {},
	}
	for rows.Next() {
		var id, lname, typ, currency string
		if err := rows.Scan(&id, &lname, &typ, &currency); err != nil {
			rows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：读取目标账户失败")
		}
		mode := modeFromAccountType(typ)
		existingAccounts[mode+"|"+lname+"|"+currency] = id
		existingNamesByMode[mode][lname] = struct{}{}
	}
	if err := finishRows(rows, "跨账号恢复失败：读取目标账户失败"); err != nil {
		return 0, err
	}

	accRows, err := srcDB.QueryContext(ctx, `SELECT id, name, type, currency, is_active FROM accounts WHERE user_id = ?`, sourceUserID)
	if err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：读取备份账户失败")
	}
	for accRows.Next() {
		var oldID, name, typ, currency string
		var active int
		if err := accRows.Scan(&oldID, &name, &typ, &currency, &active); err != nil {
			accRows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：读取备份账户失败")
		}
		mode := modeFromAccountType(typ)
		if !scopeAllowsMode(restoreScope, mode) {
			continue
		}
		key := mode + "|" + strings.ToLower(name) + "|" + currency
		targetName := name
		if _, ok := existingAccounts[key]; ok {
			targetName = resolveRecoveredAccountName(existingNamesByMode[mode], name)
			renamedAccounts++
			log.Printf("RESTORE_ACCOUNT name=%q renamed_to=%q mode=%s action=rename_conflict request_user_id=%s backup_user_id=%s", name, targetName, strings.ToUpper(mode), targetUserID, sourceUserID)
		}
		newID := deterministicRestoreID("account", sourceUserID, targetUserID, oldID)
		res, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO accounts (id, user_id, name, type, currency, balance_cents, version, is_active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 0, 1, ?, ?, ?)
		`, newID, targetUserID, targetName, typ, currency, active, now, now)
		if err != nil {
			accRows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：写入账户失败")
		}
		if affected, err := res.RowsAffected(); err == nil {
			insertedAccounts += affected
			insertedAccountsByMode[mode] += affected
		}
		log.Printf("RESTORE_ACCOUNT name=%q target_name=%q mode=%s action=insert new_id=%s request_user_id=%s backup_user_id=%s", name, targetName, strings.ToUpper(mode), newID, targetUserID, sourceUserID)
		accountMap[oldID] = newID
		existingAccounts[mode+"|"+strings.ToLower(targetName)+"|"+currency] = newID
		existingNamesByMode[mode][strings.ToLower(targetName)] = struct{}{}
	}
	if err := finishRows(accRows, "跨账号恢复失败：读取备份账户失败"); err != nil {
		return 0, err
	}
	logTableRows("accounts", insertedAccounts)
	logTableRowsByMode("accounts", string(model.ModeWork), insertedAccountsByMode[string(model.ModeWork)])
	logTableRowsByMode("accounts", string(model.ModeLife), insertedAccountsByMode[string(model.ModeLife)])
	log.Printf("RESTORE_TABLE table=accounts renamed_conflicts=%d request_user_id=%s backup_user_id=%s restore_mode=%s", renamedAccounts, targetUserID, sourceUserID, restoreCtx.RestoreMode)

	// categories: deterministic dedup by (name,type)
	catRows, err := tx.QueryContext(ctx, `SELECT id, lower(name), type FROM categories WHERE user_id=?`, targetUserID)
	if err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：读取目标分类失败")
	}
	existingCategories := map[string]string{}
	for catRows.Next() {
		var id, lname, typ string
		if err := catRows.Scan(&id, &lname, &typ); err != nil {
			catRows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：读取目标分类失败")
		}
		existingCategories[typ+"|"+lname] = id
	}
	if err := finishRows(catRows, "跨账号恢复失败：读取目标分类失败"); err != nil {
		return 0, err
	}

	type pendingParent struct{ child, parent string }
	var parentFixes []pendingParent
	srcCatRows, err := srcDB.QueryContext(ctx, `SELECT id, name, type, parent_id, sort_order, is_active FROM categories WHERE user_id=?`, sourceUserID)
	if err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：读取备份分类失败")
	}
	for srcCatRows.Next() {
		var oldID, name, typ string
		var parent sql.NullString
		var sort int
		var active int
		if err := srcCatRows.Scan(&oldID, &name, &typ, &parent, &sort, &active); err != nil {
			srcCatRows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：读取备份分类失败")
		}
		key := typ + "|" + strings.ToLower(name)
		if existingID, ok := existingCategories[key]; ok {
			categoryMap[oldID] = existingID
		} else {
			newID := deterministicRestoreID("category", sourceUserID, targetUserID, oldID)
			res, err := tx.ExecContext(ctx, `
				INSERT OR IGNORE INTO categories (id, user_id, name, type, parent_id, sort_order, is_active, created_at, version)
				VALUES (?, ?, ?, ?, NULL, ?, ?, ?, 1)
			`, newID, targetUserID, name, typ, sort, active, now)
			if err != nil {
				srcCatRows.Close()
				return 0, fmt.Errorf("跨账号恢复失败：写入分类失败")
			}
			if affected, err := res.RowsAffected(); err == nil {
				insertedCategories += affected
			}
			categoryMap[oldID] = newID
			existingCategories[key] = newID
		}
		if parent.Valid {
			parentFixes = append(parentFixes, pendingParent{child: oldID, parent: parent.String})
		}
	}
	if err := finishRows(srcCatRows, "跨账号恢复失败：读取备份分类失败"); err != nil {
		return 0, err
	}
	logTableRows("categories", insertedCategories)
	for _, pf := range parentFixes {
		childID, okChild := categoryMap[pf.child]
		parentID, okParent := categoryMap[pf.parent]
		if !okChild || !okParent || childID == parentID {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE categories SET parent_id=? WHERE id=? AND user_id=?`, parentID, childID, targetUserID); err != nil {
			return 0, fmt.Errorf("跨账号恢复失败：更新分类层级失败")
		}
	}

	// projects: no user_id, dedup by code then name
	projRows, err := tx.QueryContext(ctx, `SELECT id, code, lower(name) FROM projects`)
	if err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：读取目标项目失败")
	}
	projectByCode := map[string]string{}
	projectByName := map[string]string{}
	for projRows.Next() {
		var id, code, lname string
		if err := projRows.Scan(&id, &code, &lname); err != nil {
			projRows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：读取目标项目失败")
		}
		projectByCode[code] = id
		projectByName[lname] = id
	}
	if err := finishRows(projRows, "跨账号恢复失败：读取目标项目失败"); err != nil {
		return 0, err
	}

	srcProjRows, err := srcDB.QueryContext(ctx, `SELECT id, name, code, created_at FROM projects`)
	if err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：读取备份项目失败")
	}
	for srcProjRows.Next() {
		var oldID, name, code string
		var createdAt int64
		if err := srcProjRows.Scan(&oldID, &name, &code, &createdAt); err != nil {
			srcProjRows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：读取备份项目失败")
		}
		if id, ok := projectByCode[code]; ok {
			projectMap[oldID] = id
			continue
		}
		if id, ok := projectByName[strings.ToLower(name)]; ok {
			projectMap[oldID] = id
			continue
		}
		newID := deterministicRestoreID("project", sourceUserID, targetUserID, oldID)
		newCode := code
		if _, exists := projectByCode[newCode]; exists {
			newCode = fmt.Sprintf("%s-%s", code, newID[:6])
		}
		res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO projects (id, name, code, created_at, version) VALUES (?, ?, ?, ?, 1)`, newID, name, newCode, createdAt)
		if err != nil {
			srcProjRows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：写入项目失败")
		}
		if affected, err := res.RowsAffected(); err == nil {
			insertedProjects += affected
		}
		projectMap[oldID] = newID
		projectByCode[newCode] = newID
		projectByName[strings.ToLower(name)] = newID
	}
	if err := finishRows(srcProjRows, "跨账号恢复失败：读取备份项目失败"); err != nil {
		return 0, err
	}
	logTableRows("projects", insertedProjects)

	srcTxRows, err := srcDB.QueryContext(ctx, `
		SELECT id, group_id, direction, account_id, amount_cents, currency, exchange_rate,
		       COALESCE(exchange_rate_source, 'legacy'), exchange_rate_at, base_amount_cents, base_currency,
		       type, category_id, category, reimb_status, reimb_to_account, project_id, project,
		       note, uploaded, txn_date, transaction_time, reported_at, reimbursed_at,
		       created_at, updated_at, mode
		FROM transactions
		WHERE user_id = ?
		ORDER BY created_at ASC, id ASC
	`, sourceUserID)
	if err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：读取备份交易失败")
	}
	for srcTxRows.Next() {
		var oldID, oldGroupID, dir, oldAccountID, currency, exchangeRateSource, baseCurrency, typ, txnMode string
		var amount, baseAmount int64
		var exchangeRate float64
		var exchangeRateAt sql.NullInt64
		var oldCategoryID, category, reimbStatus sql.NullString
		var oldReimbToAccount, oldProjectID, project, note sql.NullString
		var transactionTime, reportedAt, reimbursedAt sql.NullInt64
		var uploaded int
		var txnDate, createdAt, updatedAt string
		if err := srcTxRows.Scan(&oldID, &oldGroupID, &dir, &oldAccountID, &amount, &currency, &exchangeRate,
			&exchangeRateSource, &exchangeRateAt, &baseAmount, &baseCurrency,
			&typ, &oldCategoryID, &category, &reimbStatus, &oldReimbToAccount, &oldProjectID, &project,
			&note, &uploaded, &txnDate, &transactionTime, &reportedAt, &reimbursedAt,
			&createdAt, &updatedAt, &txnMode); err != nil {
			srcTxRows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：读取备份交易失败")
		}
		txnMode = strings.ToLower(strings.TrimSpace(txnMode))
		if txnMode != string(model.ModeWork) && txnMode != string(model.ModeLife) {
			txnMode = string(model.ModeWork)
		}
		if !scopeAllowsMode(restoreScope, txnMode) {
			continue
		}
		mappedAccountID := accountMap[oldAccountID]
		if mappedAccountID == "" {
			continue
		}
		var mappedCategoryID sql.NullString
		if oldCategoryID.Valid {
			if v := categoryMap[oldCategoryID.String]; v != "" {
				mappedCategoryID = sql.NullString{String: v, Valid: true}
			}
		}
		var mappedProjectID sql.NullString
		if oldProjectID.Valid {
			if v := projectMap[oldProjectID.String]; v != "" {
				mappedProjectID = sql.NullString{String: v, Valid: true}
			}
		}
		var mappedReimbTo sql.NullString
		if oldReimbToAccount.Valid {
			if v := accountMap[oldReimbToAccount.String]; v != "" {
				mappedReimbTo = sql.NullString{String: v, Valid: true}
			}
		}
		categoryText := ""
		if category.Valid {
			categoryText = category.String
		}
		reimbText := "none"
		if reimbStatus.Valid && strings.TrimSpace(reimbStatus.String) != "" {
			reimbText = reimbStatus.String
		}
		projectText := ""
		if project.Valid {
			projectText = project.String
		}
		noteText := ""
		if note.Valid {
			noteText = note.String
		}
		fp := strings.Join([]string{
			mappedAccountID, mappedCategoryID.String, mappedProjectID.String, dir, typ,
			fmt.Sprint(amount), currency, fmt.Sprint(exchangeRate), fmt.Sprint(baseAmount),
			baseCurrency, txnDate, noteText, txnMode,
		}, "|")
		restoreTxnHashSum := sha256.Sum256([]byte(fp))
		restoreTxnHash := hex.EncodeToString(restoreTxnHashSum[:])

		newID := deterministicRestoreID("transaction", sourceUserID, targetUserID, oldID)
		mappedGroupID, ok := groupMap[oldGroupID]
		if !ok {
			mappedGroupID = deterministicRestoreID("group", sourceUserID, targetUserID, oldGroupID)
			groupMap[oldGroupID] = mappedGroupID
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO transactions (
				id, user_id, group_id, direction, account_id,
				amount_cents, currency, exchange_rate, exchange_rate_source, exchange_rate_at,
				base_amount_cents, base_currency,
				type, category_id, category,
				reimb_status, reimb_to_account, reimbursement_id,
				project_id, project, mode,
				note, uploaded, idempotency_key, txn_date, transaction_time,
				reported_at, reimbursed_at, created_at, updated_at, version,
				restore_source_backup_id, restore_import_batch_id, restore_recovered_at, restore_txn_hash
			) VALUES (
				?, ?, ?, ?, ?, ?, ?,
				?, ?, ?, ?, ?,
				?, ?, ?,
				?, ?, NULL,
				?, ?, ?,
				?, ?, NULL, ?, ?,
				?, ?, ?, ?, 1,
				?, ?, ?, ?
			)
			ON CONFLICT(id) DO NOTHING
		`, newID, targetUserID, mappedGroupID, dir, mappedAccountID,
			amount, currency, exchangeRate, exchangeRateSource, exchangeRateAt, baseAmount, baseCurrency,
			typ, mappedCategoryID, categoryText,
			reimbText, mappedReimbTo,
			mappedProjectID, projectText, txnMode,
			noteText, uploaded, txnDate, transactionTime,
			reportedAt, reimbursedAt, createdAt, updatedAt,
			sourceBackupID, importBatchID, recoveryTimestamp, restoreTxnHash)
		if err != nil {
			srcTxRows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：写入交易失败")
		}
		transactionMap[oldID] = newID
		if affected, err := res.RowsAffected(); err == nil {
			if affected == 0 {
				skippedDuplicateTransactions++
				log.Printf("RESTORE_DUPLICATE table=transactions action=skip reason=deterministic_id old_txn_id=%s mode=%s request_user_id=%s backup_user_id=%s", oldID, strings.ToUpper(txnMode), targetUserID, sourceUserID)
				continue
			}
			insertedTransactions += affected
			insertedTransactionsByMode[txnMode] += affected
		}
	}
	if err := finishRows(srcTxRows, "跨账号恢复失败：读取备份交易失败"); err != nil {
		return 0, err
	}
	logTableRows("transactions", insertedTransactions)
	logTableRowsByMode("transactions", string(model.ModeWork), insertedTransactionsByMode[string(model.ModeWork)])
	logTableRowsByMode("transactions", string(model.ModeLife), insertedTransactionsByMode[string(model.ModeLife)])
	log.Printf("RESTORE_TABLE table=transactions skipped_duplicates=%d request_user_id=%s backup_user_id=%s restore_mode=%s", skippedDuplicateTransactions, targetUserID, sourceUserID, restoreCtx.RestoreMode)
	log.Printf("RESTORE_SUMMARY recovered_work_accounts=%d recovered_life_accounts=%d imported_transactions=%d skipped_duplicates=%d renamed_accounts=%d request_user_id=%s backup_user_id=%s restore_mode=%s scope=%s",
		insertedAccountsByMode[string(model.ModeWork)], insertedAccountsByMode[string(model.ModeLife)], insertedTransactions, skippedDuplicateTransactions, renamedAccounts, targetUserID, sourceUserID, restoreCtx.RestoreMode, restoreScope)

	if strings.TrimSpace(restoreCtx.AttachmentRestoreDir) != "" && s.attachmentSvc != nil {
		srcAttachmentRows, err := srcDB.QueryContext(ctx, `
			SELECT id, transaction_id, storage_key, original_filename, content_type, size_bytes, sha256, kind,
			       ocr_status, ocr_provider, ocr_text, ocr_json, ocr_error, created_at, updated_at
			FROM attachments
			WHERE user_id = ?
			ORDER BY created_at ASC, id ASC
		`, sourceUserID)
		if err != nil {
			return 0, fmt.Errorf("跨账号恢复失败：读取备份附件失败")
		}
		for srcAttachmentRows.Next() {
			var oldID, oldStorageKey, filename, contentType, sha, kind, ocrStatus, createdAt, updatedAt string
			var oldTransactionID, ocrProvider, ocrText, ocrJSON, ocrError sql.NullString
			var sizeBytes int64
			if err := srcAttachmentRows.Scan(&oldID, &oldTransactionID, &oldStorageKey, &filename, &contentType, &sizeBytes, &sha, &kind, &ocrStatus, &ocrProvider, &ocrText, &ocrJSON, &ocrError, &createdAt, &updatedAt); err != nil {
				srcAttachmentRows.Close()
				return 0, fmt.Errorf("跨账号恢复失败：读取备份附件失败")
			}
			var mappedTransactionID sql.NullString
			if oldTransactionID.Valid && strings.TrimSpace(oldTransactionID.String) != "" {
				mappedID := transactionMap[oldTransactionID.String]
				if mappedID == "" {
					continue
				}
				mappedTransactionID = sql.NullString{String: mappedID, Valid: true}
			} else if restoreScope != "both" {
				// An unlinked attachment has no authoritative mode. Importing it
				// during a scoped restore could disclose a receipt from the excluded
				// ledger, so only an explicit full restore may carry it across.
				continue
			}
			newID := deterministicRestoreID("attachment", sourceUserID, targetUserID, oldID)
			ext := strings.ToLower(filepath.Ext(oldStorageKey))
			if ext == "" {
				ext = strings.ToLower(filepath.Ext(filename))
			}
			targetKey := filepath.ToSlash(filepath.Join(safeStorageSegment(targetUserID), newID+ext))
			res, err := tx.ExecContext(ctx, `
				INSERT OR IGNORE INTO attachments (
					id, user_id, transaction_id, storage_key, original_filename, content_type,
					size_bytes, sha256, kind, ocr_status, ocr_provider, ocr_text, ocr_json,
					ocr_error, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, newID, targetUserID, mappedTransactionID, targetKey, filename, contentType, sizeBytes, sha, kind, ocrStatus, ocrProvider, ocrText, ocrJSON, ocrError, createdAt, updatedAt)
			if err != nil {
				srcAttachmentRows.Close()
				return 0, fmt.Errorf("跨账号恢复失败：写入附件失败")
			}
			affected, err := res.RowsAffected()
			if err != nil {
				srcAttachmentRows.Close()
				return 0, fmt.Errorf("跨账号恢复失败：确认附件写入结果失败")
			}
			if affected == 0 {
				var matchingExisting int
				if strings.TrimSpace(sha) != "" {
					err = tx.QueryRowContext(ctx, `
					SELECT COUNT(*)
					FROM attachments
					WHERE id = ? AND user_id = ? AND storage_key = ?
					  AND COALESCE(transaction_id, '') = COALESCE(?, '')
					  AND original_filename = ? AND content_type = ?
					  AND size_bytes = ? AND lower(sha256) = lower(?) AND kind = ?
				`, newID, targetUserID, targetKey, mappedTransactionID,
						filename, contentType, sizeBytes, sha, kind,
					).Scan(&matchingExisting)
				}
				if err != nil {
					srcAttachmentRows.Close()
					return 0, fmt.Errorf("跨账号恢复失败：校验已有附件失败")
				}
				if matchingExisting != 1 {
					// A deterministic ID or storage-key collision must never
					// overwrite unrelated live bytes.
					log.Printf("RESTORE_DUPLICATE table=attachments action=skip reason=metadata_conflict old_attachment_id=%s request_user_id=%s backup_user_id=%s", oldID, targetUserID, sourceUserID)
					continue
				}
			}
			// A previously deleted deterministic object may still be queued
			// for asynchronous removal. Cancel that tombstone in the same SQL
			// transaction before restoring the bytes, including idempotent
			// re-imports whose metadata row already existed.
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM attachment_deletion_queue WHERE storage_key = ?`, targetKey,
			); err != nil {
				srcAttachmentRows.Close()
				return 0, fmt.Errorf("跨账号恢复失败：取消附件删除任务失败")
			}
			if mappedTransactionID.Valid {
				if _, err := tx.ExecContext(ctx, `UPDATE transactions SET attachment_key = COALESCE(attachment_key, ?), updated_at = ? WHERE id = ? AND user_id = ?`, targetKey, now, mappedTransactionID.String, targetUserID); err != nil {
					srcAttachmentRows.Close()
					return 0, fmt.Errorf("跨账号恢复失败：关联附件与交易失败")
				}
			}
			insertedAttachments += affected
			attachmentFiles = append(attachmentFiles, attachmentRestoreFile{SourceKey: oldStorageKey, TargetKey: targetKey, SizeBytes: sizeBytes, SHA256: sha})
		}
		if err := finishRows(srcAttachmentRows, "跨账号恢复失败：读取备份附件失败"); err != nil {
			return 0, err
		}
		journalRoot, err := restoreSafetyArtifactDirectory(s.dbPath)
		if err != nil {
			return 0, fmt.Errorf("跨账号恢复失败：附件回滚目录无效: %w", err)
		}
		if len(attachmentFiles) > 0 {
			restoreOperation, err = beginCrossRestorePreparation(s.dbPath, importBatchID)
			if err != nil {
				return 0, fmt.Errorf("跨账号恢复失败：创建准备阶段恢复操作日志失败: %w", err)
			}
			attachmentJournal, err = prepareAttachmentRollbackJournal(ctx, journalRoot, attachmentFiles, s.attachmentSvc, restoreOperation)
			if err != nil {
				return 0, fmt.Errorf("跨账号恢复失败：创建附件回滚日志失败: %w", err)
			}
			if err := restoreOperation.attachJournal(attachmentJournal); err != nil {
				return 0, fmt.Errorf("跨账号恢复失败：发布附件回滚日志失败: %w", err)
			}
			if err := restoreOperation.markPrepared(); err != nil {
				return 0, fmt.Errorf("跨账号恢复失败：持久化附件回滚准备状态失败: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO restore_operations(operation_id, batch_id, kind, created_at)
				VALUES (?, ?, 'cross_merge', ?)
			`, restoreOperation.marker.OperationID, importBatchID, time.Now().UTC().Unix()); err != nil {
				return 0, fmt.Errorf("跨账号恢复失败：记录恢复提交账本失败: %w", err)
			}
			attachmentRestoreStarted = true
		}
		if err := restoreAttachmentFiles(ctx, restoreCtx.AttachmentRestoreDir, attachmentFiles, s.attachmentSvc); err != nil {
			return 0, fmt.Errorf("跨账号恢复失败：恢复附件文件失败: %w", err)
		}
	}
	logTableRows("attachments", insertedAttachments)

	// recalculate balances idempotently
	if _, err := tx.ExecContext(ctx, `
		UPDATE accounts
		SET balance_cents = (
			SELECT COALESCE(SUM(CASE t.direction WHEN 'credit' THEN t.base_amount_cents ELSE -t.base_amount_cents END), 0)
			FROM transactions t WHERE t.account_id = accounts.id
		),
		updated_at = ?,
		version = version + 1
		WHERE user_id = ?
	`, now, targetUserID); err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：更新账户余额失败")
	}

	if err := s.validateRestoreIntegrity(ctx, tx, targetUserID); err != nil {
		return 0, err
	}
	commitAttempted = true
	commit := tx.Commit
	if s.crossRestoreCommit != nil {
		commit = func() error { return s.crossRestoreCommit(tx) }
	}
	if err := commit(); err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：提交失败: %w", err)
	}
	txCommitted = true
	if restoreOperation != nil {
		if err := ensureRestoreDatabaseDurable(ctx, s.db, s.dbPath); err != nil {
			return 0, fmt.Errorf("跨账号恢复已提交，但数据库持久化屏障失败: %w", err)
		}
		if err := restoreOperation.markCommitted(); err != nil {
			return 0, fmt.Errorf("跨账号恢复已提交，但操作日志更新失败: %w", err)
		}
	}

	if err := findb.ReapplyPragmas(ctx, s.db); err != nil {
		log.Printf("[WARN] cross-restore: reapply pragmas: %v", err)
	}
	if err := s.acctSvc.EnsureDefaultAccounts(ctx, targetUserID); err != nil {
		log.Printf("[WARN] cross-restore: ensure default accounts failed: %v", err)
	}
	return restoredVersion, nil
}

func (s *Server) validateRestoreIntegrity(ctx context.Context, tx *sql.Tx, targetUserID string) error {
	var wrongAccountOwnership int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM transactions t
		JOIN accounts a ON a.id = t.account_id
		WHERE t.user_id = ? AND a.user_id <> ?
	`, targetUserID, targetUserID).Scan(&wrongAccountOwnership); err != nil {
		return fmt.Errorf("恢复校验失败：无法校验账户归属")
	}
	if wrongAccountOwnership > 0 {
		return fmt.Errorf("恢复校验失败：存在交易引用其他用户账户")
	}

	var missingCategoryFK int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM transactions t
		LEFT JOIN categories c ON c.id = t.category_id
		WHERE t.user_id = ? AND t.category_id IS NOT NULL AND c.id IS NULL
	`, targetUserID).Scan(&missingCategoryFK); err != nil {
		return fmt.Errorf("恢复校验失败：无法校验分类外键")
	}
	if missingCategoryFK > 0 {
		return fmt.Errorf("恢复校验失败：存在交易引用不存在的分类")
	}

	var missingProjectFK int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM transactions t
		LEFT JOIN projects p ON p.id = t.project_id
		WHERE t.user_id = ? AND t.project_id IS NOT NULL AND p.id IS NULL
	`, targetUserID).Scan(&missingProjectFK); err != nil {
		return fmt.Errorf("恢复校验失败：无法校验项目外键")
	}
	if missingProjectFK > 0 {
		return fmt.Errorf("恢复校验失败：存在交易引用不存在的项目")
	}

	var duplicateCategories int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM (
			SELECT lower(name), type, COUNT(1)
			FROM categories
			WHERE user_id = ?
			GROUP BY lower(name), type
			HAVING COUNT(1) > 1
		)
	`, targetUserID).Scan(&duplicateCategories); err != nil {
		return fmt.Errorf("恢复校验失败：无法校验分类重复")
	}
	if duplicateCategories > 0 {
		return fmt.Errorf("恢复校验失败：分类存在重复")
	}

	return nil
}

// ─── Legacy upload-verification helpers (not registered as public routes) ───

// generateCode produces a cryptographically random 6-digit code.
func generateCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1_000_000))
	return fmt.Sprintf("%06d", n.Int64())
}

// maskEmail masks the middle of an email address for privacy (e.g. u***r@example.com).
func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return "***"
	}
	local := parts[0]
	if len(local) <= 2 {
		return local[:1] + "***@" + parts[1]
	}
	return local[:1] + strings.Repeat("*", len(local)-2) + local[len(local)-1:] + "@" + parts[1]
}

// handleRestoreRequest accepts a backup .db or .zip file, extracts the owner email,
// sends a 6-digit verification code, and returns a restore_id for step 2.
func (s *Server) handleRestoreRequest(c *gin.Context) {
	fh, err := restoreFormFile(c, maxRestoreRequestSize)
	if err != nil {
		if restoreBodyTooLarge(err) {
			fail(c, http.StatusRequestEntityTooLarge, 40004, "恢复请求体过大")
			return
		}
		fail(c, 400, 40001, "请上传数据库文件（multipart field: file）")
		return
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext != ".db" && ext != ".zip" {
		fail(c, 400, 40002, "仅接受 .db 或 .zip 备份文件")
		return
	}
	if fh.Size > maxRestoreSize {
		fail(c, 400, 40004, fmt.Sprintf("文件过大（%d MB），上限 100 MB", fh.Size>>20))
		return
	}

	tmpPath, attachmentDir, cleanupPath, cleanup, err := extractRestoreUpload(fh)
	if err != nil {
		fail(c, 400, 40003, "备份文件解压或读取失败")
		return
	}

	if err := validateSQLiteMagic(tmpPath); err != nil {
		cleanup()
		fail(c, 400, 40003, "文件不是有效的 SQLite 数据库")
		return
	}

	srcDB, err := sql.Open("sqlite3", tmpPath+"?mode=ro")
	if err != nil {
		cleanup()
		fail(c, 500, 50002, "无法打开备份文件")
		return
	}
	if err := ensureSQLiteIntegrity(c.Request.Context(), srcDB); err != nil {
		srcDB.Close()
		cleanup()
		fail(c, 400, 40003, "备份文件完整性校验失败")
		return
	}
	if err := srcDB.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(new(int)); err != nil {
		srcDB.Close()
		cleanup()
		fail(c, 400, 40005, "备份文件缺少 schema_migrations 表，不是有效的 FinArch 备份")
		return
	}
	identity, err := readBackupIdentity(srcDB)
	srcDB.Close()
	if err != nil {
		cleanup()
		if strings.Contains(err.Error(), "备份文件中未找到用户数据") {
			fail(c, 400, 40007, "备份文件中未找到用户数据")
			return
		}
		fail(c, 400, 40007, "读取备份所有者信息失败")
		return
	}

	ownerEmail := strings.TrimSpace(identity.MetadataUserEmail)
	if ownerEmail == "" {
		ownerEmail = strings.TrimSpace(identity.FallbackOwnerEmail)
	}
	ownerName := strings.TrimSpace(identity.FallbackOwnerName)
	if ownerEmail == "" {
		cleanup()
		fail(c, 400, 40007, "备份文件中的用户没有邮箱地址")
		return
	}

	code := generateCode()
	restoreID := uuid.NewString()
	pr := &pendingRestore{
		code:          code,
		tmpPath:       tmpPath,
		attachmentDir: attachmentDir,
		cleanupPath:   cleanupPath,
		expiresAt:     time.Now().Add(10 * time.Minute),
		email:         ownerEmail,
		name:          ownerName,
	}
	if !s.storePendingRestore(restoreID, pr) {
		cleanupPendingRestore(pr)
		fail(c, http.StatusTooManyRequests, "RESTORE_CAPACITY_REACHED", "恢复会话过多，请稍后重试")
		return
	}

	if err := s.emailSvc.SendRestoreCode(ownerEmail, ownerName, code); err != nil {
		s.pendingRestores.CompareAndDelete(restoreID, pr)
		cleanupPendingRestore(pr)
		log.Printf("[ERROR] disaster-restore: send code to %s: %v", maskEmail(ownerEmail), err)
		fail(c, 500, 50005, "验证码发送失败，请检查邮件服务配置")
		return
	}

	log.Printf("[INFO] disaster-restore: code sent to %s (restore_id=%s)", maskEmail(ownerEmail), restoreID[:8])

	ok(c, gin.H{
		"restore_id":   restoreID,
		"masked_email": maskEmail(ownerEmail),
		"expires_in":   600,
	})
}

// handleRestoreConfirm verifies the 6-digit code and performs the restore.
func (s *Server) handleRestoreConfirm(c *gin.Context) {
	var req struct {
		RestoreID string `json:"restore_id" binding:"required"`
		Code      string `json:"code"       binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, 40001, "请提供 restore_id 和验证码")
		return
	}

	val, ok_ := s.pendingRestores.Load(req.RestoreID)
	if !ok_ {
		fail(c, 400, 40008, "恢复会话不存在或已过期，请重新上传备份文件")
		return
	}
	pr := val.(*pendingRestore)

	pr.mu.Lock()
	if pr.consumed {
		pr.mu.Unlock()
		fail(c, 400, 40008, "恢复会话不存在或已过期，请重新上传备份文件")
		return
	}
	if !time.Now().Before(pr.expiresAt) {
		pr.consumed = true
		pr.mu.Unlock()
		s.pendingRestores.CompareAndDelete(req.RestoreID, pr)
		cleanupPendingRestore(pr)
		fail(c, 400, 40009, "验证码已过期，请重新上传备份文件")
		return
	}
	if pr.attempts >= 5 {
		pr.consumed = true
		pr.mu.Unlock()
		s.pendingRestores.CompareAndDelete(req.RestoreID, pr)
		cleanupPendingRestore(pr)
		fail(c, 400, 40010, "验证码错误次数过多，请重新上传备份文件")
		return
	}
	if req.Code != pr.code {
		pr.attempts++
		remaining := 5 - pr.attempts
		exhausted := remaining == 0
		if exhausted {
			pr.consumed = true
		}
		pr.mu.Unlock()
		if exhausted {
			s.pendingRestores.CompareAndDelete(req.RestoreID, pr)
			cleanupPendingRestore(pr)
		}
		fail(c, 400, 40011, fmt.Sprintf("验证码错误，剩余 %d 次尝试", remaining))
		return
	}

	pr.consumed = true
	tmpPath := pr.tmpPath
	attachmentDir := pr.attachmentDir
	pr.mu.Unlock()
	s.pendingRestores.CompareAndDelete(req.RestoreID, pr)
	defer cleanupPendingRestore(pr)

	srcDB, err := sql.Open("sqlite3", tmpPath+"?mode=ro")
	if err != nil {
		fail(c, 500, 50002, "无法打开备份文件")
		return
	}
	var uploadedVersion int
	_ = srcDB.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&uploadedVersion)
	srcDB.Close()

	var currentVersion int
	_ = s.db.QueryRowContext(c.Request.Context(), `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&currentVersion)
	if uploadedVersion > currentVersion {
		fail(c, 400, 40006, fmt.Sprintf("备份文件版本 (v%d) 高于当前系统版本 (v%d)，请先升级系统再恢复", uploadedVersion, currentVersion))
		return
	}

	engineResult, err := s.executeRestoreEngine(c.Request.Context(), restoreEngineRequest{
		RestoreID:            req.RestoreID,
		Source:               restoreSourceDisasterRecover,
		SnapshotID:           "upload_confirm",
		TempDBPath:           tmpPath,
		UploadedVersion:      uploadedVersion,
		SchemaBefore:         currentVersion,
		AttachmentRestoreDir: attachmentDir,
	})
	if err != nil {
		fail(c, 500, 50003, err.Error())
		return
	}

	ok(c, gin.H{
		"message":          "灾难恢复成功！数据库已恢复",
		"restored_version": uploadedVersion,
		"migrated_to":      engineResult.SchemaAfter,
	})
}

// ─── Account handlers ────────────────────────────────────────────────────────

func (s *Server) handleListAccounts(c *gin.Context) {
	mode, modeOK := parseMode(c.Query("mode"))
	if !modeOK {
		fail(c, 400, 40001, "invalid mode")
		return
	}
	accounts, err := s.acctSvc.ListAccounts(c.Request.Context(), userID(c))
	if err != nil {
		failInternal(c, err)
		return
	}

	if mode == model.ModeWork {
		filtered := make([]model.Account, 0, len(accounts))
		for _, a := range accounts {
			if a.Type == model.AccountTypePublic {
				filtered = append(filtered, a)
			}
		}
		accounts = filtered
	} else {
		filtered := make([]model.Account, 0, len(accounts))
		for _, a := range accounts {
			if a.Type == model.AccountTypePersonal {
				filtered = append(filtered, a)
			}
		}
		accounts = filtered
	}
	var diagBalanceCents int64
	dtos := make([]gin.H, 0, len(accounts))
	for _, a := range accounts {
		diagBalanceCents += a.BalanceCents
		dtos = append(dtos, gin.H{
			"id":            a.ID,
			"name":          a.Name,
			"type":          string(a.Type),
			"currency":      a.Currency,
			"balance_cents": a.BalanceCents,
			"balance_yuan":  a.BalanceYuan().Float64(),
			"is_active":     a.IsActive,
		})
	}
	log.Printf("BALANCE_DIAG scope=accounts_list user_id=%s mode=%s account_count=%d total_balance_cents=%d", userID(c), mode, len(accounts), diagBalanceCents)
	ok(c, dtos)
}

func (s *Server) handleCreateAccount(c *gin.Context) {
	var req struct {
		Name     string `json:"name"     binding:"required"`
		Type     string `json:"type"     binding:"required"`
		Currency string `json:"currency"`
		Mode     string `json:"mode"     binding:"required"` // "work" | "life"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	mode, modeOK := parseMode(req.Mode)
	if !modeOK {
		fail(c, 400, 40001, "invalid mode: must be 'work' or 'life'")
		return
	}
	cur := req.Currency
	if cur == "" {
		cur = "CNY"
	}
	acct, err := s.acctSvc.CreateAccount(c.Request.Context(), userID(c), req.Name, model.AccountType(req.Type), cur, mode)
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	created(c, gin.H{
		"id":            acct.ID,
		"name":          acct.Name,
		"type":          string(acct.Type),
		"currency":      acct.Currency,
		"balance_cents": acct.BalanceCents,
		"balance_yuan":  acct.BalanceYuan().Float64(),
		"is_active":     acct.IsActive,
	})
}

func (s *Server) handleUpdateAccount(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	if err := s.acctSvc.RenameAccount(c.Request.Context(), id, userID(c), req.Name); err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	ok(c, gin.H{"id": id, "name": req.Name})
}

func (s *Server) handleDeleteAccount(c *gin.Context) {
	id := c.Param("id")
	if err := s.acctSvc.DeleteAccount(c.Request.Context(), id, userID(c)); err != nil {
		if errors.Is(err, service.ErrResourceConflict) {
			failDomain(c, err)
			return
		}
		fail(c, 400, 40001, err.Error())
		return
	}
	ok(c, gin.H{"id": id, "deleted": true})
}

// ─── Category handlers ───────────────────────────────────────────────────────

func (s *Server) handleListCategories(c *gin.Context) {
	rows, err := s.db.QueryContext(c.Request.Context(),
		`SELECT id, name, type, parent_id, sort_order, is_active FROM categories WHERE user_id=? AND is_active=1 ORDER BY sort_order`,
		userID(c),
	)
	if err != nil {
		failInternal(c, err)
		return
	}
	defer rows.Close()
	type catDTO struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		Type      string  `json:"type"`
		ParentID  *string `json:"parent_id"`
		SortOrder int     `json:"sort_order"`
		IsActive  bool    `json:"is_active"`
	}
	var dtos []catDTO
	for rows.Next() {
		var d catDTO
		var isActive int
		if err := rows.Scan(&d.ID, &d.Name, &d.Type, &d.ParentID, &d.SortOrder, &isActive); err != nil {
			failInternal(c, err)
			return
		}
		d.IsActive = isActive == 1
		dtos = append(dtos, d)
	}
	if dtos == nil {
		dtos = []catDTO{}
	}
	ok(c, dtos)
}

func (s *Server) handleCreateCategory(c *gin.Context) {
	var req struct {
		Name      string  `json:"name"       binding:"required"`
		Type      string  `json:"type"       binding:"required"`
		ParentID  *string `json:"parent_id"`
		SortOrder int     `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	id := uuid.NewString()
	if _, err := s.db.ExecContext(c.Request.Context(),
		`INSERT INTO categories(id, user_id, name, type, parent_id, sort_order, is_active, created_at) VALUES(?,?,?,?,?,?,1,?)`,
		id, userID(c), req.Name, req.Type, req.ParentID, req.SortOrder, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		log.Printf("[ERROR] create category: %v", err)
		if strings.Contains(err.Error(), "UNIQUE") {
			fail(c, 422, 40001, "该分类名称已存在")
		} else {
			fail(c, 422, 40001, "创建分类失败，请稍后重试")
		}
		return
	}
	created(c, gin.H{"id": id, "name": req.Name, "type": req.Type})
}
