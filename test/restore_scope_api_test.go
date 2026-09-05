package test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"finarch/internal/infrastructure/auth"
)

func TestDisasterRecoveryRejectsUnknownScopeBeforeRestoreExecution(t *testing.T) {
	enableTestSystemOperations(t)
	database := setupDB(t)
	defer database.Close()

	jwtSvc := auth.NewJWTService("test-secret")
	token := issueTestAccessSession(t, database, jwtSvc, testUserID)
	if _, err := database.Exec(`CREATE TABLE restore_scope_probe (value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO restore_scope_probe(value) VALUES ('unchanged')`); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	litestreamMarker := filepath.Join(t.TempDir(), "litestream-invoked")
	script := "#!/bin/sh\nset -eu\nprintf invoked > \"$RESTORE_SCOPE_LITESTREAM_MARKER\"\nexit 99\n"
	if err := os.WriteFile(filepath.Join(binDir, "litestream"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("RESTORE_SCOPE_LITESTREAM_MARKER", litestreamMarker)
	t.Setenv("DISASTER_SNAPSHOT_METADATA_PATH", "")

	srv := newTestServer(t, database, jwtSvc)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/disaster-recovery/restore",
		bytes.NewBufferString(`{"snapshot_id":"never-load-this","confirm":true,"apply_mode":"merge","restore_scope":"wrok","authorization_token":"never-verify-this"}`),
	)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	authorizeTestSystemOperations(req)
	response := serveTestRequest(srv, req)
	if response.Code != http.StatusUnprocessableEntity || apiErrorCode(t, response) != "INVALID_INPUT" {
		t.Fatalf("unknown restore scope: status=%d body=%s", response.Code, response.Body.String())
	}

	if _, err := os.Stat(litestreamMarker); !os.IsNotExist(err) {
		t.Fatalf("invalid scope reached snapshot/restore execution: %v", err)
	}
	var probe string
	if err := database.QueryRow(`SELECT value FROM restore_scope_probe`).Scan(&probe); err != nil {
		t.Fatal(err)
	}
	if probe != "unchanged" {
		t.Fatalf("invalid scope changed live data to %q", probe)
	}
	var transactionCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM transactions WHERE user_id = ?`, testUserID).Scan(&transactionCount); err != nil {
		t.Fatal(err)
	}
	if transactionCount != 0 {
		t.Fatalf("invalid scope wrote %d transactions", transactionCount)
	}
}
