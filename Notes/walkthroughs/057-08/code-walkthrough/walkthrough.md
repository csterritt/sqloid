# Issue #057 Code Walkthrough: Production TUI Composition and Binary Smoke Path

*2026-08-31T15:40:52Z by Showboat 0.6.1*
<!-- showboat-id: 54adc080-6883-463c-b624-98e8e29b23e5 -->

Issue #57 (Notes/issues/057-production-tui-composition.md, Notes/PRD-sqloid.md Module Design and Testing Decisions) lands the production application-composition path that was missing between `cmd/sqloid/main.go`, `internal/connection/startup.go`, and `internal/ui`. Before this issue the CLI handlers opened the database and returned silently after validation — `connection.Session` deferred `db.Close()` and exited without constructing any UI, wiring any executor, or running any Bubble Tea program. Every module test passed because it tested disconnected components with fake seams, so the missing composition root was invisible at the package level. Issue #57 lands the production path that makes the v1 TUI, execution, history, and export stories reachable from the shipped binary.

The composition root lives in a new `internal/session` package. First, the package itself and its single source file.

```bash
ls internal/session/ && echo '---' && wc -l internal/session/*.go
```

```output
compose_test.go
lifecycle_test.go
session.go
---
  582 internal/session/compose_test.go
  281 internal/session/lifecycle_test.go
  387 internal/session/session.go
 1250 total
```

The `Compose` constructor is the one composition root: it loads the initial schema catalog synchronously, installs it through `SchemaRefreshedMsg`, wires every database seam to a thin adapter over `db`, and leaves the filesystem seams nil so the real implementations are used.

```bash
sed -n '38,90p' internal/session/session.go
```

```output
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
```

The Select adapter shows the typed-outcome mapping: `errors.Is(err, context.Canceled)` classifies lease-boundary cancellation as `Cancelled`, and `*connection.HealthError` is surfaced as `Err` when `Err` is nil but `Health` is set so the UI's `errors.As` mapping in `healthTerminalFor` classifies it without parsing driver text.

```bash
sed -n '140,185p' internal/session/session.go
```

```output
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
```

The CLI lifecycle is testable: `RunSQLiteWith` accepts an injected program runner and observable close hook so the open → compose → run → close ordering can be verified without a real TTY. `RunSQLite` is the production handler that uses `tea.NewProgram`.

```bash
sed -n '335,395p' internal/session/session.go
```

```output
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
}```
```

`cmd/sqloid/main.go` now wires `session.RunSQLite` for the sqlite command and `cli.RunD1` for D1; `cli.RunD1` delegates to `runD1With(d1.Discover, session.RunSQLite)` so both startup modes flow through the one composition root.

```bash
cat cmd/sqloid/main.go && echo '---' && grep -n 'RunD1\|runD1With\|session.RunSQLite' internal/cli/d1.go
```

```output
// Command sqloid is the executable entrypoint for the Sqloid CLI. It maps the
// exit status returned by the internal/cli shell onto the process and supplies
// the sqlite command's production composition handler and the d1 command's
// discovery handler; all other construction and dispatch live in internal/cli
// and internal/session.
package main

import (
	"os"

	"github.com/chris/sqloid/internal/cli"
	"github.com/chris/sqloid/internal/session"
)

func main() {
	handlers := cli.Handlers{
		SQLite: session.RunSQLite,
		D1:     cli.RunD1,
	}
	os.Exit(cli.Main(os.Args, handlers))
}
---
18:// RunD1 is the D1 startup handler for Handlers.D1: it requests the sole
29:func RunD1() error {
30:	return runD1With(d1.Discover, session.RunSQLite)
33:// RunD1WithRunner is the testable D1 handler: it uses the injected program
37:func RunD1WithRunner(run func(tea.Model) (tea.Model, error)) func() error {
39:		return runD1With(d1.Discover, func(path string) error {
40:			return session.RunSQLiteWith(path, run, nil)
45:// runD1With composes discovery with a shared opener, with both injected for
50:func runD1With(discover func() (string, error), open func(path string) error) error {
```

The PTY integration test builds the real binary and runs it under `github.com/creack/pty`. It responds to Bubble Tea's `\x1b[6n` cursor-position-report request so the UI renders, verifies the builder's `Command` field appears, sends `q`+Enter to confirm the universal quit, and asserts exit status 0.

```bash
sed -n '40,120p' cmd/sqloid/pty_integration_test.go
```

```output
	binPath := filepath.Join(t.TempDir(), "sqloid")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/sqloid")
	build.Dir = projectRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	t.Cleanup(func() { os.Remove(binPath) })

	// Create a real SQLite database with a table and some rows.
	dbPath := filepath.Join(t.TempDir(), "pty_test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec("INSERT INTO users (id, name) VALUES (1, 'alice'), (2, 'bob')"); err != nil {
		t.Fatalf("insert rows: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	// Spawn the binary under a PTY with a fixed terminal size.
	cmd := exec.Command(binPath, "sqlite", dbPath)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 30, Cols: 100})
	if err != nil {
		t.Fatalf("pty.StartWithSize: %v", err)
	}
	t.Cleanup(func() {
		ptmx.Close()
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	})

	// Read the PTY output in a goroutine, responding to Bubble Tea's
	// terminal capability queries so the UI renders. Bubble Tea sends
	// \x1b[6n (cursor position report request) on startup and blocks until
	// a response arrives; we respond with \x1b[1;1R (cursor at row 1, col
	// 1) so rendering proceeds.
	var output bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				output.Write(buf[:n])
				// Respond to the cursor position report request so Bubble
				// Tea proceeds to render.
				if bytes.Contains(buf[:n], []byte("\x1b[6n")) {
					ptmx.Write([]byte("\x1b[1;1R"))
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for the initial builder view to render — the "Command" field
	// label that the startup model focuses must appear in the output.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), "Command") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(output.String(), "Command") {
		t.Fatalf("PTY output did not contain the builder's Command field within timeout.\nOutput:\n%s", output.String())
	}

	// Send `q` to open the universal quit confirmation, then `Enter` to
	// confirm and exit.
	if _, err := ptmx.Write([]byte("q")); err != nil {
		t.Fatalf("write q: %v", err)
```

```bash
sed -n '330,387p' internal/session/session.go | head -55
```

```output

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
```
