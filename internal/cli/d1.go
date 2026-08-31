package cli

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/d1"
	"github.com/chris/sqloid/internal/session"
)

// zeroCandidateHint is the exact second diagnostic line required by Issue #4
// and the D1 discovery section of Notes/PRD-sqloid.md: it names the expected
// working-directory-relative Wrangler path and gives explicit-open recovery
// guidance. It appears only for zero-candidate outcomes.
const zeroCandidateHint = "Expected " + d1.Dir + "; your Wrangler version may use a different local-state layout. Use sqloid sqlite <file> to open the database explicitly."

// RunD1 is the D1 startup handler for Handlers.D1: it requests the sole
// candidate path from internal/d1's exact-rule discovery, maps every typed
// discovery failure onto its exact Issue #4 diagnostic without calling the
// opener, and passes a discovered path unchanged to the shared Issue #2
// pre-open validation, read-write open, schema-probe, and production
// composition flow in internal/connection and internal/session. There is
// deliberately no D1-specific validation or SQLite-opening path here.
//
// Discovery failures carry typed outcomes from internal/d1 and are mapped
// here to the exact Issue #4 diagnostics before Main renders them verbatim
// with exit status 1; no database target is created for any of them.
func RunD1() error {
	return runD1With(d1.Discover, session.RunSQLite)
}

// RunD1WithRunner is the testable D1 handler: it uses the injected program
// runner (and nil close hook, which means the session's own Close) instead of
// tea.NewProgram so the discovery → open → compose → run → close lifecycle
// can be exercised without a real TTY.
func RunD1WithRunner(run func(tea.Model) (tea.Model, error)) func() error {
	return func() error {
		return runD1With(d1.Discover, func(path string) error {
			return session.RunSQLiteWith(path, run, nil)
		})
	}
}

// runD1With composes discovery with a shared opener, with both injected for
// tests so the handoff contract can be observed without filesystem fixtures.
// A discovery failure returns the mapped diagnostic and never invokes open,
// so failed startup bypasses internal/connection entirely and creates no
// database target.
func runD1With(discover func() (string, error), open func(path string) error) error {
	path, err := discover()
	if err != nil {
		return mapDiscoveryDiagnostic(err)
	}
	return open(path)
}

// mapDiscoveryDiagnostic converts typed internal/d1 outcomes into the exact
// user-facing diagnostics owned by internal/cli (Issue #4): zero candidates
// yield exactly two lines — the typed message plus the expected-path hint with
// explicit-open guidance; multiple candidates yield only the PRD's single
// line with no layout hint. Unknown errors pass through unchanged.
func mapDiscoveryDiagnostic(err error) error {
	switch {
	case errors.Is(err, d1.ErrNoCandidate):
		return errors.New(d1.ErrNoCandidate.Error() + "\n" + zeroCandidateHint)
	case errors.Is(err, d1.ErrMultipleCandidates):
		// The sentinel text is lower-case by Go convention; the PRD-mandated
		// diagnostic capitalizes the first word, so it is spelled out here.
		return errors.New("There is more than one SQLite database in .wrangler")
	default:
		return err
	}
}
