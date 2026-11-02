package sqlite

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenDBWaitsForSchemaLockRelease(t *testing.T) {
	for _, mode := range []struct {
		name     string
		writable bool
	}{
		{"read-only", false},
		{"read-write", true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "locked.db")
			holder, err := OpenDB(path, true)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = holder.Close() })
			holder.SetMaxOpenConns(1)
			if _, err := holder.ExecContext(t.Context(), "CREATE TABLE fixture (value INTEGER)"); err != nil {
				t.Fatal(err)
			}
			// An exclusive WAL connection keeps schema readers out until it closes.
			const lock = "PRAGMA locking_mode=EXCLUSIVE; BEGIN EXCLUSIVE; INSERT INTO fixture VALUES (1)"
			if _, err := holder.ExecContext(t.Context(), lock); err != nil {
				t.Fatal(err)
			}

			type releaseResult struct {
				started time.Time
				err     error
			}
			released := make(chan releaseResult, 1)
			time.AfterFunc(300*time.Millisecond, func() {
				started := time.Now()
				_, rollbackErr := holder.ExecContext(t.Context(), "ROLLBACK")
				closeErr := holder.Close()
				released <- releaseResult{started, errors.Join(rollbackErr, closeErr)}
			})

			db, openErr := OpenDB(path, mode.writable)
			if openErr == nil {
				t.Cleanup(func() { _ = db.Close() })
				// OpenDB is lazy; PingContext opens the physical connection and applies its pragmas.
				openErr = db.PingContext(t.Context())
			}
			returned := time.Now()
			release := <-released
			if release.err != nil {
				t.Fatalf("release schema lock: %v", release.err)
			}
			if openErr != nil {
				t.Errorf("OpenDB and PingContext while schema is locked: %v; want success after release", openErr)
			}
			if returned.Before(release.started) {
				t.Errorf("open returned at %v before lock release started at %v", returned, release.started)
			}
		})
	}
}
