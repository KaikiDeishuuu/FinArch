package apiv1

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	findb "finarch/internal/infrastructure/db"
	"finarch/internal/infrastructure/email"
	sqliterepo "finarch/internal/infrastructure/repository"

	"github.com/gin-gonic/gin"
)

func TestNormalizeRestoreScope(t *testing.T) {
	cases := map[string]string{
		"":      "both",
		"both":  "both",
		"BoTh":  "both",
		"work":  "work",
		"WORK":  "work",
		"life":  "life",
		"LiFe":  "life",
		"other": "",
	}
	for in, want := range cases {
		if got := normalizeRestoreScope(in); got != want {
			t.Fatalf("normalizeRestoreScope(%q)=%q want %q", in, got, want)
		}
	}
}

func TestExecuteDisasterRecoveryReturnsMeasuredDuration(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	result := (&Server{}).executeDisasterRecovery(context.Background(), "snapshot", 1, "replace", "both")
	if result.Success {
		t.Fatal("restore unexpectedly succeeded without litestream")
	}
	if result.Duration <= 0 {
		t.Fatalf("duration = %s, want a measured positive duration", result.Duration)
	}
}

func TestExecuteDisasterRecoveryPassesAbsentPathToLitestream(t *testing.T) {
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "output-state")
	outputPathMarker := filepath.Join(t.TempDir(), "output-path")
	script := `#!/bin/sh
set -eu
for output do :; done
if [ -e "$output" ]; then
  printf 'exists' > "$FAKE_LITESTREAM_MARKER"
  exit 90
fi
printf 'absent' > "$FAKE_LITESTREAM_MARKER"
printf '%s' "$output" > "$FAKE_LITESTREAM_OUTPUT_PATH"
printf 'not a sqlite database' > "$output"
`
	if err := os.WriteFile(filepath.Join(binDir, "litestream"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_LITESTREAM_MARKER", marker)
	t.Setenv("FAKE_LITESTREAM_OUTPUT_PATH", outputPathMarker)

	result := (&Server{}).executeDisasterRecovery(context.Background(), "2026-01-01T00:00:00Z", 1, "replace", "both")
	if result.Success {
		t.Fatal("invalid fake snapshot unexpectedly restored")
	}
	state, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(state) != "absent" {
		t.Fatalf("litestream output state = %q, want absent", state)
	}
	outputPath, err := os.ReadFile(outputPathMarker)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(string(outputPath)); !os.IsNotExist(err) {
		t.Fatalf("temporary disaster recovery output was not removed: %v", err)
	}
}

func TestRestoreSendVerificationRejectsNonSQLiteUploadBeforeCreatingSession(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "malicious.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("not a sqlite database")); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("original_email", "owner@example.test"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/backup/restore/send-verification", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = request
	c.Set("userID", "restore-test-user")
	c.Set("userEmail", "current@example.test")

	s := &Server{}
	s.handleRestoreSendVerification(c)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed restore upload status=%d body=%s", response.Code, response.Body.String())
	}
	count := 0
	s.pendingRestores.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != 0 {
		t.Fatalf("malformed restore upload created %d pending session(s)", count)
	}
}

func TestExecuteDisasterRecoveryMergeRejectsDatabaseOnlyAttachmentSnapshot(t *testing.T) {
	baseDir := t.TempDir()
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "r2-attachment", true)
	insertRestoreTestAttachment(t, sourceDB, "r2-attachment", "restore-test-user/receipt.pdf", []byte("receipt bytes"), 0)
	sourceVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	script := `#!/bin/sh
set -eu
for output do :; done
if [ -e "$output" ]; then
  exit 90
fi
cp "$FAKE_LITESTREAM_SOURCE" "$output"
`
	if err := os.WriteFile(filepath.Join(binDir, "litestream"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_LITESTREAM_SOURCE", sourcePath)

	result := (&Server{}).executeDisasterRecovery(context.Background(), "2026-01-01T00:00:00Z", sourceVersion, "merge", "both")
	if result.Success || !strings.Contains(result.Error, "R2 不包含附件文件") {
		t.Fatalf("database-only attachment merge result = %+v, want fail-closed rejection", result)
	}
}

func TestLoadDisasterSnapshotsUsesFixedLitestreamCommand(t *testing.T) {
	binDir := t.TempDir()
	argsPath := filepath.Join(t.TempDir(), "args")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$@" > "$FAKE_LITESTREAM_ARGS"
printf 'replica generation index size created\n'
printf 'r2 generation-old 1 2048 2026-01-01T00:00:00Z\n'
printf 'r2 generation-new 2 4096 2026-02-01T00:00:00Z\n'
`
	if err := os.WriteFile(filepath.Join(binDir, "litestream"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_LITESTREAM_ARGS", argsPath)
	t.Setenv("DISASTER_SNAPSHOT_METADATA_PATH", "")

	snapshots, err := (&Server{dbPath: "/data/finarch.db"}).loadDisasterSnapshots(context.Background())
	if err != nil {
		t.Fatalf("loadDisasterSnapshots: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("snapshot count = %d, want 2", len(snapshots))
	}
	if snapshots[0].SnapshotID != "2026-02-01T00:00:00Z" || snapshots[0].DBSize != 4096 {
		t.Fatalf("newest snapshot = %#v", snapshots[0])
	}
	if snapshots[0].HasMetadata || snapshots[0].SchemaVersion != 0 {
		t.Fatalf("raw Litestream snapshot unexpectedly claimed application metadata: %#v", snapshots[0])
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := "snapshots\n-config\n/etc/litestream.yml\n-replica\nr2\n/data/finarch.db\n"
	if string(args) != wantArgs {
		t.Fatalf("litestream args = %q, want %q", args, wantArgs)
	}
}

func TestParseLitestreamSnapshotsRejectsMalformedOutput(t *testing.T) {
	for name, output := range map[string]string{
		"missing header": "",
		"wrong replica":  "replica generation index size created\nother gen 1 2 2026-01-01T00:00:00Z\n",
		"bad size":       "replica generation index size created\nr2 gen 1 unknown 2026-01-01T00:00:00Z\n",
		"bad timestamp":  "replica generation index size created\nr2 gen 1 2 yesterday\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseLitestreamSnapshots(output); err == nil {
				t.Fatal("malformed Litestream output unexpectedly parsed")
			}
		})
	}
}

func TestScopeAllowsMode(t *testing.T) {
	if !scopeAllowsMode("both", "work") || !scopeAllowsMode("both", "life") {
		t.Fatalf("both scope should allow all modes")
	}
	if !scopeAllowsMode("work", "work") || scopeAllowsMode("work", "life") {
		t.Fatalf("work scope mismatch")
	}
	if !scopeAllowsMode("life", "life") || scopeAllowsMode("life", "work") {
		t.Fatalf("life scope mismatch")
	}
	if scopeAllowsMode("wrok", "work") || scopeAllowsMode("unknown", "life") {
		t.Fatalf("unknown scope must not expand to both modes")
	}
}

func TestResolveRecoveredAccountName(t *testing.T) {
	existing := map[string]struct{}{
		"cash":             {},
		"cash (recovered)": {},
	}
	got := resolveRecoveredAccountName(existing, "Cash")
	if got != "Cash (Recovered 2)" {
		t.Fatalf("unexpected recovered name: %s", got)
	}
	if _, ok := existing["cash (recovered 2)"]; !ok {
		t.Fatalf("expected new name to be reserved")
	}
}

func TestRestoreFormFileAppliesBodyLimitBeforeMultipartParsing(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "backup.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(strings.Repeat("x", 64))); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/backup/restore", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	if _, err := restoreFormFile(c, 32); err == nil || !restoreBodyTooLarge(err) {
		t.Fatalf("expected MaxBytesReader rejection, got %v", err)
	}
}

func TestWriteGateSerializesRestoreBeforeTakingWriteLease(t *testing.T) {
	guard := findb.Global()
	previousState := guard.State()
	guard.SetState(findb.StateNormal)
	defer guard.SetState(previousState)

	s := &Server{}
	router := gin.New()
	router.Use(s.writeGateMiddleware())

	firstEntered := make(chan struct{})
	secondEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	requestsDone := make(chan struct{}, 2)
	calls := 0
	router.POST("/api/v1/backup/restore", func(c *gin.Context) {
		releaseRequestWriteLease(c.Request.Context())
		prior := guard.BeginMaintenance(findb.StateRestore)
		defer guard.EndMaintenance(prior)
		calls++
		if calls == 1 {
			close(firstEntered)
			<-releaseFirst
		} else {
			close(secondEntered)
		}
		c.Status(http.StatusNoContent)
	})

	request := func() {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/backup/restore", nil))
		if recorder.Code != http.StatusNoContent {
			t.Errorf("restore response status=%d, want %d", recorder.Code, http.StatusNoContent)
		}
		requestsDone <- struct{}{}
	}
	go request()
	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first restore did not enter maintenance")
	}
	go request()
	select {
	case <-secondEntered:
		t.Fatal("second restore entered before the first restore released maintenance")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)
	for i := 0; i < 2; i++ {
		select {
		case <-requestsDone:
		case <-time.After(time.Second):
			t.Fatal("concurrent restore requests deadlocked")
		}
	}
	select {
	case <-secondEntered:
	default:
		t.Fatal("second restore never entered")
	}
}

func TestExtractRestoreZipRejectsCompressionBomb(t *testing.T) {
	baseDir := t.TempDir()
	zipPath := filepath.Join(baseDir, "bomb.zip")
	writeTestRestoreZip(t, zipPath, map[string]string{
		"finarch.db": strings.Repeat("0", 1<<20),
	})

	if _, _, err := extractRestoreZip(zipPath, baseDir); err == nil || !strings.Contains(err.Error(), "compression ratio") {
		t.Fatalf("expected compression-ratio rejection, got %v", err)
	}
}

func TestExtractRestoreZipRejectsTooManyEntries(t *testing.T) {
	baseDir := t.TempDir()
	zipPath := filepath.Join(baseDir, "many.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for i := 0; i <= maxRestoreZipEntries; i++ {
		if _, err := zw.CreateHeader(&zip.FileHeader{Name: filepath.ToSlash(filepath.Join("empty", strings.Repeat("a", i%8), time.Unix(int64(i), 0).Format("150405.000000000")))}); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if _, _, err := extractRestoreZip(zipPath, baseDir); err == nil || !strings.Contains(err.Error(), "too many entries") {
		t.Fatalf("expected entry-count rejection, got %v", err)
	}
}

func TestPendingRestoreTokenIsConsumedOnceConcurrently(t *testing.T) {
	s := &Server{}
	pr := &pendingRestore{
		token:     "one-time-token",
		verified:  true,
		requester: "user-a",
		expiresAt: time.Now().Add(time.Minute),
	}
	if !s.storePendingRestore("restore-1", pr) {
		t.Fatal("failed to store restore session")
	}

	const workers = 20
	var wg sync.WaitGroup
	results := make(chan *pendingRestore, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, got := s.consumePendingRestoreToken("one-time-token", "user-a", time.Now())
			results <- got
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	for got := range results {
		if got != nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("one-time restore token consumed %d times, want 1", successes)
	}
}

func TestPendingRestoreExpiryRemovesTemporaryFile(t *testing.T) {
	tmpPath := filepath.Join(t.TempDir(), "pending.db")
	if err := os.WriteFile(tmpPath, []byte("temporary"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{}
	pr := &pendingRestore{tmpPath: tmpPath, expiresAt: time.Now().Add(20 * time.Millisecond)}
	if !s.storePendingRestore("restore-expiring", pr) {
		t.Fatal("failed to store restore session")
	}

	deadline := time.Now().Add(time.Second)
	for {
		_, statErr := os.Stat(tmpPath)
		if os.IsNotExist(statErr) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expired restore file was not removed: %v", statErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, ok := s.pendingRestores.Load("restore-expiring"); ok {
		t.Fatal("expired restore session remains in store")
	}
}

func TestPendingRestoreCapacityRejectsAndCleansNewSession(t *testing.T) {
	s := &Server{}
	for i := 0; i < maxPendingRestores; i++ {
		id := time.Now().Add(time.Duration(i) * time.Nanosecond).String()
		if !s.storePendingRestore(id, &pendingRestore{expiresAt: time.Now().Add(time.Minute)}) {
			t.Fatalf("session %d unexpectedly rejected", i)
		}
	}

	tmpPath := filepath.Join(t.TempDir(), "rejected.db")
	if err := os.WriteFile(tmpPath, []byte("temporary"), 0o600); err != nil {
		t.Fatal(err)
	}
	rejected := &pendingRestore{tmpPath: tmpPath, expiresAt: time.Now().Add(time.Minute)}
	if s.storePendingRestore("over-capacity", rejected) {
		t.Fatal("over-capacity restore session was accepted")
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("rejected restore session file was not cleaned: %v", err)
	}
}

func TestRestoreEngineBackupFailureDoesNotReplaceLiveDatabase(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	defer liveDB.Close()

	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "replacement", true)
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	notDirectory := filepath.Join(baseDir, "not-a-directory")
	if err := os.WriteFile(notDirectory, []byte("block safety directory creation"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: liveDB, dbPath: filepath.Join(notDirectory, "live.db")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.executeRestoreEngine(ctx, restoreEngineRequest{
		TempDBPath:      sourcePath,
		UploadedVersion: uploadedVersion,
		SchemaBefore:    restoreTestSchemaVersion(t, liveDB),
	})
	if err == nil || !strings.Contains(err.Error(), "创建恢复安全备份失败") {
		t.Fatalf("expected hard safety-backup failure, got %v", err)
	}
	assertRestoreTestMarker(t, liveDB, "original")
	if state := findb.Global().State(); state != findb.StateNormal {
		t.Fatalf("global guard state=%v, want normal", state)
	}
}

func TestRestoreEnginePostValidationFailureRollsBackAndCleansSafetyBackup(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	liveDB.SetMaxOpenConns(1)
	defer liveDB.Close()

	// This is a structurally valid SQLite/FinArch database, but it has no user.
	// The replacement succeeds and the post-restore identity check then fails.
	sourcePath := filepath.Join(baseDir, "source-without-owner.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "replacement", false)
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: liveDB, dbPath: livePath}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.executeRestoreEngine(ctx, restoreEngineRequest{
		TempDBPath:      sourcePath,
		UploadedVersion: uploadedVersion,
		SchemaBefore:    restoreTestSchemaVersion(t, liveDB),
	})
	if err == nil || !strings.Contains(err.Error(), "无用户数据") || !strings.Contains(err.Error(), "原数据库已自动恢复") {
		t.Fatalf("expected post-validation error with successful rollback, got %v", err)
	}
	assertRestoreTestMarker(t, liveDB, "original")
	if err := ensureSQLiteIntegrity(context.Background(), liveDB); err != nil {
		t.Fatalf("rolled-back live database failed integrity check: %v", err)
	}
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
	if state := findb.Global().State(); state != findb.StateNormal {
		t.Fatalf("global guard state=%v, want normal", state)
	}
}

func TestRestoreEngineIdentityFailureRollsBackLiveDatabase(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	liveDB.SetMaxOpenConns(1)
	defer liveDB.Close()

	sourcePath := filepath.Join(baseDir, "source-with-wrong-owner.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "replacement", true)
	if _, err := sourceDB.Exec(`
		CREATE TABLE backup_metadata (
			id INTEGER PRIMARY KEY,
			user_id TEXT NOT NULL,
			user_email TEXT NOT NULL,
			schema_version INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			app_version TEXT NOT NULL
		)
	`); err != nil {
		t.Fatal(err)
	}
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if _, err := sourceDB.Exec(`
		INSERT INTO backup_metadata (id, user_id, user_email, schema_version, created_at, app_version)
		VALUES (1, 'missing-owner', 'missing-owner@example.test', ?, '2026-09-04T00:00:00Z', 'test')
	`, uploadedVersion); err != nil {
		t.Fatal(err)
	}
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: liveDB, dbPath: livePath}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.executeRestoreEngine(ctx, restoreEngineRequest{
		TempDBPath:      sourcePath,
		UploadedVersion: uploadedVersion,
		SchemaBefore:    restoreTestSchemaVersion(t, liveDB),
	})
	if err == nil || !strings.Contains(err.Error(), "备份所有者不存在") || !strings.Contains(err.Error(), "原数据库已自动恢复") {
		t.Fatalf("expected identity-validation error with successful rollback, got %v", err)
	}
	assertRestoreTestMarker(t, liveDB, "original")
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
}

func TestRestoreEngineSuccessKeepsReplacementAndCleansSafetyBackup(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	liveDB.SetMaxOpenConns(1)
	defer liveDB.Close()

	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "replacement", true)
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: liveDB, dbPath: livePath}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := s.executeRestoreEngine(ctx, restoreEngineRequest{
		TempDBPath:      sourcePath,
		UploadedVersion: uploadedVersion,
		SchemaBefore:    restoreTestSchemaVersion(t, liveDB),
	})
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if result.SchemaAfter < uploadedVersion {
		t.Fatalf("schema after=%d, want at least %d", result.SchemaAfter, uploadedVersion)
	}
	if result.RestoreDuration <= 0 {
		t.Fatalf("restore duration was not returned: %s", result.RestoreDuration)
	}
	assertRestoreTestMarker(t, liveDB, "replacement")
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
	if state := findb.Global().State(); state != findb.StateNormal {
		t.Fatalf("global guard state=%v, want normal", state)
	}
}

func TestRestoreEngineInvalidatesRestoredSessionsAndPendingActions(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	liveDB.SetMaxOpenConns(1)
	defer liveDB.Close()

	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "replacement", true)
	ctx := context.Background()
	if _, err := sourceDB.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = 'restore-test-user'`); err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWTService("restore-credential-test-secret")
	sourceSessions, err := service.NewSessionService(
		sqliterepo.NewSQLiteRefreshTokenRepository(sourceDB), jwt, "restore-credential-test-secret",
	)
	if err != nil {
		t.Fatal(err)
	}
	oldSession, err := sourceSessions.CreateSession(ctx, model.User{
		ID: "restore-test-user", Email: "replacement@example.test", Role: "owner", EmailVerified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	actionTokens := auth.NewActionTokenService("restore-action-test-secret")
	oldActionToken, actionJTI, actionExpiry, err := actionTokens.Issue(
		"restore-test-user", service.ActionPasswordReset, "", time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sourceDB.ExecContext(ctx, `
		INSERT INTO action_requests (jti, user_id, action, status, meta, expires_at, created_at)
		VALUES (?, 'restore-test-user', ?, 'pending', '', ?, ?)`,
		actionJTI, service.ActionPasswordReset, actionExpiry.Unix(), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: liveDB, dbPath: livePath}
	result, err := s.executeRestoreEngine(ctx, restoreEngineRequest{
		TempDBPath:      sourcePath,
		UploadedVersion: uploadedVersion,
		SchemaBefore:    restoreTestSchemaVersion(t, liveDB),
	})
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if result.SchemaAfter < uploadedVersion {
		t.Fatalf("schema after=%d, want at least %d", result.SchemaAfter, uploadedVersion)
	}

	var revokedAt sql.NullInt64
	var revokeReason sql.NullString
	if err := liveDB.QueryRowContext(ctx, `SELECT revoked_at, revoke_reason FROM auth_sessions WHERE id = ?`, oldSession.SessionID).Scan(&revokedAt, &revokeReason); err != nil {
		t.Fatal(err)
	}
	if !revokedAt.Valid || !revokeReason.Valid || revokeReason.String != "database_restore" {
		t.Fatalf("restored session was not revoked: revoked_at=%v reason=%v", revokedAt, revokeReason)
	}
	var actionStatus string
	if err := liveDB.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE jti = ?`, actionJTI).Scan(&actionStatus); err != nil {
		t.Fatal(err)
	}
	if actionStatus != "expired" {
		t.Fatalf("restored pending action status=%q, want expired", actionStatus)
	}

	restoredSessions, err := service.NewSessionService(
		sqliterepo.NewSQLiteRefreshTokenRepository(liveDB), jwt, "restore-credential-test-secret",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restoredSessions.AuthenticateAccess(ctx, oldSession.AccessToken); !errors.Is(err, service.ErrSessionInvalid) {
		t.Fatalf("restored access token error=%v, want invalid session", err)
	}
	if _, err := restoredSessions.RotateSession(ctx, oldSession.RefreshToken); !errors.Is(err, service.ErrInvalidOrUsedToken) {
		t.Fatalf("restored refresh token error=%v, want invalid token", err)
	}
	restoredAuth := service.NewAuthService(
		sqliterepo.NewSQLiteUserRepository(liveDB),
		actionTokens,
		auth.NewLoginAttemptTracker(5, time.Minute),
		&email.NoopSender{},
		true,
		"http://localhost",
		sqliterepo.NewSQLiteTransactionManager(liveDB),
		restoredSessions,
	)
	if err := restoredAuth.ResetPassword(ctx, oldActionToken, "MustNotReplacePassword123"); !errors.Is(err, service.ErrExpiredToken) {
		t.Fatalf("restored action token error=%v, want expired token", err)
	}
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
}

func TestRestoreEngineCredentialInvalidationFailureRollsBackLiveDatabase(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	liveDB.SetMaxOpenConns(1)
	defer liveDB.Close()
	ctx := context.Background()
	now := time.Now().Unix()
	if _, err := liveDB.ExecContext(ctx, `
		INSERT INTO auth_sessions (id, user_id, pwd_version, absolute_expires_at, created_at, last_used_at)
		VALUES ('original-session', 'restore-test-user', 0, ?, ?, ?)`, now+3600, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDB.ExecContext(ctx, `
		INSERT INTO action_requests (jti, user_id, action, status, meta, expires_at, created_at)
		VALUES ('original-action', 'restore-test-user', ?, 'pending', '', ?, ?)`,
		service.ActionPasswordReset, now+3600, now); err != nil {
		t.Fatal(err)
	}

	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "replacement", true)
	if _, err := sourceDB.ExecContext(ctx, `
		INSERT INTO action_requests (jti, user_id, action, status, meta, expires_at, created_at)
		VALUES ('replacement-action', 'restore-test-user', ?, 'pending', '', ?, ?)`,
		service.ActionPasswordReset, now+3600, now); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceDB.ExecContext(ctx, `
		CREATE TRIGGER reject_restored_action_expiry
		BEFORE UPDATE OF status ON action_requests
		WHEN OLD.status = 'pending'
		BEGIN
			SELECT RAISE(ABORT, 'injected restored action expiry failure');
		END`); err != nil {
		t.Fatal(err)
	}
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: liveDB, dbPath: livePath}
	_, err := s.executeRestoreEngine(ctx, restoreEngineRequest{
		TempDBPath:      sourcePath,
		UploadedVersion: uploadedVersion,
		SchemaBefore:    restoreTestSchemaVersion(t, liveDB),
	})
	if err == nil || !strings.Contains(err.Error(), "恢复后凭证失效处理失败") || !strings.Contains(err.Error(), "原数据库已自动恢复") {
		t.Fatalf("expected credential invalidation failure with rollback, got %v", err)
	}
	assertRestoreTestMarker(t, liveDB, "original")
	var revokedAt sql.NullInt64
	if err := liveDB.QueryRowContext(ctx, `SELECT revoked_at FROM auth_sessions WHERE id = 'original-session'`).Scan(&revokedAt); err != nil {
		t.Fatal(err)
	}
	if revokedAt.Valid {
		t.Fatalf("original session was revoked despite safety rollback: %v", revokedAt)
	}
	var status string
	if err := liveDB.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE jti = 'original-action'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("original action status after rollback=%q, want pending", status)
	}
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
}

func TestRestoreEngineRawDatabaseRejectsMissingAttachmentBeforeReplacement(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	liveDB.SetMaxOpenConns(1)
	defer liveDB.Close()

	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "replacement", true)
	insertRestoreTestAttachment(t, sourceDB, "missing-attachment", "missing.bin", []byte("missing bytes"), 0)
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	storage := &restoreJournalStorage{files: map[string][]byte{}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	s := &Server{db: liveDB, dbPath: livePath, attachmentSvc: attachmentSvc}
	_, err := s.executeRestoreEngine(context.Background(), restoreEngineRequest{
		TempDBPath:      sourcePath,
		UploadedVersion: uploadedVersion,
		SchemaBefore:    restoreTestSchemaVersion(t, liveDB),
	})
	if err == nil || !strings.Contains(err.Error(), "备份附件校验失败") || !strings.Contains(err.Error(), "missing.bin") {
		t.Fatalf("expected raw restore attachment preflight failure, got %v", err)
	}
	assertRestoreTestMarker(t, liveDB, "original")
	entries, readErr := os.ReadDir(filepath.Join(baseDir, "safety_backups"))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("preflight failure created safety artifacts: %v", entries)
	}
	if state := findb.Global().State(); state != findb.StateNormal {
		t.Fatalf("global guard state=%v, want normal", state)
	}
}

func TestRestoreEngineRejectsInvalidAttachmentIntegrityMetadataBeforeReplacement(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	defer liveDB.Close()

	contents := []byte("matching bytes")
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "replacement", true)
	insertRestoreTestAttachment(t, sourceDB, "invalid-metadata", "matching.bin", contents, 0)
	if _, err := sourceDB.Exec(`UPDATE attachments SET sha256 = '' WHERE id = 'invalid-metadata'`); err != nil {
		t.Fatal(err)
	}
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	storage := &restoreJournalStorage{files: map[string][]byte{"matching.bin": contents}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	s := &Server{db: liveDB, dbPath: livePath, attachmentSvc: attachmentSvc}
	_, err := s.executeRestoreEngine(context.Background(), restoreEngineRequest{
		TempDBPath: sourcePath, UploadedVersion: uploadedVersion,
		SchemaBefore: restoreTestSchemaVersion(t, liveDB),
	})
	if err == nil || !strings.Contains(err.Error(), "备份附件校验失败") {
		t.Fatalf("invalid attachment metadata restore error = %v", err)
	}
	assertRestoreTestMarker(t, liveDB, "original")
	if _, statErr := os.Stat(filepath.Join(baseDir, "safety_backups")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("metadata preflight created safety artifacts: %v", statErr)
	}
}

func TestValidateAttachmentRestoreMetadataRejectsInvalidValues(t *testing.T) {
	validSHA := strings.Repeat("a", sha256.Size*2)
	cases := []attachmentRestoreFile{
		{SourceKey: "zero.bin", SizeBytes: 0, SHA256: validSHA},
		{SourceKey: "negative.bin", SizeBytes: -1, SHA256: validSHA},
		{SourceKey: "oversize.bin", SizeBytes: maxRestoreZipEntryBytes + 1, SHA256: validSHA},
		{SourceKey: "empty-sha.bin", SizeBytes: 1, SHA256: ""},
		{SourceKey: "short-sha.bin", SizeBytes: 1, SHA256: strings.Repeat("a", sha256.Size*2-1)},
		{SourceKey: "non-hex-sha.bin", SizeBytes: 1, SHA256: strings.Repeat("g", sha256.Size*2)},
	}
	for _, file := range cases {
		if err := validateAttachmentRestoreMetadata(file); err == nil {
			t.Fatalf("metadata %+v unexpectedly accepted", file)
		}
	}
	if err := validateAttachmentRestoreMetadata(attachmentRestoreFile{
		SourceKey: "valid.bin", SizeBytes: 1, SHA256: validSHA,
	}); err != nil {
		t.Fatalf("valid metadata rejected: %v", err)
	}
}

func TestRestoreEngineRejectsForeignKeyViolationBeforeReplacement(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	defer liveDB.Close()

	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "foreign-key-violation", true)
	if _, err := sourceDB.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceDB.Exec(`
		INSERT INTO accounts (
			id, user_id, name, type, currency, balance_cents, version,
			is_active, created_at, updated_at
		) VALUES ('orphan-account', 'missing-user', 'Orphan', 'personal',
		          'CNY', 0, 1, 1, '2026-09-05T00:00:00Z', '2026-09-05T00:00:00Z')
	`); err != nil {
		t.Fatal(err)
	}
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: liveDB, dbPath: livePath}
	_, err := s.executeRestoreEngine(context.Background(), restoreEngineRequest{
		TempDBPath: sourcePath, UploadedVersion: uploadedVersion,
		SchemaBefore: restoreTestSchemaVersion(t, liveDB),
	})
	if err == nil || !strings.Contains(err.Error(), "foreign_key_check") {
		t.Fatalf("foreign-key-invalid backup error = %v", err)
	}
	assertRestoreTestMarker(t, liveDB, "original")
	if _, statErr := os.Stat(filepath.Join(baseDir, "safety_backups")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("foreign key preflight created safety artifacts: %v", statErr)
	}
}

func TestCrossAccountMergeRejectsForeignKeyViolationBeforeImport(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	sourceUserID := "cross-invalid-fk-source"
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
	insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)
	if _, err := sourceDB.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceDB.Exec(`
		INSERT INTO accounts (
			id, user_id, name, type, currency, balance_cents, version,
			is_active, created_at, updated_at
		) VALUES ('orphan-source-account', 'missing-source-user', 'Orphan', 'personal',
		          'CNY', 0, 1, 1, '2026-09-05T00:00:00Z', '2026-09-05T00:00:00Z')
	`); err != nil {
		t.Fatal(err)
	}
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: liveDB, dbPath: livePath}
	_, err := s.performCrossAccountMergeRestore(
		context.Background(), sourcePath, uploadedVersion, restoreTestSchemaVersion(t, liveDB),
		restoreContext{
			RequesterUserID: "restore-test-user",
			BackupUserID:    sourceUserID,
			RestoreMode:     "CROSS_ACCOUNT",
			DataScope:       "both",
		},
	)
	if err == nil || !strings.Contains(err.Error(), "foreign_key_check") {
		t.Fatalf("foreign-key-invalid cross-account backup error = %v", err)
	}
	assertRestoreTestMarker(t, liveDB, "target")
	if state := findb.Global().State(); state != findb.StateNormal {
		t.Fatalf("global guard state=%v, want normal", state)
	}
}

func TestCrossAccountMergeRejectsBrokenProjectsSchema(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	sourceUserID := "cross-broken-project-source"
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
	insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)
	if _, err := sourceDB.Exec(`
		INSERT INTO projects (id, name, code, created_at, version)
		VALUES ('source-project', 'Source Project', 'SOURCE-PROJECT', 123456789, 1)
	`); err != nil {
		t.Fatal(err)
	}
	// Keep SQLite structurally sound while making the application schema
	// incompatible with the authoritative project query.
	if _, err := sourceDB.Exec(`ALTER TABLE projects RENAME COLUMN created_at TO broken_created_at`); err != nil {
		t.Fatal(err)
	}
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	s := newCrossAccountRestoreTestServer(t, liveDB, livePath, nil)
	_, err := s.performCrossAccountMergeRestore(
		context.Background(), sourcePath, uploadedVersion, restoreTestSchemaVersion(t, liveDB),
		restoreContext{
			RequesterUserID: "restore-test-user",
			BackupUserID:    sourceUserID,
			RestoreMode:     "CROSS_ACCOUNT",
			DataScope:       "both",
		},
	)
	if err == nil || !strings.Contains(err.Error(), "读取备份项目失败") {
		t.Fatalf("broken projects schema error = %v", err)
	}
	assertRestoreTestMarker(t, liveDB, "target")
	var imported int
	if err := liveDB.QueryRow(`SELECT COUNT(*) FROM projects WHERE code = 'SOURCE-PROJECT'`).Scan(&imported); err != nil {
		t.Fatal(err)
	}
	if imported != 0 {
		t.Fatalf("broken project schema committed %d source project(s)", imported)
	}
	if state := findb.Global().State(); state != findb.StateNormal {
		t.Fatalf("global guard state=%v, want normal", state)
	}
}

func TestCrossAccountMergePreservesProjectCreatedAt(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	sourceUserID := "cross-project-time-source"
	targetUserID := "restore-test-user"
	sourceProjectID := "source-project-time"
	const sourceCreatedAt int64 = 123456789
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
	insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)
	if _, err := sourceDB.Exec(`
		INSERT INTO projects (id, name, code, created_at, version)
		VALUES (?, 'Historic Project', 'HISTORIC-PROJECT', ?, 1)
	`, sourceProjectID, sourceCreatedAt); err != nil {
		t.Fatal(err)
	}
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	s := newCrossAccountRestoreTestServer(t, liveDB, livePath, nil)
	if _, err := s.performCrossAccountMergeRestore(
		context.Background(), sourcePath, uploadedVersion, restoreTestSchemaVersion(t, liveDB),
		restoreContext{
			RequesterUserID: targetUserID,
			BackupUserID:    sourceUserID,
			RestoreMode:     "CROSS_ACCOUNT",
			DataScope:       "both",
		},
	); err != nil {
		t.Fatalf("cross-account project restore failed: %v", err)
	}

	var restoredCreatedAt int64
	if err := liveDB.QueryRow(
		`SELECT created_at FROM projects WHERE id = ?`,
		deterministicRestoreID("project", sourceUserID, targetUserID, sourceProjectID),
	).Scan(&restoredCreatedAt); err != nil {
		t.Fatal(err)
	}
	if restoredCreatedAt != sourceCreatedAt {
		t.Fatalf("restored project created_at=%d, want %d", restoredCreatedAt, sourceCreatedAt)
	}
}

func TestRestoreEngineRawDatabaseReconcilesOldAndRestoredAttachmentDeletions(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	liveDB.SetMaxOpenConns(1)
	defer liveDB.Close()
	oldBytes := []byte("old-only bytes")
	restoredBytes := []byte("restored bytes")
	insertRestoreTestAttachment(t, liveDB, "old-only", "old-only.bin", oldBytes, 0)

	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "replacement", true)
	insertRestoreTestAttachment(t, sourceDB, "restored", "restored.bin", restoredBytes, 0)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := sourceDB.Exec(`
		INSERT INTO attachment_deletion_queue(storage_key, user_id, attempts, created_at, updated_at)
		VALUES ('restored.bin', 'restore-test-user', 1, ?, ?)
	`, now, now); err != nil {
		t.Fatal(err)
	}
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	storage := &restoreJournalStorage{requireRestoreMaintenanceOnOpen: true, files: map[string][]byte{
		"old-only.bin": oldBytes,
		"restored.bin": restoredBytes,
	}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	s := &Server{db: liveDB, dbPath: livePath, attachmentSvc: attachmentSvc}
	if _, err := s.executeRestoreEngine(context.Background(), restoreEngineRequest{
		TempDBPath:      sourcePath,
		UploadedVersion: uploadedVersion,
		SchemaBefore:    restoreTestSchemaVersion(t, liveDB),
	}); err != nil {
		t.Fatalf("raw database restore failed: %v", err)
	}
	assertRestoreTestMarker(t, liveDB, "replacement")

	var oldQueued, restoredQueued int
	if err := liveDB.QueryRow(`SELECT COUNT(*) FROM attachment_deletion_queue WHERE storage_key = 'old-only.bin' AND user_id = 'restore-test-user'`).Scan(&oldQueued); err != nil {
		t.Fatal(err)
	}
	if err := liveDB.QueryRow(`SELECT COUNT(*) FROM attachment_deletion_queue WHERE storage_key = 'restored.bin'`).Scan(&restoredQueued); err != nil {
		t.Fatal(err)
	}
	if oldQueued != 1 || restoredQueued != 0 {
		t.Fatalf("replacement deletion reconciliation = old:%d restored:%d, want 1/0", oldQueued, restoredQueued)
	}
	storage.mu.Lock()
	openedOutsideMaintenance := storage.openedOutsideRestoreMaintenance
	storage.mu.Unlock()
	if openedOutsideMaintenance {
		t.Fatal("raw attachment preflight ran outside restore maintenance")
	}
	assertRestoreStorageFile(t, storage, "old-only.bin", oldBytes, true)
	assertRestoreStorageFile(t, storage, "restored.bin", restoredBytes, true)
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
}

func TestRestoreEngineAttachmentFailureCompensatesFilesAfterRequestCancellation(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	liveDB.SetMaxOpenConns(1)
	defer liveDB.Close()
	insertRestoreTestAttachment(t, liveDB, "original-attachment", "shared.bin", []byte("old shared bytes"), 0)

	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "replacement", true)
	sourceDir := filepath.Join(baseDir, "source-attachments")
	insertRestoreTestAttachment(t, sourceDB, "source-shared", "shared.bin", []byte("replacement shared bytes"), 0)
	writeRestoreTestAttachment(t, sourceDir, "shared.bin", []byte("replacement shared bytes"))
	insertRestoreTestAttachment(t, sourceDB, "source-new", "new.bin", []byte("new bytes"), 1)
	writeRestoreTestAttachment(t, sourceDir, "new.bin", []byte("new bytes"))
	insertRestoreTestAttachment(t, sourceDB, "source-fail", "fail.bin", []byte("failure bytes"), 2)
	writeRestoreTestAttachment(t, sourceDir, "fail.bin", []byte("failure bytes"))
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	storage := &restoreJournalStorage{
		files:              map[string][]byte{"shared.bin": []byte("old shared bytes")},
		failRestoreKey:     "fail.bin",
		cancelOnRestoreErr: cancelRequest,
	}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	s := &Server{db: liveDB, dbPath: livePath, attachmentSvc: attachmentSvc}
	result, err := s.executeRestoreEngine(requestCtx, restoreEngineRequest{
		RestoreID:            "attachment-rollback",
		TempDBPath:           sourcePath,
		UploadedVersion:      uploadedVersion,
		SchemaBefore:         restoreTestSchemaVersion(t, liveDB),
		AttachmentRestoreDir: sourceDir,
	})
	if err == nil || !strings.Contains(err.Error(), "injected attachment restore failure") || !strings.Contains(err.Error(), "原数据库已自动恢复") {
		t.Fatalf("expected attachment failure with successful compensation, got %v", err)
	}
	if result.RestoreDuration <= 0 {
		t.Fatalf("failed restore duration was not returned: %s", result.RestoreDuration)
	}
	assertRestoreTestMarker(t, liveDB, "original")
	assertRestoreStorageFile(t, storage, "shared.bin", []byte("old shared bytes"), true)
	assertRestoreStorageFile(t, storage, "new.bin", nil, false)
	assertRestoreStorageFile(t, storage, "fail.bin", nil, false)
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
	if state := findb.Global().State(); state != findb.StateNormal {
		t.Fatalf("global guard state=%v, want normal", state)
	}
}

func TestRestoreEngineAttachmentPanicCompensatesDatabaseAndFiles(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	defer liveDB.Close()
	insertRestoreTestAttachment(t, liveDB, "original-attachment", "shared.bin", []byte("old shared bytes"), 0)

	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "replacement", true)
	sourceDir := filepath.Join(baseDir, "source-attachments")
	insertRestoreTestAttachment(t, sourceDB, "source-shared", "shared.bin", []byte("replacement shared bytes"), 0)
	writeRestoreTestAttachment(t, sourceDir, "shared.bin", []byte("replacement shared bytes"))
	insertRestoreTestAttachment(t, sourceDB, "source-panic", "panic.bin", []byte("panic bytes"), 1)
	writeRestoreTestAttachment(t, sourceDir, "panic.bin", []byte("panic bytes"))
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	storage := &restoreJournalStorage{
		files:           map[string][]byte{"shared.bin": []byte("old shared bytes")},
		panicRestoreKey: "panic.bin",
	}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	s := &Server{db: liveDB, dbPath: livePath, attachmentSvc: attachmentSvc}
	guard := findb.Global()
	previousState := guard.State()
	guard.SetState(findb.StateNormal)
	defer guard.SetState(previousState)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = s.executeRestoreEngine(context.Background(), restoreEngineRequest{
			RestoreID:            "panic-compensation",
			TempDBPath:           sourcePath,
			UploadedVersion:      uploadedVersion,
			SchemaBefore:         restoreTestSchemaVersion(t, liveDB),
			AttachmentRestoreDir: sourceDir,
		})
	}()
	if recovered == nil {
		t.Fatal("restore storage panic was not propagated")
	}
	assertRestoreTestMarker(t, liveDB, "original")
	assertRestoreStorageFile(t, storage, "shared.bin", []byte("old shared bytes"), true)
	assertRestoreStorageFile(t, storage, "panic.bin", nil, false)
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
	if state := guard.State(); state != findb.StateNormal {
		t.Fatalf("global guard state=%v, want normal after successful panic compensation", state)
	}
}

func TestPrepareAttachmentRollbackJournalCleansPartialJournalOnPanic(t *testing.T) {
	root := filepath.Join(t.TempDir(), "safety_backups")
	storage := &restoreJournalStorage{
		files:        map[string][]byte{"first.bin": []byte("first"), "panic.bin": []byte("panic")},
		panicOpenKey: "panic.bin",
	}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = prepareAttachmentRollbackJournal(context.Background(), root, []attachmentRestoreFile{
			{TargetKey: "first.bin"},
			{TargetKey: "panic.bin"},
		}, attachmentSvc)
	}()
	if recovered == nil {
		t.Fatal("journal storage panic was not propagated")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial attachment journals leaked after panic: %v", entries)
	}
}

func TestRestoreEngineAttachmentRollbackFailureRetainsSafetyArtifactAndMaintenanceState(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	liveDB.SetMaxOpenConns(1)
	defer liveDB.Close()
	insertRestoreTestAttachment(t, liveDB, "original-attachment", "shared.bin", []byte("old shared bytes"), 0)

	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "replacement", true)
	sourceDir := filepath.Join(baseDir, "source-attachments")
	insertRestoreTestAttachment(t, sourceDB, "source-shared", "shared.bin", []byte("replacement shared bytes"), 0)
	writeRestoreTestAttachment(t, sourceDir, "shared.bin", []byte("replacement shared bytes"))
	insertRestoreTestAttachment(t, sourceDB, "source-new", "new.bin", []byte("new bytes"), 1)
	writeRestoreTestAttachment(t, sourceDir, "new.bin", []byte("new bytes"))
	insertRestoreTestAttachment(t, sourceDB, "source-fail", "fail.bin", []byte("failure bytes"), 2)
	writeRestoreTestAttachment(t, sourceDir, "fail.bin", []byte("failure bytes"))
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	storage := &restoreJournalStorage{
		files:          map[string][]byte{"shared.bin": []byte("old shared bytes")},
		failRestoreKey: "fail.bin",
		failDeleteKey:  "new.bin",
	}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	s := &Server{db: liveDB, dbPath: livePath, attachmentSvc: attachmentSvc}
	guard := findb.Global()
	previousState := guard.State()
	defer guard.SetState(previousState)
	result, err := s.executeRestoreEngine(context.Background(), restoreEngineRequest{
		RestoreID:            "retain-test",
		TempDBPath:           sourcePath,
		UploadedVersion:      uploadedVersion,
		SchemaBefore:         restoreTestSchemaVersion(t, liveDB),
		AttachmentRestoreDir: sourceDir,
	})
	if err == nil || !strings.Contains(err.Error(), "附件回滚错误") || !strings.Contains(err.Error(), "需人工恢复") || !strings.Contains(err.Error(), "附件回滚日志已保留") {
		t.Fatalf("expected typed attachment rollback failure, got %v", err)
	}
	if strings.Contains(err.Error(), baseDir) {
		t.Fatalf("rollback error leaked an absolute artifact path: %v", err)
	}
	if result.RestoreDuration <= 0 {
		t.Fatalf("failed restore duration was not returned: %s", result.RestoreDuration)
	}
	assertRestoreTestMarker(t, liveDB, "original")
	assertRestoreStorageFile(t, storage, "shared.bin", []byte("old shared bytes"), true)
	if state := guard.State(); state != findb.StateRestore {
		t.Fatalf("global guard state=%v, want restore maintenance state", state)
	}

	safetyDir := filepath.Join(baseDir, "safety_backups")
	entries, readErr := os.ReadDir(safetyDir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 2 {
		t.Fatalf("retained safety directory entries=%d, want database and attachment artifacts: %v", len(entries), entries)
	}
	var retainedDBEntry, retainedAttachmentEntry os.DirEntry
	for _, entry := range entries {
		switch {
		case strings.HasPrefix(entry.Name(), restoreSafetyFilePrefix) && strings.HasSuffix(entry.Name(), ".db"):
			retainedDBEntry = entry
		case strings.HasPrefix(entry.Name(), "failed-restore-attachments-retain-test-"):
			retainedAttachmentEntry = entry
		default:
			t.Fatalf("unexpected transient or sidecar artifact %q", entry.Name())
		}
	}
	if retainedDBEntry == nil || retainedAttachmentEntry == nil {
		t.Fatalf("missing retained artifacts: %v", entries)
	}
	info, statErr := retainedDBEntry.Info()
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("retained safety artifact permissions=%#o, want 0600", info.Mode().Perm())
	}
	retainedPath := filepath.Join(safetyDir, retainedDBEntry.Name())
	retainedDB, openErr := openExistingSQLiteReadOnly(context.Background(), retainedPath)
	if openErr != nil {
		t.Fatalf("open retained safety artifact: %v", openErr)
	}
	defer retainedDB.Close()
	if err := ensureSQLiteIntegrity(context.Background(), retainedDB); err != nil {
		t.Fatalf("retained safety artifact integrity: %v", err)
	}
	assertRestoreTestMarker(t, retainedDB, "original")
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, statErr := os.Stat(retainedPath + suffix); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("retained safety sidecar %s was not cleaned: %v", suffix, statErr)
		}
	}

	attachmentInfo, statErr := retainedAttachmentEntry.Info()
	if statErr != nil {
		t.Fatal(statErr)
	}
	if !attachmentInfo.IsDir() || attachmentInfo.Mode().Perm() != 0o700 {
		t.Fatalf("retained attachment artifact mode=%v, want directory 0700", attachmentInfo.Mode())
	}
	attachmentArtifactPath := filepath.Join(safetyDir, retainedAttachmentEntry.Name())
	journalEntries, readErr := os.ReadDir(attachmentArtifactPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	manifestFound := false
	for _, entry := range journalEntries {
		entryInfo, statErr := entry.Info()
		if statErr != nil {
			t.Fatal(statErr)
		}
		if !entryInfo.Mode().IsRegular() || entryInfo.Mode().Perm() != 0o600 {
			t.Fatalf("retained journal entry %q mode=%v, want regular 0600", entry.Name(), entryInfo.Mode())
		}
		manifestFound = manifestFound || entry.Name() == "manifest.json"
	}
	if !manifestFound {
		t.Fatal("retained attachment journal is missing manifest.json")
	}
	marker, markerErr := readRestoreOperationMarker(filepath.Join(attachmentArtifactPath, restoreOperationJournalFile))
	if markerErr != nil {
		t.Fatalf("decode retained restore marker: %v", markerErr)
	}
	if marker.SafetyBackup != retainedDBEntry.Name() {
		t.Fatalf("retained marker safety backup=%q, want %q", marker.SafetyBackup, retainedDBEntry.Name())
	}
	manifestBytes, readErr := os.ReadFile(filepath.Join(attachmentArtifactPath, "manifest.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	var manifest attachmentRollbackManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode retained attachment manifest: %v", err)
	}
	if manifest.Version != rollbackManifestVersion || len(manifest.Entries) != 3 {
		t.Fatalf("unexpected retained attachment manifest: %+v", manifest)
	}
	if manifest.Entries[0].SizeBytes != int64(len("old shared bytes")) || !validRollbackSHA256(manifest.Entries[0].SHA256) {
		t.Fatalf("retained attachment manifest is missing integrity metadata: %+v", manifest.Entries[0])
	}
}

func TestCrossAccountMergeAttachmentFailureCompensatesFilesAndDatabaseAfterCancellation(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	sourceUserID := "cross-restore-source"
	targetUserID := "restore-test-user"
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
	insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)
	sourceDir := filepath.Join(baseDir, "source-attachments")
	insertCrossAccountRestoreAttachment(t, sourceDB, sourceUserID, "source-existing", "existing.bin", []byte("replacement existing bytes"), 0)
	writeRestoreTestAttachment(t, sourceDir, "existing.bin", []byte("replacement existing bytes"))
	insertCrossAccountRestoreAttachment(t, sourceDB, sourceUserID, "source-fail", "fail.bin", []byte("failure bytes"), 1)
	writeRestoreTestAttachment(t, sourceDir, "fail.bin", []byte("failure bytes"))
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	existingTargetKey := crossAccountAttachmentTargetKey(sourceUserID, targetUserID, "source-existing", ".bin")
	failingTargetKey := crossAccountAttachmentTargetKey(sourceUserID, targetUserID, "source-fail", ".bin")
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	storage := &restoreJournalStorage{
		files:              map[string][]byte{existingTargetKey: []byte("original existing bytes")},
		failRestoreKey:     failingTargetKey,
		cancelOnRestoreErr: cancelRequest,
	}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	s := &Server{db: liveDB, dbPath: livePath, attachmentSvc: attachmentSvc}
	guard := findb.Global()
	previousState := guard.State()
	defer guard.SetState(previousState)

	_, err := s.performCrossAccountMergeRestore(requestCtx, sourcePath, uploadedVersion, restoreTestSchemaVersion(t, liveDB), restoreContext{
		RequesterUserID:      targetUserID,
		BackupUserID:         sourceUserID,
		RestoreMode:          "CROSS_ACCOUNT",
		DataScope:            "both",
		AttachmentRestoreDir: sourceDir,
	})
	if err == nil || !strings.Contains(err.Error(), "injected attachment restore failure") {
		t.Fatalf("expected injected cross-account attachment failure, got %v", err)
	}
	var targetAttachmentCount int
	if queryErr := liveDB.QueryRow(`SELECT COUNT(1) FROM attachments WHERE user_id = ?`, targetUserID).Scan(&targetAttachmentCount); queryErr != nil {
		t.Fatal(queryErr)
	}
	if targetAttachmentCount != 0 {
		t.Fatalf("cross-account database transaction retained %d attachments after failure", targetAttachmentCount)
	}
	assertRestoreStorageFile(t, storage, existingTargetKey, []byte("original existing bytes"), true)
	assertRestoreStorageFile(t, storage, failingTargetKey, nil, false)
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
	if state := guard.State(); state != previousState {
		t.Fatalf("global guard state=%v, want previous state %v", state, previousState)
	}
}

func TestCrossAccountMergeAttachmentPanicBeforeCommitCompensates(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	sourceUserID := "cross-panic-source"
	targetUserID := "restore-test-user"
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
	insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)
	sourceDir := filepath.Join(baseDir, "source-attachments")
	insertCrossAccountRestoreAttachment(t, sourceDB, sourceUserID, "source-first", "first.bin", []byte("first bytes"), 0)
	writeRestoreTestAttachment(t, sourceDir, "first.bin", []byte("first bytes"))
	insertCrossAccountRestoreAttachment(t, sourceDB, sourceUserID, "source-panic", "panic.bin", []byte("panic bytes"), 1)
	writeRestoreTestAttachment(t, sourceDir, "panic.bin", []byte("panic bytes"))
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	firstTargetKey := crossAccountAttachmentTargetKey(sourceUserID, targetUserID, "source-first", ".bin")
	panicTargetKey := crossAccountAttachmentTargetKey(sourceUserID, targetUserID, "source-panic", ".bin")
	storage := &restoreJournalStorage{files: map[string][]byte{}, panicRestoreKey: panicTargetKey}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	s := &Server{db: liveDB, dbPath: livePath, attachmentSvc: attachmentSvc}
	guard := findb.Global()
	previousState := guard.State()
	guard.SetState(findb.StateNormal)
	defer guard.SetState(previousState)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = s.performCrossAccountMergeRestore(context.Background(), sourcePath, uploadedVersion, restoreTestSchemaVersion(t, liveDB), restoreContext{
			RequesterUserID:      targetUserID,
			BackupUserID:         sourceUserID,
			RestoreMode:          "CROSS_ACCOUNT",
			DataScope:            "both",
			AttachmentRestoreDir: sourceDir,
		})
	}()
	if recovered == nil {
		t.Fatal("cross-account storage panic was not propagated")
	}
	var targetAttachmentCount int
	if err := liveDB.QueryRow(`SELECT COUNT(1) FROM attachments WHERE user_id = ?`, targetUserID).Scan(&targetAttachmentCount); err != nil {
		t.Fatal(err)
	}
	if targetAttachmentCount != 0 {
		t.Fatalf("cross-account transaction retained %d attachments after panic", targetAttachmentCount)
	}
	assertRestoreStorageFile(t, storage, firstTargetKey, nil, false)
	assertRestoreStorageFile(t, storage, panicTargetKey, nil, false)
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
	if state := guard.State(); state != findb.StateNormal {
		t.Fatalf("global guard state=%v, want normal after compensated panic", state)
	}
}

func TestCrossAccountMergePanicAfterCommitRetainsJournalAndMaintenance(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	sourceUserID := "cross-committed-panic-source"
	targetUserID := "restore-test-user"
	sourceAttachmentID := "source-attachment"
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
	insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)
	sourceDir := filepath.Join(baseDir, "source-attachments")
	insertCrossAccountRestoreAttachment(t, sourceDB, sourceUserID, sourceAttachmentID, "committed.bin", []byte("committed bytes"), 0)
	writeRestoreTestAttachment(t, sourceDir, "committed.bin", []byte("committed bytes"))
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	targetKey := crossAccountAttachmentTargetKey(sourceUserID, targetUserID, sourceAttachmentID, ".bin")
	storage := &restoreJournalStorage{files: map[string][]byte{}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	// A nil account service deliberately panics at the first post-commit hook.
	s := &Server{db: liveDB, dbPath: livePath, attachmentSvc: attachmentSvc}
	guard := findb.Global()
	previousState := guard.State()
	guard.SetState(findb.StateNormal)
	defer guard.SetState(previousState)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = s.performCrossAccountMergeRestore(context.Background(), sourcePath, uploadedVersion, restoreTestSchemaVersion(t, liveDB), restoreContext{
			RequesterUserID:      targetUserID,
			BackupUserID:         sourceUserID,
			RestoreMode:          "CROSS_ACCOUNT",
			DataScope:            "both",
			AttachmentRestoreDir: sourceDir,
		})
	}()
	if recovered == nil {
		t.Fatal("post-commit panic was not propagated")
	}
	var targetAttachmentCount int
	if err := liveDB.QueryRow(`SELECT COUNT(1) FROM attachments WHERE user_id = ?`, targetUserID).Scan(&targetAttachmentCount); err != nil {
		t.Fatal(err)
	}
	if targetAttachmentCount != 1 {
		t.Fatalf("committed database change count=%d, want 1 retained for manual recovery", targetAttachmentCount)
	}
	assertRestoreStorageFile(t, storage, targetKey, []byte("committed bytes"), true)
	if state := guard.State(); state != findb.StateRestore {
		t.Fatalf("global guard state=%v, want restore maintenance after committed panic", state)
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "safety_backups"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].IsDir() || !strings.HasPrefix(entries[0].Name(), "failed-restore-attachments-") {
		t.Fatalf("committed panic did not retain exactly one attachment journal: %v", entries)
	}
}

func TestCrossAccountMergeCommitErrorRetainsJournalAndMaintenance(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	sourceUserID := "cross-uncertain-commit-source"
	targetUserID := "restore-test-user"
	sourceAttachmentID := "source-attachment"
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
	insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)
	sourceDir := filepath.Join(baseDir, "source-attachments")
	insertCrossAccountRestoreAttachment(t, sourceDB, sourceUserID, sourceAttachmentID, "uncertain.bin", []byte("uncertain bytes"), 0)
	writeRestoreTestAttachment(t, sourceDir, "uncertain.bin", []byte("uncertain bytes"))
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	targetKey := crossAccountAttachmentTargetKey(sourceUserID, targetUserID, sourceAttachmentID, ".bin")
	storage := &restoreJournalStorage{files: map[string][]byte{}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	s := &Server{
		db:            liveDB,
		dbPath:        livePath,
		attachmentSvc: attachmentSvc,
		crossRestoreCommit: func(tx *sql.Tx) error {
			if err := tx.Commit(); err != nil {
				return err
			}
			return errors.New("injected uncertain commit result")
		},
	}
	guard := findb.Global()
	previousState := guard.State()
	guard.SetState(findb.StateNormal)
	defer guard.SetState(previousState)

	_, err := s.performCrossAccountMergeRestore(context.Background(), sourcePath, uploadedVersion, restoreTestSchemaVersion(t, liveDB), restoreContext{
		RequesterUserID:      targetUserID,
		BackupUserID:         sourceUserID,
		RestoreMode:          "CROSS_ACCOUNT",
		DataScope:            "both",
		AttachmentRestoreDir: sourceDir,
	})
	if err == nil || !strings.Contains(err.Error(), "injected uncertain commit result") {
		t.Fatalf("expected injected commit error, got %v", err)
	}
	var rollbackFailure *restoreRollbackFailure
	if !errors.As(err, &rollbackFailure) {
		t.Fatalf("commit uncertainty was not surfaced as manual-recovery failure: %T %v", err, err)
	}
	var targetAttachmentCount int
	if err := liveDB.QueryRow(`SELECT COUNT(1) FROM attachments WHERE user_id = ?`, targetUserID).Scan(&targetAttachmentCount); err != nil {
		t.Fatal(err)
	}
	if targetAttachmentCount != 1 {
		t.Fatalf("committed database change count=%d, want 1 retained for manual recovery", targetAttachmentCount)
	}
	assertRestoreStorageFile(t, storage, targetKey, []byte("uncertain bytes"), true)
	if state := guard.State(); state != findb.StateRestore {
		t.Fatalf("global guard state=%v, want restore maintenance after uncertain commit", state)
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "safety_backups"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].IsDir() || !strings.HasPrefix(entries[0].Name(), "failed-restore-attachments-") {
		t.Fatalf("uncertain commit did not retain exactly one attachment journal: %v", entries)
	}
}

func TestCrossAccountMergeAttachmentRollbackFailureRetainsJournalAndMaintenanceState(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	sourceUserID := "cross-restore-source"
	targetUserID := "restore-test-user"
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
	insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)
	sourceDir := filepath.Join(baseDir, "source-attachments")
	insertCrossAccountRestoreAttachment(t, sourceDB, sourceUserID, "source-existing", "existing.bin", []byte("replacement existing bytes"), 0)
	writeRestoreTestAttachment(t, sourceDir, "existing.bin", []byte("replacement existing bytes"))
	insertCrossAccountRestoreAttachment(t, sourceDB, sourceUserID, "source-fail", "fail.bin", []byte("failure bytes"), 1)
	writeRestoreTestAttachment(t, sourceDir, "fail.bin", []byte("failure bytes"))
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	existingTargetKey := crossAccountAttachmentTargetKey(sourceUserID, targetUserID, "source-existing", ".bin")
	failingTargetKey := crossAccountAttachmentTargetKey(sourceUserID, targetUserID, "source-fail", ".bin")
	storage := &restoreJournalStorage{
		files:          map[string][]byte{existingTargetKey: []byte("original existing bytes")},
		failRestoreKey: failingTargetKey,
		failDeleteKey:  failingTargetKey,
	}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	s := &Server{db: liveDB, dbPath: livePath, attachmentSvc: attachmentSvc}
	guard := findb.Global()
	previousState := guard.State()
	defer guard.SetState(previousState)

	_, err := s.performCrossAccountMergeRestore(context.Background(), sourcePath, uploadedVersion, restoreTestSchemaVersion(t, liveDB), restoreContext{
		RequesterUserID:      targetUserID,
		BackupUserID:         sourceUserID,
		RestoreMode:          "CROSS_ACCOUNT",
		DataScope:            "both",
		AttachmentRestoreDir: sourceDir,
	})
	var rollbackFailure *restoreRollbackFailure
	if err == nil || !errors.As(err, &rollbackFailure) || !strings.Contains(err.Error(), "附件回滚日志已保留") {
		t.Fatalf("expected typed cross-account rollback failure with retained journal, got %v", err)
	}
	if strings.Contains(err.Error(), baseDir) {
		t.Fatalf("cross-account rollback error leaked an absolute path: %v", err)
	}
	var targetAttachmentCount int
	if queryErr := liveDB.QueryRow(`SELECT COUNT(1) FROM attachments WHERE user_id = ?`, targetUserID).Scan(&targetAttachmentCount); queryErr != nil {
		t.Fatal(queryErr)
	}
	if targetAttachmentCount != 0 {
		t.Fatalf("cross-account database transaction retained %d attachments after rollback failure", targetAttachmentCount)
	}
	assertRestoreStorageFile(t, storage, existingTargetKey, []byte("original existing bytes"), true)
	if state := guard.State(); state != findb.StateRestore {
		t.Fatalf("global guard state=%v, want restore maintenance state", state)
	}
	entries, readErr := os.ReadDir(filepath.Join(baseDir, "safety_backups"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 1 || !entries[0].IsDir() || !strings.HasPrefix(entries[0].Name(), "failed-restore-attachments-") {
		t.Fatalf("unexpected retained cross-account rollback artifacts: %v", entries)
	}
	manifestPath := filepath.Join(baseDir, "safety_backups", entries[0].Name(), "manifest.json")
	manifestInfo, statErr := os.Stat(manifestPath)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if manifestInfo.Mode().Perm() != 0o600 {
		t.Fatalf("retained cross-account manifest permissions=%#o, want 0600", manifestInfo.Mode().Perm())
	}
}

func TestCrossAccountMergeDoesNotOverwriteIgnoredAttachment(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	sourceUserID := "cross-restore-source"
	targetUserID := "restore-test-user"
	sourceAttachmentID := "source-existing"
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
	insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)
	sourceDir := filepath.Join(baseDir, "source-attachments")
	insertCrossAccountRestoreAttachment(t, sourceDB, sourceUserID, sourceAttachmentID, "existing.bin", []byte("replacement bytes"), 0)
	writeRestoreTestAttachment(t, sourceDir, "existing.bin", []byte("replacement bytes"))
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	targetID := deterministicRestoreID("attachment", sourceUserID, targetUserID, sourceAttachmentID)
	targetKey := crossAccountAttachmentTargetKey(sourceUserID, targetUserID, sourceAttachmentID, ".bin")
	insertCrossAccountRestoreAttachment(t, liveDB, targetUserID, targetID, targetKey, []byte("original bytes"), 0)
	storage := &restoreJournalStorage{
		files:          map[string][]byte{targetKey: []byte("original bytes")},
		failRestoreKey: targetKey,
	}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	txManager := sqliterepo.NewSQLiteTransactionManager(liveDB)
	accountSvc := service.NewAccountService(
		sqliterepo.NewSQLiteAccountRepository(liveDB),
		sqliterepo.NewSQLiteTransactionRepository(liveDB),
		txManager,
	)
	s := &Server{db: liveDB, dbPath: livePath, attachmentSvc: attachmentSvc, acctSvc: accountSvc}

	if _, err := s.performCrossAccountMergeRestore(context.Background(), sourcePath, uploadedVersion, restoreTestSchemaVersion(t, liveDB), restoreContext{
		RequesterUserID:      targetUserID,
		BackupUserID:         sourceUserID,
		RestoreMode:          "CROSS_ACCOUNT",
		DataScope:            "both",
		AttachmentRestoreDir: sourceDir,
	}); err != nil {
		t.Fatalf("idempotent cross-account merge failed: %v", err)
	}
	assertRestoreStorageFile(t, storage, targetKey, []byte("original bytes"), true)
	storage.mu.Lock()
	restoreAttempted := storage.failedRestore
	storage.mu.Unlock()
	if restoreAttempted {
		t.Fatal("INSERT OR IGNORE attachment was unexpectedly written to storage")
	}
}

func TestCrossAccountScopedMergeExcludesUnlinkedAttachments(t *testing.T) {
	for _, scope := range []string{"work", "life"} {
		t.Run(scope, func(t *testing.T) {
			baseDir := t.TempDir()
			livePath := filepath.Join(baseDir, "live.db")
			liveDB := openRestoreTestDatabase(t, livePath, "target", true)
			defer liveDB.Close()

			sourceUserID := "cross-scoped-orphan-" + scope
			targetUserID := "restore-test-user"
			sourceAttachmentID := "unlinked-" + scope
			contents := []byte("private unlinked " + scope + " attachment")
			sourcePath := filepath.Join(baseDir, "source.db")
			sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
			insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)
			sourceDir := filepath.Join(baseDir, "source-attachments")
			insertCrossAccountRestoreAttachment(t, sourceDB, sourceUserID, sourceAttachmentID, "unlinked.bin", contents, 0)
			writeRestoreTestAttachment(t, sourceDir, "unlinked.bin", contents)
			uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
			if err := sourceDB.Close(); err != nil {
				t.Fatal(err)
			}

			storage := &restoreJournalStorage{files: map[string][]byte{}}
			attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
			txManager := sqliterepo.NewSQLiteTransactionManager(liveDB)
			accountSvc := service.NewAccountService(
				sqliterepo.NewSQLiteAccountRepository(liveDB),
				sqliterepo.NewSQLiteTransactionRepository(liveDB),
				txManager,
			)
			s := &Server{db: liveDB, dbPath: livePath, attachmentSvc: attachmentSvc, acctSvc: accountSvc}

			if _, err := s.performCrossAccountMergeRestore(
				context.Background(), sourcePath, uploadedVersion, restoreTestSchemaVersion(t, liveDB),
				restoreContext{
					RequesterUserID:      targetUserID,
					BackupUserID:         sourceUserID,
					RestoreMode:          "CROSS_ACCOUNT",
					DataScope:            scope,
					AttachmentRestoreDir: sourceDir,
				},
			); err != nil {
				t.Fatalf("scoped cross-account merge failed: %v", err)
			}

			var imported int
			if err := liveDB.QueryRow(`SELECT COUNT(*) FROM attachments WHERE user_id = ?`, targetUserID).Scan(&imported); err != nil {
				t.Fatal(err)
			}
			if imported != 0 {
				t.Fatalf("scope %q imported %d unlinked attachment(s)", scope, imported)
			}
			targetKey := crossAccountAttachmentTargetKey(sourceUserID, targetUserID, sourceAttachmentID, ".bin")
			assertRestoreStorageFile(t, storage, targetKey, nil, false)
		})
	}
}

func TestCrossAccountMergeReimportRestoresMissingAttachmentAndCancelsDeletion(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	sourceUserID := "cross-reimport-source"
	targetUserID := "restore-test-user"
	sourceAttachmentID := "source-reimport"
	contents := []byte("reimported attachment bytes")
	digest := sha256.Sum256(contents)
	checksum := hex.EncodeToString(digest[:])
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
	insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)
	sourceDir := filepath.Join(baseDir, "source-attachments")
	insertCrossAccountRestoreAttachment(t, sourceDB, sourceUserID, sourceAttachmentID, "reimport.bin", contents, 0)
	if _, err := sourceDB.Exec(`UPDATE attachments SET sha256 = ? WHERE id = ?`, checksum, sourceAttachmentID); err != nil {
		t.Fatal(err)
	}
	writeRestoreTestAttachment(t, sourceDir, "reimport.bin", contents)
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	targetKey := crossAccountAttachmentTargetKey(sourceUserID, targetUserID, sourceAttachmentID, ".bin")
	storage := &restoreJournalStorage{files: map[string][]byte{}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	txManager := sqliterepo.NewSQLiteTransactionManager(liveDB)
	accountSvc := service.NewAccountService(
		sqliterepo.NewSQLiteAccountRepository(liveDB),
		sqliterepo.NewSQLiteTransactionRepository(liveDB),
		txManager,
	)
	s := &Server{db: liveDB, dbPath: livePath, attachmentSvc: attachmentSvc, acctSvc: accountSvc}
	restore := func() {
		t.Helper()
		if _, err := s.performCrossAccountMergeRestore(context.Background(), sourcePath, uploadedVersion, restoreTestSchemaVersion(t, liveDB), restoreContext{
			RequesterUserID:      targetUserID,
			BackupUserID:         sourceUserID,
			RestoreMode:          "CROSS_ACCOUNT",
			DataScope:            "both",
			AttachmentRestoreDir: sourceDir,
		}); err != nil {
			t.Fatalf("cross-account merge failed: %v", err)
		}
	}

	restore()
	assertRestoreStorageFile(t, storage, targetKey, contents, true)
	storage.mu.Lock()
	delete(storage.files, targetKey)
	storage.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := liveDB.Exec(`
		INSERT INTO attachment_deletion_queue(storage_key, user_id, attempts, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?)
	`, targetKey, targetUserID, now, now); err != nil {
		t.Fatal(err)
	}

	restore()
	assertRestoreStorageFile(t, storage, targetKey, contents, true)
	var queued int
	if err := liveDB.QueryRow(`SELECT COUNT(*) FROM attachment_deletion_queue WHERE storage_key = ?`, targetKey).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Fatalf("restored attachment still has %d pending deletion(s)", queued)
	}
}

func TestCrossAccountMergePreservesTransactionBaseCurrencyAndRateAudit(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	sourceUserID := "foreign-restore-source"
	targetUserID := "restore-test-user"
	sourcePath := filepath.Join(baseDir, "source.db")
	sourceDB := openRestoreTestDatabase(t, sourcePath, "source", false)
	insertCrossAccountRestoreUser(t, sourceDB, sourceUserID)
	sourceTxRepo := sqliterepo.NewSQLiteTransactionRepository(sourceDB)
	sourceAccountRepo := sqliterepo.NewSQLiteAccountRepository(sourceDB)
	sourceTM := sqliterepo.NewSQLiteTransactionManager(sourceDB)
	sourceAccountSvc := service.NewAccountService(sourceAccountRepo, sourceTxRepo, sourceTM)
	sourceAccount, err := sourceAccountSvc.CreateAccount(
		context.Background(), sourceUserID, "USD Wallet",
		model.AccountTypePersonal, "USD", model.ModeLife,
	)
	if err != nil {
		t.Fatal(err)
	}
	occurredAt := time.Date(2026, 7, 8, 9, 10, 11, 0, time.UTC)
	sourceTxSvc := service.NewTransactionService(sourceTxRepo, sourceAccountRepo, nil)
	createdTransaction, err := sourceTxSvc.CreateTransaction(context.Background(), service.CreateTransactionRequest{
		UserID: sourceUserID, Mode: model.ModeLife, AccountID: sourceAccount.ID,
		TxType: model.TxTypeExpense, Category: "travel", AmountCents: 12_345,
		Currency: "USD", OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if createdTransaction.BaseCurrency != "USD" || createdTransaction.ExchangeRateSource != "identity" {
		t.Fatalf("source transaction currency metadata=%+v", createdTransaction)
	}
	uploadedVersion := restoreTestSchemaVersion(t, sourceDB)
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}

	targetTxRepo := sqliterepo.NewSQLiteTransactionRepository(liveDB)
	targetAccountRepo := sqliterepo.NewSQLiteAccountRepository(liveDB)
	targetTM := sqliterepo.NewSQLiteTransactionManager(liveDB)
	s := &Server{
		db: liveDB, dbPath: livePath,
		acctSvc: service.NewAccountService(targetAccountRepo, targetTxRepo, targetTM),
	}
	restore := func() {
		t.Helper()
		if _, err := s.performCrossAccountMergeRestore(
			context.Background(), sourcePath, uploadedVersion, restoreTestSchemaVersion(t, liveDB),
			restoreContext{
				RequesterUserID: targetUserID,
				BackupUserID:    sourceUserID,
				RestoreMode:     "CROSS_ACCOUNT",
				DataScope:       "both",
			},
		); err != nil {
			t.Fatalf("cross-account restore: %v", err)
		}
	}
	restore()
	restore()

	var count int
	var currency, baseCurrency, rateSource string
	var amountCents, baseAmountCents, rateAt int64
	if err := liveDB.QueryRow(`
		SELECT COUNT(*), MIN(currency), MIN(base_currency), MIN(amount_cents),
		       MIN(base_amount_cents), MIN(exchange_rate_source), MIN(exchange_rate_at)
		FROM transactions
		WHERE user_id = ? AND restore_source_backup_id IS NOT NULL
	`, targetUserID).Scan(
		&count, &currency, &baseCurrency, &amountCents,
		&baseAmountCents, &rateSource, &rateAt,
	); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("idempotent restore imported %d foreign transactions, want 1", count)
	}
	if currency != "USD" || baseCurrency != "USD" ||
		amountCents != 12_345 || baseAmountCents != 12_345 ||
		rateSource != "identity" || rateAt != occurredAt.Unix() {
		t.Fatalf(
			"restored currency metadata=(%s,%s,%d,%d,%s,%d)",
			currency, baseCurrency, amountCents, baseAmountCents, rateSource, rateAt,
		)
	}
}

func TestRestoreRollbackFailureIncludesOriginalAndRollbackErrors(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	defer liveDB.Close()

	s := &Server{db: liveDB, dbPath: livePath}
	err := s.failRestoreAndRollback(context.Background(), filepath.Join(baseDir, "missing-safety.db"), context.DeadlineExceeded)
	if err == nil {
		t.Fatal("expected combined rollback failure")
	}
	message := err.Error()
	if !strings.Contains(message, "原始恢复错误") || !strings.Contains(message, context.DeadlineExceeded.Error()) || !strings.Contains(message, "自动回滚错误") {
		t.Fatalf("combined error lacks diagnostic context: %v", err)
	}
	if strings.Contains(message, baseDir) {
		t.Fatalf("combined rollback error leaked an absolute safety path: %v", err)
	}
	assertRestoreTestMarker(t, liveDB, "original")
}

func openRestoreTestDatabase(t *testing.T, path, marker string, withUser bool) *sql.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	database, err := findb.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	if err := findb.Migrate(ctx, database); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if withUser {
		if _, err := database.ExecContext(ctx, `
			INSERT INTO users (id, email, name, password_hash, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "restore-test-user", marker+"@example.test", marker, "not-a-real-password-hash", "owner", time.Now().Unix(), time.Now().Unix()); err != nil {
			database.Close()
			t.Fatal(err)
		}
	}
	if _, err := database.ExecContext(ctx, `CREATE TABLE restore_test_probe (marker TEXT NOT NULL)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO restore_test_probe(marker) VALUES (?)`, marker); err != nil {
		database.Close()
		t.Fatal(err)
	}
	return database
}

func restoreTestSchemaVersion(t *testing.T, database *sql.DB) int {
	t.Helper()
	var version int
	if err := database.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func assertRestoreTestMarker(t *testing.T, database *sql.DB, want string) {
	t.Helper()
	var marker string
	if err := database.QueryRow(`SELECT marker FROM restore_test_probe`).Scan(&marker); err != nil {
		t.Fatalf("read restore marker: %v", err)
	}
	if marker != want {
		t.Fatalf("restore marker=%q, want %q", marker, want)
	}
}

func assertRestoreSafetyDirectoryEmpty(t *testing.T, baseDir string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(baseDir, "safety_backups"))
	if err != nil {
		t.Fatalf("read safety backup directory: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("safety backup files were not cleaned: %v", names)
	}
}

func insertRestoreTestAttachment(t *testing.T, database *sql.DB, id, storageKey string, contents []byte, order int) {
	t.Helper()
	createdAt := time.Date(2026, 9, 4, 0, 0, order, 0, time.UTC).Format(time.RFC3339)
	digest := sha256.Sum256(contents)
	if _, err := database.Exec(`
		INSERT INTO attachments (
			id, user_id, transaction_id, storage_key, original_filename, content_type,
			size_bytes, sha256, kind, ocr_status, created_at, updated_at
		) VALUES (?, 'restore-test-user', NULL, ?, ?, 'application/octet-stream', ?, ?, 'other', 'not_requested', ?, ?)
	`, id, storageKey, storageKey, len(contents), hex.EncodeToString(digest[:]), createdAt, createdAt); err != nil {
		t.Fatal(err)
	}
}

func insertCrossAccountRestoreUser(t *testing.T, database *sql.DB, userID string) {
	t.Helper()
	now := time.Now().Unix()
	if _, err := database.Exec(`
		INSERT INTO users (id, email, name, password_hash, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, userID, userID+"@example.test", userID, "not-a-real-password-hash", "owner", now, now); err != nil {
		t.Fatal(err)
	}
}

func newCrossAccountRestoreTestServer(t *testing.T, database *sql.DB, dbPath string, attachmentSvc *service.AttachmentService) *Server {
	t.Helper()
	txManager := sqliterepo.NewSQLiteTransactionManager(database)
	accountSvc := service.NewAccountService(
		sqliterepo.NewSQLiteAccountRepository(database),
		sqliterepo.NewSQLiteTransactionRepository(database),
		txManager,
	)
	return &Server{db: database, dbPath: dbPath, attachmentSvc: attachmentSvc, acctSvc: accountSvc}
}

func insertCrossAccountRestoreAttachment(t *testing.T, database *sql.DB, userID, id, storageKey string, contents []byte, order int) {
	t.Helper()
	createdAt := time.Date(2026, 9, 4, 0, 0, order, 0, time.UTC).Format(time.RFC3339)
	digest := sha256.Sum256(contents)
	if _, err := database.Exec(`
		INSERT INTO attachments (
			id, user_id, transaction_id, storage_key, original_filename, content_type,
			size_bytes, sha256, kind, ocr_status, created_at, updated_at
		) VALUES (?, ?, NULL, ?, ?, 'application/octet-stream', ?, ?, 'other', 'not_requested', ?, ?)
	`, id, userID, storageKey, filepath.Base(storageKey), len(contents), hex.EncodeToString(digest[:]), createdAt, createdAt); err != nil {
		t.Fatal(err)
	}
}

func crossAccountAttachmentTargetKey(sourceUserID, targetUserID, sourceAttachmentID, extension string) string {
	return filepath.ToSlash(filepath.Join(
		safeStorageSegment(targetUserID),
		deterministicRestoreID("attachment", sourceUserID, targetUserID, sourceAttachmentID)+extension,
	))
}

func writeRestoreTestAttachment(t *testing.T, root, storageKey string, contents []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(storageKey))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

type restoreJournalStorage struct {
	mu                              sync.Mutex
	files                           map[string][]byte
	failRestoreKey                  string
	failDeleteKey                   string
	panicRestoreKey                 string
	panicOpenKey                    string
	failedRestore                   bool
	panickedRestore                 bool
	panickedOpen                    bool
	cancelOnRestoreErr              context.CancelFunc
	requireRestoreMaintenanceOnOpen bool
	openedOutsideRestoreMaintenance bool
}

func (s *restoreJournalStorage) Save(context.Context, string, string, string, string, io.Reader, int64) (service.StoredAttachment, error) {
	return service.StoredAttachment{}, errors.New("unexpected save")
}

func (s *restoreJournalStorage) Restore(ctx context.Context, storageKey string, reader io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	contents, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	s.mu.Lock()
	shouldFail := storageKey == s.failRestoreKey && !s.failedRestore
	shouldPanic := storageKey == s.panicRestoreKey && !s.panickedRestore
	if shouldFail {
		s.failedRestore = true
	}
	if shouldPanic {
		s.panickedRestore = true
	}
	s.mu.Unlock()
	if shouldPanic {
		panic("injected attachment restore panic")
	}
	if shouldFail {
		if s.cancelOnRestoreErr != nil {
			s.cancelOnRestoreErr()
		}
		return errors.New("injected attachment restore failure")
	}
	s.mu.Lock()
	s.files[storageKey] = append([]byte(nil), contents...)
	s.mu.Unlock()
	return nil
}

func (s *restoreJournalStorage) Open(ctx context.Context, storageKey string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.requireRestoreMaintenanceOnOpen && findb.Global().State() != findb.StateRestore {
		s.openedOutsideRestoreMaintenance = true
	}
	shouldPanic := storageKey == s.panicOpenKey && !s.panickedOpen
	if shouldPanic {
		s.panickedOpen = true
	}
	contents, ok := s.files[storageKey]
	copyOfContents := append([]byte(nil), contents...)
	s.mu.Unlock()
	if shouldPanic {
		panic("injected attachment open panic")
	}
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(copyOfContents)), nil
}

func (s *restoreJournalStorage) Delete(ctx context.Context, storageKey string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if storageKey == s.failDeleteKey {
		return errors.New("injected attachment delete failure")
	}
	s.mu.Lock()
	delete(s.files, storageKey)
	s.mu.Unlock()
	return nil
}

func assertRestoreStorageFile(t *testing.T, storage *restoreJournalStorage, storageKey string, want []byte, wantExists bool) {
	t.Helper()
	storage.mu.Lock()
	got, exists := storage.files[storageKey]
	copyOfContents := append([]byte(nil), got...)
	storage.mu.Unlock()
	if exists != wantExists {
		t.Fatalf("storage key %q exists=%t, want %t", storageKey, exists, wantExists)
	}
	if wantExists && !bytes.Equal(copyOfContents, want) {
		t.Fatalf("storage key %q contents=%q, want %q", storageKey, copyOfContents, want)
	}
}

func writeTestRestoreZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, contents := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
