package apiv1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"

	"github.com/gin-gonic/gin"
)

type recurringRuleDTO struct {
	ID                 string  `json:"id"`
	Mode               string  `json:"mode"`
	Name               string  `json:"name"`
	Status             string  `json:"status"`
	AccountID          string  `json:"account_id"`
	Type               string  `json:"type"`
	Direction          string  `json:"direction"`
	Category           string  `json:"category"`
	AmountCents        int64   `json:"amount_cents"`
	AmountYuan         float64 `json:"amount_yuan"`
	Currency           string  `json:"currency"`
	ExchangeRate       float64 `json:"exchange_rate"`
	Note               string  `json:"note"`
	ProjectID          *string `json:"project_id"`
	Frequency          string  `json:"frequency"`
	Interval           int     `json:"interval"`
	StartDate          string  `json:"start_date"`
	EndDate            *string `json:"end_date"`
	TimeOfDay          string  `json:"time_of_day"`
	Timezone           string  `json:"timezone"`
	DayOfWeek          *int    `json:"day_of_week"`
	DayOfMonth         *int    `json:"day_of_month"`
	MonthEndPolicy     string  `json:"month_end_policy"`
	NextRunAt          int64   `json:"next_run_at"`
	NextOccurredAt     string  `json:"next_occurred_at"`
	LastGeneratedFor   *string `json:"last_generated_for"`
	CatchUpEnabled     bool    `json:"catch_up_enabled"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
	PreviewOccurrences []any   `json:"preview_occurrences,omitempty"`
}

type recurringInstanceDTO struct {
	ID             string  `json:"id"`
	RuleID         string  `json:"rule_id"`
	OccurrenceDate string  `json:"occurrence_date"`
	ScheduledAt    int64   `json:"scheduled_at"`
	OccurredAt     string  `json:"occurred_at"`
	TransactionID  *string `json:"transaction_id"`
	Status         string  `json:"status"`
	Error          *string `json:"error"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

func recurringRuleToDTO(rule model.RecurringTransactionRule) recurringRuleDTO {
	return recurringRuleDTO{
		ID:               rule.ID,
		Mode:             string(rule.Mode),
		Name:             rule.Name,
		Status:           string(rule.Status),
		AccountID:        rule.AccountID,
		Type:             string(rule.TxType),
		Direction:        string(rule.TxType),
		Category:         rule.Category,
		AmountCents:      rule.AmountCents,
		AmountYuan:       float64(rule.AmountCents) / 100,
		Currency:         rule.Currency,
		ExchangeRate:     rule.ExchangeRate,
		Note:             rule.Note,
		ProjectID:        rule.ProjectID,
		Frequency:        string(rule.Frequency),
		Interval:         rule.Interval,
		StartDate:        rule.StartDate,
		EndDate:          rule.EndDate,
		TimeOfDay:        rule.TimeOfDay,
		Timezone:         rule.Timezone,
		DayOfWeek:        rule.DayOfWeek,
		DayOfMonth:       rule.DayOfMonth,
		MonthEndPolicy:   string(rule.MonthEndPolicy),
		NextRunAt:        rule.NextRunAt,
		NextOccurredAt:   formatSecond(time.Unix(rule.NextRunAt, 0)),
		LastGeneratedFor: rule.LastGeneratedFor,
		CatchUpEnabled:   rule.CatchUpEnabled,
		CreatedAt:        formatSecond(rule.CreatedAt),
		UpdatedAt:        formatSecond(rule.UpdatedAt),
	}
}

func recurringInstanceToDTO(inst model.RecurringTransactionInstance) recurringInstanceDTO {
	return recurringInstanceDTO{
		ID:             inst.ID,
		RuleID:         inst.RuleID,
		OccurrenceDate: inst.OccurrenceDate,
		ScheduledAt:    inst.ScheduledAt,
		OccurredAt:     formatSecond(time.Unix(inst.ScheduledAt, 0)),
		TransactionID:  inst.TransactionID,
		Status:         string(inst.Status),
		Error:          inst.Error,
		CreatedAt:      formatSecond(inst.CreatedAt),
		UpdatedAt:      formatSecond(inst.UpdatedAt),
	}
}

type recurringRuleJSONRequest struct {
	Mode           string  `json:"mode"`
	Name           string  `json:"name"`
	Status         string  `json:"status"`
	AccountID      string  `json:"account_id"`
	Type           string  `json:"type"`
	Direction      string  `json:"direction"`
	Category       string  `json:"category"`
	AmountCents    int64   `json:"amount_cents"`
	AmountYuan     float64 `json:"amount_yuan"`
	Currency       string  `json:"currency"`
	ExchangeRate   float64 `json:"exchange_rate"`
	Note           string  `json:"note"`
	ProjectID      *string `json:"project_id"`
	Frequency      string  `json:"frequency"`
	Interval       int     `json:"interval"`
	StartDate      string  `json:"start_date"`
	EndDate        *string `json:"end_date"`
	TimeOfDay      string  `json:"time_of_day"`
	Timezone       string  `json:"timezone"`
	DayOfWeek      *int    `json:"day_of_week"`
	DayOfMonth     *int    `json:"day_of_month"`
	MonthEndPolicy string  `json:"month_end_policy"`
	CatchUpEnabled *bool   `json:"catch_up_enabled"`
}

func recurringRequestFromJSON(uid, id string, req recurringRuleJSONRequest) (service.UpsertRecurringRuleRequest, error) {
	var mode model.Mode
	if strings.TrimSpace(req.Mode) == "" && id != "" {
		mode = ""
	} else {
		parsedMode, modeOK := parseMode(req.Mode)
		if !modeOK {
			return service.UpsertRecurringRuleRequest{}, fmt.Errorf("invalid mode")
		}
		mode = parsedMode
	}
	return service.UpsertRecurringRuleRequest{
		ID:             id,
		UserID:         uid,
		Mode:           mode,
		Name:           req.Name,
		Status:         model.RecurringRuleStatus(req.Status),
		AccountID:      req.AccountID,
		TxType:         model.TxType(req.Type),
		Direction:      model.Direction(req.Direction),
		Category:       req.Category,
		AmountCents:    req.AmountCents,
		AmountYuan:     model.Money(req.AmountYuan),
		Currency:       req.Currency,
		ExchangeRate:   req.ExchangeRate,
		Note:           req.Note,
		ProjectID:      req.ProjectID,
		Frequency:      model.RecurringFrequency(req.Frequency),
		Interval:       req.Interval,
		StartDate:      req.StartDate,
		EndDate:        req.EndDate,
		TimeOfDay:      req.TimeOfDay,
		Timezone:       req.Timezone,
		DayOfWeek:      req.DayOfWeek,
		DayOfMonth:     req.DayOfMonth,
		MonthEndPolicy: model.MonthEndPolicy(req.MonthEndPolicy),
		CatchUpEnabled: req.CatchUpEnabled,
	}, nil
}

func (s *Server) handleListRecurringRules(c *gin.Context) {
	if s.recurringSvc == nil {
		fail(c, 503, "RECURRING_UNAVAILABLE", "周期交易服务不可用")
		return
	}
	mode, modeOK := parseMode(c.Query("mode"))
	if !modeOK {
		fail(c, 400, 40001, "invalid mode")
		return
	}
	rules, err := s.recurringSvc.ListRules(c.Request.Context(), userID(c), mode)
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	dtos := make([]recurringRuleDTO, 0, len(rules))
	for _, rule := range rules {
		dtos = append(dtos, recurringRuleToDTO(rule))
	}
	ok(c, dtos)
}

func (s *Server) handleCreateRecurringRule(c *gin.Context) {
	if s.recurringSvc == nil {
		fail(c, 503, "RECURRING_UNAVAILABLE", "周期交易服务不可用")
		return
	}
	var req recurringRuleJSONRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	upsert, err := recurringRequestFromJSON(userID(c), "", req)
	if err != nil {
		fail(c, 400, 40001, err.Error())
		return
	}
	if err := s.ensureProjectExists(c.Request.Context(), upsert.ProjectID); err != nil {
		fail(c, 500, 50001, "创建项目失败，请稍后重试")
		return
	}
	rule, err := s.recurringSvc.CreateRule(c.Request.Context(), upsert)
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	created(c, recurringRuleToDTO(rule))
}

func (s *Server) handleUpdateRecurringRule(c *gin.Context) {
	if s.recurringSvc == nil {
		fail(c, 503, "RECURRING_UNAVAILABLE", "周期交易服务不可用")
		return
	}
	var req recurringRuleJSONRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	upsert, err := recurringRequestFromJSON(userID(c), c.Param("id"), req)
	if err != nil {
		fail(c, 400, 40001, err.Error())
		return
	}
	if err := s.ensureProjectExists(c.Request.Context(), upsert.ProjectID); err != nil {
		fail(c, 500, 50001, "创建项目失败，请稍后重试")
		return
	}
	rule, err := s.recurringSvc.UpdateRule(c.Request.Context(), upsert)
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	ok(c, recurringRuleToDTO(rule))
}

func (s *Server) handleUpdateRecurringRuleStatus(c *gin.Context) {
	if s.recurringSvc == nil {
		fail(c, 503, "RECURRING_UNAVAILABLE", "周期交易服务不可用")
		return
	}
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	rule, err := s.recurringSvc.SetStatus(c.Request.Context(), userID(c), c.Param("id"), model.RecurringRuleStatus(req.Status))
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	ok(c, recurringRuleToDTO(rule))
}

func (s *Server) handleDeleteRecurringRule(c *gin.Context) {
	if s.recurringSvc == nil {
		fail(c, 503, "RECURRING_UNAVAILABLE", "周期交易服务不可用")
		return
	}
	if err := s.recurringSvc.DeleteRule(c.Request.Context(), userID(c), c.Param("id")); err != nil {
		fail(c, 400, 40001, err.Error())
		return
	}
	ok(c, gin.H{"id": c.Param("id"), "deleted": true})
}

func (s *Server) handleListRecurringInstances(c *gin.Context) {
	if s.recurringSvc == nil {
		fail(c, 503, "RECURRING_UNAVAILABLE", "周期交易服务不可用")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, err := s.recurringSvc.ListInstances(c.Request.Context(), userID(c), c.Param("id"), limit)
	if err != nil {
		fail(c, 400, 40001, err.Error())
		return
	}
	dtos := make([]recurringInstanceDTO, 0, len(items))
	for _, item := range items {
		dtos = append(dtos, recurringInstanceToDTO(item))
	}
	ok(c, dtos)
}

func (s *Server) handleGenerateRecurringRuleNow(c *gin.Context) {
	if s.recurringSvc == nil {
		fail(c, 503, "RECURRING_UNAVAILABLE", "周期交易服务不可用")
		return
	}
	res, err := s.recurringSvc.GenerateRuleNow(c.Request.Context(), userID(c), c.Param("id"), false)
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	ok(c, res)
}

func (s *Server) handlePreviewRecurringRules(c *gin.Context) {
	if s.recurringSvc == nil {
		fail(c, 503, "RECURRING_UNAVAILABLE", "周期交易服务不可用")
		return
	}
	amountYuan, _ := strconv.ParseFloat(c.Query("amount_yuan"), 64)
	amountCents, _ := strconv.ParseInt(c.Query("amount_cents"), 10, 64)
	interval, _ := strconv.Atoi(c.Query("interval"))
	count, _ := strconv.Atoi(c.DefaultQuery("count", "5"))
	var dayOfMonth *int
	if v := c.Query("day_of_month"); v != "" {
		parsed, _ := strconv.Atoi(v)
		dayOfMonth = &parsed
	}
	var dayOfWeek *int
	if v := c.Query("day_of_week"); v != "" {
		parsed, _ := strconv.Atoi(v)
		dayOfWeek = &parsed
	}
	mode, modeOK := parseMode(c.Query("mode"))
	if !modeOK {
		fail(c, 400, 40001, "invalid mode")
		return
	}
	items, err := s.recurringSvc.PreviewFromRequest(service.UpsertRecurringRuleRequest{
		UserID:         userID(c),
		Mode:           mode,
		Name:           c.DefaultQuery("name", "preview"),
		AccountID:      c.Query("account_id"),
		TxType:         model.TxType(c.Query("type")),
		Direction:      model.Direction(c.Query("direction")),
		Category:       c.DefaultQuery("category", "preview"),
		AmountCents:    amountCents,
		AmountYuan:     model.Money(amountYuan),
		Currency:       c.DefaultQuery("currency", "CNY"),
		Frequency:      model.RecurringFrequency(c.DefaultQuery("frequency", "monthly")),
		Interval:       interval,
		StartDate:      c.Query("start_date"),
		EndDate:        optionalQuery(c, "end_date"),
		TimeOfDay:      c.Query("time_of_day"),
		Timezone:       c.Query("timezone"),
		DayOfWeek:      dayOfWeek,
		DayOfMonth:     dayOfMonth,
		MonthEndPolicy: model.MonthEndPolicy(c.Query("month_end_policy")),
	}, count)
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	ok(c, items)
}

func optionalQuery(c *gin.Context, key string) *string {
	v := strings.TrimSpace(c.Query(key))
	if v == "" {
		return nil
	}
	return &v
}

func (s *Server) ensureProjectExists(ctx context.Context, projectID *string) error {
	if projectID == nil || strings.TrimSpace(*projectID) == "" {
		return nil
	}
	id := strings.TrimSpace(*projectID)
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO projects(id, name, code, created_at) VALUES (?, ?, ?, ?)`,
		id, id, id, time.Now().Unix(),
	)
	return err
}

type attachmentDTO struct {
	ID               string  `json:"id"`
	TransactionID    *string `json:"transaction_id"`
	StorageKey       string  `json:"storage_key"`
	OriginalFilename string  `json:"original_filename"`
	ContentType      string  `json:"content_type"`
	SizeBytes        int64   `json:"size_bytes"`
	SHA256           string  `json:"sha256"`
	Kind             string  `json:"kind"`
	OCRStatus        string  `json:"ocr_status"`
	OCRProvider      *string `json:"ocr_provider"`
	OCRText          *string `json:"ocr_text"`
	OCRJSON          *string `json:"ocr_json"`
	OCRResult        any     `json:"ocr_result,omitempty"`
	OCRError         *string `json:"ocr_error"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

func attachmentToDTO(a model.Attachment) attachmentDTO {
	dto := attachmentDTO{ID: a.ID, TransactionID: a.TransactionID, StorageKey: a.StorageKey, OriginalFilename: a.OriginalFilename, ContentType: a.ContentType, SizeBytes: a.SizeBytes, SHA256: a.SHA256, Kind: string(a.Kind), OCRStatus: string(a.OCRStatus), OCRProvider: a.OCRProvider, OCRText: a.OCRText, OCRJSON: a.OCRJSON, OCRError: a.OCRError, CreatedAt: formatSecond(a.CreatedAt), UpdatedAt: formatSecond(a.UpdatedAt)}
	if a.OCRJSON != nil && *a.OCRJSON != "" {
		var raw any
		if err := json.Unmarshal([]byte(*a.OCRJSON), &raw); err == nil {
			dto.OCRResult = raw
		}
	}
	return dto
}

func (s *Server) handleUploadAttachment(c *gin.Context) {
	s.uploadAttachment(c, nil)
}

func (s *Server) handleUploadTransactionAttachment(c *gin.Context) {
	txID := c.Param("id")
	s.uploadAttachment(c, &txID)
}

func (s *Server) uploadAttachment(c *gin.Context, forcedTransactionID *string) {
	if s.attachmentSvc == nil {
		fail(c, 503, "ATTACHMENTS_UNAVAILABLE", "附件服务不可用")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.attachmentSvc.MaxBytes()+1<<20)
	fh, err := c.FormFile("file")
	if err != nil {
		fail(c, 400, 40001, "请上传附件文件（multipart field: file）")
		return
	}
	txID := forcedTransactionID
	if txID == nil {
		txID = optionalQuery(c, "transaction_id")
	}
	runOCR := c.PostForm("run_ocr") == "true" || c.Query("run_ocr") == "true"
	kind := model.AttachmentKind(c.DefaultPostForm("kind", c.DefaultQuery("kind", "receipt")))
	file, err := fh.Open()
	if err != nil {
		fail(c, 500, 50001, "无法读取上传文件")
		return
	}
	defer file.Close()
	attachment, err := s.attachmentSvc.Upload(c.Request.Context(), service.UploadAttachmentRequest{UserID: userID(c), TransactionID: txID, OriginalFilename: fh.Filename, ContentType: fh.Header.Get("Content-Type"), Kind: kind, Reader: file, RunOCR: runOCR})
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	created(c, attachmentToDTO(attachment))
}

func (s *Server) handleListTransactionAttachments(c *gin.Context) {
	if s.attachmentSvc == nil {
		fail(c, 503, "ATTACHMENTS_UNAVAILABLE", "附件服务不可用")
		return
	}
	attachments, err := s.attachmentSvc.ListByTransaction(c.Request.Context(), userID(c), c.Param("id"))
	if err != nil {
		fail(c, 400, 40001, err.Error())
		return
	}
	dtos := make([]attachmentDTO, 0, len(attachments))
	for _, a := range attachments {
		dtos = append(dtos, attachmentToDTO(a))
	}
	ok(c, dtos)
}

func (s *Server) handleGetAttachment(c *gin.Context) {
	if s.attachmentSvc == nil {
		fail(c, 503, "ATTACHMENTS_UNAVAILABLE", "附件服务不可用")
		return
	}
	attachment, err := s.attachmentSvc.Get(c.Request.Context(), userID(c), c.Param("id"))
	if err != nil {
		fail(c, 404, 40401, "附件不存在")
		return
	}
	ok(c, attachmentToDTO(attachment))
}

func (s *Server) handleDownloadAttachment(c *gin.Context) {
	if s.attachmentSvc == nil {
		fail(c, 503, "ATTACHMENTS_UNAVAILABLE", "附件服务不可用")
		return
	}
	attachment, r, err := s.attachmentSvc.Open(c.Request.Context(), userID(c), c.Param("id"))
	if err != nil {
		fail(c, 404, 40401, "附件不存在")
		return
	}
	defer r.Close()
	filename := strings.ReplaceAll(filepath.Base(attachment.OriginalFilename), `"`, "'")
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.DataFromReader(http.StatusOK, attachment.SizeBytes, attachment.ContentType, r, nil)
}

func (s *Server) handleDeleteAttachment(c *gin.Context) {
	if s.attachmentSvc == nil {
		fail(c, 503, "ATTACHMENTS_UNAVAILABLE", "附件服务不可用")
		return
	}
	if err := s.attachmentSvc.Delete(c.Request.Context(), userID(c), c.Param("id")); err != nil {
		fail(c, 404, 40401, "附件不存在")
		return
	}
	ok(c, gin.H{"id": c.Param("id"), "deleted": true})
}

func (s *Server) handleLinkAttachment(c *gin.Context) {
	if s.attachmentSvc == nil {
		fail(c, 503, "ATTACHMENTS_UNAVAILABLE", "附件服务不可用")
		return
	}
	var req struct {
		TransactionID string `json:"transaction_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		failBind(c)
		return
	}
	attachment, err := s.attachmentSvc.Link(c.Request.Context(), userID(c), c.Param("id"), req.TransactionID)
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	ok(c, attachmentToDTO(attachment))
}

func (s *Server) handleRunAttachmentOCR(c *gin.Context) {
	if s.attachmentSvc == nil {
		fail(c, 503, "ATTACHMENTS_UNAVAILABLE", "附件服务不可用")
		return
	}
	attachment, err := s.attachmentSvc.RunOCR(c.Request.Context(), userID(c), c.Param("id"))
	if err != nil {
		fail(c, 422, 40001, err.Error())
		return
	}
	ok(c, attachmentToDTO(attachment))
}

func (s *Server) handleGetAttachmentOCR(c *gin.Context) {
	s.handleGetAttachment(c)
}
