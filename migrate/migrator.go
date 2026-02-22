// Package migrate provides SQL migration support for autoscan's embedded migration files.
package migrate

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/oriser/regroup"
)

// Migrator applies SQL migrations from an embedded filesystem to a database.
type Migrator struct {
	db  *sql.DB
	dir string

	re *regroup.ReGroup
}

/* Credits to https://github.com/Boostport/migration */

// New creates a Migrator for the given database and migration directory prefix.
func New(db *sql.DB, dir string) (*Migrator, error) {
	var err error

	migr := &Migrator{
		db:  db,
		dir: dir,
	}

	// verify schema
	if err = migr.verify(); err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}

	// compile migration regexp
	migr.re, err = regroup.Compile(`(?P<Version>\d+)\w?(?P<Name>.+)?\.sql`)
	if err != nil {
		return nil, fmt.Errorf("regexp: %w", err)
	}

	return migr, nil
}

// Migrate applies all pending up-migrations from the embedded FS for the given component.
func (m *Migrator) Migrate(fs *embed.FS, component string) error {
	// parse migrations
	migrations, err := m.parse(fs)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if len(migrations) == 0 {
		return nil
	}

	// get current migration versions
	versions, err := m.versions(component)
	if err != nil {
		return fmt.Errorf("versions: %v: %w", component, err)
	}

	// migrate
	for _, mg := range migrations {
		// already have this version?
		if _, exists := versions[mg.Version]; exists {
			continue
		}

		// migrate
		if err := m.exec(component, mg); err != nil {
			return fmt.Errorf("migrate: %v: %w", mg.Filename, err)
		}
	}

	return nil
}
