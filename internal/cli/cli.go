// Package cli implements the Sqloid command-line shell: command routing,
// argument parsing, help, and version handling on top of the PRD-mandated
// mow.cli command structure.
//
// The package owns the `sqlite <file>` and `d1` command contracts from
// Notes/PRD-sqloid.md and Issue #1, plus the startup-failure rendering from
// Issue #2: each command handler may return an error whose Error() is already
// the exact one-line diagnostic (internal/connection guarantees this), which
// is printed verbatim on stderr with exit status 1. Successful dispatch stays
// silent.
package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/jawher/mow.cli"
)

// Version is the build version string reported by the version flags. It is a
// variable so release builds can override it at link time with
// -ldflags "-X github.com/chris/sqloid/internal/cli.Version=<version>".
var Version = "dev"

// Handlers holds the functions invoked when a routed command reaches startup.
// A nil handler is treated as not yet wired and does nothing. A non-nil
// handler that returns an error must have prepared its Error() string as the
// exact user-facing diagnostic; the shell prints it unmodified.
type Handlers struct {
	// SQLite opens the database at the path passed as the `sqlite <file>`
	// argument. It is called only after successful routing.
	SQLite func(path string) error
	// D1 starts discovery of the local D1 database. It is called only after
	// successful routing.
	D1 func() error
}

// exitStatus records the process exit code chosen inside a routed command
// action, since mow.cli actions cannot return errors directly.
type exitStatus struct {
	code int
}

// fail marks a dispatched command as failed and returns true exactly once, so
// callers can report their diagnostic without printing twice.
func (s *exitStatus) fail() { s.code = 1 }

// buildApp constructs the Sqloid command surface: the `sqlite <file>` and
// `d1` commands, the version flags, and mow.cli's default help handling.
//
// Usage failures are reported by mow.cli on stderr; Main turns them into the
// exit status 2 required by the PRD. ContinueOnError keeps that decision
// inside this package instead of letting mow.cli call os.Exit directly.
// Handler failures print the handler's exact one-line diagnostic on stderr
// and set status 1 (Issue #2).
func buildApp(h Handlers, status *exitStatus) *cli.Cli {
	app := cli.App("sqloid", "Browse and edit SQLite databases.")
	// Propagate to subcommands; see cli.doInit, which copies this policy.
	app.ErrorHandling = flag.ContinueOnError

	showVersion := app.Bool(cli.BoolOpt{
		Name:      "v version",
		Desc:      "Show the version and exit",
		HideValue: true,
	})

	app.Command("sqlite", "Open a SQLite database file", func(cmd *cli.Cmd) {
		cmd.Spec = "FILE"
		file := cmd.String(cli.StringArg{
			Name:      "FILE",
			Value:     "",
			Desc:      "Path to the SQLite database file",
			HideValue: true,
		})
		cmd.Action = func() {
			if h.SQLite == nil {
				return
			}
			if err := h.SQLite(*file); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				status.fail()
			}
		}
	})

	app.Command("d1", "Open the local D1 database discovered in .wrangler", func(cmd *cli.Cmd) {
		cmd.Action = func() {
			if h.D1 == nil {
				return
			}
			if err := h.D1(); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				status.fail()
			}
		}
	})

	app.Action = func() {
		if *showVersion {
			fmt.Fprintf(os.Stdout, "sqloid %s\n", Version)
			return
		}
		app.PrintLongHelp()
	}

	return app
}

// New builds the Sqloid command surface for direct construction when a caller
// drives app.Run itself; handler failures within it are recorded only for
// reporting through Main, so interactive callers should prefer Main.
func New(h Handlers) *cli.Cli {
	return buildApp(h, &exitStatus{})
}

// Main runs the CLI and returns the process exit status without calling
// os.Exit, so both the real entrypoint and tests can control termination.
//
// Usage failures return 2 after mow.cli has already written the error and
// usage message to stderr. A routed command whose handler reports failure
// returns 1 after the exact one-line diagnostic was written to stderr.
// Successful dispatch returns 0 and adds no output of its own.
func Main(args []string, h Handlers) int {
	status := &exitStatus{}
	app := buildApp(h, status)
	if err := app.Run(args); err != nil {
		return 2
	}
	return status.code
}
