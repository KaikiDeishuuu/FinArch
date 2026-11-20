package test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
)

func TestAttachmentUploadFileAndEnvelopeLimits(t *testing.T) {
	db := setupDB(t)
	defer db.Close()
	jwtSvc := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, db, jwtSvc, testUserID)
	server := newTestServer(t, db, jwtSvc)
	config := serveTestRequest(server, httptest.NewRequest("GET", "/api/v1/config", nil))
	var limits struct {
		Data struct {
			File int `json:"attachment_max_bytes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(config.Body.Bytes(), &limits); err != nil || limits.Data.File != int(service.DefaultAttachmentMaxBytes) {
		t.Fatalf("public limit: %+v, %v", limits, err)
	}
	for _, tc := range []struct {
		name, path    string
		extra, status int
	}{
		{"at limit", "/api/v1/attachments", 0, http.StatusCreated},
		{"one byte over", "/api/v1/attachments", 1, http.StatusRequestEntityTooLarge},
		{"linked upload", "/api/v1/transactions/test-id/attachments", 1, http.StatusRequestEntityTooLarge},
		{"envelope exceeded", "/api/v1/attachments", 1 << 20, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, err := writer.CreateFormFile("file", "receipt.pdf")
			if err != nil {
				t.Fatal(err)
			}
			file := make([]byte, limits.Data.File+tc.extra)
			copy(file, "%PDF-1.7\n")
			if _, err := part.Write(file); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("POST", tc.path, &body)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			resp := serveTestRequest(server, req)
			if resp.Code != tc.status {
				t.Fatalf("status %d: %s", resp.Code, resp.Body.String())
			}
			if tc.status == http.StatusRequestEntityTooLarge && apiErrorCode(t, resp) != "ATTACHMENT_TOO_LARGE" {
				t.Fatal(resp.Body.String())
			}
		})
	}
}
