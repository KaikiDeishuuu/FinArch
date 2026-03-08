package apiv1

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	findb "finarch/internal/infrastructure/db"
	"finarch/internal/infrastructure/email"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	sqlite3 "github.com/mattn/go-sqlite3"
)

// Server is the Gin-based API v1 server.
type Server struct {
	engine           *gin.Engine
	addr             string
	db               *sql.DB
	dbPath           string
	authSvc          *service.AuthService
	txSvc            *service.TransactionService
	reimSvc          *service.ReimbursementService
	matchSvc         *service.MatchingService
	statsSvc         *service.StatsService
	txRepo           repository.TransactionRepository
	tagRepo          repository.TagRepository
	txManager        repository.TransactionManager
	jwtSvc           *auth.JWTService
	authLimiter      *auth.IPRateLimiter
	captchaVerifier  *auth.TurnstileVerifier
	turnstileSiteKey string
	acctSvc          *service.AccountService
	emailSvc         email.Sender
	pendingRestores  sync.Map // restoreID → *pendingRestore
	activeDevices    sync.Map // "userID:deviceID" → *deviceSession
}

// deviceSession tracks a single device's last heartbeat.
type deviceSession struct {
	LastSeen  time.Time
	UserAgent string
}

// pendingRestore holds temporary state for a disaster recovery restore session.
type pendingRestore struct {
	code      string    // 6-digit verification code
	tmpPath   string    // path to the uploaded .db tmp file
	expiresAt time.Time // when this session expires
	email     string    // owner email extracted from backup
	name      string    // owner name extracted from backup
	ownerID   string
	requester string
	attempts  int // wrong code attempts
	verified  bool
	token     string
}

func isCrossAccountRestore(backupOwnerID, backupOwnerEmail, currentUserID, currentUserEmail string) bool {
	if backupOwnerID != "" && currentUserID != "" && backupOwnerID == currentUserID {
		return false
	}
	if backupOwnerEmail != "" && currentUserEmail != "" && strings.EqualFold(strings.TrimSpace(backupOwnerEmail), strings.TrimSpace(currentUserEmail)) {
		return false
	}
	return backupOwnerID != "" && currentUserID != ""
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
	jwtSvc *auth.JWTService,
	authLimiter *auth.IPRateLimiter,
	captchaVerifier *auth.TurnstileVerifier,
	turnstileSiteKey string,
	acctSvc *service.AccountService,
	emailSvc email.Sender,
) *Server {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	s := &Server{
		engine:           gin.New(),
		addr:             addr,
		db:               db,
		dbPath:           dbPath,
		authSvc:          authSvc,
		txSvc:            txSvc,
		reimSvc:          reimSvc,
		matchSvc:         matchSvc,
		statsSvc:         statsSvc,
		txRepo:           txRepo,
		tagRepo:          tagRepo,
		txManager:        txManager,
		jwtSvc:           jwtSvc,
		authLimiter:      authLimiter,
		captchaVerifier:  captchaVerifier,
		turnstileSiteKey: turnstileSiteKey,
		acctSvc:          acctSvc,
		emailSvc:         emailSvc,
	}
	s.registerRoutes()
	return s
}

func (s *Server) Run() error {
	return s.engine.Run(s.addr)
}

func (s *Server) registerRoutes() {
	r := s.engine
	r.Use(gin.Recovery(), s.securityHeaders(), s.corsMiddleware(), s.writeGateMiddleware())

	// ─── Public routes ───────────────────────────────────────────
	pub := r.Group("/api/v1")
	pub.GET("/config", s.handleConfig)
	pub.POST("/auth/register", s.authRateLimitMiddleware(), s.handleRegister)
	pub.POST("/auth/login", s.authRateLimitMiddleware(), s.handleLogin)
	pub.GET("/auth/verify-email", s.handleVerifyEmail)
	pub.POST("/auth/verify-email", s.handleVerifyEmailJSON)
	pub.POST("/auth/resend-verification", s.authRateLimitMiddleware(), s.handleResendVerification)
	pub.POST("/auth/forgot-password", s.authRateLimitMiddleware(), s.handleForgotPassword)
	pub.POST("/auth/reset-password", s.handleResetPassword)
	pub.POST("/auth/confirm-delete-account", s.handleConfirmDeleteAccount)
	pub.POST("/auth/confirm-email-change-old", s.handleConfirmOldEmailChange)
	pub.POST("/auth/confirm-email-change", s.handleConfirmEmailChange)

	// Disaster recovery (public — email-verified restore when JWT auth unavailable)
	pub.POST("/backup/restore-request", s.authRateLimitMiddleware(), s.handleRestoreRequest)
	pub.POST("/backup/restore-confirm", s.handleRestoreConfirm)

	// Shortcut: /verify-email → same handler (for emails already sent with old link)
	r.GET("/verify-email", s.handleVerifyEmail)

	// ─── Protected routes (JWT required) ──────────────────────────
	api := r.Group("/api/v1", s.jwtMiddleware())

	// User
	api.GET("/auth/me", s.handleGetMe)
	api.POST("/auth/refresh", s.handleRefreshToken)
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

	// Backup & Restore
	api.POST("/backup/export-request", s.handleBackupExportRequest)
	api.GET("/backup/download", s.handleBackupDownload)
	api.GET("/backup/info", s.handleBackupInfo)
	api.GET("/backup/litestream-health", s.handleLitestreamHealth)
	api.POST("/backup/restore", s.handleRestore)
	api.POST("/backup/restore/send-verification", s.handleRestoreSendVerification)
	api.POST("/backup/restore/verify", s.handleRestoreVerify)
	api.POST("/backup/restore/execute", s.handleRestoreExecute)

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
		origin := c.GetHeader("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func (s *Server) jwtMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "缺少认证令牌"})
			return
		}
		claims, err := s.jwtSvc.Verify(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "认证令牌无效或已过期"})
			return
		}
		// Verify pwd_version matches DB — invalidates tokens from before a password change.
		var dbPwdVer int
		_ = s.db.QueryRowContext(c.Request.Context(),
			"SELECT COALESCE(pwd_version,0) FROM users WHERE id = ? AND deleted_at IS NULL",
			claims.UserID,
		).Scan(&dbPwdVer)
		if dbPwdVer != claims.PwdVersion {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40102, "message": "密码已更改，请重新登录"})
			return
		}
		c.Set("userID", claims.UserID)
		c.Set("userEmail", claims.Email)
		c.Set("userRole", claims.Role)
		c.Next()
	}
}

func userID(c *gin.Context) string { return c.GetString("userID") }

// realIP extracts the best-effort real client IP, honouring X-Forwarded-For
// and X-Real-IP headers set by a trusted reverse proxy.
func realIP(c *gin.Context) string {
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For may be a comma-separated list; take the first entry.
		if idx := strings.IndexByte(xff, ','); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	return c.ClientIP()
}

// securityHeaders adds standard security headers to every response.
func (s *Server) securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

// authRateLimitMiddleware restricts auth endpoints by IP to prevent brute-force
// and credential-stuffing attacks.
func (s *Server) authRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := realIP(c)
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
	case errors.Is(err, service.ErrNotAuthorized):
		return http.StatusUnauthorized, apiErrorPayload{Code: "not_authorized", Message: "You are not authorized to perform this action."}
	case errors.Is(err, service.ErrUserNotFound):
		return http.StatusNotFound, apiErrorPayload{Code: "user_not_found", Message: "User not found."}
	case errors.Is(err, service.ErrResourceConflict):
		return http.StatusConflict, apiErrorPayload{Code: "resource_conflict", Message: "The request conflicts with current data."}
	case errors.Is(err, service.ErrEmailNotVerified):
		return http.StatusForbidden, apiErrorPayload{Code: "email_not_verified", Message: "Please verify your email before logging in."}
	case errors.Is(err, service.ErrConcurrentModification):
		return http.StatusConflict, apiErrorPayload{Code: "concurrent_modification", Message: "The resource was modified by another request. Please refresh and try again."}
	case errors.Is(err, service.ErrInvalidOrUsedToken):
		return http.StatusBadRequest, apiErrorPayload{Code: "invalid_or_used_token", Message: "The refresh token is invalid, expired, or already consumed."}
	case errors.Is(err, service.ErrSystemUnavailable):
		return http.StatusServiceUnavailable, apiErrorPayload{Code: "system_unavailable", Message: "System temporarily unavailable due to maintenance."}
	default:
		return http.StatusInternalServerError, apiErrorPayload{Code: "internal_error", Message: "Something went wrong. Please try again."}
	}
}

func mapAuthError(c *gin.Context, err error, _ int) {
	failDomain(c, err)
}

// writeGateMiddleware rejects all mutating requests when the system is not in StateNormal.
func (s *Server) writeGateMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !findb.Global().IsWritable() {
			switch c.Request.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
					"success": false,
					"error": apiErrorPayload{
						Code:    "system_unavailable",
						Message: "System temporarily unavailable due to maintenance.",
					},
				})
				return
			}
		}
		c.Next()
	}
}

// ─── Config ──────────────────────────────────────────────────────────────────

// handleConfig returns public runtime configuration consumed by the frontend.
func (s *Server) handleConfig(c *gin.Context) {
	ok(c, gin.H{
		"turnstile_site_key":          s.turnstileSiteKey,
		"email_verification_required": s.authSvc.EmailVerificationRequired(),
	})
}

// ─── Auth ────────────────────────────────────────────────────────────────────

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
	if err := s.captchaVerifier.Verify(req.CaptchaToken, realIP(c)); err != nil {
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
		failInternal(c, err)
		return
	}
	created(c, gin.H{
		"token":      resp.Token,
		"expires_at": resp.ExpiresAt.Format(time.RFC3339),
		"user_id":    resp.UserID,
		"email":      resp.Email,
		"username":   resp.Username,
		"nickname":   resp.Nickname,
		"role":       resp.Role,
	})
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
	if err := s.captchaVerifier.Verify(req.CaptchaToken, realIP(c)); err != nil {
		fail(c, http.StatusBadRequest, "invalid_captcha", "Captcha verification failed.")
		return
	}
	resp, err := s.authSvc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		failDomain(c, err)
		return
	}
	ok(c, gin.H{
		"token":      resp.Token,
		"expires_at": resp.ExpiresAt.Format(time.RFC3339),
		"user_id":    resp.UserID,
		"email":      resp.Email,
		"username":   resp.Username,
		"nickname":   resp.Nickname,
		"role":       resp.Role,
	})
}

func (s *Server) handleVerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.Redirect(http.StatusFound, "/login?error=invalid_token")
		return
	}
	if err := s.authSvc.VerifyEmail(c.Request.Context(), token); err != nil {
		c.Redirect(http.StatusFound, "/login?error=invalid_token")
		return
	}
	c.Redirect(http.StatusFound, "/login?verified=1")
}

// handleVerifyEmailJSON is the JSON-returning counterpart for the frontend
// VerifyEmailPage (PWA-safe: no server-side redirect).
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
		fail(c, 400, 40002, err.Error())
		return
	}
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
	ok(c, gin.H{"message": "账户已注销，感谢您使用 FinArch"})
}

func (s *Server) handleGetMe(c *gin.Context) {
	u, err := s.authSvc.GetUserProfile(c.Request.Context(), userID(c))
	if err != nil {
		fail(c, 404, 40401, "用户不存在")
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

// handleRefreshToken re-issues a fresh 6h token for the current user.
func (s *Server) handleRefreshToken(c *gin.Context) {
	u, err := s.authSvc.GetUserProfile(c.Request.Context(), userID(c))
	if err != nil {
		fail(c, 404, 40401, "用户不存在")
		return
	}
	var pwdVer int
	_ = s.db.QueryRowContext(c.Request.Context(),
		"SELECT COALESCE(pwd_version,0) FROM users WHERE id = ?", u.ID,
	).Scan(&pwdVer)
	token, exp, err := s.jwtSvc.Issue(u.ID, u.Email, u.Role, pwdVer)
	if err != nil {
		failInternal(c, err)
		return
	}
	ok(c, gin.H{
		"token":      token,
		"expires_at": exp.Format(time.RFC3339),
		"user_id":    u.ID,
		"email":      u.Email,
		"username":   u.Username,
		"nickname":   u.Nickname,
		"role":       u.Role,
	})
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
		ID              string  `json:"id"`
		GroupID         string  `json:"group_id"`
		AccountID       string  `json:"account_id"`
		AccountType     string  `json:"account_type"`
		LedgerDir       string  `json:"ledger_dir"`
		TxType          string  `json:"type"`
		AmountCents     int64   `json:"amount_cents"`
		BaseAmountCents int64   `json:"base_amount_cents"`
		ExchangeRate    float64 `json:"exchange_rate"`
		ReimbStatus     string  `json:"reimb_status"`
		TxnDate         string  `json:"txn_date"`
		// Backward-compat fields retained for frontend
		OccurredAt string   `json:"occurred_at"`
		Direction  string   `json:"direction"`
		Source     string   `json:"source"`
		Category   string   `json:"category"`
		AmountYuan float64  `json:"amount_yuan"`
		Currency   string   `json:"currency"`
		Note       string   `json:"note"`
		ProjectID  *string  `json:"project_id"`
		Project    *string  `json:"project"`
		Reimbursed bool     `json:"reimbursed"`
		Uploaded   bool     `json:"uploaded"`
		Mode       string   `json:"mode"`
		Tags       []string `json:"tags"`
	}
	dtos := make([]txDTO, 0, len(txs))
	for _, t := range txs {
		tags, _ := s.tagRepo.ListByTransaction(c.Request.Context(), t.ID)
		tagNames := make([]string, 0, len(tags))
		for _, tg := range tags {
			tagNames = append(tagNames, tg.Name)
		}
		dtos = append(dtos, txDTO{
			ID: t.ID, GroupID: t.GroupID,
			AccountID: t.AccountID, AccountType: string(t.AccountType),
			LedgerDir: string(t.LedgerDir), TxType: string(t.TxType),
			AmountCents: t.AmountCents, BaseAmountCents: t.BaseAmountCents,
			ExchangeRate: t.ExchangeRate, ReimbStatus: string(t.ReimbStatus),
			TxnDate: t.TxnDate,
			// backward-compat
			OccurredAt: t.TxnDate,
			Direction:  string(t.Direction), Source: string(t.Source),
			Category: t.Category, AmountYuan: t.AmountYuan.Float64(),
			Currency: t.Currency, Note: t.Note,
			ProjectID: t.ProjectID, Project: t.Project,
			Reimbursed: t.Reimbursed, Uploaded: t.Uploaded, Mode: string(t.Mode), Tags: tagNames,
		})
	}
	ok(c, dtos)
}

func (s *Server) handleCreateTransaction(c *gin.Context) {
	var req struct {
		OccurredAt string `json:"occurred_at"`
		// V9 preferred fields
		AccountID    string  `json:"account_id"`
		Type         string  `json:"type"` // 'income'|'expense'|'transfer'
		AmountCents  int64   `json:"amount_cents"`
		ExchangeRate float64 `json:"exchange_rate"`
		// Backward-compat fields (still accepted)
		Direction  string  `json:"direction"`
		Source     string  `json:"source"`
		AmountYuan float64 `json:"amount_yuan"`
		// Common
		Mode      string   `json:"mode"`
		Category  string   `json:"category"  binding:"required"`
		Currency  string   `json:"currency"`
		Note      string   `json:"note"`
		ProjectID *string  `json:"project_id"`
		TagIDs    []string `json:"tag_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	if req.AmountYuan <= 0 && req.AmountCents <= 0 {
		fail(c, 422, 40001, "请输入有效的金额")
		return
	}
	txDate := time.Now()
	if req.OccurredAt != "" {
		if parsed, err := time.Parse("2006-01-02", req.OccurredAt); err == nil {
			txDate = parsed
		}
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
	if projID != nil {
		if _, err := s.db.ExecContext(c.Request.Context(),
			`INSERT OR IGNORE INTO projects(id, name, code, created_at) VALUES (?, ?, ?, ?)`,
			*projID, *projID, *projID, time.Now().Unix(),
		); err != nil {
			fail(c, 500, 50001, "创建项目失败，请稍后重试")
			return
		}
	}
	// Ensure default accounts exist (idempotent; covers legacy users without accounts).
	_ = s.acctSvc.EnsureDefaultAccounts(c.Request.Context(), userID(c))

	var createdTx model.Transaction
	var tagFoundErr = errors.New("标签不存在")

	err := s.txManager.WithinTransaction(c.Request.Context(), func(ctx context.Context) error {
		var inErr error
		createdTx, inErr = s.txSvc.CreateTransaction(ctx, service.CreateTransactionRequest{
			UserID:       userID(c),
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
		})
		if inErr != nil {
			return inErr
		}
		for _, tagID := range req.TagIDs {
			if inErr := s.tagRepo.AddToTransaction(ctx, createdTx.ID, tagID); inErr != nil {
				return tagFoundErr
			}
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, tagFoundErr) {
			fail(c, 422, 40002, "标签不存在")
		} else {
			fail(c, 422, 40001, err.Error())
		}
		return
	}

	created(c, gin.H{
		"id":           createdTx.ID,
		"amount_yuan":  createdTx.AmountYuan.Float64(),
		"amount_cents": createdTx.AmountCents,
		"reimb_status": string(createdTx.ReimbStatus),
		"account_id":   createdTx.AccountID,
		"mode":         string(createdTx.Mode),
	})
}

func (s *Server) handleToggleReimbursed(c *gin.Context) {
	id := c.Param("id")
	newState, err := s.txRepo.ToggleReimbursed(c.Request.Context(), id, userID(c))
	if err != nil {
		fail(c, 400, 40001, err.Error())
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
	if err := s.tagRepo.AddToTransaction(c.Request.Context(), txID, req.TagID); err != nil {
		failInternal(c, err)
		return
	}
	ok(c, gin.H{"transaction_id": txID, "tag_id": req.TagID})
}

func (s *Server) handleRemoveTag(c *gin.Context) {
	if err := s.tagRepo.RemoveFromTransaction(c.Request.Context(), c.Param("id"), c.Param("tagID")); err != nil {
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
		txList, ferr := s.txRepo.GetByIDs(c.Request.Context(), allIDs)
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
		Applicant: req.Applicant, TransactionIDs: req.TransactionIDs, RequestNo: req.RequestNo,
	})
	if err != nil {
		fail(c, 400, 40001, err.Error())
		return
	}
	created(c, gin.H{
		"id": reim.ID, "request_no": reim.RequestNo,
		"total_yuan": reim.TotalYuan.Float64(), "status": reim.Status,
	})
}

// ─── Stats ───────────────────────────────────────────────────────────────────

func (s *Server) handleStatsSummary(c *gin.Context) {
	b, err := s.statsSvc.Summary(c.Request.Context(), userID(c))
	if err != nil {
		failInternal(c, err)
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
		failInternal(c, err)
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
		failInternal(c, err)
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
		failInternal(c, err)
		return
	}
	if stats == nil {
		stats = []service.ProjectStat{}
	}
	ok(c, stats)
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
	exportToken := c.Query("export_token")
	if exportToken == "" {
		fail(c, 403, 40052, "not_authorized")
		return
	}
	if err := s.authSvc.ConsumeBackupExportToken(c.Request.Context(), userID(c), exportToken); err != nil {
		mapAuthError(c, err, 40053)
		return
	}

	// Create temp file for the snapshot
	tmp, err := os.CreateTemp("", "finarch-backup-*.db")
	if err != nil {
		fail(c, 500, 50001, "无法创建临时文件")
		return
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	// Use VACUUM INTO to produce a defragmented, consistent snapshot
	// Add a timeout to prevent the connection from hanging indefinitely
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", tmpPath)); err != nil {
		log.Printf("[ERROR] handleBackupDownload: VACUUM INTO failed: %v", err)
		fail(c, 500, 50001, "备份生成失败，请稍后重试")
		return
	}

	// Embed schema version in filename for traceability
	var schemaVer int
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&schemaVer)
	ts := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("finarch_backup_v%d_%s.db", schemaVer, ts)
	// Add proper cache-control headers to prevent the browser from caching the backup download
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, filename))
	c.Header("Content-Type", "application/octet-stream")
	c.File(tmpPath)
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

const maxRestoreSize = 100 << 20 // 100 MB

// handleRestore accepts a SQLite database file upload and restores it into the
// live database using the SQLite Online Backup API — no restart needed.
// Safety: automatically creates a pre-restore snapshot so data is never lost.
func (s *Server) handleRestore(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		fail(c, 400, 40001, "请上传数据库文件（multipart field: file）")
		return
	}
	if filepath.Ext(fh.Filename) != ".db" {
		fail(c, 400, 40002, "仅接受 .db 文件")
		return
	}
	if fh.Size > maxRestoreSize {
		fail(c, 400, 40004, fmt.Sprintf("文件过大（%d MB），上限 100 MB", fh.Size>>20))
		return
	}

	// Save upload to temp file
	tmp, err := os.CreateTemp("", "finarch-restore-*.db")
	if err != nil {
		fail(c, 500, 50001, "无法创建临时文件")
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	src, err := fh.Open()
	if err != nil {
		tmp.Close()
		fail(c, 500, 50001, "无法读取上传文件")
		return
	}
	defer src.Close()

	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		fail(c, 500, 50001, "写入临时文件失败")
		return
	}
	tmp.Close()

	// Validate SQLite magic bytes
	magic := make([]byte, 16)
	f, err := os.Open(tmpPath)
	if err != nil {
		fail(c, 500, 50001, "无法验证文件")
		return
	}
	f.Read(magic)
	f.Close()
	if string(magic[:15]) != "SQLite format 3" {
		fail(c, 400, 40003, "文件不是有效的 SQLite 数据库")
		return
	}

	// ── Schema + owner inspection for cross-account restore ──
	srcDB, err := sql.Open("sqlite3", tmpPath+"?mode=ro")
	if err != nil {
		fail(c, 500, 50002, "无法打开备份文件")
		return
	}

	var uploadedVersion int
	row := srcDB.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`)
	if err := row.Scan(&uploadedVersion); err != nil {
		srcDB.Close()
		fail(c, 400, 40005, "备份文件缺少 schema_migrations 表，不是有效的 FinArch 备份")
		return
	}

	// Extract backup owner (same heuristic as disaster-recovery flow)
	var backupOwnerEmail, backupOwnerName, backupOwnerID string
	ownerRow := srcDB.QueryRow(`SELECT id, email, COALESCE(nickname, username, '') FROM users WHERE deleted_at IS NULL ORDER BY CASE WHEN role='admin' THEN 0 ELSE 1 END, CASE WHEN email_verified=1 THEN 0 ELSE 1 END, created_at ASC LIMIT 1`)
	switch err := ownerRow.Scan(&backupOwnerID, &backupOwnerEmail, &backupOwnerName); err {
	case nil:
		// ok
	case sql.ErrNoRows:
		srcDB.Close()
		fail(c, 400, 40007, "备份文件中未找到用户数据")
		return
	default:
		srcDB.Close()
		fail(c, 400, 40007, "读取备份所有者信息失败")
		return
	}

	currentUserID := userID(c)
	currentUserEmail := c.GetString("userEmail")
	crossAccount := isCrossAccountRestore(backupOwnerID, backupOwnerEmail, currentUserID, currentUserEmail)

	log.Printf("[RESTORE] backup_owner=%s current_user=%s cross_account=%t", backupOwnerEmail, currentUserEmail, crossAccount)

	// Cross-account restore now requires out-of-band email verification.
	if crossAccount {
		srcDB.Close()
		fail(c, http.StatusConflict, "RESTORE_VERIFICATION_REQUIRED", "跨账号恢复需要邮箱验证码")
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

	migratedTo, err := s.performRestoreWithMerge(c.Request.Context(), tmpPath, uploadedVersion, currentVersion, "jwt", "")
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
	fh, err := c.FormFile("file")
	if err != nil {
		fail(c, 400, 40001, "请上传数据库文件（multipart field: file）")
		return
	}
	if filepath.Ext(fh.Filename) != ".db" {
		fail(c, 400, 40002, "仅接受 .db 文件")
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

	tmp, err := os.CreateTemp("", "finarch-restore-verify-*.db")
	if err != nil {
		fail(c, 500, 50001, "无法创建临时文件")
		return
	}
	tmpPath := tmp.Name()
	src, err := fh.Open()
	if err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		fail(c, 500, 50001, "无法读取上传文件")
		return
	}
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		src.Close()
		os.Remove(tmpPath)
		fail(c, 500, 50001, "写入临时文件失败")
		return
	}
	tmp.Close()
	src.Close()

	srcDB, err := sql.Open("sqlite3", tmpPath+"?mode=ro")
	if err != nil {
		os.Remove(tmpPath)
		fail(c, 500, 50002, "无法打开备份文件")
		return
	}
	defer srcDB.Close()

	var uploadedVersion int
	if err := srcDB.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&uploadedVersion); err != nil {
		os.Remove(tmpPath)
		fail(c, 400, 40005, "备份文件缺少 schema_migrations 表，不是有效的 FinArch 备份")
		return
	}
	var ownerID, ownerEmail, ownerName string
	row := srcDB.QueryRow(`SELECT id, email, COALESCE(nickname, username, '') FROM users WHERE deleted_at IS NULL ORDER BY CASE WHEN role='admin' THEN 0 ELSE 1 END, CASE WHEN email_verified=1 THEN 0 ELSE 1 END, created_at ASC LIMIT 1`)
	if err := row.Scan(&ownerID, &ownerEmail, &ownerName); err != nil {
		os.Remove(tmpPath)
		fail(c, 400, 40007, "备份文件中未找到用户数据")
		return
	}
	if !strings.EqualFold(ownerEmail, originalEmail) {
		os.Remove(tmpPath)
		fail(c, http.StatusForbidden, "RESTORE_EMAIL_MISMATCH", "原账号邮箱与备份不匹配")
		return
	}

	currentUserID := userID(c)
	currentUserEmail := c.GetString("userEmail")
	if !isCrossAccountRestore(ownerID, ownerEmail, currentUserID, currentUserEmail) {
		os.Remove(tmpPath)
		fail(c, http.StatusBadRequest, "RESTORE_VERIFICATION_NOT_REQUIRED", "当前备份属于本账号，无需邮箱验证")
		return
	}

	var currentVersion int
	_ = s.db.QueryRowContext(c.Request.Context(), `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&currentVersion)
	if uploadedVersion > currentVersion {
		os.Remove(tmpPath)
		fail(c, 400, 40006, fmt.Sprintf("备份文件版本 (v%d) 高于当前系统版本 (v%d)，请先升级系统再恢复", uploadedVersion, currentVersion))
		return
	}

	code := generateCode()
	restoreID := uuid.NewString()
	s.pendingRestores.Store(restoreID, &pendingRestore{code: code, tmpPath: tmpPath, expiresAt: time.Now().Add(10 * time.Minute), email: ownerEmail, name: ownerName, ownerID: ownerID, requester: currentUserID})
	if err := s.emailSvc.SendRestoreCode(ownerEmail, ownerName, code); err != nil {
		os.Remove(tmpPath)
		s.pendingRestores.Delete(restoreID)
		fail(c, 500, 50005, "验证码发送失败，请检查邮件服务配置")
		return
	}

	ok(c, gin.H{"restore_id": restoreID, "masked_email": maskEmail(ownerEmail), "expires_in": 600, "message": "验证码已发送"})
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
	if time.Now().After(pr.expiresAt) {
		os.Remove(pr.tmpPath)
		s.pendingRestores.Delete(req.RestoreID)
		fail(c, 400, 40009, "验证码已过期，请重新上传备份文件")
		return
	}
	if pr.attempts >= 5 {
		os.Remove(pr.tmpPath)
		s.pendingRestores.Delete(req.RestoreID)
		fail(c, 400, 40010, "验证码错误次数过多，请重新上传备份文件")
		return
	}
	if req.Code != pr.code {
		pr.attempts++
		fail(c, 400, 40011, fmt.Sprintf("验证码错误，剩余 %d 次尝试", 5-pr.attempts))
		return
	}
	pr.verified = true
	pr.token = uuid.NewString()
	ok(c, gin.H{"restore_token": pr.token, "message": "验证成功"})
}

func (s *Server) handleRestoreExecute(c *gin.Context) {
	var req struct {
		RestoreToken string `json:"restore_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, "INVALID_INPUT", "请提供 restore_token")
		return
	}
	var restoreID string
	var pr *pendingRestore
	s.pendingRestores.Range(func(key, value any) bool {
		candidate := value.(*pendingRestore)
		if candidate.token == req.RestoreToken && candidate.verified {
			restoreID = key.(string)
			pr = candidate
			return false
		}
		return true
	})
	if pr == nil {
		fail(c, http.StatusForbidden, "RESTORE_TOKEN_INVALID", "恢复授权已失效，请重新验证")
		return
	}
	if time.Now().After(pr.expiresAt) {
		os.Remove(pr.tmpPath)
		s.pendingRestores.Delete(restoreID)
		fail(c, 400, 40009, "恢复授权已过期，请重新上传备份文件")
		return
	}

	if pr.requester != "" && pr.requester != userID(c) {
		fail(c, http.StatusForbidden, "RESTORE_TOKEN_INVALID", "恢复授权与当前登录账号不匹配")
		return
	}

	tmpPath := pr.tmpPath
	s.pendingRestores.Delete(restoreID)
	defer os.Remove(tmpPath)

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

	migratedTo, err := s.performRestoreWithMerge(c.Request.Context(), tmpPath, uploadedVersion, currentVersion, "verified", userID(c))
	if err != nil {
		fail(c, 500, "RESTORE_EXECUTE_FAILED", err.Error())
		return
	}
	ok(c, gin.H{"code": "SUCCESS", "message": "Restore completed", "restored_version": uploadedVersion, "migrated_to": migratedTo})
}

func (s *Server) performRestoreWithMerge(ctx context.Context, tmpPath string, uploadedVersion, currentVersion int, source, targetUserID string) (int, error) {
	safetyDir := filepath.Join(filepath.Dir(s.dbPath), "safety_backups")
	if err := os.MkdirAll(safetyDir, 0o755); err == nil {
		safetyPath := filepath.Join(safetyDir, fmt.Sprintf("pre_restore_%s.db", time.Now().Format("20060102_150405")))
		_, _ = s.db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", safetyPath))
	}

	if source == "verified" && strings.TrimSpace(targetUserID) != "" {
		return s.performCrossAccountMergeRestore(ctx, tmpPath, uploadedVersion, currentVersion, targetUserID)
	}

	return s.performReplaceRestore(ctx, tmpPath, uploadedVersion, currentVersion, source)
}

func (s *Server) performReplaceRestore(ctx context.Context, tmpPath string, uploadedVersion, currentVersion int, source string) (int, error) {
	restoreSrcDB, err := sql.Open("sqlite3", tmpPath)
	if err != nil {
		return 0, fmt.Errorf("无法打开备份文件")
	}
	defer restoreSrcDB.Close()

	findb.Global().SetState(findb.StateRestore)
	defer findb.Global().SetState(findb.StateNormal)

	destConn, err := s.db.Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf("获取数据库连接失败")
	}
	defer destConn.Close()
	srcConn, err := restoreSrcDB.Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf("获取备份连接失败")
	}
	defer srcConn.Close()

	var restoreErr error
	srcConn.Raw(func(srcRaw interface{}) error {
		return destConn.Raw(func(destRaw interface{}) error {
			srcSQLite, ok1 := srcRaw.(*sqlite3.SQLiteConn)
			destSQLite, ok2 := destRaw.(*sqlite3.SQLiteConn)
			if !ok1 || !ok2 {
				restoreErr = fmt.Errorf("无法获取底层 SQLite 连接")
				return restoreErr
			}
			bk, err := destSQLite.Backup("main", srcSQLite, "main")
			if err != nil {
				restoreErr = err
				return err
			}
			defer bk.Finish()
			if _, err := bk.Step(-1); err != nil {
				restoreErr = err
				return err
			}
			return nil
		})
	})
	if restoreErr != nil {
		return 0, fmt.Errorf("数据恢复失败，请确认文件完整后重试")
	}

	if err := findb.ReapplyPragmas(ctx, s.db); err != nil {
		log.Printf("[WARN] restore(%s): reapply pragmas: %v", source, err)
	}

	migratedTo := uploadedVersion
	if uploadedVersion < currentVersion {
		if err := findb.Migrate(ctx, s.db); err != nil {
			return 0, fmt.Errorf("数据已恢复但自动迁移失败: %s", err.Error())
		}
		_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&migratedTo)
	}

	var users int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users WHERE deleted_at IS NULL`).Scan(&users); err != nil || users == 0 {
		return 0, fmt.Errorf("恢复校验失败：用户数据不存在")
	}
	return migratedTo, nil
}

func deterministicRestoreID(entity, sourceUserID, targetUserID, oldID string) string {
	seed := strings.Join([]string{"finarch-restore", entity, sourceUserID, targetUserID, oldID}, ":")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed)).String()
}

func (s *Server) performCrossAccountMergeRestore(ctx context.Context, tmpPath string, uploadedVersion, currentVersion int, targetUserID string) (int, error) {
	if uploadedVersion < currentVersion {
		backupDB, err := sql.Open("sqlite3", tmpPath)
		if err != nil {
			return 0, fmt.Errorf("无法打开备份文件")
		}
		if err := findb.Migrate(ctx, backupDB); err != nil {
			backupDB.Close()
			return 0, fmt.Errorf("跨账号恢复失败：备份数据迁移失败: %s", err.Error())
		}
		backupDB.Close()
	}

	srcDB, err := sql.Open("sqlite3", tmpPath+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("无法打开备份文件")
	}
	defer srcDB.Close()

	var sourceUserID string
	ownerRow := srcDB.QueryRow(`SELECT id FROM users WHERE deleted_at IS NULL ORDER BY CASE WHEN role='admin' THEN 0 ELSE 1 END, CASE WHEN email_verified=1 THEN 0 ELSE 1 END, created_at ASC LIMIT 1`)
	if err := ownerRow.Scan(&sourceUserID); err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：无法识别备份所有者")
	}

	findb.Global().SetState(findb.StateRestore)
	defer findb.Global().SetState(findb.StateNormal)

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：无法开启事务")
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	accountMap := map[string]string{}
	categoryMap := map[string]string{}
	projectMap := map[string]string{}
	groupMap := map[string]string{}

	// accounts: map by (type,name,currency), else deterministic remap id
	rows, err := tx.QueryContext(ctx, `SELECT id, lower(name), type, currency FROM accounts WHERE user_id=?`, targetUserID)
	if err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：读取目标账户失败")
	}
	existingAccounts := map[string]string{}
	for rows.Next() {
		var id, lname, typ, currency string
		if err := rows.Scan(&id, &lname, &typ, &currency); err != nil {
			rows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：读取目标账户失败")
		}
		existingAccounts[typ+"|"+lname+"|"+currency] = id
	}
	rows.Close()

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
		key := typ + "|" + strings.ToLower(name) + "|" + currency
		if existingID, ok := existingAccounts[key]; ok {
			accountMap[oldID] = existingID
			continue
		}
		newID := deterministicRestoreID("account", sourceUserID, targetUserID, oldID)
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO accounts (id, user_id, name, type, currency, balance_cents, version, is_active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 0, 1, ?, ?, ?)
		`, newID, targetUserID, name, typ, currency, active, now, now); err != nil {
			accRows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：写入账户失败")
		}
		accountMap[oldID] = newID
		existingAccounts[key] = newID
	}
	accRows.Close()

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
	catRows.Close()

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
			if _, err := tx.ExecContext(ctx, `
				INSERT OR IGNORE INTO categories (id, user_id, name, type, parent_id, sort_order, is_active, created_at, version)
				VALUES (?, ?, ?, ?, NULL, ?, ?, ?, 1)
			`, newID, targetUserID, name, typ, sort, active, now); err != nil {
				srcCatRows.Close()
				return 0, fmt.Errorf("跨账号恢复失败：写入分类失败")
			}
			categoryMap[oldID] = newID
			existingCategories[key] = newID
		}
		if parent.Valid {
			parentFixes = append(parentFixes, pendingParent{child: oldID, parent: parent.String})
		}
	}
	srcCatRows.Close()
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
	projRows.Close()

	srcProjRows, err := srcDB.QueryContext(ctx, `SELECT id, name, code, created_at FROM projects`)
	if err == nil {
		for srcProjRows.Next() {
			var oldID, name, code string
			var createdAt any
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
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO projects (id, name, code, created_at, version) VALUES (?, ?, ?, ?, 1)`, newID, name, newCode, now); err != nil {
				srcProjRows.Close()
				return 0, fmt.Errorf("跨账号恢复失败：写入项目失败")
			}
			projectMap[oldID] = newID
			projectByCode[newCode] = newID
			projectByName[strings.ToLower(name)] = newID
		}
		srcProjRows.Close()
	}

	// pre-load transaction fingerprints for deterministic dedup
	fingerprintSet := map[string]struct{}{}
	existingTxRows, err := tx.QueryContext(ctx, `
		SELECT account_id, COALESCE(category_id,''), COALESCE(project_id,''), direction, type, amount_cents, currency, txn_date, COALESCE(note,'')
		FROM transactions WHERE user_id = ?
	`, targetUserID)
	if err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：读取现有交易失败")
	}
	for existingTxRows.Next() {
		var acc, cat, proj, dir, typ, currency, txnDate, note string
		var amount int64
		if err := existingTxRows.Scan(&acc, &cat, &proj, &dir, &typ, &amount, &currency, &txnDate, &note); err != nil {
			existingTxRows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：读取现有交易失败")
		}
		fp := strings.Join([]string{acc, cat, proj, dir, typ, fmt.Sprint(amount), currency, txnDate, note}, "|")
		fingerprintSet[fp] = struct{}{}
	}
	existingTxRows.Close()

	srcTxRows, err := srcDB.QueryContext(ctx, `
		SELECT id, group_id, direction, account_id, amount_cents, currency, exchange_rate, base_amount_cents,
		       type, category_id, category, reimb_status, reimb_to_account, project_id, project,
		       note, uploaded, txn_date, created_at, updated_at
		FROM transactions
		WHERE user_id = ?
		ORDER BY created_at ASC, id ASC
	`, sourceUserID)
	if err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：读取备份交易失败")
	}
	for srcTxRows.Next() {
		var oldID, oldGroupID, dir, oldAccountID, currency, typ string
		var amount, baseAmount int64
		var exchangeRate float64
		var oldCategoryID, category, reimbStatus sql.NullString
		var oldReimbToAccount, oldProjectID, project, note sql.NullString
		var uploaded int
		var txnDate, createdAt, updatedAt string
		if err := srcTxRows.Scan(&oldID, &oldGroupID, &dir, &oldAccountID, &amount, &currency, &exchangeRate, &baseAmount,
			&typ, &oldCategoryID, &category, &reimbStatus, &oldReimbToAccount, &oldProjectID, &project,
			&note, &uploaded, &txnDate, &createdAt, &updatedAt); err != nil {
			srcTxRows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：读取备份交易失败")
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
		fp := strings.Join([]string{mappedAccountID, mappedCategoryID.String, mappedProjectID.String, dir, typ, fmt.Sprint(amount), currency, txnDate, noteText}, "|")
		if _, exists := fingerprintSet[fp]; exists {
			continue
		}

		newID := deterministicRestoreID("transaction", sourceUserID, targetUserID, oldID)
		mappedGroupID, ok := groupMap[oldGroupID]
		if !ok {
			mappedGroupID = deterministicRestoreID("group", sourceUserID, targetUserID, oldGroupID)
			groupMap[oldGroupID] = mappedGroupID
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO transactions (
				id, user_id, group_id, direction, account_id,
				amount_cents, currency, exchange_rate, base_amount_cents,
				type, category_id, category,
				reimb_status, reimb_to_account, reimbursement_id,
				project_id, project, mode,
				note, uploaded, idempotency_key, txn_date, created_at, updated_at, version
			) VALUES (
				?, ?, ?, ?, ?,
				?, ?, ?, ?,
				?, ?, ?,
				?, ?, NULL,
				?, ?, 'work',
				?, ?, NULL, ?, ?, ?, 1
			)
		`, newID, targetUserID, mappedGroupID, dir, mappedAccountID,
			amount, currency, exchangeRate, baseAmount,
			typ, mappedCategoryID, categoryText,
			reimbText, mappedReimbTo,
			mappedProjectID, projectText,
			noteText, uploaded, txnDate, createdAt, updatedAt); err != nil {
			srcTxRows.Close()
			return 0, fmt.Errorf("跨账号恢复失败：写入交易失败")
		}
		fingerprintSet[fp] = struct{}{}
	}
	srcTxRows.Close()

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
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("跨账号恢复失败：提交失败")
	}

	if err := findb.ReapplyPragmas(ctx, s.db); err != nil {
		log.Printf("[WARN] cross-restore: reapply pragmas: %v", err)
	}
	if err := s.acctSvc.EnsureDefaultAccounts(ctx, targetUserID); err != nil {
		log.Printf("[WARN] cross-restore: ensure default accounts failed: %v", err)
	}
	return currentVersion, nil
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

// ─── Disaster Recovery (public, email-verified) ──────────────────────────────

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

// handleRestoreRequest accepts a backup .db file, extracts the owner email,
// sends a 6-digit verification code, and returns a restore_id for step 2.
func (s *Server) handleRestoreRequest(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		fail(c, 400, 40001, "请上传数据库文件（multipart field: file）")
		return
	}
	if filepath.Ext(fh.Filename) != ".db" {
		fail(c, 400, 40002, "仅接受 .db 文件")
		return
	}
	if fh.Size > maxRestoreSize {
		fail(c, 400, 40004, fmt.Sprintf("文件过大（%d MB），上限 100 MB", fh.Size>>20))
		return
	}

	// Save upload to temp file
	tmp, err := os.CreateTemp("", "finarch-dr-*.db")
	if err != nil {
		fail(c, 500, 50001, "无法创建临时文件")
		return
	}
	tmpPath := tmp.Name()

	src, err := fh.Open()
	if err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		fail(c, 500, 50001, "无法读取上传文件")
		return
	}
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		src.Close()
		os.Remove(tmpPath)
		fail(c, 500, 50001, "写入临时文件失败")
		return
	}
	tmp.Close()
	src.Close()

	// Validate SQLite magic bytes
	magic := make([]byte, 16)
	f, err := os.Open(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		fail(c, 500, 50001, "无法验证文件")
		return
	}
	f.Read(magic)
	f.Close()
	if string(magic[:15]) != "SQLite format 3" {
		os.Remove(tmpPath)
		fail(c, 400, 40003, "文件不是有效的 SQLite 数据库")
		return
	}

	// Open backup and extract owner email
	srcDB, err := sql.Open("sqlite3", tmpPath+"?mode=ro")
	if err != nil {
		os.Remove(tmpPath)
		fail(c, 500, 50002, "无法打开备份文件")
		return
	}

	// Check schema_migrations exists
	var schemaVer int
	if err := srcDB.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&schemaVer); err != nil {
		srcDB.Close()
		os.Remove(tmpPath)
		fail(c, 400, 40005, "备份文件缺少 schema_migrations 表，不是有效的 FinArch 备份")
		return
	}

	// Extract owner: first admin user, or first verified user, or first user
	var ownerID, ownerEmail, ownerName string
	row := srcDB.QueryRow(`SELECT id, email, COALESCE(nickname, username, '') FROM users WHERE deleted_at IS NULL ORDER BY CASE WHEN role='admin' THEN 0 ELSE 1 END, CASE WHEN email_verified=1 THEN 0 ELSE 1 END, created_at ASC LIMIT 1`)
	if err := row.Scan(&ownerID, &ownerEmail, &ownerName); err != nil {
		srcDB.Close()
		os.Remove(tmpPath)
		fail(c, 400, 40007, "备份文件中未找到用户数据")
		return
	}
	srcDB.Close()

	if ownerEmail == "" {
		os.Remove(tmpPath)
		fail(c, 400, 40007, "备份文件中的用户没有邮箱地址")
		return
	}

	// Generate verification code and store pending restore
	code := generateCode()
	restoreID := uuid.NewString()

	s.pendingRestores.Store(restoreID, &pendingRestore{
		code:      code,
		tmpPath:   tmpPath,
		expiresAt: time.Now().Add(10 * time.Minute),
		email:     ownerEmail,
		name:      ownerName,
	})

	// Cleanup expired pending restores
	s.pendingRestores.Range(func(key, value any) bool {
		pr := value.(*pendingRestore)
		if time.Now().After(pr.expiresAt) {
			os.Remove(pr.tmpPath)
			s.pendingRestores.Delete(key)
		}
		return true
	})

	// Send verification code via email
	if err := s.emailSvc.SendRestoreCode(ownerEmail, ownerName, code); err != nil {
		os.Remove(tmpPath)
		s.pendingRestores.Delete(restoreID)
		log.Printf("[ERROR] disaster-restore: send code to %s: %v", ownerEmail, err)
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

	// Check expiry
	if time.Now().After(pr.expiresAt) {
		os.Remove(pr.tmpPath)
		s.pendingRestores.Delete(req.RestoreID)
		fail(c, 400, 40009, "验证码已过期，请重新上传备份文件")
		return
	}

	// Check attempts (max 5)
	if pr.attempts >= 5 {
		os.Remove(pr.tmpPath)
		s.pendingRestores.Delete(req.RestoreID)
		fail(c, 400, 40010, "验证码错误次数过多，请重新上传备份文件")
		return
	}

	// Verify code
	if req.Code != pr.code {
		pr.attempts++
		fail(c, 400, 40011, fmt.Sprintf("验证码错误，剩余 %d 次尝试", 5-pr.attempts))
		return
	}

	// Code verified — perform restore using same logic as handleRestore
	tmpPath := pr.tmpPath
	s.pendingRestores.Delete(req.RestoreID)

	// Schema version compatibility check
	srcDB, err := sql.Open("sqlite3", tmpPath+"?mode=ro")
	if err != nil {
		os.Remove(tmpPath)
		fail(c, 500, 50002, "无法打开备份文件")
		return
	}
	var uploadedVersion int
	_ = srcDB.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&uploadedVersion)
	srcDB.Close()

	var currentVersion int
	_ = s.db.QueryRowContext(c.Request.Context(),
		`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&currentVersion)

	if uploadedVersion > currentVersion {
		os.Remove(tmpPath)
		fail(c, 400, 40006, fmt.Sprintf(
			"备份文件版本 (v%d) 高于当前系统版本 (v%d)，请先升级系统再恢复",
			uploadedVersion, currentVersion,
		))
		return
	}

	// Auto safety backup
	safetyDir := filepath.Join(filepath.Dir(s.dbPath), "safety_backups")
	if err := os.MkdirAll(safetyDir, 0o755); err == nil {
		safetyPath := filepath.Join(safetyDir, fmt.Sprintf(
			"pre_drestore_%s.db", time.Now().Format("20060102_150405"),
		))
		_, _ = s.db.ExecContext(c.Request.Context(), fmt.Sprintf("VACUUM INTO '%s'", safetyPath))

		entries, _ := os.ReadDir(safetyDir)
		var safetyFiles []os.DirEntry
		for _, e := range entries {
			if !e.IsDir() && strings.HasPrefix(e.Name(), "pre_") && strings.HasSuffix(e.Name(), ".db") {
				safetyFiles = append(safetyFiles, e)
			}
		}
		if len(safetyFiles) > 5 {
			for i := 0; i < len(safetyFiles)-5; i++ {
				_ = os.Remove(filepath.Join(safetyDir, safetyFiles[i].Name()))
			}
		}
	}

	log.Printf(`{"event":"restore_start","source":"disaster","restore_id":"%s"}`, req.RestoreID)
	// Perform restore via SQLite Backup API
	restoreSrcDB, err := sql.Open("sqlite3", tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		fail(c, 500, 50002, "无法打开备份文件")
		return
	}
	defer restoreSrcDB.Close()
	defer os.Remove(tmpPath)

	ctx := context.Background()

	destConn, err := s.db.Conn(ctx)
	if err != nil {
		fail(c, 500, 50002, "获取数据库连接失败")
		return
	}
	defer destConn.Close()

	srcConn, err := restoreSrcDB.Conn(ctx)
	if err != nil {
		fail(c, 500, 50002, "获取备份连接失败")
		return
	}
	defer srcConn.Close()

	var restoreErr error
	srcConn.Raw(func(srcRaw interface{}) error {
		return destConn.Raw(func(destRaw interface{}) error {
			srcSQLite, ok1 := srcRaw.(*sqlite3.SQLiteConn)
			destSQLite, ok2 := destRaw.(*sqlite3.SQLiteConn)
			if !ok1 || !ok2 {
				restoreErr = fmt.Errorf("无法获取底层 SQLite 连接")
				return restoreErr
			}
			bk, err := destSQLite.Backup("main", srcSQLite, "main")
			if err != nil {
				restoreErr = err
				return err
			}
			defer bk.Finish()
			if _, err := bk.Step(-1); err != nil {
				restoreErr = err
				return err
			}
			return nil
		})
	})

	if restoreErr != nil {
		log.Printf(`{"event":"restore_failed","source":"disaster","error":%q}`, restoreErr.Error())
		fail(c, 500, 50003, "数据恢复失败，请确认文件完整后重试")
		return
	}

	// Post-restore: re-apply PRAGMAs
	if err := findb.ReapplyPragmas(ctx, s.db); err != nil {
		log.Printf("[WARN] disaster-restore: reapply pragmas: %v", err)
	}

	// Post-restore: auto-migrate
	var migratedTo int
	if uploadedVersion < currentVersion {
		if err := findb.Migrate(ctx, s.db); err != nil {
			log.Printf("[ERROR] disaster-restore: migration failed: %v", err)
			fail(c, 500, 50004, fmt.Sprintf(
				"数据已恢复但自动迁移失败 (v%d → v%d): %s。请重启服务。",
				uploadedVersion, currentVersion, err.Error(),
			))
			return
		}
		_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&migratedTo)
		log.Printf("[INFO] disaster-restore: migrated from v%d to v%d", uploadedVersion, migratedTo)
	} else {
		migratedTo = uploadedVersion
	}

	log.Printf(`{"event":"restore_success","source":"disaster","version":%d,"email":"%s"}`, migratedTo, maskEmail(pr.email))

	ok(c, gin.H{
		"message":          "灾难恢复成功！数据库已恢复",
		"restored_version": uploadedVersion,
		"migrated_to":      migratedTo,
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
	dtos := make([]gin.H, 0, len(accounts))
	for _, a := range accounts {
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
