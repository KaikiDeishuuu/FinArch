package db

import (
	"sync"
	"sync/atomic"
)

// SystemState represents the current state of the system.
type SystemState int32

const (
	// StateNormal means the system is running normally and accepts all requests.
	StateNormal SystemState = iota
	// StateRestore means a restore operation is in progress; writes are blocked.
	StateRestore
	// StateMigration means a schema migration is running; writes are blocked.
	StateMigration
	// StateBackup means a backup snapshot is being taken; writes are blocked.
	StateBackup
)

// ConcurrencyGuard is a thread-safe singleton controlling global write access.
// It is used to block writes during restore, migration, and backup operations.
type ConcurrencyGuard struct {
	state         atomic.Int32
	writeBarrier  sync.RWMutex
	maintenanceMu sync.Mutex
}

var globalGuard = &ConcurrencyGuard{}

// Global returns the singleton ConcurrencyGuard.
func Global() *ConcurrencyGuard { return globalGuard }

// State returns the current system state.
func (g *ConcurrencyGuard) State() SystemState {
	return SystemState(g.state.Load())
}

// SetState transitions the system to the target state.
func (g *ConcurrencyGuard) SetState(s SystemState) {
	g.state.Store(int32(s))
}

// IsWritable returns true only when the system is in StateNormal.
func (g *ConcurrencyGuard) IsWritable() bool {
	return g.State() == StateNormal
}

// TryBeginWrite atomically admits an ordinary write only while the system is
// normal. The returned release function is idempotent and must be held for the
// complete logical write, including external side effects that are committed
// together with database state.
func (g *ConcurrencyGuard) TryBeginWrite() (release func(), ok bool) {
	// Do not queue behind a maintenance writer. Callers use this method from
	// request admission and background workers, where maintenance must be
	// reported/skipped immediately rather than turning into an unbounded wait.
	if !g.writeBarrier.TryRLock() {
		return nil, false
	}
	if !g.IsWritable() {
		g.writeBarrier.RUnlock()
		return nil, false
	}
	var once sync.Once
	return func() {
		once.Do(g.writeBarrier.RUnlock)
	}, true
}

// BeginMaintenance prevents new write leases, then waits for every admitted
// write to drain. Maintenance operations are serialized independently so a
// second restore cannot capture a stale previous state while waiting.
func (g *ConcurrencyGuard) BeginMaintenance(state SystemState) SystemState {
	g.maintenanceMu.Lock()
	previous := g.State()
	g.SetState(state)
	g.writeBarrier.Lock()
	return previous
}

// EndMaintenance publishes the final state before allowing waiting writers to
// retry. Passing StateRestore keeps the service read-only after a failed
// rollback while still releasing the exclusive lock for manual recovery.
func (g *ConcurrencyGuard) EndMaintenance(finalState SystemState) {
	g.SetState(finalState)
	g.writeBarrier.Unlock()
	g.maintenanceMu.Unlock()
}
