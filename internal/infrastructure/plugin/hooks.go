package plugin

import "context"

// Hook defines extension points for future plugins.
type Hook interface {
	// Name returns unique hook name.
	Name() string
	// OnLedgerUpdated is triggered after balance related updates.
	OnLedgerUpdated(ctx context.Context) error
}
