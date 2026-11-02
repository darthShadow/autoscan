//go:build !cgo

package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3"
)

func TestBusyWaitCarriesContextDeadline(t *testing.T) {
	for _, operation := range []string{"read", "write"} {
		t.Run(operation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "busy-context.db")
			holder, err := OpenDB(path, true)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = holder.Close() })
			holder.SetMaxOpenConns(1)
			if _, err = holder.ExecContext(t.Context(), "CREATE TABLE fixture (value INTEGER)"); err != nil {
				t.Fatal(err)
			}
			// Rollback journals let an exclusive transaction block an already-open reader.
			if operation == "read" {
				if _, err = holder.ExecContext(t.Context(), "PRAGMA journal_mode=DELETE"); err != nil {
					t.Fatal(err)
				}
			}
			db, err := OpenDB(path, operation == "write")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			db.SetMaxOpenConns(1)
			var timeout int
			if err = db.QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&timeout); err != nil {
				t.Fatal(err)
			}
			if timeout != 10000 {
				t.Fatalf("busy_timeout = %d, want 10000", timeout)
			}
			const lock = "BEGIN EXCLUSIVE; INSERT INTO fixture VALUES (1)"
			if _, err = holder.ExecContext(t.Context(), lock); err != nil {
				t.Fatal(err)
			}
			defer func() { _, _ = holder.ExecContext(t.Context(), "ROLLBACK") }()

			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			started := time.Now()
			if operation == "read" {
				var count int
				err = db.QueryRowContext(ctx, "SELECT count(*) FROM fixture").Scan(&count)
			} else {
				_, err = db.ExecContext(ctx, "INSERT INTO fixture VALUES (2)")
			}
			elapsed := time.Since(started)
			t.Logf("busy %s returned after %v: %v", operation, elapsed, err)
			if elapsed >= 5*time.Second {
				t.Errorf("busy %s elapsed = %v, want below 5s", operation, elapsed)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("busy %s error = %v, want context.DeadlineExceeded", operation, err)
			}
		})
	}
}

func TestColdOpenCarriesContextDeadline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cold.db")
	holder, err := OpenDB(path, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Close() })
	holder.SetMaxOpenConns(1)
	const lock = "CREATE TABLE fixture (value INTEGER); PRAGMA journal_mode=DELETE; BEGIN EXCLUSIVE"
	if _, err = holder.ExecContext(t.Context(), lock); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = holder.ExecContext(t.Context(), "ROLLBACK") }()
	db, err := OpenDB(path, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	err = db.PingContext(ctx)
	if !errors.Is(err, sqlite3.BUSY) {
		t.Errorf("cold connection error = %v, want sqlite3.BUSY", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("cold connection lost context.DeadlineExceeded: %v", err)
	}
}
