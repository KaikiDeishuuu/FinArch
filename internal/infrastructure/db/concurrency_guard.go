package db

import (
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
	state atomic.Int32
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
