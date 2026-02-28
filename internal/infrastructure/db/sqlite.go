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
	return nil
}

func execStatements(ctx context.Context, database *sql.DB, script string) error {
	for _, stmt := range strings.Split(script, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := database.ExecContext(ctx, stmt+";"); err != nil {
			// Ignore benign errors: duplicate column, already exists.
			msg := err.Error()
			if strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists") {
				continue
			}
			return fmt.Errorf("%w (stmt: %.80s)", err, stmt)
		}
	}
	return nil
}
