package cli

import (
	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/d1"
)

// RunD1 is the D1 startup handler for Handlers.D1: it requests the sole
// candidate path from internal/d1's exact-rule discovery and passes that path
// unchanged to the shared Issue #2 pre-open validation, read-write open, and
// schema-probe flow in internal/connection. There is deliberately no
// D1-specific validation or SQLite-opening path here.
//
// Discovery failures carry typed outcomes from internal/d1; their exact
// diagnostics are defined by Issue #4 and rendered verbatim by Main as any
// other handler error.
func RunD1() error {
	return runD1With(d1.Discover, connection.Session)
}

// runD1With composes discovery with a shared opener, with both injected for
// tests so the handoff contract can be observed without filesystem fixtures.
func runD1With(discover func() (string, error), open func(path string) error) error {
	path, err := discover()
	if err != nil {
		return err
	}
	return open(path)
}
