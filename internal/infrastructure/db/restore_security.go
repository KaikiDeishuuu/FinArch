package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// InvalidateRestoredAuthenticationState makes a physical database snapshot
// safe to activate with the deployment's current signing keys. Without this
// step, sessions and one-time links that were live at snapshot time can become
// valid again after a rollback or disaster restore.
func InvalidateRestoredAuthenticationState(ctx context.Context, database *sql.DB) error {
	if database == nil {
		return fmt.Errorf("restored database is unavailable")
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin restored credential invalidation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Unix()
	if _, err := tx.ExecContext(ctx, `
		UPDATE auth_sessions
		SET revoked_at = COALESCE(revoked_at, ?),
		    revoke_reason = COALESCE(revoke_reason, 'database_restore')
		WHERE revoked_at IS NULL`, now); err != nil {
		return fmt.Errorf("revoke restored auth sessions: %w", err)
	}
	// A revoked family can never be refreshed again. Remove its bearer hashes
	// and encrypted lost-response recovery material instead of retaining live
	// credentials that serve no post-restore purpose.
	if _, err := tx.ExecContext(ctx, `DELETE FROM refresh_tokens`); err != nil {
		return fmt.Errorf("remove restored refresh credentials: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE action_requests
		SET status = 'expired'
		WHERE status = 'pending'`); err != nil {
		return fmt.Errorf("expire restored action requests: %w", err)
	}
	// email_tokens is a legacy store, but old snapshots can still contain rows.
	// Delete them so no historical reset/change/delete link can revive if a
	// compatibility path is reintroduced later.
	if _, err := tx.ExecContext(ctx, `DELETE FROM email_tokens`); err != nil {
		return fmt.Errorf("remove restored legacy email tokens: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET pending_email = NULL WHERE pending_email IS NOT NULL`); err != nil {
		return fmt.Errorf("clear restored pending email changes: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit restored credential invalidation: %w", err)
	}
	return nil
}
