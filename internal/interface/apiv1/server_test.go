package apiv1

import (
	"errors"
	"net/http"
	"testing"

	"finarch/internal/domain/service"
)

func TestMapDomainError_UsernameTaken(t *testing.T) {
	status, payload := mapDomainError(service.ErrUsernameTaken)
	if status != http.StatusConflict {
		t.Fatalf("expected 409, got %d", status)
	}
	if payload.Code != "username_taken" {
		t.Fatalf("expected username_taken code, got %s", payload.Code)
	}
}

func TestMapDomainError_ExpiredToken(t *testing.T) {
	status, payload := mapDomainError(service.ErrExpiredToken)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", status)
	}
	if payload.Code != "expired_token" {
		t.Fatalf("expected expired_token code, got %s", payload.Code)
	}
}

func TestMapDomainError_InternalNeverLeaksSQL(t *testing.T) {
	status, payload := mapDomainError(errors.New("sql: no rows in result set"))
	if status != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", status)
	}
	if payload.Code != "internal_error" {
		t.Fatalf("expected internal_error code, got %s", payload.Code)
	}
	if payload.Message == "sql: no rows in result set" {
		t.Fatalf("raw SQL error must not be exposed")
	}
}
