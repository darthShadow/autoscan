package processor

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/cloudbox/autoscan"
	"github.com/cloudbox/autoscan/migrate"

	// sqlite3 driver
	_ "modernc.org/sqlite"
)

type datastore struct {
	*sql.DB
}

var (
	//go:embed migrations
	migrations embed.FS
)

func newDatastore(db *sql.DB, mg *migrate.Migrator) (*datastore, error) {
	// migrations
	if err := mg.Migrate(&migrations, "processor"); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &datastore{db}, nil
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
	tx, err := store.Begin()
	if err != nil {
		return err
	}

	for _, scan := range scans {
		if err = store.upsert(tx, scan); err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				panic(rollbackErr)
			}

			return err
		}
	}

	return tx.Commit()
}

const sqlGetScansRemaining = `SELECT COUNT(folder) FROM scan`

func (store *datastore) GetScansRemaining() (int, error) {
	row := store.QueryRow(sqlGetScansRemaining)

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
	row := store.QueryRow(sqlGetAvailableScan, now().Add(-1*minAge))

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

func (store *datastore) GetAll() (scans []autoscan.Scan, err error) {
	rows, err := store.Query(sqlGetAll)
	if err != nil {
		return scans, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		scan := autoscan.Scan{}
		err = rows.Scan(&scan.Folder, &scan.RelativePath, &scan.Priority, &scan.Time)
		if err != nil {
			return scans, err
		}

		scans = append(scans, scan)
	}

	return scans, rows.Err()
}

const sqlDelete = `
DELETE FROM scan WHERE folder=?
`

func (store *datastore) Delete(scan autoscan.Scan) error {
	_, err := store.Exec(sqlDelete, scan.Folder)
	if err != nil {
		return fmt.Errorf("delete: %s: %w", err, autoscan.ErrFatal)
	}

	return nil
}

var now = time.Now
