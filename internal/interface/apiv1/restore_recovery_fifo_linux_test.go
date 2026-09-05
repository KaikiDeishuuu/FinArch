package apiv1

import (
	"path/filepath"
	"syscall"
	"testing"
)

func TestDiscoverRestoreOperationsRejectsFIFOMarkers(t *testing.T) {
	testCases := []struct {
		name       string
		markerPath func(*testing.T, string) string
	}{
		{
			name: "root marker",
			markerPath: func(_ *testing.T, safetyDir string) string {
				return filepath.Join(safetyDir, restoreOperationRootPrefix+"fifo"+restoreOperationRootSuffix)
			},
		},
		{
			name: "journal marker",
			markerPath: func(t *testing.T, safetyDir string) string {
				t.Helper()
				journalDir := filepath.Join(safetyDir, restoreJournalDirPrefix+"fifo")
				if err := syscall.Mkdir(journalDir, 0o700); err != nil {
					t.Fatalf("create journal directory: %v", err)
				}
				return filepath.Join(journalDir, restoreOperationJournalFile)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			baseDir := t.TempDir()
			safetyDir := filepath.Join(baseDir, "safety_backups")
			if err := syscall.Mkdir(safetyDir, 0o700); err != nil {
				t.Fatal(err)
			}
			markerPath := testCase.markerPath(t, safetyDir)
			if err := syscall.Mkfifo(markerPath, 0o600); err != nil {
				t.Skipf("FIFOs unavailable: %v", err)
			}

			assertNonRegularRestoreMarkerRejected(t, filepath.Join(baseDir, "live.db"))
		})
	}
}
