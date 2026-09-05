package apiv1

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"finarch/internal/domain/service"

	"github.com/google/uuid"
)

const (
	restoreOperationMarkerVersion   = 1
	restoreOperationKindReplace     = "replace"
	restoreOperationKindCross       = "cross_merge"
	restoreOperationPhasePreparing  = "preparing"
	restoreOperationPhasePrepared   = "prepared"
	restoreOperationPhaseCommitted  = "committed"
	restoreOperationPhaseRolledBack = "rolled_back"

	restoreOperationRootPrefix   = ".restore-operation-"
	restoreOperationRootSuffix   = ".json"
	restoreOperationJournalFile  = ".restore-operation.json"
	restoreSafetyFilePrefix      = "restore-safety-"
	restoreJournalDirPrefix      = "attachment-rollback-"
	restoreRetainedJournalPrefix = "failed-restore-attachments-"
	restoreRetainedOperationTag  = "-operation-"
	maxRestoreOperationBytes     = 64 << 10
	maxRollbackManifestBytes     = 8 << 20
	maxRollbackBackupBytes       = 100 << 20
	rollbackManifestVersion      = 2
)

// restoreOperationMarker is deliberately stored outside SQLite. For a
// replacement restore it remains readable even when the live database was
// only partly replaced; a matching ledger row in the validated replacement
// resolves an external marker whose final directory sync was ambiguous. For a
// cross-account restore, BatchID is paired with the restore_operations row
// committed in the import transaction.
type restoreOperationMarker struct {
	Version           int    `json:"version"`
	OperationID       string `json:"operation_id"`
	Kind              string `json:"kind"`
	Phase             string `json:"phase"`
	SafetyBackup      string `json:"safety_backup,omitempty"`
	AttachmentJournal string `json:"attachment_journal,omitempty"`
	BatchID           string `json:"batch_id,omitempty"`
}

type durableRestoreOperation struct {
	marker     restoreOperationMarker
	markerPath string
}

type restoreMarkerWriteError struct {
	err       error
	published bool
}

func (err *restoreMarkerWriteError) Error() string { return err.err.Error() }
func (err *restoreMarkerWriteError) Unwrap() error { return err.err }

func restoreMarkerWritePublished(err error) bool {
	var markerErr *restoreMarkerWriteError
	return errors.As(err, &markerErr) && markerErr.published
}

type discoveredRestoreOperation struct {
	marker     restoreOperationMarker
	markerPath string
	safetyDir  string
}

// RecoverPendingRestoresBeforeOpen resolves replacement restores before
// SQLite is opened or migrated.  This is essential because the live database
// may contain only a partial replacement image after a process crash.
func RecoverPendingRestoresBeforeOpen(ctx context.Context, dsn string, attachmentSvc *service.AttachmentService) error {
	databasePath, durable, err := restoreFilesystemPath(dsn)
	if err != nil {
		return fmt.Errorf("resolve restore database path: %w", err)
	}
	if !durable {
		return nil
	}
	operations, err := discoverRestoreOperations(databasePath)
	if err != nil {
		return fmt.Errorf("discover pending restore operations: %w", err)
	}

	var replacements []discoveredRestoreOperation
	for _, operation := range operations {
		if operation.marker.Kind == restoreOperationKindReplace {
			replacements = append(replacements, operation)
		}
	}
	if len(replacements) > 1 {
		return fmt.Errorf("multiple pending replacement restores require manual recovery")
	}
	for _, operation := range replacements {
		if err := recoverReplacementOperation(ctx, databasePath, operation, attachmentSvc); err != nil {
			return fmt.Errorf("recover replacement operation %s: %w", operation.marker.OperationID, err)
		}
	}
	return nil
}

// RecoverPendingCrossAccountRestores resolves external attachment writes once
// SQLite is open, but still before schema migration or request processing.
// The transaction-local restore_operations row is the authoritative commit
// decision; absence means attachment changes must be compensated.
func RecoverPendingCrossAccountRestores(ctx context.Context, database *sql.DB, dsn string, attachmentSvc *service.AttachmentService) error {
	if database == nil {
		return fmt.Errorf("database is nil")
	}
	databasePath, durable, err := restoreFilesystemPath(dsn)
	if err != nil {
		return fmt.Errorf("resolve restore database path: %w", err)
	}
	if !durable {
		return nil
	}
	operations, err := discoverRestoreOperations(databasePath)
	if err != nil {
		return fmt.Errorf("discover pending restore operations: %w", err)
	}
	for _, operation := range operations {
		if operation.marker.Kind == restoreOperationKindReplace {
			return fmt.Errorf("replacement restore marker remains after pre-open recovery")
		}
		switch operation.marker.Phase {
		case restoreOperationPhasePreparing:
			// Preparing is durable before any external attachment write is
			// permitted. Discard only its operation-bound snapshot artifacts;
			// there is deliberately nothing to compensate. Keep the preparing
			// marker at the root until cleanup finishes: a journal-backed terminal
			// marker is only valid once it has been embedded in that journal.
			log.Printf("RESTORE_CRASH_RECOVERY operation_id=%s kind=cross_merge decision=discard_preparation", operation.marker.OperationID)
		case restoreOperationPhaseRolledBack:
			log.Printf("RESTORE_CRASH_RECOVERY operation_id=%s kind=cross_merge decision=rollback_already_complete", operation.marker.OperationID)
		case restoreOperationPhaseCommitted:
			committed, err := crossRestoreOperationCommitted(ctx, database, operation.marker)
			if err != nil {
				return fmt.Errorf("resolve cross-account operation %s: %w", operation.marker.OperationID, err)
			}
			if !committed {
				return fmt.Errorf("committed cross-account marker has no matching durable ledger row")
			}
			log.Printf("RESTORE_CRASH_RECOVERY operation_id=%s kind=cross_merge decision=commit", operation.marker.OperationID)
		case restoreOperationPhasePrepared:
			committed, err := crossRestoreOperationCommitted(ctx, database, operation.marker)
			if err != nil {
				return fmt.Errorf("resolve cross-account operation %s: %w", operation.marker.OperationID, err)
			}
			if !committed {
				journal, err := loadOperationAttachmentJournal(operation, attachmentSvc)
				if err != nil {
					return fmt.Errorf("load cross-account attachment journal: %w", err)
				}
				if journal != nil {
					if err := journal.rollback(ctx); err != nil {
						return fmt.Errorf("roll back cross-account attachments: %w", err)
					}
				}
				if err := updateDiscoveredOperationPhase(&operation, restoreOperationPhaseRolledBack); err != nil {
					return fmt.Errorf("persist cross-account rollback decision: %w", err)
				}
				log.Printf("RESTORE_CRASH_RECOVERY operation_id=%s kind=cross_merge decision=rollback", operation.marker.OperationID)
			} else {
				if err := updateDiscoveredOperationPhase(&operation, restoreOperationPhaseCommitted); err != nil {
					return fmt.Errorf("persist cross-account commit decision: %w", err)
				}
				log.Printf("RESTORE_CRASH_RECOVERY operation_id=%s kind=cross_merge decision=commit", operation.marker.OperationID)
			}
		}
		if err := finishRecoveredOperation(operation); err != nil {
			return err
		}
	}
	return nil
}

func ensureRestoreArtifactRoot(dsn string) (databasePath, safetyDir string, returnErr error) {
	databasePath, durable, err := restoreFilesystemPath(dsn)
	if err != nil {
		return "", "", err
	}
	if !durable {
		return "", "", fmt.Errorf("restore operation requires a filesystem database")
	}
	safetyDir = filepath.Join(filepath.Dir(databasePath), "safety_backups")
	if err := os.MkdirAll(safetyDir, 0o700); err != nil {
		return "", "", fmt.Errorf("create restore artifact directory: %w", pathSafeFilesystemError(err))
	}
	// Validate before chmod so a symlink cannot redirect the permission change.
	if err := validateRestoreArtifactDirectory(safetyDir); err != nil {
		return "", "", fmt.Errorf("validate restore artifact directory: %w", err)
	}
	if err := os.Chmod(safetyDir, 0o700); err != nil {
		return "", "", fmt.Errorf("secure restore artifact directory: %w", pathSafeFilesystemError(err))
	}
	if err := validatePrivateRestoreArtifactDirectory(safetyDir); err != nil {
		return "", "", fmt.Errorf("validate private restore artifact directory: %w", err)
	}
	if err := syncRestoreArtifactDirectory(safetyDir); err != nil {
		return "", "", fmt.Errorf("sync restore artifact directory: %w", err)
	}
	if err := syncRestoreArtifactDirectory(filepath.Dir(safetyDir)); err != nil {
		return "", "", fmt.Errorf("sync restore artifact parent: %w", err)
	}
	return databasePath, safetyDir, nil
}

// beginReplacementRestorePreparation publishes the intended safety filename
// before the file is created. A crash can therefore never leave an
// unreferenced database snapshot, and preparing recovery may discard the
// operation-bound partial file because live database mutation is forbidden
// until markPrepared succeeds.
func beginReplacementRestorePreparation(dsn string, withAttachmentJournal ...bool) (*durableRestoreOperation, error) {
	if len(withAttachmentJournal) > 1 {
		return nil, fmt.Errorf("multiple replacement attachment-journal options")
	}
	_, safetyDir, err := ensureRestoreArtifactRoot(dsn)
	if err != nil {
		return nil, err
	}
	operationID := uuid.NewString()
	operation := &durableRestoreOperation{
		marker: restoreOperationMarker{
			Version:      restoreOperationMarkerVersion,
			OperationID:  operationID,
			Kind:         restoreOperationKindReplace,
			Phase:        restoreOperationPhasePreparing,
			SafetyBackup: restoreSafetyFilePrefix + operationID + ".db",
		},
		markerPath: filepath.Join(safetyDir, restoreOperationRootPrefix+operationID+restoreOperationRootSuffix),
	}
	if len(withAttachmentJournal) == 1 && withAttachmentJournal[0] {
		operation.marker.AttachmentJournal = restoreJournalDirPrefix + operationID
	}
	if err := writeDurableRestoreMarker(operation.markerPath, operation.marker); err != nil {
		return operation, err
	}
	return operation, nil
}

// beginCrossRestorePreparation publishes an operation-bound journal name
// before snapshotting any live attachment bytes.
func beginCrossRestorePreparation(dsn, batchID string) (*durableRestoreOperation, error) {
	if _, err := uuid.Parse(batchID); err != nil {
		return nil, fmt.Errorf("invalid restore batch ID")
	}
	_, safetyDir, err := ensureRestoreArtifactRoot(dsn)
	if err != nil {
		return nil, err
	}
	operationID := uuid.NewString()
	operation := &durableRestoreOperation{
		marker: restoreOperationMarker{
			Version:           restoreOperationMarkerVersion,
			OperationID:       operationID,
			Kind:              restoreOperationKindCross,
			Phase:             restoreOperationPhasePreparing,
			AttachmentJournal: restoreJournalDirPrefix + operationID,
			BatchID:           batchID,
		},
		markerPath: filepath.Join(safetyDir, restoreOperationRootPrefix+operationID+restoreOperationRootSuffix),
	}
	if err := writeDurableRestoreMarker(operation.markerPath, operation.marker); err != nil {
		return operation, err
	}
	return operation, nil
}

// attachJournal publishes the journal reference before any attachment is
// overwritten, then moves the marker into the journal.  Retaining/renaming the
// journal later therefore cannot separate it from its recovery decision.
func (operation *durableRestoreOperation) attachJournal(journal *attachmentRollbackJournal) error {
	if operation == nil || journal == nil || strings.TrimSpace(journal.dir) == "" {
		return nil
	}
	operation.marker.AttachmentJournal = filepath.Base(journal.dir)
	if err := writeDurableRestoreMarker(operation.markerPath, operation.marker); err != nil {
		return err
	}
	target := filepath.Join(journal.dir, restoreOperationJournalFile)
	if filepath.Clean(target) == filepath.Clean(operation.markerPath) {
		return nil
	}
	sourceDir := filepath.Dir(operation.markerPath)
	if err := os.Rename(operation.markerPath, target); err != nil {
		return fmt.Errorf("move restore marker into attachment journal: %w", pathSafeFilesystemError(err))
	}
	operation.markerPath = target
	if err := syncRestoreArtifactDirectory(journal.dir); err != nil {
		return fmt.Errorf("sync attachment journal marker: %w", err)
	}
	if err := syncRestoreArtifactDirectory(sourceDir); err != nil {
		return fmt.Errorf("sync restore marker directory: %w", err)
	}
	return nil
}

func (operation *durableRestoreOperation) markCommitted() error {
	return operation.setPhase(restoreOperationPhaseCommitted)
}

func (operation *durableRestoreOperation) markPrepared() error {
	return operation.setPhase(restoreOperationPhasePrepared)
}

func (operation *durableRestoreOperation) markRolledBack() error {
	return operation.setPhase(restoreOperationPhaseRolledBack)
}

func (operation *durableRestoreOperation) setPhase(phase string) error {
	if operation == nil || operation.markerPath == "" {
		return nil
	}
	previousPhase := operation.marker.Phase
	operation.marker.Phase = phase
	if err := writeDurableRestoreMarker(operation.markerPath, operation.marker); err != nil {
		if !restoreMarkerWritePublished(err) {
			operation.marker.Phase = previousPhase
		}
		return err
	}
	return nil
}

func (operation *durableRestoreOperation) removeMarker() error {
	if operation == nil || operation.markerPath == "" {
		return nil
	}
	markerPath := operation.markerPath
	embeddedInJournal := filepath.Base(markerPath) == restoreOperationJournalFile
	markerDir := filepath.Dir(markerPath)
	if err := os.Remove(markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove restore operation marker: %w", pathSafeFilesystemError(err))
	}
	if err := syncRestoreArtifactDirectory(markerDir); err != nil {
		return fmt.Errorf("sync restore operation marker removal: %w", err)
	}
	operation.markerPath = ""
	if embeddedInJournal {
		if err := os.Remove(markerDir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove empty attachment journal: %w", pathSafeFilesystemError(err))
		}
		if err := syncRestoreArtifactDirectory(filepath.Dir(markerDir)); err != nil {
			return fmt.Errorf("sync attachment journal removal: %w", err)
		}
	}
	return nil
}

func (journal *attachmentRollbackJournal) cleanupPreservingMarker(operation *durableRestoreOperation) error {
	if journal == nil || strings.TrimSpace(journal.dir) == "" {
		return nil
	}
	if operation == nil || operation.markerPath == "" || filepath.Clean(filepath.Dir(operation.markerPath)) != filepath.Clean(journal.dir) {
		return journal.cleanup()
	}
	return removeRestoreArtifactDirectoryPreserving(journal.dir, operation.markerPath)
}

func writeDurableRestoreMarker(path string, marker restoreOperationMarker) (returnErr error) {
	if err := validateRestoreOperationMarker(marker); err != nil {
		return err
	}
	if err := validateRestoreOperationMarkerLocation(marker, path, restoreMarkerSafetyDirectory(path)); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return fmt.Errorf("encode restore operation marker: %w", err)
	}
	payload = append(payload, '\n')
	dir := filepath.Dir(path)
	if err := validatePrivateRestoreArtifactDirectory(dir); err != nil {
		return fmt.Errorf("validate restore operation directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".restore-operation-*.tmp")
	if err != nil {
		return fmt.Errorf("create restore operation marker: %w", pathSafeFilesystemError(err))
	}
	tmpPath := tmp.Name()
	tmpOpen := true
	defer func() {
		var closeErr error
		if tmpOpen {
			closeErr = tmp.Close()
		}
		removeErr := os.Remove(tmpPath)
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		returnErr = errors.Join(returnErr, closeErr, removeErr)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("secure restore operation marker: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		return fmt.Errorf("write restore operation marker: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync restore operation marker: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close restore operation marker: %w", err)
	}
	tmpOpen = false
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish restore operation marker: %w", pathSafeFilesystemError(err))
	}
	if err := syncRestoreArtifactDirectory(dir); err != nil {
		return &restoreMarkerWriteError{
			published: true,
			err:       fmt.Errorf("sync restore operation directory: %w", err),
		}
	}
	return nil
}

func discoverRestoreOperations(databasePath string) ([]discoveredRestoreOperation, error) {
	safetyDir := filepath.Join(filepath.Dir(databasePath), "safety_backups")
	if err := validatePrivateRestoreArtifactDirectory(safetyDir); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(safetyDir)
	if err != nil {
		return nil, pathSafeFilesystemError(err)
	}
	var markerPaths []string
	for _, entry := range entries {
		name := entry.Name()
		switch {
		case strings.HasPrefix(name, restoreOperationRootPrefix) && strings.HasSuffix(name, restoreOperationRootSuffix):
			markerPaths = append(markerPaths, filepath.Join(safetyDir, name))
		case entry.IsDir():
			journalDir := filepath.Join(safetyDir, name)
			if err := validatePrivateRestoreArtifactDirectory(journalDir); err != nil {
				return nil, fmt.Errorf("validate attachment rollback journal %s: %w", name, err)
			}
			candidate := filepath.Join(journalDir, restoreOperationJournalFile)
			_, statErr := os.Lstat(candidate)
			if statErr == nil {
				markerPaths = append(markerPaths, candidate)
			} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
				return nil, pathSafeFilesystemError(statErr)
			}
		}
	}
	sort.Strings(markerPaths)
	operations := make([]discoveredRestoreOperation, 0, len(markerPaths))
	seen := make(map[string]struct{}, len(markerPaths))
	for _, markerPath := range markerPaths {
		marker, err := readRestoreOperationMarker(markerPath)
		if err != nil {
			return nil, fmt.Errorf("read marker %s: %w", filepath.Base(markerPath), err)
		}
		if err := validateRestoreOperationMarkerLocation(marker, markerPath, safetyDir); err != nil {
			return nil, fmt.Errorf("validate marker %s location: %w", filepath.Base(markerPath), err)
		}
		if _, duplicate := seen[marker.OperationID]; duplicate {
			return nil, fmt.Errorf("duplicate restore operation marker %s", marker.OperationID)
		}
		seen[marker.OperationID] = struct{}{}
		operations = append(operations, discoveredRestoreOperation{marker: marker, markerPath: markerPath, safetyDir: safetyDir})
	}
	return operations, nil
}

func readRestoreOperationMarker(path string) (restoreOperationMarker, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return restoreOperationMarker{}, pathSafeFilesystemError(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Size() <= 0 || info.Size() > maxRestoreOperationBytes {
		return restoreOperationMarker{}, fmt.Errorf("restore operation marker is not a valid regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return restoreOperationMarker{}, pathSafeFilesystemError(err)
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, maxRestoreOperationBytes+1))
	decoder.DisallowUnknownFields()
	var marker restoreOperationMarker
	if err := decoder.Decode(&marker); err != nil {
		return restoreOperationMarker{}, fmt.Errorf("decode restore operation marker: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return restoreOperationMarker{}, fmt.Errorf("restore operation marker has trailing data")
	}
	if err := validateRestoreOperationMarker(marker); err != nil {
		return restoreOperationMarker{}, err
	}
	return marker, nil
}

func validateRestoreOperationMarker(marker restoreOperationMarker) error {
	if marker.Version != restoreOperationMarkerVersion {
		return fmt.Errorf("unsupported restore operation marker version %d", marker.Version)
	}
	if !isCanonicalRestoreUUID(marker.OperationID) {
		return fmt.Errorf("invalid restore operation ID")
	}
	if marker.Phase != restoreOperationPhasePreparing && marker.Phase != restoreOperationPhasePrepared && marker.Phase != restoreOperationPhaseCommitted && marker.Phase != restoreOperationPhaseRolledBack {
		return fmt.Errorf("invalid restore operation phase")
	}
	if marker.AttachmentJournal != "" && !safeRestoreArtifactBase(marker.AttachmentJournal) {
		return fmt.Errorf("invalid attachment journal name")
	}
	switch marker.Kind {
	case restoreOperationKindReplace:
		if marker.SafetyBackup != restoreSafetyFilePrefix+marker.OperationID+".db" || marker.BatchID != "" {
			return fmt.Errorf("invalid replacement restore marker")
		}
		if marker.AttachmentJournal != "" && marker.AttachmentJournal != restoreJournalDirPrefix+marker.OperationID {
			return fmt.Errorf("replacement attachment journal is not operation-bound")
		}
	case restoreOperationKindCross:
		if marker.SafetyBackup != "" || marker.AttachmentJournal != restoreJournalDirPrefix+marker.OperationID {
			return fmt.Errorf("invalid cross-account restore marker")
		}
		if !isCanonicalRestoreUUID(marker.BatchID) {
			return fmt.Errorf("invalid cross-account restore batch ID")
		}
	default:
		return fmt.Errorf("invalid restore operation kind")
	}
	return nil
}

func isCanonicalRestoreUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

func restoreMarkerSafetyDirectory(markerPath string) string {
	markerPath = filepath.Clean(markerPath)
	if filepath.Base(markerPath) == restoreOperationJournalFile {
		return filepath.Dir(filepath.Dir(markerPath))
	}
	return filepath.Dir(markerPath)
}

func validateRestoreOperationMarkerLocation(marker restoreOperationMarker, markerPath, safetyDir string) error {
	markerPath = filepath.Clean(markerPath)
	safetyDir = filepath.Clean(safetyDir)
	markerDir := filepath.Dir(markerPath)
	if markerDir == safetyDir {
		expected := restoreOperationRootPrefix + marker.OperationID + restoreOperationRootSuffix
		if filepath.Base(markerPath) != expected {
			return fmt.Errorf("root restore marker filename is not bound to its operation ID")
		}
		if marker.AttachmentJournal != "" && marker.Phase != restoreOperationPhasePreparing {
			return fmt.Errorf("journal-backed restore marker is outside its attachment journal")
		}
		return nil
	}

	if filepath.Base(markerPath) != restoreOperationJournalFile || filepath.Dir(markerDir) != safetyDir {
		return fmt.Errorf("embedded restore marker is outside the restore artifact directory")
	}
	if marker.AttachmentJournal == "" {
		return fmt.Errorf("restore marker without an attachment journal is embedded in a journal")
	}
	actualJournal := filepath.Base(markerDir)
	if actualJournal != marker.AttachmentJournal && !validRetainedRestoreJournalName(actualJournal, marker.OperationID) {
		return fmt.Errorf("embedded restore marker journal is not bound to its operation ID")
	}
	return nil
}

func retainedRestoreJournalName(restoreID, operationID string) string {
	return fmt.Sprintf("%s%s%s%s-%s", restoreRetainedJournalPrefix, safeRestoreArtifactID(restoreID), restoreRetainedOperationTag, operationID, uuid.NewString())
}

func validRetainedRestoreJournalName(name, operationID string) bool {
	if !safeRestoreArtifactBase(name) || !isCanonicalRestoreUUID(operationID) || !strings.HasPrefix(name, restoreRetainedJournalPrefix) {
		return false
	}
	const uuidTextBytes = 36
	if len(name) <= uuidTextBytes || name[len(name)-uuidTextBytes-1] != '-' {
		return false
	}
	retentionID := name[len(name)-uuidTextBytes:]
	if !isCanonicalRestoreUUID(retentionID) {
		return false
	}
	head := name[:len(name)-uuidTextBytes-1]
	operationSuffix := restoreRetainedOperationTag + operationID
	if !strings.HasSuffix(head, operationSuffix) {
		return false
	}
	restoreID := strings.TrimSuffix(strings.TrimPrefix(head, restoreRetainedJournalPrefix), operationSuffix)
	return restoreID != "" && safeRestoreArtifactID(restoreID) == restoreID
}

func safeRestoreArtifactBase(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name &&
		!strings.ContainsAny(name, `/\\`) && !strings.ContainsRune(name, 0)
}

func recoverReplacementOperation(ctx context.Context, databasePath string, operation discoveredRestoreOperation, attachmentSvc *service.AttachmentService) error {
	switch operation.marker.Phase {
	case restoreOperationPhasePreparing:
		// The preparing marker is durable before snapshot creation and live
		// database mutation is forbidden until the complete snapshot has been
		// fsynced and this marker advances to prepared. Keep it in preparing
		// while deleting the planned artifacts so it remains valid at the root.
		log.Printf("RESTORE_CRASH_RECOVERY operation_id=%s kind=replace decision=discard_preparation", operation.marker.OperationID)
	case restoreOperationPhasePrepared:
		// A marker rename can reach the filesystem while its directory fsync
		// reports an error. The ledger row is written and checkpointed first, so
		// it is the authoritative tie-breaker when the old prepared marker is
		// what survives a crash.
		committed, err := replacementRestoreOperationCommitted(ctx, databasePath, operation.marker)
		if err != nil {
			return fmt.Errorf("resolve replacement commit ledger: %w", err)
		}
		if committed {
			if err := validateCommittedReplacement(ctx, databasePath); err != nil {
				return err
			}
			if err := updateDiscoveredOperationPhase(&operation, restoreOperationPhaseCommitted); err != nil {
				return fmt.Errorf("persist replacement commit decision: %w", err)
			}
			log.Printf("RESTORE_CRASH_RECOVERY operation_id=%s kind=replace decision=commit_from_ledger", operation.marker.OperationID)
			break
		}
		// Fully resolve and validate the external compensation journal before the
		// first database mutation. Otherwise a malformed/missing journal could
		// leave the database rolled back while attachment bytes remain restored.
		journal, err := loadOperationAttachmentJournal(operation, attachmentSvc)
		if err != nil {
			return err
		}
		safetyPath := filepath.Join(operation.safetyDir, operation.marker.SafetyBackup)
		if err := restoreDatabaseFileFromSafety(ctx, databasePath, safetyPath); err != nil {
			return err
		}
		if journal != nil {
			if err := journal.rollback(ctx); err != nil {
				return fmt.Errorf("roll back replacement attachments: %w", err)
			}
		}
		if err := updateDiscoveredOperationPhase(&operation, restoreOperationPhaseRolledBack); err != nil {
			return fmt.Errorf("persist replacement rollback decision: %w", err)
		}
		log.Printf("RESTORE_CRASH_RECOVERY operation_id=%s kind=replace decision=rollback", operation.marker.OperationID)
	case restoreOperationPhaseCommitted:
		if err := validateCommittedReplacement(ctx, databasePath); err != nil {
			return err
		}
		log.Printf("RESTORE_CRASH_RECOVERY operation_id=%s kind=replace decision=commit", operation.marker.OperationID)
	case restoreOperationPhaseRolledBack:
		log.Printf("RESTORE_CRASH_RECOVERY operation_id=%s kind=replace decision=rollback_already_complete", operation.marker.OperationID)
	}
	return finishRecoveredOperation(operation)
}

func validateCommittedReplacement(ctx context.Context, databasePath string) error {
	liveDB, err := openExistingSQLiteReadOnly(ctx, databasePath)
	if err != nil {
		return fmt.Errorf("open committed restored database: %w", pathSafeFilesystemError(err))
	}
	integrityErr := ensureSQLiteIntegrity(ctx, liveDB)
	closeErr := liveDB.Close()
	if integrityErr != nil || closeErr != nil {
		return fmt.Errorf("validate committed restored database: %w", errors.Join(integrityErr, closeErr))
	}
	return nil
}

func replacementRestoreOperationCommitted(ctx context.Context, databasePath string, marker restoreOperationMarker) (bool, error) {
	liveDB, err := openExistingSQLiteReadOnly(ctx, databasePath)
	if err != nil {
		return false, pathSafeFilesystemError(err)
	}
	committed, ledgerErr := restoreOperationLedgerCommitted(ctx, liveDB, marker.OperationID, marker.OperationID, restoreOperationKindReplace)
	closeErr := liveDB.Close()
	if ledgerErr != nil || closeErr != nil {
		return false, errors.Join(ledgerErr, closeErr)
	}
	return committed, nil
}

func updateDiscoveredOperationPhase(operation *discoveredRestoreOperation, phase string) error {
	if operation.marker.Phase == phase {
		return nil
	}
	previousPhase := operation.marker.Phase
	operation.marker.Phase = phase
	if err := writeDurableRestoreMarker(operation.markerPath, operation.marker); err != nil {
		operation.marker.Phase = previousPhase
		return err
	}
	return nil
}

func crossRestoreOperationCommitted(ctx context.Context, database *sql.DB, marker restoreOperationMarker) (bool, error) {
	return restoreOperationLedgerCommitted(ctx, database, marker.OperationID, marker.BatchID, restoreOperationKindCross)
}

func restoreOperationLedgerCommitted(ctx context.Context, database *sql.DB, operationID, batchID, kind string) (bool, error) {
	var tableExists int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM sqlite_master
		WHERE type = 'table' AND name = 'restore_operations'
	`).Scan(&tableExists); err != nil {
		return false, err
	}
	if tableExists == 0 {
		return false, nil
	}
	var committed int
	if err := database.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM restore_operations
			WHERE operation_id = ? AND batch_id = ? AND kind = ?
		)
	`, operationID, batchID, kind).Scan(&committed); err != nil {
		return false, err
	}
	return committed == 1, nil
}

// ensureRestoreDatabaseDurable establishes an explicit durability boundary
// between SQLite state and external restore artifacts. All pooled connections
// use WAL+synchronous=FULL; this explicit FULL checkpoint additionally proves
// that every preceding WAL frame reached the database before a terminal marker
// or rollback journal may be removed.
func ensureRestoreDatabaseDurable(ctx context.Context, database *sql.DB, dsn string) (returnErr error) {
	if database == nil {
		return fmt.Errorf("database is nil")
	}
	databasePath, durable, err := restoreFilesystemPath(dsn)
	if err != nil {
		return err
	}
	if !durable {
		return fmt.Errorf("restore durability requires a filesystem database")
	}
	conn, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire restore durability connection: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, conn.Close()) }()

	var previousSynchronous int
	if err := conn.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&previousSynchronous); err != nil {
		return fmt.Errorf("read restore durability level: %w", err)
	}
	if previousSynchronous < 0 || previousSynchronous > 3 {
		return fmt.Errorf("unsupported SQLite synchronous level %d", previousSynchronous)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA synchronous = FULL`); err != nil {
		return fmt.Errorf("enable full restore durability: %w", err)
	}
	defer func() {
		restoreCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := conn.ExecContext(restoreCtx, fmt.Sprintf("PRAGMA synchronous = %d", previousSynchronous))
		if err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("restore SQLite durability level: %w", err))
		}
	}()

	var busy, logFrames, checkpointedFrames int
	if err := conn.QueryRowContext(ctx, `PRAGMA wal_checkpoint(FULL)`).Scan(&busy, &logFrames, &checkpointedFrames); err != nil {
		return fmt.Errorf("checkpoint restore database: %w", err)
	}
	if busy != 0 || logFrames != checkpointedFrames {
		return fmt.Errorf("restore database checkpoint incomplete (busy=%d log=%d checkpointed=%d)", busy, logFrames, checkpointedFrames)
	}
	if err := syncRestoreDatabaseFiles(databasePath); err != nil {
		return fmt.Errorf("sync restore database files: %w", err)
	}
	return nil
}

func syncRestoreDatabaseFiles(databasePath string) error {
	for _, path := range []string{databasePath + "-wal", databasePath} {
		file, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) && path != databasePath {
			continue
		}
		if err != nil {
			return pathSafeFilesystemError(err)
		}
		info, statErr := file.Stat()
		if statErr == nil && !info.Mode().IsRegular() {
			statErr = fmt.Errorf("restore database artifact is not a regular file")
		}
		syncErr := file.Sync()
		closeErr := file.Close()
		if statErr != nil || syncErr != nil || closeErr != nil {
			return errors.Join(pathSafeFilesystemError(statErr), syncErr, closeErr)
		}
	}
	return syncRestoreArtifactDirectory(filepath.Dir(databasePath))
}

func loadOperationAttachmentJournal(operation discoveredRestoreOperation, attachmentSvc *service.AttachmentService) (*attachmentRollbackJournal, error) {
	if operation.marker.AttachmentJournal == "" {
		return nil, nil
	}
	if attachmentSvc == nil {
		return nil, fmt.Errorf("attachment storage is required for pending restore recovery")
	}
	journalDir := filepath.Join(operation.safetyDir, operation.marker.AttachmentJournal)
	markerDir := filepath.Dir(operation.markerPath)
	if markerDir != operation.safetyDir {
		if filepath.Dir(markerDir) != operation.safetyDir {
			return nil, fmt.Errorf("restore marker is outside the artifact directory")
		}
		// A retained journal is renamed together with its embedded marker.  Its
		// current parent is therefore stronger evidence than the old JSON name.
		journalDir = markerDir
	}
	return loadAttachmentRollbackJournal(journalDir, attachmentSvc)
}

func loadAttachmentRollbackJournal(dir string, attachmentSvc *service.AttachmentService) (*attachmentRollbackJournal, error) {
	if err := validatePrivateRestoreArtifactDirectory(dir); err != nil {
		return nil, fmt.Errorf("validate attachment rollback journal: %w", err)
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil {
		return nil, pathSafeFilesystemError(err)
	}
	if !manifestInfo.Mode().IsRegular() || manifestInfo.Mode().Perm()&0o022 != 0 || manifestInfo.Size() <= 0 || manifestInfo.Size() > maxRollbackManifestBytes {
		return nil, fmt.Errorf("attachment rollback manifest is invalid")
	}
	f, err := os.Open(manifestPath)
	if err != nil {
		return nil, pathSafeFilesystemError(err)
	}
	decoder := json.NewDecoder(io.LimitReader(f, maxRollbackManifestBytes+1))
	decoder.DisallowUnknownFields()
	var manifest attachmentRollbackManifest
	decodeErr := decoder.Decode(&manifest)
	var trailingErr error
	if decodeErr == nil {
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			trailingErr = fmt.Errorf("attachment rollback manifest has trailing data")
		}
	}
	closeErr := f.Close()
	if decodeErr != nil || trailingErr != nil || closeErr != nil {
		return nil, fmt.Errorf("read attachment rollback manifest: %w", errors.Join(decodeErr, trailingErr, closeErr))
	}
	if manifest.Version != rollbackManifestVersion {
		return nil, fmt.Errorf("unsupported attachment rollback manifest version %d", manifest.Version)
	}
	journal := &attachmentRollbackJournal{dir: dir, attachmentSvc: attachmentSvc, entries: make([]attachmentRollbackEntry, 0, len(manifest.Entries))}
	seen := make(map[string]struct{}, len(manifest.Entries))
	seenBackupFiles := make(map[string]struct{}, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if strings.TrimSpace(entry.StorageKey) == "" {
			return nil, fmt.Errorf("attachment rollback manifest contains an empty storage key")
		}
		if _, duplicate := seen[entry.StorageKey]; duplicate {
			return nil, fmt.Errorf("attachment rollback manifest contains duplicate storage keys")
		}
		seen[entry.StorageKey] = struct{}{}
		rollbackEntry := attachmentRollbackEntry{
			storageKey:     entry.StorageKey,
			existed:        entry.Existed,
			expectedSize:   entry.SizeBytes,
			expectedSHA256: entry.SHA256,
		}
		if entry.Existed {
			if !safeRestoreArtifactBase(entry.BackupFile) {
				return nil, fmt.Errorf("attachment rollback manifest contains an invalid backup filename")
			}
			if _, duplicate := seenBackupFiles[entry.BackupFile]; duplicate {
				return nil, fmt.Errorf("attachment rollback manifest contains duplicate backup filenames")
			}
			seenBackupFiles[entry.BackupFile] = struct{}{}
			if entry.SizeBytes < 0 || entry.SizeBytes > maxRollbackBackupBytes {
				return nil, fmt.Errorf("attachment rollback manifest contains an invalid backup size")
			}
			if !validRollbackSHA256(entry.SHA256) {
				return nil, fmt.Errorf("attachment rollback manifest contains an invalid backup digest")
			}
			backupPath := filepath.Join(dir, entry.BackupFile)
			backupInfo, err := os.Lstat(backupPath)
			if err != nil {
				return nil, pathSafeFilesystemError(err)
			}
			if !backupInfo.Mode().IsRegular() || backupInfo.Mode().Perm()&0o022 != 0 || backupInfo.Size() != entry.SizeBytes {
				return nil, fmt.Errorf("attachment rollback entry is not a regular file")
			}
			backup, err := os.Open(backupPath)
			if err != nil {
				return nil, pathSafeFilesystemError(err)
			}
			verifyErr := verifyAttachmentRollbackBackup(backup, entry.SizeBytes, entry.SHA256)
			closeErr := backup.Close()
			if verifyErr != nil || closeErr != nil {
				return nil, fmt.Errorf("verify attachment rollback entry %q: %w", entry.StorageKey, errors.Join(verifyErr, closeErr))
			}
			rollbackEntry.backupPath = backupPath
		} else if entry.BackupFile != "" || entry.SizeBytes != 0 || entry.SHA256 != "" {
			return nil, fmt.Errorf("new attachment rollback entry unexpectedly has backup metadata")
		}
		journal.entries = append(journal.entries, rollbackEntry)
	}
	return journal, nil
}

func finishRecoveredOperation(operation discoveredRestoreOperation) error {
	if operation.marker.Phase == restoreOperationPhasePrepared {
		return fmt.Errorf("refusing to clean artifacts for an unresolved restore operation")
	}
	if operation.marker.AttachmentJournal != "" {
		journalDir := filepath.Join(operation.safetyDir, operation.marker.AttachmentJournal)
		if markerDir := filepath.Dir(operation.markerPath); markerDir != operation.safetyDir {
			journalDir = markerDir
		}
		var cleanupErr error
		if filepath.Clean(filepath.Dir(operation.markerPath)) == filepath.Clean(journalDir) {
			cleanupErr = removeRestoreArtifactDirectoryPreserving(journalDir, operation.markerPath)
		} else {
			cleanupErr = removeRestoreArtifactTree(journalDir)
		}
		if cleanupErr != nil {
			return fmt.Errorf("cleanup attachment rollback journal: %w", cleanupErr)
		}
	}
	if operation.marker.SafetyBackup != "" {
		safetyPath := filepath.Join(operation.safetyDir, operation.marker.SafetyBackup)
		if err := removeSQLiteFiles(safetyPath); err != nil {
			return fmt.Errorf("cleanup safety backup: %w", err)
		}
		if err := syncRestoreArtifactDirectory(operation.safetyDir); err != nil {
			return fmt.Errorf("sync safety backup cleanup: %w", err)
		}
	}
	if err := removeDurableRestoreMarker(operation.markerPath); err != nil {
		return err
	}
	if filepath.Base(operation.markerPath) == restoreOperationJournalFile {
		journalDir := filepath.Dir(operation.markerPath)
		if err := os.Remove(journalDir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove empty attachment journal: %w", pathSafeFilesystemError(err))
		} else if err := syncRestoreArtifactDirectory(filepath.Dir(journalDir)); err != nil {
			return fmt.Errorf("sync attachment journal removal: %w", err)
		}
	}
	return nil
}

func removeDurableRestoreMarker(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove restore operation marker: %w", pathSafeFilesystemError(err))
	}
	if err := syncRestoreArtifactDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync restore operation marker removal: %w", err)
	}
	return nil
}

func restoreDatabaseFileFromSafety(ctx context.Context, targetPath, safetyPath string) (returnErr error) {
	safetyInfo, err := os.Lstat(safetyPath)
	if err != nil {
		return pathSafeFilesystemError(err)
	}
	if !safetyInfo.Mode().IsRegular() || safetyInfo.Mode().Perm()&0o022 != 0 || safetyInfo.Size() <= 0 {
		return fmt.Errorf("safety backup is not a non-empty regular file")
	}
	safetyDB, err := openExistingSQLiteReadOnly(ctx, safetyPath)
	if err != nil {
		return fmt.Errorf("open safety backup: %w", pathSafeFilesystemError(err))
	}
	integrityErr := ensureSQLiteIntegrity(ctx, safetyDB)
	closeErr := safetyDB.Close()
	if integrityErr != nil || closeErr != nil {
		return fmt.Errorf("validate safety backup: %w", errors.Join(integrityErr, closeErr))
	}

	targetDir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(targetDir, ".restore-recovery-*.db")
	if err != nil {
		return fmt.Errorf("create recovered database: %w", pathSafeFilesystemError(err))
	}
	tmpPath := tmp.Name()
	tmpOpen := true
	defer func() {
		var closeErr error
		if tmpOpen {
			closeErr = tmp.Close()
		}
		removeErr := os.Remove(tmpPath)
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		returnErr = errors.Join(returnErr, closeErr, removeErr)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	source, err := os.Open(safetyPath)
	if err != nil {
		return pathSafeFilesystemError(err)
	}
	copied, copyErr := io.CopyBuffer(tmp, source, make([]byte, 1024*1024))
	sourceCloseErr := source.Close()
	if copyErr != nil || sourceCloseErr != nil {
		return fmt.Errorf("copy safety backup: %w", errors.Join(copyErr, sourceCloseErr))
	}
	if copied != safetyInfo.Size() {
		return fmt.Errorf("safety backup changed while being copied")
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync recovered database: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close recovered database: %w", err)
	}
	tmpOpen = false
	if err := removeSQLiteSidecars(targetPath); err != nil {
		return fmt.Errorf("remove stale SQLite sidecars: %w", err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("activate recovered database: %w", pathSafeFilesystemError(err))
	}
	if err := syncRestoreArtifactDirectory(targetDir); err != nil {
		return fmt.Errorf("sync recovered database directory: %w", err)
	}
	recoveredDB, err := openExistingSQLiteReadOnly(ctx, targetPath)
	if err != nil {
		return fmt.Errorf("open recovered database: %w", pathSafeFilesystemError(err))
	}
	integrityErr = ensureSQLiteIntegrity(ctx, recoveredDB)
	closeErr = recoveredDB.Close()
	if integrityErr != nil || closeErr != nil {
		return fmt.Errorf("validate recovered database: %w", errors.Join(integrityErr, closeErr))
	}
	return nil
}

func removeRestoreArtifactTree(path string) error {
	if err := validatePrivateRestoreArtifactDirectory(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	parent := filepath.Dir(path)
	entries, err := os.ReadDir(path)
	if err != nil {
		return pathSafeFilesystemError(err)
	}
	for _, entry := range entries {
		// Journals contain files only. Avoid recursive deletion: an unexpected
		// directory or mount must fail closed instead of becoming a traversal
		// primitive if an artifact path is swapped or tampered with.
		candidate := filepath.Join(path, entry.Name())
		info, err := os.Lstat(candidate)
		if err != nil {
			return pathSafeFilesystemError(err)
		}
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("restore artifact contains an unexpected directory")
		}
		if err := os.Remove(candidate); err != nil {
			return pathSafeFilesystemError(err)
		}
	}
	if err := syncRestoreArtifactDirectory(path); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return pathSafeFilesystemError(err)
	}
	return syncRestoreArtifactDirectory(parent)
}

func removeRestoreArtifactDirectoryPreserving(path, preservePath string) error {
	if err := validatePrivateRestoreArtifactDirectory(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	path = filepath.Clean(path)
	preservePath = filepath.Clean(preservePath)
	if filepath.Dir(preservePath) != path {
		return fmt.Errorf("preserved restore artifact is outside its journal")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return pathSafeFilesystemError(err)
	}
	for _, entry := range entries {
		candidate := filepath.Join(path, entry.Name())
		if filepath.Clean(candidate) == preservePath {
			continue
		}
		info, err := os.Lstat(candidate)
		if err != nil {
			return pathSafeFilesystemError(err)
		}
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("restore artifact contains an unexpected directory")
		}
		if err := os.Remove(candidate); err != nil {
			return pathSafeFilesystemError(err)
		}
	}
	return syncRestoreArtifactDirectory(path)
}

func validateRestoreArtifactDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return pathSafeFilesystemError(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("restore artifact path is not a real directory")
	}
	return nil
}

func validatePrivateRestoreArtifactDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return pathSafeFilesystemError(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("restore artifact path is not a real directory")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("restore artifact directory is group- or world-writable")
	}
	return nil
}

func syncRestoreArtifactDirectory(path string) error {
	if err := validateRestoreArtifactDirectory(path); err != nil {
		return err
	}
	dir, err := os.Open(path)
	if err != nil {
		return pathSafeFilesystemError(err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil && runtime.GOOS != "windows" {
		return err
	}
	return nil
}

// restoreFilesystemPath resolves the file-backed subset of SQLite DSNs.  An
// in-memory DSN has no durable artifacts and therefore needs no crash recovery.
func restoreFilesystemPath(dsn string) (string, bool, error) {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return "", false, fmt.Errorf("database path is empty")
	}
	base, rawQuery, _ := strings.Cut(trimmed, "?")
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", false, fmt.Errorf("invalid SQLite DSN query: %w", err)
	}
	if base == ":memory:" || strings.EqualFold(query.Get("mode"), "memory") {
		return "", false, nil
	}
	path := base
	if strings.HasPrefix(strings.ToLower(base), "file:") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", false, err
		}
		if parsed.Host != "" && parsed.Host != "localhost" {
			return "", false, fmt.Errorf("remote SQLite file authorities are unsupported")
		}
		path = parsed.Path
		if parsed.Opaque != "" {
			path = parsed.Opaque
		}
		if path == ":memory:" {
			return "", false, nil
		}
	}
	if strings.TrimSpace(path) == "" {
		return "", false, fmt.Errorf("database path is empty")
	}
	abs, err := filepath.Abs(filepath.FromSlash(path))
	if err != nil {
		return "", false, err
	}
	return filepath.Clean(abs), true, nil
}

func restoreSafetyArtifactDirectory(dsn string) (string, error) {
	databasePath, durable, err := restoreFilesystemPath(dsn)
	if err != nil {
		return "", err
	}
	if !durable {
		return "", fmt.Errorf("restore operations require a filesystem database")
	}
	return filepath.Join(filepath.Dir(databasePath), "safety_backups"), nil
}
