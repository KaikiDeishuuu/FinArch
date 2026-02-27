package plugin

import "context"

// Registry stores and executes hooks.
type Registry struct {
	hooks []Hook
}

// Register appends one hook.
func (r *Registry) Register(h Hook) {
	r.hooks = append(r.hooks, h)
}

// EmitLedgerUpdated invokes all registered hooks.
func (r *Registry) EmitLedgerUpdated(ctx context.Context) error {
	for _, h := range r.hooks {
		if err := h.OnLedgerUpdated(ctx); err != nil {
			return err
		}
	}
	return nil
}
