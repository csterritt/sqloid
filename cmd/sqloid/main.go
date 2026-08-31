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
