package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migration.sql
var migrationSQL string

//go:embed migration_v2.sql
var migrationV2SQL string

//go:embed migration_v3.sql
var migrationV3SQL string

//go:embed migration_v4.sql
var migrationV4SQL string

//go:embed migration_v5.sql
var migrationV5SQL string

//go:embed migration_v6.sql
var migrationV6SQL string

//go:embed migration_v7.sql
var migrationV7SQL string

//go:embed migration_v8.sql
var migrationV8SQL string

//go:embed migration_v9.sql
var migrationV9SQL string

//go:embed migration_v10.sql
var migrationV10SQL string

//go:embed migration_v11.sql
var migrationV11SQL string

//go:embed migration_v12.sql
var migrationV12SQL string

//go:embed migration_v13.sql
var migrationV13SQL string

// OpenSQLite opens SQLite and configures pragmas for reliability and performance.
func OpenSQLite(ctx context.Context, dsn string) (*sql.DB, error) {
	database, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA busy_timeout = 5000;",
	}

	for _, q := range pragmas {
		if _, err := database.ExecContext(ctx, q); err != nil {
			_ = database.Close()
			return nil, fmt.Errorf("apply pragma: %w", err)
		}
	}

	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return database, nil
}

// ReapplyPragmas re-sets critical pragmas after a restore (Backup API may reset them).
func ReapplyPragmas(ctx context.Context, database *sql.DB) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, q := range pragmas {
		if _, err := database.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("reapply pragma: %w", err)
		}
	}
	return nil
}

// Migrate executes schema migrations in order, tracking applied versions.
func Migrate(ctx context.Context, database *sql.DB) error {
	// Ensure migrations tracking table exists.
	if _, err := database.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version  INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	migrations := []struct {
		version int
		sql     string
	}{
		{1, migrationSQL},
		{2, migrationV2SQL},
		{3, migrationV3SQL},
		{4, migrationV4SQL},
		{5, migrationV5SQL},
		{6, migrationV6SQL},
		{7, migrationV7SQL},
		{8, migrationV8SQL},
		{9, migrationV9SQL},
		{10, migrationV10SQL},
		{11, migrationV11SQL},
		{12, migrationV12SQL},
		{13, migrationV13SQL},
	}

	for _, m := range migrations {
		var exists int
		_ = database.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version,
		).Scan(&exists)
		if exists > 0 {
			continue
		}
		if err := execStatements(ctx, database, m.sql); err != nil {
			return fmt.Errorf("migration v%d: %w", m.version, err)
		}
		if _, err := database.ExecContext(ctx,
			`INSERT INTO schema_migrations(version, applied_at) VALUES(?, strftime('%s','now'))`,
			m.version,
		); err != nil {
			return fmt.Errorf("record migration v%d: %w", m.version, err)
		}
	}

	// Re-apply triggers on every startup (idempotent IF NOT EXISTS).
	// Triggers live outside the migrations table so they survive a fresh DB too.
	if err := ApplyTriggers(ctx, database); err != nil {
		return fmt.Errorf("apply triggers: %w", err)
	}
	return nil
}

func execStatements(ctx context.Context, database *sql.DB, script string) error {
	for _, stmt := range strings.Split(script, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		// Skip statements that are entirely SQL comments (-- lines).
		nonComment := false
		for _, line := range strings.Split(stmt, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "--") {
				nonComment = true
				break
			}
		}
		if !nonComment {
			continue
		}
		if _, err := database.ExecContext(ctx, stmt+";"); err != nil {
			// Ignore benign errors: duplicate column, already exists.
			msg := err.Error()
			if strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists") {
				continue
			}
			return fmt.Errorf("migration %w (stmt: %.80s)", err, stmt)
		}
	}
	return nil
}
