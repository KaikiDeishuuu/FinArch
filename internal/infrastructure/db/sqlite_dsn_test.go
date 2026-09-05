package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeSQLiteDSN(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		want     string
		notWant  []string
		contains []string
	}{
		{
			name: "plain path",
			dsn:  "/data/finarch.db",
			want: "/data/finarch.db?_txlock=immediate&_busy_timeout=5000&_fk=1&_synchronous=FULL",
		},
		{
			name: "preserves existing file query",
			dsn:  "file:test.db?mode=memory&cache=shared",
			want: "file:test.db?mode=memory&cache=shared&_txlock=immediate&_busy_timeout=5000&_fk=1&_synchronous=FULL",
		},
		{
			name: "unsafe busy timeout aliases are overridden",
			dsn:  "file:test.db?mode=memory&_busy_timeout=1&_TIMEOUT=2&%5Fbusy_timeout=3",
			want: "file:test.db?mode=memory&_txlock=immediate&_busy_timeout=5000&_fk=1&_synchronous=FULL",
			notWant: []string{
				"_busy_timeout=1", "_TIMEOUT=2", "%5Fbusy_timeout=3",
			},
		},
		{
			name: "fk inside value is not treated as key",
			dsn:  "file:test.db?label=_fk=1",
			want: "file:test.db?label=_fk=1&_txlock=immediate&_busy_timeout=5000&_fk=1&_synchronous=FULL",
		},
		{
			name: "txlock suffix in another key is not treated as key",
			dsn:  "file:test.db?x_txlock=immediate",
			want: "file:test.db?x_txlock=immediate&_txlock=immediate&_busy_timeout=5000&_fk=1&_synchronous=FULL",
		},
		{
			name: "unsafe transaction lock aliases are overridden",
			dsn:  "file:test.db?_txlock=deferred&_TXLOCK=exclusive&%5Ftxlock=deferred",
			want: "file:test.db?_txlock=immediate&_busy_timeout=5000&_fk=1&_synchronous=FULL",
			notWant: []string{
				"_txlock=deferred", "_TXLOCK=exclusive", "%5Ftxlock=deferred",
			},
		},
		{
			name: "unsafe foreign key aliases are overridden",
			dsn:  "file:test.db?_fk=0&_foreign_keys=off&%5Ffk=false",
			want: "file:test.db?_txlock=immediate&_busy_timeout=5000&_fk=1&_synchronous=FULL",
			notWant: []string{
				"_fk=0", "_foreign_keys=off", "%5Ffk=false",
			},
		},
		{
			name: "filename containing fk still appends fk param",
			dsn:  "/data/_fk=backup/finarch.db",
			want: "/data/_fk=backup/finarch.db?_txlock=immediate&_busy_timeout=5000&_fk=1&_synchronous=FULL",
		},
		{
			name: "unsafe synchronous aliases are overridden",
			dsn:  "file:test.db?_sync=OFF&_synchronous=NORMAL&%5Fsync=0",
			want: "file:test.db?_txlock=immediate&_busy_timeout=5000&_fk=1&_synchronous=FULL",
			notWant: []string{
				"_sync=OFF", "_synchronous=NORMAL", "%5Fsync=0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeSQLiteDSN(tt.dsn)
			if got != tt.want {
				t.Fatalf("normalizeSQLiteDSN(%q) = %q, want %q", tt.dsn, got, tt.want)
			}
			for _, s := range tt.notWant {
				if strings.Contains(got, s) {
					t.Fatalf("normalizeSQLiteDSN(%q) = %q, contains unwanted %q", tt.dsn, got, s)
				}
			}
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Fatalf("normalizeSQLiteDSN(%q) = %q, missing %q", tt.dsn, got, s)
				}
			}
		})
	}
}

func TestOpenSQLiteAppliesSafetyPragmasToEveryPooledConnection(t *testing.T) {
	ctx := context.Background()
	database, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "durable.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	connections := make([]*sql.Conn, 0, 3)
	defer func() {
		for _, connection := range connections {
			_ = connection.Close()
		}
	}()
	for index := 0; index < 3; index++ {
		connection, err := database.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, connection)
		var synchronous int
		if err := connection.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&synchronous); err != nil {
			t.Fatal(err)
		}
		if synchronous != 2 {
			t.Fatalf("connection %d synchronous=%d, want FULL (2)", index, synchronous)
		}
		var foreignKeys int
		if err := connection.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
			t.Fatal(err)
		}
		if foreignKeys != 1 {
			t.Fatalf("connection %d foreign_keys=%d, want enabled (1)", index, foreignKeys)
		}
		var busyTimeout int
		if err := connection.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
			t.Fatal(err)
		}
		if busyTimeout != 5000 {
			t.Fatalf("connection %d busy_timeout=%d, want 5000", index, busyTimeout)
		}
	}
}
