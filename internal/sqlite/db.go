// Package sqlite provides a dual-connection SQLite database wrapper (read-only + read-write).
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// NewDB opens (or creates) a SQLite database at dbPath with separate read-only and read-write connections.
func NewDB(ctx context.Context, dbPath string) (*DB, error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o750); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	rwDB, err := OpenDB(dbPath, true)
	if err != nil {
		return nil, err
	}

	roDB, err := OpenDB(dbPath, false)
	if err != nil {
		return nil, err
	}

	db := &DB{
		roDB:   roDB,
		rwDB:   rwDB,
		dbPath: dbPath,
	}

	db.setConnectionCounts()

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	return db, nil
}

// setConnectionCounts calculates the optimal number
// of parallel connections to the database.
// Source: https://github.com/nalgeon/redka/blob/017c0b28f7685311c3948b2e6a531012c8092bd3/internal/sqlx/db.go#L85
func (db *DB) setConnectionCounts() {
	// For the read-only DB handle the number of open connections
	// should be equal to the number of idle connections. Otherwise,
	// the handle will keep opening and closing connections, severely
	// impacting the througput.
	sqlConnections := suggestConnectionCount()
	db.roDB.SetMaxIdleConns(sqlConnections)
	db.roDB.SetMaxOpenConns(sqlConnections)

	// SQLite allows only one writer at a time. Setting the maximum
	// number of DB connections to 1 for the read-write DB handle
	// is the best and fastest way to enforce this.
	db.rwDB.SetMaxIdleConns(1)
	db.rwDB.SetMaxOpenConns(1)
}

// Path returns the filesystem path of the database file.
func (db *DB) Path() string {
	return db.dbPath
}

// RO returns the read-only database connection.
func (db *DB) RO() *sql.DB {
	return db.roDB
}

// RW returns the read-write database connection.
func (db *DB) RW() *sql.DB {
	return db.rwDB
}

// PingContext verifies connectivity to the database using the given context.
// Only the read-write connection is checked; the read-only path would attempt
// a write (WAL initialization) and is therefore not suitable for pinging.
func (db *DB) PingContext(ctx context.Context) error {
	if err := db.rwDB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping read-write database: %w", err)
	}
	return nil
}

// Close closes both read-only and read-write database connections.
func (db *DB) Close() error {
	_ = db.roDB.Close()
	if err := db.rwDB.Close(); err != nil {
		return fmt.Errorf("close write db: %w", err)
	}
	return nil
}

// Stats returns database statistics from the read-only connection.
func (db *DB) Stats() sql.DBStats {
	// TODO: Combine stats from both roDB and rwDB.
	return db.roDB.Stats()
}
