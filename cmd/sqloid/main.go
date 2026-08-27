// Command sqloid is the executable entrypoint for the Sqloid CLI. It maps the
// exit status returned by the internal/cli shell onto the process and supplies
// the sqlite command's session handler; all other construction and dispatch
// live in internal/cli.
package main

import (
	"os"

	"github.com/chris/sqloid/internal/cli"
	"github.com/chris/sqloid/internal/connection"
)

func main() {
	handlers := cli.Handlers{
		SQLite: connection.Session,
	}
	os.Exit(cli.Main(os.Args, handlers))
}
