package sqlite

import (
	"database/sql"
)

type (
	// DB holds dual SQLite connections: one read-only and one read-write.
	DB struct {
		roDB   *sql.DB
		rwDB   *sql.DB
		dbPath string
	}
)
