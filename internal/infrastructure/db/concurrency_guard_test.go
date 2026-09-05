package db

import (
	"testing"
	"time"
)

func TestMaintenanceWaitsForAdmittedWriteAndRejectsNewWrites(t *testing.T) {
	guard := &ConcurrencyGuard{}
	releaseWrite, ok := guard.TryBeginWrite()
	if !ok {
		t.Fatal("normal guard rejected initial write")
	}

	entered := make(chan SystemState, 1)
	done := make(chan struct{})
	go func() {
		previous := guard.BeginMaintenance(StateRestore)
		entered <- previous
		guard.EndMaintenance(previous)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for guard.State() != StateRestore && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if guard.State() != StateRestore {
		t.Fatal("maintenance did not publish restore state")
	}
	if release, admitted := guard.TryBeginWrite(); admitted {
		release()
		t.Fatal("new write was admitted after maintenance began")
	}
	select {
	case <-done:
		t.Fatal("maintenance did not wait for the active write")
	default:
	}

	releaseWrite()
	select {
	case previous := <-entered:
		if previous != StateNormal {
			t.Fatalf("previous state = %v, want normal", previous)
		}
	case <-time.After(time.Second):
		t.Fatal("maintenance did not start after active write drained")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("maintenance did not finish")
	}
	if guard.State() != StateNormal {
		t.Fatalf("final state = %v, want normal", guard.State())
	}
}

func TestWriteLeaseReleaseIsIdempotent(t *testing.T) {
	guard := &ConcurrencyGuard{}
	release, ok := guard.TryBeginWrite()
	if !ok {
		t.Fatal("write was not admitted")
	}
	release()
	release()
	previous := guard.BeginMaintenance(StateRestore)
	guard.EndMaintenance(previous)
}
