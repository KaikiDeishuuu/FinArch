package db

import (
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
			want: "/data/finarch.db?_txlock=immediate&_busy_timeout=5000&_fk=1",
		},
		{
			name: "preserves existing file query",
			dsn:  "file:test.db?mode=memory&cache=shared",
			want: "file:test.db?mode=memory&cache=shared&_txlock=immediate&_busy_timeout=5000&_fk=1",
		},
		{
			name: "does not duplicate real busy timeout",
			dsn:  "file:test.db?mode=memory&_busy_timeout=10000",
			want: "file:test.db?mode=memory&_busy_timeout=10000&_txlock=immediate&_fk=1",
			notWant: []string{
				"_busy_timeout=10000&_busy_timeout=5000",
			},
		},
		{
			name: "fk inside value is not treated as key",
			dsn:  "file:test.db?label=_fk=1",
			want: "file:test.db?label=_fk=1&_txlock=immediate&_busy_timeout=5000&_fk=1",
		},
		{
			name: "txlock suffix in another key is not treated as key",
			dsn:  "file:test.db?x_txlock=immediate",
			want: "file:test.db?x_txlock=immediate&_txlock=immediate&_busy_timeout=5000&_fk=1",
		},
		{
			name: "existing fk override is preserved",
			dsn:  "file:test.db?_fk=0",
			want: "file:test.db?_fk=0&_txlock=immediate&_busy_timeout=5000",
			notWant: []string{
				"_fk=0&_fk=1",
			},
		},
		{
			name: "filename containing fk still appends fk param",
			dsn:  "/data/_fk=backup/finarch.db",
			want: "/data/_fk=backup/finarch.db?_txlock=immediate&_busy_timeout=5000&_fk=1",
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
