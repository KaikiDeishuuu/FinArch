package apiv1

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"finarch/internal/domain/service"

	"github.com/google/uuid"
)

func TestRecoverPreparedReplacementBeforeOpenRollsBackDatabase(t *testing.T) {
	baseDir := t.TempDir()
	livePath := baseDir + "/live.db"
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	server := &Server{db: liveDB, dbPath: livePath}
	operation, _ := prepareReplacementRestoreForTest(t, server, livePath, false)
	if err := operation.markPrepared(); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDB.Exec(`UPDATE restore_test_probe SET marker = 'partly-restored'`); err != nil {
		t.Fatal(err)
	}
	if err := liveDB.Close(); err != nil {
		t.Fatal(err)
	}

	if err := RecoverPendingRestoresBeforeOpen(context.Background(), livePath, nil); err != nil {
		t.Fatalf("pre-open recovery: %v", err)
	}
	recovered := openExistingRestoreTestDatabase(t, livePath)
	defer recovered.Close()
	assertRestoreTestMarker(t, recovered, "original")
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
}

func TestRecoverPreparingReplacementDiscardsOnlyPlannedArtifacts(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	if err := liveDB.Close(); err != nil {
		t.Fatal(err)
	}

	operation, err := beginReplacementRestorePreparation(livePath, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(operation.markerPath); err != nil {
		t.Fatalf("preparing marker was not durable before artifact creation: %v", err)
	}
	safetyPath := filepath.Join(filepath.Dir(operation.markerPath), operation.marker.SafetyBackup)
	journalPath := filepath.Join(filepath.Dir(operation.markerPath), operation.marker.AttachmentJournal)
	for _, plannedPath := range []string{safetyPath, journalPath} {
		if _, err := os.Stat(plannedPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("planned artifact existed before preparation started: %s (%v)", filepath.Base(plannedPath), err)
		}
	}
	if err := os.WriteFile(safetyPath, []byte("partial sqlite snapshot"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(journalPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalPath, "partial.bin"), []byte("sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RecoverPendingRestoresBeforeOpen(context.Background(), livePath, nil); err != nil {
		t.Fatalf("discard preparing replacement: %v", err)
	}
	reopened := openExistingRestoreTestDatabase(t, livePath)
	defer reopened.Close()
	assertRestoreTestMarker(t, reopened, "original")
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
}

func TestRecoverPreparingCrossRestoreDoesNotCompensateAttachments(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	operation, err := beginCrossRestorePreparation(livePath, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(filepath.Dir(operation.markerPath), operation.marker.AttachmentJournal)
	if err := os.Mkdir(journalPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalPath, "partial.bin"), []byte("sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}
	storage := &restoreJournalStorage{files: map[string][]byte{"receipt.bin": []byte("untouched")}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)

	if err := RecoverPendingCrossAccountRestores(context.Background(), liveDB, livePath, attachmentSvc); err != nil {
		t.Fatalf("discard preparing cross-account restore: %v", err)
	}
	assertRestoreStorageFile(t, storage, "receipt.bin", []byte("untouched"), true)
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
}

func TestRecoverCommittedReplacementBeforeOpenKeepsRestoredDatabase(t *testing.T) {
	baseDir := t.TempDir()
	livePath := baseDir + "/live.db"
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	server := &Server{db: liveDB, dbPath: livePath}
	operation, _ := prepareReplacementRestoreForTest(t, server, livePath, false)
	if err := operation.markPrepared(); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDB.Exec(`UPDATE restore_test_probe SET marker = 'replacement'`); err != nil {
		t.Fatal(err)
	}
	if err := operation.markCommitted(); err != nil {
		t.Fatal(err)
	}
	if err := liveDB.Close(); err != nil {
		t.Fatal(err)
	}

	if err := RecoverPendingRestoresBeforeOpen(context.Background(), livePath, nil); err != nil {
		t.Fatalf("pre-open recovery: %v", err)
	}
	recovered := openExistingRestoreTestDatabase(t, livePath)
	defer recovered.Close()
	assertRestoreTestMarker(t, recovered, "replacement")
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
}

func TestRecoverPreparedReplacementUsesDurableLedgerAfterAmbiguousMarkerSync(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	server := &Server{db: liveDB, dbPath: livePath}
	operation, _ := prepareReplacementRestoreForTest(t, server, livePath, false)
	if err := operation.markPrepared(); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDB.Exec(`UPDATE restore_test_probe SET marker = 'replacement'`); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDB.Exec(`
		INSERT INTO restore_operations(operation_id, batch_id, kind, created_at)
		VALUES (?, ?, 'replace', 1)
	`, operation.marker.OperationID, operation.marker.OperationID); err != nil {
		t.Fatal(err)
	}
	if err := ensureRestoreDatabaseDurable(context.Background(), liveDB, livePath); err != nil {
		t.Fatal(err)
	}
	// Leave the external phase at prepared, modelling a committed marker rename
	// whose parent-directory fsync failed and later exposed the old name/content.
	if err := liveDB.Close(); err != nil {
		t.Fatal(err)
	}

	if err := RecoverPendingRestoresBeforeOpen(context.Background(), livePath, nil); err != nil {
		t.Fatalf("pre-open recovery: %v", err)
	}
	recovered := openExistingRestoreTestDatabase(t, livePath)
	defer recovered.Close()
	assertRestoreTestMarker(t, recovered, "replacement")
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
}

func TestRecoverPreparedReplacementLedgerQueryErrorFailsClosed(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	server := &Server{db: liveDB, dbPath: livePath}
	operation, safetyPath := prepareReplacementRestoreForTest(t, server, livePath, false)
	if err := operation.markPrepared(); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDB.Exec(`UPDATE restore_test_probe SET marker = 'partly-restored'`); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDB.Exec(`DROP TABLE restore_operations`); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDB.Exec(`CREATE TABLE restore_operations(unrelated_column TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := liveDB.Close(); err != nil {
		t.Fatal(err)
	}

	err := RecoverPendingRestoresBeforeOpen(context.Background(), livePath, nil)
	if err == nil {
		t.Fatal("expected malformed replacement ledger to fail closed")
	}
	reopened := openExistingRestoreTestDatabase(t, livePath)
	defer reopened.Close()
	assertRestoreTestMarker(t, reopened, "partly-restored")
	if _, statErr := os.Stat(safetyPath); statErr != nil {
		t.Fatalf("safety backup was removed after ledger query error: %v", statErr)
	}
	if _, statErr := os.Stat(operation.markerPath); statErr != nil {
		t.Fatalf("prepared marker was removed after ledger query error: %v", statErr)
	}
}

func TestRecoverPreparedReplacementWithJournal(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	server := &Server{db: liveDB, dbPath: livePath}
	operation, _ := prepareReplacementRestoreForTest(t, server, livePath, true)
	storage := &restoreJournalStorage{files: map[string][]byte{"receipt.bin": []byte("original")}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	journal, err := prepareAttachmentRollbackJournal(context.Background(), filepath.Join(baseDir, "safety_backups"), []attachmentRestoreFile{{TargetKey: "receipt.bin"}}, attachmentSvc, operation)
	if err != nil {
		t.Fatal(err)
	}
	if err := operation.attachJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := operation.markPrepared(); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDB.Exec(`UPDATE restore_test_probe SET marker = 'partly-restored'`); err != nil {
		t.Fatal(err)
	}
	storage.files["receipt.bin"] = []byte("replacement")
	if err := liveDB.Close(); err != nil {
		t.Fatal(err)
	}

	if err := RecoverPendingRestoresBeforeOpen(context.Background(), livePath, attachmentSvc); err != nil {
		t.Fatalf("pre-open recovery: %v", err)
	}
	recovered := openExistingRestoreTestDatabase(t, livePath)
	defer recovered.Close()
	assertRestoreTestMarker(t, recovered, "original")
	assertRestoreStorageFile(t, storage, "receipt.bin", []byte("original"), true)
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
}

func TestRecoverPreparedReplacementValidatesJournalBeforeDatabaseMutation(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	server := &Server{db: liveDB, dbPath: livePath}
	operation, safetyPath := prepareReplacementRestoreForTest(t, server, livePath, true)
	storage := &restoreJournalStorage{files: map[string][]byte{"receipt.bin": []byte("original")}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	journal, err := prepareAttachmentRollbackJournal(context.Background(), filepath.Join(baseDir, "safety_backups"), []attachmentRestoreFile{{TargetKey: "receipt.bin"}}, attachmentSvc, operation)
	if err != nil {
		t.Fatal(err)
	}
	if err := operation.attachJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := operation.markPrepared(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journal.dir, "manifest.json"), []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDB.Exec(`UPDATE restore_test_probe SET marker = 'partly-restored'`); err != nil {
		t.Fatal(err)
	}
	if err := liveDB.Close(); err != nil {
		t.Fatal(err)
	}

	err = RecoverPendingRestoresBeforeOpen(context.Background(), livePath, attachmentSvc)
	if err == nil {
		t.Fatal("expected malformed attachment journal to block recovery")
	}
	reopened := openExistingRestoreTestDatabase(t, livePath)
	defer reopened.Close()
	assertRestoreTestMarker(t, reopened, "partly-restored")
	if _, statErr := os.Stat(safetyPath); statErr != nil {
		t.Fatalf("safety backup was removed after journal validation error: %v", statErr)
	}
	if _, statErr := os.Stat(operation.markerPath); statErr != nil {
		t.Fatalf("prepared marker was removed after journal validation error: %v", statErr)
	}
}

func TestRecoverPreparedReplacementRejectsTamperedJournalBeforeDatabaseMutation(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	server := &Server{db: liveDB, dbPath: livePath}
	operation, safetyPath := prepareReplacementRestoreForTest(t, server, livePath, true)
	storage := &restoreJournalStorage{files: map[string][]byte{"receipt.bin": []byte("original")}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	journal, err := prepareAttachmentRollbackJournal(context.Background(), filepath.Join(baseDir, "safety_backups"), []attachmentRestoreFile{{TargetKey: "receipt.bin"}}, attachmentSvc, operation)
	if err != nil {
		t.Fatal(err)
	}
	if err := operation.attachJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := operation.markPrepared(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journal.entries[0].backupPath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDB.Exec(`UPDATE restore_test_probe SET marker = 'partly-restored'`); err != nil {
		t.Fatal(err)
	}
	storage.files["receipt.bin"] = []byte("replacement")
	if err := liveDB.Close(); err != nil {
		t.Fatal(err)
	}

	err = RecoverPendingRestoresBeforeOpen(context.Background(), livePath, attachmentSvc)
	if err == nil {
		t.Fatal("expected tampered attachment journal to block recovery")
	}
	reopened := openExistingRestoreTestDatabase(t, livePath)
	defer reopened.Close()
	assertRestoreTestMarker(t, reopened, "partly-restored")
	assertRestoreStorageFile(t, storage, "receipt.bin", []byte("replacement"), true)
	for _, retainedPath := range []string{safetyPath, operation.markerPath, journal.dir} {
		if _, statErr := os.Stat(retainedPath); statErr != nil {
			t.Fatalf("recovery artifact %q was removed after integrity failure: %v", filepath.Base(retainedPath), statErr)
		}
	}
}

func TestRecoverRejectsRootMarkerFilenameFromAnotherOperationWithoutCleanup(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	if err := liveDB.Close(); err != nil {
		t.Fatal(err)
	}
	operation, err := beginReplacementRestorePreparation(livePath)
	if err != nil {
		t.Fatal(err)
	}
	safetyPath := filepath.Join(filepath.Dir(operation.markerPath), operation.marker.SafetyBackup)
	if err := os.WriteFile(safetyPath, []byte("keep safety artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	forgedMarkerPath := filepath.Join(filepath.Dir(operation.markerPath), restoreOperationRootPrefix+uuid.NewString()+restoreOperationRootSuffix)
	if err := os.Rename(operation.markerPath, forgedMarkerPath); err != nil {
		t.Fatal(err)
	}

	if err := RecoverPendingRestoresBeforeOpen(context.Background(), livePath, nil); err == nil {
		t.Fatal("expected operation-unbound root marker filename to fail closed")
	}
	if contents, err := os.ReadFile(safetyPath); err != nil || string(contents) != "keep safety artifact" {
		t.Fatalf("safety artifact changed after marker rejection: contents=%q err=%v", contents, err)
	}
	if _, err := os.Stat(forgedMarkerPath); err != nil {
		t.Fatalf("forged marker was removed after rejection: %v", err)
	}
}

func TestRecoverRejectsForeignOperationArtifactReferencesWithoutCleanup(t *testing.T) {
	for _, phase := range []string{restoreOperationPhasePrepared, restoreOperationPhaseCommitted, restoreOperationPhaseRolledBack} {
		for _, field := range []string{"safety", "journal"} {
			t.Run(phase+"_"+field, func(t *testing.T) {
				baseDir := t.TempDir()
				livePath := filepath.Join(baseDir, "live.db")
				liveDB := openRestoreTestDatabase(t, livePath, "original", true)
				if err := liveDB.Close(); err != nil {
					t.Fatal(err)
				}
				safetyDir := filepath.Join(baseDir, "safety_backups")
				if err := os.Mkdir(safetyDir, 0o700); err != nil {
					t.Fatal(err)
				}
				operationID := uuid.NewString()
				foreignOperationID := uuid.NewString()
				marker := restoreOperationMarker{
					Version:      restoreOperationMarkerVersion,
					OperationID:  operationID,
					Kind:         restoreOperationKindReplace,
					Phase:        phase,
					SafetyBackup: restoreSafetyFilePrefix + operationID + ".db",
				}
				foreignSafety := filepath.Join(safetyDir, restoreSafetyFilePrefix+foreignOperationID+".db")
				foreignJournal := filepath.Join(safetyDir, restoreJournalDirPrefix+foreignOperationID)
				switch field {
				case "safety":
					marker.SafetyBackup = filepath.Base(foreignSafety)
					if err := os.WriteFile(foreignSafety, []byte("foreign safety"), 0o600); err != nil {
						t.Fatal(err)
					}
				case "journal":
					marker.AttachmentJournal = filepath.Base(foreignJournal)
					if err := os.Mkdir(foreignJournal, 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(foreignJournal, "sentinel"), []byte("foreign journal"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				markerPath := filepath.Join(safetyDir, restoreOperationRootPrefix+operationID+restoreOperationRootSuffix)
				writeRawRestoreMarkerForTest(t, markerPath, marker)

				if err := RecoverPendingRestoresBeforeOpen(context.Background(), livePath, nil); err == nil {
					t.Fatal("expected foreign operation artifact reference to fail closed")
				}
				if field == "safety" {
					if contents, err := os.ReadFile(foreignSafety); err != nil || string(contents) != "foreign safety" {
						t.Fatalf("foreign safety artifact changed: contents=%q err=%v", contents, err)
					}
				} else if contents, err := os.ReadFile(filepath.Join(foreignJournal, "sentinel")); err != nil || string(contents) != "foreign journal" {
					t.Fatalf("foreign journal artifact changed: contents=%q err=%v", contents, err)
				}
			})
		}
	}
}

func TestRecoverRejectsEmbeddedMarkerInAnotherOperationJournalWithoutCleanup(t *testing.T) {
	for _, journalName := range []string{
		restoreJournalDirPrefix + uuid.NewString(),
		retainedRestoreJournalName("forged-retention", uuid.NewString()),
	} {
		t.Run(journalName[:strings.IndexByte(journalName, '-')], func(t *testing.T) {
			baseDir := t.TempDir()
			livePath := filepath.Join(baseDir, "live.db")
			liveDB := openRestoreTestDatabase(t, livePath, "original", true)
			if err := liveDB.Close(); err != nil {
				t.Fatal(err)
			}
			safetyDir := filepath.Join(baseDir, "safety_backups")
			foreignJournal := filepath.Join(safetyDir, journalName)
			if err := os.MkdirAll(foreignJournal, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(foreignJournal, "sentinel"), []byte("do not clean"), 0o600); err != nil {
				t.Fatal(err)
			}
			operationID := uuid.NewString()
			marker := restoreOperationMarker{
				Version:           restoreOperationMarkerVersion,
				OperationID:       operationID,
				Kind:              restoreOperationKindReplace,
				Phase:             restoreOperationPhasePrepared,
				SafetyBackup:      restoreSafetyFilePrefix + operationID + ".db",
				AttachmentJournal: restoreJournalDirPrefix + operationID,
			}
			writeRawRestoreMarkerForTest(t, filepath.Join(foreignJournal, restoreOperationJournalFile), marker)

			if err := RecoverPendingRestoresBeforeOpen(context.Background(), livePath, nil); err == nil {
				t.Fatal("expected marker embedded in a foreign journal to fail closed")
			}
			if contents, err := os.ReadFile(filepath.Join(foreignJournal, "sentinel")); err != nil || string(contents) != "do not clean" {
				t.Fatalf("foreign journal changed after rejection: contents=%q err=%v", contents, err)
			}
		})
	}
}

func TestRecoverRecognizesJournalRetainedByProductionPath(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "original", true)
	server := &Server{db: liveDB, dbPath: livePath}
	operation, _ := prepareReplacementRestoreForTest(t, server, livePath, true)
	storage := &restoreJournalStorage{files: map[string][]byte{"receipt.bin": []byte("original")}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	journal, err := prepareAttachmentRollbackJournal(context.Background(), filepath.Join(baseDir, "safety_backups"), []attachmentRestoreFile{{TargetKey: "receipt.bin"}}, attachmentSvc, operation)
	if err != nil {
		t.Fatal(err)
	}
	if err := operation.attachJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := operation.markPrepared(); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDB.Exec(`UPDATE restore_test_probe SET marker = 'partly-restored'`); err != nil {
		t.Fatal(err)
	}
	storage.files["receipt.bin"] = []byte("replacement")
	retainedName, err := journal.retain("restore-case", operation)
	if err != nil {
		t.Fatal(err)
	}
	if !validRetainedRestoreJournalName(retainedName, operation.marker.OperationID) {
		t.Fatalf("production retain generated invalid journal name %q", retainedName)
	}
	if err := liveDB.Close(); err != nil {
		t.Fatal(err)
	}

	operations, err := discoverRestoreOperations(livePath)
	if err != nil {
		t.Fatalf("discover retained journal: %v", err)
	}
	if len(operations) != 1 || operations[0].markerPath != operation.markerPath {
		t.Fatalf("retained operation discovery = %#v, want marker %q", operations, operation.markerPath)
	}
	if err := RecoverPendingRestoresBeforeOpen(context.Background(), livePath, attachmentSvc); err != nil {
		t.Fatalf("recover retained journal: %v", err)
	}
	recovered := openExistingRestoreTestDatabase(t, livePath)
	defer recovered.Close()
	assertRestoreTestMarker(t, recovered, "original")
	assertRestoreStorageFile(t, storage, "receipt.bin", []byte("original"), true)
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
}

func TestRecoverRejectsPreparedJournalMarkerLeftAtRoot(t *testing.T) {
	for _, kind := range []string{restoreOperationKindReplace, restoreOperationKindCross} {
		t.Run(kind, func(t *testing.T) {
			baseDir := t.TempDir()
			livePath := filepath.Join(baseDir, "live.db")
			liveDB := openRestoreTestDatabase(t, livePath, "original", true)
			defer liveDB.Close()

			var operation *durableRestoreOperation
			var err error
			if kind == restoreOperationKindReplace {
				operation, err = beginReplacementRestorePreparation(livePath, true)
			} else {
				operation, err = beginCrossRestorePreparation(livePath, uuid.NewString())
			}
			if err != nil {
				t.Fatal(err)
			}
			journalDir := filepath.Join(filepath.Dir(operation.markerPath), operation.marker.AttachmentJournal)
			if err := os.Mkdir(journalDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(journalDir, "sentinel"), []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			operation.marker.Phase = restoreOperationPhasePrepared
			writeRawRestoreMarkerForTest(t, operation.markerPath, operation.marker)

			if kind == restoreOperationKindReplace {
				err = RecoverPendingRestoresBeforeOpen(context.Background(), livePath, nil)
			} else {
				err = RecoverPendingCrossAccountRestores(context.Background(), liveDB, livePath, nil)
			}
			if err == nil {
				t.Fatal("expected prepared journal-backed root marker to fail closed")
			}
			if contents, statErr := os.ReadFile(filepath.Join(journalDir, "sentinel")); statErr != nil || string(contents) != "keep" {
				t.Fatalf("journal changed after root marker rejection: contents=%q err=%v", contents, statErr)
			}
		})
	}
}

func TestAttachmentRollbackRevalidatesBackupImmediatelyBeforeRestore(t *testing.T) {
	baseDir := t.TempDir()
	storage := &restoreJournalStorage{files: map[string][]byte{"receipt.bin": []byte("original")}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	journal, err := prepareAttachmentRollbackJournal(context.Background(), filepath.Join(baseDir, "safety_backups"), []attachmentRestoreFile{{TargetKey: "receipt.bin"}}, attachmentSvc)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadAttachmentRollbackJournal(journal.dir, attachmentSvc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journal.entries[0].backupPath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	storage.files["receipt.bin"] = []byte("replacement")

	if err := loaded.rollback(context.Background()); err == nil {
		t.Fatal("expected rollback to reject backup bytes changed after journal load")
	}
	assertRestoreStorageFile(t, storage, "receipt.bin", []byte("replacement"), true)
}

func TestRecoverRejectsSymlinkedSafetyDirectoryWithoutTouchingTarget(t *testing.T) {
	baseDir := t.TempDir()
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(baseDir, "safety_backups")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	err := RecoverPendingRestoresBeforeOpen(context.Background(), filepath.Join(baseDir, "live.db"), nil)
	if err == nil {
		t.Fatal("expected symlinked safety directory to be rejected")
	}
	contents, readErr := os.ReadFile(sentinel)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != "keep" {
		t.Fatalf("outside sentinel changed to %q", contents)
	}
}

func TestRecoverRejectsGroupWritableSafetyDirectory(t *testing.T) {
	baseDir := t.TempDir()
	safetyDir := filepath.Join(baseDir, "safety_backups")
	if err := os.Mkdir(safetyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(safetyDir, 0o770); err != nil {
		t.Fatal(err)
	}

	err := RecoverPendingRestoresBeforeOpen(context.Background(), filepath.Join(baseDir, "live.db"), nil)
	if err == nil {
		t.Fatal("expected group-writable restore root to be rejected")
	}
	info, statErr := os.Stat(safetyDir)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o770 {
		t.Fatalf("recovery changed untrusted root permissions to %#o", info.Mode().Perm())
	}
}

func TestDiscoverRestoreOperationsRejectsNonRegularRootMarkers(t *testing.T) {
	testCases := []struct {
		name   string
		create func(*testing.T, string)
	}{
		{
			name: "symlink",
			create: func(t *testing.T, markerPath string) {
				t.Helper()
				target := filepath.Join(filepath.Dir(filepath.Dir(markerPath)), "symlink-target.json")
				if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, markerPath); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
		},
		{
			name: "directory",
			create: func(t *testing.T, markerPath string) {
				t.Helper()
				if err := os.Mkdir(markerPath, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			baseDir := t.TempDir()
			safetyDir := filepath.Join(baseDir, "safety_backups")
			if err := os.Mkdir(safetyDir, 0o700); err != nil {
				t.Fatal(err)
			}
			markerPath := filepath.Join(safetyDir, restoreOperationRootPrefix+uuid.NewString()+restoreOperationRootSuffix)
			testCase.create(t, markerPath)

			assertNonRegularRestoreMarkerRejected(t, filepath.Join(baseDir, "live.db"))
		})
	}
}

func TestDiscoverRestoreOperationsRejectsNonRegularJournalMarkers(t *testing.T) {
	testCases := []struct {
		name   string
		create func(*testing.T, string)
	}{
		{
			name: "symlink",
			create: func(t *testing.T, markerPath string) {
				t.Helper()
				target := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(markerPath))), "symlink-target.json")
				if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, markerPath); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
		},
		{
			name: "directory",
			create: func(t *testing.T, markerPath string) {
				t.Helper()
				if err := os.Mkdir(markerPath, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			baseDir := t.TempDir()
			safetyDir := filepath.Join(baseDir, "safety_backups")
			if err := os.Mkdir(safetyDir, 0o700); err != nil {
				t.Fatal(err)
			}
			journalDir := filepath.Join(safetyDir, restoreJournalDirPrefix+uuid.NewString())
			if err := os.Mkdir(journalDir, 0o700); err != nil {
				t.Fatal(err)
			}
			testCase.create(t, filepath.Join(journalDir, restoreOperationJournalFile))

			assertNonRegularRestoreMarkerRejected(t, filepath.Join(baseDir, "live.db"))
		})
	}
}

func assertNonRegularRestoreMarkerRejected(t *testing.T, databasePath string) {
	t.Helper()
	_, err := discoverRestoreOperations(databasePath)
	if err == nil {
		t.Fatal("expected non-regular restore marker to be rejected")
	}
	if !strings.Contains(err.Error(), "restore operation marker is not a valid regular file") {
		t.Fatalf("unexpected non-regular restore marker error: %v", err)
	}
}

func TestRestorePreparationRejectsSymlinkRootBeforeChangingTargetPermissions(t *testing.T) {
	for _, testCase := range []struct {
		name string
		run  func(string, string) error
	}{
		{
			name: "database safety backup",
			run: func(livePath, _ string) error {
				_, err := (&Server{dbPath: livePath}).prepareSafetyBackup(context.Background(), "test")
				return err
			},
		},
		{
			name: "attachment journal",
			run: func(_ string, safetyRoot string) error {
				storage := &restoreJournalStorage{files: map[string][]byte{}}
				attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
				_, err := prepareAttachmentRollbackJournal(context.Background(), safetyRoot, []attachmentRestoreFile{{TargetKey: "new.bin"}}, attachmentSvc)
				return err
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			baseDir := t.TempDir()
			outside := t.TempDir()
			if err := os.Chmod(outside, 0o755); err != nil {
				t.Fatal(err)
			}
			safetyRoot := filepath.Join(baseDir, "safety_backups")
			if err := os.Symlink(outside, safetyRoot); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}

			if err := testCase.run(filepath.Join(baseDir, "live.db"), safetyRoot); err == nil {
				t.Fatal("expected symlinked restore root to be rejected")
			}
			info, err := os.Stat(outside)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o755 {
				t.Fatalf("symlink target permissions changed to %#o", info.Mode().Perm())
			}
		})
	}
}

func TestRecoverCrossAccountWithoutCommitLedgerRollsBackAttachments(t *testing.T) {
	baseDir := t.TempDir()
	livePath := baseDir + "/live.db"
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	storage := &restoreJournalStorage{files: map[string][]byte{"receipt.bin": []byte("original")}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	operation, err := beginCrossRestorePreparation(livePath, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := prepareAttachmentRollbackJournal(context.Background(), baseDir+"/safety_backups", []attachmentRestoreFile{{TargetKey: "receipt.bin"}}, attachmentSvc, operation)
	if err != nil {
		t.Fatal(err)
	}
	if err := operation.attachJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := operation.markPrepared(); err != nil {
		t.Fatal(err)
	}
	storage.files["receipt.bin"] = []byte("uncommitted replacement")

	if err := RecoverPendingCrossAccountRestores(context.Background(), liveDB, livePath, attachmentSvc); err != nil {
		t.Fatalf("cross-account recovery: %v", err)
	}
	assertRestoreStorageFile(t, storage, "receipt.bin", []byte("original"), true)
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
}

func TestRecoverCommittedCrossAccountWithoutLedgerFailsClosed(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	storage := &restoreJournalStorage{files: map[string][]byte{"receipt.bin": []byte("original")}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	operation, err := beginCrossRestorePreparation(livePath, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := prepareAttachmentRollbackJournal(context.Background(), filepath.Join(baseDir, "safety_backups"), []attachmentRestoreFile{{TargetKey: "receipt.bin"}}, attachmentSvc, operation)
	if err != nil {
		t.Fatal(err)
	}
	if err := operation.attachJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := operation.markPrepared(); err != nil {
		t.Fatal(err)
	}
	if err := operation.markCommitted(); err != nil {
		t.Fatal(err)
	}
	storage.files["receipt.bin"] = []byte("committed replacement")

	err = RecoverPendingCrossAccountRestores(context.Background(), liveDB, livePath, attachmentSvc)
	if err == nil {
		t.Fatal("expected committed marker without ledger to fail closed")
	}
	assertRestoreStorageFile(t, storage, "receipt.bin", []byte("committed replacement"), true)
	if _, statErr := os.Stat(operation.markerPath); statErr != nil {
		t.Fatalf("committed marker was removed after ledger mismatch: %v", statErr)
	}
}

func TestRecoverCrossAccountUsesLedgerWhenExternalPhaseIsPrepared(t *testing.T) {
	baseDir := t.TempDir()
	livePath := baseDir + "/live.db"
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	storage := &restoreJournalStorage{files: map[string][]byte{"receipt.bin": []byte("original")}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	batchID := uuid.NewString()
	operation, err := beginCrossRestorePreparation(livePath, batchID)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := prepareAttachmentRollbackJournal(context.Background(), baseDir+"/safety_backups", []attachmentRestoreFile{{TargetKey: "receipt.bin"}}, attachmentSvc, operation)
	if err != nil {
		t.Fatal(err)
	}
	if err := operation.attachJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := operation.markPrepared(); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDB.Exec(`
		INSERT INTO restore_operations(operation_id, batch_id, kind, created_at)
		VALUES (?, ?, 'cross_merge', 1)
	`, operation.marker.OperationID, batchID); err != nil {
		t.Fatal(err)
	}
	storage.files["receipt.bin"] = []byte("committed replacement")

	if err := RecoverPendingCrossAccountRestores(context.Background(), liveDB, livePath, attachmentSvc); err != nil {
		t.Fatalf("cross-account recovery: %v", err)
	}
	assertRestoreStorageFile(t, storage, "receipt.bin", []byte("committed replacement"), true)
	assertRestoreSafetyDirectoryEmpty(t, baseDir)
}

func TestCrossTerminalCleanupKeepsMarkerUntilJournalIsDurablyClean(t *testing.T) {
	baseDir := t.TempDir()
	livePath := filepath.Join(baseDir, "live.db")
	liveDB := openRestoreTestDatabase(t, livePath, "target", true)
	defer liveDB.Close()

	storage := &restoreJournalStorage{files: map[string][]byte{"receipt.bin": []byte("original")}}
	attachmentSvc := service.NewAttachmentService(nil, nil, storage, nil, service.DefaultAttachmentMaxBytes)
	operation, err := beginCrossRestorePreparation(livePath, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := prepareAttachmentRollbackJournal(context.Background(), filepath.Join(baseDir, "safety_backups"), []attachmentRestoreFile{{TargetKey: "receipt.bin"}}, attachmentSvc, operation)
	if err != nil {
		t.Fatal(err)
	}
	if err := operation.attachJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := operation.markPrepared(); err != nil {
		t.Fatal(err)
	}
	if err := operation.markCommitted(); err != nil {
		t.Fatal(err)
	}

	if err := journal.cleanupPreservingMarker(operation); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(operation.markerPath); err != nil {
		t.Fatalf("terminal marker disappeared before journal cleanup completed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(journal.dir, "manifest.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal manifest still exists after terminal cleanup: %v", err)
	}
	if err := operation.removeMarker(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(journal.dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal directory still exists after final marker removal: %v", err)
	}
}

func prepareReplacementRestoreForTest(t *testing.T, server *Server, livePath string, withJournal bool) (*durableRestoreOperation, string) {
	t.Helper()
	operation, err := beginReplacementRestorePreparation(livePath, withJournal)
	if err != nil {
		t.Fatal(err)
	}
	safetyPath := filepath.Join(filepath.Dir(operation.markerPath), operation.marker.SafetyBackup)
	preparedPath, err := server.prepareSafetyBackup(context.Background(), "restore-test", safetyPath)
	if err != nil {
		t.Fatal(err)
	}
	if preparedPath != safetyPath {
		t.Fatalf("prepared safety path = %q, want planned path %q", preparedPath, safetyPath)
	}
	return operation, safetyPath
}

func writeRawRestoreMarkerForTest(t *testing.T, markerPath string, marker restoreOperationMarker) {
	t.Helper()
	payload, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, append(payload, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func openExistingRestoreTestDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	database, err := openExistingSQLiteReadOnly(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return database
}
