package apiv1

import (
	"context"
	"crypto/rand"
	"database/sql"
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
	attempts  int       // wrong code attempts
}

func NewServer(
	addr string,
	db *sql.DB,
	dbPath string,
	txRepo repository.TransactionRepository,
	tagRepo repository.TagRepository,
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
	r.Use(gin.Recovery(), s.securityHeaders(), s.corsMiddleware())

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
	api.GET("/backup/download", s.handleBackupDownload)
	api.GET("/backup/info", s.handleBackupInfo)
	api.POST("/backup/restore", s.handleRestore)

	// ─── Frontend static files ────────────────────────────────────
	staticDir := os.Getenv("FINARCH_STATIC")
	if staticDir == "" {
		staticDir = "./frontend/dist"
	}
	r.Static("/assets", staticDir+"/assets")
	// Serve any other static file that exists in dist root (favicon, etc.)
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		// API routes that truly don't exist → 404 JSON
		if strings.HasPrefix(path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		// Resolve and validate the requested path against the static directory
		staticAbs, err := filepath.Abs(staticDir)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "static dir misconfigured"})
			return
		}
		// Make the request path relative before joining, to avoid absolute path override
		relPath := strings.TrimPrefix(path, "/")
		candidate := filepath.Join(staticAbs, relPath)
		// Ensure the candidate path is still within the static directory
		if !strings.HasPrefix(candidate, staticAbs+string(os.PathSeparator)) && candidate != staticAbs {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		// Try to serve the file directly first (e.g. /favicon.svg)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			c.File(candidate)
			return
		}
		// SPA fallback: let React Router handle the path
		indexPath := filepath.Join(staticAbs, "index.html")
		c.File(indexPath)
	})
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

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": data})
}

func created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, gin.H{"code": 0, "message": "created", "data": data})
}

func fail(c *gin.Context, status, code int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"code": code, "message": msg})
}

// failBind returns a user-friendly validation error instead of raw Gin binding errors.
func failBind(c *gin.Context, code int) {
	fail(c, 422, code, "请求参数不正确，请检查输入")
}

// failInternal logs the real error and returns a generic message to the user.
func failInternal(c *gin.Context, err error) {
	log.Printf("[ERROR] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	fail(c, 500, 50001, "服务器内部错误，请稍后重试")
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
		failBind(c, 40001)
		return
	}
	if err := s.captchaVerifier.Verify(req.CaptchaToken, realIP(c)); err != nil {
		fail(c, 400, 40003, err.Error())
		return
	}
	newUser, err := s.authSvc.Register(c.Request.Context(), service.RegisterRequest{
		Email: req.Email, Username: req.Username, Password: req.Password, Nickname: req.Nickname,
	})
	if err != nil {
		if err.Error() == "username_taken" {
			fail(c, 409, 40902, "该用户名已被使用")
			return
		}
		if err.Error() == "email_taken" {
			fail(c, 409, 40901, "该邮箱已被注册")
			return
		}
		fail(c, 409, 40901, err.Error())
		return
	}
	// Create default personal & public accounts for the new user.
	if err := s.acctSvc.EnsureDefaultAccounts(c.Request.Context(), newUser.ID); err != nil {
		log.Printf("[WARN] failed to create default accounts for user %s: %v", newUser.ID, err)
	}
	// If email verification is required, don't auto-login.
	if s.authSvc.EmailVerificationRequired() {
		c.JSON(http.StatusAccepted, gin.H{"message": "注册成功，验证邮件已发送，请检查邮箱后登录"})
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
		failBind(c, 40001)
		return
	}
	if err := s.captchaVerifier.Verify(req.CaptchaToken, realIP(c)); err != nil {
		fail(c, 400, 40003, err.Error())
		return
	}
	resp, err := s.authSvc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if err.Error() == "email_not_verified" {
			fail(c, 403, 40301, "邮箱尚未验证，请检查您的邮箱并点击验证链接")
			return
		}
		fail(c, 401, 40101, err.Error())
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
		failBind(c, 40001)
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
		failBind(c, 40001)
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
		failBind(c, 40001)
		return
	}
	if err := s.authSvc.ResetPassword(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		fail(c, 400, 40002, err.Error())
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
		failBind(c, 40001)
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
	ok(c, gin.H{"message": "注销确认邮件已发送，请在 1 小时内点击邮件中的链接完成操作"})
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
		fail(c, 400, 40012, err.Error())
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
		NewEmail string `json:"new_email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c, 40020)
		return
	}
	if err := s.authSvc.RequestEmailChange(c.Request.Context(), userID(c), req.NewEmail); err != nil {
		fail(c, 400, 40021, err.Error())
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
		fail(c, 400, 40025, err.Error())
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
		fail(c, 400, 40023, err.Error())
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

// ─── Transactions ────────────────────────────────────────────────────────────

func (s *Server) handleListTransactions(c *gin.Context) {
	txs, err := s.txRepo.ListByUser(c.Request.Context(), userID(c))
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
			Reimbursed: t.Reimbursed, Uploaded: t.Uploaded, Tags: tagNames,
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
		Category  string   `json:"category"  binding:"required"`
		Currency  string   `json:"currency"`
		Note      string   `json:"note"`
		ProjectID *string  `json:"project_id"`
		TagIDs    []string `json:"tag_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c, 40001)
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

	created_, err := s.txSvc.CreateTransaction(c.Request.Context(), service.CreateTransactionRequest{
		UserID:       userID(c),
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
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	for _, tagID := range req.TagIDs {
		if err := s.tagRepo.AddToTransaction(c.Request.Context(), created_.ID, tagID); err != nil {
			fail(c, 422, 40002, "标签不存在")
			return
		}
	}
	created(c, gin.H{
		"id":           created_.ID,
		"amount_yuan":  created_.AmountYuan.Float64(),
		"amount_cents": created_.AmountCents,
		"reimb_status": string(created_.ReimbStatus),
		"account_id":   created_.AccountID,
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
		failBind(c, 40001)
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
		failBind(c, 40001)
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
		failBind(c, 40001)
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
		failBind(c, 40001)
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

// handleBackupDownload creates a consistent snapshot of the database using
// SQLite VACUUM INTO and streams it as a downloadable file.
func (s *Server) handleBackupDownload(c *gin.Context) {
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
	if _, err := s.db.ExecContext(c.Request.Context(), fmt.Sprintf("VACUUM INTO '%s'", tmpPath)); err != nil {
		fail(c, 500, 50001, "备份失败，请稍后重试")
		return
	}

	// Embed schema version in filename for traceability
	var schemaVer int
	_ = s.db.QueryRowContext(c.Request.Context(), `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&schemaVer)
	ts := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("finarch_backup_v%d_%s.db", schemaVer, ts)
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

	// ── Schema version compatibility check ──
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

	// ── Auto safety backup before restore ──
	safetyDir := filepath.Join(filepath.Dir(s.dbPath), "safety_backups")
	if err := os.MkdirAll(safetyDir, 0o755); err == nil {
		safetyPath := filepath.Join(safetyDir, fmt.Sprintf(
			"pre_restore_%s.db", time.Now().Format("20060102_150405"),
		))
		// Best-effort: don't fail the restore if safety backup fails
		_, _ = s.db.ExecContext(c.Request.Context(), fmt.Sprintf("VACUUM INTO '%s'", safetyPath))

		// Keep only the last 5 safety backups
		entries, _ := os.ReadDir(safetyDir)
		var safetyFiles []os.DirEntry
		for _, e := range entries {
			if !e.IsDir() && strings.HasPrefix(e.Name(), "pre_restore_") && strings.HasSuffix(e.Name(), ".db") {
				safetyFiles = append(safetyFiles, e)
			}
		}
		if len(safetyFiles) > 5 {
			// Sort by name (timestamp-based) — oldest first
			for i := 0; i < len(safetyFiles)-5; i++ {
				_ = os.Remove(filepath.Join(safetyDir, safetyFiles[i].Name()))
			}
		}
	}

	// ── Perform restore via SQLite Backup API ──
	restoreSrcDB, err := sql.Open("sqlite3", tmpPath)
	if err != nil {
		fail(c, 500, 50002, "无法打开备份文件")
		return
	}
	defer restoreSrcDB.Close()

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
		log.Printf("[ERROR] restore: %v", restoreErr)
		fail(c, 500, 50003, "数据恢复失败，请确认文件完整后重试")
		return
	}

	// ── Post-restore: re-apply PRAGMAs (Backup API may reset WAL → DELETE) ──
	if err := findb.ReapplyPragmas(ctx, s.db); err != nil {
		log.Printf("[WARN] restore: reapply pragmas: %v", err)
	}

	// ── Post-restore: auto-migrate if backup was an older schema ──
	var migratedTo int
	if uploadedVersion < currentVersion {
		if err := findb.Migrate(ctx, s.db); err != nil {
			log.Printf("[ERROR] restore: post-restore migration failed: %v", err)
			fail(c, 500, 50004, fmt.Sprintf(
				"数据已恢复但自动迁移失败 (v%d → v%d): %s。请重启服务让迁移自动完成。",
				uploadedVersion, currentVersion, err.Error(),
			))
			return
		}
		_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&migratedTo)
		log.Printf("[INFO] restore: auto-migrated from v%d to v%d", uploadedVersion, migratedTo)
	} else {
		migratedTo = uploadedVersion
	}

	ok(c, gin.H{
		"message":          "数据恢复成功，数据库已更新",
		"restored_version": uploadedVersion,
		"migrated_to":      migratedTo,
	})
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
	var ownerEmail, ownerName string
	row := srcDB.QueryRow(`SELECT email, COALESCE(nickname, username, '') FROM users WHERE deleted_at IS NULL ORDER BY CASE WHEN role='admin' THEN 0 ELSE 1 END, CASE WHEN email_verified=1 THEN 0 ELSE 1 END, created_at ASC LIMIT 1`)
	if err := row.Scan(&ownerEmail, &ownerName); err != nil {
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
		log.Printf("[ERROR] restore: %v", restoreErr)
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

	log.Printf("[INFO] disaster-restore completed successfully (v%d) for %s", migratedTo, maskEmail(pr.email))

	ok(c, gin.H{
		"message":          "灾难恢复成功！数据库已恢复",
		"restored_version": uploadedVersion,
		"migrated_to":      migratedTo,
	})
}

// ─── Account handlers ────────────────────────────────────────────────────────

func (s *Server) handleListAccounts(c *gin.Context) {
	accounts, err := s.acctSvc.ListAccounts(c.Request.Context(), userID(c))
	if err != nil {
		failInternal(c, err)
		return
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
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c, 40001)
		return
	}
	cur := req.Currency
	if cur == "" {
		cur = "CNY"
	}
	acct, err := s.acctSvc.CreateAccount(c.Request.Context(), userID(c), req.Name, model.AccountType(req.Type), cur)
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
		"is_active":     acct.IsActive,
	})
}

func (s *Server) handleUpdateAccount(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c, 40001)
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
		failBind(c, 40001)
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
