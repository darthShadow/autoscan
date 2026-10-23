package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestConnectionPragmaDefaults(t *testing.T) {
	db, err := NewDB(t.Context(), filepath.Join(t.TempDir(), "defaults.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, handle := range []struct {
		name string
		pool *sql.DB
	}{
		{"read-write", db.RW()},
		{"read-only", db.RO()},
	} {
		t.Run(handle.name, func(t *testing.T) {
			conn, err := handle.pool.Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })

			for _, setting := range []struct {
				name string
				want int
			}{
				{"busy_timeout", 10000},
				{"cache_size", -1048576},
				{"journal_size_limit", 67108864},
				{"wal_autocheckpoint", 16000},
			} {
				t.Run(setting.name, func(t *testing.T) {
					var got int
					if err := conn.QueryRowContext(t.Context(), "PRAGMA "+setting.name).Scan(&got); err != nil {
						t.Fatal(err)
					}
					if got != setting.want {
						t.Errorf("PRAGMA %s = %d, want %d", setting.name, got, setting.want)
					}
				})
			}
		})
	}
}
