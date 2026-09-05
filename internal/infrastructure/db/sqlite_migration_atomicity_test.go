package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

func TestApplyMigrationRollsBackSchemaAndVersionOnFailure(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("file:migration-atomic-%d?mode=memory&cache=shared", time.Now().UnixNano())
	database, err := OpenSQLite(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	if _, err := database.ExecContext(ctx, `
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)`); err != nil {
		t.Fatal(err)
	}

	err = applyMigration(ctx, database, schemaMigration{
		version: 999,
		sql: `
			CREATE TABLE should_rollback (id INTEGER PRIMARY KEY);
			INSERT INTO table_that_does_not_exist(id) VALUES(1);
		`,
		disableForeignKeys: true,
	})
	if err == nil {
		t.Fatal("expected migration failure")
	}

	var count int
	if err := database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='should_rollback'`,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed migration left a partially-created table")
	}
	if err := database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE version=999`,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed migration recorded its version")
	}
	if err := database.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("foreign_keys was not restored: %d", count)
	}
}

func TestMigrateRestoresEnclosingMaintenanceState(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("file:migration-guard-%d?mode=memory&cache=shared", time.Now().UnixNano())
	database, err := OpenSQLite(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	guard := Global()
	previousState := guard.State()
	guard.SetState(StateRestore)
	t.Cleanup(func() { guard.SetState(previousState) })
	if err := Migrate(ctx, database); err != nil {
		t.Fatal(err)
	}
	if state := guard.State(); state != StateRestore {
		t.Fatalf("migration restored guard state=%v, want restore", state)
	}
}

func TestMigrateWaitsForWritesAndSerializesMaintenance(t *testing.T) {
	ctx := context.Background()
	firstDB, err := OpenSQLite(ctx, fmt.Sprintf("file:migration-serialized-a-%d?mode=memory&cache=shared", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	defer firstDB.Close()
	secondDB, err := OpenSQLite(ctx, fmt.Sprintf("file:migration-serialized-b-%d?mode=memory&cache=shared", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	defer secondDB.Close()

	guard := Global()
	previousState := guard.State()
	guard.SetState(StateNormal)
	t.Cleanup(func() { guard.SetState(previousState) })
	releaseWrite, ok := guard.TryBeginWrite()
	if !ok {
		t.Fatal("failed to acquire setup write lease")
	}
	defer releaseWrite()

	firstDone := make(chan error, 1)
	go func() { firstDone <- Migrate(ctx, firstDB) }()
	deadline := time.Now().Add(time.Second)
	for guard.State() != StateMigration && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if guard.State() != StateMigration {
		t.Fatal("first migration did not enter maintenance")
	}
	select {
	case err := <-firstDone:
		t.Fatalf("migration bypassed an admitted write lease: %v", err)
	default:
	}

	secondDone := make(chan error, 1)
	go func() { secondDone <- Migrate(ctx, secondDB) }()
	select {
	case err := <-secondDone:
		t.Fatalf("second migration bypassed maintenance serialization: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	releaseWrite()
	for index, done := range []<-chan error{firstDone, secondDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("migration %d failed: %v", index+1, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("migration %d did not finish", index+1)
		}
	}
	if state := guard.State(); state != StateNormal {
		t.Fatalf("final guard state=%v, want normal", state)
	}
}

func TestMigrationV25AddsCategoryCreatedAt(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("file:migration-v25-%d?mode=memory&cache=shared", time.Now().UnixNano())
	database, err := OpenSQLite(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := Migrate(ctx, database); err != nil {
		t.Fatal(err)
	}

	rows, err := database.QueryContext(ctx, `PRAGMA table_info(categories)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == "created_at" {
			found = true
		}
	}
	if !found {
		t.Fatal("categories.created_at is missing after migration")
	}
}

func TestMigrationV26CreatesAttachmentDeletionQueueWithoutUserForeignKey(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("file:migration-v26-%d?mode=memory&cache=shared", time.Now().UnixNano())
	database, err := OpenSQLite(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := Migrate(ctx, database); err != nil {
		t.Fatal(err)
	}

	expectedColumns := map[string]bool{
		"storage_key": false, "user_id": false, "attempts": false,
		"last_error": false, "created_at": false, "updated_at": false,
	}
	rows, err := database.QueryContext(ctx, `PRAGMA table_info(attachment_deletion_queue)`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if _, exists := expectedColumns[name]; exists {
			expectedColumns[name] = true
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for name, found := range expectedColumns {
		if !found {
			t.Fatalf("attachment_deletion_queue.%s is missing", name)
		}
	}

	foreignKeys, err := database.QueryContext(ctx, `PRAGMA foreign_key_list(attachment_deletion_queue)`)
	if err != nil {
		t.Fatal(err)
	}
	defer foreignKeys.Close()
	if foreignKeys.Next() {
		t.Fatal("attachment deletion queue must not reference users or other metadata tables")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := database.ExecContext(ctx, `
		INSERT INTO attachment_deletion_queue(storage_key, user_id, created_at, updated_at)
		VALUES('orphan/object.png', 'already-deleted-user', ?, ?)`, now, now); err != nil {
		t.Fatalf("queue row must survive without a user row: %v", err)
	}
}
