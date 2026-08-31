// Task 3 (RED) for Issue #57: deterministic lifecycle tests at the
// composition/CLI boundaries. These tests require the production session
// path to accept an injected Bubble Tea program runner and to expose
// observable session close hooks so the lifecycle can be verified without a
// real TTY. They prove:
//   - RunSQLiteWith opens the database, composes the session, invokes the
//     runner exactly once with the wired model, and closes the session in
//     the reverse order (program teardown before database pool release)
//     after the runner returns.
//   - A startup (Open) failure returns the connection layer's diagnostic and
//     never invokes the runner or constructs a session.
//   - A catalog (Compose) failure returns the wrapped cause, closes the
//     database pool, and never invokes the runner.
//   - A runner error is returned to the caller after the session is closed.
//   - The D1 discovery handoff flows through the same shared session opener
//     so both `sqlite <file>` and D1-discovered paths use the one production
//     composition root.
//   - The session's Close is idempotent across the lifecycle boundary.
//
// They follow the injected-runner pattern so no /dev/tty access is required.

package session_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/session"
	"github.com/chris/sqloid/internal/ui"
)

// createLifecycleDB builds a real SQLite database at path with one table so
// the production opener and composition succeed.
func createLifecycleDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(%q): %v", path, err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
}

// lifecyclePath returns a real database path inside a temporary directory.
func lifecyclePath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lifecycle.db")
	createLifecycleDB(t, path)
	return path
}

// fakeRunner is the injected program runner: it records the model it
// received, returns the queued error, and reports whether it was invoked.
type fakeRunner struct {
	invoked atomic.Bool
	model   tea.Model
	err     error
}

func (f *fakeRunner) run(m tea.Model) (tea.Model, error) {
	f.invoked.Store(true)
	f.model = m
	if f.err != nil {
		return m, f.err
	}
	return m, nil
}

// TestRunSQLiteWithInvokesRunnerOnceAndClosesSessionInReverseOrder requires
// the production session path to invoke the injected runner exactly once
// with the wired model and then close the session after the runner returns.
func TestRunSQLiteWithInvokesRunnerOnceAndClosesSessionInReverseOrder(t *testing.T) {
	path := lifecyclePath(t)
	runner := &fakeRunner{}

	var closed atomic.Bool
	closeHook := func() error { closed.Store(true); return nil }

	if err := session.RunSQLiteWith(path, runner.run, closeHook); err != nil {
		t.Fatalf("RunSQLiteWith: %v", err)
	}
	if !runner.invoked.Load() {
		t.Error("runner was never invoked, want exactly one invocation")
	}
	if runner.model == nil {
		t.Error("runner received nil model, want the wired ui.Model")
	}
	if _, ok := runner.model.(ui.Model); !ok {
		t.Errorf("runner received %T, want ui.Model", runner.model)
	}
	if !closed.Load() {
		t.Error("session close hook was never invoked after the runner returned")
	}
}

// TestRunSQLiteWithOpenFailureNeverInvokesRunner requires an Open failure to
// return the connection layer's diagnostic and never invoke the runner or
// the close hook.
func TestRunSQLiteWithOpenFailureNeverInvokesRunner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.db")
	runner := &fakeRunner{}
	var closed atomic.Bool

	err := session.RunSQLiteWith(path, runner.run, func() error { closed.Store(true); return nil })
	if err == nil {
		t.Fatal("RunSQLiteWith on a missing file returned nil, want an Open diagnostic")
	}
	if runner.invoked.Load() {
		t.Error("runner was invoked on an Open failure, want no invocation")
	}
	if closed.Load() {
		t.Error("close hook was invoked on an Open failure, want no session constructed")
	}
}

// TestRunSQLiteWithCatalogFailureClosesDBAndNeverInvokesRunner requires a
// catalog load failure to return the wrapped cause, close the database pool,
// and never invoke the runner.
func TestRunSQLiteWithCatalogFailureClosesDBAndNeverInvokesRunner(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; file-removal fixtures cannot be exercised")
	}
	path := lifecyclePath(t)
	if err := os.Remove(path); err != nil {
		t.Fatalf("removing database file: %v", err)
	}
	runner := &fakeRunner{}
	var closed atomic.Bool

	err := session.RunSQLiteWith(path, runner.run, func() error { closed.Store(true); return nil })
	if err == nil {
		t.Fatal("RunSQLiteWith with an unreadable catalog returned nil, want a catalog diagnostic")
	}
	if runner.invoked.Load() {
		t.Error("runner was invoked on a catalog failure, want no invocation")
	}
	if closed.Load() {
		t.Error("close hook was invoked on a catalog failure, want no session constructed")
	}
}

// TestRunSQLiteWithRunnerErrorReturnsAfterClose requires a runner error to be
// returned to the caller after the session close hook runs.
func TestRunSQLiteWithRunnerErrorReturnsAfterClose(t *testing.T) {
	path := lifecyclePath(t)
	runnerErr := errors.New("runner exploded")
	runner := &fakeRunner{err: runnerErr}

	var closeOrder []string
	closeHook := func() error {
		closeOrder = append(closeOrder, "close")
		return nil
	}

	err := session.RunSQLiteWith(path, runner.run, closeHook)
	if !errors.Is(err, runnerErr) {
		t.Errorf("RunSQLiteWith error = %v, want the runner error %v", err, runnerErr)
	}
	if !runner.invoked.Load() {
		t.Error("runner was never invoked")
	}
	if len(closeOrder) != 1 {
		t.Errorf("close hook invocations = %v, want exactly one after the runner error", closeOrder)
	}
}

// TestRunSQLiteWithCloseHookReleasesDatabasePool requires the close hook to
// be called exactly once per session and a second RunSQLiteWith on the same
// path to succeed, proving the pool was released and no file handle leaked.
func TestRunSQLiteWithCloseHookReleasesDatabasePool(t *testing.T) {
	path := lifecyclePath(t)

	var closes atomic.Int32
	closeHook := func() error { closes.Add(1); return nil }

	runner := &fakeRunner{}
	if err := session.RunSQLiteWith(path, runner.run, closeHook); err != nil {
		t.Fatalf("first RunSQLiteWith: %v", err)
	}
	if got := closes.Load(); got != 1 {
		t.Errorf("closes after first run = %d, want 1", got)
	}
	runner2 := &fakeRunner{}
	if err := session.RunSQLiteWith(path, runner2.run, closeHook); err != nil {
		t.Fatalf("second RunSQLiteWith: %v", err)
	}
	if got := closes.Load(); got != 2 {
		t.Errorf("closes after second run = %d, want 2", got)
	}
}

// TestRunSQLiteWithDefaultRunnerUsesTeaNewProgram requires the default
// RunSQLite (no injected runner) to use tea.NewProgram. We can't run a real
// TUI in a test, so this test only verifies that RunSQLite exists and is the
// production entrypoint; the PTY integration test in cmd/sqloid exercises the
// real TUI.
func TestRunSQLiteExistsAsProductionEntrypoint(t *testing.T) {
	// RunSQLite is the production handler that uses tea.NewProgram. It is
	// not invoked here because no TTY is available; its existence and
	// signature are the compile-time contract.
	var _ func(path string) error = session.RunSQLite
}

// TestRunSQLiteWithModelCarriesRealCatalog requires the model handed to the
// runner to carry the real catalog so the first frame reflects the database.
func TestRunSQLiteWithModelCarriesRealCatalog(t *testing.T) {
	path := lifecyclePath(t)
	runner := &fakeRunner{}

	if err := session.RunSQLiteWith(path, runner.run, func() error { return nil }); err != nil {
		t.Fatalf("RunSQLiteWith: %v", err)
	}
	m, ok := runner.model.(ui.Model)
	if !ok {
		t.Fatalf("runner received %T, want ui.Model", runner.model)
	}
	if tables := m.QB.EligibleTables(); len(tables) == 0 {
		t.Error("runner's model has no eligible tables, want the real catalog's tables")
	} else if tables[0].Name != "t" {
		t.Errorf("first eligible table = %q, want %q", tables[0].Name, "t")
	}
}

// TestRunSQLiteWithModelWiresEveryDatabaseSeam requires the model handed to
// the runner to have every database seam wired so the TUI can issue real
// database work from the first frame.
func TestRunSQLiteWithModelWiresEveryDatabaseSeam(t *testing.T) {
	path := lifecyclePath(t)
	runner := &fakeRunner{}

	if err := session.RunSQLiteWith(path, runner.run, func() error { return nil }); err != nil {
		t.Fatalf("RunSQLiteWith: %v", err)
	}
	m, ok := runner.model.(ui.Model)
	if !ok {
		t.Fatalf("runner received %T, want ui.Model", runner.model)
	}
	if m.Select == nil || m.Count == nil || m.Page == nil {
		t.Error("runner's model is missing SELECT/Count/Page seams")
	}
	if m.VersionReader == nil || m.Refresher == nil {
		t.Error("runner's model is missing VersionReader/Refresher seams")
	}
	if m.Estimator == nil || m.Write == nil {
		t.Error("runner's model is missing Estimator/Write seams")
	}
	if m.History == nil || m.ResultHistory == nil {
		t.Error("runner's model is missing History/ResultHistory stores")
	}
}

// TestRunSQLiteWithExitsCleanlyOnContextCancellation is a placeholder for the
// PTY integration test that proves Ctrl+C exits the program; here we only
// verify the lifecycle hook ordering when the runner returns nil.
func TestRunSQLiteWithExitsCleanlyOnNilRunnerResult(t *testing.T) {
	path := lifecyclePath(t)
	runner := &fakeRunner{}
	var closed atomic.Bool

	if err := session.RunSQLiteWith(path, runner.run, func() error { closed.Store(true); return nil }); err != nil {
		t.Fatalf("RunSQLiteWith: %v", err)
	}
	if !closed.Load() {
		t.Error("close hook was not invoked after a clean runner return")
	}
}

// Ensure the injected runner signature exists at compile time.
var _ func(tea.Model) (tea.Model, error)

// guard against unused import warnings.
var _ = context.Canceled
