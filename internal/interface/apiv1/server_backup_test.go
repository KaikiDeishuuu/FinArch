package apiv1

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	findb "finarch/internal/infrastructure/db"
	"finarch/internal/infrastructure/email"
	sqliterepo "finarch/internal/infrastructure/repository"

	"github.com/gin-gonic/gin"
	"github.com/mattn/go-sqlite3"
)

var backupProbeDriverSequence atomic.Uint64

func TestBackupDownloadRejectsExportTokenInQuery(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/backup/download?export_token=must-not-be-read", nil)

	(&Server{}).handleBackupDownload(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%q, want %d", recorder.Code, recorder.Body.String(), http.StatusForbidden)
	}
}

func TestBackupManifestUsesFrozenSnapshotDatabase(t *testing.T) {
	snapshotDB := openBackupAttachmentDatabase(t)
	defer snapshotDB.Close()
	liveDB := openBackupAttachmentDatabase(t)
	defer liveDB.Close()

	frozenContents := []byte("contents present when the snapshot was frozen")
	frozenFile := backupFileForContents("user-a/frozen.bin", frozenContents)
	insertBackupAttachment(t, snapshotDB, "user-a", frozenFile)
	insertBackupAttachment(t, liveDB, "user-a", frozenFile)

	// Model a live-database mutation after the snapshot has already been
	// captured. The backup manifest must continue to describe the frozen view.
	if _, err := liveDB.Exec(`DELETE FROM attachments`); err != nil {
		t.Fatal(err)
	}
	liveContents := []byte("contents added after the snapshot")
	liveFile := backupFileForContents("user-a/live-only.bin", liveContents)
	insertBackupAttachment(t, liveDB, "user-a", liveFile)

	files, err := listSnapshotAttachmentFiles(context.Background(), snapshotDB, "user-a")
	if err != nil {
		t.Fatalf("list frozen snapshot attachments: %v", err)
	}
	if len(files) != 1 || files[0] != frozenFile {
		t.Fatalf("snapshot attachment list = %#v, want %#v", files, []attachmentBackupFile{frozenFile})
	}
	liveFiles, err := listSnapshotAttachmentFiles(context.Background(), liveDB, "user-a")
	if err != nil {
		t.Fatalf("list live attachments: %v", err)
	}
	if len(liveFiles) != 1 || liveFiles[0] != liveFile {
		t.Fatalf("test setup did not diverge live state: %#v", liveFiles)
	}

	dbPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(dbPath, []byte("frozen database bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	storage := &backupMemoryStorage{files: map[string][]byte{
		frozenFile.StorageKey: frozenContents,
		liveFile.StorageKey:   liveContents,
	}}
	server := &Server{attachmentSvc: service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)}
	zipPath, count, err := server.createBackupZip(context.Background(), dbPath, files, "snapshot-test")
	if err != nil {
		t.Fatalf("create backup zip: %v", err)
	}
	defer os.Remove(zipPath)
	if count != 1 {
		t.Fatalf("attachment count = %d, want 1", count)
	}

	manifest := readBackupManifest(t, zipPath)
	if len(manifest.Files) != 1 || manifest.Files[0] != frozenFile {
		t.Fatalf("archive manifest = %#v, want frozen file %#v", manifest.Files, frozenFile)
	}
	assertBackupZipEntry(t, zipPath, "attachments/files/"+frozenFile.StorageKey, true)
	assertBackupZipEntry(t, zipPath, "attachments/files/"+liveFile.StorageKey, false)
}

func TestBackupDownloadEnumeratesAttachmentsFromFrozenSnapshot(t *testing.T) {
	guard := findb.Global()
	previousState := guard.State()
	guard.SetState(findb.StateNormal)
	defer guard.SetState(previousState)

	probe := &backupVacuumProbeDriver{
		inner:       &sqlite3.SQLiteDriver{},
		afterVacuum: make(chan struct{}),
		resume:      make(chan struct{}),
	}
	var resumeOnce sync.Once
	defer resumeOnce.Do(func() { close(probe.resume) })
	driverName := fmt.Sprintf("finarch-backup-probe-%d", backupProbeDriverSequence.Add(1))
	sql.Register(driverName, probe)
	databasePath := filepath.Join(t.TempDir(), "live.db")
	database, err := sql.Open(driverName, databasePath)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(4)
	defer database.Close()
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := database.Exec(pragma); err != nil {
			t.Fatal(err)
		}
	}
	if err := findb.Migrate(context.Background(), database); err != nil {
		t.Fatal(err)
	}

	const userID = "snapshot-owner"
	const userEmail = "snapshot-owner@example.test"
	authService, exportToken := createBackupDownloadAuthorization(t, database, userID, userEmail)
	frozenContents := []byte("frozen attachment contents")
	frozenFile := backupFileForContents("snapshot-owner/frozen.bin", frozenContents)
	insertBackupDownloadAttachment(t, database, "frozen-attachment", userID, frozenFile)
	liveContents := []byte("post-snapshot attachment contents")
	liveFile := backupFileForContents("snapshot-owner/live-only.bin", liveContents)
	storage := &backupMemoryStorage{files: map[string][]byte{
		frozenFile.StorageKey: frozenContents,
		liveFile.StorageKey:   liveContents,
	}}
	server := &Server{
		db:            database,
		dbPath:        databasePath,
		authSvc:       authService,
		attachmentSvc: service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes),
	}

	releaseWrite, admitted := guard.TryBeginWrite()
	if !admitted {
		t.Fatal("could not acquire the request write lease")
	}
	defer releaseWrite()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/backup/download", nil)
	request.Header.Set(backupExportTokenHdr, exportToken)
	request = request.WithContext(context.WithValue(request.Context(), requestWriteLeaseContextKey{}, releaseWrite))
	w := newBackupBarrierProbeWriter(guard)
	c, _ := gin.CreateTestContext(w)
	c.Request = request
	c.Set("userID", userID)
	c.Set("userEmail", userEmail)

	handlerDone := make(chan struct{})
	go func() {
		server.handleBackupDownload(c)
		close(handlerDone)
	}()
	select {
	case <-probe.afterVacuum:
		// VACUUM has completed, but the handler has not yet been allowed to
		// enumerate attachments. Diverge the live database deterministically.
	case <-handlerDone:
		t.Fatalf("backup handler returned before snapshot probe: status=%d body=%q", w.recorder.Code, w.recorder.Body.String())
	case <-time.After(2 * time.Second):
		t.Fatal("backup handler did not complete the database snapshot")
	}
	if _, err := database.Exec(`DELETE FROM attachments`); err != nil {
		t.Fatal(err)
	}
	insertBackupDownloadAttachment(t, database, "live-attachment", userID, liveFile)
	resumeOnce.Do(func() { close(probe.resume) })
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("backup handler did not finish after snapshot probe resumed")
	}

	if w.recorder.Code != http.StatusOK {
		t.Fatalf("backup response status = %d body=%q, want 200", w.recorder.Code, w.recorder.Body.String())
	}
	if disposition := w.recorder.Header().Get("Content-Disposition"); !strings.Contains(disposition, ".zip") {
		t.Fatalf("Content-Disposition = %q, want attachment archive", disposition)
	}
	manifest := readBackupManifestBytes(t, w.recorder.Body.Bytes())
	if len(manifest.Files) != 1 || manifest.Files[0] != frozenFile {
		t.Fatalf("download manifest = %#v, want frozen snapshot file %#v", manifest.Files, frozenFile)
	}
}

func TestListSnapshotAttachmentFilesRejectsForeignUserScope(t *testing.T) {
	database := openBackupAttachmentDatabase(t)
	defer database.Close()
	insertBackupAttachment(t, database, "user-a", backupFileForContents("user-a/own.bin", []byte("own")))
	insertBackupAttachment(t, database, "user-b", backupFileForContents("user-b/private.bin", []byte("private")))

	files, err := listSnapshotAttachmentFiles(context.Background(), database, "user-a")
	if err == nil {
		t.Fatalf("foreign attachment unexpectedly accepted: %#v", files)
	}
	if !strings.Contains(err.Error(), "outside the authenticated user") {
		t.Fatalf("scope error = %v", err)
	}
}

func TestCreateBackupZipRejectsAttachmentMetadataMismatch(t *testing.T) {
	contents := []byte("authoritative storage bytes")
	valid := backupFileForContents("user-a/receipt.bin", contents)
	dbPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(dbPath, []byte("database bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := map[string]attachmentBackupFile{
		"size": {
			StorageKey: valid.StorageKey,
			SizeBytes:  valid.SizeBytes + 1,
			SHA256:     valid.SHA256,
		},
		"hash": {
			StorageKey: valid.StorageKey,
			SizeBytes:  valid.SizeBytes,
			SHA256:     strings.Repeat("0", sha256.Size*2),
		},
	}
	for name, metadata := range tests {
		t.Run(name, func(t *testing.T) {
			storage := &backupMemoryStorage{files: map[string][]byte{valid.StorageKey: contents}}
			server := &Server{attachmentSvc: service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)}
			zipPath, count, err := server.createBackupZip(context.Background(), dbPath, []attachmentBackupFile{metadata}, "mismatch-test")
			if err == nil {
				if zipPath != "" {
					_ = os.Remove(zipPath)
				}
				t.Fatal("attachment metadata mismatch unexpectedly produced an archive")
			}
			if zipPath != "" || count != 0 {
				t.Fatalf("failed archive result = (%q, %d), want empty path and zero count", zipPath, count)
			}
			if !strings.Contains(err.Error(), "does not match snapshot metadata") {
				t.Fatalf("mismatch error = %v", err)
			}
		})
	}
}

func TestWriteGateSerializesConcurrentBackupMaintenance(t *testing.T) {
	guard := findb.Global()
	previousState := guard.State()
	guard.SetState(findb.StateNormal)
	defer guard.SetState(previousState)

	server := &Server{}
	router := gin.New()
	router.Use(server.writeGateMiddleware())

	firstEntered := make(chan struct{})
	secondEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseFirstOnce sync.Once
	defer releaseFirstOnce.Do(func() { close(releaseFirst) })
	requestsDone := make(chan struct{}, 2)
	var calls atomic.Int32
	router.POST("/api/v1/backup/download", func(c *gin.Context) {
		releaseRequestWriteLease(c.Request.Context())
		prior := guard.BeginMaintenance(findb.StateBackup)
		defer guard.EndMaintenance(prior)
		switch calls.Add(1) {
		case 1:
			close(firstEntered)
			<-releaseFirst
		case 2:
			close(secondEntered)
		default:
			t.Error("unexpected extra backup handler invocation")
		}
		c.Status(http.StatusNoContent)
	})

	request := func() {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/backup/download", nil))
		if recorder.Code != http.StatusNoContent {
			t.Errorf("backup response status = %d, want %d", recorder.Code, http.StatusNoContent)
		}
		requestsDone <- struct{}{}
	}
	go request()
	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first backup did not enter maintenance")
	}
	go request()
	select {
	case <-secondEntered:
		t.Fatal("second backup entered before the first released maintenance")
	case <-time.After(50 * time.Millisecond):
	}
	releaseFirstOnce.Do(func() { close(releaseFirst) })
	for range 2 {
		select {
		case <-requestsDone:
		case <-time.After(time.Second):
			t.Fatal("concurrent backup requests deadlocked")
		}
	}
	select {
	case <-secondEntered:
	default:
		t.Fatal("second backup never entered maintenance")
	}
}

func TestBackupDownloadReleasesMaintenanceBeforeResponseBody(t *testing.T) {
	guard := findb.Global()
	previousState := guard.State()
	guard.SetState(findb.StateNormal)
	defer guard.SetState(previousState)

	databasePath := filepath.Join(t.TempDir(), "live.db")
	database, err := findb.OpenSQLite(context.Background(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := findb.Migrate(context.Background(), database); err != nil {
		t.Fatal(err)
	}

	const userID = "backup-download-user"
	const userEmail = "backup-download@example.test"
	authService, exportToken := createBackupDownloadAuthorization(t, database, userID, userEmail)
	server := &Server{db: database, dbPath: databasePath, authSvc: authService}

	releaseWrite, admitted := guard.TryBeginWrite()
	if !admitted {
		t.Fatal("could not acquire the request write lease")
	}
	defer releaseWrite()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/backup/download", nil)
	request.Header.Set(backupExportTokenHdr, exportToken)
	request = request.WithContext(context.WithValue(request.Context(), requestWriteLeaseContextKey{}, releaseWrite))
	w := newBackupBarrierProbeWriter(guard)
	c, _ := gin.CreateTestContext(w)
	c.Request = request
	c.Set("userID", userID)
	c.Set("userEmail", userEmail)

	server.handleBackupDownload(c)

	if w.recorder.Code != http.StatusOK {
		t.Fatalf("backup response status = %d body=%q, want 200", w.recorder.Code, w.recorder.Body.String())
	}
	if !w.observed {
		t.Fatal("backup response did not write a body")
	}
	if w.stateAtFirstWrite != findb.StateNormal || !w.writeAdmittedAtFirstWrite {
		t.Fatalf(
			"first response write saw state=%v admitted=%t, want normal and admitted",
			w.stateAtFirstWrite,
			w.writeAdmittedAtFirstWrite,
		)
	}
	if state := guard.State(); state != findb.StateNormal {
		t.Fatalf("guard state after backup = %v, want normal", state)
	}
	if disposition := w.recorder.Header().Get("Content-Disposition"); !strings.Contains(disposition, ".db") {
		t.Fatalf("Content-Disposition = %q, want database download", disposition)
	}
}

func createBackupDownloadAuthorization(t *testing.T, database *sql.DB, userID, userEmail string) (*service.AuthService, string) {
	t.Helper()
	now := time.Now().UTC()
	userRepo := sqliterepo.NewSQLiteUserRepository(database)
	if err := userRepo.Create(context.Background(), model.User{
		ID:            userID,
		Email:         userEmail,
		Name:          userID,
		Username:      userID,
		Nickname:      "Backup Test",
		PasswordHash:  "unused-in-this-test",
		Role:          "owner",
		EmailVerified: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatal(err)
	}

	const secret = "backup-download-test-secret"
	actionTokens := auth.NewActionTokenService(secret)
	exportToken, jti, expiresAt, err := actionTokens.Issue(userID, service.ActionBackupExport, "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := userRepo.CreateActionRequest(context.Background(), model.ActionRequest{
		JTI:       jti,
		UserID:    userID,
		Action:    service.ActionBackupExport,
		Status:    "pending",
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	jwtService := auth.NewJWTService(secret)
	sessionService, err := service.NewSessionService(sqliterepo.NewSQLiteRefreshTokenRepository(database), jwtService, secret)
	if err != nil {
		t.Fatal(err)
	}
	authService := service.NewAuthService(
		userRepo,
		actionTokens,
		auth.NewLoginAttemptTracker(5, time.Minute),
		&email.NoopSender{},
		false,
		"http://localhost",
		sqliterepo.NewSQLiteTransactionManager(database),
		sessionService,
	)
	return authService, exportToken
}

func insertBackupDownloadAttachment(t *testing.T, database *sql.DB, attachmentID, userID string, file attachmentBackupFile) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := database.Exec(`
		INSERT INTO attachments (
			id, user_id, transaction_id, storage_key, original_filename, content_type,
			size_bytes, sha256, kind, ocr_status, created_at, updated_at
		) VALUES (?, ?, NULL, ?, ?, 'application/octet-stream', ?, ?, 'other', 'not_requested', ?, ?)
	`, attachmentID, userID, file.StorageKey, filepath.Base(file.StorageKey), file.SizeBytes, file.SHA256, now, now); err != nil {
		t.Fatal(err)
	}
}

func openBackupAttachmentDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	if _, err := database.Exec(`
		CREATE TABLE attachments (
			user_id TEXT NOT NULL,
			storage_key TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			sha256 TEXT NOT NULL
		)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	return database
}

func insertBackupAttachment(t *testing.T, database *sql.DB, userID string, file attachmentBackupFile) {
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO attachments(user_id, storage_key, size_bytes, sha256) VALUES (?, ?, ?, ?)`,
		userID, file.StorageKey, file.SizeBytes, file.SHA256,
	); err != nil {
		t.Fatal(err)
	}
}

func backupFileForContents(storageKey string, contents []byte) attachmentBackupFile {
	digest := sha256.Sum256(contents)
	return attachmentBackupFile{
		StorageKey: storageKey,
		SizeBytes:  int64(len(contents)),
		SHA256:     hex.EncodeToString(digest[:]),
	}
}

type backupMemoryStorage struct {
	files map[string][]byte
}

func (s *backupMemoryStorage) Save(context.Context, string, string, string, string, io.Reader, int64) (service.StoredAttachment, error) {
	return service.StoredAttachment{}, errors.New("unexpected save")
}

func (s *backupMemoryStorage) Restore(context.Context, string, io.Reader) error {
	return errors.New("unexpected restore")
}

func (s *backupMemoryStorage) Open(ctx context.Context, storageKey string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	contents, ok := s.files[storageKey]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(contents)), nil
}

func (s *backupMemoryStorage) Delete(context.Context, string) error {
	return errors.New("unexpected delete")
}

func readBackupManifest(t *testing.T, zipPath string) attachmentBackupManifest {
	t.Helper()
	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, file := range archive.File {
		if file.Name != "attachments/manifest.json" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		var manifest attachmentBackupManifest
		decodeErr := json.NewDecoder(reader).Decode(&manifest)
		closeErr := reader.Close()
		if decodeErr != nil || closeErr != nil {
			t.Fatal(errors.Join(decodeErr, closeErr))
		}
		return manifest
	}
	t.Fatal("archive does not contain an attachment manifest")
	return attachmentBackupManifest{}
}

func readBackupManifestBytes(t *testing.T, contents []byte) attachmentBackupManifest {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if file.Name != "attachments/manifest.json" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		var manifest attachmentBackupManifest
		decodeErr := json.NewDecoder(reader).Decode(&manifest)
		closeErr := reader.Close()
		if decodeErr != nil || closeErr != nil {
			t.Fatal(errors.Join(decodeErr, closeErr))
		}
		return manifest
	}
	t.Fatal("archive does not contain an attachment manifest")
	return attachmentBackupManifest{}
}

func assertBackupZipEntry(t *testing.T, zipPath, entryName string, want bool) {
	t.Helper()
	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, file := range archive.File {
		if file.Name == entryName {
			if !want {
				t.Fatalf("archive unexpectedly contains %q", entryName)
			}
			return
		}
	}
	if want {
		t.Fatalf("archive does not contain %q", entryName)
	}
}

type backupBarrierProbeWriter struct {
	recorder                  *httptest.ResponseRecorder
	guard                     *findb.ConcurrencyGuard
	once                      sync.Once
	observed                  bool
	stateAtFirstWrite         findb.SystemState
	writeAdmittedAtFirstWrite bool
}

func newBackupBarrierProbeWriter(guard *findb.ConcurrencyGuard) *backupBarrierProbeWriter {
	return &backupBarrierProbeWriter{recorder: httptest.NewRecorder(), guard: guard}
}

func (w *backupBarrierProbeWriter) Header() http.Header {
	return w.recorder.Header()
}

func (w *backupBarrierProbeWriter) WriteHeader(statusCode int) {
	w.observeBarrier()
	w.recorder.WriteHeader(statusCode)
}

func (w *backupBarrierProbeWriter) Write(contents []byte) (int, error) {
	w.observeBarrier()
	return w.recorder.Write(contents)
}

func (w *backupBarrierProbeWriter) observeBarrier() {
	w.once.Do(func() {
		w.observed = true
		w.stateAtFirstWrite = w.guard.State()
		release, admitted := w.guard.TryBeginWrite()
		w.writeAdmittedAtFirstWrite = admitted
		if admitted {
			release()
		}
	})
}

type backupVacuumProbeDriver struct {
	inner       driver.Driver
	afterVacuum chan struct{}
	resume      chan struct{}
	once        sync.Once
}

func (d *backupVacuumProbeDriver) Open(name string) (driver.Conn, error) {
	connection, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &backupVacuumProbeConn{Conn: connection, probe: d}, nil
}

type backupVacuumProbeConn struct {
	driver.Conn
	probe *backupVacuumProbeDriver
}

func (c *backupVacuumProbeConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	execer, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	result, err := execer.ExecContext(ctx, query, args)
	if err != nil || !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "VACUUM INTO") {
		return result, err
	}
	c.probe.once.Do(func() { close(c.probe.afterVacuum) })
	select {
	case <-c.probe.resume:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
