package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

func NewDB(ctx context.Context, dbPath string) (*DB, error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
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

	if err := db.Ping(); err != nil {
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

func (db *DB) Path() string {
	return db.dbPath
}

func (db *DB) RO() *sql.DB {
	return db.roDB
}

func (db *DB) RW() *sql.DB {
	return db.rwDB
}

func (db *DB) Ping() error {
	// Can't use db.roDB.Ping() here because it will try to write to the database.
	if err := db.rwDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping read-write database: %w", err)
	}
	return nil
}

func (db *DB) Close() error {
	_ = db.roDB.Close()
	return db.rwDB.Close()
}

func (db *DB) Stats() sql.DBStats {
	// TODO: Combine stats from both roDB and rwDB.
	return db.roDB.Stats()
}
