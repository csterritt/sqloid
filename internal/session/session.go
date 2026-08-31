// Package session owns the production composition root for Sqloid: it loads
// the initial schema catalog from an opened *connection.DB and constructs the
// fully wired ui.Model with thin adapters over the real database and
// filesystem implementations. Both the explicit `sqlite <file>` and the
// D1-discovered path flow through this one composition layer after
// connection.Open succeeds; no second startup or database-opening path
// exists here.
//
// The composition owns the *connection.DB for the session lifetime, loads the
// initial schema.Catalog synchronously before construction so an unreadable
// catalog stops the session before any Bubble Tea program starts, and
// installs the QueryBuilder's schema through ui.SchemaRefreshedMsg so the
// field bar reflects the real database from the first frame. Every database
// seam (schema-version read, catalog refresh, first-page SELECT, complete-
// result count, later paging, destructive estimate, transactional write) is
// wired to a thin adapter that calls the matching *connection.DB method and
// maps the typed connection.RequestResult onto the established UI typed
// result values, so no driver type ever leaks into internal/ui. The
// filesystem seams (PickerFS, SaveFS) are left nil so the model uses the real
// filepicker.OSFS and export.OSSaveFS implementations in production.
package session

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/schema"
	"github.com/chris/sqloid/internal/ui"
)

// Session is one composed production session: the retained *connection.DB,
// the loaded initial schema.Catalog, and the wired ui.Model. Close releases
// the database pool exactly once; the model and catalog stay readable after
// Close for inspection by tests and any post-run diagnostics.
type Session struct {
	db      *connection.DB
	catalog *schema.Catalog
	model   ui.Model
	closed  bool
}

// Compose loads the initial schema catalog from db synchronously and
// constructs the fully wired ui.Model with thin adapters over db and the real
// filesystem implementations. The caller retains ownership of db until
// Close; Compose never closes db itself, even on a catalog-load failure, so
// the caller controls the database lifecycle exactly. A catalog-load failure
// returns the wrapped cause so the CLI can render the exact one-line
// diagnostic and stop before any Bubble Tea program starts.
func Compose(db *connection.DB) (*Session, error) {
	cat, res := db.ReadCatalog(context.Background())
	if res.Outcome != connection.OutcomeSuccess {
		return nil, mapCatalogResult(res)
	}
	if cat == nil {
		return nil, errors.New("could not refresh: read catalog: nil catalog on success")
	}

	m := ui.New()
	// Install the loaded catalog through the established SchemaRefreshedMsg
	// seam so the QueryBuilder's eligible-object list and the field bar
	// reflect the real database from the first frame. This is the same
	// transition a successful Table-popup refresh takes, applied once at
	// startup before any user input.
	next, _ := m.Update(ui.SchemaRefreshedMsg{Catalog: cat})
	m = next.(ui.Model)

	m.History = history.NewStore()
	m.ResultHistory = history.NewResultStore()

	m.Select = selectAdapter(db)
	m.Count = countAdapter(db)
	m.Page = pageAdapter(db)
	m.VersionReader = versionAdapter(db)
	m.Refresher = refresherAdapter(db)
	m.Estimator = estimateAdapter(db)
	m.Write = writeAdapter(db)
	// PickerFS and SaveFS stay nil so the model resolves the real
	// filepicker.OSFS and export.OSSaveFS at use time.

	return &Session{db: db, catalog: cat, model: m}, nil
}

// Catalog returns the retained initial schema.Catalog loaded by Compose. It
// stays readable after Close.
func (s *Session) Catalog() *schema.Catalog { return s.catalog }

// Model returns the wired ui.Model. It stays readable after Close; callers
// must not issue database work through it after Close.
func (s *Session) Model() ui.Model { return s.model }

// Close releases the owned database pool exactly once. A second Close is a
// safe no-op. It does not close the model's history stores, which are
// in-memory and require no cleanup.
func (s *Session) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// mapCatalogResult converts the connection.RequestResult of an initial
// catalog load into the error the CLI renders verbatim. A typed
// *connection.HealthError keeps its chain so the UI's errors.As mapping
// still classifies terminal health; an ordinary failure surfaces the
// connection layer's already-prepared diagnostic.
func mapCatalogResult(res connection.RequestResult) error {
	if res.Err != nil {
		return res.Err
	}
	if res.Health != nil {
		return res.Health
	}
	return errors.New("could not refresh: read catalog: unknown failure")
}

// selectAdapter returns the first-page SELECT executor that runs one
// RunFirstPage through db and maps the typed connection.RequestResult onto
// ui.FirstPageResult. Health classifications stay in the error chain so the
// UI's errors.As mapping classifies them without parsing driver text.
func selectAdapter(db *connection.DB) ui.SelectExecutor {
	return func(ctx context.Context, sql string, params []any) ui.FirstPageResult {
		page, res := db.RunFirstPage(ctx, sql, params)
		return mapFirstPage(page, res)
	}
}

// pageAdapter returns the paged-page SELECT executor that runs one
// ExecutePage through db and maps the typed connection.RequestResult onto
// ui.FirstPageResult. offset is supplied by the QueryBuilder page API and is
// already encoded in the statement's LIMIT/OFFSET, so the adapter passes
// through the requested offset only for Issue #31 value-limit position
// reporting.
func pageAdapter(db *connection.DB) ui.PageExecutor {
	return func(ctx context.Context, sql string, params []any) ui.FirstPageResult {
		// The page SQL already carries the exact LIMIT/OFFSET range; offset
		// is informational for value-limit position reporting and is read
		// from the statement by the connection layer.
		page, res := db.ExecutePage(ctx, sql, params, 0)
		return mapFirstPage(page, res)
	}
}

// mapFirstPage converts one connection first-page or paged-page result into
// the UI's typed FirstPageResult, preserving the typed *result.LimitFailure
// and the *connection.HealthError chain. When the request failed at the
// lease boundary with only a Health classification (Err nil), the typed
// *connection.HealthError is surfaced as Err so the UI's errors.As mapping
// in healthTerminalFor classifies it without parsing driver text. A lease
// acquisition failure on a cancelled context is classified Cancelled so the
// UI's cancellation settlement stays inert.
func mapFirstPage(page *result.Page, res connection.RequestResult) ui.FirstPageResult {
	var lf *result.LimitFailure
	if res.Err != nil {
		var target *result.LimitFailure
		if errors.As(res.Err, &target) {
			lf = target
		}
	}
	err := res.Err
	if err == nil && res.Health != nil {
		err = res.Health
	}
	cancelled := res.Outcome == connection.OutcomeCancelled
	if !cancelled && res.Err != nil && errors.Is(res.Err, context.Canceled) {
		cancelled = true
	}
	return ui.FirstPageResult{
		Page:          page,
		Err:           err,
		Cancelled:     cancelled,
		LimitFailure:  lf,
		ByteTruncated: false, // byte-cap disclosure is owned by the cache layer
	}
}

// countAdapter returns the complete-result count executor that runs one
// RunCount through db and maps the typed connection.RequestResult onto
// ui.CountResult.
func countAdapter(db *connection.DB) ui.CountExecutor {
	return func(ctx context.Context, sql string, params []any) ui.CountResult {
		total, res := db.RunCount(ctx, sql, params)
		err := res.Err
		if err == nil && res.Health != nil {
			err = res.Health
		}
		cancelled := res.Outcome == connection.OutcomeCancelled
		if !cancelled && res.Err != nil && errors.Is(res.Err, context.Canceled) {
			cancelled = true
		}
		return ui.CountResult{
			Total:     total,
			Err:       err,
			Cancelled: cancelled,
		}
	}
}

// estimateAdapter returns the destructive matching-target estimate executor
// that runs one ExecuteEstimate through db and maps the typed
// connection.RequestResult onto ui.EstimateResult.
func estimateAdapter(db *connection.DB) ui.EstimateExecutor {
	return func(ctx context.Context, sql string, params []any) ui.EstimateResult {
		total, res := db.ExecuteEstimate(ctx, sql, params)
		err := res.Err
		if err == nil && res.Health != nil {
			err = res.Health
		}
		cancelled := res.Outcome == connection.OutcomeCancelled
		if !cancelled && res.Err != nil && errors.Is(res.Err, context.Canceled) {
			cancelled = true
		}
		return ui.EstimateResult{
			Total:     total,
			Err:       err,
			Cancelled: cancelled,
		}
	}
}

// versionAdapter returns the schema-version reader that runs one
// ReadSchemaVersion through db and maps the typed connection.RequestResult
// onto schema.VersionAttempt so the UI's pre-execution validation consumes
// the same typed outcomes it does in unit tests.
func versionAdapter(db *connection.DB) ui.VersionReader {
	return versionReader{db: db}
}

// versionReader is the ui.VersionReader implementation over db. It is a
// distinct type from refresher so the two seams can be replaced
// independently in tests.
type versionReader struct{ db *connection.DB }

// ReadSchemaVersion implements ui.VersionReader.
func (v versionReader) ReadSchemaVersion() schema.VersionAttempt {
	version, res := v.db.ReadSchemaVersion(context.Background())
	switch res.Outcome {
	case connection.OutcomeSuccess:
		return schema.NewVersionOK(version)
	case connection.OutcomeCancelled:
		// A cancelled version read is an ordinary failure for the validation
		// workflow's purposes: the preparation identity guards the late
		// settlement and the UI classifies it as cancelled at its boundary.
		return schema.NewVersionFailure(errors.New("schema version read cancelled"))
	default:
		if res.Health != nil {
			return healthVersionAttempt(res.Health)
		}
		return schema.NewVersionFailure(res.Err)
	}
}

// healthVersionAttempt maps a typed *connection.HealthError onto its terminal
// schema.VersionAttempt variant.
func healthVersionAttempt(he *connection.HealthError) schema.VersionAttempt {
	switch he.Kind {
	case connection.HealthDeleted:
		return schema.NewVersionDeleted()
	case connection.HealthReplaced:
		return schema.NewVersionReplaced()
	default:
		return schema.NewVersionFailure(he)
	}
}

// refresherAdapter returns the catalog refresher that runs one ReadCatalog
// through db and maps the typed connection.RequestResult onto
// schema.Attempt.
func refresherAdapter(db *connection.DB) ui.CatalogRefresher {
	return refresher{db: db}
}

// refresher is the ui.CatalogRefresher implementation over db.
type refresher struct{ db *connection.DB }

// RefreshCatalog implements ui.CatalogRefresher.
func (r refresher) RefreshCatalog() schema.Attempt {
	cat, res := r.db.ReadCatalog(context.Background())
	switch res.Outcome {
	case connection.OutcomeSuccess:
		return schema.NewSuccess(cat)
	case connection.OutcomeCancelled:
		return schema.NewFailure(errors.New("schema refresh cancelled"))
	default:
		if res.Health != nil {
			return healthAttempt(res.Health)
		}
		return schema.NewFailure(res.Err)
	}
}

// healthAttempt maps a typed *connection.HealthError onto its terminal
// schema.Attempt variant.
func healthAttempt(he *connection.HealthError) schema.Attempt {
	switch he.Kind {
	case connection.HealthDeleted:
		return schema.NewTerminal(schema.RefreshDeleted)
	case connection.HealthReplaced:
		return schema.NewTerminal(schema.RefreshReplaced)
	default:
		return schema.NewFailure(he)
	}
}

// writeAdapter returns the transactional write executor that runs one
// StartWrite through db, relays every phase transition through the supplied
// phase callback, blocks until settlement, and returns the typed
// connection.WriteResult unchanged. The phase stream is drained fully before
// the result is returned so the UI observes the complete phase history in
// order, exactly as it does inside a tea.Cmd.
func writeAdapter(db *connection.DB) ui.WriteExecutor {
	return func(ctx context.Context, execution uint64, sql string, params []any, phase func(connection.WritePhaseMsg)) connection.WriteResult {
		w := db.StartWrite(ctx, execution, sql, params)
		for msg := range w.Phases() {
			if phase != nil {
				phase(msg)
			}
		}
		return w.Wait()
	}
}

// RunSQLite is the CLI-facing sqlite command handler: it opens path through
// connection.Open, composes the production session, runs the Bubble Tea
// program over the wired model until it quits, and then closes the session
// in the reverse order — program teardown before database pool release. A
// startup or catalog failure returns the connection layer's already-prepared
// one-line diagnostic for the CLI to render verbatim with exit status 1; no
// session is constructed on failure.
func RunSQLite(path string) error {
	return RunSQLiteWith(path, defaultRunner, nil)
}

// RunSQLiteWith is the testable production session path: it opens path
// through connection.Open, composes the session, invokes run with the wired
// model, and then calls closeHook (or the session's own Close when closeHook
// is nil) in the reverse order — program teardown before database pool
// release. run is the injected program runner (tea.Program.Run in
// production); closeHook is the observable close hook for tests (nil means
// use the session's own Close). A startup or catalog failure returns the
// connection layer's diagnostic and never invokes run; a runner error is
// returned to the caller after the session is closed.
func RunSQLiteWith(path string, run func(tea.Model) (tea.Model, error), closeHook func() error) error {
	db, err := connection.Open(path)
	if err != nil {
		return err
	}
	s, err := Compose(db)
	if err != nil {
		// Compose failed before the session took ownership: release the
		// database pool directly so no handle leaks.
		_ = db.Close()
		return err
	}
	_, runErr := run(s.Model())
	closeErr := s.closeWith(closeHook)
	if runErr != nil {
		return runErr
	}
	return closeErr
}

// closeWith closes the session through closeHook when non-nil, otherwise
// through the session's own Close. It is the single close boundary for the
// RunSQLiteWith lifecycle.
func (s *Session) closeWith(closeHook func() error) error {
	if closeHook != nil {
		return closeHook()
	}
	return s.Close()
}

// defaultRunner is the production tea.Program.Run adapter: it constructs the
// program over m, runs it until it quits, and returns the final model and
// run error.
func defaultRunner(m tea.Model) (tea.Model, error) {
	prog := tea.NewProgram(m)
	final, err := prog.Run()
	return final, err
}
