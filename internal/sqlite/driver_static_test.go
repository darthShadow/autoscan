//go:build !cgo

package sqlite

import (
	"path/filepath"
	"testing"
)

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
