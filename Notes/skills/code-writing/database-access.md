---
name: database-access
description: Rules for safe, cancellable SQLite access through Go's database/sql and modernc.org/sqlite.
---

## SQLite access

- Follow the PRD's startup-validation order, cancellation phases, and terminal health classification. Do not replace these with generic retry behavior.
- Use `database/sql` with the exact vetted `modernc.org/sqlite` version required by the project. Keep production builds pure Go/no-cgo.
- Open explicit files with `mode=rw`; never create a database during validation and never silently degrade to read-only. Probe `PRAGMA schema_version` only after file and header validation succeeds.
- Set both `SetMaxOpenConns(2)` and `SetMaxIdleConns(2)`. Configure the five-second busy timeout and the exact 64 MiB connection-local `SQLITE_LIMIT_LENGTH` on every newly opened pooled connection.
- Check the original file's device and inode immediately before every database request and on every newly opened or replacement connection, then repeat health classification after request errors as required by the PRD.
- Use `context.Context` and `QueryContext`, `QueryRowContext`, `ExecContext`, or `BeginTx` for cancellable work. Always call a derived context's cancel function.
- Lease `*sql.Conn` when operations require connection identity, per-connection limits, SQLite interrupt behavior, or concurrent page/count requests. Always close the lease.
- Use `*sql.Tx` methods for transactional work; never mix SQL `BEGIN`/`COMMIT` statements with `database/sql` transaction methods. Treat a failed commit as unconfirmed and discard results as the PRD requires.
- Close `*sql.Rows`, check `rows.Err()` after iteration, and handle every scan, close, rollback, commit, and `RowsAffected` error at the correct boundary.
- Bind every user-entered value. Construct identifiers only from refreshed schema values and quote embedded double quotes; never interpolate user text into SQL identifiers or clauses.
- Do not add automatic retries around writes, lock failures, cancellation, or outcome-unknown commit errors. A retry can duplicate work or hide required lifecycle states.
- Preserve underlying OS and driver errors with `%w` where classification uses `errors.Is` or `errors.As`. Never classify failures by matching arbitrary driver error strings when a typed or wrapped cause exists.
- Add integration and release-capability tests for behavior that depends on SQLite or the driver, especially dedicated-connection cancellation, interrupt scope, limits, locking, commit boundaries, and file replacement.

## References

- [Accessing relational databases](https://go.dev/doc/database/)
- [Managing connections](https://go.dev/doc/database/manage-connections)
- [Executing transactions](https://go.dev/doc/database/execute-transactions)
- [Cancelling database operations](https://go.dev/doc/database/cancel-operations)
