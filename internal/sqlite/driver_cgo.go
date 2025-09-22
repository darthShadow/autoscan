//go:build cgo
// +build cgo

package sqlite

import (
	"database/sql"
	"fmt"
	"net/url"

	"github.com/mattn/go-sqlite3"
)

const (
	driverName = "sqlite3custom"
)

func init() {
	sql.Register(driverName, &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			for _, query := range []string{
				"PRAGMA temp_store=MEMORY",
				"PRAGMA mmap_size=8589934592",
				"PRAGMA threads=8",
			} {
				if _, err := conn.Exec(query, nil); err != nil {
					return fmt.Errorf("failed to execute query %q: %w", query, err)
				}
			}
			return nil
		},
	})
}

// getDSN returns a DSN string for the SQLite database.
// writable is true if the database is writable, false otherwise.
// Source: https://github.com/nalgeon/redka/blob/017c0b28f7685311c3948b2e6a531012c8092bd3/internal/sqlx/db.go#L173
func getDSN(dbPath string, writable bool) string {
	dsn := fmt.Sprintf("file:%s", dbPath)

	params := url.Values{}

	// sql.DB is concurrent-safe, so we don't need SQLite mutexes.
	params.Set("_mutex", "no")

	// Disable the shared cache.
	// https://sqlite.org/sharedcache.html
	// https://sqlite.org/uri.html#uricache
	params.Set("cache", "private")

	// Enable normal synchronous mode for better performance.
	// https://sqlite.org/pragma.html#pragma_synchronous
	params.Set("_synchronous", "NORMAL")

	// Set the locking mode to normal.
	// https://sqlite.org/pragma.html#pragma_locking_mode
	params.Set("_locking_mode", "NORMAL")

	// Set the cache size to 128MB.
	// https://sqlite.org/pragma.html#pragma_cache_size
	params.Set("_cache_size", "-131072")

	// Enable foreign key constraints.
	// https://sqlite.org/foreignkeys.html
	params.Set("_foreign_keys", "ON")

	// Set busy timeout to 1 second.
	// https://sqlite.org/pragma.html#pragma_busy_timeout
	params.Set("_busy_timeout", "1000")

	if writable {
		// Enable WAL mode for better performance.
		// https://sqlite.org/wal.html
		params.Set("_journal_mode", "WAL")

		// Set auto_vacuum to incremental.
		// https://sqlite.org/pragma.html#pragma_auto_vacuum
		params.Set("_auto_vacuum", "INCREMENTAL")

		// Enable read-write-create mode for writable databases.
		// https://sqlite.org/c3ref/open.html
		// https://sqlite.org/uri.html#urimode
		params.Set("mode", "rwc")

		// Enable IMMEDIATE transactions for writable databases.
		// https://sqlite.org/lang_transaction.html
		//goland:noinspection SpellCheckingInspection
		params.Set("_txlock", "immediate")

	} else {
		// Prevent data-changes on the database.
		// https://sqlite.org/pragma.html#pragma_query_only
		params.Set("_query_only", "ON")

		// Enable read-only mode for read-only databases
		// https://sqlite.org/c3ref/open.html
		// https://sqlite.org/uri.html#urimode
		params.Set("mode", "ro")
	}

	return dsn + "?" + params.Encode()
}

func OpenDB(dbPath string, writable bool) (*sql.DB, error) {
	dsn := getDSN(dbPath, writable)
	return sql.Open(driverName, dsn)
}
