//go:build !cgo

package sqlite

import (
	"database/sql"
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectionMemoryHoldsLargeCache(t *testing.T) {
	const (
		blobSize   = 4000
		batchSize  = 250
		batchCount = 300
		rowCount   = batchSize * batchCount
	)
	written := 0
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("SQLite panicked after writing %d bytes: %v", written, recovered)
		}
	}()

	db, err := NewDB(t.Context(), filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const createTable = "CREATE TABLE pages (id INTEGER PRIMARY KEY, body BLOB NOT NULL)"
	if _, err := db.RW().ExecContext(t.Context(), createTable); err != nil {
		t.Fatal(err)
	}
	for batch := range batchCount {
		insertMemoryCapBatch(t, db.RW(), batch*batchSize, batchSize, blobSize)
		written += batchSize * blobSize
	}
	t.Logf("wrote %d distinct page bytes", written)
	for _, handle := range []struct {
		name string
		pool *sql.DB
	}{
		{"read-only", db.RO()},
		{"read-write", db.RW()},
	} {
		scanMemoryCapPages(t, handle.pool, rowCount, blobSize)
		t.Logf("%s scanned %d bytes", handle.name, rowCount*blobSize)
	}
}

func insertMemoryCapBatch(t *testing.T, pool *sql.DB, first, count, blobSize int) {
	t.Helper()
	args := make([]any, 0, count*2)
	for id := first; id < first+count; id++ {
		body := make([]byte, blobSize)
		binary.LittleEndian.PutUint64(body, uint64(id))
		args = append(args, id, body)
	}
	query := "INSERT INTO pages(id, body) VALUES " + strings.TrimSuffix(strings.Repeat("(?, ?),", count), ",")
	if _, err := pool.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("insert batch at row %d: %v", first, err)
	}
}

func scanMemoryCapPages(t *testing.T, pool *sql.DB, wantRows, blobSize int) {
	t.Helper()
	rows, err := pool.QueryContext(t.Context(), "SELECT id, body FROM pages ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	count, total := 0, 0
	for rows.Next() {
		var id int
		var body []byte
		if err := rows.Scan(&id, &body); err != nil {
			t.Fatal(err)
		}
		if len(body) != blobSize {
			t.Fatalf("row %d has %d bytes, want %d", id, len(body), blobSize)
		}
		if id != count || binary.LittleEndian.Uint64(body) != uint64(id) {
			t.Fatalf("row %d: id = %d, body id = %d", count, id, binary.LittleEndian.Uint64(body))
		}
		count++
		total += len(body)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != wantRows || total != wantRows*blobSize {
		t.Fatalf("scan = %d rows, %d bytes; want %d rows, %d bytes", count, total, wantRows, wantRows*blobSize)
	}
}
