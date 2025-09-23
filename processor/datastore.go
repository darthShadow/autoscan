package processor

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/cloudbox/autoscan"
	"github.com/cloudbox/autoscan/internal/sqlite"
	"github.com/cloudbox/autoscan/migrate"
)

type datastore struct {
	db *sqlite.DB
}

//go:embed migrations
var migrations embed.FS

func newDatastore(db *sqlite.DB) (*datastore, error) {
	// Run migrations using the RW connection
	mg, err := migrate.New(db.RW(), "migrations")
	if err != nil {
		return nil, fmt.Errorf("create migrator: %w", err)
	}

	if err := mg.Migrate(&migrations, "processor"); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &datastore{db: db}, nil
}

const sqlUpsert = `
INSERT INTO scan (folder, relative_path, priority, time)
VALUES (?, ?, ?, ?)
ON CONFLICT (folder) DO UPDATE SET
	priority = MAX(excluded.priority, scan.priority),
    relative_path = excluded.relative_path,
	time = excluded.time
`

func (store *datastore) upsert(tx *sql.Tx, scan autoscan.Scan) error {
	_, err := tx.Exec(sqlUpsert, scan.Folder, scan.RelativePath, scan.Priority, scan.Time)
	return err
}

func (store *datastore) Upsert(scans []autoscan.Scan) error {
	// Early return for empty slice - no need to create transaction
	if len(scans) == 0 {
		return nil
	}

	tx, err := store.db.RW().Begin()
	if err != nil {
		return err
	}

	// Ensure transaction is always cleaned up
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // re-throw panic after cleanup
		} else if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, scan := range scans {
		if err = store.upsert(tx, scan); err != nil {
			return err // defer will handle rollback
		}
	}

	err = tx.Commit()
	return err
}

const sqlGetScansRemaining = `SELECT COUNT(folder) FROM scan`

func (store *datastore) GetScansRemaining() (int, error) {
	row := store.db.RO().QueryRow(sqlGetScansRemaining)

	remaining := 0
	err := row.Scan(&remaining)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return remaining, nil
	case err != nil:
		return remaining, fmt.Errorf("get remaining scans: %v: %w", err, autoscan.ErrFatal)
	}

	return remaining, nil
}

const sqlGetAvailableScan = `
SELECT folder, relative_path, priority, time FROM scan
WHERE time < ?
ORDER BY priority DESC, time ASC
LIMIT 1
`

func (store *datastore) GetAvailableScan(minAge time.Duration) (autoscan.Scan, error) {
	cutoff := now().Add(-1 * minAge).Unix()
	row := store.db.RO().QueryRow(sqlGetAvailableScan, cutoff)

	scan := autoscan.Scan{}
	err := row.Scan(&scan.Folder, &scan.RelativePath, &scan.Priority, &scan.Time)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return scan, autoscan.ErrNoScans
	case err != nil:
		return scan, fmt.Errorf("get matching: %s: %w", err, autoscan.ErrFatal)
	}

	return scan, nil
}

const sqlGetAll = `
SELECT folder, relative_path, priority, time FROM scan
`

func (store *datastore) GetAll() ([]autoscan.Scan, error) {
	rows, err := store.db.RO().Query(sqlGetAll)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var scans []autoscan.Scan
	for rows.Next() {
		scan := autoscan.Scan{}
		if err := rows.Scan(&scan.Folder, &scan.RelativePath, &scan.Priority, &scan.Time); err != nil {
			return nil, err
		}

		scans = append(scans, scan)
	}

	return scans, rows.Err()
}

const sqlDelete = `
DELETE FROM scan WHERE folder=?
`

func (store *datastore) Delete(scan autoscan.Scan) error {
	_, err := store.db.RW().Exec(sqlDelete, scan.Folder)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	return nil
}

var now = time.Now
