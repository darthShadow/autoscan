//go:build !cgo
// +build !cgo

package sqlite

import (
	"database/sql"
	"fmt"
	"net/url"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

const (
	driverName = "sqlite3"
)

// getDSN returns a DSN string for the SQLite database.
// writable is true if the database is writable, false otherwise.
// Source: https://github.com/nalgeon/redka/blob/017c0b28f7685311c3948b2e6a531012c8092bd3/internal/sqlx/db.go#L173
func getDSN(dbPath string, writable bool) string {
	dsn := fmt.Sprintf("file:%s", dbPath)

	params := url.Values{}

	// sql.DB is concurrent-safe, so we don't need SQLite mutexes.
	// TODO: Investigate if this is used.
	params.Set("_mutex", "no")

	// Disable the shared cache.
	// https://sqlite.org/sharedcache.html
	// https://sqlite.org/uri.html#uricache
	params.Set("cache", "private")

	// Enable normal synchronous mode for better performance.
	// https://sqlite.org/pragma.html#pragma_synchronous
	params.Add("_pragma", "synchronous(NORMAL)")

	// Set the locking mode to normal.
	// https://sqlite.org/pragma.html#pragma_locking_mode
	params.Add("_pragma", "locking_mode(NORMAL)")

	// Set the cache size to 128MB.
	// https://sqlite.org/pragma.html#pragma_cache_size
	params.Add("_pragma", "cache_size(-131072)")

	// Enable foreign key constraints.
	// https://sqlite.org/foreignkeys.html
	params.Add("_pragma", "foreign_keys(ON)")

	// Set busy timeout to 1 second.
	// https://sqlite.org/pragma.html#pragma_busy_timeout
	params.Add("_pragma", "busy_timeout(1000)")

	// Set temp store to memory.
	// https://sqlite.org/pragma.html#pragma_temp_store
	params.Add("_pragma", "temp_store(MEMORY)")

	// Set mmap size to 8GB.
	// https://sqlite.org/pragma.html#pragma_mmap_size
	params.Add("_pragma", "mmap_size(8589934592)")

	// Set threads to 8.
	// https://sqlite.org/pragma.html#pragma_threads
	params.Add("_pragma", "threads(8)")

	if writable {
		// Enable WAL mode for better performance.
		// https://sqlite.org/wal.html
		params.Add("_pragma", "journal_mode(WAL)")

		// Set auto_vacuum to incremental.
		// https://sqlite.org/pragma.html#pragma_auto_vacuum
		params.Add("_pragma", "auto_vacuum(INCREMENTAL)")

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
		params.Add("_pragma", "query_only(ON)")

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
