package sqlite

import (
	"database/sql"
)

type (
	DB struct {
		roDB   *sql.DB
		rwDB   *sql.DB
		dbPath string
	}
)
