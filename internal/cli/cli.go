// Package cli implements the Sqloid command-line shell: command routing,
// argument parsing, help, and version handling on top of the PRD-mandated
// mow.cli command structure.
//
// The package owns the `sqlite <file>` and `d1` command contracts from
// Notes/PRD-sqloid.md and Issue #1. Database startup itself stays behind the
// injectable Handlers so later work can connect internal/connection and
// internal/d1 without replacing this shell.
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
// A nil handler is treated as not yet wired and does nothing, which keeps the
// shell testable before internal/connection and internal/d1 exist.
type Handlers struct {
	// SQLite opens the database at the path passed as the `sqlite <file>`
	// argument. It is called only after successful routing.
	SQLite func(path string) error
	// D1 starts discovery of the local D1 database. It is called only after
	// successful routing.
	D1 func() error
}

// New builds the Sqloid command surface: the `sqlite <file>` and `d1`
// commands, the version flags, and mow.cli's default help handling.
//
// Usage failures are reported by mow.cli on stderr; Main turns them into the
// exit status 2 required by the PRD. ContinueOnError keeps that decision
// inside this package instead of letting mow.cli call os.Exit directly.
func New(h Handlers) *cli.Cli {
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
			// Startup-failure reporting and exit status are defined by
			// Issue #2; the shell stays silent here.
			_ = h.SQLite(*file)
		}
	})

	app.Command("d1", "Open the local D1 database discovered in .wrangler", func(cmd *cli.Cmd) {
		cmd.Action = func() {
			if h.D1 == nil {
				return
			}
			_ = h.D1()
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

// Main runs the CLI and returns the process exit status without calling
// os.Exit, so both the real entrypoint and tests can control termination.
// Usage failures return 2 after mow.cli has already written the error and
// usage message to stderr; successful dispatch returns 0 and adds no output
// of its own.
func Main(args []string, h Handlers) int {
	app := New(h)
	if err := app.Run(args); err != nil {
		return 2
	}
	return 0
}
