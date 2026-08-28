// Schema catalog plumbing: gathering main.sqlite_master rows, PRAGMA
// schema_version, and per-object PRAGMA table_xinfo rows through this DB's
// request boundary and decoding them with internal/schema's typed rules, per
// Issue #9 and the Schema module contract in Notes/PRD-sqloid.md. Every read
// happens inside one RunRequest so request-boundary identity checks, pooling,
// and cancellation apply exactly as to any other database request; health
// classification and typed refresh failures travel back in RequestResult so
// the UI can retain stale data and distinguish deletion/replacement.

package connection

import (
	"context"
	"database/sql"

	"github.com/chris/sqloid/internal/schema"
)

// ReadSchemaVersion reads the database's current PRAGMA schema_version as one
// cancellable request. It backs pre-execution revalidation (Issue #21): the
// caller compares the value against its cached Catalog.Version and only
// issues a catalog refresh when the version changed. Failures return the
// failed RequestResult with the operation error wrapped for cause inspection.
func (db *DB) ReadSchemaVersion(parent context.Context) (int64, RequestResult) {
	var version int64
	res := db.RunRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		if err := conn.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
			return wrapCatalog("read schema version", err)
		}
		return nil
	})
	return version, res
}

// ReadCatalog gathers one complete schema snapshot as a single cancellable
// request: PRAGMA schema_version, eligible main.sqlite_master objects in
// ascending name order, and each object's table_xinfo columns decoded by
// internal/schema.BuildCatalog. On success it returns the populated Catalog
// with an OutcomeSuccess result; failures return the zero Catalog plus the
// failed RequestResult whose Err (and possibly Health) already reflects the
// post-error identity reclassification rules. Cancelling ctx aborts the read
// through the leased connection.
func (db *DB) ReadCatalog(parent context.Context) (*schema.Catalog, RequestResult) {
	var cat *schema.Catalog

	res := db.RunRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		c, err := readCatalog(ctx, conn)
		if err != nil {
			return err
		}
		cat = c
		return nil
	})
	return cat, res
}

// readCatalog performs all schema reads on the already-leased connection.
// Object rows are fetched ORDER BY name so identical schemas always produce
// identical catalogs; column reads then follow the same deterministic order.
func readCatalog(ctx context.Context, conn *sql.Conn) (*schema.Catalog, error) {
	var version int64
	if err := conn.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
		return nil, wrapCatalog("read schema version", err)
	}

	rows, err := conn.QueryContext(ctx,
		"SELECT name, type, COALESCE(sql, '') FROM main.sqlite_master WHERE type IN ('table','view') ORDER BY name")
	if err != nil {
		return nil, wrapCatalog("read main.sqlite_master", err)
	}
	var master []schema.MasterRow
	for rows.Next() {
		var r schema.MasterRow
		if err := rows.Scan(&r.Name, &r.Type, &r.SQL); err != nil {
			rows.Close()
			return nil, wrapCatalog("scan sqlite_master row", err)
		}
		master = append(master, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, wrapCatalog("iterate main.sqlite_master", err)
	}
	rows.Close()

	columns := make(map[string][]schema.ColumnRow, len(master))
	for _, r := range master {
		colRows, err := readColumns(ctx, conn, r.Name)
		if err != nil {
			return nil, err
		}
		columns[r.Name] = colRows
	}

	return schema.BuildCatalog(schema.Input{Version: version, Master: master, Columns: columns}), nil
}

// readColumns fetches one object's declared columns from its table_xinfo as
// bound-parameter lookup against the pragma table-valued function, which both
// scopes the read to the main schema and keeps the object name out of SQL
// text entirely. Only consumed fields are selected.
func readColumns(ctx context.Context, conn *sql.Conn, name string) ([]schema.ColumnRow, error) {
	rows, err := conn.QueryContext(ctx,
		"SELECT name, type, hidden FROM main.pragma_table_xinfo(?)", name)
	if err != nil {
		return nil, wrapCatalog("read table_xinfo for "+name, err)
	}
	defer rows.Close()

	var out []schema.ColumnRow
	for rows.Next() {
		var cr schema.ColumnRow
		if err := rows.Scan(&cr.Name, &cr.DeclaredType, &cr.Hidden); err != nil {
			return nil, wrapCatalog("scan table_xinfo row for "+name, err)
		}
		out = append(out, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapCatalog("iterate table_xinfo for "+name, err)
	}
	return out, nil
}

// wrapCatalog adds operation context while preserving the driver cause via
// %w so callers can still inspect it with errors.Is/errors.As.
func wrapCatalog(what string, cause error) error {
	return &catalogError{what: what, cause: cause}
}

// catalogError marks one failing step of a schema-catalog read without
// claiming any terminal classification of its own: outcome classification
// stays in RequestResult and terminal wording stays with the UI owner.
type catalogError struct {
	what  string
	cause error
}

// Error returns lower-case diagnostic text naming the failed step and its
// cause.
func (e *catalogError) Error() string {
	return "could not refresh: " + e.what + ": " + e.cause.Error()
}

// Unwrap exposes the underlying cause for errors.Is/errors.As inspection.
func (e *catalogError) Unwrap() error { return e.cause }
