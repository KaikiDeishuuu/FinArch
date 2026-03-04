package test

import (
	"context"
	"testing"
)

func TestMigration_RemovesNicknameUniqueIndex(t *testing.T) {
	db := setupDB(t)
	defer db.Close()

	ctx := context.Background()
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_users_nickname_unique'`).Scan(&count)
	if err != nil {
		t.Fatalf("query index failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("nickname unique index should be removed, got count=%d", count)
	}
}
