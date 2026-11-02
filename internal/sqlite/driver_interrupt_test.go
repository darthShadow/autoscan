//go:build !cgo

package sqlite

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
)

func TestDriverInterruptCarriesContextEnd(t *testing.T) {
	for _, operation := range []string{"read", "write", "prepared write"} {
		t.Run(operation, func(t *testing.T) {
			db, err := OpenDB(filepath.Join(t.TempDir(), "interrupt.db"), true)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			conn, err := db.Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			if _, err = conn.ExecContext(t.Context(), "CREATE TABLE results (value INTEGER)"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var calls int
			if err = conn.Raw(func(raw any) error {
				native, ok := raw.(driver.Conn)
				if !ok {
					return fmt.Errorf("connection = %T, want driver.Conn", raw)
				}
				return native.Raw().CreateFunction("cancel_step", 1, 0,
					func(result sqlite3.Context, args ...sqlite3.Value) {
						calls++
						if calls == 1000 {
							cancel()
						}
						result.ResultInt64(args[0].Int64())
					})
			}); err != nil {
				t.Fatal(err)
			}
			const query = `WITH RECURSIVE numbers(n) AS (
 VALUES(1) UNION ALL SELECT n+1 FROM numbers WHERE n<1000000
) SELECT sum(cancel_step(n)) FROM numbers`
			switch operation {
			case "read":
				err = readDriverStep(ctx, conn, query)
			case "write", "prepared write":
				tx, beginErr := conn.BeginTx(ctx, nil)
				if beginErr != nil {
					t.Fatal(beginErr)
				}
				defer func() { _ = tx.Rollback() }()
				const insert = "INSERT INTO results " + query
				if operation == "prepared write" {
					stmt, prepareErr := tx.PrepareContext(ctx, insert)
					if prepareErr != nil {
						t.Fatal(prepareErr)
					}
					defer func() { _ = stmt.Close() }()
					_, err = stmt.ExecContext(ctx)
				} else {
					_, err = tx.ExecContext(ctx, insert)
				}
			default:
				t.Fatalf("unsupported operation %q", operation)
			}
			if calls < 1000 {
				t.Fatalf("query made %d calls, want cancellation inside a step after 1000 calls", calls)
			}
			if !errors.Is(err, context.Canceled) || !errors.Is(err, sqlite3.INTERRUPT) {
				t.Errorf("query error = %v, want both context.Canceled and sqlite3.INTERRUPT", err)
			}
		})
	}
}

// Read driver rows through Raw so database/sql's concurrent cancellation
// cannot replace the driver's error before the assertion.
func readDriverStep(ctx context.Context, conn *sql.Conn, query string) error {
	err := conn.Raw(func(raw any) error {
		native, ok := raw.(driver.Conn)
		if !ok {
			return fmt.Errorf("connection = %T, want driver.Conn", raw)
		}
		stmt, err := native.PrepareContext(ctx, query)
		if err != nil {
			return fmt.Errorf("prepare read: %w", err)
		}
		defer func() { _ = stmt.Close() }()
		queryStmt, ok := stmt.(sqldriver.StmtQueryContext)
		if !ok {
			return fmt.Errorf("statement = %T, want driver.StmtQueryContext", stmt)
		}
		rows, err := queryStmt.QueryContext(ctx, nil)
		if err != nil {
			return fmt.Errorf("query read: %w", err)
		}
		defer func() { _ = rows.Close() }()
		return rows.Next(make([]sqldriver.Value, 1))
	})
	if err != nil {
		return fmt.Errorf("read driver step: %w", err)
	}
	return nil
}
