// Command sqloid is the executable entrypoint for the Sqloid CLI. It only
// maps the exit status returned by the internal/cli shell onto the process;
// command construction and dispatch live in internal/cli.
package main

import (
	"os"

	"github.com/chris/sqloid/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args, cli.Handlers{}))
}
