package test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/infrastructure/auth"
	sqliterepo "finarch/internal/infrastructure/repository"
)

func TestLegacyNavigationCannotDeleteDraftBeforeLink(t *testing.T) {
	ctx := context.Background()
	database := setupFileDB(t, ctx)
	defer database.Close()
	repo := sqliterepo.NewSQLiteAttachmentRepository(database)
	now := time.Now().UTC()
	draft := model.Attachment{
		ID: "draft-id", UserID: testUserID, StorageKey: "test/receipt.pdf", OriginalFilename: "receipt.pdf",
		ContentType: "application/pdf", SizeBytes: 100, SHA256: strings.Repeat("a", 64), Kind: model.AttachmentKindReceipt,
		OCRStatus: model.OCRStatusNotRequested, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, draft); err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, jwt, testUserID)
	handler := newTestServer(t, database, jwt).Handler()
	remove := func(suffix string) int {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/attachments/"+draft.ID+suffix, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		reply := httptest.NewRecorder()
		handler.ServeHTTP(reply, req)
		return reply.Code
	}
	if status := remove(""); status != http.StatusUnprocessableEntity {
		t.Fatalf("legacy cleanup returned %d", status)
	}
	if _, err := repo.GetByID(ctx, draft.ID, testUserID); err != nil {
		t.Fatalf("draft disappeared before the link could run: %v", err)
	}
	if status := remove("/draft"); status != http.StatusOK {
		t.Fatalf("intentional draft cleanup returned %d", status)
	}
	if _, err := repo.GetByID(ctx, draft.ID, testUserID); err == nil {
		t.Fatal("abandoned draft survived explicit cleanup")
	}
}
