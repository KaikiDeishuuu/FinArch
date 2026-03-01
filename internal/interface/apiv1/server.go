package apiv1

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"

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
	}
	s.registerRoutes()
	return s
}

func (s *Server) Run() error {
	return s.engine.Run(s.addr)
}

func (s *Server) registerRoutes() {
	r := s.engine
	r.Use(gin.Recovery(), s.corsMiddleware())

	// ─── Public routes ───────────────────────────────────────────
	pub := r.Group("/api/v1")
	pub.GET("/config", s.handleConfig)
	pub.POST("/auth/register", s.authRateLimitMiddleware(), s.handleRegister)
	pub.POST("/auth/login", s.authRateLimitMiddleware(), s.handleLogin)
	pub.GET("/auth/verify-email", s.handleVerifyEmail)
	pub.POST("/auth/resend-verification", s.authRateLimitMiddleware(), s.handleResendVerification)
	pub.POST("/auth/forgot-password", s.authRateLimitMiddleware(), s.handleForgotPassword)
	pub.POST("/auth/reset-password", s.handleResetPassword)
	pub.POST("/auth/confirm-delete-account", s.handleConfirmDeleteAccount)
	pub.POST("/auth/confirm-email-change-old", s.handleConfirmOldEmailChange)
	pub.POST("/auth/confirm-email-change", s.handleConfirmEmailChange)

	// ─── Protected routes (JWT required) ──────────────────────────
	api := r.Group("/api/v1", s.jwtMiddleware())

	// User
	api.GET("/auth/me", s.handleGetMe)
	api.POST("/auth/refresh", s.handleRefreshToken)
	api.POST("/auth/change-password", s.handleChangePassword)
	api.POST("/auth/request-delete-account", s.handleRequestDeleteAccount)
	api.POST("/auth/request-email-change", s.handleRequestEmailChange)

	// Transactions
	api.GET("/transactions", s.handleListTransactions)
	api.POST("/transactions", s.handleCreateTransaction)
	api.PATCH("/transactions/:id/reimburse", s.handleToggleReimbursed)
	api.PATCH("/transactions/:id/upload", s.handleToggleUploaded)
	api.POST("/transactions/:id/tags", s.handleAddTag)
	api.DELETE("/transactions/:id/tags/:tagID", s.handleRemoveTag)

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
		// Try to serve the file directly first (e.g. /favicon.svg)
		candidate := staticDir + path
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			c.File(candidate)
			return
		}
		// SPA fallback: let React Router handle the path
		c.File(staticDir + "/index.html")
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
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "missing token"})
			return
		}
		claims, err := s.jwtSvc.Verify(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "invalid token"})
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
		CaptchaToken string `json:"captcha_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	if err := s.captchaVerifier.Verify(req.CaptchaToken, realIP(c)); err != nil {
		fail(c, 400, 40003, "人机验证失败："+err.Error())
		return
	}
	_, err := s.authSvc.Register(c.Request.Context(), service.RegisterRequest{
		Email: req.Email, Username: req.Username, Password: req.Password,
	})
	if err != nil {
		if err.Error() == "register: username_taken" {
			fail(c, 409, 40902, "该用户名已被使用")
			return
		}
		if err.Error() == "register: email_taken" {
			fail(c, 409, 40901, "该邮箱已被注册")
			return
		}
		fail(c, 409, 40901, err.Error())
		return
	}
	// If email verification is required, don't auto-login.
	if s.authSvc.EmailVerificationRequired() {
		c.JSON(http.StatusAccepted, gin.H{"message": "注册成功，验证邮件已发送，请检查邮箱后登录"})
		return
	}
	// Auto-login after registration (email not required)
	resp, err := s.authSvc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		fail(c, 500, 50001, err.Error())
		return
	}
	created(c, gin.H{
		"token":      resp.Token,
		"expires_at": resp.ExpiresAt.Format(time.RFC3339),
		"user_id":    resp.UserID,
		"email":      resp.Email,
		"username":   resp.Username,
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
		fail(c, 422, 40001, err.Error())
		return
	}
	if err := s.captchaVerifier.Verify(req.CaptchaToken, realIP(c)); err != nil {
		fail(c, 400, 40003, "人机验证失败："+err.Error())
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

func (s *Server) handleResendVerification(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, 40001, err.Error())
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
		fail(c, 422, 40001, err.Error())
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
		fail(c, 422, 40001, err.Error())
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
		fail(c, 422, 40001, err.Error())
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
		fail(c, 422, 40011, err.Error())
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
		fail(c, 500, 50001, err.Error())
		return
	}
	ok(c, gin.H{
		"token":      token,
		"expires_at": exp.Format(time.RFC3339),
		"user_id":    u.ID,
		"email":      u.Email,
		"username":   u.Username,
		"role":       u.Role,
	})
}

func (s *Server) handleRequestEmailChange(c *gin.Context) {
	var req struct {
		NewEmail string `json:"new_email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, 40020, err.Error())
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
		fail(c, 422, 40024, err.Error())
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
		fail(c, 422, 40022, err.Error())
		return
	}
	if err := s.authSvc.ConfirmEmailChange(c.Request.Context(), req.Token); err != nil {
		fail(c, 400, 40023, err.Error())
		return
	}
	ok(c, gin.H{"message": "邮箱已更新"})
}

// ─── Transactions ────────────────────────────────────────────────────────────

func (s *Server) handleListTransactions(c *gin.Context) {
	txs, err := s.txRepo.ListByUser(c.Request.Context(), userID(c))
	if err != nil {
		fail(c, 500, 50001, err.Error())
		return
	}
	type txDTO struct {
		ID         string   `json:"id"`
		OccurredAt string   `json:"occurred_at"`
		Direction  string   `json:"direction"`
		Source     string   `json:"source"`
		Category   string   `json:"category"`
		AmountYuan float64  `json:"amount_yuan"`
		Currency   string   `json:"currency"`
		Note       string   `json:"note"`
		ProjectID  *string  `json:"project_id"`
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
			ID: t.ID, OccurredAt: t.OccurredAt.Format("2006-01-02"),
			Direction: string(t.Direction), Source: string(t.Source),
			Category: t.Category, AmountYuan: t.AmountYuan.Float64(),
			Currency: t.Currency,
			Note:     t.Note, ProjectID: t.ProjectID, Reimbursed: t.Reimbursed,
			Uploaded: t.Uploaded, Tags: tagNames,
		})
	}
	ok(c, dtos)
}

func (s *Server) handleCreateTransaction(c *gin.Context) {
	var req struct {
		OccurredAt string   `json:"occurred_at"`
		Direction  string   `json:"direction"   binding:"required"`
		Source     string   `json:"source"      binding:"required"`
		Category   string   `json:"category"    binding:"required"`
		AmountYuan float64  `json:"amount_yuan" binding:"required,gt=0"`
		Currency   string   `json:"currency"`
		Note       string   `json:"note"`
		ProjectID  *string  `json:"project_id"`
		TagIDs     []string `json:"tag_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	t := time.Now()
	if req.OccurredAt != "" {
		if parsed, err := time.Parse("2006-01-02", req.OccurredAt); err == nil {
			t = parsed
		}
	}
	projID := req.ProjectID
	if projID != nil && *projID == "" {
		projID = nil
	}
	// Auto-create the project if it doesn't exist yet (prevents FK constraint failure
	// when the user types a free-form project identifier in the form).
	if projID != nil {
		_, err := s.db.ExecContext(c.Request.Context(),
			`INSERT OR IGNORE INTO projects(id, name, code, created_at) VALUES (?, ?, ?, ?)`,
			*projID, *projID, *projID, time.Now().Unix(),
		)
		if err != nil {
			fail(c, 500, 50001, "auto-create project: "+err.Error())
			return
		}
	}
	created_, err := s.txSvc.CreateTransaction(c.Request.Context(), service.CreateTransactionRequest{
		UserID: userID(c), OccurredAt: t, Direction: model.Direction(req.Direction),
		Source: model.Source(req.Source), Category: req.Category,
		AmountYuan: model.Money(req.AmountYuan), Currency: req.Currency,
		Note: req.Note, ProjectID: projID,
	})
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	// attach tags
	for _, tagID := range req.TagIDs {
		if err := s.tagRepo.AddToTransaction(c.Request.Context(), created_.ID, tagID); err != nil {
			fail(c, 422, 40002, "tag not found: "+tagID)
			return
		}
	}
	created(c, gin.H{"id": created_.ID, "amount_yuan": created_.AmountYuan.Float64()})
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
		fail(c, 422, 40001, err.Error())
		return
	}
	if err := s.tagRepo.AddToTransaction(c.Request.Context(), txID, req.TagID); err != nil {
		fail(c, 500, 50001, err.Error())
		return
	}
	ok(c, gin.H{"transaction_id": txID, "tag_id": req.TagID})
}

func (s *Server) handleRemoveTag(c *gin.Context) {
	if err := s.tagRepo.RemoveFromTransaction(c.Request.Context(), c.Param("id"), c.Param("tagID")); err != nil {
		fail(c, 500, 50001, err.Error())
		return
	}
	ok(c, gin.H{"removed": true})
}

// ─── Tags ────────────────────────────────────────────────────────────────────

func (s *Server) handleListTags(c *gin.Context) {
	tags, err := s.tagRepo.ListByOwner(c.Request.Context(), userID(c))
	if err != nil {
		fail(c, 500, 50001, err.Error())
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
		fail(c, 422, 40001, err.Error())
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
		fail(c, 409, 40901, err.Error())
		return
	}
	created(c, tag)
}

func (s *Server) handleDeleteTag(c *gin.Context) {
	if err := s.tagRepo.Delete(c.Request.Context(), c.Param("id"), userID(c)); err != nil {
		fail(c, 500, 50001, err.Error())
		return
	}
	ok(c, gin.H{"deleted": true})
}

// ─── Match ───────────────────────────────────────────────────────────────────

func (s *Server) handleMatch(c *gin.Context) {
	var req struct {
		TargetYuan    float64 `json:"target_yuan"    binding:"required,gt=0"`
		ToleranceYuan float64 `json:"tolerance_yuan"`
		MaxItems      int     `json:"max_items"`
		ProjectID     *string `json:"project_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	maxDepth := req.MaxItems
	if maxDepth <= 0 {
		maxDepth = 10
	}
	limit := 20
	results, err := s.matchSvc.Match(
		c.Request.Context(),
		userID(c),
		model.Money(req.TargetYuan), model.Money(req.ToleranceYuan),
		maxDepth, req.ProjectID, limit,
	)
	if err != nil {
		fail(c, 500, 50001, err.Error())
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
			fail(c, 500, 50001, ferr.Error())
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
		Error        float64   `json:"error"`
		ProjectCount int       `json:"project_count"`
		ItemCount    int       `json:"item_count"`
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
			Error:        r.AbsErrorYuan.Float64(),
			ProjectCount: r.ProjectCount,
			ItemCount:    r.ItemCount,
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
		fail(c, 422, 40001, err.Error())
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
		fail(c, 500, 50001, err.Error())
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
		fail(c, 500, 50001, err.Error())
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
		fail(c, 500, 50001, err.Error())
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
		fail(c, 500, 50001, err.Error())
		return
	}
	if stats == nil {
		stats = []service.ProjectStat{}
	}
	ok(c, stats)
}

// handleBackupDownload creates a consistent snapshot of the database using
// SQLite Online Backup API and streams it as a downloadable file.
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
		fail(c, 500, 50001, "备份失败: "+err.Error())
		return
	}

	ts := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("finarch_backup_%s.db", ts)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "application/octet-stream")
	c.File(tmpPath)
}

// handleRestore accepts a SQLite database file upload and restores it into the
// live database using the SQLite Online Backup API — no restart needed.
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
	f, _ := os.Open(tmpPath)
	f.Read(magic)
	f.Close()
	if string(magic[:15]) != "SQLite format 3" {
		fail(c, 400, 40003, "文件不是有效的 SQLite 数据库")
		return
	}

	// Open the uploaded DB as source
	srcDB, err := sql.Open("sqlite3", tmpPath)
	if err != nil {
		fail(c, 500, 50002, "无法打开备份文件")
		return
	}
	defer srcDB.Close()

	ctx := context.Background()

	// Use SQLite backup API: copy from uploaded file into live DB
	destConn, err := s.db.Conn(ctx)
	if err != nil {
		fail(c, 500, 50002, "获取数据库连接失败")
		return
	}
	defer destConn.Close()

	srcConn, err := srcDB.Conn(ctx)
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
		fail(c, 500, 50003, "恢复失败: "+restoreErr.Error())
		return
	}

	ok(c, gin.H{"message": "数据恢复成功，数据库已更新"})
}
