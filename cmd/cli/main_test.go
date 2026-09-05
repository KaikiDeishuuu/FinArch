package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	findb "finarch/internal/infrastructure/db"
)

func TestCLIJWTSecretRejectsPublicListener(t *testing.T) {
	if _, err := cliJWTSecret("0.0.0.0:8080", ""); err == nil {
		t.Fatal("expected public listener without JWT_SECRET to fail")
	}
	if _, err := cliJWTSecret("0.0.0.0:8080", "0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("expected public listener with JWT_SECRET to fail")
	}
}

func TestCLIJWTSecretAllowsEphemeralSecretOnlyOnLoopback(t *testing.T) {
	secret, err := cliJWTSecret("127.0.0.1:8080", "")
	if err != nil {
		t.Fatalf("cliJWTSecret: %v", err)
	}
	if len(secret) != 64 {
		t.Fatalf("ephemeral secret length = %d, want 64", len(secret))
	}
	configured := "0123456789abcdef0123456789abcdef"
	got, err := cliJWTSecret("127.0.0.1:8080", configured)
	if err != nil || got != configured {
		t.Fatalf("configured secret = %q, %v", got, err)
	}
	if _, err := cliJWTSecret("127.0.0.1:8080", "short"); err == nil {
		t.Fatal("expected short configured secret to fail")
	}
}

func TestCLIAttachmentDir(t *testing.T) {
	t.Run("no environment uses local CLI default", func(t *testing.T) {
		t.Setenv("FINARCH_ATTACHMENTS_DIR", "")
		if got := cliAttachmentDir("finarch.db"); got != "attachments" {
			t.Fatalf("cliAttachmentDir() = %q, want %q", got, "attachments")
		}
	})

	t.Run("default follows the database directory", func(t *testing.T) {
		t.Setenv("FINARCH_ATTACHMENTS_DIR", "")
		databasePath := filepath.Join(t.TempDir(), "finarch.db")
		want := filepath.Join(filepath.Dir(databasePath), "attachments")
		if got := cliAttachmentDir(databasePath); got != want {
			t.Fatalf("cliAttachmentDir() = %q, want %q", got, want)
		}
		fileDSN := (&url.URL{Scheme: "file", Path: databasePath, RawQuery: "mode=ro"}).String()
		if got := cliAttachmentDir(fileDSN); got != want {
			t.Fatalf("cliAttachmentDir(file DSN) = %q, want %q", got, want)
		}
	})

	t.Run("explicit environment is shared", func(t *testing.T) {
		configured := filepath.Join(t.TempDir(), "restored-attachments")
		t.Setenv("FINARCH_ATTACHMENTS_DIR", configured)
		if got := cliAttachmentDir("ignored.db"); got != configured {
			t.Fatalf("cliAttachmentDir() = %q, want %q", got, configured)
		}
	})
}

func TestParseReimburseArgsRequiresExplicitUserID(t *testing.T) {
	if _, _, err := parseReimburseArgs([]string{"alice", "tx-1"}); err == nil {
		t.Fatal("expected missing user scope to fail")
	}
	userID, positional, err := parseReimburseArgs([]string{
		"--user-id", "user-1", "alice", "tx-1,tx-2", "REQ-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if userID != "user-1" || len(positional) != 3 || positional[2] != "REQ-1" {
		t.Fatalf("unexpected parse result: %q %#v", userID, positional)
	}
	userID, positional, err = parseReimburseArgs([]string{
		"alice", "--user-id=user-2", "tx-3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if userID != "user-2" || len(positional) != 2 {
		t.Fatalf("unexpected equals parse result: %q %#v", userID, positional)
	}
}

func TestMainDispatchesRestoreBeforeOpeningSQLite(t *testing.T) {
	directory := t.TempDir()
	restoredSource := filepath.Join(directory, "restored-source.db")
	createValidRestoreDatabase(t, restoredSource)
	target := filepath.Join(directory, "missing-target.db")
	marker := installFakeLitestream(t, restoredSource)
	setRestoreEnvironment(t, target, marker)

	originalArgs := os.Args
	os.Args = []string{"finarch-cli", "restore", "--from-r2"}
	t.Cleanup(func() { os.Args = originalArgs })

	main()

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("fake litestream did not run: %v", err)
	}
	if safety, err := filepath.Glob(target + ".pre-restore.*"); err != nil {
		t.Fatal(err)
	} else if len(safety) != 0 {
		t.Fatalf("restore dispatch opened and created the missing target before restore: %v", safety)
	}
	assertRestoredSQLiteContent(t, restoredSource, target)
}

func TestRunRestoreFromR2UsesAbsentOutputAndRetainsSafetyBackup(t *testing.T) {
	directory := t.TempDir()
	restoredSource := filepath.Join(directory, "restored-source.db")
	createValidRestoreDatabase(t, restoredSource)
	target := filepath.Join(directory, "finarch.db")
	original := []byte("original database bytes")
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	marker := installFakeLitestream(t, restoredSource)
	setRestoreEnvironment(t, target, marker)

	if err := runRestoreFromR2(context.Background(), []string{"--from-r2"}); err != nil {
		t.Fatalf("runRestoreFromR2: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("fake litestream did not observe an absent output path: %v", err)
	}
	assertRestoredSQLiteContent(t, restoredSource, target)
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := targetInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("restored target mode = %o, want 600", got)
	}

	safety, err := filepath.Glob(target + ".pre-restore.*")
	if err != nil {
		t.Fatal(err)
	}
	if len(safety) != 1 {
		t.Fatalf("safety backups = %v, want exactly one", safety)
	}
	gotOriginal, err := os.ReadFile(safety[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotOriginal, original) {
		t.Fatalf("safety backup = %q, want original bytes", gotOriginal)
	}
	if leftovers, err := filepath.Glob(filepath.Join(directory, ".finarch-r2-restore-*.db*")); err != nil {
		t.Fatal(err)
	} else if len(leftovers) != 0 {
		t.Fatalf("temporary restore artifacts were not removed: %v", leftovers)
	}
}

func TestRunRestoreFromR2DoesNotContinueWhenSafetyBackupFails(t *testing.T) {
	directory := t.TempDir()
	// The target fits within NAME_MAX, while its safety-backup suffix does not.
	// This produces a real destination-create failure without permission tricks
	// that behave differently when the test process runs as root.
	target := filepath.Join(directory, strings.Repeat("x", 230))
	original := []byte("must remain unchanged")
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	marker := installFakeLitestream(t, "")
	setRestoreEnvironment(t, target, marker)

	err := runRestoreFromR2(context.Background(), []string{"--from-r2"})
	if err == nil || !strings.Contains(err.Error(), "safety backup") {
		t.Fatalf("restore error = %v, want safety backup failure", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("litestream ran after safety backup failure; marker stat error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("target changed after safety backup failure: %q", got)
	}
}

func TestRunRestoreFromR2RejectsInvalidSnapshotWithoutReplacingTarget(t *testing.T) {
	directory := t.TempDir()
	invalidSource := filepath.Join(directory, "invalid-source.db")
	if err := os.WriteFile(invalidSource, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "finarch.db")
	original := []byte("original database bytes")
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	marker := installFakeLitestream(t, invalidSource)
	setRestoreEnvironment(t, target, marker)

	err := runRestoreFromR2(context.Background(), []string{"--from-r2"})
	if err == nil || !strings.Contains(err.Error(), "validate restored database") {
		t.Fatalf("restore error = %v, want validation failure", err)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("target changed after validation failure: %q", got)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("fake litestream did not run: %v", statErr)
	}
}

func TestRunRestoreFromR2RejectsIncompleteAttachmentRecoveryWithoutReplacingTarget(t *testing.T) {
	tests := []struct {
		name          string
		storedContent []byte
		wantError     string
	}{
		{name: "missing", wantError: "is unavailable"},
		{name: "checksum mismatch", storedContent: []byte("wrong attachment bytes"), wantError: "does not match"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			restoredSource := filepath.Join(directory, "restored-source.db")
			attachmentContent := []byte("complete attachment bytes")
			createValidRestoreDatabase(t, restoredSource)
			insertCLIBackupAttachment(t, restoredSource, "restore-user/receipt.pdf", attachmentContent)

			target := filepath.Join(directory, "finarch.db")
			original := []byte("original database bytes")
			if err := os.WriteFile(target, original, 0o600); err != nil {
				t.Fatal(err)
			}
			attachmentRoot := filepath.Join(directory, "attachments")
			if tc.storedContent != nil {
				attachmentPath := filepath.Join(attachmentRoot, "restore-user", "receipt.pdf")
				if err := os.MkdirAll(filepath.Dir(attachmentPath), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(attachmentPath, tc.storedContent, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			marker := installFakeLitestream(t, restoredSource)
			setRestoreEnvironment(t, target, marker)
			t.Setenv("FINARCH_ATTACHMENTS_DIR", attachmentRoot)
			err := runRestoreFromR2(context.Background(), []string{"--from-r2"})
			if err == nil || !strings.Contains(err.Error(), "validate restored attachments") || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("restore error = %v, want attachment validation failure containing %q", err, tc.wantError)
			}
			got, readErr := os.ReadFile(target)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !bytes.Equal(got, original) {
				t.Fatalf("target changed after attachment validation failure: %q", got)
			}
		})
	}
}

func TestRunRestoreFromR2AcceptsRestoredAttachmentsThatMatchMetadata(t *testing.T) {
	directory := t.TempDir()
	restoredSource := filepath.Join(directory, "restored-source.db")
	attachmentContent := []byte("complete attachment bytes")
	createValidRestoreDatabase(t, restoredSource)
	insertCLIBackupAttachment(t, restoredSource, "restore-user/receipt.pdf", attachmentContent)

	attachmentRoot := filepath.Join(directory, "attachments")
	attachmentPath := filepath.Join(attachmentRoot, "restore-user", "receipt.pdf")
	if err := os.MkdirAll(filepath.Dir(attachmentPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(attachmentPath, attachmentContent, 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "finarch.db")
	marker := installFakeLitestream(t, restoredSource)
	setRestoreEnvironment(t, target, marker)
	t.Setenv("FINARCH_ATTACHMENTS_DIR", attachmentRoot)

	if err := runRestoreFromR2(context.Background(), []string{"--from-r2"}); err != nil {
		t.Fatalf("runRestoreFromR2: %v", err)
	}
	assertRestoredSQLiteContent(t, restoredSource, target)
}

func TestRunRestoreFromR2InvalidatesSnapshotCredentialsBeforeActivation(t *testing.T) {
	directory := t.TempDir()
	restoredSource := filepath.Join(directory, "restored-source.db")
	createValidRestoreDatabase(t, restoredSource)
	insertCLIRestoredCredentials(t, restoredSource)
	target := filepath.Join(directory, "finarch.db")
	marker := installFakeLitestream(t, restoredSource)
	setRestoreEnvironment(t, target, marker)
	t.Setenv("FINARCH_ATTACHMENTS_DIR", filepath.Join(directory, "attachments"))

	if err := runRestoreFromR2(context.Background(), []string{"--from-r2"}); err != nil {
		t.Fatalf("runRestoreFromR2: %v", err)
	}
	database, err := findb.OpenSQLite(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var revokedAt sql.NullInt64
	var reason sql.NullString
	if err := database.QueryRow(`SELECT revoked_at, revoke_reason FROM auth_sessions WHERE id = 'snapshot-session'`).Scan(&revokedAt, &reason); err != nil {
		t.Fatal(err)
	}
	if !revokedAt.Valid || reason.String != "database_restore" {
		t.Fatalf("snapshot session remains active: revoked_at=%v reason=%v", revokedAt, reason)
	}
	var refreshCount, pendingActions, legacyTokens int
	if err := database.QueryRow(`SELECT COUNT(*) FROM refresh_tokens`).Scan(&refreshCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM action_requests WHERE status = 'pending'`).Scan(&pendingActions); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM email_tokens`).Scan(&legacyTokens); err != nil {
		t.Fatal(err)
	}
	var pendingEmail sql.NullString
	if err := database.QueryRow(`SELECT pending_email FROM users WHERE id = 'restore-user'`).Scan(&pendingEmail); err != nil {
		t.Fatal(err)
	}
	if refreshCount != 0 || pendingActions != 0 || legacyTokens != 0 || pendingEmail.Valid {
		t.Fatalf("restored credential material remains: refresh=%d actions=%d legacy=%d pending_email=%v", refreshCount, pendingActions, legacyTokens, pendingEmail)
	}
}

func createValidRestoreDatabase(t *testing.T, path string) {
	t.Helper()
	database, err := findb.OpenSQLite(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := findb.Migrate(context.Background(), database); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}

func insertCLIBackupAttachment(t *testing.T, databasePath, storageKey string, contents []byte) {
	t.Helper()
	database, err := findb.OpenSQLite(context.Background(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	if _, err := database.Exec(`
		INSERT INTO users (id, email, name, password_hash, role, created_at, updated_at)
		VALUES ('restore-user', 'restore@example.test', 'Restore User', 'not-a-password-hash', 'owner', ?, ?)
	`, now.Unix(), now.Unix()); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	if _, err := database.Exec(`
		INSERT INTO attachments (
			id, user_id, transaction_id, storage_key, original_filename, content_type,
			size_bytes, sha256, kind, ocr_status, created_at, updated_at
		) VALUES ('restore-attachment', 'restore-user', NULL, ?, 'receipt.pdf',
		          'application/pdf', ?, ?, 'receipt', 'not_requested', ?, ?)
	`, storageKey, len(contents), hex.EncodeToString(digest[:]), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
}

func insertCLIRestoredCredentials(t *testing.T, databasePath string) {
	t.Helper()
	database, err := findb.OpenSQLite(context.Background(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC().Unix()
	if _, err := database.Exec(`
		INSERT INTO users (id, email, name, username, password_hash, role, email_verified, pending_email, created_at, updated_at)
		VALUES ('restore-user', 'restore@example.test', 'Restore User', 'restore-user',
		        'not-a-password-hash', 'owner', 1, 'pending@example.test', ?, ?)
	`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO auth_sessions (id, user_id, pwd_version, absolute_expires_at, created_at, last_used_at)
		VALUES ('snapshot-session', 'restore-user', 0, ?, ?, ?)
	`, now+3600, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO refresh_tokens (
			id, session_id, token_hash, generation, expires_at, consumed_at,
			replaced_by_hash, retry_until, recovery_ciphertext, created_at
		) VALUES ('snapshot-refresh', 'snapshot-session', ?, 0, ?, ?, ?, ?, ?, ?)
	`, bytes.Repeat([]byte{1}, sha256.Size), now+1800, now, bytes.Repeat([]byte{2}, sha256.Size), now+60, []byte("recoverable-secret"), now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO action_requests (jti, user_id, action, status, meta, expires_at, created_at)
		VALUES ('snapshot-action', 'restore-user', 'password_reset', 'pending', '', ?, ?)
	`, now+1800, now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO email_tokens (token, user_id, kind, expires_at, created_at, meta)
		VALUES ('legacy-token', 'restore-user', 'reset', ?, ?, '')
	`, now+1800, now); err != nil {
		t.Fatal(err)
	}
}

func installFakeLitestream(t *testing.T, restoredSource string) string {
	t.Helper()
	binDirectory := t.TempDir()
	marker := filepath.Join(t.TempDir(), "litestream-called")
	script := `#!/bin/sh
set -eu
for output do :; done
if [ -e "$output" ]; then
  echo "restore output already exists: $output" >&2
  exit 90
fi
printf 'called' > "$FAKE_LITESTREAM_MARKER"
if [ -n "${FAKE_LITESTREAM_SOURCE:-}" ]; then
  cp "$FAKE_LITESTREAM_SOURCE" "$output"
fi
`
	path := filepath.Join(binDirectory, "litestream")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_LITESTREAM_SOURCE", restoredSource)
	return marker
}

func setRestoreEnvironment(t *testing.T, target, marker string) {
	t.Helper()
	t.Setenv("FINARCH_DB", target)
	t.Setenv("ALLOW_R2_RESTORE", "true")
	t.Setenv("LITESTREAM_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("LITESTREAM_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("LITESTREAM_BUCKET", "test-bucket")
	t.Setenv("LITESTREAM_ENDPOINT", "https://example.invalid")
	t.Setenv("FAKE_LITESTREAM_MARKER", marker)
}

func assertRestoredSQLiteContent(t *testing.T, wantPath, gotPath string) {
	t.Helper()
	if err := validateRestoredSQLite(context.Background(), gotPath); err != nil {
		t.Fatalf("restored target validation: %v", err)
	}
	wantDB, err := sql.Open("sqlite3", wantPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer wantDB.Close()
	gotDB, err := sql.Open("sqlite3", gotPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer gotDB.Close()
	for _, query := range []string{
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`,
		`SELECT COUNT(*) FROM users`,
		`SELECT COUNT(*) FROM attachments`,
	} {
		var want, got int64
		if err := wantDB.QueryRow(query).Scan(&want); err != nil {
			t.Fatal(err)
		}
		if err := gotDB.QueryRow(query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("restored query %q = %d, want %d", query, got, want)
		}
	}
}
