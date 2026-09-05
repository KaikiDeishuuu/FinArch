package apiv1

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"finarch/internal/domain/service"

	"github.com/gin-gonic/gin"
)

type attachmentUploadTrackingBody struct {
	reads int
}

func (b *attachmentUploadTrackingBody) Read([]byte) (int, error) {
	b.reads++
	return 0, errors.New("request body must not be read")
}

func (*attachmentUploadTrackingBody) Close() error { return nil }

func TestUploadAttachmentRateLimitRejectsBeforeReadingMultipartBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	attachmentSvc := service.NewAttachmentService(nil, nil, nil, nil, service.DefaultAttachmentMaxBytes)
	attachmentSvc.ConfigureUploadRateLimit(1, time.Minute)
	if !attachmentSvc.AllowUpload("user-1") {
		t.Fatal("failed to consume initial upload allowance")
	}
	server := &Server{attachmentSvc: attachmentSvc}
	router := gin.New()
	router.POST("/attachments", func(c *gin.Context) {
		c.Set("userID", "user-1")
		server.handleUploadAttachment(c)
	})

	body := &attachmentUploadTrackingBody{}
	request := httptest.NewRequest(http.MethodPost, "/attachments", nil)
	request.Header.Set("Content-Type", "multipart/form-data; boundary=not-read")
	request.Body = body
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusTooManyRequests, response.Body.String())
	}
	if got := response.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want 60", got)
	}
	if body.reads != 0 {
		t.Fatalf("multipart body was read %d time(s)", body.reads)
	}
	var payload struct {
		Error apiErrorPayload `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error.Code != "ATTACHMENT_UPLOAD_RATE_LIMITED" {
		t.Fatalf("error code = %q", payload.Error.Code)
	}
}
