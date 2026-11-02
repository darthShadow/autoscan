//go:build !cgo

package sqlite

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"fmt"
	"net/url"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
)

// maxConnMemory holds the 1 GiB cache, the 1 GiB mmap region, and working memory.
// On Unix, ncruces reserves this address space with PROT_NONE and commits only what it uses.
const maxConnMemory = 1 << 32

type memoryConnector struct {
	sqldriver.Connector
}

func (connector memoryConnector) Connect(ctx context.Context) (sqldriver.Conn, error) {
	conn, err := connector.Connector.Connect(sqlite3.WithMaxMemory(ctx, maxConnMemory))
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", interruptError(ctx, err))
	}
	// The pinned ncruces driver implements these optional database/sql interfaces.
	return interruptConn{conn.(sqliteConn)}, nil //nolint:forcetypeassert // Fixed upstream driver contract.
}

type sqliteConn interface {
	driver.Conn
	sqldriver.ExecerContext
	sqldriver.NamedValueChecker
}

type interruptConn struct{ sqliteConn }

func (conn interruptConn) Prepare(query string) (sqldriver.Stmt, error) {
	return conn.PrepareContext(context.Background(), query)
}

func (conn interruptConn) PrepareContext(ctx context.Context, query string) (sqldriver.Stmt, error) {
	stmt, err := conn.sqliteConn.PrepareContext(ctx, query)
	if err != nil {
		return nil, interruptError(ctx, err)
	}
	return interruptStmt{stmt.(sqliteStmt)}, nil //nolint:forcetypeassert // Fixed upstream driver contract.
}

func (conn interruptConn) BeginTx(ctx context.Context, opts sqldriver.TxOptions) (sqldriver.Tx, error) {
	tx, err := conn.sqliteConn.BeginTx(ctx, opts)
	if err != nil {
		return nil, interruptError(ctx, err)
	}
	return tx, nil
}

func (conn interruptConn) ExecContext(
	ctx context.Context, query string, args []sqldriver.NamedValue,
) (sqldriver.Result, error) {
	result, err := conn.sqliteConn.ExecContext(ctx, query, args)
	if err != nil {
		return nil, interruptError(ctx, err)
	}
	return result, nil
}

type sqliteStmt interface {
	sqldriver.Stmt
	sqldriver.StmtExecContext
	sqldriver.StmtQueryContext
	sqldriver.NamedValueChecker
}

type interruptStmt struct{ sqliteStmt }

func (stmt interruptStmt) ExecContext(ctx context.Context, args []sqldriver.NamedValue) (sqldriver.Result, error) {
	result, err := stmt.sqliteStmt.ExecContext(ctx, args)
	if err != nil {
		return nil, interruptError(ctx, err)
	}
	return result, nil
}

func (stmt interruptStmt) QueryContext(ctx context.Context, args []sqldriver.NamedValue) (sqldriver.Rows, error) {
	rows, err := stmt.sqliteStmt.QueryContext(ctx, args)
	if err != nil {
		return nil, interruptError(ctx, err)
	}
	return interruptRows{rows.(sqliteRows), ctx}, nil //nolint:forcetypeassert // Fixed upstream driver contract.
}

type sqliteRows interface {
	sqldriver.Rows
	sqldriver.RowsColumnTypeDatabaseTypeName
	sqldriver.RowsColumnTypeNullable
	sqldriver.RowsColumnTypeScanType
}

type interruptRows struct {
	sqliteRows

	ctx context.Context //nolint:containedctx // Rows.Next and Close have no context parameter.
}

func (rows interruptRows) Next(values []sqldriver.Value) error {
	return interruptError(rows.ctx, rows.sqliteRows.Next(values))
}

func (rows interruptRows) Close() error {
	return interruptError(rows.ctx, rows.sqliteRows.Close())
}

// interruptError adds the context error to an error the context's end caused. The busy handler stops
// retrying once the context ends and SQLite then reports BUSY, not INTERRUPT (ncruces conn.go Xgo_busy_timeout).
func interruptError(ctx context.Context, err error) error {
	if (errors.Is(err, sqlite3.INTERRUPT) || errors.Is(err, sqlite3.BUSY)) && ctx.Err() != nil {
		return fmt.Errorf("%w: %w", err, ctx.Err())
	}
	return err
}

func init() {
	// Enable FTS5 only in pure-Go builds; the cgo (mattn) build does not enable it.
	sqlite3.AutoExtension(fts5.Register)
}

// getDSN returns a DSN string for the SQLite database.
// writable is true if the database is writable, false otherwise.
// Source: https://github.com/nalgeon/redka/blob/017c0b28f7685311c3948b2e6a531012c8092bd3/internal/sqlx/db.go#L173
//
//nolint:revive // writable bool is essential: it selects between two distinct DSN construction paths (RO vs RW)
func getDSN(dbPath string, writable bool) string {
	dsn := "file:" + dbPath

	params := url.Values{}

	// The ncruces driver ignores _mutex; kept for parity with driver_cgo.go.
	params.Set("_mutex", "no")

	// Disable the shared cache.
	// https://sqlite.org/sharedcache.html
	// https://sqlite.org/uri.html#uricache
	params.Set("cache", "private")

	// The ncruces driver requires busy timeout and locking mode before other pragmas.
	// https://pkg.go.dev/github.com/ncruces/go-sqlite3/driver#hdr-Setting_PRAGMAs
	// synchronous and cache_size read the schema and need the busy handler when it is locked.
	// Set busy timeout to 10 seconds.
	// https://sqlite.org/pragma.html#pragma_busy_timeout
	// Source: sqlite3-builds src/auto_extension.c.
	params.Add("_pragma", "busy_timeout(10000)")

	// Set the locking mode to normal.
	// https://sqlite.org/pragma.html#pragma_locking_mode
	params.Add("_pragma", "locking_mode(NORMAL)")

	// Enable normal synchronous mode for better performance.
	// https://sqlite.org/pragma.html#pragma_synchronous
	params.Add("_pragma", "synchronous(NORMAL)")

	// Set the cache size to 1 GiB.
	// https://sqlite.org/pragma.html#pragma_cache_size
	// Source: sqlite3-builds src/auto_extension.c.
	params.Add("_pragma", "cache_size(-1048576)")

	// Limit retained journal size to 64 MiB.
	// https://sqlite.org/pragma.html#pragma_journal_size_limit
	// Source: sqlite3-builds src/auto_extension.c.
	params.Add("_pragma", "journal_size_limit(67108864)")

	// Checkpoint the WAL after 16000 pages.
	// https://sqlite.org/pragma.html#pragma_wal_autocheckpoint
	// Source: sqlite3-builds src/auto_extension.c.
	params.Add("_pragma", "wal_autocheckpoint(16000)")

	// Enable foreign key constraints.
	// https://sqlite.org/foreignkeys.html
	params.Add("_pragma", "foreign_keys(ON)")

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
		// Set auto_vacuum to incremental.
		// https://sqlite.org/pragma.html#pragma_auto_vacuum
		// auto_vacuum must precede journal_mode(WAL): this driver runs _pragma values in order,
		// and switching a new file to WAL writes its header, after which auto_vacuum changes only through VACUUM.
		params.Add("_pragma", "auto_vacuum(INCREMENTAL)")

		// Enable WAL mode for better performance.
		// https://sqlite.org/wal.html
		params.Add("_pragma", "journal_mode(WAL)")

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

// OpenDB opens a SQLite database at dbPath; set writable to true for the read-write connection.
func OpenDB(dbPath string, writable bool) (*sql.DB, error) {
	dsn := getDSN(dbPath, writable)
	connector, err := (&driver.SQLite{}).OpenConnector(dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	return sql.OpenDB(memoryConnector{connector}), nil
}
