package apiv1

import (
	"database/sql"
	"net/http"
	"os"
	"strings"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Server is the Gin-based API v1 server.
type Server struct {
	engine   *gin.Engine
	addr     string
	authSvc  *service.AuthService
	txSvc    *service.TransactionService
	reimSvc  *service.ReimbursementService
	matchSvc *service.MatchingService
	statsSvc *service.StatsService
	txRepo   repository.TransactionRepository
	tagRepo  repository.TagRepository
	jwtSvc   *auth.JWTService
}

func NewServer(
	addr string,
	db *sql.DB,
	txRepo repository.TransactionRepository,
	tagRepo repository.TagRepository,
	txSvc *service.TransactionService,
	reimSvc *service.ReimbursementService,
	matchSvc *service.MatchingService,
	authSvc *service.AuthService,
	statsSvc *service.StatsService,
	jwtSvc *auth.JWTService,
) *Server {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	s := &Server{
		engine:   gin.New(),
		addr:     addr,
		authSvc:  authSvc,
		txSvc:    txSvc,
		reimSvc:  reimSvc,
		matchSvc: matchSvc,
		statsSvc: statsSvc,
		txRepo:   txRepo,
		tagRepo:  tagRepo,
		jwtSvc:   jwtSvc,
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
	pub.POST("/auth/register", s.handleRegister)
	pub.POST("/auth/login", s.handleLogin)

	// ─── Protected routes (JWT required) ─────────────────────────
	api := r.Group("/api/v1", s.jwtMiddleware())

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
		c.Set("userID", claims.UserID)
		c.Set("userEmail", claims.Email)
		c.Set("userRole", claims.Role)
		c.Next()
	}
}

func userID(c *gin.Context) string { return c.GetString("userID") }

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": data})
}

func created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, gin.H{"code": 0, "message": "created", "data": data})
}

func fail(c *gin.Context, status, code int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"code": code, "message": msg})
}

// ─── Auth ────────────────────────────────────────────────────────────────────

func (s *Server) handleRegister(c *gin.Context) {
	var req struct {
		Email    string `json:"email"    binding:"required,email"`
		Name     string `json:"name"     binding:"required"`
		Password string `json:"password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	_, err := s.authSvc.Register(c.Request.Context(), service.RegisterRequest{
		Email: req.Email, Name: req.Name, Password: req.Password,
	})
	if err != nil {
		fail(c, 409, 40901, err.Error())
		return
	}
	// Auto-login after registration
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
		"name":       resp.Name,
		"role":       resp.Role,
	})
}

func (s *Server) handleLogin(c *gin.Context) {
	var req struct {
		Email    string `json:"email"    binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	resp, err := s.authSvc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		fail(c, 401, 40101, err.Error())
		return
	}
	ok(c, gin.H{
		"token":      resp.Token,
		"expires_at": resp.ExpiresAt.Format(time.RFC3339),
		"user_id":    resp.UserID,
		"email":      resp.Email,
		"name":       resp.Name,
		"role":       resp.Role,
	})
}

// ─── Transactions ────────────────────────────────────────────────────────────

func (s *Server) handleListTransactions(c *gin.Context) {
	txs, err := s.txRepo.ListAll(c.Request.Context())
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
			Note: t.Note, ProjectID: t.ProjectID, Reimbursed: t.Reimbursed,
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
	created_, err := s.txSvc.CreateTransaction(c.Request.Context(), service.CreateTransactionRequest{
		OccurredAt: t, Direction: model.Direction(req.Direction),
		Source: model.Source(req.Source), Category: req.Category,
		AmountYuan: model.Money(req.AmountYuan), Note: req.Note, ProjectID: projID,
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
	newState, err := s.txRepo.ToggleReimbursed(c.Request.Context(), id)
	if err != nil {
		fail(c, 400, 40001, err.Error())
		return
	}
	ok(c, gin.H{"id": id, "reimbursed": newState})
}

func (s *Server) handleToggleUploaded(c *gin.Context) {
	id := c.Param("id")
	newState, err := s.txRepo.ToggleUploaded(c.Request.Context(), id)
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
		Note       string  `json:"note"`
		ProjectID  *string `json:"project_id"`
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
					Note:       t.Note,
					ProjectID:  t.ProjectID,
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
	b, err := s.statsSvc.Summary(c.Request.Context())
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
	stats, err := s.statsSvc.Monthly(c.Request.Context(), year)
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
	stats, err := s.statsSvc.ByCategory(c.Request.Context(), c.Query("date_from"), c.Query("date_to"))
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
	stats, err := s.statsSvc.ByProject(c.Request.Context())
	if err != nil {
		fail(c, 500, 50001, err.Error())
		return
	}
	if stats == nil {
		stats = []service.ProjectStat{}
	}
	ok(c, stats)
}
