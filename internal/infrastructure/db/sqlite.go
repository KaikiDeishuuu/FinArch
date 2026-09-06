package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"net/url"
	"strings"
	"time"

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

//go:embed migration_v14.sql
var migrationV14SQL string

//go:embed migration_v15.sql
var migrationV15SQL string

//go:embed migration_v16.sql
var migrationV16SQL string

//go:embed migration_v17.sql
var migrationV17SQL string

//go:embed migration_v18.sql
var migrationV18SQL string

//go:embed migration_v19.sql
var migrationV19SQL string

//go:embed migration_v20.sql
var migrationV20SQL string

//go:embed migration_v21.sql
var migrationV21SQL string

//go:embed migration_v22.sql
var migrationV22SQL string

//go:embed migration_v23.sql
var migrationV23SQL string

//go:embed migration_v24.sql
var migrationV24SQL string

//go:embed migration_v25.sql
var migrationV25SQL string

//go:embed migration_v26.sql
var migrationV26SQL string

//go:embed migration_v27.sql
var migrationV27SQL string

//go:embed migration_v28.sql
var migrationV28SQL string

//go:embed migration_v29.sql
var migrationV29SQL string

//go:embed migration_v30.sql
var migrationV30SQL string

//go:embed migration_v31.sql
var migrationV31SQL string

//go:embed migration_v32.sql
var migrationV32SQL string

//go:embed migration_v33.sql
var migrationV33SQL string

//go:embed migration_v34.sql
var migrationV34SQL string

type schemaMigration struct {
	version            int
	sql                string
	disableForeignKeys bool
}

// OpenSQLite opens SQLite and configures pragmas for reliability and performance.
func OpenSQLite(ctx context.Context, dsn string) (*sql.DB, error) {
	dsn = normalizeSQLiteDSN(dsn)

	database, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = FULL;",
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
		"PRAGMA synchronous = FULL;",
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
// It enters StateMigration on the global ConcurrencyGuard for the duration,
// preventing any concurrent writes during schema changes.
func Migrate(ctx context.Context, database *sql.DB) error {
	guard := Global()
	previousState := guard.BeginMaintenance(StateMigration)
	defer guard.EndMaintenance(previousState)
	return migrateSchema(ctx, database)
}

// MigrateWithinMaintenance executes migrations when the caller already owns
// the global maintenance lock, such as during a restore. Calling Migrate in
// that situation would attempt to acquire the non-reentrant lock again.
func MigrateWithinMaintenance(ctx context.Context, database *sql.DB) error {
	if Global().State() == StateNormal {
		return fmt.Errorf("migration requires an active maintenance scope")
	}
	return migrateSchema(ctx, database)
}

func migrateSchema(ctx context.Context, database *sql.DB) error {
	// Ensure migrations tracking table exists.
	if _, err := database.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version  INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	migrations := []schemaMigration{
		{version: 1, sql: migrationSQL},
		{version: 2, sql: migrationV2SQL},
		{version: 3, sql: migrationV3SQL},
		{version: 4, sql: migrationV4SQL},
		{version: 5, sql: migrationV5SQL},
		{version: 6, sql: migrationV6SQL},
		{version: 7, sql: migrationV7SQL},
		{version: 8, sql: migrationV8SQL},
		{version: 9, sql: migrationV9SQL, disableForeignKeys: true},
		{version: 10, sql: migrationV10SQL},
		{version: 11, sql: migrationV11SQL},
		{version: 12, sql: migrationV12SQL},
		{version: 13, sql: migrationV13SQL},
		{version: 14, sql: migrationV14SQL},
		{version: 15, sql: migrationV15SQL},
		{version: 16, sql: migrationV16SQL},
		{version: 17, sql: migrationV17SQL},
		{version: 18, sql: migrationV18SQL},
		{version: 19, sql: migrationV19SQL},
		{version: 20, sql: migrationV20SQL},
		{version: 21, sql: migrationV21SQL},
		{version: 22, sql: migrationV22SQL},
		{version: 23, sql: migrationV23SQL},
		{version: 24, sql: migrationV24SQL},
		{version: 25, sql: migrationV25SQL},
		{version: 26, sql: migrationV26SQL},
		{version: 27, sql: migrationV27SQL},
		{version: 28, sql: migrationV28SQL},
		{version: 29, sql: migrationV29SQL},
		{version: 30, sql: migrationV30SQL},
		{version: 31, sql: migrationV31SQL},
		{version: 32, sql: migrationV32SQL},
		{version: 33, sql: migrationV33SQL},
		{version: 34, sql: migrationV34SQL},
	}

	for _, m := range migrations {
		if err := applyMigration(ctx, database, m); err != nil {
			return fmt.Errorf("migration v%d: %w", m.version, err)
		}
	}

	// Re-apply triggers on every startup (idempotent IF NOT EXISTS).
	// Triggers live outside the migrations table so they survive a fresh DB too.
	if err := ApplyTriggers(ctx, database); err != nil {
		return fmt.Errorf("apply triggers: %w", err)
	}
	return nil
}

func applyMigration(ctx context.Context, database *sql.DB, migration schemaMigration) (returnErr error) {
	conn, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Close()

	var exists int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, migration.version,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check migration version: %w", err)
	}
	if exists > 0 {
		return nil
	}

	foreignKeysEnabled := 0
	if migration.disableForeignKeys {
		if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeysEnabled); err != nil {
			return fmt.Errorf("read foreign_keys pragma: %w", err)
		}
		if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
			return fmt.Errorf("disable foreign_keys: %w", err)
		}
		defer func() {
			restoreCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			pragma := `PRAGMA foreign_keys = OFF`
			if foreignKeysEnabled != 0 {
				pragma = `PRAGMA foreign_keys = ON`
			}
			if _, err := conn.ExecContext(restoreCtx, pragma); err != nil && returnErr == nil {
				returnErr = fmt.Errorf("restore foreign_keys pragma: %w", err)
			}
		}()
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := execStatements(ctx, tx, migration.sql); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, applied_at) VALUES(?, strftime('%s','now'))`,
		migration.version,
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

func normalizeSQLiteDSN(dsn string) string {
	params := []string{"_txlock=immediate", "_busy_timeout=5000", "_fk=1", "_synchronous=FULL"}
	base, rawQuery, hasQuery := strings.Cut(dsn, "?")
	seen := map[string]struct{}{}
	queryParts := make([]string, 0)
	if hasQuery {
		for _, part := range strings.Split(rawQuery, "&") {
			if part == "" {
				continue
			}
			key, _, _ := strings.Cut(part, "=")
			if decoded, err := url.QueryUnescape(key); err == nil {
				key = decoded
			}
			key = strings.ToLower(key)
			// mattn/go-sqlite3 accepts aliases for connection-local invariants,
			// and the winning value can depend on ordering. Remove every supplied
			// form and append one canonical safe value below.
			if key == "_synchronous" || key == "_sync" ||
				key == "_fk" || key == "_foreign_keys" ||
				key == "_txlock" || key == "_busy_timeout" || key == "_timeout" {
				continue
			}
			seen[key] = struct{}{}
			queryParts = append(queryParts, part)
		}
	}

	query := strings.Join(queryParts, "&")
	for _, param := range params {
		key, _, _ := strings.Cut(param, "=")
		if _, ok := seen[key]; ok {
			continue
		}
		if query != "" {
			query += "&"
		}
		query += param
	}
	if query == "" {
		return base
	}
	return base + "?" + query
}

type migrationExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func execStatements(ctx context.Context, database migrationExecutor, script string) error {
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
