//go:build !cgo

package sqlite

import (
	"net/url"
	"path/filepath"
	"testing"
)

func TestDSNPragmaPrefix(t *testing.T) {
	for _, mode := range []struct {
		name     string
		writable bool
	}{
		{"read-only", false},
		{"read-write-create", true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			dsn, err := url.Parse(getDSN("prefix.db", mode.writable))
			if err != nil {
				t.Fatal(err)
			}
			params, err := url.ParseQuery(dsn.RawQuery)
			if err != nil {
				t.Fatal(err)
			}
			pragmas := params["_pragma"]
			if len(pragmas) < 2 {
				t.Fatalf("_pragma = %q, want at least two values", pragmas)
			}
			for i, want := range []string{"busy_timeout(10000)", "locking_mode(NORMAL)"} {
				if pragmas[i] != want {
					t.Errorf("_pragma[%d] = %q, want %q", i, pragmas[i], want)
				}
			}
		})
	}
}

func TestFTS5Search(t *testing.T) {
	db, err := NewDB(t.Context(), filepath.Join(t.TempDir(), "fts5.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.RW().ExecContext(t.Context(), "CREATE VIRTUAL TABLE docs USING fts5(body)"); err != nil {
		t.Fatal(err)
	}
	const body = "searchable media metadata"
	if _, err := db.RW().ExecContext(t.Context(), "INSERT INTO docs(body) VALUES (?)", body); err != nil {
		t.Fatal(err)
	}
	var got string
	const query = "SELECT body FROM docs WHERE docs MATCH ?"
	if err := db.RW().QueryRowContext(t.Context(), query, "searchable").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != body {
		t.Errorf("MATCH body = %q, want %q", got, body)
	}
}
