package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"
	"sort"
)

type migration struct {
	Version  int    `regroup:"Version"`
	Name     string `regroup:"Name"`
	Filename string
	Schema   string
}

func (m *Migrator) verify() error {
	if _, err := m.db.ExecContext(context.Background(), sqlSchema); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	return nil
}

func (m *Migrator) versions(component string) (result map[int]bool, err error) {
	rows, err := m.db.QueryContext(context.Background(), sqlVersions, component)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("rows close: %w", cerr)
		}
	}()

	result = make(map[int]bool)
	for rows.Next() {
		var version int
		if err = rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}

		result[version] = true
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return result, nil
}

func (m *Migrator) exec(component string, migration *migration) (err error) {
	// begin tx
	tx, err := m.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	// commit - rollback
	defer func(tx *sql.Tx) {
		// roll back
		if err != nil {
			if errRb := tx.Rollback(); errRb != nil {
				err = fmt.Errorf("rollback: %w: %w", errRb, err)
			}
			return
		}

		// commit
		if errCm := tx.Commit(); errCm != nil {
			err = fmt.Errorf("commit: %w", errCm)
		}
	}(tx)

	// exec migration
	if _, err := tx.ExecContext(context.Background(), migration.Schema); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	// insert migration version
	if _, err := tx.ExecContext(context.Background(), sqlInsertVersion, component, migration.Version); err != nil {
		return fmt.Errorf("schema_migration: %w", err)
	}

	return nil
}

func (m *Migrator) parse(fsys *embed.FS) ([]*migration, error) {
	// parse migrations from filesystem
	files, err := fsys.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	// parse migrations
	migrations := make([]*migration, 0)
	for _, file := range files {
		// skip dirs
		if file.IsDir() {
			continue
		}

		// parse migration
		mig := new(migration)
		if err := m.re.MatchToTarget(file.Name(), mig); err != nil {
			return nil, fmt.Errorf("parse migration: %w", err)
		}

		b, err := fsys.ReadFile(filepath.Join(m.dir, file.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration: %w", err)
		}
		mig.Schema = string(b)
		mig.Filename = file.Name()

		// set migration
		migrations = append(migrations, mig)
	}

	// sort migrations
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

const (
	sqlSchema = "CREATE TABLE IF NOT EXISTS schema_migration" +
		" (component VARCHAR(255) NOT NULL, version INTEGER NOT NULL," +
		" PRIMARY KEY (component, version))"
	sqlVersions      = `SELECT version FROM schema_migration WHERE component = ?`
	sqlInsertVersion = `INSERT INTO schema_migration (component, version) VALUES (?, ?)`
)
